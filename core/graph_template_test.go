package core

import (
	"strings"
	"testing"
)

func TestTemplateEveryNodeMacro(t *testing.T) {
	step := 2
	n := GraphNode{
		ID: "id1", Symbol: "App.A", Name: "A", Namespace: "App", Assembly: "App.dll",
		Kind: "type", NativeKind: "class", Visibility: "public",
		File: "src/A.cs", Line: 3, EndLine: 9, Containers: []string{"c1", "c2"},
		Presence: "both", Status: "present", Entity: "e_a", Step: &step,
	}
	m := NodeMacrosOf(&n)
	tpl := MustTemplate("{step}|{name}|{fullName}|{id}|{kind}|{nativeKind}|{visibility}|{file}|{line}|{endLine}|{lines}|{namespace}|{assembly}|{containers}|{presence}|{status}|{entity}")
	got := tpl.Render(m)
	want := "2|A|App.A|id1|type|class|public|src/A.cs|3|9|3-9|App|App.dll|c1, c2|both|present|e_a"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTemplateEmptyMacroPrintsNothing(t *testing.T) {
	n := GraphNode{ID: "id1", Name: "A"}
	tpl := MustTemplate("A={name} B=[{namespace}]")
	got := tpl.Render(NodeMacrosOf(&n))
	if got != "A=A B=" {
		t.Fatalf("got %q", got)
	}
}

func TestTemplateOptionalGroupDropsAttachedLiteral(t *testing.T) {
	// the exact case named by part 3: `holds Context:7` and a bare `extends`
	// must both come out clean from `[:{memberLine}]`.
	tpl := MustTemplate("{relation}[:{memberLine}]")
	withLine := tpl.Render(NodeMacros{Relations: nil})
	_ = withLine
	// render as a relation directly via the relations block wrapper. The
	// template text on either side of `|` is taken exactly as written (no
	// trimming), so a leading/trailing space there is the author's choice.
	full := MustTemplate("{relations:{relation}[:{memberLine}]|; }")
	m := NodeMacros{Relations: []RelationMacros{
		{Relation: "holds", MemberLine: 7},
		{Relation: "extends"},
	}}
	got := full.Render(m)
	if got != "holds:7; extends" {
		t.Fatalf("got %q", got)
	}
}

func TestTemplateRelationsBlockJoinsWithSeparator(t *testing.T) {
	tpl := MustTemplate("{relations:{relation} {member}|; }")
	m := NodeMacros{Relations: []RelationMacros{
		{Relation: "holds", Member: "Log"},
		{Relation: "extends"},
	}}
	got := tpl.Render(m)
	if got != "holds Log; extends " {
		t.Fatalf("got %q", got)
	}
}

func TestTemplateRelationsBlockEmptyWhenNoRelations(t *testing.T) {
	tpl := MustTemplate("x[{relations: {relation} | ; }]y")
	got := tpl.Render(NodeMacros{})
	if got != "xy" {
		t.Fatalf("expected the optional group around an empty relations block dropped, got %q", got)
	}
}

func TestTemplateMalformedUnmatchedBrace(t *testing.T) {
	_, err := ParseTemplate("{name")
	if err == nil || !strings.Contains(err.Error(), "position") {
		t.Fatalf("expected a position-naming error, got %v", err)
	}
}

func TestTemplateMalformedUnmatchedBracket(t *testing.T) {
	_, err := ParseTemplate("[{name}")
	if err == nil {
		t.Fatal("expected an error for an unmatched '['")
	}
}

func TestTemplateMalformedUnknownMacro(t *testing.T) {
	_, err := ParseTemplate("{bogus}")
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected an error naming the unknown macro, got %v", err)
	}
}

func TestTemplateUnmatchedClosingBracket(t *testing.T) {
	_, err := ParseTemplate("{name}]")
	if err == nil {
		t.Fatal("expected an error for a stray ']'")
	}
}
