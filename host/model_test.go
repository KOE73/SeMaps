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

// The editor (HTTP) and agents (MCP) create through one service and read one
// catalog: what one writes, the other sees before Save.
func TestCreateThroughHTTPAndMCPShareOneCatalog(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	c := srv.Client()
	post := func(url, body string) int {
		res := modelRequest(t, c, srv.URL+url, s.key, http.MethodPost, body)
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("/api/projects", `{"id":"ov","title":"Overview","language":"ru"}`); code != 200 {
		t.Fatalf("create project: %d", code)
	}
	if code := post("/api/projects", `{"id":"ov","title":"Again"}`); code != http.StatusConflict {
		t.Fatalf("taken id: %d", code)
	}
	if code := post("/api/projects", `{"id":"Bad-Id","title":"x"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("id outside the pattern: %d", code)
	}
	if code := post("/api/model/ov/views", `{"id":"v_all","axis":"axis_x","name":"Всё","language":"ru"}`); code != 200 {
		t.Fatalf("create view: %d", code)
	}
	if code := post("/api/model/ov/views", `{"id":"v_all","axis":"axis_x"}`); code != http.StatusConflict {
		t.Fatalf("taken view id: %d", code)
	}
	// its name is unsaved: a second view waits for Save
	if code := post("/api/model/ov/views", `{"id":"v_two","axis":"axis_x"}`); code != http.StatusConflict {
		t.Fatalf("view on a dirty project: %d", code)
	}
	if code := post("/api/model/nope/views", `{"id":"v_two","axis":"axis_x"}`); code != http.StatusNotFound {
		t.Fatalf("view in a missing project: %d", code)
	}

	mcpSide := &mcpServer{workspace: s.workspace, models: s}
	_, out, err := mcpSide.listProjects(nil, nil, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	var ov *core.ProjectIndex
	for i, p := range out.(core.WorkspaceIndex).Projects {
		if p.ID == "ov" {
			ov = &out.(core.WorkspaceIndex).Projects[i]
		}
	}
	if ov == nil || len(ov.Views) != 1 || ov.Views[0].Names["ru"] != "Всё" {
		t.Fatalf("MCP catalog = %+v", ov)
	}
	if _, _, err := mcpSide.createView(nil, nil, createViewIn{Project: "ov", ID: "v_two", Axis: "axis_x", RequestedByHuman: true}); err == nil || !strings.Contains(err.Error(), "сначала сохраните") {
		t.Fatalf("MCP view on a dirty project: %v", err)
	}
	res := modelRequest(t, c, srv.URL+"/api/model/ov/save", s.key, http.MethodPost, "")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("save: %d", res.StatusCode)
	}
	if _, _, err := mcpSide.createView(nil, nil, createViewIn{Project: "ov", ID: "v_two", Axis: "axis_x", RequestedByHuman: true}); err != nil {
		t.Fatalf("MCP view on a clean project: %v", err)
	}

	// deleting: the project is clean now, so v_all goes; an unknown view is 404
	if code := post("/api/model/ov/views/v_nope", ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("POST on a view: %d", code)
	}
	del := func(path string) int {
		res := modelRequest(t, c, srv.URL+path, s.key, http.MethodDelete, "")
		res.Body.Close()
		return res.StatusCode
	}
	if code := del("/api/model/ov/views/v_nope"); code != http.StatusNotFound {
		t.Fatalf("delete unknown view: %d", code)
	}
	if code := del("/api/model/nope/views/v_all"); code != http.StatusNotFound {
		t.Fatalf("delete in a missing project: %d", code)
	}
	if code := del("/api/model/ov/views/v_all"); code != http.StatusNoContent {
		t.Fatalf("delete view: %d", code)
	}
	if _, err := os.Stat(filepath.Join(s.workspace, "projects", "ov", "views", "v_all.view.json")); !os.IsNotExist(err) {
		t.Fatalf("view file stays: %v", err)
	}
	// an unsaved change in the project: the delete waits for Save (409)
	if _, _, err := mcpSide.createView(nil, nil, createViewIn{Project: "ov", ID: "v_three", Axis: "axis_x", Name: "Три", RequestedByHuman: true}); err != nil {
		t.Fatal(err)
	}
	if code := del("/api/model/ov/views/v_two"); code != http.StatusConflict {
		t.Fatalf("delete on a dirty project: %d", code)
	}
}

func hostModelFixture(t *testing.T) (*modelService, *httptest.Server) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for file, body := range map[string]string{
		"project.json":         `{"id":"p","contractVersion":5,"languages":["ru"]}`,
		"entities.json":        `{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`,
		"relations.json":       `{"relations":[]}`,
		"views/main.view.json": `{"id":"v_main","project":"p","axis":"axis_test","placements":[{"entity":"e_a","parent":null,"x":1,"y":2}]}`,
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

// ADR_20260930-8 over HTTP, with the batches the editor sends: a drawn line is
// created, undone (withdrawn together with the view's mention of it) and the
// project saved — the relations file is what it was, byte for byte. A saved
// record is still refused.
func TestModelAPIWithdrawUnsavedCreation(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	c := srv.Client()
	post := func(path, body string) (int, string) {
		t.Helper()
		res := modelRequest(t, c, srv.URL+"/api/model/p/"+path, s.key, "POST", body)
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, b := post("ops", `{"client":"one","ops":[{"kind":"entity","id":"e_b","value":{"id":"e_b","name":"B","kind":"class","origin":"code"}}]}`); code != 200 {
		t.Fatalf("entity: %d %s", code, b)
	}
	if code, b := post("save", ""); code != 200 {
		t.Fatalf("save: %d %s", code, b)
	}
	file := filepath.Join(s.workspace, "projects", "p", "relations.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	draw := `{"client":"one","ops":[
{"kind":"relation","id":"r_a_b_link","value":{"id":"r_a_b_link","from":"e_a","to":"e_b","type":"link","origin":"authored"}},
{"kind":"view","id":"v_main","view":"v_main","value":{"id":"v_main","edges":[{"id":"r_a_b_link","routing":"orthogonal"}]}}]}`
	if code, b := post("ops", draw); code != 200 {
		t.Fatalf("draw: %d %s", code, b)
	}
	// withdrawing without taking the view's entry away is refused as a whole
	if code, _ := post("ops", `{"client":"one","ops":[{"kind":"relation","id":"r_a_b_link","value":null}]}`); code != 422 {
		t.Fatalf("dangling edges entry: %d", code)
	}
	undo := `{"client":"one","ops":[
{"kind":"relation","id":"r_a_b_link","value":null},
{"kind":"view","id":"v_main","view":"v_main","value":{"id":"v_main","edges":null}}]}`
	if code, b := post("ops", undo); code != 200 {
		t.Fatalf("undo: %d %s", code, b)
	}
	res := modelRequest(t, c, srv.URL+"/api/model/p/save", "", "GET", "")
	var dirty struct{ Registry []any }
	json.NewDecoder(res.Body).Decode(&dirty)
	res.Body.Close()
	if len(dirty.Registry) != 0 {
		t.Fatalf("withdrawn record still unsaved: %v", dirty.Registry)
	}
	// redo creates the same id again; undo again; then save
	if code, b := post("ops", draw); code != 200 {
		t.Fatalf("redo: %d %s", code, b)
	}
	if code, b := post("ops", undo); code != 200 {
		t.Fatalf("undo 2: %d %s", code, b)
	}
	if code, b := post("save", ""); code != 200 {
		t.Fatalf("save: %d %s", code, b)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatalf("relations.json changed:\n%s", after)
	}
	// once saved, the same authored line is removed as an unsaved change that
	// says it is a removal (ADR_20261001); a code record is refused
	if code, b := post("ops", draw); code != 200 {
		t.Fatalf("draw 3: %d %s", code, b)
	}
	if code, b := post("save", ""); code != 200 {
		t.Fatalf("save 3: %d %s", code, b)
	}
	if code, b := post("ops", undo); code != 200 || !strings.Contains(b, `"removed":true`) {
		t.Fatalf("saved authored record not removed: %d %s", code, b)
	}
	if code, b := post("ops", `{"client":"one","ops":[{"kind":"entity","id":"e_b","value":null}]}`); code != 422 || !strings.Contains(b, "comes from code") {
		t.Fatalf("code record removed: %d %s", code, b)
	}
}

// GET /api/kinds is the dictionary: the default with the workspace kinds.json
// added; without lang every text in all its languages.
func TestModelAPIKinds(t *testing.T) {
	s, srv := hostModelFixture(t)
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(s.workspace, "kinds.json"), []byte(`{"relationGroups":[{"id":"mine","name":{"en":"Mine"},"types":[{"id":"feeds","name":{"en":"feeds"}}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res := modelRequest(t, srv.Client(), srv.URL+"/api/kinds", "", "GET", "")
	defer res.Body.Close()
	var full struct {
		Groups []struct {
			Kinds []struct {
				ID        string
				Container bool
				Name      map[string]string
			}
		}
		RelationGroups []struct{ Types []struct{ ID string } }
	}
	if err := json.NewDecoder(res.Body).Decode(&full); err != nil || res.StatusCode != 200 {
		t.Fatalf("kinds: %d %v", res.StatusCode, err)
	}
	seen := map[string]bool{}
	for _, g := range full.Groups {
		for _, k := range g.Kinds {
			seen[k.ID] = true
			if k.ID == "class" && (k.Container || k.Name["ru"] == "" || k.Name["en"] == "") {
				t.Errorf("class: %+v", k)
			}
		}
	}
	if !seen["class"] || !seen["namespace"] {
		t.Errorf("default kinds missing: %v", seen)
	}
	types := map[string]bool{}
	for _, g := range full.RelationGroups {
		for _, ty := range g.Types {
			types[ty.ID] = true
		}
	}
	if !types["extends"] || !types["feeds"] {
		t.Errorf("relation types: default and workspace expected, got %v", types)
	}
	res2 := modelRequest(t, srv.Client(), srv.URL+"/api/kinds?lang=en", "", "GET", "")
	defer res2.Body.Close()
	body, _ := io.ReadAll(res2.Body)
	if !strings.Contains(string(body), `"name":"Class"`) {
		t.Errorf("lang=en: %.200s", body)
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
