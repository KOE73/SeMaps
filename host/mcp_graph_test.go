package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// manyEntitiesSession is mcpSession's shape but with `n` model-only
// entities, so get_graph (docs/plans/PLAN_20260928_host_graph-provider.md
// step 6) has enough nodes to exercise `limit`/`truncated`.
func manyEntitiesSession(t *testing.T, n int) *mcp.ClientSession {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	var b strings.Builder
	b.WriteString(`{"entities":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"e_%d","name":"E%d","kind":"class","origin":"authored","status":"planned"}`, i, i)
	}
	b.WriteString(`]}`)
	files := map[string]string{
		"project.json":        `{"id":"p"}`,
		"entities.json":       b.String(),
		"relations.json":      `{"relations":[]}`,
		"relation-types.json": `{"relationTypes":[]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callGraph(t *testing.T, cs *mcp.ClientSession, args map[string]any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_graph", Arguments: args})
	if err != nil {
		t.Fatalf("get_graph: %v", err)
	}
	if res.IsError {
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		t.Fatalf("get_graph returned an error: %s", b.String())
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T: %+v", res.StructuredContent, res.StructuredContent)
	}
	return out
}

func TestMCPGetGraphTruncates(t *testing.T) {
	cs := manyEntitiesSession(t, 10)
	out := callGraph(t, cs, map[string]any{"limit": float64(3), "format": "json"})
	if out["truncated"] != true {
		t.Fatalf("expected truncated:true, got %+v", out)
	}
	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes after truncation, got %d: %+v", len(nodes), nodes)
	}
	if fn, ok := out["fullNodes"]; !ok || fn.(float64) != 10 {
		t.Fatalf("expected fullNodes:10 alongside truncated:true, got %+v", out)
	}
}

func TestMCPGetGraphNotTruncatedUnderLimit(t *testing.T) {
	cs := manyEntitiesSession(t, 3)
	out := callGraph(t, cs, map[string]any{"limit": float64(200), "format": "json"})
	if out["truncated"] != false {
		t.Fatalf("expected truncated:false under the limit, got %+v", out)
	}
	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("expected all 3 nodes, got %+v", nodes)
	}
}

func TestMCPGetGraphAroundIsNeverTruncated(t *testing.T) {
	cs := manyEntitiesSession(t, 5)
	out := callGraph(t, cs, map[string]any{"around": "e_0", "limit": float64(1), "format": "json"})
	if out["truncated"] != false {
		t.Fatalf("expected around to bypass truncation, got %+v", out)
	}
}

// TestMCPGetGraphFieldsAbsentIsDefault: no `fields` argument at all keeps
// the default (via,position) — file positions show up on the nodes.
func TestMCPGetGraphFieldsAbsentIsDefault(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	out := callGraph(t, cs, map[string]any{"format": "json"})
	nodes, _ := out["nodes"].([]any)
	if len(nodes) == 0 {
		t.Fatalf("expected at least one node, got %+v", out)
	}
}

// TestMCPGetGraphFieldsEmptyListIsNone: an explicit empty `fields` list
// means "nothing extra", distinguished from "absent" by the tool using a
// slice (nil vs non-nil empty) rather than a plain string.
func TestMCPGetGraphFieldsEmptyListIsNone(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	out := callGraph(t, cs, map[string]any{"fields": []any{}, "format": "json"})
	nodes, _ := out["nodes"].([]any)
	for _, n := range nodes {
		nm := n.(map[string]any)
		if nm["file"] != nil {
			t.Fatalf("fields: [] must strip position, got %+v", nm)
		}
	}
}

func TestMCPGetGraphFieldsUnknownName(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_graph", Arguments: map[string]any{"fields": []any{"bogus"}}})
	if err != nil {
		t.Fatalf("get_graph: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error for an unknown field name, got %+v", res)
	}
}

// TestMCPGetGraphMissingDefaultVsInclude mirrors the HTTP endpoint's
// coverage of fix 3, through the MCP tool.
func TestMCPGetGraphMissingDefaultVsInclude(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json": `{"id":"p"}`,
		"entities.json": `{"entities":[
			{"id":"e_a","name":"A","kind":"class","origin":"authored","status":"present"},
			{"id":"e_gone","name":"Gone","kind":"class","origin":"authored","status":"missing"}
		]}`,
		"relations.json": `{"relations":[
			{"id":"r_gone","from":"e_a","to":"e_gone","type":"uses","origin":"code","status":"missing"}
		]}`,
		"relation-types.json": `{"relationTypes":[]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })

	out := callGraph(t, cs, map[string]any{"format": "json"})
	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (e_gone hidden by default), got %+v", nodes)
	}
	stats := out["stats"].(map[string]any)
	hidden := stats["hiddenMissing"].(map[string]any)
	if hidden["nodes"].(float64) != 1 || hidden["edges"].(float64) != 1 {
		t.Fatalf("expected hiddenMissing {nodes:1,edges:1}, got %+v", hidden)
	}

	out2 := callGraph(t, cs, map[string]any{"missing": true, "format": "json"})
	nodes2, _ := out2["nodes"].([]any)
	if len(nodes2) != 2 {
		t.Fatalf("expected 2 nodes with missing:true, got %+v", nodes2)
	}
}

// TestMCPGetGraphDefaultFormatIsText: the provisional tool default (lines)
// returns text content, not a JSON node/edge structure, and no
// structuredContent.
func TestMCPGetGraphDefaultFormatIsText(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_graph", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("get_graph: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_graph returned an error: %+v", res)
	}
	if res.StructuredContent != nil {
		t.Fatalf("expected no structuredContent for the default (text) format, got %+v", res.StructuredContent)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected one text content item, got %+v", res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text == "" {
		t.Fatalf("expected non-empty text content, got %+v", res.Content[0])
	}
}

func TestMCPGetGraphSetAndKindsConflict(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_graph", Arguments: map[string]any{"set": "links", "kinds": "extends"}})
	if err != nil {
		t.Fatalf("get_graph: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error for set+kinds both given, got %+v", res)
	}
}

func TestMCPGraphFormatsTool(t *testing.T) {
	cs := manyEntitiesSession(t, 1)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "graph_formats", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("graph_formats: %v", err)
	}
	if res.IsError {
		t.Fatalf("graph_formats returned an error: %+v", res)
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", res.StructuredContent)
	}
	formats, _ := out["formats"].([]any)
	if len(formats) != 5 {
		t.Fatalf("expected 5 formats, got %+v", formats)
	}
	defaults, _ := out["defaults"].(map[string]any)
	if defaults["format"] != "json" {
		t.Fatalf("expected the HTTP default (json) reported, got %+v", defaults)
	}
}
