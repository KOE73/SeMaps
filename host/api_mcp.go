package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP mode of the editor (docs/API.md §5): whether the project's
// .mcp.json starts `semaps mcp`, and writing that entry on request. The file
// is the one an agent's client reads at the project root, next to .semaps.

const mcpFile = ".mcp.json"

type mcpToolView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
	InputSchema any    `json:"inputSchema"`
}

type mcpStatus struct {
	File       string         `json:"file"`       // relative to the project root
	Exists     bool           `json:"exists"`     // the file is there
	Configured bool           `json:"configured"` // it has an mcpServers entry running `semaps mcp`
	Entry      string         `json:"entry"`      // the name of that entry
	OnPath     bool           `json:"onPath"`     // `semaps` resolves on PATH, so the entry can say just "semaps"
	Snippet    string         `json:"snippet"`    // what install writes
	Tools      []mcpToolView  `json:"tools"`
	Error      string         `json:"error,omitempty"`
	raw        map[string]any `json:"-"`
	servers    map[string]any `json:"-"`
}

func (api *toolAPI) mcpPath() string { return filepath.Join(filepath.Dir(api.file), mcpFile) }

func mcpEntry() map[string]any { return map[string]any{"command": "semaps", "args": []string{"mcp"}} }

// readMCP reads .mcp.json; a missing file is not an error.
func (api *toolAPI) readMCP() mcpStatus {
	st := mcpStatus{File: mcpFile, Tools: mcpTools()}
	_, err := exec.LookPath("semaps")
	st.OnPath = err == nil
	snippet, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"semaps": mcpEntry()}}, "", "  ")
	st.Snippet = string(snippet)

	data, err := os.ReadFile(api.mcpPath())
	if errors.Is(err, fs.ErrNotExist) {
		return st
	}
	st.Exists = true
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if err := json.Unmarshal(data, &st.raw); err != nil {
		st.Error = mcpFile + ": " + err.Error()
		return st
	}
	st.servers, _ = st.raw["mcpServers"].(map[string]any)
	for name, v := range st.servers {
		e, _ := v.(map[string]any)
		args, _ := e["args"].([]any)
		cmd, _ := e["command"].(string)
		base := filepath.Base(cmd)
		if (base == "semaps" || base == "semaps.exe") && len(args) > 0 && args[0] == "mcp" {
			st.Configured, st.Entry = true, name
			break
		}
	}
	return st
}

func (api *toolAPI) getMCP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.readMCP())
}

// installMCP adds the `semaps` entry to .mcp.json, keeping every other server
// and key; a file that does not parse is left alone.
func (api *toolAPI) installMCP(w http.ResponseWriter, r *http.Request) {
	st := api.readMCP()
	if st.Error != "" {
		http.Error(w, st.Error, http.StatusConflict)
		return
	}
	if !st.Configured {
		if st.raw == nil {
			st.raw = map[string]any{}
		}
		if st.servers == nil {
			st.servers = map[string]any{}
		}
		st.servers["semaps"] = mcpEntry()
		st.raw["mcpServers"] = st.servers
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st.raw); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(api.mcpPath(), b.Bytes(), 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, api.readMCP())
}

// mcpTools lists the tools of `semaps mcp` as the server itself declares them.
func mcpTools() []mcpToolView {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := (&mcpServer{}).server()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "semaps-editor"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return nil
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		return nil
	}
	out := make([]mcpToolView, len(res.Tools))
	for i, t := range res.Tools {
		out[i] = mcpToolView{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema,
			ReadOnly: t.Annotations != nil && t.Annotations.ReadOnlyHint}
	}
	return out
}

// ------------------------------------------------------------------ sandbox

type mcpCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// mcpMessage is one JSON-RPC message of a sandbox call as it went over the
// wire: out — from the client to the server, in — back.
type mcpMessage struct {
	Dir     string          `json:"dir"`
	Message json.RawMessage `json:"message"`
}

type mcpCallResult struct {
	Messages []mcpMessage `json:"messages"`
	Ms       int64        `json:"ms"`
	IsError  bool         `json:"isError"`
	Error    string       `json:"error,omitempty"` // the call failed below the tool: transport, protocol
}

// callMCP runs one tool of `semaps mcp` on this project, as an agent would:
// a fresh session over an in-memory transport, the handshake left out of the
// record, the call and its answer recorded as they went over the wire. The
// tool acts for real — a writing tool writes.
func (api *toolAPI) callMCP(w http.ResponseWriter, r *http.Request) {
	var req mcpCallRequest
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		http.Error(w, "expected {name, arguments}", http.StatusBadRequest)
		return
	}
	proj, err := loadProject(api.file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root, _ := filepath.Abs(proj.SourceRoot)
	srv := (&mcpServer{proj: proj, workspace: api.workspace, sourceRoot: root}).server()
	srv.AddReceivingMiddleware(callLog(proj.Root, nil)) // the sandbox's calls land in the same log

	ctx := r.Context()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ss.Close()
	var wire lockedBuffer
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "semaps-sandbox"}, nil).
		Connect(ctx, &mcp.LoggingTransport{Transport: ct, Writer: &wire}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cs.Close()
	wire.Reset() // the handshake is not the call

	start := time.Now()
	res, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: req.Name, Arguments: req.Arguments})
	out := mcpCallResult{Ms: time.Since(start).Milliseconds(), Messages: []mcpMessage{}}
	if callErr != nil {
		out.Error = callErr.Error()
	} else {
		out.IsError = res.IsError
	}
	for _, line := range strings.Split(wire.String(), "\n") {
		dir, body, ok := strings.Cut(line, ": ")
		if !ok || (dir != "write" && dir != "read") || !json.Valid([]byte(body)) {
			continue
		}
		d := "out"
		if dir == "read" {
			d = "in"
		}
		out.Messages = append(out.Messages, mcpMessage{Dir: d, Message: json.RawMessage(body)})
	}
	writeJSON(w, out)
}

// lockedBuffer: the logging transport writes from the session's goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Reset()
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
