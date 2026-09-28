package core

import "testing"

func TestRelationVocabularyEveryRowHasItsInverse(t *testing.T) {
	for _, r := range RelationVocabulary() {
		inv, ok := GetRelation(r.Inverse)
		if !ok {
			t.Fatalf("relation %q names inverse %q, which does not exist", r.Name, r.Inverse)
		}
		if inv.Inverse != r.Name {
			t.Fatalf("relation %q <-> %q inverse is not symmetric", r.Name, r.Inverse)
		}
		if inv.Kind != r.Kind || inv.TypeMatch != r.TypeMatch {
			t.Fatalf("relation %q and its inverse %q must share Kind/TypeMatch, got %+v / %+v", r.Name, r.Inverse, r, inv)
		}
		if inv.Forward == r.Forward {
			t.Fatalf("relation %q and its inverse %q must have opposite Forward", r.Name, r.Inverse)
		}
	}
}

func TestParseFollowUnknownName(t *testing.T) {
	if _, err := ParseFollow([]string{"bogus"}); err == nil {
		t.Fatal("expected an error for an unknown follow name")
	}
}

func TestParseFollowEmptyIsDefault(t *testing.T) {
	rs, err := ParseFollow(nil)
	if err != nil {
		t.Fatalf("ParseFollow: %v", err)
	}
	if len(rs) != len(DefaultFollow) {
		t.Fatalf("expected DefaultFollow, got %d entries", len(rs))
	}
	for _, r := range rs {
		if r.Kind == "contains" {
			t.Fatalf("DefaultFollow must not include containment, got %+v", r)
		}
	}
}

func TestUsesExcludesInjects(t *testing.T) {
	e := GraphEdge{From: "a", To: "b", Kind: "uses", Type: "injects"}
	uses, _ := GetRelation("uses")
	if typeMatches(e.Type, uses.TypeMatch) {
		t.Fatalf("an injects edge must not satisfy the plain 'uses' relation")
	}
	injects, _ := GetRelation("injects")
	if !typeMatches(e.Type, injects.TypeMatch) {
		t.Fatalf("an injects edge must satisfy 'injects'")
	}
}

func TestHoldsCoversEveryCardinality(t *testing.T) {
	holds, _ := GetRelation("holds")
	for _, ty := range []string{"holds.one", "holds.many", "holds.many.ro", "holds.optional", "holds.keyed"} {
		if !typeMatches(ty, holds.TypeMatch) {
			t.Fatalf("unqualified 'holds' must cover %q", ty)
		}
	}
}
