package core

import "testing"

func TestDiffGraphsAddedRemovedChanged(t *testing.T) {
	old := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
		},
		Edges: []GraphEdge{{From: "a", To: "b", Kind: "uses"}},
	}
	new := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Name: "A renamed"}, // changed
			{ID: "c", Name: "C"},         // added; b removed
		},
		Edges: []GraphEdge{{From: "a", To: "c", Kind: "uses"}}, // a->b removed, a->c added
	}
	d := DiffGraphs(old, new)
	if len(d.AddedNodes) != 1 || d.AddedNodes[0] != "c" {
		t.Fatalf("added nodes: %+v", d.AddedNodes)
	}
	if len(d.RemovedNodes) != 1 || d.RemovedNodes[0] != "b" {
		t.Fatalf("removed nodes: %+v", d.RemovedNodes)
	}
	if len(d.ChangedNodes) != 1 || d.ChangedNodes[0] != "a" {
		t.Fatalf("changed nodes: %+v", d.ChangedNodes)
	}
	if len(d.AddedEdges) != 1 || d.AddedEdges[0] != (GraphEdgeKey{From: "a", To: "c", Kind: "uses"}) {
		t.Fatalf("added edges: %+v", d.AddedEdges)
	}
	if len(d.RemovedEdges) != 1 || d.RemovedEdges[0] != (GraphEdgeKey{From: "a", To: "b", Kind: "uses"}) {
		t.Fatalf("removed edges: %+v", d.RemovedEdges)
	}
	if d.Empty() {
		t.Fatal("non-empty diff reported Empty()")
	}
}

func TestDiffGraphsNoChangeIsEmpty(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "a", Name: "A"}},
		Edges: []GraphEdge{{From: "a", To: "a", Kind: "uses"}},
	}
	d := DiffGraphs(g, g)
	if !d.Empty() {
		t.Fatalf("identical graphs produced a diff: %+v", d)
	}
}

func TestDiffGraphsNilOldIsAllAdded(t *testing.T) {
	new := &Graph{Nodes: []GraphNode{{ID: "a"}}, Edges: []GraphEdge{}}
	d := DiffGraphs(nil, new)
	if len(d.AddedNodes) != 1 || d.AddedNodes[0] != "a" {
		t.Fatalf("expected the only node as added: %+v", d)
	}
	if len(d.RemovedNodes) != 0 || len(d.ChangedNodes) != 0 {
		t.Fatalf("nil old must not produce removed/changed: %+v", d)
	}
}
