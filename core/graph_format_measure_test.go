// Not a failing assertion: prints a table of answer sizes per format, on a
// graph shaped like the experiment's question 3 (docs/plans/PLAN_20260928-5_host_graph-answers-for-agents.md
// step 2, question 3: "who holds OnnxExecutionContext, and through which
// member" — 13 holders, two of them through two members apiece, plus 2 that
// only inject it), at depth 1 with the default `follow`.
package core

import (
	"fmt"
	"testing"
)

// question3Graph: X held by 13 types (two of them, H0/H1, through two
// members each: 15 `holds` edges total), plus 2 types that only inject it
// through their constructor.
func question3Graph() *Graph {
	x := "cs:App.OnnxExecutionContext"
	nodes := []GraphNode{
		{ID: x, Symbol: "App.OnnxExecutionContext", Kind: "type", Name: "OnnxExecutionContext", Namespace: "App", File: "src/OnnxExecutionContext.cs", Line: 10, EndLine: 80, Presence: "code", Language: "csharp", Extractor: "csharp"},
	}
	var edges []GraphEdge
	for i := 0; i < 13; i++ {
		id := fmt.Sprintf("cs:App.Holder%d", i)
		nodes = append(nodes, GraphNode{ID: id, Symbol: fmt.Sprintf("App.Holder%d", i), Kind: "type", Name: fmt.Sprintf("Holder%d", i), Namespace: "App", File: fmt.Sprintf("src/Holder%d.cs", i), Line: 1, EndLine: 30, Presence: "code", Language: "csharp", Extractor: "csharp"})
		edges = append(edges, GraphEdge{From: id, To: x, Kind: "holds", Type: "holds.one", Via: &Via{Member: "Context", MemberKind: "field"}, File: fmt.Sprintf("src/Holder%d.cs", i), Line: 7})
		if i < 2 {
			// two of them hold it through a second member as well.
			edges = append(edges, GraphEdge{From: id, To: x, Kind: "holds", Type: "holds.one", Via: &Via{Member: "AltContext", MemberKind: "property"}, File: fmt.Sprintf("src/Holder%d.cs", i), Line: 15})
		}
	}
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("cs:App.Injector%d", i)
		nodes = append(nodes, GraphNode{ID: id, Symbol: fmt.Sprintf("App.Injector%d", i), Kind: "type", Name: fmt.Sprintf("Injector%d", i), Namespace: "App", File: fmt.Sprintf("src/Injector%d.cs", i), Line: 1, EndLine: 20, Presence: "code", Language: "csharp", Extractor: "csharp"})
		edges = append(edges, GraphEdge{From: id, To: x, Kind: "uses", Type: "injects", Via: &Via{Member: "ctx", MemberKind: "constructor"}, File: fmt.Sprintf("src/Injector%d.cs", i), Line: 5})
	}
	return &Graph{Nodes: nodes, Edges: edges}
}

// TestMeasureFormatSizes prints the byte table the task asks for; it never
// fails on its own (go test -v to see it).
func TestMeasureFormatSizes(t *testing.T) {
	full := question3Graph()
	fields := map[string]bool{"via": true, "position": true}
	follow, err := ParseFollow(nil) // DefaultFollow
	if err != nil {
		t.Fatalf("ParseFollow: %v", err)
	}

	fmt.Println("\nformat            depth-1 bytes")
	for _, f := range GraphFormats() {
		g, _, err := Walk(full, "cs:App.OnnxExecutionContext", 1, follow, 0)
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		body, err := f.Format(g, FormatOptions{Focus: "cs:App.OnnxExecutionContext", Fields: fields, Facts: []any{}, Stats: map[string]any{}})
		if err != nil {
			t.Logf("%-16s refused: %v", f.Name(), err)
			continue
		}
		fmt.Printf("%-16s %6d bytes\n", f.Name(), len(body))
	}
}

// TestAllFormatsOnFixtureGraph prints every format's output on the shared
// golden fixture (go test -v to see it) — a live, always-current worked
// example of each format side by side.
func TestAllFormatsOnFixtureGraph(t *testing.T) {
	full := formatFixtureGraph()
	follow, _ := ParseFollow(nil)
	g, _, err := Walk(full, "cs:App.Guards.RepetitionGuard", 2, follow, 0)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, name := range []string{"facts", "lines", "tree", "locations", "json", "json-compact"} {
		f, _ := GetGraphFormat(name)
		body, err := f.Format(g, FormatOptions{Focus: "cs:App.Guards.RepetitionGuard", Fields: map[string]bool{"via": true, "position": true}, Facts: []any{}, Stats: map[string]any{}})
		if err != nil {
			continue
		}
		fmt.Printf("\n--- %s ---\n%s\n", name, body)
	}
}
