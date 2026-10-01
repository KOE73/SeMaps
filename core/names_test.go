package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The name of an authored entity is a text under its id, per language; an
// entity from code keeps its name in entities.json and is never translated
// (ADR_20260930-5).

func namedModel(t *testing.T) (*Model, string) {
	t.Helper()
	ws := editWorkspace(t)
	writeFile(t, filepath.Join(ws, "projects", "p", "project.json"), `{"id":"p","contractVersion":5,"languages":["ru","en"]}`)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	return m, ws
}

const nameText = `{"name":{"v":"%s","at":"2026-09-30T00:00:00Z","origin":"authored"}}`

func nameOf(v string) string { return strings.Replace(nameText, "%s", v, 1) }

func TestAuthoredEntityNeedsANameTextInTheSameBatch(t *testing.T) {
	m, _ := namedModel(t)
	entity := modelOp("entity", "e_new", "", "", `{"id":"e_new","kind":"group","origin":"authored"}`)
	// no name anywhere
	_, err := m.Apply([]Op{entity}, "human")
	if err == nil || !strings.Contains(err.Error(), "needs a name") {
		t.Fatalf("an authored entity without a name text: %v", err)
	}
	// a name in entities.json is the old shape
	_, err = m.Apply([]Op{modelOp("entity", "e_new", "", "", `{"id":"e_new","name":"New","kind":"group","origin":"authored"}`),
		modelOp("text", "e_new", "", "ru", nameOf("Новая"))}, "human")
	if err == nil || !strings.Contains(err.Error(), "no name in entities.json") {
		t.Fatalf("an authored entity with a name in entities.json: %v", err)
	}
	// the entity and its name in one batch, in either order
	for _, ops := range [][]Op{
		{entity, modelOp("text", "e_new", "", "en", nameOf("New"))},
		{modelOp("text", "e_new", "", "en", nameOf("New")), entity},
	} {
		m, _ := namedModel(t)
		if _, err := m.Apply(ops, "human"); err != nil {
			t.Fatalf("entity with its name text: %v", err)
		}
		// only English has it: the main language falls back to it
		if got := m.EntityName("e_new"); got != "New" {
			t.Fatalf("fallback to the other language: %q", got)
		}
	}
}

func TestNameFallbackLanguagesThenID(t *testing.T) {
	m, _ := namedModel(t)
	if got := m.EntityName("e_x"); got != "X" {
		t.Fatalf("main language: %q", got)
	}
	if err := m.SetText("en", "e_x", "name", "Ex", "human"); err != nil {
		t.Fatal(err)
	}
	if got := m.EntityName("e_x"); got != "X" {
		t.Fatalf("the main language wins over another: %q", got)
	}
	// e_core has a name only in ru; a code entity is never read from a text
	if got := m.EntityName("e_core"); got != "Core" {
		t.Fatalf("e_core: %q", got)
	}
	if got := m.EntityName("e_a"); got != "A" {
		t.Fatalf("a code entity keeps its name: %q", got)
	}
	// an authored entity with no text at all is shown by its id; loading is fine, only new ones need a name
	ws := editWorkspace(t)
	writeFile(t, filepath.Join(ws, "projects", "p", "text.ru.json"), `{"contractVersion":5,"language":"ru","entries":{}}`)
	m2, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.EntityName("e_x"); got != "e_x" {
		t.Fatalf("no name in any language: %q", got)
	}
	if got := m2.EntityName("e_nope"); got != "e_nope" {
		t.Fatalf("unknown id: %q", got)
	}
	if names := m2.EntityNames(); names["e_a"] != "A" || names["e_core"] != "e_core" {
		t.Fatalf("names: %v", names)
	}
}

// A rename of an authored entity is a text edit, unsaved until Save and shown
// among the changes as a text under the entity's id; the id stays.
func TestRenameOfAnAuthoredEntityIsATextEdit(t *testing.T) {
	m, ws := namedModel(t)
	if err := m.SetText("ru", "e_x", "name", "Икс", "human"); err != nil {
		t.Fatal(err)
	}
	if got := m.EntityName("e_x"); got != "Икс" {
		t.Fatalf("renamed: %q", got)
	}
	d := m.Dirty()
	if len(d.Registry) != 1 || d.Registry[0].Kind != "text" || d.Registry[0].ID != "e_x" || d.Registry[0].Lang != "ru" {
		t.Fatalf("dirty: %+v", d)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "text.ru.json"))
	if !strings.Contains(string(b), `"v": "Икс"`) {
		t.Fatalf("text.ru.json: %s", b)
	}
	ents, _ := os.ReadFile(filepath.Join(ws, "projects", "p", "entities.json"))
	if strings.Contains(string(ents), "Икс") || strings.Contains(string(ents), `"e_x"`) == false {
		t.Fatalf("entities.json must not carry the name: %s", ents)
	}
	// the name is not empty and not a title
	_, err := m.Apply([]Op{modelOp("text", "e_x", "", "ru", `{"name":{"v":" ","at":"2026-09-30T00:00:00Z","origin":"authored"}}`)}, "human")
	if err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Fatalf("empty name: %v", err)
	}
	_, err = m.Apply([]Op{modelOp("text", "e_x", "", "ru", `{"title":{"v":"T","at":"2026-09-30T00:00:00Z","origin":"authored"}}`)}, "human")
	if err == nil || !strings.Contains(err.Error(), "a name, not a title") {
		t.Fatalf("title of an entity: %v", err)
	}
}

func TestNameTextOfACodeEntityIsRefused(t *testing.T) {
	m, _ := namedModel(t)
	err := m.SetText("ru", "e_a", "name", "Другое", "agent")
	refused(t, err, "not translated")
	_, err = m.Apply([]Op{modelOp("text", "e_ghost", "", "ru", nameOf("Призрак"))}, "human")
	if err == nil || !strings.Contains(err.Error(), "no entity e_ghost") {
		t.Fatalf("name of nothing: %v", err)
	}
	// a description of a code entity is a text like any other
	if err := m.SetText("ru", "e_a", "description", "Держит B.", "agent"); err != nil {
		t.Fatal(err)
	}
}

func TestGetViewAndGroupsShowTheTextName(t *testing.T) {
	m, _ := namedModel(t)
	m.SetCanvas(testCanvas(t))
	v, err := m.GetView("v_main")
	if err != nil {
		t.Fatal(err)
	}
	if v.Placements[0].Entity != "e_core" || v.Placements[0].Name != "Core" {
		t.Fatalf("placements: %+v", v.Placements)
	}
	if err := m.SetText("ru", "e_core", "name", "Ядро", "human"); err != nil {
		t.Fatal(err)
	}
	if v, _ = m.GetView("v_main"); v.Placements[0].Name != "Ядро" {
		t.Fatalf("placement after rename: %+v", v.Placements[0])
	}
}
