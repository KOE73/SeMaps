package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// filesSession: an MCP session over a workspace with project p made of these files.
func filesSession(t *testing.T, files map[string]string) *mcp.ClientSession {
	t.Helper()
	ws := t.TempDir()
	for name, body := range files {
		p := filepath.Join(ws, "projects", "p", filepath.FromSlash(name))
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
	return cs
}

// bigView: two containers of 35 blocks each and four loose blocks, so 76
// placements; an implements line (visible) and a uses line (hidden by the type).
func bigView() map[string]string {
	var ents, places []string
	ents = append(ents, `{"id":"e_c1","kind":"namespace","origin":"authored"}`, `{"id":"e_c2","kind":"namespace","origin":"authored"}`)
	places = append(places, `{"entity":"e_c1","parent":null,"x":0,"y":0,"width":900,"height":900}`, `{"entity":"e_c2","parent":null,"x":1000,"y":0,"width":900,"height":900}`)
	for i := 0; i < 37; i++ {
		for _, side := range []string{"a", "b"} {
			id := fmt.Sprintf("e_%s%d", side, i)
			ents = append(ents, fmt.Sprintf(`{"id":%q,"name":%q,"kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.%s%d"}]}`, id, id, side, i))
			parent := `"e_c1"`
			if side == "b" {
				parent = `"e_c2"`
			}
			if i >= 35 {
				parent = "null"
			}
			places = append(places, fmt.Sprintf(`{"entity":%q,"parent":%s,"x":%d,"y":%d}`, id, parent, 20+(i%5)*100, 60+(i/5)*70))
		}
	}
	return map[string]string{
		"project.json":  `{"id":"p","contractVersion":5}`,
		"entities.json": `{"entities":[` + strings.Join(ents, ",") + `]}`,
		"text.ru.json":  `{"contractVersion":5,"language":"ru","entries":{"e_c1":{"name":{"v":"One","at":"2026-09-30T00:00:00Z","origin":"authored"}},"e_c2":{"name":{"v":"Two","at":"2026-09-30T00:00:00Z","origin":"authored"}}}}`,
		"relations.json": `{"relations":[` +
			`{"id":"r_a0_a1_implements","from":"e_a0","to":"e_a1","type":"implements","origin":"code","status":"present"},` +
			`{"id":"r_a1_b1_uses","from":"e_a1","to":"e_b1","type":"uses","origin":"code","status":"present"}]}`,
		"views/big.view.json": `{"id":"v_big","axis":"axis_layer","placements":[` + strings.Join(places, ",") + `]}`,
	}
}

func TestMCPGetViewAnswersOnceAndAsATreeWhenBig(t *testing.T) {
	cs := filesSession(t, bigView())

	// a big view without detail: a tree, and the first line says so
	res, tree := call(t, cs, "get_view", map[string]any{"view": "v_big"})
	if res.IsError || res.StructuredContent != nil {
		t.Fatalf("err %v structured %v: %.200s", res.IsError, res.StructuredContent, tree)
	}
	first := strings.SplitN(tree, "\n", 2)[0]
	for _, want := range []string{"Shown as a tree", "76 placements", "more than 60", "`v_big#e_x`", `detail "full"`} {
		if !strings.Contains(first, want) {
			t.Errorf("the first line lacks %q: %s", want, first)
		}
	}
	if !strings.Contains(tree, "CANVAS.") {
		t.Error("the canvas block is gone")
	}
	for _, want := range []string{`"detail":"tree"`, `"blocks":35`, `"lines":1`, `"entity":"e_a35"`, `"entity":"e_b36"`} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree lacks %s", want)
		}
	}
	for _, banned := range []string{`"edges"`, `"entity":"e_a5"`, `"entity":"e_b20"`, `"hiddenEdges"`} {
		if strings.Contains(tree, banned) {
			t.Errorf("tree has %s", banned)
		}
	}
	if n := strings.Count(tree, `"placements"`); n != 1 {
		t.Errorf("the data is given %d times", n)
	}

	// full asked for: everything, no notice, one copy
	res, full := call(t, cs, "get_view", map[string]any{"view": "v_big", "detail": "full"})
	if res.IsError || res.StructuredContent != nil || !strings.HasPrefix(full, "CANVAS.") {
		t.Fatalf("full: %v %.200s", res.IsError, full)
	}
	for _, want := range []string{`"detail":"full"`, `"entity":"e_a5"`, `"edges":[{"id":"r_a0_a1_implements"`} {
		if !strings.Contains(full, want) {
			t.Errorf("full lacks %s", want)
		}
	}
	if strings.Contains(full, `"blocks"`) || strings.Contains(full, `"lines"`) || strings.Contains(full, "hiddenEdges") {
		t.Error("full carries the tree's or the hidden fields")
	}
	if len(tree)*3 > len(full) {
		t.Errorf("the tree is not small: %d against %d", len(tree), len(full))
	}

	// a container reference: 36 placements, so full by default; a tree when asked
	_, sub := call(t, cs, "get_view", map[string]any{"view": "v_big#e_c1"})
	if !strings.HasPrefix(sub, "CANVAS.") || !strings.Contains(sub, `"detail":"full"`) || !strings.Contains(sub, `"entity":"e_a34"`) || !strings.Contains(sub, `"scope":"v_big#e_c1"`) {
		t.Fatalf("container ref: %.300s", sub)
	}
	_, sub = call(t, cs, "get_view", map[string]any{"view": "v_big#e_c1", "detail": "tree"})
	if !strings.HasPrefix(sub, "CANVAS.") || !strings.Contains(sub, `"blocks":35`) || strings.Contains(sub, `"entity":"e_a34"`) {
		t.Fatalf("container ref as a tree: %.300s", sub)
	}

	// hidden lines
	_, hid := call(t, cs, "get_view", map[string]any{"view": "v_big", "detail": "full", "hidden": true})
	if !strings.Contains(hid, `"hiddenEdges":[{"id":"r_a1_b1_uses"`) || strings.Contains(full, "r_a1_b1_uses") {
		t.Fatalf("hidden: %s", hid[strings.Index(hid, `"edges"`):])
	}
	res, text := call(t, cs, "get_view", map[string]any{"view": "v_big", "detail": "deep"})
	if !res.IsError || !strings.Contains(text, "detail is") {
		t.Fatalf("a bad detail: %v %s", res.IsError, text)
	}
}

