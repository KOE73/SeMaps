package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var styleTestKinds = []byte(`{"groups":[{"id":"types","name":{"en":"Types"},"kinds":[{"id":"class","name":{"en":"Class"}},{"id":"interface","name":{"en":"Interface"}},{"id":"assembly","name":{"en":"Assembly"},"container":true}]}],"relationGroups":[{"id":"links","name":{"en":"Links"},"types":[{"id":"uses","name":{"en":"Uses"}}]}]}`)
var styleTestSheet = []byte(`{"version":1,"styles":[{"id":"base","name":"Base","appliesTo":"block","forKinds":["class","interface"],"fill":"#fee","border":{"color":"#f00","width":2}},{"id":"variant","name":"Variant","appliesTo":"block","forKinds":["class"],"basedOn":"base","border":{"width":3}}]}`)

func stylesFixture(t *testing.T) *WorkspaceModel {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"p", "q"} {
		for file, body := range map[string]string{"project.json": `{"id":"` + id + `","contractVersion":5}`, "entities.json": `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.A"}]}]}`, "relations.json": `{"relations":[]}`, "views/v.view.json": `{"id":"v_main","axis":"layer","placements":[{"entity":"e_a","parent":null,"x":10,"y":20,"width":210,"height":60,"styleId":"base","template":"keep","override":{"icon":{"glyph":"class"}}}]}`} {
			path := filepath.Join(root, "projects", id, file)
			_ = os.MkdirAll(filepath.Dir(path), 0755)
			if err := os.WriteFile(path, []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	w, err := LoadWorkspaceModel(root, styleTestSheet, styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func editTestStyle(t *testing.T, w *WorkspaceModel, e StyleEdit) StyleDetail {
	t.Helper()
	d, err := w.EditStyle(e, "agent")
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func stylePlacement(t *testing.T, m *Model) *object {
	t.Helper()
	data, err := m.View("v_main")
	if err != nil {
		t.Fatal(err)
	}
	return viewItems(mapObject(json.RawMessage(data)), "placements")[0]
}
func changeStylePlacement(t *testing.T, m *Model, mutate func(*object)) {
	t.Helper()
	p := stylePlacement(t, m)
	mutate(p)
	_, err := m.Apply([]Op{{Kind: "placement", ID: "e_a", View: "v_main", Value: encodeJSON(p)}}, "human")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStylesInheritanceNullValidationAndConflicts(t *testing.T) {
	w := stylesFixture(t)
	editTestStyle(t, w, StyleEdit{Action: "update", ID: "variant", Patch: map[string]any{"border": map[string]any{"width": nil}}})
	detail, err := w.GetStyle("variant")
	if err != nil {
		t.Fatal(err)
	}
	resolved := mapObject(detail.Resolved)
	border, _ := child(resolved, "border")
	if border.str("color") != "#f00" || string(border.vals["width"]) != "2" {
		t.Fatal(string(detail.Resolved))
	}
	if len(detail.Dirty) != 1 || detail.Dirty[0].Author != "agent" {
		t.Fatal(detail.Dirty)
	}
	for _, e := range []StyleEdit{{Action: "create", ID: "untyped", Definition: map[string]any{"name": "Untyped"}}, {Action: "create", ID: "wrong", Definition: map[string]any{"name": "Wrong", "appliesTo": "container", "forKinds": []string{"class"}}}, {Action: "update", ID: "base", Patch: map[string]any{"basedOn": "variant"}}, {Action: "update", ID: "base", Patch: map[string]any{"border": map[string]any{"width": "bad"}}}, {Action: "update", ID: "base", Patch: map[string]any{"basedOn": "missing"}}} {
		if _, err := w.EditStyle(e, "human"); err == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
	old := uint64(0)
	if _, err := w.EditStyle(StyleEdit{Action: "update", ID: "base", Patch: map[string]any{"fill": "#abc"}, Expected: &old}, "human"); err == nil {
		t.Fatal("stale revision accepted")
	}
	items, err := w.ListStyles(StyleFilter{AppliesTo: "block", ForKinds: []string{"interface"}, Name: "base", ID: "base"})
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if err := w.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
	d, _ := w.GetStyle("variant")
	if len(d.Dirty) != 0 {
		t.Fatal(d.Dirty)
	}
}

func TestStylesRenameAtomicRestartSaveAndDiscardPreserveGeometry(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	changeStylePlacement(t, p, func(o *object) { o.set("x", 123) })
	editTestStyle(t, w, StyleEdit{Action: "rename", ID: "base", NewID: "renamed"})
	// A project that has never been opened was included in the transaction.
	q, _ := w.Project("q")
	if stylePlacement(t, q).str("styleId") != "renamed" {
		t.Fatal("closed project did not rename")
	}
	if err := p.Save(); err == nil {
		t.Fatal("project saved an unsaved shared rename")
	}
	changeStylePlacement(t, p, func(o *object) { o.set("y", 234) })
	restart, err := LoadWorkspaceModel(w.root, styleTestSheet, styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	p, err = restart.Project("p")
	if err != nil {
		t.Fatal(err)
	}
	if got := stylePlacement(t, p); got.str("styleId") != "renamed" || string(got.vals["x"]) != "123" || string(got.vals["y"]) != "234" {
		t.Fatal(got)
	}
	if err = restart.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
	if got := stylePlacement(t, p); got.str("styleId") != "base" || string(got.vals["x"]) != "123" || string(got.vals["y"]) != "234" || got.str("template") != "keep" {
		t.Fatal(got)
	}
	editTestStyle(t, restart, StyleEdit{Action: "rename", ID: "base", NewID: "renamed"})
	if err = restart.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	disk, err := LoadModelWithoutJournal(w.root, "p", styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	got := stylePlacement(t, disk)
	if got.str("styleId") != "renamed" || string(got.vals["x"]) != "10" || string(got.vals["y"]) != "20" {
		t.Fatalf("style Save included unrelated geometry: %s", encodeJSON(got))
	}
	if got := stylePlacement(t, p); got.str("styleId") != "renamed" || string(got.vals["x"]) != "123" {
		t.Fatal("working state lost")
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := LoadWorkspaceModel(w.root, styleTestSheet, styleTestKinds)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = again.Project("p")
	if got := stylePlacement(t, p); got.str("styleId") != "renamed" || string(got.vals["x"]) != "123" {
		t.Fatal(got)
	}
}

func TestStylesDeleteRequiresReplacementAndAssignmentsRequireDefinition(t *testing.T) {
	w := stylesFixture(t)
	p, _ := w.Project("p")
	if _, err := w.EditStyle(StyleEdit{Action: "delete", ID: "base"}, "agent"); err == nil || !strings.Contains(err.Error(), "q") || !strings.Contains(err.Error(), "variant") {
		t.Fatal(err)
	}
	// Replacement cannot create a cycle by replacing a base with its own child.
	if _, err := w.EditStyle(StyleEdit{Action: "delete", ID: "base", Replacement: "variant"}, "agent"); err == nil {
		t.Fatal("incompatible replacement accepted")
	}
	editTestStyle(t, w, StyleEdit{Action: "create", ID: "replacement", Definition: map[string]any{"name": "Replacement", "appliesTo": "block", "forKinds": []string{"class", "interface"}, "fill": "#0f0"}})
	editTestStyle(t, w, StyleEdit{Action: "delete", ID: "base", Replacement: "replacement"})
	if got := stylePlacement(t, p).str("styleId"); got != "replacement" {
		t.Fatal(got)
	}
	if err := w.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"missing", "container-style"} {
		got := stylePlacement(t, p)
		got.set("styleId", id)
		if _, err := p.Apply([]Op{{Kind: "placement", ID: "e_a", View: "v_main", Value: encodeJSON(got)}}, "agent"); err == nil {
			t.Fatal("unknown style accepted")
		}
	}
	editTestStyle(t, w, StyleEdit{Action: "create", ID: "new", Definition: map[string]any{"name": "New", "appliesTo": "block", "forKinds": []string{"class"}}})
	changeStylePlacement(t, p, func(o *object) { o.set("styleId", "new") })
	if err := w.DiscardStyles(); err == nil {
		t.Fatal("dangling assignment allowed by discard")
	}
	if err := p.Discard("all", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.DiscardStyles(); err != nil {
		t.Fatal(err)
	}
}
