package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func lookView(t *testing.T, cs *mcp.ClientSession) (placements map[string]map[string]any, view map[string]any, edges []map[string]any) {
	t.Helper()
	_, text := call(t, cs, "get_view", map[string]any{"view": "v_main"})
	body := text[strings.Index(text, `{"view"`):]
	var v struct {
		View       map[string]any   `json:"view"`
		Placements []map[string]any `json:"placements"`
		Edges      []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("get_view: %v\n%s", err, text)
	}
	placements = map[string]map[string]any{}
	var walk func(list []map[string]any)
	walk = func(list []map[string]any) {
		for _, p := range list {
			placements[p["entity"].(string)] = p
			if ch, ok := p["children"].([]any); ok {
				var sub []map[string]any
				for _, c := range ch {
					sub = append(sub, c.(map[string]any))
				}
				walk(sub)
			}
		}
	}
	walk(v.Placements)
	return placements, v.View, v.Edges
}

func TestMCPSetPlacement(t *testing.T) {
	cs, _ := mcpSession(t)
	call(t, cs, "add_container", map[string]any{"view": "v_main", "entity": "e_core", "name": "Core", "x": 0, "y": 0, "width": 300, "height": 200, "requestedByHuman": true})
	call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 500, "y": 0}}})

	res, text := call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "styleId": "class.detail", "requestedByHuman": false})
	if !res.IsError || !strings.Contains(text, "direct request") {
		t.Fatalf("without requestedByHuman: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "give at least one") {
		t.Fatalf("no field: %v %s", res.IsError, text)
	}

	// each field set; a reference works as elements
	res, text = call(t, cs, "set_placement", map[string]any{"elements": []string{"v_main#e_a"}, "requestedByHuman": true,
		"styleId": "class.detail", "template": "@Asset(brain)", "override": map[string]any{"fill": "#ff0000", "border": map[string]any{"color": "#00ff00"}}})
	if res.IsError || !strings.Contains(text, "not saved") || !strings.Contains(text, "highlight=e_a") {
		t.Fatalf("set_placement: %v %s", res.IsError, text)
	}
	p, _, _ := lookView(t, cs)
	a := p["e_a"]
	ov, _ := json.Marshal(a["override"])
	if a["styleId"] != "class.detail" || a["template"] != "@Asset(brain)" || string(ov) != `{"border":{"color":"#00ff00"},"fill":"#ff0000"}` {
		t.Fatalf("fields not set: %v", a)
	}
	if a["x"] != float64(500) {
		t.Fatalf("geometry touched: %v", a)
	}

	// absent stays, null drops: only the styleId goes
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "requestedByHuman": true, "styleId": nil})
	if res.IsError {
		t.Fatalf("styleId null: %s", text)
	}
	p, _, _ = lookView(t, cs)
	if _, has := p["e_a"]["styleId"]; has || p["e_a"]["template"] != "@Asset(brain)" || p["e_a"]["override"] == nil {
		t.Fatalf("null dropped more or less than styleId: %v", p["e_a"])
	}
	for _, field := range []string{"override", "template"} {
		if res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "requestedByHuman": true, field: nil}); res.IsError {
			t.Fatalf("%s null: %s", field, text)
		}
	}
	p, _, _ = lookView(t, cs)
	for _, field := range []string{"styleId", "override", "template"} {
		if _, has := p["e_a"][field]; has {
			t.Fatalf("%s not dropped: %v", field, p["e_a"])
		}
	}

	// the write rules of the core: a field outside the override table, collapsed on a block
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "requestedByHuman": true, "override": map[string]any{"opacity": 0.5}})
	if !res.IsError || !strings.Contains(text, "cannot be overridden") {
		t.Fatalf("override outside the table: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "requestedByHuman": true, "collapsed": true})
	if !res.IsError || !strings.Contains(text, "not a container") {
		t.Fatalf("collapsed on a block: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a", "e_core"}, "requestedByHuman": true, "collapsed": true})
	if !res.IsError {
		t.Fatalf("one block among the elements must refuse the whole step: %s", text)
	}
	if p, _, _ = lookView(t, cs); p["e_core"]["collapsed"] != nil {
		t.Fatalf("a refused step wrote: %v", p["e_core"])
	}
	// on a container, and null drops it
	if res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_core"}, "requestedByHuman": true, "collapsed": true}); res.IsError {
		t.Fatalf("collapsed on a container: %s", text)
	}
	if p, _, _ = lookView(t, cs); p["e_core"]["collapsed"] != true {
		t.Fatalf("collapsed not set: %v", p["e_core"])
	}
	if res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_core"}, "requestedByHuman": true, "collapsed": nil}); res.IsError {
		t.Fatalf("collapsed null: %s", text)
	}
	if p, _, _ = lookView(t, cs); p["e_core"]["collapsed"] != nil {
		t.Fatalf("collapsed not dropped: %v", p["e_core"])
	}
	// an entity not on the view
	res, text = call(t, cs, "set_placement", map[string]any{"view": "v_main", "elements": []string{"e_b"}, "requestedByHuman": true, "styleId": "x"})
	if !res.IsError || !strings.Contains(text, "e_b is not on view v_main") {
		t.Fatalf("not placed: %v %s", res.IsError, text)
	}
}

