package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSetRelationVisibleTakesAListAllOrNothing(t *testing.T) {
	cs, _ := mcpSession(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "set_relation_visible" {
			continue
		}
		b, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Required   []string       `json:"required"`
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		if _, old := schema.Properties["relation"]; old || schema.Properties["relations"] == nil || schema.Properties["types"] == nil {
			t.Fatalf("the single `relation` parameter must be gone, `relations` and `types` there: %s", b)
		}
		if !strings.Contains(strings.Join(schema.Required, ","), "visible") {
			t.Fatalf("visible is required: %s", b)
		}
	}
	// exactly one of relations or types
	r0, text0 := call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "visible": false})
	if !r0.IsError || !strings.Contains(text0, "exactly one of relations") {
		t.Fatalf("neither: %v %s", r0.IsError, text0)
	}
	r0, text0 = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{"r_a_b_implements"}, "types": []string{"implements"}, "visible": false})
	if !r0.IsError || !strings.Contains(text0, "exactly one of relations") {
		t.Fatalf("both: %v %s", r0.IsError, text0)
	}

	// an unknown id refuses the whole call and names it
	r, text := call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{"r_a_b_implements", "r_nope"}, "visible": false})
	if !r.IsError || !strings.Contains(text, "r_nope") {
		t.Fatalf("unknown id: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{}, "visible": false})
	if !r.IsError || !strings.Contains(text, "exactly one") {
		t.Fatalf("empty list: %v %s", r.IsError, text)
	}

	// the view places nothing: the relation is set, and the answer says it is not drawn anyway
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{"r_a_b_implements"}, "visible": false})
	if r.IsError || !strings.Contains(text, "1 relation(s)") || !strings.Contains(text, "0 already") || !strings.Contains(text, "not drawn on this view anyway") || !strings.Contains(text, "r_a_b_implements") || !strings.Contains(text, "not saved") {
		t.Fatalf("answer: %v %s", r.IsError, text)
	}
	// asking the same again: nothing to change, said so (the unsaved edit is in the journal)
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{"r_a_b_implements"}, "visible": false})
	if r.IsError || !strings.Contains(text, "nothing to change") {
		t.Fatalf("already so: %v %s", r.IsError, text)
	}
}

