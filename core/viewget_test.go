package core

import "testing"

func TestGetViewTreeAndEdgeVisibility(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	ops := []Op{
		modelOp("zone", "z_in", "v_main", "", `{"id":"z_in","container":null,"parent":"z_core","x":20,"y":40,"width":300,"height":200}`),
		modelOp("node", "e_b", "v_main", "", `{"entity":"e_b","zone":"z_in","x":40,"y":80}`),
		modelOp("text", "z_core", "", "ru", `{"name":{"v":"Ядро","at":"2026-09-25T00:00:00Z","origin":"authored"}}`),
	}
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetView("v_main", "ru")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Zones) != 1 || got.Zones[0].ID != "z_core" || got.Zones[0].Name != "Ядро" {
		t.Fatalf("top zones = %+v", got.Zones)
	}
	inner := got.Zones[0].Zones
	if len(inner) != 1 || inner[0].ID != "z_in" || len(inner[0].Nodes) != 1 || inner[0].Nodes[0].Entity != "e_b" {
		t.Fatalf("nesting = %+v", inner)
	}
	if n := inner[0].Nodes[0]; n.Width != NodeWidth || n.Height != NodeHeight || n.Name != "B" || n.Kind != "class" {
		t.Fatalf("node = %+v", n)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Entity != "e_a" || got.Nodes[0].Zone != "" {
		t.Fatalf("loose nodes = %+v", got.Nodes)
	}
	if len(got.Unsaved) == 0 {
		t.Fatalf("unsaved = %+v", got.Unsaved)
	}
	// both ends on the view, the type says visible
	if len(got.Edges) != 1 || got.Edges[0].ID != "r_a_b_items_item" {
		t.Fatalf("edges = %+v", got.Edges)
	}
	// except flips a visible one
	if err := m.SetRelationVisible("v_main", "r_a_b_items_item", false, "human"); err != nil {
		t.Fatal(err)
	}
	if got, _ = m.GetView("v_main", "ru"); len(got.Edges) != 0 {
		t.Fatalf("except ignored: %+v", got.Edges)
	}
	// a zone reference limits the answer to its subtree
	sub, err := m.GetView("v_main#z_in", "ru")
	if err != nil || len(sub.Zones) != 1 || sub.Zones[0].ID != "z_in" || len(sub.Nodes) != 0 || sub.View.Scope != "v_main#z_in" {
		t.Fatalf("subtree = %+v, %v", sub, err)
	}
	if _, err := m.GetView("v_main#e_a", "ru"); err == nil {
		t.Fatal("a node is not a scope")
	}
	// the view's own edges list replaces the registry
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_x","from":"e_a","to":"e_b","type":"call"}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if got, _ = m.GetView("v_main", "ru"); len(got.Edges) != 1 || got.Edges[0].ID != "r_x" {
		t.Fatalf("own edges = %+v", got.Edges)
	}
}
