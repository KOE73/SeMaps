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
	if len(res.Suggestions) == 0 || res.Suggestions[0] != "App.Guards.RepetitionGuard" {
		t.Fatalf("expected the nearest name suggested first, got %+v", res.Suggestions)
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
