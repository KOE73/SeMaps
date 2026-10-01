package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPutSetupMcpTakesEffectLive (PLAN_20260928-7 step 2): a PUT /api/setup
// with an `mcp` patch is written to the file AND reaches every reader of the
// shared mcpSettingsBox (the HTTP graph endpoint and the MCP server both
// hold the same box) without a restart — checked here by reading the box
// directly, exactly as graphService.serve and mcpServer.getGraph do.
func TestPutSetupMcpTakesEffectLive(t *testing.T) {
	file := writeProject(t, "name: Demo\n")
	proj, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	box := newMcpSettingsBox(proj.Mcp)
	if got := box.Get(); got.Format != "facts" || got.Limit != 200 || got.ListCap != 50 {
		t.Fatalf("expected the defaults before any PUT, got %+v", got)
	}

	models, err := newModelService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &toolAPI{file: file, workspace: models.workspace, models: models, settings: box}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/setup", api.guard(api.putSetup))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/setup", strings.NewReader(`{"mcp":{"format":"lines","listCap":5,"limit":10}}`))
	req.Header.Set("Authorization", "Bearer "+models.key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/setup: %d", res.StatusCode)
	}

	got := box.Get()
	if got.Format != "lines" || got.ListCap != 5 || got.Limit != 10 {
		t.Fatalf("expected the box to carry the PUT values at once, got %+v", got)
	}

	// The file itself was patched too, and re-loading it agrees with the box.
	reloaded, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mcp != got {
		t.Fatalf("file %+v does not match the live box %+v", reloaded.Mcp, got)
	}
}

// TestPutSetupMcpRejectsBadValue: a bad mcp value refuses the request and
// leaves the box (and the file) exactly as they were.
func TestPutSetupMcpRejectsBadValue(t *testing.T) {
	file := writeProject(t, "name: Demo\n")
	proj, _ := loadProject(file)
	box := newMcpSettingsBox(proj.Mcp)
	models, err := newModelService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &toolAPI{file: file, workspace: models.workspace, models: models, settings: box}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/setup", api.guard(api.putSetup))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/setup", strings.NewReader(`{"mcp":{"tools":"wide"}}`))
	req.Header.Set("Authorization", "Bearer "+models.key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	if got := box.Get(); got.Tools != "one" {
		t.Fatalf("expected the box untouched, got %+v", got)
	}
}
