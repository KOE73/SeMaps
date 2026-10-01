package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR_20261001: a saved authored record can be removed, with the names the
// views and relations hold of it, as one unsaved batch.

// removalWorkspace is editWorkspace with saved authored records: e_x stands on
// v_main, e_y is unplaced; r_a_x_link (e_a → e_x) is drawn with an edges entry,
// r_x_y_link (e_x → e_y) is hidden through relations.except.
func removalWorkspace(t *testing.T) string {
	t.Helper()
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("entities.json", `{"entities":[
  {"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.A"}]},
  {"id":"e_b","name":"B","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.B"}]},
  {"id":"e_x","kind":"app","origin":"authored"},
  {"id":"e_y","kind":"app","origin":"authored"},
  {"id":"e_core","kind":"group","origin":"authored"}]}`)
	write("relations.json", `{"contractVersion":5,"relations":[
  {"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","origin":"code","status":"present",
   "evidence":[{"lang":"csharp","symbol":"N.A","via":{"member":"items","path":["item"],"cardinality":"many","mutability":"mutable"}}]},
  {"id":"r_a_x_link","from":"e_a","to":"e_x","type":"link","origin":"authored"},
  {"id":"r_x_y_link","from":"e_x","to":"e_y","type":"link","origin":"authored"}]}`)
	write("text.ru.json", `{"contractVersion":5,"language":"ru","entries":{
  "e_x":{"name":{"v":"X","at":"2026-09-24T00:00:00Z","origin":"authored"}},
  "r_a_x_link":{"title":{"v":"L","at":"2026-09-24T00:00:00Z","origin":"authored"}},
  "e_core":{"name":{"v":"Core","at":"2026-09-24T00:00:00Z","origin":"authored"}}}}`)
	write("views/main.view.json", `{"id":"v_main","project":"p","axis":"axis_layer","relations":{"default":"visible","except":["r_x_y_link"]},
  "placements":[{"entity":"e_core","parent":null,"x":0,"y":0,"width":500,"height":500},
   {"entity":"e_a","parent":null,"x":10,"y":20},{"entity":"e_x","parent":null,"x":60,"y":20}],
  "edges":[{"id":"r_a_x_link","routing":"orthogonal"}]}`)
	return ws
}

func TestRemoveAuthoredRelationCleansViewsAndDiscardRestores(t *testing.T) {
	ws := removalWorkspace(t)
	files := []string{"relations.json", "entities.json", "text.ru.json", "views/main.view.json"}
	before := readAll(t, ws, files...)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.RemoveRecords([]string{"r_a_x_link", "r_x_y_link"}, false, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Relations) != 2 || len(res.Edges) != 1 || res.Edges[0] != "v_main#r_a_x_link" || len(res.Except) != 1 || res.Except[0] != "v_main#r_x_y_link" {
		t.Fatalf("result: %+v", res)
	}
	removed := 0
	for _, r := range m.Dirty().Registry {
		if r.Kind == "relation" && r.Removed && r.Author == "agent" {
			removed++
		}
	}
	if removed != 2 {
		t.Fatalf("a removal must be listed as such: %+v", m.Dirty().Registry)
	}
	if items, _ := m.Records("relations.json"); len(items) != 1 {
		t.Fatalf("relations in the working model: %d", len(items))
	}
	// the file still has it until Save
	if got := readAll(t, ws, "relations.json")["relations.json"]; !strings.Contains(string(got), "r_a_x_link") {
		t.Fatal("removal reached the file before Save")
	}
	// the journal replays: a reload of the working state still has it removed
	m2, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if items, _ := m2.Records("relations.json"); len(items) != 1 {
		t.Fatalf("replayed: %d relations", len(items))
	}
	// Discard brings everything back
	if err := m2.Discard("all", ""); err != nil {
		t.Fatal(err)
	}
	if d := m2.Dirty(); len(d.Registry) != 0 || len(d.Views) != 0 {
		t.Fatalf("dirty after discard: %+v", d)
	}
	if items, _ := m2.Records("relations.json"); len(items) != 3 {
		t.Fatalf("discard did not restore: %d relations", len(items))
	}
	// Save writes the files without the records and the entries naming them
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after := readAll(t, ws, files...)
	for _, gone := range []string{"relations.json", "views/main.view.json", "text.ru.json"} {
		if strings.Contains(string(after[gone]), "r_a_x_link") || strings.Contains(string(after[gone]), "r_x_y_link") {
			t.Fatalf("%s still names a removed relation:\n%s", gone, after[gone])
		}
	}
	if string(after["entities.json"]) != string(before["entities.json"]) {
		t.Fatal("entities.json changed")
	}
	if !strings.Contains(string(after["relations.json"]), "r_a_b_items_item") {
		t.Fatal("the code relation went too")
	}
}

