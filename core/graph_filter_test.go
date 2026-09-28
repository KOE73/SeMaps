package core

import "testing"

func filterFixtureGraph() *Graph {
	return &Graph{
		Nodes: []GraphNode{
			{ID: "csharp:A", Kind: "type", Containers: []string{"c_app"}},
			{ID: "csharp:A.Run", Kind: "function", Containers: []string{"c_app"}},
			{ID: "csharp:B", Kind: "type", Containers: []string{"c_lib"}, File: "src/B.cs", Line: 3, Entity: "e_b"},
		},
		Edges: []GraphEdge{
			{From: "csharp:A", To: "csharp:A.Run", Kind: "holds", Via: &Via{Member: "Run"}},
			{From: "csharp:A", To: "csharp:B", Kind: "uses"},
		},
	}
}

func TestFilterLevelTypesDropsFunctionsAndValues(t *testing.T) {
	g := filterFixtureGraph()
	out, err := FilterLevel(g, "types")
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(out.Nodes), out.Nodes)
	}
	for _, n := range out.Nodes {
		if n.Kind == "function" || n.Kind == "value" {
			t.Fatalf("function/value node survived: %+v", n)
		}
	}
	if len(out.Edges) != 1 || out.Edges[0].Kind != "uses" {
		t.Fatalf("expected only the edge not touching the dropped node, got %+v", out.Edges)
	}
}

func TestFilterLevelAllIsNoOp(t *testing.T) {
	g := filterFixtureGraph()
	out, err := FilterLevel(g, "all")
	if err != nil || len(out.Nodes) != len(g.Nodes) || len(out.Edges) != len(g.Edges) {
		t.Fatalf("level=all changed the graph: %v %+v", err, out)
	}
	if out, err := FilterLevel(g, ""); err != nil || len(out.Nodes) != len(g.Nodes) {
		t.Fatalf("level='' changed the graph: %v %+v", err, out)
	}
}

func TestFilterLevelBadValue(t *testing.T) {
	if _, err := FilterLevel(filterFixtureGraph(), "bogus"); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

func TestFilterEdgeKinds(t *testing.T) {
	g := filterFixtureGraph()
	out := FilterEdgeKinds(g, []string{"uses"})
	if len(out.Edges) != 1 || out.Edges[0].Kind != "uses" {
		t.Fatalf("expected only uses edges, got %+v", out.Edges)
	}
	if len(out.Nodes) != len(g.Nodes) {
		t.Fatalf("FilterEdgeKinds must not touch nodes")
	}
	if same := FilterEdgeKinds(g, nil); len(same.Edges) != len(g.Edges) {
		t.Fatalf("empty kinds must keep every edge")
	}
}

func TestNeighborhoodDepth(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "uses"},
			{From: "b", To: "c", Kind: "uses"},
			{From: "d", To: "a", Kind: "uses"}, // reverse direction from a
		},
	}
	out, err := Neighborhood(g, "a", 1)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range out.Nodes {
		ids[n.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["d"] || ids["c"] {
		t.Fatalf("depth 1 neighbourhood wrong: %+v", out.Nodes)
	}
	out2, err := Neighborhood(g, "a", 2)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	if len(out2.Nodes) != 4 {
		t.Fatalf("depth 2 should reach every node, got %+v", out2.Nodes)
	}
}

func TestNeighborhoodUnknownNode(t *testing.T) {
	if _, err := Neighborhood(filterFixtureGraph(), "nope", 1); err == nil {
		t.Fatal("expected an error for an unknown node id")
	}
}

func TestFilterContainer(t *testing.T) {
	g := filterFixtureGraph()
	known := map[string]bool{"c_app": true, "c_lib": true}
	out, err := FilterContainer(g, "c_app", known)
	if err != nil {
		t.Fatalf("FilterContainer: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("expected the 2 c_app nodes, got %+v", out.Nodes)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("expected the edge between the two kept nodes, got %+v", out.Edges)
	}
}

func TestFilterContainerUnknown(t *testing.T) {
	known := map[string]bool{"c_app": true}
	if _, err := FilterContainer(filterFixtureGraph(), "c_typo", known); err == nil {
		t.Fatal("expected an error for an unknown container id")
	}
}

func TestStripFieldsDefaults(t *testing.T) {
	g := filterFixtureGraph()
	out := StripFields(g, map[string]bool{"via": true, "position": true})
	if out.Nodes[2].File == "" || out.Edges[0].Via == nil {
		t.Fatalf("via+position asked for should be kept: %+v", out)
	}
	stripped := StripFields(g, map[string]bool{})
	if stripped.Nodes[2].File != "" || stripped.Nodes[2].Line != 0 {
		t.Fatalf("position not asked for should be stripped: %+v", stripped.Nodes[2])
	}
	if stripped.Edges[0].Via != nil {
		t.Fatalf("via not asked for should be stripped: %+v", stripped.Edges[0])
	}
	// the input graph itself must not be mutated
	if g.Nodes[2].File == "" || g.Edges[0].Via == nil {
		t.Fatalf("StripFields must not mutate its input: %+v", g)
	}
}
