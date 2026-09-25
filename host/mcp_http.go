package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMCPHTTP(mux *http.ServeMux, proj project, workspace, sourceRoot string, models *modelService) {
	s := &mcpServer{proj: proj, workspace: workspace, sourceRoot: sourceRoot, models: models}
	srv := s.server()
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
