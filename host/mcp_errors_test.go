package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A call whose arguments do not fit is answered with the SDK's own message
// and, under it, the parameters the tool takes — required ones first.
func TestArgumentErrorListsAcceptedParameters(t *testing.T) {
	cs := narrowFixtureSession(t, "one")
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_view", Arguments: map[string]any{"items": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || len(res.Content) == 0 {
		t.Fatalf("expected a tool error, got %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "unexpected additional properties") {
		t.Fatalf("the SDK's own message is gone: %s", text)
	}
	if !strings.Contains(text, "get_view accepts: view (required), detail, hidden, project") {
		t.Fatalf("no list of accepted parameters: %s", text)
	}

	// An error of the tool itself is left alone.
	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_view", Arguments: map[string]any{"view": "no-such-view", "project": "p"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) > 0 && strings.Contains(res.Content[0].(*mcp.TextContent).Text, "accepts:") {
		t.Fatalf("a tool's own error must not get the parameter list: %s", res.Content[0].(*mcp.TextContent).Text)
	}
}

// The mental-model paragraph is in the standard and full instructions of both
// tool sets, the short form in brief, and names only tools the set has.
func TestInstructionsCarryTheMentalModel(t *testing.T) {
	for _, set := range []string{"one", "narrow"} {
		for _, level := range []string{"standard", "full"} {
			text := serverInstructions(set, level)
			for _, want := range []string{"three layers", "sync_preview", "`present`", "`missing`", "get_view", "get_relations"} {
				if !strings.Contains(text, want) {
					t.Errorf("%s/%s: instructions lack %q", set, level, want)
				}
			}
		}
		if brief := serverInstructions(set, "brief"); !strings.Contains(brief, "three layers") || strings.Contains(brief, "sync_preview") {
			t.Errorf("%s/brief: want the two-sentence form:\n%s", set, brief)
		}
	}
	if text := serverInstructions("narrow", "standard"); strings.Contains(text, "get_graph") || strings.Contains(text, "graph_formats") {
		t.Errorf("the narrow set has neither get_graph nor graph_formats:\n%s", text)
	}
	if text := serverInstructions("one", "standard"); !strings.Contains(text, "graph_formats") || !strings.Contains(text, "get_graph") {
		t.Errorf("the one set points to get_graph and graph_formats:\n%s", text)
	}
}
