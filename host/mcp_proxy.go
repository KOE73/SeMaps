package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type hostInfo struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func hostEndpoint(proj project, workspace string) (string, string, error) {
	file := filepath.Join(proj.Root, ".semaps", "host.json")
	read := func() (string, string, bool) {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", "", false
		}
		var h hostInfo
		if json.Unmarshal(b, &h) != nil || h.Port == 0 || h.Key == "" {
			return "", "", false
		}
		base := fmt.Sprintf("http://localhost:%d", h.Port)
		c := http.Client{Timeout: 500 * time.Millisecond}
		res, err := c.Get(base + "/api/info")
		if err != nil {
			return "", "", false
		}
		defer res.Body.Close()
		var info struct{ Workspace string }
		if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&info) != nil || !strings.EqualFold(filepath.Clean(info.Workspace), filepath.Clean(workspace)) {
			return "", "", false
		}
		return base + "/mcp", h.Key, true
	}
	if endpoint, key, ok := read(); ok {
		return endpoint, key, nil
	}
	if err := startBackgroundHost(proj.File); err != nil {
		return "", "", err
	}
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if endpoint, key, ok := read(); ok {
			return endpoint, key, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", "", fmt.Errorf("host did not start within 15 seconds")
}

type bearerTransport struct {
	key  string
	next http.RoundTripper
}

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.key)
	return t.next.RoundTrip(r)
}

// mcpProxy is the stdio server an agent talks to. It forwards to the host's
// HTTP endpoint and keeps its own tool list equal to the host's: the host can
// be restarted with a newer build (new tools, changed schemas, another port)
// or change its tools while running (a settings change), and the agent's
// session must not go stale (docs/API.md §6).
type mcpProxy struct {
	local     *mcp.Server
	projectID string
	// endpoint finds the host: it re-reads .semaps/host.json (and starts the host
	// when none answers), so a new port or key is found on every connect.
	endpoint func(ctx context.Context) (url, key string, err error)
	client   *mcp.Client

	mu      sync.Mutex
	session *mcp.ClientSession
	tools   map[string]string // name -> JSON of the tool as registered on local
}

func newMCPProxy(projectID string, endpoint func(ctx context.Context) (string, string, error)) *mcpProxy {
	// HasTools: the server advertises tools.listChanged from the start, so a
	// client knows a later AddTool/RemoveTools is announced.
	local := mcp.NewServer(&mcp.Implementation{Name: "semaps", Version: "1"}, &mcp.ServerOptions{HasTools: true})
	p := &mcpProxy{local: local, projectID: projectID, endpoint: endpoint, tools: map[string]string{}}
	p.client = mcp.NewClient(&mcp.Implementation{Name: "semaps-stdio-proxy", Version: "1"}, &mcp.ClientOptions{
		// the host tells when its tools change while it runs; the handler must
		// not list on the session's own read loop, hence the goroutine
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = p.refresh(ctx)
			}()
		},
	})
	return p
}

// dial opens a session to the host, wherever it is now.
func (p *mcpProxy) dial(ctx context.Context) (*mcp.ClientSession, error) {
	url, key, err := p.endpoint(ctx)
	if err != nil {
		return nil, err
	}
	transport := &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: bearerTransport{key: key, next: http.DefaultTransport}}}
	return p.client.Connect(ctx, transport, nil)
}

// connect opens the first session and registers the host's tools.
func (p *mcpProxy) connect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connectLocked(ctx)
}

func (p *mcpProxy) connectLocked(ctx context.Context) error {
	session, err := p.dial(ctx)
	if err != nil {
		return err
	}
	if err := p.syncLocked(ctx, session); err != nil {
		session.Close()
		return err
	}
	p.session = session
	return nil
}

// reconnect replaces a session that failed. When another call already did,
// its session is used and nothing is opened twice.
func (p *mcpProxy) reconnect(ctx context.Context, failed *mcp.ClientSession) (*mcp.ClientSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil && p.session != failed {
		return p.session, nil
	}
	if failed != nil {
		go failed.Close() // a dead host may make Close wait
	}
	p.session = nil
	if err := p.connectLocked(ctx); err != nil {
		return nil, err
	}
	return p.session, nil
}

