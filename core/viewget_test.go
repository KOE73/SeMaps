package core

import "testing"

func TestGetViewTreeAndEdgeVisibility(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	cv := testCanvas(t)
	m.SetCanvas(cv)
	ops := []Op{
		modelOp("entity", "e_in", "", "", `{"id":"e_in","kind":"group","origin":"authored"}`),
		modelOp("text", "e_in", "", "ru", `{"name":{"v":"Inner","at":"2026-09-30T00:00:00Z","origin":"authored"}}`),
		modelOp("placement", "e_in", "v_main", "", `{"entity":"e_in","parent":"e_core","x":20,"y":40,"width":300,"height":200,"override":{"fill":"#ffffff","border":{"color":"#000000"}}}`),
		modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":"e_in","x":40,"y":80}`),
	}
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetView("v_main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Placements) != 2 || got.Placements[0].Entity != "e_core" || got.Placements[0].Name != "Core" || !got.Placements[0].Container {
		t.Fatalf("top placements = %+v", got.Placements)
	}
	inner := got.Placements[0].Children
	if len(inner) != 1 || inner[0].Entity != "e_in" || !inner[0].Container || len(inner[0].Children) != 1 || inner[0].Children[0].Entity != "e_b" {
		t.Fatalf("nesting = %+v", inner)
	}
	if string(inner[0].Override) != `{"fill":"#ffffff","border":{"color":"#000000"}}` {
		t.Fatalf("override = %s", inner[0].Override)
	}
	if n := inner[0].Children[0]; n.Width != cv.Node.Width || n.Height != cv.Node.Height || n.Name != "B" || n.Kind != "class" || n.Container {
		t.Fatalf("block = %+v", n)
	}
	loose := got.Placements[1]
	if loose.Entity != "e_a" || loose.Parent != nil || loose.Container {
		t.Fatalf("loose block = %+v", loose)
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
	if got, _ = m.GetView("v_main"); len(got.Edges) != 0 {
		t.Fatalf("except ignored: %+v", got.Edges)
	}
	// a container reference limits the answer to its subtree
	sub, err := m.GetView("v_main#e_in")
	if err != nil || len(sub.Placements) != 1 || sub.Placements[0].Entity != "e_in" || sub.View.Scope != "v_main#e_in" {
		t.Fatalf("subtree = %+v, %v", sub, err)
	}
	if _, err := m.GetView("v_main#e_a"); err == nil {
		t.Fatal("a block is not a scope")
	}
	// the view's own edges list replaces the registry and may carry an override
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_x","from":"e_a","to":"e_b","type":"call","override":{"line":{"color":"#ff0000","dash":"4,4"}}}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if got, _ = m.GetView("v_main"); len(got.Edges) != 1 || got.Edges[0].ID != "r_x" || string(got.Edges[0].Override) != `{"line":{"color":"#ff0000","dash":"4,4"}}` {
		t.Fatalf("own edges = %+v", got.Edges)
	}
	// an edge override outside the table is refused
	_, err = m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_x","from":"e_a","to":"e_b","type":"call","override":{"fill":"#ff0000"}}]}`)}, "human")
	refused(t, err, `"fill" cannot be overridden on an edge`)
}
