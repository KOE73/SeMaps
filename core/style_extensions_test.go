package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func extensionEdit(t *testing.T, w *WorkspaceModel, collection string, edit StyleEdit) StyleExtensionDetail {
	t.Helper()
	d, err := w.EditStyleExtension(collection, edit, "agent")
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func modifier(t *testing.T, w *WorkspaceModel, id string, patches map[string]any) {
	t.Helper()
	extensionEdit(t, w, ModifiersCollection, StyleEdit{Action: "create", ID: id, Definition: map[string]any{"name": id, "patches": patches}})
}
func ruleDefinition(id string, selector any, modifiers ...string) map[string]any {
	return map[string]any{"id": id, "name": id, "selectorVersion": 1, "selector": selector, "modifierIds": modifiers}
}
func addRule(t *testing.T, w *WorkspaceModel, id string, selector any, modifiers ...string) {
	t.Helper()
	extensionEdit(t, w, StyleRulesCollection, StyleEdit{Action: "create", ID: id, Definition: ruleDefinition(id, selector, modifiers...)})
}
func appearance(t *testing.T, w *WorkspaceModel) StyleAppearance {
	t.Helper()
	v, err := w.View("p", "v_main")
	if err != nil {
		t.Fatal(err)
	}
	return v.Appearances["e_a"]
}
func resolved(t *testing.T, a StyleAppearance) map[string]any {
	t.Helper()
	var r map[string]any
	if err := json.Unmarshal(a.Resolved, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestStyleModifierCascadeExclusionsAndSparseFonts(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	editTestStyle(t, w, StyleEdit{Action: "update", ID: "base", Patch: map[string]any{"title": map[string]any{"family": "Consolas", "size": 19, "color": "#111"}}})
	modifier(t, w, "palette", map[string]any{"block": map[string]any{"fill": "#aabbcc80", "border": map[string]any{"color": "#aa0000"}, "title": map[string]any{"color": "#222", "opacity": 0.6}}})
	modifier(t, w, "second", map[string]any{"block": map[string]any{"border": map[string]any{"color": "#00aa00"}}})
	modifier(t, w, "manual", map[string]any{"block": map[string]any{"border": map[string]any{"color": "#0000aa"}}})
	selector := map[string]any{"field": "kind", "op": "eq", "value": "class"}
	addRule(t, w, "a", selector, "palette")
	addRule(t, w, "z", selector, "second")
	a := appearance(t, w)
	r := resolved(t, a)
	if r["fill"] != "#aabbcc80" || r["border"].(map[string]any)["color"] != "#00aa00" || a.Origins["border.color"] != "rule:z/second" {
		t.Fatal(string(a.Resolved), a.Origins)
	}
	title := r["title"].(map[string]any)
	if title["family"] != "Consolas" || title["size"] != float64(19) || title["opacity"] != 0.6 {
		t.Fatal(title)
	}
	// An entity placed after rules exist receives formatting without stored style assignments.
	if _, err := p.Apply([]Op{
		{Kind: "entity", ID: "e_new", Value: json.RawMessage(`{"id":"e_new","name":"New","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.New"}]}`)},
		{Kind: "placement", ID: "e_new", View: "v_main", Value: json.RawMessage(`{"entity":"e_new","parent":null,"x":700,"y":20,"width":210,"height":60}`)},
	}, "human"); err != nil {
		t.Fatal(err)
	}
	newView, err := w.View("p", "v_main")
	if err != nil {
		t.Fatal(err)
	}
	if got := newView.Appearances["e_new"]; len(got.Applied) != 2 || resolved(t, got)["border"].(map[string]any)["color"] != "#00aa00" {
		t.Fatal("new placement did not receive active rules", got)
	}
	if got := findPlacement(mapObject(newView.View), "e_new"); has(got, "modifiers") || has(got, "override") || has(got, "styleId") {
		t.Fatal("automatic formatting materialized in placement", string(encodeJSON(got)))
	}
	if _, err := p.SetStyleModifiers("v_main", []string{"e_a"}, nil, map[string]json.RawMessage{"modifiers": json.RawMessage(`["manual"]`)}, true, "human"); err != nil {
		t.Fatal(err)
	}
	a = appearance(t, w)
	if resolved(t, a)["border"].(map[string]any)["color"] != "#0000aa" || a.Origins["border.color"] != "manual:manual" {
		t.Fatal(a)
	}
	changeStylePlacement(t, p, func(o *object) {
		o.set("override", map[string]any{"border": map[string]any{"color": "#ffff00"}, "icon": map[string]any{"glyph": "class"}})
	})
	a = appearance(t, w)
	if resolved(t, a)["border"].(map[string]any)["color"] != "#ffff00" || a.Origins["border.color"] != "override" {
		t.Fatal(a)
	}
	_, err = p.SetStyleModifiers("v_main", []string{"e_a"}, nil, map[string]json.RawMessage{"modifiers": json.RawMessage(`[]`), "disabledModifiers": json.RawMessage(`["second"]`)}, true, "human")
	if err != nil {
		t.Fatal(err)
	}
	a = appearance(t, w)
	if len(a.Applied) != 1 || len(a.Skipped) != 1 {
		t.Fatal(a)
	}
	_, err = p.SetStyleRuleExclusions("v_main", []string{"a"}, true, "human")
	if err != nil {
		t.Fatal(err)
	}
	a = appearance(t, w)
	if len(a.Applied) != 0 || len(a.Skipped) != 2 {
		t.Fatal(a)
	}
	if _, err := p.SetStyleRuleExclusions("v_main", nil, true, "human"); err != nil {
		t.Fatal(err)
	}
	view, _ := p.View("v_main")
	if strings.Contains(string(view), "disabledStyleRules") || len(appearance(t, w).Applied) != 1 {
		t.Fatal("rule exclusions did not clear", string(view))
	}
	before := string(encodeJSON(stylePlacement(t, p)))
	for _, fields := range []map[string]json.RawMessage{{"modifiers": json.RawMessage(`["absent"]`)}, {"automaticModifiers": json.RawMessage(`"false"`)}} {
		if _, err := p.SetStyleModifiers("v_main", []string{"e_a"}, nil, fields, true, "human"); err == nil {
			t.Fatal("invalid assignment accepted")
		}
		if string(encodeJSON(stylePlacement(t, p))) != before {
			t.Fatal("failed operation mutated placement")
		}
	}
}

func TestStyleSelectorsStructuralFactsAndEndpointFiltering(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	var e map[string]any
	_ = json.Unmarshal(encodeJSON(p.record("entity", "e_a")), &e)
	e["namespace"] = "N.Core.Services"
	ops := []Op{{Kind: "library", ID: "lib.external", Value: json.RawMessage(`{"id":"lib.external","name":"External","version":"1"}`)}, {Kind: "entity", ID: "e_a", Value: encodeJSON(e)}}
	for _, id := range []string{"e_real", "e_visual", "e_b"} {
		kind := "assembly"
		ns := "N.CoreX"
		if id == "e_b" {
			kind = "interface"
		}
		ops = append(ops, Op{Kind: "entity", ID: id, Value: json.RawMessage(`{"id":"` + id + `","kind":"` + kind + `","namespace":"` + ns + `","name":"` + id + `","origin":"code","code":[{"lang":"csharp","symbol":"` + id + `","library":"lib.external"}]}`)})
	}
	ops = append(ops, Op{Kind: "relation", ID: "r_contains", Value: json.RawMessage(`{"id":"r_contains","type":"contains","from":"e_real","to":"e_a","origin":"code"}`)}, Op{Kind: "relation", ID: "r_uses", Value: json.RawMessage(`{"id":"r_uses","type":"uses","from":"e_a","to":"e_b","origin":"code"}`)}, Op{Kind: "placement", ID: "e_visual", View: "v_main", Value: json.RawMessage(`{"entity":"e_visual","parent":null,"x":0,"y":0,"width":500,"height":400}`)}, Op{Kind: "placement", ID: "e_a", View: "v_main", Value: json.RawMessage(`{"entity":"e_a","parent":"e_visual","x":10,"y":20,"width":210,"height":60,"styleId":"variant"}`)}, Op{Kind: "placement", ID: "e_b", View: "v_main", Value: json.RawMessage(`{"entity":"e_b","parent":null,"x":600,"y":0,"width":210,"height":60}`)})
	if _, err := p.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	modifier(t, w, "library", map[string]any{"block": map[string]any{"fill": "#abc"}, "container": map[string]any{"fill": "#abc"}, "edge": map[string]any{"line": map[string]any{"color": "#abc"}}})
	both := map[string]any{"all": []any{map[string]any{"field": "assembly", "op": "eq", "value": "e_real"}, map[string]any{"field": "namespace", "op": "within", "value": "N.Core"}}}
	addRule(t, w, "structural", both, "library")
	a := appearance(t, w)
	if a.BaseStyleID != "variant" || len(a.Applied) != 1 || !reflect.DeepEqual(a.Facts["assembly"], []string{"e_real"}) {
		t.Fatal(a)
	}
	addRule(t, w, "edge", map[string]any{"all": []any{map[string]any{"field": "family", "op": "eq", "value": "edge"}, map[string]any{"field": "source.assembly", "op": "eq", "value": "e_real"}, map[string]any{"field": "target.library", "op": "eq", "value": "lib.external"}}}, "library")
	v, err := w.View("p", "v_main")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Appearances["r_uses"].Applied) != 1 || len(v.Appearances["e_b"].Applied) != 0 {
		t.Fatal(v.Appearances)
	}
	draft := ruleDefinition("preview", map[string]any{"field": "namespace", "op": "within", "value": "N.Core"}, "library")
	matches, err := w.PreviewStyleRule("p", "v_main", encodeJSON(draft))
	if err != nil || len(matches) != 1 || matches[0].ID != "e_a" {
		t.Fatal(matches, err)
	}
	contexts := appearanceFacts(p, "p", "v_main")
	for _, raw := range []string{`{"not":{"field":"assembly","op":"eq","value":"e_real"}}`, `{"field":"assembly","op":"neq","value":"e_real"}`} {
		s, err := parseSelector(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		if selectorMatch(s, contexts["e_b"]) != selectorUnknown {
			t.Fatal("missing assembly matched negation")
		}
	}
	s, _ := parseSelector(json.RawMessage(`{"field":"assembly","op":"exists","value":false}`))
	if selectorMatch(s, contexts["e_b"]) != selectorTrue {
		t.Fatal("exists failed")
	}
}

func TestStyleExtensionValidationPersistenceRenameAndDelete(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	modifier(t, w, "old", map[string]any{"block": map[string]any{"fill": "#abc"}})
	addRule(t, w, "rule", map[string]any{"field": "kind", "op": "eq", "value": "class"}, "old")
	if _, err := p.SetStyleModifiers("v_main", []string{"e_a"}, nil, map[string]json.RawMessage{"modifiers": json.RawMessage(`["old"]`)}, true, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SetStyleRuleExclusions("v_main", []string{"rule"}, true, "human"); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err == nil {
		t.Fatal("project saved before new shared definitions")
	}
	if err := w.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []StyleEdit{{Action: "update", ID: "old", Patch: map[string]any{"patches": map[string]any{"block": map[string]any{"template": "x"}}}}, {Action: "update", ID: "old", Patch: map[string]any{"patches": map[string]any{"block": map[string]any{"header": map[string]any{"height": 100}}}}}, {Action: "delete", ID: "old"}} {
		if _, err := w.EditStyleExtension(ModifiersCollection, edit, "agent"); err == nil {
			t.Fatal("unsafe definition accepted", edit)
		}
	}
	extensionEdit(t, w, ModifiersCollection, StyleEdit{Action: "rename", ID: "old", NewID: "new"})
	extensionEdit(t, w, StyleRulesCollection, StyleEdit{Action: "rename", ID: "rule", NewID: "renamed.rule"})
	changeStylePlacement(t, p, func(o *object) { o.set("x", 345) })
	restart, err := LoadWorkspaceModel(w.root, styleTestSheet, styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := restart.Project("p")
	if !reflect.DeepEqual(stringList(stylePlacement(t, p2), "modifiers"), []string{"new"}) {
		t.Fatal("rename missing after restart")
	}
	if err := restart.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
	placement := stylePlacement(t, p2)
	if !reflect.DeepEqual(stringList(placement, "modifiers"), []string{"old"}) || string(placement.vals["x"]) != "345" {
		t.Fatal(string(encodeJSON(placement)))
	}
	v, _ := p2.View("v_main")
	if !reflect.DeepEqual(stringList(mapObject(json.RawMessage(v)), "disabledStyleRules"), []string{"rule"}) {
		t.Fatal(string(v))
	}
	extensionEdit(t, restart, ModifiersCollection, StyleEdit{Action: "rename", ID: "old", NewID: "new"})
	if err := restart.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	if err := p2.Save(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(w.root, "projects/p/views/v.view.json"))
	if !strings.Contains(string(body), `"new"`) || !strings.Contains(string(body), "345") {
		t.Fatal(string(body))
	}
	old := uint64(0)
	if _, err := restart.EditStyleExtension(ModifiersCollection, StyleEdit{Action: "update", ID: "new", Patch: map[string]any{"name": "conflict"}, Expected: &old}, "agent"); err == nil {
		t.Fatal("stale revision accepted")
	}
	modifier(t, restart, "replacement", map[string]any{"block": map[string]any{"border": map[string]any{"color": "#111"}}})
	extensionEdit(t, restart, ModifiersCollection, StyleEdit{Action: "delete", ID: "new", Replacement: "replacement"})
	if !reflect.DeepEqual(stringList(stylePlacement(t, p2), "modifiers"), []string{"replacement"}) {
		t.Fatal("replacement not assigned")
	}
}

func TestStyleSelectorValidation(t *testing.T) {
	if _, err := parseSelector(json.RawMessage(`{"field":"id","op":"exists","value":null}`)); err == nil {
		t.Fatal("null presence predicate accepted")
	}
	for _, raw := range []string{`{}`, `{"all":[]}`, `{"all":[{"field":"id","op":"eq","value":"a"}],"field":"id"}`, `{"field":"script","op":"eq","value":"a"}`, `{"field":"id","op":"eval","value":"a"}`, `{"field":"id","op":"exists","value":"yes"}`, `{"field":"id","op":"in","value":[]}`, `{"field":"id","op":"eq","value":true}`, `{"field":"id","op":"eq","value":"a","code":"x"}`} {
		if _, err := parseSelector(json.RawMessage(raw)); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	raw := `{"field":"id","op":"eq","value":"a"}`
	for i := 0; i < 18; i++ {
		raw = `{"not":` + raw + `}`
	}
	if _, err := parseSelector(json.RawMessage(raw)); err == nil {
		t.Fatal("unbounded tree accepted")
	}
}

func TestModifierSavedReferencesHiddenByPendingAssignments(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	modifier(t, w, "old", map[string]any{"block": map[string]any{"fill": "#abc"}})
	if err := w.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SetStyleModifiers("v_main", []string{"e_a"}, nil, map[string]json.RawMessage{"modifiers": json.RawMessage(`["old"]`)}, true, "human"); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	changeStylePlacement(t, p, func(o *object) { o.del("modifiers"); o.set("x", 123) })
	modifier(t, w, "edge.only", map[string]any{"edge": map[string]any{}})
	for _, edit := range []StyleEdit{{Action: "delete", ID: "old", Replacement: "edge.only"}, {Action: "update", ID: "old", Patch: map[string]any{"patches": map[string]any{"block": nil, "edge": map[string]any{}}}}} {
		if _, err := w.EditStyleExtension(ModifiersCollection, edit, "agent"); err == nil {
			t.Fatal("hidden saved reference allowed incompatible modifier", edit)
		}
	}
	extensionEdit(t, w, ModifiersCollection, StyleEdit{Action: "rename", ID: "old", NewID: "new"})
	if err := w.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	disk, err := LoadModelWithoutJournal(w.root, "p", styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	if got := stylePlacement(t, disk); !reflect.DeepEqual(stringList(got, "modifiers"), []string{"new"}) || string(got.vals["x"]) != "10" {
		t.Fatal(string(encodeJSON(got)))
	}
	if got := stylePlacement(t, p); len(stringList(got, "modifiers")) != 0 || string(got.vals["x"]) != "123" {
		t.Fatal(string(encodeJSON(got)))
	}
	if err := p.Discard("all", ""); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stringList(stylePlacement(t, p), "modifiers"), []string{"new"}) {
		t.Fatal("discard restored stale modifier reference")
	}
}

func TestDiscardModifierReplacementRestoresListsAndPreservesIndependentAssignments(t *testing.T) {
	w := stylesFixture(t)
	modifier(t, w, "old", map[string]any{"block": map[string]any{"fill": "#abc"}})
	modifier(t, w, "replacement", map[string]any{"block": map[string]any{"fill": "#def"}})
	if err := w.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p", "q"} {
		m, _ := w.Project(id)
		list := json.RawMessage(`["old","replacement"]`)
		if id == "q" {
			list = json.RawMessage(`["replacement"]`)
		}
		if _, err := m.SetStyleModifiers("v_main", []string{"e_a"}, nil, map[string]json.RawMessage{"modifiers": list}, true, "human"); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
	}
	extensionEdit(t, w, ModifiersCollection, StyleEdit{Action: "delete", ID: "old", Replacement: "replacement"})
	for _, id := range []string{"p", "q"} {
		m, _ := w.Project(id)
		changeStylePlacement(t, m, func(o *object) { o.set("x", 333) })
	}
	if err := w.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p", "q"} {
		m, _ := w.Project(id)
		got := stylePlacement(t, m)
		want := []string{"old", "replacement"}
		if id == "q" {
			want = []string{"replacement"}
		}
		if !reflect.DeepEqual(stringList(got, "modifiers"), want) || string(got.vals["x"]) != "333" {
			t.Fatal(id, string(encodeJSON(got)))
		}
	}
}

func TestCheckStyleExtensionsUsesSavedFilesAndRejectsMalformedDefinitions(t *testing.T) {
	w := stylesFixture(t)
	modifier(t, w, "palette", map[string]any{"block": map[string]any{}})
	// Unsaved workspace definitions do not become saved check input.
	if err := CheckStyleExtensions(w.root, styleTestKinds); err != nil {
		t.Fatal(err)
	}
	for _, sheet := range []string{`{"modifiers":null}`, `{"modifiers":{}}`, `{"modifiers":[null]}`, `{"modifiers":[{"id":"a","name":"A","patches":{"block":{}}},{"id":"a","name":"B","patches":{"block":{}}}]}`, `{"modifiers":[{"id":"bad,id","name":"A","patches":{"block":{}}}]}`, `{"modifiers":[{"id":"a","name":"A","description":12,"patches":{"block":{}}}]}`} {
		if err := os.WriteFile(filepath.Join(w.root, "styles.json"), []byte(sheet), 0644); err != nil {
			t.Fatal(err)
		}
		if err := CheckStyleExtensions(w.root, styleTestKinds); err == nil {
			t.Fatal("saved check accepted malformed definitions", sheet)
		}
		if _, err := LoadWorkspaceModel(w.root, styleTestSheet, styleTestKinds); err == nil {
			t.Fatal("host accepted malformed definitions", sheet)
		}
	}
}
