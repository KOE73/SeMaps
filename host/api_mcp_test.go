package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
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
