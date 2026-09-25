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
