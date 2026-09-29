package core

import (
	"os"
	"path/filepath"
	"testing"
)

// The graph mode groups by the zone an entity sits in on the views of an axis;
// a block outside any zone, and a view without an axis, add nothing.
func TestModelGroupsPerAxis(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p", "views")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.view.json", `{"id":"v_main","axis":"axis_layer",
  "zones":[{"id":"z_core","container":"c_core","x":0,"y":0,"width":500,"height":500}],
  "nodes":[{"entity":"e_a","zone":"z_core","x":10,"y":20},{"entity":"e_b","zone":null,"x":900,"y":20}]}`)
	write("other.view.json", `{"id":"v_other","axis":"axis_process",
  "zones":[{"id":"z_proc","container":null,"x":0,"y":0,"width":9,"height":9}],
  "nodes":[{"entity":"e_b","zone":"z_proc"}]}`)
	write("noaxis.view.json", `{"id":"v_none","zones":[{"id":"z_lost"}],"nodes":[{"entity":"e_x","zone":"z_lost"}]}`)

	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatal(err)
	}
	data, err := m.Groups()
	if err != nil {
		t.Fatal(err)
	}
	of := map[string]map[string]string{}
	for _, a := range data.Axes {
		of[a.Axis] = a.Of
	}
	if len(data.Axes) != 2 {
		t.Fatalf("want the two declared axes, got %+v", data.Axes)
	}
	if of["axis_layer"]["e_a"] != "z_core" {
		t.Fatalf("e_a should sit in z_core on axis_layer: %+v", of)
	}
	if _, placed := of["axis_layer"]["e_b"]; placed {
		t.Fatalf("a block outside any zone must not be grouped: %+v", of)
	}
	if of["axis_process"]["e_b"] != "z_proc" {
		t.Fatalf("e_b should sit in z_proc on axis_process: %+v", of)
	}
}
