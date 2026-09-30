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

// chainSession: a registry with a 7-level inheritance chain, N0 <- N1 <- ... <- N7
// (each extends the one before), and nothing else.
func chainSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	var entities, relations []string
	for i := 0; i <= 7; i++ {
		entities = append(entities, fmt.Sprintf(`{"id":"e_%d","name":"N%d","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.N%d"}]}`, i, i, i))
		if i > 0 {
			relations = append(relations, fmt.Sprintf(`{"id":"r_%d","from":"e_%d","to":"e_%d","type":"extends","origin":"code","status":"present"}`, i, i, i-1))
		}
	}
	files := map[string]string{
		"project.json":        `{"id":"p","contractVersion":5}`,
		"entities.json":       `{"entities":[` + strings.Join(entities, ",") + `]}`,
		"relations.json":      `{"relations":[` + strings.Join(relations, ",") + `]}`,
		"relation-types.json": `{"relationTypes":[{"id":"extends","origin":"code","visibility":"visible"}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	s.proj.Mcp = mcpSettings{Tools: "one"}.withDefaults()
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

// depth "all" along inheritance walks a 7-level chain to its end; a number
// still works; with any other relation in follow, or none, it is refused with
// the relations it takes.
func TestGetGraphDepthAll(t *testing.T) {
	cs := chainSession(t)
	follow := []string{"extended-by", "implemented-by"}

	res, all := call(t, cs, "get_graph", map[string]any{"around": "N0", "follow": follow, "depth": "all"})
	if res.IsError || !strings.Contains(all, "N7") {
		t.Fatalf("depth all should reach N7 (7 levels down): error=%v\n%s", res.IsError, all)
	}
	_, five := call(t, cs, "get_graph", map[string]any{"around": "N0", "follow": follow, "depth": 5})
	if !strings.Contains(five, "N5") || strings.Contains(five, "N6") {
		t.Fatalf("a numeric depth 5 stops at level 5:\n%s", five)
	}
	if res, text := call(t, cs, "get_graph", map[string]any{"around": "N0", "follow": follow, "depth": "3"}); res.IsError || strings.Contains(text, "N4") {
		t.Fatalf(`a numeric string "3" is depth 3: error=%v\n%s`, res.IsError, text)
	}

	for name, args := range map[string]map[string]any{
		"no follow":         {"around": "N0", "depth": "all"},
		"a call relation":   {"around": "N0", "follow": []string{"extended-by", "calls"}, "depth": "all"},
		"a holds relation":  {"around": "N0", "follow": []string{"held-by"}, "depth": "ALL"},
		"a word, not 'all'": {"around": "N0", "follow": follow, "depth": "deep"},
	} {
		res, text := call(t, cs, "get_graph", args)
		if !res.IsError {
			t.Errorf("%s: expected an error, got:\n%s", name, text)
			continue
		}
		if name != "a word, not 'all'" && !strings.Contains(text, "extended-by") && !strings.Contains(text, "inheritance") {
			t.Errorf("%s: the error should name the allowed relations: %s", name, text)
		}
	}
	if _, text := call(t, cs, "get_graph", map[string]any{"around": "N0", "follow": []string{"extended-by", "calls"}, "depth": "all"}); !strings.Contains(text, "calls") {
		t.Errorf("the error should name the offending relation: %s", text)
	}
}
