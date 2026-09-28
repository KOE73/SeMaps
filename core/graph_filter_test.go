package core

import (
	"fmt"
	"testing"
	"time"
)

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

func TestFilterLevelTypesDropsFunctionsValuesAndMethods(t *testing.T) {
	g := filterFixtureGraph()
	out, err := FilterLevel(g, "types")
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(out.Nodes), out.Nodes)
	}
	for _, n := range out.Nodes {
		if n.Kind == "function" || n.Kind == "value" || n.Kind == "method" {
			t.Fatalf("function/value/method node survived: %+v", n)
		}
	}
	if len(out.Edges) != 1 || out.Edges[0].Kind != "uses" {
		t.Fatalf("expected only the edge not touching the dropped node, got %+v", out.Edges)
	}

	g2 := &Graph{
		Nodes: []GraphNode{{ID: "t", Kind: "type"}, {ID: "m", Kind: "method"}},
		Edges: []GraphEdge{{From: "t", To: "m", Kind: "contains"}},
	}
	out2, err := FilterLevel(g2, "types")
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if len(out2.Nodes) != 1 || out2.Nodes[0].ID != "t" || len(out2.Edges) != 0 {
		t.Fatalf("expected the method node and its edge dropped, got %+v", out2)
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

func mustFollow(t *testing.T, names ...string) []Relation {
	t.Helper()
	rs, err := ParseFollow(names)
	if err != nil {
		t.Fatalf("ParseFollow: %v", err)
	}
	return rs
}

func TestWalkDepth(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "uses", Type: "uses"},
			{From: "b", To: "c", Kind: "uses", Type: "uses"},
			{From: "d", To: "a", Kind: "uses", Type: "uses"}, // reverse direction from a
		},
	}
	follow := mustFollow(t, "uses", "used-by")
	out, _, err := Walk(g, "a", 1, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range out.Nodes {
		ids[n.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["d"] || ids["c"] {
		t.Fatalf("depth 1 neighbourhood wrong: %+v", out.Nodes)
	}
	out2, _, err := Walk(g, "a", 2, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(out2.Nodes) != 4 {
		t.Fatalf("depth 2 should reach every node, got %+v", out2.Nodes)
	}
}

func TestWalkUnknownNode(t *testing.T) {
	if _, _, err := Walk(filterFixtureGraph(), "nope", 1, mustFollow(t, "uses"), 0); err == nil {
		t.Fatal("expected an error for an unknown node id")
	}
}

// TestWalkDirectionNeverClimbsToInterface: the exact case of the experiment
// (PLAN_20260928-5 step 2, question 2) — `follow=extended-by` at depth 2
// must give descendants only, and must never reach an interface's other
// implementers through `implemented-by`.
func TestWalkDirectionNeverClimbsToInterface(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "Base"}, {ID: "Mid"}, {ID: "Leaf"}, {ID: "IOp"}, {ID: "OtherImpl"}},
		Edges: []GraphEdge{
			{From: "Mid", To: "Base", Kind: "extends", Type: "extends"},
			{From: "Leaf", To: "Mid", Kind: "extends", Type: "extends"},
			{From: "Base", To: "IOp", Kind: "implements", Type: "implements"},
			{From: "OtherImpl", To: "IOp", Kind: "implements", Type: "implements"},
		},
	}
	out, _, err := Walk(g, "Base", 2, mustFollow(t, "extended-by"), 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range out.Nodes {
		ids[n.ID] = true
	}
	if !ids["Base"] || !ids["Mid"] || !ids["Leaf"] {
		t.Fatalf("expected Base, Mid, Leaf, got %+v", out.Nodes)
	}
	if ids["IOp"] || ids["OtherImpl"] {
		t.Fatalf("extended-by must never climb to the interface or its other implementers: %+v", out.Nodes)
	}
}

// TestWalkStep: every node of a neighbourhood answer carries its distance
// from the focus.
func TestWalkStep(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "uses", Type: "uses"},
			{From: "b", To: "c", Kind: "uses", Type: "uses"},
		},
	}
	out, _, err := Walk(g, "a", 2, mustFollow(t, "uses"), 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	steps := map[string]int{}
	for _, n := range out.Nodes {
		if n.Step == nil {
			t.Fatalf("node %s has no step", n.ID)
		}
		steps[n.ID] = *n.Step
	}
	if steps["a"] != 0 || steps["b"] != 1 || steps["c"] != 2 {
		t.Fatalf("wrong steps: %+v", steps)
	}
}

// TestWalkFanout: at most `fanout` fresh neighbours per (node, relation) are
// taken into the walk, and the excess is reported, never silently dropped
// from the count.
func TestWalkFanout(t *testing.T) {
	g := &Graph{Nodes: []GraphNode{{ID: "a"}}}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("n%d", i)
		g.Nodes = append(g.Nodes, GraphNode{ID: id})
		g.Edges = append(g.Edges, GraphEdge{From: "a", To: id, Kind: "uses", Type: "uses"})
	}
	out, notes, err := Walk(g, "a", 1, mustFollow(t, "uses"), 2)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(out.Nodes) != 3 { // a + 2 kept
		t.Fatalf("expected 3 nodes (fanout 2 + focus), got %+v", out.Nodes)
	}
	if len(notes) != 1 || notes[0].Node != "a" || notes[0].Relation != "uses" || notes[0].Kept != 2 || notes[0].Left != 3 {
		t.Fatalf("expected one fanout note (kept 2, left 3), got %+v", notes)
	}
}

