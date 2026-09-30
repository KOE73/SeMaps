package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
		"project.json":      `{"id":"p","contractVersion":5}`,
		"entities.json":     `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.A"}]},{"id":"e_b","name":"B","kind":"interface","origin":"code","code":[{"lang":"csharp","symbol":"N.B"}]}]}`,
		"relations.json":    `{"relations":[{"id":"r_a_b_implements","from":"e_a","to":"e_b","type":"implements","origin":"code","status":"present"}]}`,
		"views/v.view.json": `{"id":"v_main","axis":"axis_layer","placements":[]}`,
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
		"get_text", "sync_preview", "doctor", "set_text", "add_entity", "add_relation",
		"set_relation_visible", "confirm_rename", "extract", "sync", "place_entities", "get_view", "move_elements", "resize_elements", "set_parent", "set_placement", "set_routing", "add_container", "fit_container", "align_elements",
		"get_kinds", "layout_guide", "render_view", "create_view", "create_project"} {
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
	if strings.Contains(string(data), "N.A2") {
		t.Fatal("agent edit bypassed Save")
	}
	call(t, cs, "save", map[string]any{"requestedByHuman": true})
	data, _ = os.ReadFile(filepath.Join(ws, "projects", "p", "entities.json"))
	if !strings.Contains(string(data), "N.A2") {
		t.Fatal("rename not saved")
	}
}

func TestMCPRefusalsAreToolErrors(t *testing.T) {
	cs, ws := mcpSession(t)
	for name, args := range map[string]map[string]any{
		"set_text":       {"lang": "ru", "key": "r_a_b_implements", "field": "name", "value": "x"},
		"place_entities": {"view": "v_main", "entities": []any{map[string]any{"entity": "e_a", "x": 1, "y": 1}}},
		"add_relation":   {"from": "e_a", "to": "e_b", "type": "no pe"},
	} {
		res, text := call(t, cs, name, args)
		if !res.IsError {
			t.Errorf("%s: not refused: %s", name, text)
		}
	}
	var v map[string]json.RawMessage
	data, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "views", "v.view.json"))
	json.Unmarshal(data, &v)
	if string(v["placements"]) != "[]" {
		t.Fatalf("view written: %s", v["placements"])
	}
	for _, gone := range []string{"set_zone", "add_zone", "fit_zone"} {
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: gone, Arguments: map[string]any{}}); err == nil {
			t.Errorf("%s is gone with the zones (ADR_20260927-6): it must not be a tool", gone)
		}
	}
	res, _ := call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 1, "y": 1}}})
	if res.IsError {
		t.Fatal("placement asked by a human refused")
	}
}

// Clients (Claude Code) reject a tools/call whose structuredContent is not an
// object; every reading tool must answer with one.
func TestMCPStructuredContentIsAnObject(t *testing.T) {
	cs, _ := mcpSession(t)
	for name, args := range map[string]map[string]any{
		"list_projects": {},
		"list_views":    {},
		"get_kinds":     {},
		"find_entities": {},
		"get_relations": {},
		"get_entity":    {"id": "e_a"},
		"get_text":      {"lang": "ru", "key": "e_a"},
		"doctor":        {},
	} {
		res, text := call(t, cs, name, args)
		if res.IsError {
			t.Errorf("%s: %s", name, text)
			continue
		}
		raw, _ := json.Marshal(res.StructuredContent)
		if len(raw) == 0 || raw[0] != '{' {
			t.Errorf("%s: structuredContent is not an object: %.80s", name, raw)
		}
	}
}

