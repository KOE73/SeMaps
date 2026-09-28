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
	if !strings.Contains(out, "holds ctx field (injected)") {
		t.Fatalf("expected the combined 'holds … (injected)' relation, (injected) at the end, got:\n%s", out)
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
	if !strings.Contains(out, "App.Middleware  src/Middleware.cs:1-60  [extends]") {
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

// TestFactsFormatHoldsInjectsMemberSpellings: the merge (defect 3) compares
// member names case-insensitively and ignoring one leading underscore — a
// field `_context`, a property `Context` and a constructor parameter
// `context` are all "the same member" for pairing purposes.
func TestFactsFormatHoldsInjectsMemberSpellings(t *testing.T) {
	cases := []struct {
		name       string
		holds, inj string
	}{
		{"identical", "context", "context"},
		{"case only", "Context", "context"},
		{"leading underscore", "_context", "context"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step0, step1 := 0, 1
			g := &Graph{
				Nodes: []GraphNode{
					{ID: "a", Symbol: "App.A", Name: "A", Step: &step0},
					{ID: "b", Symbol: "App.B", Name: "B", Step: &step1},
				},
				Edges: []GraphEdge{
					{From: "a", To: "b", Kind: "holds", Type: "holds.one", Via: &Via{Member: c.holds, MemberKind: "field"}},
					{From: "a", To: "b", Kind: "uses", Type: "injects", Via: &Via{Member: c.inj, MemberKind: "constructor"}},
				},
			}
			f, _ := GetGraphFormat("facts")
			body, err := f.Format(g, FormatOptions{Focus: "a"})
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			out := string(body)
			if strings.Count(out, "(injected)") != 1 {
				t.Fatalf("%s: expected exactly one merged '(injected)' relation, got:\n%s", c.name, out)
			}
			if strings.Contains(out, "injected-into") || strings.Contains(out, "injects") {
				t.Fatalf("%s: the plain injects/injected-into line must be folded away, got:\n%s", c.name, out)
			}
		})
	}
}

// TestFactsFormatHoldsInjectsDifferentMembersNotMerged: two DIFFERENT
// members of the same pair (one held, a different one injected) must NOT
// merge — merging is keyed on (pair, normalized member), not just the pair.
func TestFactsFormatHoldsInjectsDifferentMembersNotMerged(t *testing.T) {
	step0, step1 := 0, 1
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Symbol: "App.A", Name: "A", Step: &step0},
			{ID: "b", Symbol: "App.B", Name: "B", Step: &step1},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "holds", Type: "holds.one", Via: &Via{Member: "context", MemberKind: "field"}},
			{From: "a", To: "b", Kind: "uses", Type: "injects", Via: &Via{Member: "other", MemberKind: "constructor"}},
		},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{Focus: "a"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if strings.Contains(out, "(injected)") {
		t.Fatalf("different members must not merge, got:\n%s", out)
	}
	if !strings.Contains(out, "injects other") {
		t.Fatalf("expected the plain injects line for the unrelated member, got:\n%s", out)
	}
}

// TestFactsFormatLiftedCountOneStillNamesMethods: an unmerged lifted edge
// (Count <= 1) still shows {fromMethods}/{toMethods} — only the "×N" prefix
// is left out (defect 2c).
func TestFactsFormatLiftedCountOneStillNamesMethods(t *testing.T) {
	step0, step1 := 0, 1
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Symbol: "App.A", Name: "A", Step: &step0},
			{ID: "b", Symbol: "App.B", Name: "B", Step: &step1},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "calls", Type: "calls", Count: 1, FromMethods: []string{"CreateRunner"}, ToMethods: []string{"Model"}},
		},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{Focus: "a"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "[calls from CreateRunner to Model]") {
		t.Fatalf("expected an unmerged lifted call to still name its methods, with no '×', got:\n%s", out)
	}
	if strings.Contains(out, "×") {
		t.Fatalf("count <= 1 must not print '×', got:\n%s", out)
	}
}