func TestFilterContainer(t *testing.T) {
	g := filterFixtureGraph()
	defs := []Container{{ID: "c_app"}, {ID: "c_lib"}}
	out, err := FilterContainer(g, "c_app", defs)
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
	defs := []Container{{ID: "c_app"}}
	if _, err := FilterContainer(filterFixtureGraph(), "c_typo", defs); err == nil {
		t.Fatal("expected an error for an unknown container id")
	}
}

// TestFilterContainerIncludesGrandchild: a node resolved into the most
// specific (grandchild) container must still show up under its grandparent
// container, since containment by longest-prefix resolution otherwise hides
// everything but the leaf container (the bug this fix addresses).
func TestFilterContainerIncludesGrandchild(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "n1", Containers: []string{"c_onnx_core"}},
			{ID: "n2", Containers: []string{"c_other"}},
		},
		Edges: []GraphEdge{{From: "n1", To: "n2", Kind: "uses"}},
	}
	defs := []Container{
		{ID: "c_onnx"},
		{ID: "c_onnx_core", Parent: "c_onnx"},
		{ID: "c_onnx_core_leaf", Parent: "c_onnx_core"},
		{ID: "c_other"},
	}
	out, err := FilterContainer(g, "c_onnx", defs)
	if err != nil {
		t.Fatalf("FilterContainer: %v", err)
	}
	if len(out.Nodes) != 1 || out.Nodes[0].ID != "n1" {
		t.Fatalf("expected n1 kept via its grandparent container, got %+v", out.Nodes)
	}
	// the node's own Containers list stays as resolved, not widened with ancestors.
	if len(out.Nodes[0].Containers) != 1 || out.Nodes[0].Containers[0] != "c_onnx_core" {
		t.Fatalf("Containers must stay as resolved, got %+v", out.Nodes[0].Containers)
	}
	if len(out.Edges) != 0 {
		t.Fatalf("n2 is outside c_onnx, so the edge must not survive: %+v", out.Edges)
	}
}

// TestFilterContainerParentCycle: a cycle in `parent` must not hang the
// descendant walk.
func TestFilterContainerParentCycle(t *testing.T) {
	g := &Graph{Nodes: []GraphNode{{ID: "n1", Containers: []string{"c_b"}}}}
	defs := []Container{
		{ID: "c_a", Parent: "c_b"},
		{ID: "c_b", Parent: "c_a"}, // cycle
	}
	done := make(chan struct{})
	var out *Graph
	var err error
	go func() {
		out, err = FilterContainer(g, "c_a", defs)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FilterContainer hung on a parent cycle")
	}
	if err != nil {
		t.Fatalf("FilterContainer: %v", err)
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("expected n1 kept (c_b is c_a's descendant despite the cycle), got %+v", out.Nodes)
	}
}

func TestFilterMissingDefaultExcludes(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "both1", Presence: "both", Status: "present"},
			{ID: "code1", Presence: "code"},
			{ID: "model_present", Presence: "model", Status: "present"},
			{ID: "model_missing", Presence: "model", Status: "missing"},
		},
		Edges: []GraphEdge{
			{From: "both1", To: "code1", Kind: "uses", Presence: "code"},
			{From: "both1", To: "model_present", Kind: "uses", Presence: "model", Status: "present"},
			{From: "both1", To: "model_missing", Kind: "uses", Presence: "model", Status: "missing"},    // touches a dropped node
			{From: "code1", To: "model_present", Kind: "extends", Presence: "model", Status: "missing"}, // model-only edge itself missing
		},
	}
	out, hiddenNodes, hiddenEdges := FilterMissing(g, false)
	if hiddenNodes != 1 {
		t.Fatalf("expected 1 hidden node, got %d: %+v", hiddenNodes, out.Nodes)
	}
	if hiddenEdges != 2 {
		t.Fatalf("expected 2 hidden edges, got %d: %+v", hiddenEdges, out.Edges)
	}
	if len(out.Nodes) != 3 {
		t.Fatalf("expected 3 remaining nodes, got %+v", out.Nodes)
	}
	if len(out.Edges) != 2 {
		t.Fatalf("expected 2 remaining edges, got %+v", out.Edges)
	}
}

func TestFilterMissingIncludeKeepsEverything(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "model_missing", Presence: "model", Status: "missing"}},
		Edges: []GraphEdge{{From: "model_missing", To: "model_missing", Kind: "uses", Presence: "model", Status: "missing"}},
	}
	out, hiddenNodes, hiddenEdges := FilterMissing(g, true)
	if hiddenNodes != 0 || hiddenEdges != 0 {
		t.Fatalf("include:true must report nothing hidden, got %d/%d", hiddenNodes, hiddenEdges)
	}
	if len(out.Nodes) != 1 || len(out.Edges) != 1 {
		t.Fatalf("include:true must keep everything: %+v", out)
	}
}

func TestFilterMissingNeverDropsBothOrCode(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "both1", Presence: "both", Status: "missing"}, // status missing, but presence both: never dropped
			{ID: "code1", Presence: "code", Status: "missing"},
		},
	}
	out, hiddenNodes, _ := FilterMissing(g, false)
	if hiddenNodes != 0 || len(out.Nodes) != 2 {
		t.Fatalf("both/code presence must never be dropped by FilterMissing, got %+v (hidden %d)", out.Nodes, hiddenNodes)
	}
}

func TestParseFieldsUnknownName(t *testing.T) {
	if _, err := ParseFields([]string{"via", "bogus"}); err == nil {
		t.Fatal("expected an error for an unknown field name")
	}
}

func TestParseFieldsEmptyIsEmptySet(t *testing.T) {
	set, err := ParseFields(nil)
	if err != nil || len(set) != 0 {
		t.Fatalf("expected an empty set for an empty list, got %v, %v", set, err)
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
