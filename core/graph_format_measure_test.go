// Not a failing assertion: prints a table of answer sizes per format, on a
// graph shaped like the case the formatter task was measured against
// (docs/plans/PLAN_20260928_host_graph-provider.md, "Результат проверки"/
// "Экономия для агента" — a focus type with 6 relations, inside a namespace
// with 40 other types), at depth 1 and depth 2 with the default set.
package core

import (
	"fmt"
	"testing"
)

func measuredGraph() *Graph {
	nodes := []GraphNode{}
	edges := []GraphEdge{}
	ns := "App.Guards"
	focus := "cs:App.Guards.RepetitionGuard"
	nodes = append(nodes, GraphNode{
		ID: focus, Symbol: "App.Guards.RepetitionGuard", Kind: "type", Name: "RepetitionGuard",
		Namespace: ns, File: "src/Guards/RepetitionGuard.cs", Line: 10, EndLine: 90,
		Presence: "code", Language: "csharp", Extractor: "csharp",
	})
	// 6 relations off the focus type, each to its own small type.
	kinds := []string{"implements", "extends", "holds", "holds", "uses", "uses"}
	for i, k := range kinds {
		id := fmt.Sprintf("cs:App.Guards.R%d", i)
		nodes = append(nodes, GraphNode{
			ID: id, Symbol: fmt.Sprintf("App.Guards.R%d", i), Kind: "type", Name: fmt.Sprintf("R%d", i),
			Namespace: ns, File: fmt.Sprintf("src/Guards/R%d.cs", i), Line: 1, EndLine: 20,
			Presence: "code", Language: "csharp", Extractor: "csharp",
		})
		var via *Via
		if k == "holds" || k == "uses" {
			via = &Via{Member: "m"}
		}
		edges = append(edges, GraphEdge{From: focus, To: id, Kind: k, Type: k, Via: via, File: "src/Guards/RepetitionGuard.cs", Line: 11 + i})
		// depth-2 neighbours of each relation, one apiece, plus a members
		// edge among them so depth 2 has something to grow.
		nb := fmt.Sprintf("cs:App.Guards.R%d.Nb", i)
		nodes = append(nodes, GraphNode{ID: nb, Symbol: fmt.Sprintf("App.Guards.R%d.Nb", i), Kind: "type", Name: fmt.Sprintf("R%dNb", i), Namespace: ns, File: fmt.Sprintf("src/Guards/R%d.cs", i), Line: 5, EndLine: 8, Presence: "code", Language: "csharp", Extractor: "csharp"})
		edges = append(edges, GraphEdge{From: id, To: nb, Kind: "uses", Type: "uses", File: fmt.Sprintf("src/Guards/R%d.cs", i), Line: 6})
	}
	// 40 other types in the same namespace, containment-linked to a module
	// node (never walked by the default set), so they inflate a whole-graph
	// answer but not a `links`-set neighbourhood of the focus type.
	module := "cs:App.Guards.Module"
	nodes = append(nodes, GraphNode{ID: module, Kind: "module", NativeKind: "file", Name: "Module", Namespace: ns, File: "src/Guards/Module.cs", Presence: "code", Language: "csharp", Extractor: "csharp"})
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("cs:App.Guards.Other%d", i)
		nodes = append(nodes, GraphNode{ID: id, Symbol: fmt.Sprintf("App.Guards.Other%d", i), Kind: "type", Name: fmt.Sprintf("Other%d", i), Namespace: ns, File: fmt.Sprintf("src/Guards/Other%d.cs", i), Line: 1, EndLine: 15, Presence: "code", Language: "csharp", Extractor: "csharp"})
		edges = append(edges, GraphEdge{From: module, To: id, Kind: "contains", Type: "contains"})
	}
	edges = append(edges, GraphEdge{From: module, To: focus, Kind: "contains", Type: "contains"})
	return &Graph{Nodes: nodes, Edges: edges}
}

// TestMeasureFormatSizes prints the byte table asked for by the task; it
// never fails on its own (go test -v to see it).
func TestMeasureFormatSizes(t *testing.T) {
	full := measuredGraph()
	fields := map[string]bool{"via": true, "position": true}

	fmt.Println("\nformat            depth-1 bytes   depth-2 bytes")
	for _, f := range GraphFormats() {
		for _, depth := range []int{1, 2} {
			g, err := Neighborhood(full, "cs:App.Guards.RepetitionGuard", depth)
			if err != nil {
				t.Fatalf("Neighborhood: %v", err)
			}
			kinds, err := ResolveEdgeKinds("", nil, true) // default neighbourhood set: links
			if err != nil {
				t.Fatalf("ResolveEdgeKinds: %v", err)
			}
			g = FilterEdgeKinds(full, kinds)
			g, err = Neighborhood(g, "cs:App.Guards.RepetitionGuard", depth)
			if err != nil {
				t.Fatalf("Neighborhood: %v", err)
			}
			body, err := f.Format(g, FormatOptions{Focus: "cs:App.Guards.RepetitionGuard", Fields: fields, Facts: []any{}, Stats: map[string]any{}})
			if err != nil {
				t.Logf("%-16s depth-%d: refused: %v", f.Name(), depth, err)
				continue
			}
			if depth == 1 {
				fmt.Printf("%-16s %6d bytes", f.Name(), len(body))
			} else {
				fmt.Printf("      %6d bytes\n", len(body))
			}
		}
	}
}
