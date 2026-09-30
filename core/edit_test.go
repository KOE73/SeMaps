package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// editWorkspace: one project with two entities, a code relation with via, a
// type and a view with one node.
func editWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json":     `{"id":"p","contractVersion":5}`,
		"../../kinds.json": `{"groups":[{"id":"t","name":{"ru":"т"},"kinds":[{"id":"group","name":{"ru":"группа"},"container":true}]}]}`,
		"entities.json": `{"entities":[
  {"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.A"}]},
  {"id":"e_b","name":"B","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.B"}]},
  {"id":"e_x","kind":"app","origin":"authored"},
  {"id":"e_core","kind":"group","origin":"authored"}]}`,
		"text.ru.json": `{"contractVersion":5,"language":"ru","entries":{
  "e_x":{"name":{"v":"X","at":"2026-09-24T00:00:00Z","origin":"authored"}},
  "e_core":{"name":{"v":"Core","at":"2026-09-24T00:00:00Z","origin":"authored"}}}}`,
		"relations.json": `{"contractVersion":5,"relations":[
  {"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","origin":"code","status":"present",
   "evidence":[{"lang":"csharp","symbol":"N.A","via":{"member":"items","path":["item"],"cardinality":"many","mutability":"mutable"}}]}]}`,
		"relation-types.json": `{"contractVersion":5,"relationTypes":[
  {"id":"holds.many","origin":"code","visibility":"visible"},
  {"id":"call","origin":"authored"}]}`,
		"views/main.view.json": `{"id":"v_main","project":"p","axis":"axis_layer","relations":{"default":"visible"},
  "placements":[{"entity":"e_core","parent":null,"x":0,"y":0,"width":500,"height":500},
   {"entity":"e_a","parent":null,"x":10,"y":20}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func readAs(t *testing.T, ws, name string, into any) {
	t.Helper()
	if err := readJSON(filepath.Join(ws, "projects", "p", filepath.FromSlash(name)), into); err != nil {
		t.Fatal(err)
	}
}

func refused(t *testing.T, err error, want string) {
	t.Helper()
	var e *EditError
	if !errors.As(err, &e) || !strings.Contains(err.Error(), want) {
		t.Fatalf("want a refusal with %q, got %v", want, err)
	}
}

func TestSetTextWritesAuthoredValueAndDropsTranslation(t *testing.T) {
	ws := editWorkspace(t)
	now = func() time.Time { return time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC) }
	defer func() { now = func() time.Time { return time.Now().UTC() } }()
	file := filepath.Join(ws, "projects", "p", "text.ru.json")
	os.WriteFile(file, []byte(`{"contractVersion":5,"language":"ru","entries":{"e_a":{"doc":{"v":"old","origin":"translated","from":"en","fromHash":"1"}}}}`), 0o644)

	if err := SetText(ws, "", "ru", "e_a", "description", "Держит B."); err != nil {
		t.Fatal(err)
	}
	if err := SetText(ws, "", "ru", "e_a", "doc", "Новое."); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Entries map[string]map[string]map[string]string `json:"entries"`
	}
	readAs(t, ws, "text.ru.json", &doc)
	d := doc.Entries["e_a"]["description"]
	if d["v"] != "Держит B." || d["origin"] != "authored" || d["at"] != "2026-09-24T10:00:00Z" {
		t.Fatalf("description: %v", d)
	}
	if doc.Entries["e_a"]["doc"]["from"] != "" || doc.Entries["e_a"]["doc"]["origin"] != "authored" {
		t.Fatalf("doc keeps its translation: %v", doc.Entries["e_a"]["doc"])
	}
	// A new language file is created.
	if err := SetText(ws, "", "en", "rt_call", "name", "call"); err != nil {
		t.Fatal(err)
	}
}

func TestSetTextRefusals(t *testing.T) {
	ws := editWorkspace(t)
	refused(t, SetText(ws, "", "ru", "r_a_b_items_item", "name", "x"), "comes from code")
	refused(t, SetText(ws, "", "ru", "e_a", "name", "x"), "not translated")
	refused(t, SetText(ws, "", "ru", "e_a", "note", "x"), "field")
	refused(t, SetText(ws, "", "ru", "q_a", "name", "x"), "prefix")
	refused(t, SetText(ws, "", "ru", "e_a", "doc", "  "), "empty")
}

func TestAddRelationMintsIDAndChecksEnds(t *testing.T) {
	ws := editWorkspace(t)
	id, err := AddRelation(ws, "", "e_a", "e_x", "call")
	if err != nil || id != "r_a_x_call" {
		t.Fatalf("id %q, err %v", id, err)
	}
	_, err = AddRelation(ws, "", "e_a", "e_x", "call")
	refused(t, err, "already")
	_, err = AddRelation(ws, "", "e_a", "e_nope", "call")
	refused(t, err, "no entity")
	_, err = AddRelation(ws, "", "e_a", "e_b", "flows")
	refused(t, err, "no relation type")

	var rels struct{ Relations []map[string]any }
	readAs(t, ws, "relations.json", &rels)
	last := rels.Relations[len(rels.Relations)-1]
	if last["origin"] != "authored" || last["type"] != "call" {
		t.Fatalf("added: %v", last)
	}
	// The code relation kept its via, in its evidence.
	if ev, _ := rels.Relations[0]["evidence"].([]any); len(ev) != 1 || ev[0].(map[string]any)["via"] == nil {
		t.Fatal("via of the code relation is gone")
	}
}

func TestAddRelationType(t *testing.T) {
	ws := editWorkspace(t)
	if err := AddRelationType(ws, "", "security", "hidden"); err != nil {
		t.Fatal(err)
	}
	refused(t, AddRelationType(ws, "", "security", ""), "exists")
	refused(t, AddRelationType(ws, "", "x", "shown"), "visibility")
}

func TestSetRelationVisibleKeepsExceptTheSmallerSide(t *testing.T) {
	ws := editWorkspace(t)
	view := func() map[string]any {
		var v map[string]any
		readAs(t, ws, "views/main.view.json", &v)
		return v
	}
	if err := SetRelationVisible(ws, "", "v_main", "r_a_b_items_item", false); err != nil {
		t.Fatal(err)
	}
	ex := view()["relations"].(map[string]any)["except"].([]any)
	if len(ex) != 1 || ex[0] != "r_a_b_items_item" {
		t.Fatalf("except %v", ex)
	}
	if err := SetRelationVisible(ws, "", "v_main", "r_a_b_items_item", true); err != nil {
		t.Fatal(err)
	}
	v := view()
	if ex := v["relations"].(map[string]any)["except"].([]any); len(ex) != 0 {
		t.Fatalf("except %v", ex)
	}
	if len(v["placements"].([]any)) != 2 {
		t.Fatal("geometry touched")
	}
	refused(t, SetRelationVisible(ws, "", "v_none", "r_a_b_items_item", true), "no view")
}

func TestConfirmRenames(t *testing.T) {
	ws := editWorkspace(t)
	if err := ConfirmEntityRename(ws, "", "e_a", "N.A2"); err != nil {
		t.Fatal(err)
	}
	refused(t, ConfirmEntityRename(ws, "", "e_x", "N.X"), "authored")
	if err := ConfirmRelationRename(ws, "", "r_a_b_items_item", "entries"); err != nil {
		t.Fatal(err)
	}
	id, _ := AddRelation(ws, "", "e_a", "e_x", "call")
	refused(t, ConfirmRelationRename(ws, "", id, "m"), "no via")

	var ents struct {
		Entities []struct{ Code []map[string]any }
	}
	readAs(t, ws, "entities.json", &ents)
	if code := ents.Entities[0].Code; len(code) != 1 || code[0]["symbol"] != "N.A2" || code[0]["lang"] != "csharp" {
		t.Fatalf("code %v", code)
	}
	var rels struct {
		Relations []struct {
			Evidence []struct{ Via map[string]any }
		}
	}
	readAs(t, ws, "relations.json", &rels)
	if via := rels.Relations[0].Evidence[0].Via; via["member"] != "entries" || via["path"] == nil {
		t.Fatalf("via %v", via)
	}
}

func TestPlaceEntitiesOnlyOnRequestAndNeverMoves(t *testing.T) {
	ws := editWorkspace(t)
	p := []Placement{{Entity: "e_b", Parent: "e_core", X: 100, Y: 100}}
	refused(t, PlaceEntities(ws, "", "v_main", p, false), "human")
	refused(t, PlaceEntities(ws, "", "v_main", []Placement{{Entity: "e_a", X: 1, Y: 1}}, true), "already")
	refused(t, PlaceEntities(ws, "", "v_main", []Placement{{Entity: "e_b", Parent: "e_none"}}, true), "no container")
	if err := PlaceEntities(ws, "", "v_main", p, true); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Placements []map[string]any `json:"placements"`
	}
	readAs(t, ws, "views/main.view.json", &v)
	if len(v.Placements) != 3 || v.Placements[1]["x"] != 10.0 || v.Placements[2]["parent"] != "e_core" {
		t.Fatalf("placements %v", v.Placements)
	}
}

func TestEditsKeepKeyOrder(t *testing.T) {
	ws := editWorkspace(t)
	if _, err := AddRelation(ws, "", "e_a", "e_x", "call"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "relations.json"))
	if !json.Valid(data) || !strings.Contains(string(data), `"id": "r_a_b_items_item",
      "from": "e_a"`) {
		t.Fatalf("order lost:\n%s", data)
	}
}
