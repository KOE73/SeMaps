package core

import (
	"reflect"
	"testing"
)

func node(id, kind, name string) GraphNode {
	return GraphNode{ID: id, Kind: kind, Name: name}
}

func containsEdge(from, to string) GraphEdge {
	return GraphEdge{From: from, To: to, Kind: "contains", Type: "contains"}
}

func TestLiftToTypesRecursionDisappears(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{node("A", "type", "A"), node("A.M1", "method", "M1"), node("A.M2", "method", "M2")},
		Edges: []GraphEdge{
			containsEdge("A", "A.M1"), containsEdge("A", "A.M2"),
			{From: "A.M1", To: "A.M2", Kind: "calls", Type: "calls"},
		},
	}
	out := LiftToTypes(g)
	if len(out.Nodes) != 1 || out.Nodes[0].ID != "A" {
		t.Fatalf("expected only the type node A, got %+v", out.Nodes)
	}
	if len(out.Edges) != 0 {
		t.Fatalf("expected recursion (A calls A) to disappear, got %+v", out.Edges)
	}
}

func TestLiftToTypesMergesManyToOne(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			node("A", "type", "A"), node("B", "type", "B"),
			node("A.M1", "method", "M1"), node("A.M2", "method", "M2"),
			node("B.N1", "method", "N1"), node("B.N2", "method", "N2"), node("B.N3", "method", "N3"),
		},
		Edges: []GraphEdge{
			containsEdge("A", "A.M1"), containsEdge("A", "A.M2"),
			containsEdge("B", "B.N1"), containsEdge("B", "B.N2"), containsEdge("B", "B.N3"),
			{From: "A.M1", To: "B.N1", Kind: "calls", Type: "calls", Line: 10},
			{From: "A.M1", To: "B.N2", Kind: "calls", Type: "calls", Line: 11},
			{From: "A.M2", To: "B.N3", Kind: "calls", Type: "calls", Line: 20},
		},
	}
	out := LiftToTypes(g)
	if len(out.Edges) != 1 {
		t.Fatalf("expected exactly one merged edge, got %+v", out.Edges)
	}
	e := out.Edges[0]
	if e.From != "A" || e.To != "B" || e.Kind != "calls" {
		t.Fatalf("unexpected merged edge: %+v", e)
	}
	if e.Count != 3 {
		t.Fatalf("expected count 3, got %d", e.Count)
	}
	if got := joinCapped(e.FromMethods, 5); got != "M1,M2" {
		t.Fatalf("expected fromMethods M1,M2, got %q", got)
	}
	if got := joinCapped(e.ToMethods, 5); got != "N1,N2,N3" {
		t.Fatalf("expected toMethods N1,N2,N3, got %q", got)
	}
}

func TestLiftToTypesThroughInterface(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			node("Caller", "type", "Caller"), node("Caller.Run", "method", "Run"),
			node("IRunner", "interface", "IRunner"), node("IRunner.Go", "method", "Go"),
		},
		Edges: []GraphEdge{
			containsEdge("Caller", "Caller.Run"), containsEdge("IRunner", "IRunner.Go"),
			{From: "Caller.Run", To: "IRunner.Go", Kind: "calls", Type: "calls"},
		},
	}
	out := LiftToTypes(g)
	if len(out.Edges) != 1 {
		t.Fatalf("expected one lifted edge, got %+v", out.Edges)
	}
	e := out.Edges[0]
	if e.From != "Caller" || e.To != "IRunner" {
		t.Fatalf("expected a call through the interface to lift to it, got %+v", e)
	}
}

func TestLiftToTypesFactoryConstructsSeveralTypes(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			node("Factory", "type", "Factory"), node("Factory.Create", "method", "Create"),
			node("Factory.List", "method", "List"),
			node("Widget", "type", "Widget"), node("Gadget", "type", "Gadget"),
		},
		Edges: []GraphEdge{
			containsEdge("Factory", "Factory.Create"), containsEdge("Factory", "Factory.List"),
			{From: "Factory.Create", To: "Widget", Kind: "constructs", Type: "constructs"},
			{From: "Factory.List", To: "Gadget", Kind: "constructs", Type: "constructs"},
		},
	}
	out := LiftToTypes(g)
	if len(out.Edges) != 2 {
		t.Fatalf("expected two constructs edges, got %+v", out.Edges)
	}
	for _, e := range out.Edges {
		if e.From != "Factory" {
			t.Fatalf("expected every constructs edge from Factory, got %+v", e)
		}
		if e.Count != 1 {
			t.Fatalf("expected an unmerged lifted edge to still carry count 1, got %+v", e)
		}
	}
}

