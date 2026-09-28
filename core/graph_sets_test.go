package core

import "testing"

func TestRelationSetsListedAndDefault(t *testing.T) {
	sets := RelationSets()
	want := map[string][]string{
		"links":       {"extends", "implements", "holds", "uses", "depends"},
		"inheritance": {"extends", "implements"},
		"members":     {"holds", "uses"},
		"containment": {"contains"},
		"all":         {"extends", "implements", "holds", "uses", "depends", "contains"},
	}
	if len(sets) != len(want) {
		t.Fatalf("expected %d sets, got %d: %+v", len(want), len(sets), sets)
	}
	for _, s := range sets {
		wk, ok := want[s.Name]
		if !ok {
			t.Fatalf("unexpected set %q", s.Name)
		}
		if len(s.Kinds) != len(wk) {
			t.Fatalf("set %q: expected kinds %v, got %v", s.Name, wk, s.Kinds)
		}
		for i := range wk {
			if s.Kinds[i] != wk[i] {
				t.Fatalf("set %q: expected kinds %v, got %v", s.Name, wk, s.Kinds)
			}
		}
		if s.Description == "" {
			t.Fatalf("set %q has no description", s.Name)
		}
	}
	if DefaultNeighborhoodSet != "links" {
		t.Fatalf("expected links as the neighbourhood default, got %q", DefaultNeighborhoodSet)
	}
	if DefaultWholeGraphSet != "all" {
		t.Fatalf("expected all as the whole-graph default, got %q", DefaultWholeGraphSet)
	}
}

func TestResolveEdgeKindsDefaults(t *testing.T) {
	// whole graph, nothing given: nil (no filtering at all) — the shortcut
	// that keeps a plain GET /api/graph/{project} byte-identical to before
	// named sets existed.
	kinds, err := ResolveEdgeKinds("", nil, false)
	if err != nil || kinds != nil {
		t.Fatalf("expected no filtering for a whole-graph request with nothing given, got %v, %v", kinds, err)
	}
	// neighbourhood, nothing given: "links", so `contains` is not walked.
	kinds2, err := ResolveEdgeKinds("", nil, true)
	if err != nil {
		t.Fatalf("ResolveEdgeKinds: %v", err)
	}
	for _, k := range kinds2 {
		if k == "contains" {
			t.Fatalf("default neighbourhood set must not include contains: %v", kinds2)
		}
	}
}

func TestResolveEdgeKindsSetAndKindsConflict(t *testing.T) {
	if _, err := ResolveEdgeKinds("links", []string{"extends"}, false); err == nil {
		t.Fatal("expected an error naming the conflict when both set and kinds are given")
	}
}

func TestResolveEdgeKindsUnknownSet(t *testing.T) {
	if _, err := ResolveEdgeKinds("bogus", nil, false); err == nil {
		t.Fatal("expected an error for an unknown set name")
	}
}

func TestResolveEdgeKindsExplicitKindsUsedAsIs(t *testing.T) {
	kinds, err := ResolveEdgeKinds("", []string{"contains"}, true)
	if err != nil || len(kinds) != 1 || kinds[0] != "contains" {
		t.Fatalf("expected explicit kinds to pass through unchanged, got %v, %v", kinds, err)
	}
}

func TestResolveEdgeKindsNamedSetOverridesNeighbourhoodDefault(t *testing.T) {
	kinds, err := ResolveEdgeKinds("containment", nil, true)
	if err != nil || len(kinds) != 1 || kinds[0] != "contains" {
		t.Fatalf("expected set=containment to resolve to [contains], got %v, %v", kinds, err)
	}
}
