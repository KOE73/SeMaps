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
	out := callGraph(t, cs, map[string]any{"limit": float64(3)})
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
	out := callGraph(t, cs, map[string]any{"limit": float64(200)})
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
	out := callGraph(t, cs, map[string]any{"around": "e_0", "limit": float64(1)})
	if out["truncated"] != false {
		t.Fatalf("expected around to bypass truncation, got %+v", out)
	}
}
