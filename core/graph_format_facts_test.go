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
		"7 nodes (7 types, 0 methods), 7 edges\n"
	got := string(body)
	if got != want {
		t.Fatalf("golden facts answer changed.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// pointOfViewFixture: A calls B (lifted, count 1) and A extends B — small
// enough to check every text format names both relations the same way.
func pointOfViewFixture() *Graph {
	step0, step1 := 0, 1
	return &Graph{
		Nodes: []GraphNode{
			{ID: "a", Symbol: "App.A", Name: "A", Kind: "type", File: "A.cs", Line: 1, EndLine: 10, Step: &step0},
			{ID: "b", Symbol: "App.B", Name: "B", Kind: "type", File: "B.cs", Line: 1, EndLine: 10, Step: &step1},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "calls", Type: "calls", Count: 1, FromMethods: []string{"Run"}, ToMethods: []string{"Go"}},
			{From: "a", To: "b", Kind: "extends", Type: "extends"},
		},
	}
}

// TestOnePointOfViewAcrossFormats: defect 7 — facts, lines and tree all name
// the same edges the same way (from the point of view of the node the walk
// came from), and lines/tree get the lifted `from … to …` naming too,
// through the same shared functions.
func TestOnePointOfViewAcrossFormats(t *testing.T) {
	g := pointOfViewFixture()
	for _, name := range []string{"facts", "lines", "tree"} {
		f, ok := GetGraphFormat(name)
		if !ok {
			t.Fatalf("format %q not registered", name)
		}
		body, err := f.Format(g, FormatOptions{Focus: "a"})
		if err != nil {
			t.Fatalf("%s: Format: %v", name, err)
		}
		out := string(body)
		if !strings.Contains(out, "calls from Run to Go") {
			t.Fatalf("%s: expected 'calls from Run to Go' (A's own forward view), got:\n%s", name, out)
		}
		// The relation about B, reached from A, must say "extends" (A's own
		// forward direction), never "extended-by" (B's own). `tree` also
		// prints B's own outward edges once it expands B as a parent in its
		// own right — correctly, in B's own words — so only the first
		// relation line (depth 1, reached from A) is checked for it.
		checked := out
		if name == "tree" {
			lines := strings.SplitN(out, "\n", 4)
			checked = strings.Join(lines[1:3], "\n")
		}
		if !strings.Contains(checked, "extends") || strings.Contains(checked, "extended-by") {
			t.Fatalf("%s: expected 'extends' (A's forward view), not 'extended-by', in:\n%s", name, checked)
		}
	}
}

// TestPlacesOfCallsShowFileWhenNotOnTheLine: defect B — a lifted `calls`
// relation names the call sites, and when the caller's file differs from
// the file already printed on that line, says so.
func TestPlacesOfCallsShowFileWhenNotOnTheLine(t *testing.T) {
	g := pointOfViewFixture()
	g.Edges[0].Line = 38
	g.Edges[0].Lines = []int{38, 41, 44}
	g.Edges[0].Count = 3
	g.Edges[0].ToMethods = []string{"Go1", "Go2", "Go3"}

	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{Focus: "a"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	// B's own line (reached via "calls" from A, the caller) must say the
	// sites are in A's file (A.cs), since B's own line already prints B.cs.
	if !strings.Contains(out, "@38,41,44 in A.cs") {
		t.Fatalf("expected the call sites with 'in A.cs' (not B's own file), got:\n%s", out)
	}
}

// TestPlacesOfCallsNoFileWhenAlreadyOnTheLine: the reverse direction
// (`called-by`, viewed from the callee back to the caller) needs no "in
// FILE" suffix — the caller's file is the callee's OWN file, already
// printed on that very line.
func TestPlacesOfCallsNoFileWhenAlreadyOnTheLine(t *testing.T) {
	// No Step data (a plain whole-graph list): relationsReachingNode falls
	// back to each node's own forward perspective, so B's own line shows
	// its OWN outward relation to A — here that is "called-by" (B is called
	// by A), and the call site is in A.From's file... no: e.From is "a"
	// regardless of viewing direction, so the site is in A's file, which
	// IS printed on A's own line elsewhere, but NOT on B's line — so this
	// checks the opposite pairing: A's own line, viewed from A itself,
	// where the site file (A.cs) equals A's own printed file.
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Symbol: "App.A", Name: "A", Kind: "type", File: "A.cs", Line: 1, EndLine: 10},
			{ID: "b", Symbol: "App.B", Name: "B", Kind: "type", File: "B.cs", Line: 1, EndLine: 10},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "calls", Type: "calls", Line: 38, Lines: []int{38, 41, 44}},
		},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "@38,41,44") {
		t.Fatalf("expected the call sites, got:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "App.A") && strings.Contains(line, "@38,41,44 in") {
			t.Fatalf("A's own 'calls' line must not say 'in FILE': the site is already in A's own printed file, got:\n%s", line)
		}
	}
}