func TestMCPSetRouting(t *testing.T) {
	cs, ws := mcpSession(t)
	viewFile := filepath.Join(ws, "projects", "p", "views", "v.view.json")
	// only one end placed: the line is not drawn
	call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 0, "y": 0}}})

	res, text := call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "orthogonal", "requestedByHuman": false})
	if !res.IsError || !strings.Contains(text, "direct request") {
		t.Fatalf("without requestedByHuman: %v %s", res.IsError, text)
	}
	// the view's own routing
	res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "tree-vertical", "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "not saved") || !strings.Contains(text, "tree-vertical") {
		t.Fatalf("view routing: %v %s", res.IsError, text)
	}
	if _, v, _ := lookView(t, cs); v["routing"] != "tree-vertical" {
		t.Fatalf("view.routing: %v", v)
	}
	if res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": nil, "requestedByHuman": true}); res.IsError {
		t.Fatalf("view routing null: %s", text)
	}
	if _, v, _ := lookView(t, cs); v["routing"] != nil {
		t.Fatalf("view.routing not removed: %v", v)
	}
	// a value outside the four, an unknown relation
	res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "curvy", "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "one of bezier, orthogonal, tree-horizontal, tree-vertical") {
		t.Fatalf("bad value: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "bezier", "relations": []string{"r_nope"}, "requestedByHuman": true})
	if !res.IsError || !strings.Contains(text, "no relation r_nope") {
		t.Fatalf("unknown relation: %v %s", res.IsError, text)
	}

	// one relation whose end is not placed: accepted, and said not to be drawn
	res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "orthogonal", "relations": []string{"r_a_b_implements"}, "requestedByHuman": true})
	if res.IsError || !strings.Contains(text, "not drawn on this view") || !strings.Contains(text, "r_a_b_implements") {
		t.Fatalf("relation not drawn: %v %s", res.IsError, text)
	}
	// both ends placed: the line is drawn and shows its own routing
	call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_b", "x": 300, "y": 0}}})
	res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": "tree-horizontal", "relations": []string{"r_a_b_implements"}, "requestedByHuman": true})
	if res.IsError || strings.Contains(text, "not drawn") || !strings.Contains(text, "highlight=r_a_b_implements") {
		t.Fatalf("relation routing: %v %s", res.IsError, text)
	}
	_, _, edges := lookView(t, cs)
	if len(edges) != 1 || edges[0]["routing"] != "tree-horizontal" {
		t.Fatalf("edge routing: %v", edges)
	}
	// null removes it and the emptied entry, and with it the edges key
	if res, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "routing": nil, "relations": []string{"r_a_b_implements"}, "requestedByHuman": true}); res.IsError {
		t.Fatalf("relation routing null: %s", text)
	}
	if _, _, edges = lookView(t, cs); len(edges) != 1 || edges[0]["routing"] != nil {
		t.Fatalf("edge routing not removed: %v", edges)
	}
	call(t, cs, "save", map[string]any{"requestedByHuman": true})
	data, _ := os.ReadFile(viewFile)
	if strings.Contains(string(data), `"edges"`) || strings.Contains(string(data), `"routing"`) {
		t.Fatalf("the emptied edges list stayed in the file:\n%s", data)
	}
}
