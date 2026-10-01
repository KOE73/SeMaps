package core

import "testing"

func findFixtureGraph() *Graph {
	return &Graph{Nodes: []GraphNode{
		{ID: "nmfn:App.Runner`2", Symbol: "App.Runner`2", Kind: "type", Name: "Runner`2", Namespace: "App"},
		{ID: "nmfn:App.Sub.IRunner", Symbol: "App.Sub.IRunner", Kind: "interface", Name: "IRunner", Namespace: "App.Sub"},
		{ID: "nmfn:App.Other.IRunner", Symbol: "App.Other.IRunner", Kind: "interface", Name: "IRunner", Namespace: "App.Other"},
		{ID: "nmfn:App.Hidden", Symbol: "App.Hidden", Kind: "type", Name: "Hidden", Namespace: "App", Presence: "model", Status: "missing"},
		{ID: "nmfn:App.Guards.RepetitionGuard", Symbol: "App.Guards.RepetitionGuard", Kind: "type", Name: "RepetitionGuard", Namespace: "App.Guards", File: "src/Guards/RepetitionGuard.cs", Line: 10},
	}}
}

func TestResolveNodeExactID(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "nmfn:App.Guards.RepetitionGuard")
	if res.Node == nil || res.Node.ID != "nmfn:App.Guards.RepetitionGuard" {
		t.Fatalf("expected exact id match, got %+v", res)
	}
	if res.Notice != "" {
		t.Fatalf("exact id hit should carry no notice, got %q", res.Notice)
	}
}

func TestResolveNodeIDWithoutExtractorPrefix(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "App.Guards.RepetitionGuard")
	if res.Node == nil || res.Node.ID != "nmfn:App.Guards.RepetitionGuard" {
		t.Fatalf("expected a match by id without the extractor prefix, got %+v", res)
	}
	if res.Notice == "" {
		t.Fatalf("expected a notice: the id differs from the literal query")
	}
}

func TestResolveNodeShortNameWithoutArity(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "Runner")
	if res.Node == nil || res.Node.ID != "nmfn:App.Runner`2" {
		t.Fatalf("expected a match on the short name with arity stripped, got %+v", res)
	}
}

func TestResolveNodeCaseInsensitive(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "repetitionguard")
	if res.Node == nil || res.Node.ID != "nmfn:App.Guards.RepetitionGuard" {
		t.Fatalf("expected a case-insensitive match, got %+v", res)
	}
}

func TestResolveNodeAmbiguous(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "IRunner")
	if res.Node != nil {
		t.Fatalf("expected no single node, got %+v", res.Node)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %+v", res.Candidates)
	}
}

func TestResolveNodeNoneSuggestsNearest(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "RepetitionGaurd") // typo
	if res.Node != nil || len(res.Candidates) != 0 {
		t.Fatalf("expected no match, got %+v", res)
	}
	if len(res.Suggestions) == 0 || res.Suggestions[0] != "RepetitionGuard (nmfn:App.Guards.RepetitionGuard)" {
		t.Fatalf("expected the nearest short name, with its full id, suggested first, got %+v", res.Suggestions)
	}
}

func TestResolveNodeMissingHidden(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "Hidden")
	if res.Node == nil {
		t.Fatalf("expected the hidden node to still resolve (caller decides to refuse it)")
	}
	if !res.MissingHidden {
		t.Fatalf("expected MissingHidden true for a model-only node with status missing")
	}
}

func TestFindNodesSubstring(t *testing.T) {
	g := findFixtureGraph()
	out := FindNodes(g, "irunner", 0)
	if len(out) != 2 {
		t.Fatalf("expected 2 substring matches, got %+v", out)
	}
}

// runnerTypeVsMethodGraph: a type `Runner`, its constructor (a method named
// like the type, per C# convention: id ends in `..ctor()`), and an unrelated
// type `StrategyRunner` — defect 11's own example: `Runner` must resolve to
// the type, never an ambiguity with the constructor.
func runnerTypeVsMethodGraph() *Graph {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "nmfn:App.Runner", Symbol: "App.Runner", Kind: "type", Name: "Runner", Namespace: "App"},
			{ID: "nmfn:App.Runner..ctor()", Symbol: "App.Runner..ctor()", Kind: "method", NativeKind: "constructor", Name: "ctor"},
			{ID: "nmfn:App.StrategyRunner", Symbol: "App.StrategyRunner", Kind: "type", Name: "StrategyRunner", Namespace: "App"},
			{ID: "nmfn:App.Factory", Symbol: "App.Factory", Kind: "type", Name: "YoloObbFactory", Namespace: "App"},
			{ID: "nmfn:App.Ctx", Symbol: "App.Ctx", Kind: "type", Name: "OnnxExecutionContext", Namespace: "App"},
			{ID: "nmfn:App.Factory.CreateRunner`1(App.Ctx)", Symbol: "App.Factory.CreateRunner`1(App.Ctx)", Kind: "method", NativeKind: "method", Name: "CreateRunner"},
		},
		Edges: []GraphEdge{
			{From: "nmfn:App.Factory", To: "nmfn:App.Factory.CreateRunner`1(App.Ctx)", Kind: "contains", Type: "contains"},
			{From: "nmfn:App.Runner", To: "nmfn:App.Runner..ctor()", Kind: "contains", Type: "contains"},
		},
	}
	return g
}

