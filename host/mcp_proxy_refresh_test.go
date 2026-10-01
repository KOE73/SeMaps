package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoLoudIn struct {
	Text string `json:"text"`
	Loud bool   `json:"loud"`
}

// fakeHost is an upstream MCP server the proxy tests point at: a version tag
// tells which "build" answered, and the served server can be replaced (a host
// restarted with a newer build) or changed while it runs.
type fakeHost struct {
	mu  sync.Mutex
	srv *mcp.Server
	h   http.Handler
}

func newFakeHost(srv *mcp.Server) *fakeHost {
	f := &fakeHost{}
	f.use(srv)
	return f
}

func (f *fakeHost) use(srv *mcp.Server) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.srv = srv
	f.h = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
}

func (f *fakeHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	h := f.h
	f.mu.Unlock()
	h.ServeHTTP(w, r)
}

func hostBuild(version string, extraTool string, loud bool) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: version}, nil)
	if loud {
		mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "echo " + version}, func(_ context.Context, _ *mcp.CallToolRequest, in echoLoudIn) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: version + ":" + in.Text}}}, nil, nil
		})
	} else {
		mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "echo " + version}, func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: version + ":" + in.Text}}}, nil, nil
		})
	}
	mcp.AddTool(srv, &mcp.Tool{Name: extraTool}, func(context.Context, *mcp.CallToolRequest, echoIn) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: extraTool}}}, nil, nil
	})
	return srv
}

// agent is the stdio side: a client connected to the proxy's local server that
// counts notifications/tools/list_changed.
type agent struct {
	session *mcp.ClientSession
	changed chan struct{}
}

func connectAgent(t *testing.T, p *mcpProxy) *agent {
	t.Helper()
	a := &agent{changed: make(chan struct{}, 32)}
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := p.local.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "agent"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { a.changed <- struct{}{} },
	})
	a.session, err = client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.session.Close() })
	return a
}

func (a *agent) tools(t *testing.T) map[string]*mcp.Tool {
	t.Helper()
	res, err := a.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		out[tool.Name] = tool
	}
	return out
}

func names(m map[string]*mcp.Tool) string {
	var n []string
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return strings.Join(n, ",")
}

func (a *agent) waitChanged(t *testing.T) {
	t.Helper()
	select {
	case <-a.changed:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent was not told that the tools changed")
	}
}

func (a *agent) call(t *testing.T, name string, args map[string]any) (string, error) {
	t.Helper()
	res, err := a.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("tool error: %+v", res)
	}
	return res.Content[0].(*mcp.TextContent).Text, nil
}

func proxyTo(t *testing.T, url func() string) *mcpProxy {
	t.Helper()
	p := newMCPProxy("", func(context.Context) (string, string, error) { return url(), "key", nil })
	if err := p.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p.session != nil {
			p.session.Close()
		}
	})
	return p
}

func stop(ts *httptest.Server) {
	ts.CloseClientConnections()
	ts.Close()
}

// A host restarted with a newer build (another tool set, another schema): the
// session the proxy holds is gone; the next call reconnects, retries and
// reaches the new host, and the agent is told the list changed and finds the
// new list.
func TestProxyFollowsAHostRestartedWithANewBuild(t *testing.T) {
	old := newFakeHost(hostBuild("v1", "old_only", false))
	ts := httptest.NewServer(old)
	defer func() { stop(ts) }()
	p := proxyTo(t, func() string { return ts.URL })
	a := connectAgent(t, p)

	first := a.tools(t)
	if names(first) != "echo,old_only" || strings.Contains(fmtSchema(first["echo"]), "loud") {
		t.Fatalf("tools %s", names(first))
	}
	if got, err := a.call(t, "echo", map[string]any{"text": "a"}); err != nil || got != "v1:a" {
		t.Fatalf("v1: %q %v", got, err)
	}

	// the restart: the same address now answers with a different build; the old session id is unknown to it
	old.use(hostBuild("v2", "new_only", true))
	if got, err := a.call(t, "echo", map[string]any{"text": "b", "loud": true}); err != nil || got != "v2:b" {
		t.Fatalf("after the restart: %q %v", got, err)
	}
	a.waitChanged(t)
	now := a.tools(t)
	if names(now) != "echo,new_only" || !strings.Contains(fmtSchema(now["echo"]), "loud") || now["echo"].Description != "echo v2" {
		t.Fatalf("the list did not follow: %s %s", names(now), fmtSchema(now["echo"]))
	}
	if got, err := a.call(t, "new_only", map[string]any{"text": "x"}); err != nil || got != "new_only" {
		t.Fatalf("new tool: %q %v", got, err)
	}
}

