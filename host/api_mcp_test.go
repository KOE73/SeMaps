package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPInstallKeepsOtherServers(t *testing.T) {
	dir := t.TempDir()
	api := &toolAPI{file: filepath.Join(dir, "x.semaps")}
	os.WriteFile(filepath.Join(dir, mcpFile), []byte(`{"mcpServers":{"other":{"command":"x"}},"keep":1}`), 0o644)

	if st := api.readMCP(); st.Configured || !st.Exists || len(st.Tools) < 10 {
		t.Fatalf("before: %+v", st)
	}
	rec := httptest.NewRecorder()
	api.installMCP(rec, httptest.NewRequest("POST", "/api/mcp/install", nil))
	if rec.Code != 200 {
		t.Fatalf("install: %d %s", rec.Code, rec.Body)
	}
	var file map[string]any
	data, _ := os.ReadFile(filepath.Join(dir, mcpFile))
	json.Unmarshal(data, &file)
	servers := file["mcpServers"].(map[string]any)
	if servers["other"] == nil || servers["semaps"] == nil || file["keep"] == nil {
		t.Fatalf("file: %s", data)
	}
	if st := api.readMCP(); !st.Configured || st.Entry != "semaps" {
		t.Fatalf("after: %+v", st)
	}
}

func TestMCPInstallCreatesFileAndRefusesBrokenOne(t *testing.T) {
	dir := t.TempDir()
	api := &toolAPI{file: filepath.Join(dir, "x.semaps")}
	rec := httptest.NewRecorder()
	api.installMCP(rec, httptest.NewRequest("POST", "/api/mcp/install", nil))
	if rec.Code != 200 || !api.readMCP().Configured {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	os.WriteFile(filepath.Join(dir, mcpFile), []byte(`{broken`), 0o644)
	rec = httptest.NewRecorder()
	api.installMCP(rec, httptest.NewRequest("POST", "/api/mcp/install", nil))
	if rec.Code != 409 {
		t.Fatalf("broken file overwritten: %d", rec.Code)
	}
}

func TestMCPCallRecordsTheWire(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.semaps")
	os.WriteFile(file, []byte("version: 1\nname: t\nworkspace: ws\n"), 0o644)
	p := filepath.Join(dir, "ws", "projects", "p")
	os.MkdirAll(p, 0o755)
	os.WriteFile(filepath.Join(p, "project.json"), []byte(`{"id":"p"}`), 0o644)
	os.WriteFile(filepath.Join(p, "entities.json"), []byte(`{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`), 0o644)
	models, err := newModelService(filepath.Join(dir, "ws"))
	if err != nil {
		t.Fatal(err)
	}
	api := &toolAPI{file: file, workspace: filepath.Join(dir, "ws"), models: models}
	mux := http.NewServeMux()
	registerMCPHTTP(mux, project{Root: dir}, api.workspace, dir, models)
	mux.HandleFunc("POST /api/mcp/call", api.callMCP)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	body := strings.NewReader(`{"name":"get_entity","arguments":{"id":"e_a"}}`)
	response, err := srv.Client().Post(srv.URL+"/api/mcp/call", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("%d", response.StatusCode)
	}
	var res mcpCallResult
	json.NewDecoder(response.Body).Decode(&res)
	if res.IsError || res.Error != "" || len(res.Messages) != 2 || res.Messages[0].Dir != "out" || res.Messages[1].Dir != "in" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(string(res.Messages[0].Message), `"tools/call"`) || !strings.Contains(string(res.Messages[1].Message), `e_a`) {
		t.Fatalf("wire: %s / %s", res.Messages[0].Message, res.Messages[1].Message)
	}

	if st := api.readMCP(); len(st.Tools) == 0 || st.Tools[0].InputSchema == nil {
		t.Fatal("tools without schema")
	}
}
