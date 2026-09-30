package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLayoutGuideAndCanvasBlock(t *testing.T) {
	cs, _ := mcpSession(t)
	res, text := call(t, cs, "layout_guide", map[string]any{})
	if res.IsError {
		t.Fatal(text)
	}
	for _, want := range []string{"get_view", "render_view", "move_elements", "resize_elements", "set_parent", "add_container", "fit_container", "align_elements",
		"place_entities", "create_view", "create_project", "save", "set_text", "get_kinds", "requestedByHuman", "does not snap",
		"Look at the sample", "Grid step 10", "180x60", "28 high", "40 between containers", "3 x 180 + 2 x 40 = 620"} {
		if !strings.Contains(text, want) {
			t.Errorf("layout_guide lacks %q; tail:\n%s", want, text[max(0, len(text)-1500):])
		}
	}
	for _, banned := range []string{".view.json", `"zones"`, `"nodes"`, "set_zone", "add_zone", "fit_zone", "CONTRACT", "LAYOUT.md"} {
		if strings.Contains(text, banned) {
			t.Errorf("layout_guide names the file format: %q", banned)
		}
	}
	// get_view opens with the very same canvas block, before the data
	_, view := call(t, cs, "get_view", map[string]any{"view": "v_main"})
	if !strings.HasPrefix(view, "CANVAS.") || strings.Index(view, "Grid step 10") > strings.Index(view, `"placements"`) {
		t.Fatalf("get_view does not start with the canvas:\n%s", view)
	}
	if !strings.Contains(text, strings.SplitN(view, "\n", 2)[0]) {
		t.Fatal("layout_guide and get_view describe the canvas differently")
	}
	// a workspace canvas.json wins over the shipped one, in both
	cs2, ws := mcpSession(t)
	over := strings.Replace(string(mustRead(t, "defaults/canvas.json")), `"grid":10`, `"grid":20`, 1)
	if err := os.WriteFile(filepath.Join(ws, "canvas.json"), []byte(over), 0o644); err != nil {
		t.Fatal(err)
	}
	_, view = call(t, cs2, "get_view", map[string]any{"view": "v_main"})
	_, text = call(t, cs2, "layout_guide", map[string]any{})
	if !strings.Contains(view, "Grid step 20") || !strings.Contains(text, "Grid step 20") {
		t.Fatalf("override ignored:\n%s", view)
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateViewAndProject(t *testing.T) {
	cs, ws := mcpSession(t)
	res, text := call(t, cs, "create_view", map[string]any{"project": "p", "id": "v_new", "axis": "axis_layer", "name": "Новый"})
	if !res.IsError || !strings.Contains(text, "requestedByHuman") {
		t.Fatalf("without a human: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "create_project", map[string]any{"id": "other"})
	if !res.IsError || !strings.Contains(text, "requestedByHuman") {
		t.Fatalf("project without a human: %v %s", res.IsError, text)
	}
	if _, err := os.Stat(filepath.Join(ws, "projects", "other")); err == nil {
		t.Fatal("project written without a human")
	}
	res, text = call(t, cs, "create_project", map[string]any{"id": "other", "title": "Other", "language": "en", "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "other") {
		t.Fatalf("create_project: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "create_project", map[string]any{"id": "other", "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "already exists") {
		t.Fatalf("second create_project: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "create_view", map[string]any{"project": "other", "id": "v_other", "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "axis") {
		t.Fatalf("no axis: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "create_view", map[string]any{"project": "other", "id": "v_new", "axis": "axis_layer", "name": "New", "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "name is not saved") || !strings.Contains(text, "/app/#v_new") {
		t.Fatalf("create_view: %v %s", res.IsError, text)
	}
	// the view is a file: written at once; its name is unsaved
	data, err := os.ReadFile(filepath.Join(ws, "projects", "other", "views", "v_new.view.json"))
	if err != nil || !strings.Contains(string(data), `"axis_layer"`) {
		t.Fatalf("view file: %s %v", data, err)
	}
	_, text = call(t, cs, "list_projects", map[string]any{})
	if !strings.Contains(text, `"v_new"`) || !strings.Contains(text, `"en":"New"`) {
		t.Fatalf("list_projects without the unsaved name: %s", text)
	}
	res, text = call(t, cs, "create_view", map[string]any{"project": "other", "id": "v_two", "axis": "axis_layer", "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "сначала сохраните") {
		t.Fatalf("view on a project with unsaved changes: %v %s", res.IsError, text)
	}
	if res, text = call(t, cs, "save", map[string]any{"project": "other", "requestedByHuman": true}); res.IsError {
		t.Fatal(text)
	}
	_, text = call(t, cs, "get_text", map[string]any{"project": "other", "lang": "en", "key": "v_new"})
	if !strings.Contains(text, "New") {
		t.Fatalf("view name: %s", text)
	}
}

func TestAddEntityThroughMCP(t *testing.T) {
	cs, ws := mcpSession(t)
	res, text := call(t, cs, "add_entity", map[string]any{"project": "p", "name": "Billing DB", "kind": "database", "description": "Holds invoices"})
	if res.IsError || !strings.Contains(text, "e_billing_db") {
		t.Fatalf("add_entity: %v %s", res.IsError, text)
	}
	_, text = call(t, cs, "get_entity", map[string]any{"project": "p", "id": "e_billing_db"})
	if !strings.Contains(text, `"origin":"authored"`) || !strings.Contains(text, `"status":"present"`) {
		t.Fatalf("entity: %s", text)
	}
	_, text = call(t, cs, "get_text", map[string]any{"project": "p", "lang": "ru", "key": "e_billing_db"})
	if !strings.Contains(text, "Holds invoices") {
		t.Fatalf("description: %s", text)
	}
	res, text = call(t, cs, "add_entity", map[string]any{"project": "p", "id": "e_billing_db", "name": "Again", "kind": "database"})
	if !res.IsError || !strings.Contains(text, "exists") {
		t.Fatalf("existing id: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "add_entity", map[string]any{"project": "p", "id": "node_1", "name": "N", "kind": "app"})
	if !res.IsError || !strings.Contains(text, "starts with e_") {
		t.Fatalf("id without e_: %v %s", res.IsError, text)
	}
	// unsaved: nothing on disk yet
	b, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "entities.json"))
	if strings.Contains(string(b), "e_billing_db") {
		t.Fatal("entity written before Save")
	}
}

// ---------------------------------------------------------------- render_view

type renderFixture struct {
	cs  *mcp.ClientSession
	ms  *modelService
	srv *httptest.Server
}

func newRenderFixture(t *testing.T) *renderFixture {
	t.Helper()
	_, ws := mcpSession(t)
	ms, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	ms.register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := &mcpServer{workspace: ws, sourceRoot: ws, models: ms}
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &renderFixture{cs, ms, srv}
}

// subscribe is a fake editor: it opens the event stream and hands over the
// modelEvents that carry a render request.
func (f *renderFixture) subscribe(t *testing.T, query string) <-chan renderRequest {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", f.srv.URL+"/api/events?project=p"+query, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	out := make(chan renderRequest, 4)
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	ready := make(chan struct{})
	go func() {
		for sc.Scan() {
			line := sc.Text()
			if line == ": connected" {
				close(ready)
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var ev modelEvent
				if json.Unmarshal([]byte(data), &ev) == nil && ev.Render != nil {
					out <- *ev.Render
				}
			}
		}
	}()
	<-ready
	return out
}

func (f *renderFixture) answer(t *testing.T, id string, body any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/render/"+id, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+f.ms.key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("answer: %s", res.Status)
	}
}

func TestRenderViewOK(t *testing.T) {
	f := newRenderFixture(t)
	plain := f.subscribe(t, "") // a graph-mode subscriber: gets no render request
	editor := f.subscribe(t, "&render=1")
	png := []byte("\x89PNG-fake")
	go func() {
		req := <-editor
		if req.View != "v_main" || req.Scale != 2 || req.MaxSize != 4096 || req.Rect == nil || req.Rect.Width != 300 || req.ID == "" {
			t.Errorf("request = %+v", req)
		}
		f.answer(t, req.ID, map[string]any{"png": base64.StdEncoding.EncodeToString(png), "width": 600, "height": 400,
			"problems": []map[string]any{{"kind": "overlap", "ids": []string{"e_a", "e_b"}, "text": "A covers B"}}})
	}()
	res, err := f.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "render_view",
		Arguments: map[string]any{"project": "p", "view": "v_main", "scale": 2, "maxSize": 100000, "rect": map[string]any{"x": 0, "y": 0, "width": 300, "height": 200}}})
	if err != nil || res.IsError {
		t.Fatalf("render_view: %v %+v", err, res)
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" || !bytes.Equal(img.Data, png) {
		t.Fatalf("content[0] = %#v", res.Content[0])
	}
	text := res.Content[1].(*mcp.TextContent).Text
	for _, want := range []string{"overlap: A covers B (e_a, e_b)", "width=300", "600x400", "unsaved"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	select {
	case r := <-plain:
		t.Fatalf("a client without render=1 got a request: %+v", r)
	default:
	}
}

func TestRenderViewErrorsThenOK(t *testing.T) {
	f := newRenderFixture(t)
	bad := f.subscribe(t, "&render=1")
	good := f.subscribe(t, "&render=1")
	go func() {
		f.answer(t, (<-bad).ID, map[string]any{"error": "canvas is not ready"})
	}()
	go func() {
		req := <-good
		time.Sleep(50 * time.Millisecond)
		f.answer(t, req.ID, map[string]any{"png": base64.StdEncoding.EncodeToString([]byte("x")), "width": 1, "height": 1})
	}()
	res, err := f.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "render_view", Arguments: map[string]any{"project": "p", "ref": "v_main"}})
	if err != nil || res.IsError {
		t.Fatalf("an error from one editor must not fail the call: %v %+v", err, res)
	}
}

func TestRenderViewAllEditorsFail(t *testing.T) {
	f := newRenderFixture(t)
	one := f.subscribe(t, "&render=1")
	go func() { f.answer(t, (<-one).ID, map[string]any{"error": "no such view in this editor"}) }()
	res, text := call(t, f.cs, "render_view", map[string]any{"project": "p", "view": "v_main"})
	if !res.IsError || !strings.Contains(text, "no such view in this editor") {
		t.Fatalf("%v %s", res.IsError, text)
	}
}

func TestRenderViewNoEditor(t *testing.T) {
	f := newRenderFixture(t)
	f.subscribe(t, "") // graph mode only
	res, text := call(t, f.cs, "render_view", map[string]any{"project": "p", "view": "v_main"})
	if !res.IsError || !strings.Contains(text, "no editor is open on this project: open the view in the editor (/app/#v_main) and repeat") {
		t.Fatalf("%v %s", res.IsError, text)
	}
}

func TestRenderViewTimeout(t *testing.T) {
	old := renderTimeout
	renderTimeout = 100 * time.Millisecond
	t.Cleanup(func() { renderTimeout = old })
	f := newRenderFixture(t)
	f.subscribe(t, "&render=1") // never answers
	res, text := call(t, f.cs, "render_view", map[string]any{"project": "p", "view": "v_main"})
	if !res.IsError || !strings.Contains(text, "did not answer") {
		t.Fatalf("%v %s", res.IsError, text)
	}
	// a late answer finds nothing waiting
	f.ms.renderMu.Lock()
	n := len(f.ms.pending)
	f.ms.renderMu.Unlock()
	if n != 0 {
		t.Fatalf("%d requests left pending", n)
	}
}

func TestRenderViewArguments(t *testing.T) {
	f := newRenderFixture(t)
	for name, args := range map[string]map[string]any{
		"scale":     {"view": "v_main", "scale": 9},
		"nothing":   {},
		"no view":   {"view": "v_nope"},
		"bad rect":  {"view": "v_main", "rect": map[string]any{"x": 0, "y": 0, "width": 0, "height": 10}},
		"two views": {"view": "v_other", "ref": "v_main#e_x"},
	} {
		args["project"] = "p"
		if res, text := call(t, f.cs, "render_view", args); !res.IsError {
			t.Errorf("%s: not refused: %s", name, text)
		}
	}
}

func TestRenderAnswerNeedsTheKey(t *testing.T) {
	f := newRenderFixture(t)
	res, err := http.Post(f.srv.URL+"/api/render/none", "application/json", strings.NewReader(`{"error":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}