func TestMCPGetViewSmallViewIsFullWithNoNotice(t *testing.T) {
	cs, _ := mcpSession(t)
	res, text := call(t, cs, "get_view", map[string]any{"view": "v_main"})
	if res.IsError || res.StructuredContent != nil || !strings.HasPrefix(text, "CANVAS.") || !strings.Contains(text, `"detail":"full"`) {
		t.Fatalf("%v %v %.200s", res.IsError, res.StructuredContent, text)
	}
	if n := strings.Count(text, `"placements"`); n != 1 {
		t.Fatalf("the data is given %d times", n)
	}
}

func contentsProject() map[string]string {
	return map[string]string{
		"project.json": `{"id":"p","contractVersion":5}`,
		"entities.json": `{"entities":[
{"id":"e_ns","kind":"namespace","origin":"authored"},
{"id":"e_sub","kind":"namespace","origin":"authored"},
{"id":"e_gone","kind":"class","origin":"authored","status":"missing"},
{"id":"e_t1","name":"Zed","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.Zed"}]},
{"id":"e_t2","name":"Alpha","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.Alpha"}]},
{"id":"e_t3","name":"Mid","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.Mid"}]}]}`,
		"text.ru.json": `{"contractVersion":5,"language":"ru","entries":{"e_ns":{"name":{"v":"Ns","at":"2026-09-30T00:00:00Z","origin":"authored"}},"e_sub":{"name":{"v":"Sub","at":"2026-09-30T00:00:00Z","origin":"authored"}},"e_gone":{"name":{"v":"Gone","at":"2026-09-30T00:00:00Z","origin":"authored"}}}}`,
		"relations.json": `{"relations":[
{"id":"r_1","from":"e_ns","to":"e_t1","type":"contains","origin":"code","status":"present"},
{"id":"r_2","from":"e_ns","to":"e_t2","type":"contains","origin":"code","status":"present"},
{"id":"r_3","from":"e_ns","to":"e_t3","type":"contains","origin":"code","status":"present"},
{"id":"r_4","from":"e_ns","to":"e_sub","type":"contains","origin":"code","status":"present"},
{"id":"r_5","from":"e_ns","to":"e_gone","type":"contains","origin":"code","status":"present"}]}`,
		"views/v.view.json": `{"id":"v_probe","axis":"axis_layer","placements":[]}`,
	}
}