// `types` is a one-time expansion to the relations of those types that are on
// the view now, then the same write as a list of ids: no stored shape, and zero
// matches is an answer, not an error.
func TestTypesSelectorExpandsOnceForVisibilityAndRouting(t *testing.T) {
	cs, _ := mcpSession(t)
	// nothing is placed yet: nothing matches
	r, text := call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements"}, "visible": false})
	if r.IsError || !strings.Contains(text, "no relation of type implements has both ends placed") {
		t.Fatalf("zero matches: %v %s", r.IsError, text)
	}
	if r, text := call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 0, "y": 0}, map[string]any{"entity": "e_b", "x": 300, "y": 0}}}); r.IsError {
		t.Fatal(text)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements", "nothing"}, "visible": false})
	if r.IsError || !strings.Contains(text, "1 relation(s) of type implements, nothing on v_main now visible=false") || !strings.Contains(text, "added to the registry later follow the view's defaults") {
		t.Fatalf("hide a type: %v %s", r.IsError, text)
	}
	_, text = call(t, cs, "get_view", map[string]any{"view": "v_main", "hidden": true})
	if !strings.Contains(text, `"except":["r_a_b_implements"]`) || strings.Contains(text, `"types"`) {
		t.Fatalf("the plain except list, no new shape: %s", text)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements"}, "visible": false})
	if r.IsError || !strings.Contains(text, "nothing to change") {
		t.Fatalf("again: %v %s", r.IsError, text)
	}

	// the shape: a human's call, the per-line overlay, expansion once
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"implements"}, "routing": "orthogonal"})
	if !r.IsError {
		t.Fatalf("set_routing needs requestedByHuman: %s", text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"implements"}, "relations": []string{"r_a_b_implements"}, "routing": "orthogonal", "requestedByHuman": true})
	if !r.IsError {
		t.Fatalf("types and relations together: %s", text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"implements"}, "routing": "orthogonal", "requestedByHuman": true})
	if r.IsError || !strings.Contains(text, "the routing of 1 line(s) of type implements set to orthogonal") || !strings.Contains(text, "one-time") {
		t.Fatalf("routing by type: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"nothing"}, "routing": "orthogonal", "requestedByHuman": true})
	if r.IsError || !strings.Contains(text, "nothing changed") {
		t.Fatalf("zero matches for routing: %v %s", r.IsError, text)
	}
}

// `list: true` returns the relations acted on, in full, with ends and type;
// without it there is no such block; zero matches with it is an empty list.
func TestListReturnsTheRelationsActedOn(t *testing.T) {
	cs, _ := mcpSession(t)
	r, text := call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements"}, "visible": false, "list": true})
	if r.IsError || !strings.Contains(text, "changed (0):") || !strings.Contains(text, "already (0):") {
		t.Fatalf("zero matches with list: %v %s", r.IsError, text)
	}
	if r, text := call(t, cs, "place_entities", map[string]any{"view": "v_main", "requestedByHuman": true,
		"entities": []any{map[string]any{"entity": "e_a", "x": 0, "y": 0}, map[string]any{"entity": "e_b", "x": 300, "y": 0}}}); r.IsError {
		t.Fatal(text)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements"}, "visible": false, "list": true})
	if r.IsError || !strings.Contains(text, "changed (1):\nr_a_b_implements  e_a -> e_b  implements") || !strings.Contains(text, "already (0):") {
		t.Fatalf("changed listed: %v %s", r.IsError, text)
	}
	sc, _ := json.Marshal(r.StructuredContent)
	var got struct {
		Changed []map[string]string `json:"changed"`
		Already []map[string]string `json:"already"`
	}
	if err := json.Unmarshal(sc, &got); err != nil || len(got.Changed) != 1 || len(got.Already) != 0 ||
		got.Changed[0]["id"] != "r_a_b_implements" || got.Changed[0]["from"] != "e_a" || got.Changed[0]["to"] != "e_b" || got.Changed[0]["type"] != "implements" {
		t.Fatalf("structured: %s", sc)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "relations": []string{"r_a_b_implements"}, "visible": false, "list": true})
	if r.IsError || !strings.Contains(text, "changed (0):") || !strings.Contains(text, "already (1):\nr_a_b_implements  e_a -> e_b  implements") {
		t.Fatalf("already listed: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_relation_visible", map[string]any{"view": "v_main", "types": []string{"implements"}, "visible": false})
	if r.IsError || strings.Contains(text, "already (") || strings.Contains(text, " -> ") {
		t.Fatalf("no list, no block: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"implements"}, "routing": "orthogonal", "requestedByHuman": true, "list": true})
	if r.IsError || !strings.Contains(text, "written (1):\nr_a_b_implements  e_a -> e_b  implements") {
		t.Fatalf("routing list: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"nothing"}, "routing": "orthogonal", "requestedByHuman": true, "list": true})
	if r.IsError || !strings.Contains(text, "written (0):") {
		t.Fatalf("routing zero matches: %v %s", r.IsError, text)
	}
	r, text = call(t, cs, "set_routing", map[string]any{"view": "v_main", "types": []string{"implements"}, "routing": "bezier", "requestedByHuman": true})
	if r.IsError || strings.Contains(text, "written (") {
		t.Fatalf("routing without list: %v %s", r.IsError, text)
	}
}

func TestNameIDsCapsALongList(t *testing.T) {
	if got := nameIDs([]string{"a", "b"}, 5); got != "a, b" {
		t.Fatal(got)
	}
	if got := nameIDs([]string{"a", "b", "c", "d", "e", "f", "g"}, 5); got != "a, b, c, d, e and 2 more" {
		t.Fatal(got)
	}
}