func TestRemoveRefusesCodeRecords(t *testing.T) {
	ws := removalWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.RemoveRecords([]string{"r_a_b_items_item"}, true, "agent")
	refused(t, err, "come from code")
	_, err = m.RemoveRecords([]string{"e_a"}, true, "agent")
	refused(t, err, "come from code")
	_, err = m.RemoveRecords([]string{"r_nope"}, true, "agent")
	refused(t, err, "no entity or relation r_nope")
	// an authored entity that is the end of a relation from code stays
	rel := `{"id":"r_a_y_code","from":"e_a","to":"e_y","type":"link","origin":"code","status":"present"}`
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_y_code", "", "", rel)}, "human"); err != nil {
		t.Fatal(err)
	}
	_, err = m.RemoveRecords([]string{"e_y"}, true, "agent")
	refused(t, err, "comes from code")
	if len(m.Dirty().Registry) != 1 {
		t.Fatalf("a refused removal changed the model: %+v", m.Dirty().Registry)
	}
}

func TestRemoveEntityNeedsCascadeForItsRelations(t *testing.T) {
	ws := removalWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.RemoveRecords([]string{"e_x"}, false, "agent")
	refused(t, err, "r_a_x_link")
	if len(m.Dirty().Registry) != 0 {
		t.Fatal("a refused removal changed the model")
	}
	res, err := m.RemoveRecords([]string{"e_x"}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) != 1 || len(res.Relations) != 2 || len(res.Placements) != 1 || res.Placements[0] != "v_main#e_x" || len(res.Edges) != 1 || len(res.Except) != 1 {
		t.Fatalf("result: %+v", res)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after := readAll(t, ws, "entities.json", "relations.json", "text.ru.json", "views/main.view.json")
	for name, b := range after {
		for _, id := range []string{`"e_x"`, "r_a_x_link", "r_x_y_link"} {
			if strings.Contains(string(b), id) {
				t.Fatalf("%s still names %s:\n%s", name, id, b)
			}
		}
	}
	if !strings.Contains(string(after["entities.json"]), "e_y") || !strings.Contains(string(after["views/main.view.json"]), "e_a") {
		t.Fatal("removed more than asked")
	}
	// a reload sees the saved state, with no dirty left
	m3, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if d := m3.Dirty(); len(d.Registry) != 0 {
		t.Fatalf("dirty after save: %+v", d)
	}
}

// A removal in the unsaved state alone, through the plain op, is a removal
// too: the dirty ref says so and a re-created record is an edit again.
func TestRemoveByOpIsMarkedRemovedUntilRecreated(t *testing.T) {
	ws := removalWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	// e_y alone is still the end of r_x_y_link: the batch rule refuses
	_, err = m.Apply([]Op{modelOp("entity", "e_y", "", "", "null")}, "human")
	refused(t, err, "r_x_y_link")
	_, err = m.Apply([]Op{modelOp("relation", "r_x_y_link", "", "", "null"), modelOp("view", "v_main", "v_main", "", `{"id":"v_main","relations":{"default":"visible"}}`), modelOp("entity", "e_y", "", "", "null")}, "human")
	if err != nil {
		t.Fatal(err)
	}
	refs := m.Dirty().Registry
	if len(refs) != 2 || !refs[0].Removed && !refs[1].Removed {
		t.Fatalf("dirty: %+v", refs)
	}
	if _, err := m.Apply([]Op{modelOp("entity", "e_y", "", "", `{"id":"e_y","kind":"app","origin":"authored"}`), modelOp("text", "e_y", "", "ru", withdrawName)}, "human"); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Dirty().Registry {
		if r.Kind == "entity" && r.Removed {
			t.Fatalf("re-created record still marked removed: %+v", r)
		}
	}
}