func TestMCPAddContainerWithContents(t *testing.T) {
	cs := filesSession(t, contentsProject())
	args := func(extra map[string]any) map[string]any {
		a := map[string]any{"view": "v_probe", "entity": "e_ns", "x": 100, "y": 50, "requestedByHuman": true}
		for k, v := range extra {
			a[k] = v
		}
		return a
	}
	// a human's request is needed
	res, text := call(t, cs, "add_container", args(map[string]any{"contents": true, "requestedByHuman": false}))
	if !res.IsError || !strings.Contains(text, "direct request") {
		t.Fatalf("without a human: %v %s", res.IsError, text)
	}
	// width and height are needed unless contents is given
	res, text = call(t, cs, "add_container", args(nil))
	if !res.IsError || !strings.Contains(text, "give width and height") {
		t.Fatalf("no size: %v %s", res.IsError, text)
	}
	// a refusal leaves nothing behind
	res, text = call(t, cs, "add_container", args(map[string]any{"contents": true, "parent": "e_nope"}))
	if !res.IsError {
		t.Fatalf("a missing parent: %s", text)
	}
	_, view := call(t, cs, "get_view", map[string]any{"view": "v_probe"})
	if !strings.Contains(view, `"placements":[]`) {
		t.Fatalf("a refused call placed something: %s", view)
	}

	res, text = call(t, cs, "add_container", args(map[string]any{"contents": true}))
	if res.IsError {
		t.Fatalf("add_container contents: %s", text)
	}
	for _, want := range []string{"container e_ns:", "4 member(s) placed inside it", "e_t2, e_t3, e_sub, e_t1", "only a starting arrangement", "Skipped: e_gone (the entity is missing", "not saved", "highlight="} {
		if !strings.Contains(text, want) {
			t.Errorf("the answer lacks %q: %s", want, text)
		}
	}
	_, view = call(t, cs, "get_view", map[string]any{"view": "v_probe"})
	for _, want := range []string{`"entity":"e_ns"`, `"parent":"e_ns"`, `"entity":"e_sub","name":"Sub","kind":"namespace","container":true`} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %s", want)
		}
	}
	// the same again: nothing to add, and it says so
	res, text = call(t, cs, "add_container", args(map[string]any{"contents": true}))
	if res.IsError || !strings.Contains(text, "0 member(s) placed inside it") || !strings.Contains(text, "e_t1 (already on the view, not moved)") {
		t.Fatalf("second call: %v %s", res.IsError, text)
	}
	// without contents a container already there is refused, as before
	res, text = call(t, cs, "add_container", args(map[string]any{"width": 300, "height": 200}))
	if !res.IsError || !strings.Contains(text, "already on") {
		t.Fatalf("again without contents: %v %s", res.IsError, text)
	}
}

func TestMCPAddContainerAndLayoutGuideSayWhatContentsIs(t *testing.T) {
	cs := filesSession(t, contentsProject())
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		switch tl.Name {
		case "add_container":
			for _, want := range []string{"contents: true", "`contains` relations", "starting arrangement", "whole or not at all"} {
				if !strings.Contains(tl.Description, want) {
					t.Errorf("add_container lacks %q", want)
				}
			}
		case "get_view":
			for _, want := range []string{"detail", "tree", "at most 60 placements", "hidden: true", "hiddenEdges"} {
				if !strings.Contains(tl.Description, want) {
					t.Errorf("get_view lacks %q", want)
				}
			}
		}
	}
	_, guide := call(t, cs, "layout_guide", map[string]any{})
	for _, want := range []string{"`contents: true`", "a fact of the registry", "starting arrangement", "`detail`", "`hiddenEdges`", "`blocks`"} {
		if !strings.Contains(guide, want) {
			t.Errorf("layout_guide lacks %q", want)
		}
	}
	if strings.Contains(guide, "axis") {
		t.Error("layout_guide talks about the axis")
	}
}