// TestResolveNodeTypePreferredOverMethod: defect 11 — `Runner` resolves to
// the type, never made ambiguous by a constructor or a method of a similar
// name (StrategyRunner is unrelated and does not interfere either).
func TestResolveNodeTypePreferredOverMethod(t *testing.T) {
	g := runnerTypeVsMethodGraph()
	res := ResolveNode(g, "Runner")
	if res.Node == nil {
		t.Fatalf("expected Runner to resolve, got %+v", res)
	}
	if res.Node.ID != "nmfn:App.Runner" || res.Node.Kind != "type" {
		t.Fatalf("expected the type Runner, got %+v", res.Node)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected no ambiguity with the constructor, got %+v", res.Candidates)
	}
}

// TestResolveNodeTypeDotMember: `YoloObbFactory.CreateRunner` (the
// `Type.Member` form) resolves to the method, even though a type of a
// similar short name does not exist here — defect 11.
func TestResolveNodeTypeDotMember(t *testing.T) {
	g := runnerTypeVsMethodGraph()
	res := ResolveNode(g, "YoloObbFactory.CreateRunner")
	if res.Node == nil {
		t.Fatalf("expected a method match, got %+v", res)
	}
	if res.Node.Kind != "method" || res.Node.ID != "nmfn:App.Factory.CreateRunner`1(App.Ctx)" {
		t.Fatalf("expected the CreateRunner method, got %+v", res.Node)
	}
}

// TestMethodDisplayNameRoundTrips: the short signature facts/lines/tree
// print for a method (methodDisplayName) resolves back to that same method
// (defect 11) — not the full, up-to-250-character id.
func TestMethodDisplayNameRoundTrips(t *testing.T) {
	g := runnerTypeVsMethodGraph()
	byID := nodeByID(g)
	methodOf := methodContainerMap(g)
	var method *GraphNode
	for i := range g.Nodes {
		if g.Nodes[i].ID == "nmfn:App.Factory.CreateRunner`1(App.Ctx)" {
			method = &g.Nodes[i]
		}
	}
	disp := methodDisplayName(method, byID, methodOf)
	if disp != "YoloObbFactory.CreateRunner`1(Ctx)" {
		t.Fatalf("unexpected display name: %q", disp)
	}
	res := ResolveNode(g, disp)
	if res.Node == nil || res.Node.ID != method.ID {
		t.Fatalf("printed name %q did not resolve back to the method, got %+v", disp, res)
	}
}

// TestResolveNodeSuggestionsTyposPreferTypes: "IRaner" (typo of IRunner) —
// suggestions compare short names, keep only candidates within a third of
// the query's length (>=1, <=3), and list at most 5, types first (defect 9).
func TestResolveNodeSuggestionsTyposPreferTypes(t *testing.T) {
	g := &Graph{Nodes: []GraphNode{
		{ID: "nmfn:App.IRunner`2", Symbol: "App.IRunner`2", Kind: "interface", Name: "IRunner`2", Namespace: "App"},
		{ID: "nmfn:App.ITracker", Symbol: "App.ITracker", Kind: "interface", Name: "ITracker", Namespace: "App"},
		{ID: "nmfn:App.Unrelated", Symbol: "App.Unrelated", Kind: "type", Name: "CompletelyUnrelatedName", Namespace: "App"},
	}}
	res := ResolveNode(g, "IRaner")
	if len(res.Suggestions) == 0 {
		t.Fatalf("expected at least one suggestion, got none")
	}
	if res.Suggestions[0] != "IRunner (nmfn:App.IRunner`2)" {
		t.Fatalf("expected IRunner suggested first, got %+v", res.Suggestions)
	}
	for _, s := range res.Suggestions {
		if s == "CompletelyUnrelatedName (nmfn:App.Unrelated)" {
			t.Fatalf("a name far over the distance threshold must not be suggested, got %+v", res.Suggestions)
		}
	}
}

// TestResolveNodeMissingArityNotice: defect 8 — asking a generic type
// without its arity says so, instead of a blanket "no such id".
func TestResolveNodeMissingArityNotice(t *testing.T) {
	g := findFixtureGraph()
	res := ResolveNode(g, "Runner")
	want := "asked `Runner`; no `Runner` without type parameters; taken `nmfn:App.Runner`2`"
	if res.Notice != want {
		t.Fatalf("got %q\nwant %q", res.Notice, want)
	}
}