func (p *mcpProxy) current(ctx context.Context) (*mcp.ClientSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session == nil {
		if err := p.connectLocked(ctx); err != nil {
			return nil, err
		}
	}
	return p.session, nil
}

// refresh re-lists the host's tools through its current session.
func (p *mcpProxy) refresh(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session == nil {
		return nil
	}
	return p.syncLocked(ctx, p.session)
}

// syncLocked makes local's tools the host's: a tool that is new or whose
// description, schemas or annotations differ is (re)registered, one the host no
// longer has is removed. Each change makes the SDK send
// notifications/tools/list_changed to the connected agent (debounced); nothing
// is touched, hence nothing is sent, while the lists agree.
func (p *mcpProxy) syncLocked(ctx context.Context, session *mcp.ClientSession) error {
	next := map[string]string{}
	var fresh []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		b, _ := json.Marshal(tool)
		next[tool.Name] = string(b)
		if p.tools[tool.Name] != string(b) {
			fresh = append(fresh, tool)
		}
	}
	var gone []string
	for name := range p.tools {
		if _, ok := next[name]; !ok {
			gone = append(gone, name)
		}
	}
	for _, tool := range fresh {
		name := tool.Name
		p.local.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return p.forward(ctx, name, req)
		})
	}
	if len(gone) > 0 {
		p.local.RemoveTools(gone...)
	}
	p.tools = next
	return nil
}

func (p *mcpProxy) forward(ctx context.Context, name string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]any
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
	}
	if p.projectID != "" && projectTool(name) {
		if args == nil {
			args = map[string]any{}
		}
		if _, ok := args["project"]; !ok {
			args["project"] = p.projectID
		}
	}
	return p.call(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}

// call forwards one tool call. When the host does not answer at all (restarted,
// another port, its session gone) it reconnects — which re-lists the tools —
// and tries once more. When the host answers with an error, that error goes to
// the agent as it is, after the list was refreshed: a call made with a stale
// schema is refused by the new host, and the next one is made against the
// list the agent has been told changed.
func (p *mcpProxy) call(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	session, err := p.current(ctx)
	if err != nil {
		return nil, err
	}
	res, err := session.CallTool(ctx, params)
	if err != nil && !hostAnswered(err) && ctx.Err() == nil {
		next, rerr := p.reconnect(ctx, session)
		if rerr != nil {
			return nil, fmt.Errorf("%w; reconnecting to the host: %v", err, rerr)
		}
		res, err = next.CallTool(ctx, params)
	}
	if err != nil && hostAnswered(err) {
		_ = p.refresh(ctx)
	}
	return res, err
}

// codeRejectedByTransport is the SDK's code for a request its transport could
// not deliver (host down, connection refused): not an answer of the host.
const codeRejectedByTransport = -32005

// hostAnswered: err is the host's own JSON-RPC error (unknown tool, arguments
// its schema refuses...), not a failure to reach it or a lost session.
func hostAnswered(err error) bool {
	if errors.Is(err, mcp.ErrConnectionClosed) || errors.Is(err, mcp.ErrSessionMissing) {
		return false
	}
	var wire *jsonrpc.Error
	return errors.As(err, &wire) && wire.Code != codeRejectedByTransport
}

func runMCPProxy(proj project, workspace, projectID string) int {
	p := newMCPProxy(projectID, func(context.Context) (string, string, error) { return hostEndpoint(proj, workspace) })
	if err := p.connect(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	if err := p.local.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	return 0
}

func projectTool(name string) bool {
	switch name {
	case "list_views", "get_entity", "find_entities", "get_relations", "get_kinds", "get_view", "get_text", "set_text", "mark_translated", "set_view_axis", "add_entity", "add_relation", "set_relation_visible", "confirm_rename", "place_entities", "move_elements", "resize_elements", "set_parent", "set_placement", "set_routing", "add_container", "fit_container", "align_elements", "remove", "save", "discard", "render_view", "create_view":
		return true
	}
	return false
}
