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

// TestTemplateEscapedBrackets: `\[`/`\]` are literal brackets (defect 4d) —
// needed because `[`/`]` are the optional-group syntax, and `facts` wraps
// its relations in real ones.
func TestTemplateEscapedBrackets(t *testing.T) {
	tpl := MustTemplate(`\[{name}\]`)
	got := tpl.Render(NodeMacrosOf(&GraphNode{Name: "A"}))
	if got != "[A]" {
		t.Fatalf("got %q, want %q", got, "[A]")
	}
}

// TestTemplateEscapedBracketsAroundOptionalGroup: an escaped bracket right
// next to a real (unescaped) optional group — the escape must not confuse
// the parser about which `]` closes the real group.
func TestTemplateEscapedBracketsAroundOptionalGroup(t *testing.T) {
	tpl := MustTemplate(`[\[{relations:{relation}|; }\]]`)
	m := NodeMacros{Relations: []RelationMacros{{Relation: "extends"}}}
	got := tpl.Render(m)
	if got != "[extends]" {
		t.Fatalf("got %q, want %q", got, "[extends]")
	}
	empty := tpl.Render(NodeMacros{})
	if empty != "" {
		t.Fatalf("expected the escaped brackets, wrapped in a real optional group, to vanish with the empty relations block, got %q", empty)
	}
}

// TestTemplateEscapedBraces: `\{`/`\}` are literal braces (PLAN_20260928-7
// step 6) — needed because `{...}` is the macro syntax, and the default
// templates wrap `{dynamic}` in real ones.
func TestTemplateEscapedBraces(t *testing.T) {
	tpl := MustTemplate(`\{{name}\}`)
	got := tpl.Render(NodeMacrosOf(&GraphNode{Name: "A"}))
	if got != "{A}" {
		t.Fatalf("got %q, want %q", got, "{A}")
	}
}

// TestTemplateEscapedBracesAroundOptionalGroup: an escaped brace right next
// to a real macro, wrapped in an optional group — vanishes whole when the
// macro is empty, same as the bracket case.
func TestTemplateEscapedBracesAroundOptionalGroup(t *testing.T) {
	tpl := MustTemplate(`[\{dynamic: {dynamic}\}]`)
	n := &GraphNode{Dynamic: []GraphDynamicMark{{Kind: "create", Line: 57}}}
	got := tpl.Render(NodeMacrosOf(n))
	if got != "{dynamic: create @57}" {
		t.Fatalf("got %q, want %q", got, "{dynamic: create @57}")
	}
	empty := tpl.Render(NodeMacrosOf(&GraphNode{}))
	if empty != "" {
		t.Fatalf("expected the escaped braces to vanish with no marks, got %q", empty)
	}
}

