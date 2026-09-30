package core

import (
	"os"
	"path/filepath"
	"testing"
)

// The graph mode groups by the container an entity sits in on the views of an
// axis; a block outside any container, and a view without an axis, add nothing.
func TestModelGroupsPerAxis(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p", "views")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.view.json", `{"id":"v_main","axis":"axis_layer","placements":[
  {"entity":"e_core","parent":null,"x":0,"y":0,"width":500,"height":500},
  {"entity":"e_a","parent":"e_core","x":10,"y":20},
  {"entity":"e_b","parent":null,"x":900,"y":20}]}`)
	write("other.view.json", `{"id":"v_other","axis":"axis_process","placements":[
  {"entity":"e_core","parent":null,"x":0,"y":0,"width":9,"height":9},
  {"entity":"e_b","parent":"e_core","x":1,"y":1}]}`)
	write("noaxis.view.json", `{"id":"v_none","placements":[
  {"entity":"e_core","parent":null},{"entity":"e_x","parent":"e_core"}]}`)

	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	axes, err := m.AxisGroups()
	if err != nil {
		t.Fatal(err)
	}
	of := map[string]map[string]string{}
	for _, a := range axes {
		of[a.Axis] = a.Of
		if len(a.Containers) != 1 || a.Containers[0].ID != "e_core" || a.Containers[0].Name != "Core" {
			t.Fatalf("axis %s containers = %+v", a.Axis, a.Containers)
		}
	}
	if len(axes) != 2 {
		t.Fatalf("want the two declared axes, got %+v", axes)
	}
	if of["axis_layer"]["e_a"] != "e_core" {
		t.Fatalf("e_a should sit in e_core on axis_layer: %+v", of)
	}
	if _, placed := of["axis_layer"]["e_b"]; placed {
		t.Fatalf("a block outside any container must not be grouped: %+v", of)
	}
	if of["axis_process"]["e_b"] != "e_core" {
		t.Fatalf("e_b should sit in e_core on axis_process: %+v", of)
	}

	// Groups joins them with the containers of the graph
	g := &Graph{Nodes: []GraphNode{{ID: "x:N", Name: "N", Container: true}, {ID: "x:T", Name: "T", Containers: []string{"x:N"}}}}
	data, err := m.Groups(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Containers) != 1 || data.Containers[0].ID != "x:N" || len(data.Axes) != 2 {
		t.Fatalf("groups = %+v", data)
	}
}

// A view of the old shape is refused, not read.
func TestAxisGroupsRefuseTheOldShape(t *testing.T) {
	ws := editWorkspace(t)
	writeFile(t, filepath.Join(ws, "projects", "p", "views", "old.view.json"), `{"id":"v_old","axis":"axis_layer","zones":[],"nodes":[]}`)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AxisGroups(); err == nil {
		t.Fatal("a view with zones was read")
	}
}
