package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// geomModel: e_core (container, top) > e_in (container) > e_a (block); e_b is a
// block outside any container; the relation e_a -> e_b is visible.

func TestGetViewTreeDetail(t *testing.T) {
	m := geomModel(t)
	got, err := m.GetViewWith("v_main", ViewOptions{Detail: DetailTree})
	if err != nil {
		t.Fatal(err)
	}
	if got.View.Detail != DetailTree || got.Edges != nil || got.Lines != 1 || got.Scoped != 4 || got.FellBack {
		t.Fatalf("head = %+v lines %d scoped %d edges %v", got.View, got.Lines, got.Scoped, got.Edges)
	}
	if len(got.Placements) != 2 {
		t.Fatalf("top = %+v", got.Placements)
	}
	core, loose := got.Placements[0], got.Placements[1]
	if core.Entity != "e_core" || core.Blocks == nil || *core.Blocks != 0 || len(core.Children) != 1 {
		t.Fatalf("e_core = %+v", core)
	}
	in := core.Children[0]
	if in.Entity != "e_in" || in.Blocks == nil || *in.Blocks != 1 || len(in.Children) != 0 || in.Width != 200 {
		t.Fatalf("e_in = %+v", in)
	}
	// a block outside every container is given in full, with no count
	if loose.Entity != "e_b" || loose.Blocks != nil || loose.Container {
		t.Fatalf("loose block = %+v", loose)
	}
	raw, _ := json.Marshal(got)
	text := string(raw)
	if !strings.Contains(text, `"lines":1`) || strings.Contains(text, `"edges"`) || !strings.Contains(text, `"blocks":1`) || !strings.Contains(text, `"detail":"tree"`) {
		t.Fatalf("tree json: %s", text)
	}
	if strings.Contains(text, `"entity":"e_a"`) {
		t.Fatalf("a block inside a container is listed in the tree: %s", text)
	}
	// full: everything, the list of lines and no count
	full, err := m.GetView("v_main")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(full)
	text = string(raw)
	if full.View.Detail != DetailFull || !strings.Contains(text, `"edges":[`) || strings.Contains(text, `"lines"`) || !strings.Contains(text, `"entity":"e_a"`) || strings.Contains(text, `"blocks"`) {
		t.Fatalf("full json: %s", text)
	}
	// a container reference works with both details
	sub, err := m.GetViewWith("v_main#e_in", ViewOptions{Detail: DetailTree})
	if err != nil || len(sub.Placements) != 1 || sub.Placements[0].Entity != "e_in" || *sub.Placements[0].Blocks != 1 || sub.Scoped != 2 || sub.Lines != 1 {
		t.Fatalf("subtree as tree = %+v, %v", sub, err)
	}
	sub, err = m.GetViewWith("v_main#e_in", ViewOptions{Detail: DetailFull})
	if err != nil || len(sub.Placements[0].Children) != 1 || sub.Placements[0].Blocks != nil || len(sub.Edges) != 1 {
		t.Fatalf("subtree in full = %+v, %v", sub, err)
	}
	if _, err := m.GetViewWith("v_main", ViewOptions{Detail: "deep"}); err == nil {
		t.Fatal("an unknown detail was accepted")
	}
}

func TestGetViewDefaultDetailFollowsTheSize(t *testing.T) {
	m := geomModel(t)
	for _, c := range []struct {
		ref     string
		max     int
		want    string
		fellBak bool
	}{
		{"v_main", 4, DetailFull, false}, // 4 placements: at most 4 is full
		{"v_main", 3, DetailTree, true},
		{"v_main", 0, DetailFull, false}, // no limit given: always full
		{"v_main#e_in", 2, DetailFull, false},
		{"v_main#e_in", 1, DetailTree, true},
	} {
		got, err := m.GetViewWith(c.ref, ViewOptions{FullMax: c.max})
		if err != nil {
			t.Fatal(err)
		}
		if got.View.Detail != c.want || got.FellBack != c.fellBak {
			t.Errorf("%s max %d: detail %s fellBack %v, want %s %v (scoped %d)", c.ref, c.max, got.View.Detail, got.FellBack, c.want, c.fellBak, got.Scoped)
		}
	}
	// a detail that was asked for is never a fall back
	got, _ := m.GetViewWith("v_main", ViewOptions{Detail: DetailFull, FullMax: 1})
	if got.View.Detail != DetailFull || got.FellBack {
		t.Fatalf("asked full: %+v", got.View)
	}
}

func TestGetViewHiddenEdges(t *testing.T) {
	m := geomModel(t)
	got, _ := m.GetViewWith("v_main", ViewOptions{Hidden: true})
	raw, _ := json.Marshal(got)
	if got.HiddenEdges == nil || len(got.HiddenEdges) != 0 || !strings.Contains(string(raw), `"hiddenEdges":[]`) {
		t.Fatalf("nothing is hidden yet: %+v %s", got.HiddenEdges, raw)
	}
	if _, err := m.SetRelationsVisible("v_main", []string{"r_a_b_items_item"}, false, "human"); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetViewWith("v_main", ViewOptions{Hidden: true})
	if len(got.Edges) != 0 || len(got.HiddenEdges) != 1 || got.HiddenEdges[0].ID != "r_a_b_items_item" || got.HiddenEdges[0].From != "e_a" || got.HiddenEdges[0].Type == "" {
		t.Fatalf("edges %+v hidden %+v", got.Edges, got.HiddenEdges)
	}
	// not asked for: no list at all
	got, _ = m.GetViewWith("v_main", ViewOptions{})
	raw, _ = json.Marshal(got)
	if got.HiddenEdges != nil || strings.Contains(string(raw), "hiddenEdges") {
		t.Fatalf("hiddenEdges without hidden: %s", raw)
	}
	// with the tree the hidden ones are still listed, the visible only counted
	got, _ = m.GetViewWith("v_main", ViewOptions{Detail: DetailTree, Hidden: true})
	if got.Lines != 0 || len(got.HiddenEdges) != 1 {
		t.Fatalf("tree + hidden: lines %d hidden %+v", got.Lines, got.HiddenEdges)
	}
	// a relation the type hides by default is hidden too, and comes back with the view's own look
	if _, err := m.Apply([]Op{modelOp("relation", "r_b_a_secret", "", "", `{"id":"r_b_a_secret","from":"e_b","to":"e_a","type":"secret","origin":"authored"}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetViewWith("v_main#e_in", ViewOptions{Hidden: true})
	if len(got.HiddenEdges) != 2 {
		t.Fatalf("hidden in the subtree (a line touching it counts): %+v", got.HiddenEdges)
	}
}
