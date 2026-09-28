package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMCPHTTP(mux *http.ServeMux, proj project, workspace, sourceRoot string, models *modelService, onRunFinish func(*runInfo), settings *mcpSettingsBox) {
	s := &mcpServer{proj: proj, workspace: workspace, sourceRoot: sourceRoot, models: models, onRunFinish: onRunFinish, settings: settings}
	srv := s.server()
	// A change of mcp.tools/mcp.description (PUT /api/setup) rebuilds this
	// SAME *mcp.Server's graph tools in place and notifies every connected
	// session (RemoveTools/AddTool both trigger notifications/tools/list_changed,
	// PLAN_20260928-7 step 4) — the HTTP handler below hands every session
	// this one srv (NewStreamableHTTPHandler's factory always returns it), so
	// a running session sees the new list as soon as it next asks for it, and
	// a brand new session always sees the current one.
	if settings != nil {
		settings.OnChange(s.rebuildGraphTools(srv))
	}
	root := proj.Root
	if root == "" {
		root = filepath.Dir(workspace)
	}
	srv.AddReceivingMiddleware(callLog(root, os.Stderr))
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !models.authorize(w, r) {
			return
		}
		handler.ServeHTTP(w, r)
	}))
}
