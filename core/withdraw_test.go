package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ADR_20260930-8: a record created in the unsaved state can be withdrawn.

const withdrawRel = `{"id":"r_a_x_link","from":"e_a","to":"e_x","type":"link","origin":"authored"}`
const withdrawEnt = `{"id":"e_new","kind":"class","origin":"authored"}`
const withdrawName = `{"name":{"v":"New","origin":"authored","at":"2026-09-30T00:00:00Z"}}`
const withdrawPlace = `{"entity":"e_new","parent":null,"x":1,"y":2}`

func readAll(t *testing.T, ws string, names ...string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(ws, "projects", "p", filepath.FromSlash(n)))
		if err != nil {
			t.Fatal(err)
		}
		out[n] = b
	}
	return out
}

func TestWithdrawRelationLeavesNothing(t *testing.T) {
	ws := editWorkspace(t)
	files := []string{"relations.json", "entities.json", "text.ru.json", "views/main.view.json"}
	before := readAll(t, ws, files...)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", withdrawRel)}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("text", "r_a_x_link", "", "ru", `{"title":{"v":"L","origin":"authored","at":"2026-09-30T00:00:00Z"}}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(); len(d.Registry) != 2 {
		t.Fatalf("dirty after create: %+v", d)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", "null")}, "human"); err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(); len(d.Registry) != 0 {
		t.Fatalf("withdrawn record still unsaved: %+v", d)
	}
	items, _ := m.Records("relations.json")
	if len(items) != 1 {
		t.Fatalf("relation still in the working model: %d", len(items))
	}
	// the journal replays: create, text, withdraw
	m2, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if d := m2.Dirty(); len(d.Registry) != 0 {
		t.Fatalf("replayed dirty: %+v", d)
	}
	if items, _ := m2.Records("relations.json"); len(items) != 1 {
		t.Fatal("ghost relation after replay")
	}
	// Save writes nothing for it
	if err := m2.Save(); err != nil {
		t.Fatal(err)
	}
	after := readAll(t, ws, files...)
	for _, f := range files {
		if !bytes.Equal(before[f], after[f]) {
			t.Fatalf("%s changed by create+withdraw+save:\n%s", f, after[f])
		}
	}
	// and the id is free to be created again
	if _, err := m2.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", withdrawRel)}, "human"); err != nil {
		t.Fatal(err)
	}
	if len(m2.Dirty().Registry) != 1 {
		t.Fatal("re-created record not unsaved")
	}
}

func TestWithdrawThenDiscard(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", withdrawRel)}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", "null")}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", withdrawRel)}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := m.Discard("registry", ""); err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(); len(d.Registry) != 0 {
		t.Fatalf("dirty after discard: %+v", d)
	}
	if items, _ := m.Records("relations.json"); len(items) != 1 {
		t.Fatal("ghost after discard")
	}
	// a view discard keeps the registry create and withdraw and replays clean
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", withdrawRel), modelOp("view", "v_main", "v_main", "", `{"id":"v_main","title":"t"}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply([]Op{modelOp("relation", "r_a_x_link", "", "", "null")}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := m.Discard("view", "v_main"); err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(); len(d.Registry) != 0 || len(d.Views) != 0 {
		t.Fatalf("dirty: %+v", d)
	}
}

func TestWithdrawEntityWithItsPlacementAndTexts(t *testing.T) {
	ws := editWorkspace(t)
	files := []string{"entities.json", "text.ru.json", "views/main.view.json"}
	before := readAll(t, ws, files...)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	create := []Op{
		modelOp("entity", "e_new", "", "", withdrawEnt),
		modelOp("text", "e_new", "", "ru", withdrawName),
		modelOp("placement", "e_new", "v_main", "", withdrawPlace),
	}
	if _, err := m.Apply(create, "human"); err != nil {
		t.Fatal(err)
	}
	// still placed: refused as a whole, nothing changes
	_, err = m.Apply([]Op{modelOp("entity", "e_new", "", "", "null")}, "human")
	refused(t, err, "still stands on v_main")
	if items, _ := m.Records("entities.json"); len(items) != 5 {
		t.Fatal("refused batch changed the model")
	}
	// the texts go with the record, the placement is removed in the same batch
	if _, err := m.Apply([]Op{modelOp("entity", "e_new", "", "", "null"), modelOp("placement", "e_new", "v_main", "", "null")}, "human"); err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(); len(d.Registry) != 0 {
		t.Fatalf("registry dirty: %+v", d.Registry)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after := readAll(t, ws, []string{"entities.json", "text.ru.json"}...)
	for _, f := range []string{"entities.json", "text.ru.json"} {
		if !bytes.Equal(before[f], after[f]) {
			t.Fatalf("%s changed:\n%s", f, after[f])
		}
	}
}

func TestWithdrawRefusals(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	// a saved record of code is never removed (an authored one is: remove_test.go)
	_, err = m.Apply([]Op{modelOp("relation", "r_a_b_items_item", "", "", "null")}, "human")
	refused(t, err, "comes from code")
	_, err = m.Apply([]Op{modelOp("entity", "e_a", "", "", "null")}, "human")
	refused(t, err, "comes from code")
	// a text is still not removable by itself
	_, err = m.Apply([]Op{modelOp("text", "e_x", "", "ru", "null")}, "human")
	refused(t, err, "cannot be removed")

	// dangling references
	if _, err := m.Apply([]Op{modelOp("entity", "e_new", "", "", withdrawEnt), modelOp("text", "e_new", "", "ru", withdrawName)}, "human"); err != nil {
		t.Fatal(err)
	}
	rel := `{"id":"r_new_x_link","from":"e_new","to":"e_x","type":"link","origin":"authored"}`
	if _, err := m.Apply([]Op{modelOp("relation", "r_new_x_link", "", "", rel)}, "human"); err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply([]Op{modelOp("entity", "e_new", "", "", "null")}, "human")
	refused(t, err, "relation r_new_x_link still has it as an end")

	edge := `{"id":"v_main","relations":{"default":"visible","except":["r_new_x_link"]}}`
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", edge)}, "human"); err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply([]Op{modelOp("relation", "r_new_x_link", "", "", "null")}, "human")
	refused(t, err, "relations.except")
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","relations":{"default":"visible"},"edges":[{"id":"r_new_x_link","routing":"orthogonal"}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply([]Op{modelOp("relation", "r_new_x_link", "", "", "null")}, "human")
	refused(t, err, "entry for it in edges")

	// everything in one batch is accepted
	batch := []Op{
		modelOp("relation", "r_new_x_link", "", "", "null"),
		modelOp("view", "v_main", "v_main", "", `{"id":"v_main","relations":{"default":"visible"},"edges":null}`),
		modelOp("entity", "e_new", "", "", "null"),
	}
	if _, err := m.Apply(batch, "human"); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Dirty().Registry {
		if r.Kind != "project" {
			t.Fatalf("leftover unsaved ref %+v", r)
		}
	}
}
