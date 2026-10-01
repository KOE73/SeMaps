package core

import (
	"encoding/json"
	"testing"
)

func lookModel(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	return m
}

func lookFields(kv ...string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i]] = json.RawMessage(kv[i+1])
	}
	return out
}

func placementOf(t *testing.T, m *Model, id string) *object {
	t.Helper()
	doc, err := m.view("v_main")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range viewItems(doc, "placements") {
		if p.str("entity") == id {
			return p
		}
	}
	t.Fatalf("no placement %s", id)
	return nil
}

func TestSetPlacementFieldsAbsentAndNull(t *testing.T) {
	m := lookModel(t)
	if _, err := m.SetPlacement("v_main", []string{"e_a"}, lookFields("styleId", `"s"`), false, "agent"); err == nil {
		t.Fatal("written without requestedByHuman")
	}
	_, err := m.SetPlacement("v_main", []string{"e_a"}, lookFields(), true, "agent")
	refused(t, err, "give at least one")
	_, err = m.SetPlacement("v_main", []string{"e_a"}, lookFields("x", `1`), true, "agent")
	refused(t, err, `field "x"`)

	rep, err := m.SetPlacement("v_main", []string{"e_a"}, lookFields("styleId", `"class.detail"`, "override", `{"fill":"#fff"}`, "template", `"t"`), true, "agent")
	if err != nil || len(rep.Touched) != 1 || rep.Touched[0] != "v_main#e_a" {
		t.Fatalf("%+v %v", rep, err)
	}
	a := placementOf(t, m, "e_a")
	if a.str("styleId") != "class.detail" || a.str("template") != "t" || string(a.vals["override"]) != `{"fill":"#fff"}` || string(a.vals["x"]) != "10" {
		t.Fatalf("set: %s", objString(a))
	}
	// a field left out stays; null drops only that one
	if _, err := m.SetPlacement("v_main", []string{"e_a"}, lookFields("template", `null`), true, "agent"); err != nil {
		t.Fatal(err)
	}
	a = placementOf(t, m, "e_a")
	if has(a, "template") || a.str("styleId") != "class.detail" || !has(a, "override") {
		t.Fatalf("null: %s", objString(a))
	}
	if _, err := m.SetPlacement("v_main", []string{"e_a"}, lookFields("styleId", `null`, "override", `null`), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if a = placementOf(t, m, "e_a"); has(a, "styleId") || has(a, "override") || string(a.vals["y"]) != "20" {
		t.Fatalf("null both: %s", objString(a))
	}
	// the rules of the core
	_, err = m.SetPlacement("v_main", []string{"e_a"}, lookFields("override", `{"line":{"color":"#000"}}`), true, "agent")
	refused(t, err, "cannot be overridden")
	_, err = m.SetPlacement("v_main", []string{"e_a"}, lookFields("styleId", `""`), true, "agent")
	refused(t, err, "non-empty string")
	_, err = m.SetPlacement("v_main", []string{"e_a"}, lookFields("collapsed", `true`), true, "agent")
	refused(t, err, "not a container")
	_, err = m.SetPlacement("v_main", []string{"e_core"}, lookFields("collapsed", `"yes"`), true, "agent")
	refused(t, err, "true or false")
	if _, err := m.SetPlacement("v_main", []string{"e_core"}, lookFields("collapsed", `true`), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if c := placementOf(t, m, "e_core"); string(c.vals["collapsed"]) != "true" {
		t.Fatalf("collapsed: %s", objString(c))
	}
}

func TestSetRoutingViewAndRelation(t *testing.T) {
	m := lookModel(t)
	view := func() *object {
		d, err := m.view("v_main")
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	bad := "curvy"
	if _, err := m.SetRouting("v_main", &bad, nil, true, "agent"); err == nil {
		t.Fatal("bad routing accepted")
	}
	orth := "orthogonal"
	if _, err := m.SetRouting("v_main", &orth, nil, false, "agent"); err == nil {
		t.Fatal("written without requestedByHuman")
	}
	if _, err := m.SetRouting("v_main", &orth, nil, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if view().str("routing") != "orthogonal" {
		t.Fatalf("view routing: %s", objString(view()))
	}
	if _, err := m.SetRouting("v_main", nil, nil, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if has(view(), "routing") {
		t.Fatal("view routing not removed")
	}
	// relations: an unknown one is refused; one whose end is not placed is accepted and reported
	_, err := m.SetRouting("v_main", &orth, []string{"r_nope"}, true, "agent")
	refused(t, err, "no relation r_nope")
	rep, err := m.SetRouting("v_main", &orth, []string{"r_a_b_items_item"}, true, "agent")
	if err != nil || len(rep.NotDrawn) != 1 || rep.NotDrawn[0] != "r_a_b_items_item" || len(rep.Relations) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if got := string(view().vals["edges"]); got != `[{"id":"r_a_b_items_item","routing":"orthogonal"}]` {
		t.Fatalf("edges: %s", got)
	}
	// a look of its own keeps the entry when the routing goes; without one the entry and the key go
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_a_b_items_item","styleId":"call","routing":"bezier"}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetRouting("v_main", nil, []string{"r_a_b_items_item", "r_a_b_items_item"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := string(view().vals["edges"]); got != `[{"id":"r_a_b_items_item","styleId":"call"}]` {
		t.Fatalf("entry with a style: %s", got)
	}
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_a_b_items_item","routing":"bezier"}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetRouting("v_main", nil, []string{"r_a_b_items_item"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if has(view(), "edges") {
		t.Fatalf("empty edges stayed: %s", objString(view()))
	}
	// nothing to remove, nothing written
	if _, err := m.SetRouting("v_main", nil, []string{"r_a_b_items_item"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
}

// The value of a routing is checked whoever writes it (CONTRACT §11.3).
func TestRoutingValueIsARule(t *testing.T) {
	m := lookModel(t)
	_, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","routing":"curvy"}`)}, "human")
	refused(t, err, "routing")
	_, err = m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","routing":3}`)}, "human")
	refused(t, err, "routing")
	_, err = m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","edges":[{"id":"r_a_b_items_item","routing":"curvy"}]}`)}, "human")
	refused(t, err, "routing")
	if _, err := m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","routing":"tree-horizontal","edges":[{"id":"r_a_b_items_item","routing":"tree-vertical"}]}`)}, "human"); err != nil {
		t.Fatal(err)
	}
}