// The host comes back on another port (host.json says so): the call after it
// reconnects through the endpoint function, which is what re-reads host.json.
func TestProxyReconnectsToAHostOnAnotherPort(t *testing.T) {
	ts1 := httptest.NewServer(newFakeHost(hostBuild("v1", "one", false)))
	var mu sync.Mutex
	url := ts1.URL
	p := proxyTo(t, func() string { mu.Lock(); defer mu.Unlock(); return url })
	a := connectAgent(t, p)
	if got, err := a.call(t, "echo", map[string]any{"text": "a"}); err != nil || got != "v1:a" {
		t.Fatalf("v1: %q %v", got, err)
	}
	stop(ts1)
	ts2 := httptest.NewServer(newFakeHost(hostBuild("v2", "two", false)))
	defer stop(ts2)
	mu.Lock()
	url = ts2.URL
	mu.Unlock()
	if got, err := a.call(t, "echo", map[string]any{"text": "b"}); err != nil || got != "v2:b" {
		t.Fatalf("after the move: %q %v", got, err)
	}
	if got := names(a.tools(t)); got != "echo,two" {
		t.Fatalf("tools %s", got)
	}
}

// A host that changes its tools while running (a settings change) tells its
// sessions; the proxy re-lists and passes the news on. A call made against the
// old schema is refused by the host and the refusal reaches the agent as it is.
func TestProxyFollowsToolsChangedByARunningHost(t *testing.T) {
	srv := hostBuild("v1", "going", false)
	ts := httptest.NewServer(newFakeHost(srv))
	defer stop(ts)
	p := proxyTo(t, func() string { return ts.URL })
	a := connectAgent(t, p)
	if names(a.tools(t)) != "echo,going" {
		t.Fatal("start")
	}
	srv.RemoveTools("going")
	mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "echo v1b"}, func(_ context.Context, _ *mcp.CallToolRequest, in echoLoudIn) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "v1b:" + in.Text}}}, nil, nil
	})
	a.waitChanged(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		tools := a.tools(t)
		if names(tools) == "echo" && tools["echo"].Description == "echo v1b" && strings.Contains(fmtSchema(tools["echo"]), "loud") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the proxy did not follow: %s", names(tools))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The host refuses a call: its error reaches the agent unchanged, and the
// proxy has re-listed by the time the agent sees it.
func TestProxyReturnsTheHostsRefusalAndRefreshesTheList(t *testing.T) {
	srv := hostBuild("v1", "gone", false)
	ts := httptest.NewServer(newFakeHost(srv))
	defer stop(ts)
	p := proxyTo(t, func() string { return ts.URL })
	a := connectAgent(t, p)
	a.tools(t)
	// the host drops a tool; the proxy has not looked yet, its local list still has it
	p.mu.Lock()
	srv.RemoveTools("gone")
	p.mu.Unlock()
	_, err := a.call(t, "gone", nil)
	var wire *jsonrpc.Error
	if err == nil || !errors.As(err, &wire) {
		t.Fatalf("want the host's protocol error, got %v", err)
	}
	// refresh ran before the answer: the local list has lost the tool without another call
	if got := names(a.tools(t)); got != "echo" {
		t.Fatalf("tools %s", got)
	}
}

func TestProxyRefreshWithoutChangeSendsNothing(t *testing.T) {
	ts := httptest.NewServer(newFakeHost(hostBuild("v1", "x", false)))
	defer stop(ts)
	p := proxyTo(t, func() string { return ts.URL })
	a := connectAgent(t, p)
	a.tools(t)
	for i := 0; i < 3; i++ {
		if err := p.refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-a.changed:
		t.Fatal("a refresh that found nothing new sent list_changed")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestProxyInjectsTheProjectArgument(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "fake"}, nil)
	type projIn struct {
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "get_entity"}, func(_ context.Context, _ *mcp.CallToolRequest, in projIn) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: in.Project + "/" + in.ID}}}, nil, nil
	})
	ts := httptest.NewServer(newFakeHost(srv))
	defer stop(ts)
	p := newMCPProxy("pp", func(context.Context) (string, string, error) { return ts.URL, "key", nil })
	if err := p.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := connectAgent(t, p)
	if got, err := a.call(t, "get_entity", map[string]any{"id": "e_a"}); err != nil || got != "pp/e_a" {
		t.Fatalf("%q %v", got, err)
	}
}

func fmtSchema(t *mcp.Tool) string {
	if t == nil {
		return ""
	}
	b, _ := json.Marshal(t.InputSchema)
	return string(b)
}
