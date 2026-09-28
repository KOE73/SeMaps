package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// formatFixtureGraph: a small, fixed graph used as the golden fixture for
// every format — one focus type (I) with an implementer, a base, a held
// member and a containing namespace, so each format has something of every
// kind to print.
func formatFixtureGraph() *Graph {
	return &Graph{
		Nodes: []GraphNode{
			{ID: "cs:App.Guards.RepetitionGuard", Symbol: "App.Guards.RepetitionGuard", Kind: "type", Name: "RepetitionGuard", Namespace: "App.Guards", File: "src/Guards/RepetitionGuard.cs", Line: 10, EndLine: 40, Presence: "code", Language: "csharp", Extractor: "csharp"},
			{ID: "cs:App.ILlmMiddleware", Symbol: "App.ILlmMiddleware", Kind: "type", Name: "ILlmMiddleware", Namespace: "App", File: "src/ILlmMiddleware.cs", Line: 5, EndLine: 8, Presence: "code", Language: "csharp", Extractor: "csharp"},
			{ID: "cs:App.Middleware", Symbol: "App.Middleware", Kind: "type", Name: "Middleware", Namespace: "App", File: "src/Middleware.cs", Line: 1, EndLine: 60, Presence: "code", Language: "csharp", Extractor: "csharp"},
			{ID: "cs:App.Guards.RepetitionGuard.Log", Symbol: "App.Guards.RepetitionGuard.Log", Kind: "value", Name: "Log", Namespace: "App.Guards", File: "src/Guards/RepetitionGuard.cs", Line: 12, EndLine: 12, Presence: "code", Language: "csharp", Extractor: "csharp"},
		},
		Edges: []GraphEdge{
			{From: "cs:App.Guards.RepetitionGuard", To: "cs:App.ILlmMiddleware", Kind: "implements", Type: "implements", File: "src/Guards/RepetitionGuard.cs", Line: 10},
			{From: "cs:App.Guards.RepetitionGuard", To: "cs:App.Middleware", Kind: "extends", Type: "extends", File: "src/Guards/RepetitionGuard.cs", Line: 10},
			{From: "cs:App.Guards.RepetitionGuard", To: "cs:App.Guards.RepetitionGuard.Log", Kind: "holds", Type: "holds.one", Via: &Via{Member: "Log"}, File: "src/Guards/RepetitionGuard.cs", Line: 15},
		},
	}
}

func TestJSONFormatUnchangedShape(t *testing.T) {
	g := formatFixtureGraph()
	f, ok := GetGraphFormat("json")
	if !ok {
		t.Fatal("json format not registered")
	}
	body, err := f.Format(g, FormatOptions{Fields: map[string]bool{"via": true, "position": true}, Facts: []any{}, Stats: map[string]any{"nodes": 4}})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad JSON: %v: %s", err, body)
	}
	nodes, _ := m["nodes"].([]any)
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %+v", nodes)
	}
	n0 := nodes[0].(map[string]any)
	if n0["id"] != "cs:App.Guards.RepetitionGuard" || n0["file"] != "src/Guards/RepetitionGuard.cs" {
		t.Fatalf("json format changed node shape: %+v", n0)
	}
	if _, hasTruncated := m["truncated"]; hasTruncated {
		t.Fatalf("json format must not carry truncated when Truncated is false, got %+v", m)
	}
}

func TestJSONFormatDeterministic(t *testing.T) {
	g := formatFixtureGraph()
	f, _ := GetGraphFormat("json")
	opts := FormatOptions{Fields: map[string]bool{"via": true, "position": true}}
	b1, _ := f.Format(g, opts)
	b2, _ := f.Format(g, opts)
	if string(b1) != string(b2) {
		t.Fatalf("json format is not deterministic")
	}
}

func TestJSONCompactFormat(t *testing.T) {
	g := formatFixtureGraph()
	f, ok := GetGraphFormat("json-compact")
	if !ok {
		t.Fatal("json-compact format not registered")
	}
	body, err := f.Format(g, FormatOptions{Fields: map[string]bool{"via": true, "position": true}})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad JSON: %v: %s", err, body)
	}
	// language/extractor go once at the head, not per node.
	langs, _ := m["language"].([]any)
	if len(langs) != 1 || langs[0] != "csharp" {
		t.Fatalf("expected language:[\"csharp\"] once at the head, got %+v", m["language"])
	}
	// files are interned: 4 nodes share 3 distinct files (two nodes share
	// src/Guards/RepetitionGuard.cs).
	files, _ := m["files"].([]any)
	if len(files) != 3 {
		t.Fatalf("expected 3 distinct files in the table, got %+v", files)
	}
	nodes, _ := m["nodes"].([]any)
	n0 := nodes[0].(map[string]any)
	if _, hasNamespace := n0["namespace"]; hasNamespace {
		t.Fatalf("json-compact must not repeat namespace (contained in symbol), got %+v", n0)
	}
	if n0["fi"] == nil {
		t.Fatalf("expected a file index (fi) on the node, got %+v", n0)
	}
	edges, _ := m["edges"].([]any)
	e0 := edges[0].(map[string]any)
	if _, hasFromID := e0["from"]; hasFromID {
		t.Fatalf("json-compact edges must use the short key 'f', not 'from': %+v", e0)
	}
	fromID, ok := e0["f"].(string)
	if !ok || fromID == "" {
		t.Fatalf("expected edge.f to be a node id (part 1: never an index an agent could misread), got %+v", e0)
	}
}