func TestMCPCallLog(t *testing.T) {
	cs, ws := mcpSession(t)
	root := t.TempDir()
	// A second session on the same workspace, with the log on.
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	srv := s.server()
	var echo strings.Builder
	srv.AddReceivingMiddleware(callLog(root, &echo))
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	logged, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer logged.Close()
	_ = cs
	call(t, logged, "get_entity", map[string]any{"id": "e_a"})
	call(t, logged, "get_entity", map[string]any{"id": "e_none"})

	files, _ := filepath.Glob(filepath.Join(root, ".semaps", "logs", "mcp-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("log files: %v", files)
	}
	data, _ := os.ReadFile(files[0])
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"tool":"get_entity"`) || !strings.Contains(lines[1], `"isError":true`) {
		t.Fatalf("log:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(root, ".semaps", ".gitignore")); err != nil {
		t.Fatal("no .gitignore in .semaps")
	}
	if !strings.Contains(echo.String(), "get_entity") || !strings.Contains(echo.String(), "ERROR") {
		t.Fatalf("stderr: %q", echo.String())
	}
}

func TestMCPGetView(t *testing.T) {
	cs, _ := mcpSession(t)
	res, text := call(t, cs, "get_view", map[string]any{"view": "v_main"})
	if res.IsError || !strings.Contains(text, `"placements":[]`) || !strings.Contains(text, `"edges":[]`) {
		t.Fatalf("get_view: %s", text)
	}
	res, text = call(t, cs, "get_view", map[string]any{"view": "v_main#e_nope"})
	if !res.IsError || !strings.Contains(text, "e_nope is not on view v_main") {
		t.Fatalf("unknown object: %v %s", res.IsError, text)
	}
}

func TestMCPGeometryTools(t *testing.T) {
	cs, _ := mcpSession(t)
	res, text := call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "e_core", "name": "Core", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": false})
	if !res.IsError || !strings.Contains(text, "direct request") {
		t.Fatalf("without requestedByHuman: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "e_core", "name": "Core", "kind": "class", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "not a container kind") {
		t.Fatalf("a kind that is no container: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "add_container", map[string]any{"view": "v_main", "name": "Core", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, `missing properties: ["entity"]`) {
		t.Fatalf("no id in the call: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "", "name": "Core", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "entity is required") {
		t.Fatalf("an empty id: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "e_core", "name": "Core", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "not saved") || !strings.Contains(text, "container e_core placed") || !strings.Contains(text, "highlight=e_core") {
		t.Fatalf("add_container: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "move_elements", map[string]any{"elements": []string{"v_main#e_core"}, "dx": 40, "dy": 0, "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "1 changed") {
		t.Fatalf("move_elements by reference: %v %s", res.IsError, text)
	}
	_, text = call(t, cs, "get_view", map[string]any{"view": "v_main#e_core"})
	if !strings.Contains(text, `"x":40`) || !strings.Contains(text, `"container":true`) {
		t.Fatalf("get_view after move: %s", text)
	}
}

// A block goes into a container through `parent`; get_view shows the placements
// as a tree and fit_container closes the frame around them.
func TestMCPContainerTree(t *testing.T) {
	cs, _ := mcpSession(t)
	call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "e_core", "name": "Core", "x": 0, "y": 0, "width": 160, "height": 100, "requestedByHuman": true})
	res, text := call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "parent": "e_core", "x": 400, "y": 300}}})
	if res.IsError {
		t.Fatalf("place_entities into a container: %s", text)
	}
	res, text = call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_b", "parent": "e_a", "x": 0, "y": 0}}})
	if !res.IsError || !strings.Contains(text, "not a container") {
		t.Fatalf("a block as a parent: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "fit_container", map[string]any{"container": "v_main#e_core", "requestedByHuman": true})
	if res.IsError {
		t.Fatalf("fit_container: %s", text)
	}
	_, text = call(t, cs, "get_view", map[string]any{"view": "v_main"})
	var v struct {
		Placements []struct {
			Entity   string
			Children []struct{ Entity string }
			X, Y     float64
		}
	}
	if err := json.Unmarshal([]byte(text[strings.Index(text, "{"):]), &v); err != nil {
		// the answer starts with the canvas block, then the JSON
		t.Fatalf("get_view: %v\n%s", err, text)
	}
	if len(v.Placements) != 1 || v.Placements[0].Entity != "e_core" || len(v.Placements[0].Children) != 1 || v.Placements[0].Children[0].Entity != "e_a" {
		t.Fatalf("placements are not a tree: %s", text)
	}
	if v.Placements[0].X >= 400 {
		t.Fatalf("the container was not fitted to its content: %s", text)
	}
	res, text = call(t, cs, "set_parent", map[string]any{"elements": []string{"e_a"}, "view": "v_main", "parent": nil, "requestedByHuman": true})
	if res.IsError {
		t.Fatalf("set_parent null: %s", text)
	}
	_, text = call(t, cs, "get_view", map[string]any{"view": "v_main#e_core"})
	if !strings.Contains(text, `"children":[]`) {
		t.Fatalf("e_a still in the container: %s", text)
	}
}

func TestMCPGetKinds(t *testing.T) {
	cs, ws := mcpSession(t)
	res, text := call(t, cs, "get_kinds", map[string]any{"lang": "en"})
	if res.IsError {
		t.Fatalf("get_kinds: %s", text)
	}
	var out struct {
		Groups []struct {
			Kinds []struct {
				ID        string
				Name      string
				Container bool
			}
		}
		RelationGroups []struct {
			Types []struct{ ID, Name, Visibility string }
		} `json:"relationGroups"`
		Unknown struct {
			Kinds         []string
			RelationTypes []string `json:"relationTypes"`
		}
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("get_kinds: %v\n%s", err, text)
	}
	kinds := map[string]bool{}
	for _, g := range out.Groups {
		for _, k := range g.Kinds {
			kinds[k.ID] = k.Container
		}
	}
	if container, ok := kinds["namespace"]; !ok || !container {
		t.Errorf("namespace is a container kind of the dictionary: %v", kinds["namespace"])
	}
	if container, ok := kinds["class"]; !ok || container {
		t.Errorf("class is a kind but no container: %v", kinds["class"])
	}
	types, visibility := map[string]bool{}, map[string]string{}
	for _, g := range out.RelationGroups {
		for _, ty := range g.Types {
			types[ty.ID], visibility[ty.ID] = true, ty.Visibility
		}
	}
	if !types["implements"] || !types["call"] {
		t.Errorf("the relation types of the dictionary are missing: %v", types)
	}
	// the default visibility of a type is the dictionary's; an authored type says none: the view decides
	if visibility["uses"] != "hidden" || visibility["implements"] != "visible" || visibility["call"] != "" {
		t.Errorf("visibility of relation types: %v", visibility)
	}
	if len(out.Unknown.Kinds) != 0 || len(out.Unknown.RelationTypes) != 0 {
		t.Errorf("nothing of the fixture is outside the dictionary: %+v", out.Unknown)
	}

	// the workspace kinds.json adds to it, and an entity of a kind outside it is listed, not refused
	os.WriteFile(filepath.Join(ws, "kinds.json"), []byte(`{"groups":[{"id":"mine","name":{"en":"Mine"},"kinds":[{"id":"cell","name":{"en":"Cell"},"container":true}]}]}`), 0o644)
	call(t, cs, "add_entity", map[string]any{"name": "Odd", "kind": "gadget"})
	_, text = call(t, cs, "get_kinds", map[string]any{"lang": "en"})
	if !strings.Contains(text, `"cell"`) || !strings.Contains(text, `"gadget"`) {
		t.Fatalf("workspace kind or unknown kind missing: %s", text)
	}

	// the set of relation types is open: a relation of a type the dictionary lacks is listed, not refused
	call(t, cs, "add_relation", map[string]any{"from": "e_a", "to": "e_b", "type": "homemade"})
	call(t, cs, "add_relation", map[string]any{"from": "e_b", "to": "e_a", "type": "call"})
	_, text = call(t, cs, "get_kinds", map[string]any{"lang": "en"})
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("get_kinds: %v\n%s", err, text)
	}
	if !reflect.DeepEqual(out.Unknown.RelationTypes, []string{"homemade"}) {
		t.Fatalf("unknown relation types: %v", out.Unknown.RelationTypes)
	}
}
