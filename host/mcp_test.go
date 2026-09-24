package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSession connects a client to the server over in-memory transports, on a
// workspace with one project.
func mcpSession(t *testing.T) (*mcp.ClientSession, string) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json":        `{"id":"p"}`,
		"entities.json":       `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","symbol":"N.A"},{"id":"e_b","name":"B","kind":"interface","origin":"code","symbol":"N.B"}]}`,
		"relations.json":      `{"relations":[{"id":"r_a_b_implements","from":"e_a","to":"e_b","type":"implements","origin":"code","status":"present"}]}`,
		"relation-types.json": `{"relationTypes":[{"id":"implements","origin":"code","visibility":"visible"},{"id":"call","origin":"authored"}]}`,
		"views/v.view.json":   `{"id":"v_main","axis":"axis_layer","nodes":[]}`,
	}
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

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return res, b.String()
}

func TestMCPListsAllTools(t *testing.T) {
	cs, _ := mcpSession(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tl := range res.Tools {
		have[tl.Name] = true
	}
	for _, n := range []string{"list_projects", "list_views", "get_entity", "find_entities", "get_relations",
		"get_text", "sync_preview", "doctor", "set_text", "add_relation", "add_relation_type",
		"set_relation_visible", "confirm_rename", "extract", "sync", "place_entities"} {
		if !have[n] {
			t.Errorf("no tool %s", n)
		}
	}
}

func TestMCPReadTools(t *testing.T) {
	cs, _ := mcpSession(t)
	_, text := call(t, cs, "get_entity", map[string]any{"symbol": "N.B"})
	if !strings.Contains(text, `"e_b"`) {
		t.Fatalf("get_entity: %s", text)
	}
	_, text = call(t, cs, "find_entities", map[string]any{"kind": "class"})
	if !strings.Contains(text, `"total":1`) {
		t.Fatalf("find_entities: %s", text)
	}
	_, text = call(t, cs, "get_relations", map[string]any{"entity": "e_b", "direction": "in"})
	if !strings.Contains(text, "r_a_b_implements") {
		t.Fatalf("get_relations: %s", text)
	}
	_, text = call(t, cs, "list_views", map[string]any{})
	if !strings.Contains(text, "v_main") {
		t.Fatalf("list_views: %s", text)
	}
}

func TestMCPWritesGoThroughCore(t *testing.T) {
	cs, ws := mcpSession(t)
	_, text := call(t, cs, "add_relation", map[string]any{"from": "e_b", "to": "e_a", "type": "call"})
	if !strings.Contains(text, "r_b_a_call") {
		t.Fatalf("add_relation: %s", text)
	}
	call(t, cs, "set_text", map[string]any{"lang": "ru", "key": "r_b_a_call", "field": "name", "value": "вызывает"})
	_, text = call(t, cs, "get_text", map[string]any{"lang": "ru", "key": "r_b_a_call"})
	if !strings.Contains(text, "вызывает") || !strings.Contains(text, "authored") {
		t.Fatalf("get_text: %s", text)
	}
	call(t, cs, "confirm_rename", map[string]any{"entity": "e_a", "symbol": "N.A2"})
	data, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "entities.json"))
	if !strings.Contains(string(data), "N.A2") {
		t.Fatal("rename not written")
	}
}

func TestMCPRefusalsAreToolErrors(t *testing.T) {
	cs, ws := mcpSession(t)
	for name, args := range map[string]map[string]any{
		"set_text":       {"lang": "ru", "key": "r_a_b_implements", "field": "name", "value": "x"},
		"place_entities": {"view": "v_main", "entities": []any{map[string]any{"entity": "e_a", "x": 1, "y": 1}}},
		"add_relation":   {"from": "e_a", "to": "e_b", "type": "nope"},
	} {
		res, text := call(t, cs, name, args)
		if !res.IsError {
			t.Errorf("%s: not refused: %s", name, text)
		}
	}
	var v map[string]json.RawMessage
	data, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "views", "v.view.json"))
	json.Unmarshal(data, &v)
	if string(v["nodes"]) != "[]" {
		t.Fatalf("view written: %s", v["nodes"])
	}
	res, _ := call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 1, "y": 1}}})
	if res.IsError {
		t.Fatal("placement asked by a human refused")
	}
}
