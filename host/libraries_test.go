package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryAnalysisPreservesSettingsAndFactsAndRequiresAuthentication(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalog, []byte(`{"libraries":[{"id":"Unused","name":"Unused","version":"1.0.0.0"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	command := fakeExtractorCommand(t, catalog, false)
	runtimeMu.Lock()
	previous := runtimeCache["csharp"]
	runtimeCache["csharp"] = &runtimeInfo{OK: true}
	runtimeMu.Unlock()
	t.Cleanup(func() {
		runtimeMu.Lock()
		defer runtimeMu.Unlock()
		if previous == nil {
			delete(runtimeCache, "csharp")
		} else {
			runtimeCache["csharp"] = previous
		}
	})
	file := filepath.Join(dir, "x.semaps")
	original := []byte(fmt.Sprintf("version: 1\nworkspace: ws\nextractors:\n  - id: cs\n    language: csharp\n    project: p\n    root: .\n    external: full\n    libraries: [Unused]\n    command: %q\n", command))
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	api := &toolAPI{file: file, models: &modelService{key: "key"}, runs: &runStore{dir: filepath.Join(dir, "runs")}}
	for _, tc := range []struct {
		body, auth, origin string
		status             int
	}{
		{`{}`, "", "", 401},
		{`{}`, "Bearer key", "http://another-host", 403},
		{`{"command":"other"}`, "Bearer key", "", 400},
		{`{"root":"` + filepath.ToSlash(dir) + `"}`, "Bearer key", "", 400},
		{`{"root":"probe","include":["probe"]}`, "Bearer key", "", 200},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/setup/extractors/cs/libraries", strings.NewReader(tc.body))
		req.SetPathValue("id", "cs")
		req.Header.Set("Authorization", tc.auth)
		req.Header.Set("Origin", tc.origin)
		recorder := httptest.NewRecorder()
		api.guard(api.listLibraries)(recorder, req)
		if recorder.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, recorder.Code, recorder.Body)
		}
		if tc.status == 200 && !strings.Contains(recorder.Body.String(), `"id":"Unused"`) {
			t.Fatal(recorder.Body.String())
		}
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("configuration changed: %v", err)
	}
	if len(api.runs.list("cs")) != 0 {
		t.Fatal("analysis created a facts run")
	}
}
