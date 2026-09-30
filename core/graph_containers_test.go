package core

import (
	"reflect"
	"testing"
)

func nodeOf(t *testing.T, g *Graph, id string) GraphNode {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %s in %+v", id, g.Nodes)
	return GraphNode{}
}

// A node belongs to the innermost container that `contains` it; a container
// is a node whose kind is a container kind of the dictionary.
func TestBuildGraphContainersFromContains(t *testing.T) {
	m := graphModel(t, `{"entities":[]}`, `{"relations":[]}`)
	sym := func(id, kind, native string) Symbol {
		return Symbol{ID: id, Kind: kind, NativeKind: native, Name: id, File: "src/" + id + ".cs"}
	}
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{
		sym("Asm", "module", "assembly"),
		sym("Asm.Ns", "module", "namespace"),
		sym("Other", "module", "namespace"),
		sym("Asm.Ns.A", "type", "class"),
		sym("Asm.Ns.B", "type", "class"),       // contained by two unrelated namespaces
		sym("Asm.Ns.C", "type", "class"),       // contained by the assembly and the namespace inside it
		sym("Loose", "type", "class"),          // contained by nothing
		sym("Asm.Ns.A.Inner", "type", "class"), // nested in a class: a class is no container
	}, Edges: []Edge{
		{From: "Asm", To: "Asm.Ns", Kind: "contains"},
		{From: "Asm.Ns", To: "Asm.Ns.A", Kind: "contains"},
		{From: "Asm.Ns", To: "Asm.Ns.B", Kind: "contains"},
		{From: "Other", To: "Asm.Ns.B", Kind: "contains"},
		{From: "Asm", To: "Asm.Ns.C", Kind: "contains"},
		{From: "Asm.Ns", To: "Asm.Ns.C", Kind: "contains"},
		{From: "Asm.Ns.A", To: "Asm.Ns.A.Inner", Kind: "contains"},
	}}
	g, err := BuildGraph([]FactsSource{{Extractor: "cs", Facts: facts}}, m)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"cs:Asm":            nil,
		"cs:Asm.Ns":         {"cs:Asm"},
		"cs:Other":          nil,
		"cs:Asm.Ns.A":       {"cs:Asm.Ns"},
		"cs:Asm.Ns.B":       nil, // two unrelated containers: none, nothing is guessed
		"cs:Asm.Ns.C":       {"cs:Asm.Ns"},
		"cs:Loose":          nil,
		"cs:Asm.Ns.A.Inner": nil,
	}
	for id, containers := range want {
		if got := nodeOf(t, g, id).Containers; !reflect.DeepEqual(got, containers) {
			t.Errorf("%s: containers %v, want %v", id, got, containers)
		}
	}
	for id, isContainer := range map[string]bool{"cs:Asm": true, "cs:Asm.Ns": true, "cs:Other": true, "cs:Asm.Ns.A": false} {
		if got := nodeOf(t, g, id).Container; got != isContainer {
			t.Errorf("%s: container %v, want %v", id, got, isContainer)
		}
	}
	list := GraphContainers(g)
	if len(list) != 3 || list[0] != (GroupContainer{ID: "cs:Asm", Name: "Asm"}) || list[1].Parent != "cs:Asm" {
		t.Fatalf("GraphContainers = %+v", list)
	}
}

// A `contains` relation written by a person between entities counts the same,
// for a group entity — a container of no code at all.
func TestBuildGraphContainersFromAuthoredRelation(t *testing.T) {
	m := graphModel(t, `{"entities":[
		{"id":"e_grp","name":"Frontend","kind":"group","origin":"authored","status":"present"},
		{"id":"e_ui","name":"Ui","kind":"component","origin":"authored","status":"present"},
		{"id":"e_x","name":"X","kind":"class","origin":"code","status":"present","symbol":"A.X"}]}`,
		`{"relations":[
		{"id":"r_grp_ui_contains","from":"e_grp","to":"e_ui","type":"contains","origin":"authored"},
		{"id":"r_ui_x_contains","from":"e_ui","to":"e_x","type":"contains","origin":"authored"},
		{"id":"r_grp_x_contains","from":"e_grp","to":"e_x","type":"contains","origin":"authored"}]}`)
	facts := &Facts{Language: "csharp", Root: ".", Symbols: []Symbol{{ID: "A.X", Kind: "type", NativeKind: "class", Name: "X", File: "src/X.cs"}}}
	g, err := BuildGraph([]FactsSource{{Extractor: "cs", Facts: facts}}, m)
	if err != nil {
		t.Fatal(err)
	}
	if n := nodeOf(t, g, "e_grp"); !n.Container || len(n.Containers) != 0 {
		t.Errorf("e_grp = %+v", n)
	}
	// a component is no container kind: it does not hold e_x, so e_grp does
	if n := nodeOf(t, g, "e_ui"); n.Container || !reflect.DeepEqual(n.Containers, []string{"e_grp"}) {
		t.Errorf("e_ui = %+v", n)
	}
	if n := nodeOf(t, g, "cs:A.X"); !reflect.DeepEqual(n.Containers, []string{"e_grp"}) {
		t.Errorf("A.X = %+v", n)
	}
}

// Nothing is a container without the dictionary, and a cycle of containers
// ends in none, not in a loop.
func TestAssignContainersCycleAndNoDictionary(t *testing.T) {
	nodes := []GraphNode{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	edges := []GraphEdge{
		{From: "a", To: "b", Kind: "contains"},
		{From: "b", To: "a", Kind: "contains"},
		{From: "a", To: "c", Kind: "contains"},
	}
	kindOf := map[string]string{"a": "group", "b": "group", "c": "class"}
	catalog, err := ParseKinds([]byte(`{"groups":[{"id":"g","name":{"ru":"г"},"kinds":[{"id":"group","name":{"ru":"г"},"container":true}]}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	assignContainers(nodes, edges, kindOf, catalog)
	// each of the two has exactly one container; the loop does not make it none, nor hang
	if !reflect.DeepEqual(nodes[0].Containers, []string{"b"}) || !reflect.DeepEqual(nodes[1].Containers, []string{"a"}) {
		t.Errorf("a cycle: %+v", nodes)
	}
	if !reflect.DeepEqual(nodes[2].Containers, []string{"a"}) {
		t.Errorf("c is in a: %+v", nodes[2])
	}
	plain := []GraphNode{{ID: "a"}, {ID: "c"}}
	assignContainers(plain, edges[2:], kindOf, nil)
	if plain[0].Container || len(plain[1].Containers) != 0 {
		t.Errorf("without a dictionary nothing is a container: %+v", plain)
	}
}
