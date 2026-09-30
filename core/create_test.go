package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateProjectAndView(t *testing.T) {
	ws := t.TempDir()
	if err := CreateProject(ws, NewProject{ID: "Bad-Id", Title: "x"}); err == nil {
		t.Fatal("id outside the pattern accepted")
	}
	if err := CreateProject(ws, NewProject{ID: "ov", Title: " "}); err == nil {
		t.Fatal("empty title accepted")
	}
	if err := CreateProject(ws, NewProject{ID: "ov", Title: "Overview", Language: "e/n"}); err == nil {
		t.Fatal("language with a path separator accepted")
	}
	if err := CreateProject(ws, NewProject{ID: "ov", Title: "Overview", Language: "en", DefaultAxis: "axis_subsystem", Icon: "map"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateProject(ws, NewProject{ID: "ov", Title: "Again"}); !errors.Is(err, ErrExists) {
		t.Fatalf("second create: %v", err)
	}
	m, err := LoadModel(ws, "ov", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Languages(); len(got) != 1 || got[0] != "en" {
		t.Fatalf("languages = %v", got)
	}
	if err := CreateView(ws, "ov", NewView{ID: "main"}); err == nil {
		t.Fatal("view id without v_ accepted")
	}
	if err := CreateView(ws, "nope", NewView{ID: "v_all"}); err == nil {
		t.Fatal("view in a missing project accepted")
	}
	if err := CreateProject(ws, NewProject{ID: "noaxis", Title: "No axis"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateView(ws, "noaxis", NewView{ID: "v_x"}); err == nil || !strings.Contains(err.Error(), "axis") {
		t.Fatalf("no axis and no defaultAxis: %v", err)
	}
	if err := CreateView(ws, "noaxis", NewView{ID: "v_x", Axis: "axis_layer"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateView(ws, "ov", NewView{ID: "v_all", SetDefault: true}); err != nil {
		t.Fatal(err)
	}
	if err := CreateView(ws, "ov", NewView{ID: "v_all"}); !errors.Is(err, ErrExists) {
		t.Fatalf("second view: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(ws, "projects", "ov", "project.json"))
	manifest := string(b)
	// defaultView goes last; the keys that were there keep their order.
	if i, j, k := strings.Index(manifest, `"id"`), strings.Index(manifest, `"icon"`), strings.Index(manifest, `"defaultView": "v_all"`); !(i < j && j < k) {
		t.Fatalf("project.json = %s", manifest)
	}
	view, _ := os.ReadFile(filepath.Join(ws, "projects", "ov", "views", "v_all.view.json"))
	if strings.Contains(string(view), `"edges"`) || !strings.Contains(string(view), `"placements": []`) {
		t.Fatalf("view file = %s", view)
	}
}

func TestLiveIndexShowsUnsavedNamesAndGetViewInheritsAxis(t *testing.T) {
	ws := t.TempDir()
	if err := CreateProject(ws, NewProject{ID: "ov", Title: "Overview", DefaultAxis: "axis_subsystem"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateView(ws, "ov", NewView{ID: "v_all"}); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(ws, "ov", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	if err := m.SetText("ru", "v_all", "name", "Всё", "agent"); err != nil {
		t.Fatal(err)
	}
	if got := Index(ws).Projects[0].Views[0].Names["ru"]; got != "" {
		t.Fatalf("disk index already has %q before save", got)
	}
	live := LiveIndex(ws, func(string) (*Model, error) { return m, nil })
	if got := live.Projects[0].Views[0].Names["ru"]; got != "Всё" {
		t.Fatalf("live name = %q", got)
	}
	v, err := m.GetView("v_all")
	if err != nil {
		t.Fatal(err)
	}
	if v.View.Axis != "axis_subsystem" || !v.View.AxisInherited {
		t.Fatalf("view head = %+v", v.View)
	}
}

func TestAddEntityMintsRefusesAndChecks(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.AddEntity("", "Billing DB", "database", "", "agent")
	if err != nil || id != "e_billing_db" {
		t.Fatalf("minted id = %q, %v", id, err)
	}
	if again, err := m.AddEntity("", "Billing DB", "database", "", "agent"); err != nil || again == id {
		t.Fatalf("second mint = %q, %v", again, err)
	}
	if _, err := m.AddEntity("e_billing_db", "Other", "app", "", "agent"); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("existing id: %v", err)
	}
	if _, err := m.AddEntity("node_1", "N", "app", "", "agent"); err == nil || !strings.Contains(err.Error(), "starts with e_") {
		t.Fatalf("id without e_: %v", err)
	}
	if _, err := m.AddEntity("", "N", " ", "", "agent"); err == nil || !strings.Contains(err.Error(), "kind is empty") {
		t.Fatalf("empty kind: %v", err)
	}
	if _, err := m.AddEntity("", " ", "app", "", "agent"); err == nil || !strings.Contains(err.Error(), "name is empty") {
		t.Fatalf("empty name: %v", err)
	}
	e := m.record("entity", id)
	if e == nil || e.str("origin") != "authored" || e.str("status") != "present" {
		t.Fatalf("entity = %v", e)
	}
	// an authored entity has no name in entities.json: its name is a text of the main language (ADR_20260930-5)
	if has(e, "name") {
		t.Fatalf("an authored entity carries a name in entities.json: %v", e)
	}
	if got := m.EntityName(id); got != "Billing DB" {
		t.Fatalf("name = %q", got)
	}
	if raw, _ := m.Text("ru", id); !strings.Contains(string(raw), `"Billing DB"`) {
		t.Fatalf("name text = %s", raw)
	}
	// the name can change, the id stays
	if err := m.SetText("ru", id, "name", "Billing", "agent"); err != nil {
		t.Fatal(err)
	}
	if got := m.EntityName(id); got != "Billing" {
		t.Fatalf("renamed = %q", got)
	}
}
