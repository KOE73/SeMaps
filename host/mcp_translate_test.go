package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"semaps/core"
)

// mcpSessionFiles connects a client to a server over a workspace with one
// project p made of the given files.
func mcpSessionFiles(t *testing.T, files map[string]string) (*mcp.ClientSession, string) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, ws
}

func authoredText(v string) string {
	return `{"v":"` + v + `","at":"2026-09-01T10:00:00Z","origin":"authored"}`
}

// findingsOf are the messages of one kind `semaps check` reports.
func findingsOf(ws, kind string) []string {
	var out []string
	for _, f := range core.Check(ws, ws, defaultKinds()) {
		if f.Kind == kind {
			out = append(out, f.Message)
		}
	}
	return out
}

func translateFiles() map[string]string {
	return map[string]string{
		"project.json": `{"id":"p","contractVersion":5,"defaultAxis":"axis_a","languages":["ru","en"]}`,
		"entities.json": `{"entities":[` +
			`{"id":"e_a","kind":"app","origin":"authored","status":"present"},` +
			`{"id":"e_b","kind":"app","origin":"authored","status":"present"},` +
			`{"id":"e_c","kind":"app","origin":"authored","status":"present"}]}`,
		"relations.json": `{"relations":[]}`,
		"text.ru.json": `{"contractVersion":5,"language":"ru","entries":{` +
			`"e_a":{"name":` + authoredText("А") + `,"description":` + authoredText("Описание") + `},` +
			`"e_b":{"name":` + authoredText("Б") + `},` +
			`"e_c":{"name":` + authoredText("Ц") + `}}}`,
		"text.en.json": `{"contractVersion":5,"language":"en","entries":{` +
			`"e_a":{"name":` + authoredText("A") + `,"description":` + authoredText("Description") + `},` +
			`"e_b":{"name":` + authoredText("B") + `},` +
			`"e_c":{"name":` + authoredText("C") + `,"doc":` + authoredText("English only") + `}}}`,
	}
}