func TestLinesFormat(t *testing.T) {
	g := formatFixtureGraph()
	f, _ := GetGraphFormat("lines")
	body, err := f.Format(g, FormatOptions{Focus: "cs:App.Guards.RepetitionGuard"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.HasPrefix(out, "App.Guards.RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40  in App.Guards\n") {
		t.Fatalf("expected the focus node first with its full name, location and namespace, got:\n%s", out)
	}
	if !strings.Contains(out, "extends  App.Middleware  src/Middleware.cs:1-60\n") {
		t.Fatalf("expected an 'extends' line, got:\n%s", out)
	}
	if !strings.Contains(out, "implements  App.ILlmMiddleware  src/ILlmMiddleware.cs:5-8\n") {
		t.Fatalf("expected an 'implements' line, got:\n%s", out)
	}
	if !strings.Contains(out, "holds: Log:15  App.Guards.RepetitionGuard.Log  src/Guards/RepetitionGuard.cs:12\n") {
		t.Fatalf("expected a 'holds' line with the member name and its line, got:\n%s", out)
	}
	if !strings.Contains(out, "implemented-by  App.Guards.RepetitionGuard") {
		t.Fatalf("expected the reverse direction ('implemented-by') on ILlmMiddleware's own block, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "4 nodes, 3 edges\n") {
		t.Fatalf("expected a trailing counts line, got:\n%s", out)
	}
}

func TestLocationsFormat(t *testing.T) {
	g := &Graph{Nodes: []GraphNode{
		{ID: "a", Name: "A", File: "src/A.cs", Line: 3, EndLine: 9},
		{ID: "b", Name: "B"},
	}}
	f, _ := GetGraphFormat("locations")
	body, err := f.Format(g, FormatOptions{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	want := "A\tsrc/A.cs\t3\t9\nB\t\t\t\n2 nodes, 0 edges\n"
	if string(body) != want {
		t.Fatalf("locations format mismatch:\ngot:  %q\nwant: %q", body, want)
	}
}

func TestTreeFormatWithoutFocusRefused(t *testing.T) {
	g := formatFixtureGraph()
	f, _ := GetGraphFormat("tree")
	if _, err := f.Format(g, FormatOptions{}); err == nil {
		t.Fatal("expected tree without a focus node to be refused")
	}
}

func TestTreeFormat(t *testing.T) {
	g := formatFixtureGraph()
	f, _ := GetGraphFormat("tree")
	body, err := f.Format(g, FormatOptions{Focus: "cs:App.Guards.RepetitionGuard"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if !strings.HasPrefix(out, "App.Guards.RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40\n") {
		t.Fatalf("expected the focus node as the root, full name, got:\n%s", out)
	}
	if !strings.Contains(out, "extends → App.Middleware  src/Middleware.cs:1-60\n") {
		t.Fatalf("expected an indented 'extends -> Middleware' line, got:\n%s", out)
	}
	if !strings.Contains(out, "holds Log:15 → App.Guards.RepetitionGuard.Log  src/Guards/RepetitionGuard.cs:12\n") {
		t.Fatalf("expected the member name and its line on the 'holds' line, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "4 nodes, 3 edges\n") {
		t.Fatalf("expected a trailing counts line, got:\n%s", out)
	}
}

func TestTreeFormatRepeatedNodePrintedAsReference(t *testing.T) {
	// diamond: focus -> a -> shared, focus -> b -> shared
	g := &Graph{
		Nodes: []GraphNode{
			{ID: "focus", Name: "Focus"},
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
			{ID: "shared", Name: "Shared"},
		},
		Edges: []GraphEdge{
			{From: "focus", To: "a", Kind: "uses"},
			{From: "focus", To: "b", Kind: "uses"},
			{From: "a", To: "shared", Kind: "uses"},
			{From: "b", To: "shared", Kind: "uses"},
		},
	}
	f, _ := GetGraphFormat("tree")
	body, err := f.Format(g, FormatOptions{Focus: "focus"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := string(body)
	if strings.Count(out, "Shared") != 2 {
		t.Fatalf("expected Shared printed twice (once in full, once as a reference), got:\n%s", out)
	}
	if !strings.Contains(out, "(see above)") {
		t.Fatalf("expected the second mention to say '(see above)', got:\n%s", out)
	}
}

func TestFormatsAreDeterministic(t *testing.T) {
	g := formatFixtureGraph()
	for _, f := range GraphFormats() {
		opts := FormatOptions{Focus: "cs:App.Guards.RepetitionGuard", Fields: map[string]bool{"via": true, "position": true}}
		b1, err1 := f.Format(g, opts)
		b2, err2 := f.Format(g, opts)
		if err1 != nil {
			continue // tree without applicable focus etc. is covered by its own tests
		}
		if err2 != nil || string(b1) != string(b2) {
			t.Fatalf("format %q is not deterministic", f.Name())
		}
	}
}

func TestUnknownFormatError(t *testing.T) {
	if _, ok := GetGraphFormat("bogus"); ok {
		t.Fatal("expected bogus format to be unknown")
	}
	if err := UnknownFormatError("bogus"); err == nil || !strings.Contains(err.Error(), "json") {
		t.Fatalf("expected the error to name available formats, got %v", err)
	}
}