// TestTemplateDynamicMacroMethod: on a method node, {dynamic} renders the
// method's own marks, no "in ...".
func TestTemplateDynamicMacroMethod(t *testing.T) {
	n := &GraphNode{Kind: "method", Dynamic: []GraphDynamicMark{{Kind: "create", Line: 57}, {Kind: "make-type", Line: 55}}}
	tpl := MustTemplate("{dynamic}")
	got := tpl.Render(NodeMacrosOf(n))
	want := "create @57; make-type @55"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTemplateDynamicMacroType: on a type node (after lifting), {dynamic}
// names each mark's method.
func TestTemplateDynamicMacroType(t *testing.T) {
	n := &GraphNode{Kind: "type", Dynamic: []GraphDynamicMark{
		{Kind: "make-type", Line: 55, Method: "CreateRunner"},
		{Kind: "create", Line: 57, Method: "CreateRunner"},
	}}
	tpl := MustTemplate("{dynamic}")
	got := tpl.Render(NodeMacrosOf(n))
	want := "make-type @55 in CreateRunner; create @57 in CreateRunner"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTemplateDynamicMacroCap: {dynamic} is capped by NodeMacros.ListCap,
// then "+N", same rule as {fromMethods}/{toMethods}.
func TestTemplateDynamicMacroCap(t *testing.T) {
	n := &GraphNode{Kind: "method", Dynamic: []GraphDynamicMark{
		{Kind: "create", Line: 1}, {Kind: "create", Line: 2}, {Kind: "create", Line: 3},
	}}
	m := NodeMacrosOf(n)
	m.ListCap = 2
	tpl := MustTemplate("{dynamic}")
	got := tpl.Render(m)
	want := "create @1; create @2; +1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTemplateWhitespaceRuleDropsLeadingSpaceOfEmptyMacro: a leading
// "{step} " with no step must not leave a stray space before what follows
// (defect 4/5).
func TestTemplateWhitespaceRuleDropsLeadingSpaceOfEmptyMacro(t *testing.T) {
	tpl := MustTemplate("{step} {name}")
	got := tpl.Render(NodeMacrosOf(&GraphNode{Name: "A"})) // no Step
	if got != "A" {
		t.Fatalf("got %q, want %q (leading space of an empty {step} must be dropped)", got, "A")
	}
}

// TestTemplateWhitespaceRuleDropsTrailingSpaceOfDroppedGroup: a trailing
// "  [...]" whose group turns out empty must leave no trailing space.
func TestTemplateWhitespaceRuleDropsTrailingSpaceOfDroppedGroup(t *testing.T) {
	tpl := MustTemplate("{name}  [{namespace}]")
	got := tpl.Render(NodeMacrosOf(&GraphNode{Name: "A"})) // no Namespace
	if got != "A" {
		t.Fatalf("got %q, want %q (trailing space before a dropped group must be dropped)", got, "A")
	}
	if strings.HasSuffix(got, " ") {
		t.Fatalf("got a trailing space: %q", got)
	}
}

// TestTemplateWhitespaceRuleKeepsSandwichedSpace: a space that has real
// content on BOTH sides (even if the atom immediately touching it on one
// side is empty) is kept exactly as written — this is what keeps `facts`'
// deliberate double space between name and position, and what a `lines`
// relation clause needs when its own optional member group is empty but a
// neighbour's name still follows.
func TestTemplateWhitespaceRuleKeepsSandwichedSpace(t *testing.T) {
	tpl := MustTemplate("{relation}[: {member}]  {name}")
	m := NodeMacros{Name: "Nb"}
	rel := RelationMacros{Relation: "extends"} // Member empty: the "[: {member}]" group drops
	got := tpl.RenderRelation(m, &rel)
	if got != "extends  Nb" {
		t.Fatalf("got %q, want %q (the two spaces must survive even though the group between them is empty)", got, "extends  Nb")
	}
}

// TestTemplateWhitespaceRuleNeverTouchesTabs: a tab is not the space
// character, so it is never dropped by the whitespace rule, even next to an
// empty macro — this is what keeps `locations`' column count fixed.
func TestTemplateWhitespaceRuleNeverTouchesTabs(t *testing.T) {
	tpl := MustTemplate("{fullName}\t{file}\t{line}\t{endLine}")
	got := tpl.Render(NodeMacrosOf(&GraphNode{Name: "B"})) // no File/Line/EndLine
	want := "B\t\t\t"
	if got != want {
		t.Fatalf("got %q, want %q (tabs must survive even with every later macro empty)", got, want)
	}
}

// TestTemplateViaOnlyFromStepTwo: {via} is empty unless NodeMacros.Via was
// explicitly set (defect 6: the default `facts` template only ever sets it
// at step >= 2 — this test is the macro's own contract, independent of that
// population rule, which core/graph_format_text.go's renderPerNodeTemplate
// tests separately).
func TestTemplateViaOnlyFromStepTwo(t *testing.T) {
	tpl := MustTemplate("{name}[ via {via}]")
	withVia := NodeMacrosOf(&GraphNode{Name: "A"})
	withVia.Via = "Base"
	if got := tpl.Render(withVia); got != "A via Base" {
		t.Fatalf("got %q, want %q", got, "A via Base")
	}
	withoutVia := NodeMacrosOf(&GraphNode{Name: "A"})
	if got := tpl.Render(withoutVia); got != "A" {
		t.Fatalf("got %q, want %q (no stray ' via' with Via unset)", got, "A")
	}
}
