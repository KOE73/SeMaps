package main

import (
	"context"
	"strings"
	"testing"
)

// ADR_20261001: `remove` takes authored records out of the registry, only for a
// human, with what views and relations hold of them.
func TestMCPRemoveAuthoredRecords(t *testing.T) {
	cs, _ := mcpSession(t)
	if res, err := cs.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	} else {
		found := false
		for _, tl := range res.Tools {
			found = found || tl.Name == "remove"
		}
		if !found {
			t.Fatal("no tool remove")
		}
	}
	call(t, cs, "add_entity", map[string]any{"id": "e_x", "name": "X", "kind": "app"})
	call(t, cs, "add_relation", map[string]any{"from": "e_x", "to": "e_a", "type": "call"})
	call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_x", "x": 1, "y": 1}}})
	call(t, cs, "save", map[string]any{"requestedByHuman": true})

	// only for a human
	r, text := call(t, cs, "remove", map[string]any{"ids": []string{"r_x_a_call"}, "requestedByHuman": false})
	if !r.IsError || !strings.Contains(text, "requires requestedByHuman") {
		t.Fatalf("without a human: %v %s", r.IsError, text)
	}
	// a record from code is refused
	r, text = call(t, cs, "remove", map[string]any{"ids": []string{"r_a_b_implements"}, "requestedByHuman": true})
	if !r.IsError || !strings.Contains(text, "come from code") {
		t.Fatalf("code relation: %v %s", r.IsError, text)
	}
	// an entity that still has a relation: refused, the relation named
	r, text = call(t, cs, "remove", map[string]any{"ids": []string{"e_x"}, "requestedByHuman": true})
	if !r.IsError || !strings.Contains(text, "r_x_a_call") {
		t.Fatalf("no cascade: %v %s", r.IsError, text)
	}
	// with cascade: entity, relation and placement go, nothing is saved
	r, text = call(t, cs, "remove", map[string]any{"ids": []string{"e_x"}, "cascade": true, "requestedByHuman": true})
	if r.IsError || !strings.Contains(text, "e_x") || !strings.Contains(text, "r_x_a_call") || !strings.Contains(text, "v_main#e_x") || !strings.Contains(text, "not saved") {
		t.Fatalf("cascade: %v %s", r.IsError, text)
	}
	_, text = call(t, cs, "get_entity", map[string]any{"id": "e_x"})
	if strings.Contains(text, `"id":"e_x"`) {
		t.Fatalf("e_x still there: %s", text)
	}
	// discard brings it back
	call(t, cs, "discard", map[string]any{"scope": "all", "requestedByHuman": true})
	_, text = call(t, cs, "get_entity", map[string]any{"id": "e_x"})
	if !strings.Contains(text, `"e_x"`) {
		t.Fatalf("discard did not restore e_x: %s", text)
	}
}
