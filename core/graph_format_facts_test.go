package core

import (
	"strings"
	"testing"
)

// TestFactsFormatCombinesHoldsAndInjectsOfSameMember: the same member of the
// same pair held AND injected (a constructor parameter that also becomes a
// field) prints as one relation "holds … (injected)" (part 3).
func TestFactsFormatCombinesHoldsAndInjectsOfSameMember(t *testing.T) {
	step0, step1 := 0, 1
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Symbol: "App.A", Name: "A", Step: &step0},
			{ID: "b", Symbol: "App.B", Name: "B", Step: &step1},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "holds", Type: "holds.one", Via: &Via{Member: "ctx", MemberKind: "field"}},
			{From: "a", To: "b", Kind: "uses", Type: "injects", Via: &Via{Member: "ctx", MemberKind: "constructor"}},
		},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{Focus: "a"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "holds (injected) ctx") {
		t.Fatalf("expected the combined 'holds … (injected)' relation, got:\n%s", out)
	}
	if strings.Count(out, "injects ctx") != 0 {
		t.Fatalf("the plain 'injects' line must be folded away, got:\n%s", out)
	}
}

// TestFactsFormatRelationFromReachedFromPerspective: {relation} is named
// from the point of view of the node the neighbour was reached FROM (part
// 3) — RepetitionGuard extends Middleware, so Middleware's own line says
// "extends" (RepetitionGuard's forward direction), not "extended-by"
// (Middleware's own).
func TestFactsFormatRelationFromReachedFromPerspective(t *testing.T) {
	g := formatFixtureGraph()
	follow, _ := ParseFollow(nil)
	walked, _, err := Walk(g, "cs:App.Guards.RepetitionGuard", 1, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(walked, FormatOptions{Focus: "cs:App.Guards.RepetitionGuard"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "App.Middleware  src/Middleware.cs:1-60   extends") {
		t.Fatalf("expected 'extends' (RepetitionGuard's perspective), got:\n%s", out)
	}
	if strings.Contains(out, "extended-by") {
		t.Fatalf("must not print the neighbour's own perspective, got:\n%s", out)
	}
}

// TestGraphWorksWithMethodsAndCalls: the graph must render sensibly even
// though core/facts.go does not yet accept `method` symbols or `calls`
// edges (ADR_20260928-4 is still extractor work in progress) — built
// directly as a core.Graph, per the task's instruction to not wait for the
// extractor.
func TestGraphWorksWithMethodsAndCalls(t *testing.T) {
	step0, step1, step2 := 0, 1, 1
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "t:Factory", Symbol: "App.Factory", Name: "Factory", Kind: "type", Step: &step0},
			{ID: "m:Factory.Build()", Symbol: "App.Factory.Build()", Name: "Build()", Kind: "method", NativeKind: "method", Step: &step1},
			{ID: "t:Widget", Symbol: "App.Widget", Name: "Widget", Kind: "type", Step: &step2},
		},
		Edges: []GraphEdge{
			{From: "t:Factory", To: "m:Factory.Build()", Kind: "contains", Type: "contains"},
			{From: "m:Factory.Build()", To: "t:Widget", Kind: "constructs", Type: "constructs", Line: 42},
		},
	}
	follow, err := ParseFollow([]string{"contains", "constructs"})
	if err != nil {
		t.Fatalf("ParseFollow: %v", err)
	}
	out, _, err := Walk(g, "t:Factory", 2, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(out.Nodes) != 3 {
		t.Fatalf("expected all 3 nodes reached via contains+constructs, got %+v", out.Nodes)
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(out, FormatOptions{Focus: "t:Factory"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.Contains(string(body), "App.Widget") {
		t.Fatalf("expected the constructed type in the answer, got:\n%s", body)
	}
}

// TestFilterLevelTypesDropsMethodsOfCallsGraph: level=types also drops
// method nodes of a graph that has them (ADR_20260928-3 §7).
func TestFilterLevelTypesDropsMethodsOfCallsGraph(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{{ID: "t", Kind: "type"}, {ID: "m", Kind: "method"}},
		Edges: []GraphEdge{{From: "m", To: "t", Kind: "calls", Type: "calls"}},
	}
	out, err := FilterLevel(g, "types")
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if len(out.Nodes) != 1 || out.Nodes[0].ID != "t" {
		t.Fatalf("expected only the type node, got %+v", out.Nodes)
	}
}
