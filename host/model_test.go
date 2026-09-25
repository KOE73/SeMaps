package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"semaps/core"
)

func TestStructuralReloadRequiresCleanModel(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	m, err := s.get("p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]core.Op{{Kind: "project", ID: "p", Value: json.RawMessage(`{"id":"p","title":"New","languages":["ru"]}`)}}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := s.clean("p"); err == nil {
		t.Fatal("dirty project accepted for structural operation")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.clean("p"); err != nil {
		t.Fatal(err)
	}
	if err := s.reload(projectReloaded{OldProject: "p", NewProject: "p", NewView: "v_new"}); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.get("p")
	if err != nil || fresh == m {
		t.Fatalf("model not reloaded: %v", err)
	}
}

func hostModelFixture(t *testing.T) (*modelService, *httptest.Server) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for file, body := range map[string]string{
		"project.json":         `{"id":"p","languages":["ru"]}`,
		"entities.json":        `{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`,
		"relations.json":       `{"relations":[]}`,
		"relation-types.json":  `{"relationTypes":[]}`,
		"views/main.view.json": `{"id":"v_main","project":"p","axis":"axis_test","zones":[],"nodes":[{"entity":"e_a","zone":null,"x":1,"y":2}]}`,
	} {
		p := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.register(mux)
	return s, httptest.NewServer(mux)
}

func modelRequest(t *testing.T, c *http.Client, url, key, method, body string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestModelAPIAuthSnapshotSaveDiscardAndRules(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	c := srv.Client()
	res := modelRequest(t, c, srv.URL+"/api/model/p", "", "GET", "")
	var snapshot map[string]any
	if err := json.NewDecoder(res.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || snapshot["registry"] == nil || snapshot["views"] == nil {
		t.Fatalf("snapshot: %d %v", res.StatusCode, snapshot)
	}
	op := `{"client":"one","ops":[{"kind":"entity","id":"e_a","value":{"id":"e_a","name":"Edited","kind":"class"}}]}`
	res = modelRequest(t, c, srv.URL+"/api/model/p/ops", "", "POST", op)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("missing key: %d", res.StatusCode)
	}
	res = modelRequest(t, c, srv.URL+"/api/model/p/ops", s.key, "POST", `{"ops":[{"kind":"entity","id":"e_a","value":null}]}`)
	res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("rule: %d", res.StatusCode)
	}
	res = modelRequest(t, c, srv.URL+"/api/model/p/ops", s.key, "POST", op)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("ops: %d", res.StatusCode)
	}
	file := filepath.Join(s.workspace, "projects", "p", "entities.json")
	b, _ := os.ReadFile(file)
	if bytes.Contains(b, []byte("Edited")) {
		t.Fatal("unsaved edit wrote file")
	}
	res = modelRequest(t, c, srv.URL+"/api/model/p/save", "", "GET", "")
	var dirty map[string]any
	json.NewDecoder(res.Body).Decode(&dirty)
	res.Body.Close()
	if len(dirty["registry"].([]any)) != 1 {
		t.Fatalf("dirty: %v", dirty)
	}
	res = modelRequest(t, c, srv.URL+"/api/model/p/discard", s.key, "POST", `{"scope":"registry"}`)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("discard: %d", res.StatusCode)
	}
	res = modelRequest(t, c, srv.URL+"/api/model/p/ops", s.key, "POST", op)
	res.Body.Close()
	res = modelRequest(t, c, srv.URL+"/api/model/p/save", s.key, "POST", "")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("save: %d", res.StatusCode)
	}
	b, _ = os.ReadFile(file)
	if !bytes.Contains(b, []byte("Edited")) {
		t.Fatal("save did not write file")
	}
}

func TestModelAPIEventAtOtherSubscriber(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	c := srv.Client()
	c.Timeout = 3 * time.Second
	res := modelRequest(t, c, srv.URL+"/api/events?project=p", "", "GET", "")
	defer res.Body.Close()
	scan := bufio.NewScanner(res.Body)
	if !scan.Scan() {
		t.Fatal("no SSE greeting")
	}
	op := `{"client":"one","ops":[{"kind":"entity","id":"e_a","value":{"id":"e_a","name":"Changed","kind":"class"}}]}`
	w := modelRequest(t, c, srv.URL+"/api/model/p/ops", s.key, "POST", op)
	io.Copy(io.Discard, w.Body)
	w.Body.Close()
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, `"client":"one"`) || !strings.Contains(line, `"author":"human"`) {
				t.Fatalf("event: %s", line)
			}
			return
		}
	}
	t.Fatalf("no SSE event: %v", scan.Err())
}