// TestFactsFormatListCapFromOptions: {fromMethods}/{toMethods}/{relationLines}
// are capped by FormatOptions.ListCap, not the old fixed 5/8 (step 3a).
func TestFactsFormatListCapFromOptions(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Name: "A", Kind: "type", File: "A.cs", Line: 1},
			{ID: "b", Name: "B", Kind: "type", File: "B.cs", Line: 1},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Kind: "calls", Type: "calls", Count: 3,
				FromMethods: []string{"M1", "M2", "M3"}, ToMethods: []string{"N1", "N2", "N3"},
				Lines: []int{1, 2, 3, 4}},
		},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{ListCap: 2})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "from M1,M2+1 to N1,N2+1") {
		t.Fatalf("expected fromMethods/toMethods capped at 2, got:\n%s", out)
	}
	if !strings.Contains(out, "@1,2+2") {
		t.Fatalf("expected relationLines capped at 2, got:\n%s", out)
	}
}

// TestFactsFormatCutNoticeFirstLine (step 3b): a limit-truncated answer says
// so on the first line (after any name-resolution notice), and the trailing
// counts line carries only the counts.
func TestFactsFormatCutNoticeFirstLine(t *testing.T) {
	g := &Graph{Nodes: []GraphNode{{ID: "a", Name: "A", Kind: "type"}}}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{
		Notice: `asked "a", took "A"`, Truncated: true, FullNodes: 50, FullEdges: 10,
		FanoutNotes: []FanoutNote{{Node: "a", Relation: "calls", Kept: 2, Left: 8}},
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	lines := strings.Split(string(body), "\n")
	if lines[0] != `asked "a", took "A"` {
		t.Fatalf("expected the resolution notice first, got:\n%s", lines[0])
	}
	if !strings.Contains(lines[1], "cut by limit") || !strings.Contains(lines[1], "fanout cut") {
		t.Fatalf("expected the cut notice as the second line (right after the resolution notice), got:\n%s", lines[1])
	}
	last := lines[len(lines)-2] // len-1 is the trailing "" after the final \n
	if strings.Contains(last, "cut") || strings.Contains(last, "limit") || strings.Contains(last, "fanout") {
		t.Fatalf("expected the counts line to carry counts only, got:\n%s", last)
	}
	if !strings.Contains(last, "nodes (") || !strings.Contains(last, "edges") {
		t.Fatalf("expected a counts line, got:\n%s", last)
	}
}

// TestFactsFormatAssemblyOnlyForFocus (step 3c): the focus node's line
// carries namespace and assembly; a neighbour's does not, even though it has
// the same data.
func TestFactsFormatAssemblyOnlyForFocus(t *testing.T) {
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "a", Name: "A", Kind: "type", File: "A.cs", Line: 1, Namespace: "App.NS", Assembly: "App.Asm", Step: intPtr(0)},
			{ID: "b", Name: "B", Kind: "type", File: "B.cs", Line: 1, Namespace: "App.NS", Assembly: "App.Asm", Step: intPtr(1)},
		},
		Edges: []GraphEdge{{From: "a", To: "b", Kind: "holds"}},
	}
	f, _ := GetGraphFormat("facts")
	body, err := f.Format(g, FormatOptions{Focus: "a"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	lines := strings.Split(string(body), "\n")
	if !strings.Contains(lines[0], "in App.NS (assembly App.Asm)") {
		t.Fatalf("expected the focus line to carry namespace/assembly, got:\n%s", lines[0])
	}
	if strings.Contains(lines[1], "(assembly") || strings.Contains(lines[1], "  in App.NS") {
		t.Fatalf("expected the neighbour's line to stay short, got:\n%s", lines[1])
	}
}

func intPtr(n int) *int { return &n }
