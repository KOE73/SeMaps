package core

import (
	"reflect"
	"testing"
)

func graphModel(t *testing.T, entities, relations string) *Model {
	t.Helper()
	ws, _ := workspace(t, `{"id":"p"}`, entities, relations, "")
	m, err := LoadModel(ws, "p")
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	return m
}

func TestBuildGraphPresenceBoth(t *testing.T) {
	m := graphModel(t, `{"entities":[
		{"id":"e_x","name":"X","kind":"class","origin":"code","status":"present","codeRef":"src/X.cs","symbol":"A.X"}
	]}`, `{"relations":[]}`)
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{
		{ID: "A.X", Kind: "type", NativeKind: "class", Name: "X", File: "src/X.cs"},
	}}
	g, err := BuildGraph([]FactsSource{{Extractor: "csharp", Facts: facts}}, m)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d: %+v", len(g.Nodes), g.Nodes)
	}
	n := g.Nodes[0]
	if n.Presence != "both" || n.Entity != "e_x" || n.Symbol != "A.X" || n.ID != "csharp:A.X" {
		t.Fatalf("unexpected node: %+v", n)
	}
}

func TestBuildGraphPresenceCodeOnly(t *testing.T) {
	m := graphModel(t, `{"entities":[]}`, `{"relations":[]}`)
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{
		{ID: "A.Y", Kind: "type", NativeKind: "class", Name: "Y", File: "src/Y.cs"},
	}}
	g, err := BuildGraph([]FactsSource{{Extractor: "csharp", Facts: facts}}, m)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].Presence != "code" || g.Nodes[0].Entity != "" {
		t.Fatalf("unexpected nodes: %+v", g.Nodes)
	}
}

func TestBuildGraphPresenceModelOnlyAndMissingStatus(t *testing.T) {
	m := graphModel(t, `{"entities":[
		{"id":"e_planned","name":"Planned","kind":"class","origin":"authored","status":"planned"},
		{"id":"e_missing","name":"Gone","kind":"class","origin":"code","status":"missing","codeRef":"src/Gone.cs","symbol":"A.Gone"}
	]}`, `{"relations":[]}`)
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{}}
	g, err := BuildGraph([]FactsSource{{Extractor: "csharp", Facts: facts}}, m)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 model-only nodes, got %d: %+v", len(g.Nodes), g.Nodes)
	}
	for _, n := range g.Nodes {
		if n.Presence != "model" {
			t.Fatalf("expected model presence, got %+v", n)
		}
	}
	missing := g.Nodes[0]
	if missing.ID != "e_missing" {
		missing = g.Nodes[1]
	}
	if missing.Status != "missing" {
		t.Fatalf("expected status missing, got %+v", missing)
	}
}

func TestBuildGraphTwoExtractorsOneProject(t *testing.T) {
	m := graphModel(t, `{"entities":[
		{"id":"e_cs","name":"App","kind":"class","origin":"code","status":"present","codeRef":"App.cs","symbol":"App"},
		{"id":"e_ts","name":"Widget","kind":"module","origin":"code","status":"present","codeRef":"widget.ts","symbol":"widget"}
	]}`, `{"relations":[]}`)
	csFacts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{
		{ID: "App", Kind: "type", NativeKind: "class", Name: "App", File: "App.cs"},
	}}
	tsFacts := &Facts{Language: "typescript", Root: ".", Symbols: []Symbol{
		{ID: "widget", Kind: "module", NativeKind: "file", Name: "Widget", File: "widget.ts"},
	}}
	g, err := BuildGraph([]FactsSource{
		{Extractor: "csharp", Facts: csFacts},
		{Extractor: "typescript", Facts: tsFacts},
	}, m)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (one per extractor), got %d: %+v", len(g.Nodes), g.Nodes)
	}
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	if !ids["csharp:App"] || !ids["typescript:widget"] {
		t.Fatalf("expected extractor-prefixed keys to avoid collision, got %+v", g.Nodes)
	}
}

func TestBuildGraphDeterministic(t *testing.T) {
	m := graphModel(t, `{"entities":[
		{"id":"e_a","name":"A","kind":"class","origin":"code","status":"present","codeRef":"src/A.cs","symbol":"A"},
		{"id":"e_b","name":"B","kind":"class","origin":"code","status":"present","codeRef":"src/B.cs","symbol":"B"}
	]}`, `{"relations":[
		{"id":"r_a_b_extends","from":"e_a","to":"e_b","type":"extends","origin":"code","status":"present"}
	]}`)
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{
		{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "src/A.cs"},
		{ID: "B", Kind: "type", NativeKind: "class", Name: "B", File: "src/B.cs"},
	}, Edges: []Edge{{From: "A", To: "B", Kind: "extends"}}}
	var last *Graph
	for i := 0; i < 5; i++ {
		g, err := BuildGraph([]FactsSource{{Extractor: "csharp", Facts: facts}}, m)
		if err != nil {
			t.Fatalf("BuildGraph: %v", err)
		}
		if last != nil {
			if len(g.Nodes) != len(last.Nodes) || len(g.Edges) != len(last.Edges) {
				t.Fatalf("non-deterministic shape")
			}
			for i := range g.Nodes {
				if g.Nodes[i].ID != last.Nodes[i].ID {
					t.Fatalf("non-deterministic node order: %v vs %v", g.Nodes, last.Nodes)
				}
			}
			for i := range g.Edges {
				if !reflect.DeepEqual(g.Edges[i], last.Edges[i]) {
					t.Fatalf("non-deterministic edge order: %v vs %v", g.Edges, last.Edges)
				}
			}
		}
		last = g
	}
	if len(last.Edges) != 1 || last.Edges[0].Presence != "both" || last.Edges[0].Relation != "r_a_b_extends" {
		t.Fatalf("expected the extends edge matched to its relation: %+v", last.Edges)
	}
}
