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
