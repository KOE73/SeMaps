package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testCanvas is the tool's shipped canvas.json — the one definition of the numbers.
func testCanvas(t *testing.T) Canvas {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "host", "defaults", CanvasFile))
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCanvas(data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadCanvasWorkspaceWins(t *testing.T) {
	def, err := os.ReadFile(filepath.Join("..", "host", "defaults", CanvasFile))
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	c, err := LoadCanvas(ws, def)
	if err != nil || c.Grid != 10 || c.Zone.HeaderHeight != 28 || c.Gap.Zone != 40 {
		t.Fatalf("default: %+v %v", c, err)
	}
	over := strings.Replace(string(def), `"grid":10`, `"grid":20`, 1)
	if err := os.WriteFile(filepath.Join(ws, CanvasFile), []byte(over), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err = LoadCanvas(ws, def); err != nil || c.Grid != 20 {
		t.Fatalf("override: %+v %v", c, err)
	}
	if err := os.WriteFile(filepath.Join(ws, CanvasFile), []byte(`{"grid":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadCanvas(ws, def); err == nil {
		t.Fatal("a canvas without sizes was accepted")
	}
}

func TestCanvasDescribe(t *testing.T) {
	text := testCanvas(t).Describe()
	for _, want := range []string{"1 unit = 1 screen pixel", "y grows down", "Grid step 10", "does NOT", "180x60", "100x40", "160x100", "28 high", "padding 16", "40 between nodes", "40 between zones"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
}

func TestModelWithoutCanvasRefusesGeometry(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetView("v_main", "ru"); err == nil || !strings.Contains(err.Error(), "no canvas") {
		t.Fatalf("get: %v", err)
	}
}

func TestCreateProjectAndView(t *testing.T) {
	ws := t.TempDir()
	if err := CreateProject(ws, "Bad-ID", "", ""); err == nil {
		t.Fatal("bad id accepted")
	}
	if err := CreateProject(ws, "demo", "Демо", "en"); err != nil {
		t.Fatal(err)
	}
	if err := CreateProject(ws, "demo", "", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second create: %v", err)
	}
	m, err := LoadModel(ws, "demo")
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	if got := m.Languages(); len(got) != 1 || got[0] != "en" {
		t.Fatalf("languages = %v", got)
	}
	if err := m.CreateView("v_main", "axis_layer", map[string]string{"en": "Main"}, false, "agent"); err == nil {
		t.Fatal("created without a human")
	}
	if err := m.CreateView("v_main", "", nil, true, "agent"); err == nil || !strings.Contains(err.Error(), "axis") {
		t.Fatalf("no axis and no defaultAxis: %v", err)
	}
	if err := m.CreateView("main", "axis_layer", nil, true, "agent"); err == nil {
		t.Fatal("bad view id accepted")
	}
	if err := m.CreateView("v_main", "axis_layer", map[string]string{"en": "Main"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateView("v_main", "axis_layer", nil, true, "agent"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("twice: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "projects", "demo", "views", "v_main.view.json")); err == nil {
		t.Fatal("the view file exists before Save")
	}
	info, err := m.GetView("v_main", "en")
	if err != nil || info.View.Axis != "axis_layer" {
		t.Fatalf("get_view of an unsaved view: %+v %v", info, err)
	}
	// it survives a reload from the work journal, and is written on Save
	again, err := LoadModel(ws, "demo")
	if err != nil {
		t.Fatal(err)
	}
	again.SetCanvas(testCanvas(t))
	if _, err := again.GetView("v_main", "en"); err != nil {
		t.Fatalf("journal replay: %v", err)
	}
	if err := again.Save(); err != nil {
		t.Fatal(err)
	}
	idx := Index(ws).Projects[0]
	if len(idx.Views) != 1 || idx.Views[0].ID != "v_main" || idx.Views[0].Names["en"] != "Main" {
		t.Fatalf("index after save: %+v", idx.Views)
	}
}