// TestFactsFormatGoldenDepth2: the whole `facts` answer, byte for byte, on a
// small fixed graph at depth 2 — the golden test that protects the format
// from silent changes (defects 2-6 of the agent-answers-graph task). The
// graph has: a type held through a member that is also injected (Runner,
// merged with "(injected)"), a plain inject with no holds counterpart
// (ImageRunner), an interface implemented by the focus and, one step
// further, by another type (exercising {via} at step >= 2), a lifted call
// between two types naming its methods at count 1, and a lifted construct
// at count 3.
func TestFactsFormatGoldenDepth2(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "ctx", Symbol: "App.Context", Name: "Context", Kind: "type", File: "src/Context.cs", Line: 1, EndLine: 50},
			{ID: "imgrunner", Symbol: "App.ImageRunner", Name: "ImageRunner", Kind: "type", File: "src/ImageRunner.cs", Line: 1, EndLine: 20},
			{ID: "runner", Symbol: "App.Runner", Name: "Runner", Kind: "type", File: "src/Runner.cs", Line: 3, EndLine: 10},
			{ID: "irunner", Symbol: "App.IRunner", Name: "IRunner", Kind: "interface", File: "src/IRunner.cs", Line: 1, EndLine: 5},
			{ID: "impl", Symbol: "App.RunnerImpl", Name: "RunnerImpl", Kind: "type", File: "src/RunnerImpl.cs", Line: 1, EndLine: 30},
			{ID: "factory", Symbol: "App.Factory", Name: "Factory", Kind: "type", File: "src/Factory.cs", Line: 1, EndLine: 15},
			{ID: "widget", Symbol: "App.Widget", Name: "Widget", Kind: "type", File: "src/Widget.cs", Line: 1, EndLine: 8},
		},
		Edges: []GraphEdge{
			// ctx (focus) holds Runner via "Context" and also injects it (same
			// member, case-different) -> merged, (injected).
			{From: "ctx", To: "runner", Kind: "holds", Type: "holds.one", Via: &Via{Member: "Context", MemberKind: "property", Modifiers: []string{"protected", "readonly"}}, Line: 7},
			{From: "ctx", To: "runner", Kind: "uses", Type: "injects", Via: &Via{Member: "context", MemberKind: "constructor"}, Line: 7},
			// ctx injects ImageRunner (a plain inject, no holds counterpart).
			{From: "ctx", To: "imgrunner", Kind: "uses", Type: "injects", Via: &Via{Member: "runner", MemberKind: "constructor"}, Line: 13},
			// ctx implements IRunner, which is implemented by RunnerImpl too
			// (reached at step 2 from IRunner).
			{From: "ctx", To: "irunner", Kind: "implements", Type: "implements"},
			{From: "impl", To: "irunner", Kind: "implements", Type: "implements"},
			// ctx and factory call each other's methods (lifted, count 1) and
			// factory constructs Widget from 3 methods (lifted, count 3).
			{From: "ctx", To: "factory", Kind: "calls", Type: "calls", Count: 1, FromMethods: []string{"Setup"}, ToMethods: []string{"Build"}},
			{From: "factory", To: "widget", Kind: "constructs", Type: "constructs", Count: 3, FromMethods: []string{"Create", "List", "Refresh"}, ToMethods: []string{"Widget"}},
		},
	}
	follow, err := ParseFollow(nil)
	if err != nil {
		t.Fatalf("ParseFollow: %v", err)
	}
	walked, _, err := Walk(g, "ctx", 2, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(walked, FormatOptions{Focus: "ctx"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	want := "0 App.Context  src/Context.cs:1-50\n" +
		"1 App.Factory  src/Factory.cs:1-15  [calls from Setup to Build]\n" +
		"1 App.IRunner  src/IRunner.cs:1-5  [implements]\n" +
		"1 App.ImageRunner  src/ImageRunner.cs:1-20  [injects runner:13 constructor]\n" +
		"1 App.Runner  src/Runner.cs:3-10  [holds Context:7 property protected readonly (injected)]\n" +
		"2 App.RunnerImpl  src/RunnerImpl.cs:1-30  [implemented-by] via IRunner\n" +
		"2 App.Widget  src/Widget.cs:1-8  [constructs ×3 from Create,List,Refresh to Widget] via Factory\n" +
		"7 nodes, 7 edges\n"
	got := string(body)
	if got != want {
		t.Fatalf("golden facts answer changed.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