func TestLiftToTypesLeavesExistingTypeEdgeUntouched(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{node("A", "type", "A"), node("B", "type", "B")},
		Edges: []GraphEdge{{From: "A", To: "B", Kind: "extends", Type: "extends", Relation: "r1", Presence: "both", Status: "present"}},
	}
	out := LiftToTypes(g)
	if len(out.Edges) != 1 {
		t.Fatalf("expected the one edge untouched, got %+v", out.Edges)
	}
	if !reflect.DeepEqual(out.Edges[0], g.Edges[0]) {
		t.Fatalf("expected byte-identical edge, got %+v vs %+v", out.Edges[0], g.Edges[0])
	}
}

func TestLiftToTypesDoesNotMutateInput(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{node("A", "type", "A"), node("A.M1", "method", "M1"), node("B", "type", "B")},
		Edges: []GraphEdge{containsEdge("A", "A.M1"), {From: "A.M1", To: "B", Kind: "calls", Type: "calls"}},
	}
	before := len(g.Nodes)
	_ = LiftToTypes(g)
	if len(g.Nodes) != before || g.Nodes[1].Kind != "method" {
		t.Fatalf("input graph was mutated: %+v", g.Nodes)
	}
}

// TestLiftToTypesGathersDynamicMarks: a type's methods' blind-spot marks
// (ADR_20260928-5 §4) are gathered onto the type, each carrying the method's
// short name, sorted by (line, kind, method), and no mark is dropped
// (PLAN_20260928-7 step 6).
func TestLiftToTypesGathersDynamicMarks(t *testing.T) {
	m1 := node("Factory.Create", "method", "Create")
	m1.Dynamic = []GraphDynamicMark{{Kind: "create", Line: 57}}
	m2 := node("Factory.List", "method", "List")
	m2.Dynamic = []GraphDynamicMark{{Kind: "make-type", Line: 55}, {Kind: "dynamic", Line: 55}}
	g := &Graph{
		Nodes: []GraphNode{node("Factory", "type", "Factory"), m1, m2},
		Edges: []GraphEdge{containsEdge("Factory", "Factory.Create"), containsEdge("Factory", "Factory.List")},
	}
	out := LiftToTypes(g)
	if len(out.Nodes) != 1 {
		t.Fatalf("expected only the type node, got %+v", out.Nodes)
	}
	got := out.Nodes[0].Dynamic
	want := []GraphDynamicMark{
		{Kind: "dynamic", Line: 55, Method: "List"},
		{Kind: "make-type", Line: 55, Method: "List"},
		{Kind: "create", Line: 57, Method: "Create"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected marks gathered, named and sorted by (line, kind, method), got %+v, want %+v", got, want)
	}
}

// TestLiftToTypesDynamicMarksPartialTypeCarriesFile: a method's marks are on
// the type's own file only when the method is; a partial type's method
// declared elsewhere carries `file` (base name only) on its marks.
func TestLiftToTypesDynamicMarksPartialTypeCarriesFile(t *testing.T) {
	sameFile := node("Factory.Create", "method", "Create")
	sameFile.File = "src/Factory.cs"
	sameFile.Dynamic = []GraphDynamicMark{{Kind: "create", Line: 10}}
	otherFile := node("Factory.List", "method", "List")
	otherFile.File = "src/Factory.More.cs"
	otherFile.Dynamic = []GraphDynamicMark{{Kind: "invoke", Line: 20}}
	typ := node("Factory", "type", "Factory")
	typ.File = "src/Factory.cs"
	g := &Graph{
		Nodes: []GraphNode{typ, sameFile, otherFile},
		Edges: []GraphEdge{containsEdge("Factory", "Factory.Create"), containsEdge("Factory", "Factory.List")},
	}
	out := LiftToTypes(g)
	want := []GraphDynamicMark{
		{Kind: "create", Line: 10, Method: "Create"},
		{Kind: "invoke", Line: 20, Method: "List", File: "Factory.More.cs"},
	}
	if !reflect.DeepEqual(out.Nodes[0].Dynamic, want) {
		t.Fatalf("expected the partial type's method to carry its own file's base name, got %+v, want %+v", out.Nodes[0].Dynamic, want)
	}
}

func TestLiftToTypesDeterministic(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			node("A", "type", "A"), node("B", "type", "B"),
			node("A.M1", "method", "M1"), node("A.M2", "method", "M2"), node("B.N1", "method", "N1"),
		},
		Edges: []GraphEdge{
			containsEdge("A", "A.M1"), containsEdge("A", "A.M2"), containsEdge("B", "B.N1"),
			{From: "A.M1", To: "B.N1", Kind: "calls", Type: "calls"},
			{From: "A.M2", To: "B.N1", Kind: "calls", Type: "calls"},
		},
	}
	first := LiftToTypes(g)
	for i := 0; i < 5; i++ {
		again := LiftToTypes(g)
		if len(again.Edges) != len(first.Edges) || !reflect.DeepEqual(again.Edges[0], first.Edges[0]) {
			t.Fatalf("non-deterministic output across runs")
		}
	}
}