func TestSetTextFromWritesATranslationCheckAccepts(t *testing.T) {
	cs, ws := mcpSessionFiles(t, translateFiles())
	if got := findingsOf(ws, "расхождение"); len(got) != 4 {
		t.Fatalf("before: want 4 divergences (e_a name, description, e_b name, e_c name): %v", got)
	}
	for name, args := range map[string]map[string]any{
		"from is lang":    {"lang": "en", "key": "e_a", "field": "description", "value": "x", "from": "en"},
		"no source field": {"lang": "en", "key": "e_a", "field": "doc", "value": "x", "from": "ru"},
		"no source key":   {"lang": "en", "key": "e_zz", "field": "name", "value": "x", "from": "ru"},
	} {
		if res, text := call(t, cs, "set_text", args); !res.IsError {
			t.Errorf("%s: not refused: %s", name, text)
		}
	}
	res, text := call(t, cs, "set_text", map[string]any{"lang": "en", "key": "e_a", "field": "description", "value": "A description", "from": "ru"})
	if res.IsError {
		t.Fatal(text)
	}
	_, text = call(t, cs, "get_text", map[string]any{"lang": "en", "key": "e_a"})
	for _, want := range []string{`"origin":"translated"`, `"from":"ru"`, `"fromHash":"` + core.Hash("Описание") + `"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("get_text lacks %s: %s", want, text)
		}
	}
	call(t, cs, "save", map[string]any{"requestedByHuman": true})
	for _, m := range findingsOf(ws, "расхождение") {
		if strings.Contains(m, "e_a.description") {
			t.Errorf("still reported: %s", m)
		}
	}
	if got := findingsOf(ws, "протух"); len(got) != 0 {
		t.Errorf("stale: %v", got)
	}
}

func TestMarkTranslated(t *testing.T) {
	cs, ws := mcpSessionFiles(t, translateFiles())
	if res, text := call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "ru"}); !res.IsError {
		t.Fatalf("without requestedByHuman: %s", text)
	}
	if res, text := call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "en", "requestedByHuman": true}); !res.IsError {
		t.Fatalf("same language: %s", text)
	}
	if res, text := call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "ru", "keys": []string{"e_zz"}, "requestedByHuman": true}); !res.IsError {
		t.Fatalf("a key with nothing to mark: %s", text)
	}
	res, text := call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "ru", "keys": []string{"e_b"}, "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "e_b.name") || strings.Contains(text, "e_a") {
		t.Fatalf("keys: %v %s", res.IsError, text)
	}
	// default keys: all that is left, authored in en and present in ru
	res, text = call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "ru", "requestedByHuman": true})
	if res.IsError {
		t.Fatal(text)
	}
	for _, want := range []string{"e_a.name", "e_a.description", "e_c.name", "/app/"} {
		if !strings.Contains(text, want) {
			t.Errorf("answer lacks %s: %s", want, text)
		}
	}
	if strings.Contains(text, "e_c.doc") || strings.Contains(text, "e_b.name") {
		t.Errorf("answer lists what it must not: %s", text)
	}
	_, text = call(t, cs, "get_text", map[string]any{"lang": "en", "key": "e_c"})
	if !strings.Contains(text, `"v":"English only","at":"2026-09-01T10:00:00Z","origin":"authored"`) {
		t.Errorf("a text without a source was touched: %s", text)
	}
	_, text = call(t, cs, "get_text", map[string]any{"lang": "en", "key": "e_a"})
	if !strings.Contains(text, `"v":"Description"`) || !strings.Contains(text, `"origin":"translated"`) {
		t.Errorf("value changed or not marked: %s", text)
	}
	if res, text = call(t, cs, "mark_translated", map[string]any{"lang": "en", "from": "ru", "requestedByHuman": true}); !res.IsError {
		t.Errorf("nothing left to mark, not refused: %s", text)
	}
	call(t, cs, "save", map[string]any{"requestedByHuman": true})
	if got := findingsOf(ws, "расхождение"); len(got) != 0 {
		t.Errorf("divergences left: %v", got)
	}
	if got := findingsOf(ws, "протух"); len(got) != 0 {
		t.Errorf("stale: %v", got)
	}
}

func TestSetViewAxisResolvesAContradiction(t *testing.T) {
	files := translateFiles()
	files["entities.json"] = `{"entities":[` +
		`{"id":"e_x","kind":"app","origin":"authored","status":"present"},` +
		`{"id":"e_c1","kind":"group","origin":"authored","status":"present"},` +
		`{"id":"e_c2","kind":"group","origin":"authored","status":"present"}]}`
	files["views/v_one.view.json"] = `{"id":"v_one","axis":"axis_a","placements":[` +
		`{"entity":"e_c1","parent":null,"x":0,"y":0,"width":400,"height":300},` +
		`{"entity":"e_x","parent":"e_c1","x":20,"y":50,"width":180,"height":60}]}`
	files["views/v_two.view.json"] = `{"id":"v_two","axis":"axis_a","placements":[` +
		`{"entity":"e_c2","parent":null,"x":0,"y":0,"width":400,"height":300},` +
		`{"entity":"e_x","parent":"e_c2","x":20,"y":50,"width":180,"height":60}]}`
	cs, ws := mcpSessionFiles(t, files)
	if got := findingsOf(ws, "противоречие"); len(got) != 1 {
		t.Fatalf("before: %v", got)
	}
	if res, text := call(t, cs, "set_view_axis", map[string]any{"view": "v_two", "axis": "axis_b"}); !res.IsError {
		t.Fatalf("without requestedByHuman: %s", text)
	}
	for _, args := range []map[string]any{
		{"view": "v_two", "axis": "  "},
		{"view": "v_nope", "axis": "axis_b"},
	} {
		args["requestedByHuman"] = true
		if res, text := call(t, cs, "set_view_axis", args); !res.IsError {
			t.Fatalf("%v: not refused: %s", args, text)
		}
	}
	res, text := call(t, cs, "set_view_axis", map[string]any{"view": "v_two", "axis": "axis_b", "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "not saved") {
		t.Fatalf("%v %s", res.IsError, text)
	}
	if got := findingsOf(ws, "противоречие"); len(got) != 1 {
		t.Fatalf("the change reached the file before save: %v", got)
	}
	call(t, cs, "save", map[string]any{"requestedByHuman": true})
	if got := findingsOf(ws, "противоречие"); len(got) != 0 {
		t.Errorf("after: %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "views", "v_two.view.json"))
	if !strings.Contains(string(data), `"axis": "axis_b"`) {
		t.Errorf("axis not written: %s", data)
	}
}
