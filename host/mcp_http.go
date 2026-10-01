package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverHistoryCap bounds mcpServerPool.servers (below): the Go SDK gives no
// way to learn that a *mcp.Server's last session has closed, so the pool
// cannot simply drop a server once nobody uses it any more. Instead it keeps
// only the most recent serverHistoryCap servers; one older than that still
// answers any request an open session sends it, but stops receiving tool-
// list rebuilds and keeps whichever Instructions it was built with — an
// accepted bound on memory for a long-running host, not a correctness
// promise for a session that outlives eight settings changes.
const serverHistoryCap = 8

// mcpServerPool holds every *mcp.Server built for `/mcp` that may still have
// an open session, so that a settings change (PUT /api/setup) can reach
// BOTH kinds of session it promises to reach at once (docs/API.md §6): a
// session already connected sees the new tool list on its next
// `tools/list` (RemoveTools/AddTool on that very server, unchanged since
// PLAN_20260928-7 step 4), and a session that connects from now on gets a
// brand NEW server — built through the same `build` function, so logging
// middleware and every other tool are identical — with the new Instructions
// baked in at construction (the SDK has no setter for a live server's
// Instructions, ADR/README: none — see host/mcp.go's server() comment).
// `current` always hands new sessions the newest server.
type mcpServerPool struct {
	mu      sync.Mutex
	build   func() *mcp.Server
	servers []*mcp.Server // oldest first; the last one is current
}

func newMcpServerPool(build func() *mcp.Server) *mcpServerPool {
	return &mcpServerPool{build: build, servers: []*mcp.Server{build()}}
}

func (p *mcpServerPool) current() *mcp.Server {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.servers[len(p.servers)-1]
}

// onSettingsChange is registered as mcpSettingsBox.onChange: every server
// still in the bounded history gets its graph tools rebuilt in place with
// the NEW settings (mcpServer.rebuildGraphTools, unchanged), so a session
// mid-call on an older server still sees the new tool list; then a fresh
// server is built and becomes current, so every session opened after this
// point — the stdio proxy included, next time it starts `semaps mcp` — gets
// the new tools AND the new Instructions from the first message on.
func (p *mcpServerPool) onSettingsChange(s *mcpServer) func(mcpSettings) {
	return func(v mcpSettings) {
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, old := range p.servers {
			s.rebuildGraphTools(old)(v)
		}
		p.servers = append(p.servers, p.build())
		if len(p.servers) > serverHistoryCap {
			p.servers = p.servers[len(p.servers)-serverHistoryCap:]
		}
	}
}

func registerMCPHTTP(mux *http.ServeMux, proj project, workspace, sourceRoot string, models *modelService, onRunFinish func(*runInfo), settings *mcpSettingsBox) {
	s := &mcpServer{proj: proj, workspace: workspace, sourceRoot: sourceRoot, models: models, onRunFinish: onRunFinish, settings: settings}
	root := proj.Root
	if root == "" {
		root = filepath.Dir(workspace)
	}
	// build is the one place a live *mcp.Server is constructed, so a
	// rebuilt server (on a settings change, above) is given everything a
	// freshly started host gives its first one: the same tools, the same
	// call-logging middleware.
	build := func() *mcp.Server {
		srv := s.server()
		srv.AddReceivingMiddleware(callLog(root, os.Stderr))
		return srv
	}
	pool := newMcpServerPool(build)
	if settings != nil {
		settings.OnChange(pool.onSettingsChange(s))
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return pool.current() }, nil)
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !models.authorize(w, r) {
			return
		}
		handler.ServeHTTP(w, r)
	}))
}
