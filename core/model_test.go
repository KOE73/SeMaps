package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func modelOp(kind, id, view, lang, value string) Op {
	return Op{Kind: kind, ID: id, View: view, Lang: lang, Value: json.RawMessage(value)}
}

func TestModelJournalSaveAndOrderedBytes(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	entity := `{"id":"e_a","name":"A edited","kind":"class","origin":"code","symbol":"N.A"}`
	if _, err := m.Apply([]Op{modelOp("entity", "e_a", "", "", entity)}, "human"); err != nil {
		t.Fatal(err)
	}
	m, err = LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Registry) != 1 {
		t.Fatal("journal did not replay")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(ws, "projects", "p", "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"entities\": [\n    {\n      \"id\": \"e_a\",\n      \"name\": \"A edited\",\n      \"kind\": \"class\",\n      \"origin\": \"code\",\n      \"symbol\": \"N.A\"\n    },\n    {\n      \"id\": \"e_b\",\n      \"name\": \"B\",\n      \"kind\": \"class\",\n      \"origin\": \"code\",\n      \"symbol\": \"N.B\"\n    },\n    {\n      \"id\": \"e_x\",\n      \"name\": \"X\",\n      \"kind\": \"app\",\n      \"origin\": \"authored\"\n    }\n  ]\n}\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("bytes differ from hand edit:\n%s", got)
	}
	if _, err := os.Stat(m.journalFile()); !os.IsNotExist(err) {
		t.Fatalf("journal remains: %v", err)
	}
}

func TestModelAtomicAndDiscardScopes(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	zone := `{"id":"z_core","container":null,"x":4,"y":0,"width":500,"height":500}`
	valid := modelOp("zone", "z_core", "v_main", "", zone)
	bad := modelOp("entity", "e_b", "", "", "null")
	if _, err := m.Apply([]Op{valid, bad}, "human"); err == nil {
		t.Fatal("accepted null registry value")
	}
	if len(m.Dirty().Views) != 0 {
		t.Fatal("partial batch applied")
	}
	if _, err := os.Stat(m.journalFile()); !os.IsNotExist(err) {
		t.Fatal("partial batch journaled")
	}
	text := modelOp("text", "e_a", "", "ru", `{"description":{"v":"edited","origin":"authored"}}`)
	if _, err := m.Apply([]Op{valid, text}, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := m.Discard("view", "v_main"); err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Views) != 0 || len(m.Dirty().Registry) != 1 {
		t.Fatalf("bad view discard: %+v", m.Dirty())
	}
	m, err = LoadModel(ws, "p")
	if err != nil || len(m.Dirty().Registry) != 1 {
		t.Fatalf("discard journal replay: %v", err)
	}
	if _, err := m.Apply([]Op{valid}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := m.Discard("registry", ""); err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Registry) != 0 || len(m.Dirty().Views["v_main"]) != 1 {
		t.Fatalf("bad registry discard: %+v", m.Dirty())
	}
	if err := m.Discard("all", ""); err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Registry) != 0 || len(m.Dirty().Views) != 0 {
		t.Fatalf("bad all discard: %+v", m.Dirty())
	}
}

func TestModelEditsStayUnsavedUntilSave(t *testing.T) {
	ws := editWorkspace(t)
	file := filepath.Join(ws, "projects", "p", "relations.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.AddRelation("e_a", "e_x", "call", "agent")
	if err != nil || id != "r_a_x_call" {
		t.Fatalf("%s: %v", id, err)
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("edit wrote contract before Save")
	}
	items, err := m.Records("relations.json")
	if err != nil || len(items) != 2 {
		t.Fatalf("working model not updated: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(file)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("Save did not write contract")
	}
}

func TestModelKeepsLegacyEntityTextNameWhenDescriptionChanges(t *testing.T) {
	ws := editWorkspace(t)
	file := filepath.Join(ws, "projects", "p", "text.ru.json")
	if err := os.WriteFile(file, []byte(`{"contractVersion":3,"language":"ru","entries":{"e_a":{"name":{"v":"A","origin":"authored"}}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	op := modelOp("text", "e_a", "", "ru", `{"name":{"v":"A","origin":"authored"},"description":{"v":"New","origin":"authored"}}`)
	if _, err := m.Apply([]Op{op}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"name"`)) || !bytes.Contains(b, []byte(`"New"`)) {
		t.Fatalf("lost text: %s", b)
	}
}

func TestProjectManifestIsJournaledAndSavedWithRegistry(t *testing.T) {
	ws := editWorkspace(t)
	file := filepath.Join(ws, "projects", "p", "project.json")
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	op := modelOp("project", "p", "", "", `{"id":"p","title":"Working","languages":["ru"]}`)
	if _, err := m.Apply([]Op{op}, "human"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if bytes.Contains(b, []byte("Working")) {
		t.Fatal("manifest written before Save")
	}
	m, err = LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Registry) != 1 || !bytes.Contains(m.Manifest(), []byte("Working")) {
		t.Fatal("manifest did not replay")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(file)
	if !bytes.Contains(b, []byte("Working")) {
		t.Fatal("manifest not saved")
	}
}

func TestCleanModelDirtySerializesEmptyLists(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m.Dirty())
	if string(b) != `{"registry":[],"views":{}}` {
		t.Fatalf("clean dirty summary = %s", b)
	}
}

func TestParseRefAndResolve(t *testing.T) {
	for in, want := range map[string]ObjectRef{
		"v_main":                        {View: "v_main"},
		"v_main#z_core":                 {View: "v_main", ID: "z_core"},
		"p/v_main#e_a":                  {Project: "p", View: "v_main", ID: "e_a"},
		"/app/#v_main?highlight=z_core": {View: "v_main", ID: "z_core"},
	} {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Fatalf("ParseRef(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "#z", "v#", "a/b/c#z", "v#a,b"} {
		if _, err := ParseRef(bad); err == nil {
			t.Fatalf("ParseRef(%q) accepted", bad)
		}
	}
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	for ref, kind := range map[string]string{"v_main": "view", "v_main#z_core": "zone", "v_main#e_a": "node"} {
		r, _ := ParseRef(ref)
		if k, err := m.ResolveRef(r); err != nil || k != kind {
			t.Fatalf("ResolveRef(%s) = %q, %v", ref, k, err)
		}
	}
	r, _ := ParseRef("v_main#z_nope")
	_, err = m.ResolveRef(r)
	refused(t, err, "z_nope is not on view v_main")
	r, _ = ParseRef("v_none")
	_, err = m.ResolveRef(r)
	refused(t, err, "no view v_none")
}
