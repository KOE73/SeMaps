// The template engine behind every text graph format (part 3 of the
// agent-answers rework): one small language, so a named format is nothing
// but a stored template string and an agent can write its own after reading
// the macro dictionary graph_formats returns (docs/API.md §5/§6).
//
// Grammar, in full (this is also what graph_formats explains):
//   - literal text is copied as is, subject to the whitespace rule below.
//   - {macro} is replaced by that macro's value, or by nothing when the
//     macro has no value for the node/relation being rendered.
//   - [...] is an optional group: rendered as is when every macro directly
//     inside it has a value, dropped whole (literal text included) when any
//     one of them is empty. This is the one rule that keeps `holds Context:7`
//     and a bare `extends` both clean: wrap the part that depends on a
//     macro, e.g. `[:{memberLine}]`.
//   - `\[` and `\]` are literal `[`/`]`: since `[`/`]` are the optional-group
//     syntax, a template that wants a literal bracket (the `facts` format
//     wraps its relations in `[...]`) must escape it.
//   - {relations: TEMPLATE | SEPARATOR} renders TEMPLATE once per relation
//     reaching the node from the node it was reached from, joined by the
//     literal SEPARATOR. TEMPLATE may itself use [...] groups.
//
// Whitespace rule (defect 4/5 of the agent-answers-graph task): after
// rendering, a run made ONLY of the space character ' ' that sits directly
// next to an empty macro or a dropped optional group (on either side) is
// removed together with it; a run of spaces the template author wrote
// between two parts that BOTH have a value is kept exactly as written — this
// is what lets `facts` print the deliberate two spaces between name,
// position and relations, while still cleaning up `[{modifiers}]` when there
// are none. A tab or newline is never touched by this rule (so `locations`'
// tab-separated columns stay put even when a value is empty), and a run of
// spaces at the very end of a rendered piece, with nothing non-empty after
// it, is always dropped (no trailing spaces). The rule applies once, in the
// engine, to every stored and custom template alike — never per format.
//
// A malformed template (unmatched `{`/`[`/`]`, or an unknown macro name) is
// a parse error naming the position and the macro.
package core

import (
	"fmt"
	"strconv"
	"strings"
)

// NodeMacros is everything the {…} macros of a node-level template can show.
// Built by a formatter from a GraphNode plus, when the node was reached in a
// walk, the relations that reached it.
type NodeMacros struct {
	Step                     *int
	Name, FullName, ID       string
	Kind, NativeKind         string
	Visibility               string
	File                     string
	Line, EndLine            int
	Namespace, Assembly      string
	Containers               []string
	Presence, Status, Entity string
	Relations                []RelationMacros
	// Via, ViaFullName: short names / full names of the node(s) this node was
	// reached from, comma-separated, sorted, de-duplicated — only at step >=
	// 2 (defect 6): at step 0 the node is the focus, and at step 1 it was
	// reached from the focus itself, which the line already names. Left ""
	// at every other step, so the {via}/{viaFullName} macros are empty there.
	Via, ViaFullName string
}

// RelationMacros is one edge between a node and the node it was reached
// from, as the nested {relations: …} template sees it. `Relation` is the
// directed name from the point of view of the node it was reached FROM
// (core/graph_relations.go's edgeRelationNames, applied to that node).
type RelationMacros struct {
	Relation                string
	Member, MemberKind      string
	MemberLine              int
	Modifiers               []string
	Cardinality, Type, Text string
	RelationLine            int
	RelationLines           []int
	// RelationLinesFile: set only when the call/construct sites named by
	// RelationLines are NOT in the file already printed on this line (defect
	// B, "places of calls") — the file of the node the relation starts FROM
	// (e.From, whichever of the two nodes on the line that is), computed by
	// relationMacrosFor. Empty when the sites are in the same file as the
	// node this relation entry is about (nothing extra to say).
	RelationLinesFile string
	// Count, FromMethods, ToMethods mirror GraphEdge's lifted-edge fields
	// (core/graph_lift.go): set only for an edge LiftToTypes produced from
	// one or more method-level edges.
	Count                  int
	FromMethods, ToMethods []string
	// Injected: true when this relation is a `holds`/`held-by` that
	// combineHoldsAndInjects folded the matching `injects`/`injected-into` of
	// the same member of the same pair into (defect 3/2b). Rendered by the
	// {injected} macro as the literal "(injected)", empty otherwise — kept as
	// a flag rather than baked into Relation, so a template controls its own
	// placement (the default `facts` template puts it at the end of the
	// relation clause, not right after the relation word).
	Injected bool
	// other: the id at the far end of the edge from the node this relation
	// list belongs to. Not a macro — used only to pair a holds/injects
	// relation with the same neighbour (core/graph_format_text.go's
	// combineHoldsAndInjects).
	other string
}

// NodeMacrosOf builds the plain (no relations) macro set of a node.
func NodeMacrosOf(n *GraphNode) NodeMacros {
	return NodeMacros{
		Step: n.Step, Name: n.Name, FullName: FullName(n), ID: n.ID,
		Kind: n.Kind, NativeKind: n.NativeKind, Visibility: n.Visibility,
		File: n.File, Line: n.Line, EndLine: n.EndLine,
		Namespace: n.Namespace, Assembly: n.Assembly, Containers: n.Containers,
		Presence: n.Presence, Status: n.Status, Entity: n.Entity,
	}
}

func macroValue(m NodeMacros, name string) (string, bool) {
	switch name {
	case "step":
		if m.Step == nil {
			return "", false
		}
		return strconv.Itoa(*m.Step), true
	case "name":
		return m.Name, m.Name != ""
	case "fullName":
		return m.FullName, m.FullName != ""
	case "id":
		return m.ID, m.ID != ""
	case "kind":
		return m.Kind, m.Kind != ""
	case "nativeKind":
		return m.NativeKind, m.NativeKind != ""
	case "visibility":
		return m.Visibility, m.Visibility != ""
	case "file":
		return m.File, m.File != ""
	case "line":
		if m.Line == 0 {
			return "", false
		}
		return strconv.Itoa(m.Line), true
	case "endLine":
		if m.EndLine == 0 {
			return "", false
		}
		return strconv.Itoa(m.EndLine), true
	case "lines":
		return macroLines(m.Line, m.EndLine)
	case "namespace":
		return m.Namespace, m.Namespace != ""
	case "assembly":
		return m.Assembly, m.Assembly != ""
	case "containers":
		return strings.Join(m.Containers, ", "), len(m.Containers) > 0
	case "presence":
		return m.Presence, m.Presence != ""
	case "status":
		return m.Status, m.Status != ""
	case "entity":
		return m.Entity, m.Entity != ""
	case "via":
		return m.Via, m.Via != ""
	case "viaFullName":
		return m.ViaFullName, m.ViaFullName != ""
	}
	return "", false
}

func macroLines(line, endLine int) (string, bool) {
	if line == 0 {
		return "", false
	}
	if endLine != 0 && endLine != line {
		return fmt.Sprintf("%d-%d", line, endLine), true
	}
	return strconv.Itoa(line), true
}

func relationMacroValue(r RelationMacros, name string) (string, bool) {
	switch name {
	case "relation":
		return r.Relation, r.Relation != ""
	case "member":
		return r.Member, r.Member != ""
	case "memberKind":
		return r.MemberKind, r.MemberKind != ""
	case "memberLine":
		if r.MemberLine == 0 {
			return "", false
		}
		return strconv.Itoa(r.MemberLine), true
	case "modifiers":
		return strings.Join(r.Modifiers, " "), len(r.Modifiers) > 0
	case "cardinality":
		return r.Cardinality, r.Cardinality != ""
	case "type":
		return r.Type, r.Type != ""
	case "text":
		return r.Text, r.Text != ""
	case "relationLine":
		if r.RelationLine == 0 {
			return "", false
		}
		return strconv.Itoa(r.RelationLine), true
	case "relationLines":
		if len(r.RelationLines) == 0 {
			if r.RelationLine == 0 {
				return "", false
			}
			return strconv.Itoa(r.RelationLine), true
		}
		return joinIntsCapped(r.RelationLines, 8), true
	case "relationLinesFile":
		return r.RelationLinesFile, r.RelationLinesFile != ""
	case "count":
		if r.Count <= 1 {
			return "", false // a single, non-merged edge has nothing worth showing
		}
		return strconv.Itoa(r.Count), true
	case "fromMethods":
		return joinCapped(r.FromMethods, 5), len(r.FromMethods) > 0
	case "toMethods":
		return joinCapped(r.ToMethods, 5), len(r.ToMethods) > 0
	case "injected":
		if !r.Injected {
			return "", false
		}
		return "(injected)", true
	}
	return "", false
}

// joinCapped: at most `max` names, then "+N" for the rest — the short form
// {fromMethods}/{toMethods} print (part of lifting, ADR_20260928-3 §6).
func joinCapped(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ",")
	}
	return strings.Join(names[:max], ",") + fmt.Sprintf("+%d", len(names)-max)
}

// joinIntsCapped: like joinCapped, for the call/construct site line numbers
// (defect B: "at most 8 numbers, then +N").
func joinIntsCapped(nums []int, max int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	if len(parts) <= max {
		return strings.Join(parts, ",")
	}
	return strings.Join(parts[:max], ",") + fmt.Sprintf("+%d", len(parts)-max)
}

var nodeMacroNames = map[string]bool{
	"step": true, "name": true, "fullName": true, "id": true, "kind": true,
	"nativeKind": true, "visibility": true, "file": true, "line": true,
	"endLine": true, "lines": true, "namespace": true, "assembly": true,
	"containers": true, "presence": true, "status": true, "entity": true,
	"via": true, "viaFullName": true,
}

var relationMacroNames = map[string]bool{
	"relation": true, "member": true, "memberKind": true, "memberLine": true,
	"modifiers": true, "cardinality": true, "type": true, "text": true,
	"relationLine": true, "relationLines": true, "relationLinesFile": true,
	"count": true, "fromMethods": true, "toMethods": true, "injected": true,
}

// tplNode is one piece of a parsed template.
type tplNode interface {
	// render appends its text to b, given the enclosing node's macros and
	// (only meaningful inside a relations block) the current relation. Plain
	// concatenation — the whitespace rule (defect 4/5) is applied once, by
	// trimLeadingTrailingBlanks, around a whole *top-level* render (a
	// Template's own node list); it is deliberately NOT reapplied inside
	// every nested `[...]` group, so a group that intentionally carries its
	// own leading or trailing separator (`[×{count} ]from ...`) keeps it
	// even when the group next to it renders empty. Template authors get a
	// clean line by giving each optional piece its own leading separator, so
	// two adjacent empty groups never need "collapsing into" one another.
	render(b *strings.Builder, m NodeMacros, rel *RelationMacros)
	// empty reports whether this piece has no value at all — used by the
	// optional-group rule ("a macro with no value") and by the top-level
	// trim.
	empty(m NodeMacros, rel *RelationMacros) bool
}

type literalNode string

func (l literalNode) render(b *strings.Builder, _ NodeMacros, _ *RelationMacros) {
	b.WriteString(string(l))
}

// empty: a literal is text the author wrote, never "empty" in the
// macro-value sense — even when it is pure whitespace. Its fate under the
// top-level whitespace rule is decided separately, by isSpacesOnly.
func (l literalNode) empty(_ NodeMacros, _ *RelationMacros) bool { return false }

type macroNode string

func (n macroNode) render(b *strings.Builder, m NodeMacros, rel *RelationMacros) {
	if rel != nil {
		if v, ok := relationMacroValue(*rel, string(n)); ok {
			b.WriteString(v)
			return
		}
		// A node macro is also legal inside a relations block (an agent may
		// want the neighbour's own name next to the relation), fall through.
	}
	if v, ok := macroValue(m, string(n)); ok {
		b.WriteString(v)
	}
}
func (n macroNode) empty(m NodeMacros, rel *RelationMacros) bool {
	if rel != nil {
		if _, ok := relationMacroValue(*rel, string(n)); ok {
			return false
		}
	}
	_, ok := macroValue(m, string(n))
	return !ok
}

type optionalNode struct{ inner []tplNode }

func (o optionalNode) render(b *strings.Builder, m NodeMacros, rel *RelationMacros) {
	if hasEmptyMacro(o.inner, m, rel) {
		return
	}
	for _, n := range o.inner {
		n.render(b, m, rel)
	}
}
func (o optionalNode) empty(m NodeMacros, rel *RelationMacros) bool {
	return hasEmptyMacro(o.inner, m, rel)
}

// hasEmptyMacro: the group-drop rule — true when a macro, or a nested
// `{relations: ...}` block, directly in `nodes` has no value (a nested
// *optional* group is opaque to this check: if it renders as nothing, that
// alone does not empty the outer group — see `[×{count} ]` above).
func hasEmptyMacro(nodes []tplNode, m NodeMacros, rel *RelationMacros) bool {
	for _, n := range nodes {
		switch t := n.(type) {
		case macroNode:
			if t.empty(m, rel) {
				return true
			}
		case relationsBlockNode:
			if t.empty(m, rel) {
				return true
			}
		case optionalNode:
			// opaque, as documented above.
		}
	}
	return false
}

type relationsBlockNode struct {
	inner []tplNode
	sep   string
}

func (r relationsBlockNode) render(b *strings.Builder, m NodeMacros, _ *RelationMacros) {
	for i := range m.Relations {
		if i > 0 {
			b.WriteString(r.sep)
		}
		rel := m.Relations[i]
		for _, n := range r.inner {
			n.render(b, m, &rel)
		}
	}
}
func (r relationsBlockNode) empty(m NodeMacros, _ *RelationMacros) bool { return len(m.Relations) == 0 }

// Template is a parsed template, ready to render many nodes.
type Template struct {
	nodes []tplNode
	src   string
}

func (t *Template) String() string { return t.src }

// Render produces one node's line.
func (t *Template) Render(m NodeMacros) string {
	return renderTopLevel(t.nodes, m, nil)
}

// RenderRelation renders a template (typically a `lines`/`tree` relation
// template, which mixes a neighbour's own node macros with one relation's
// macros) for one specific relation, through the same top-level whitespace
// rule as Render — so it applies there too, not just inside `facts`'
// `{relations: ...}` block.
func (t *Template) RenderRelation(m NodeMacros, rel *RelationMacros) string {
	return renderTopLevel(t.nodes, m, rel)
}

// isSpacesOnly: every rune of s is the space character ' ', and s is not
// empty (an empty literal is never produced by the parser). Only this exact
// kind of literal is ever dropped by the whitespace rule — a tab or a
// newline, or a literal that mixes spaces with other characters, is always
// kept as written (defect 4/5: `locations`' tab-separated columns must not
// move when a value is empty).
func isSpacesOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != ' ' {
			return false
		}
	}
	return true
}

// renderTopLevel renders `nodes` (always a Template's own top-level list,
// never a nested `[...]` group's — see tplNode.render's comment) and applies
// the whitespace rule once: a space-only literal that falls in the LEADING
// run (from the start of the sequence, everything up to and including it
// renders empty) or the TRAILING run (from it to the end, everything renders
// empty) is dropped; a space-only literal with real content somewhere before
// it AND somewhere after it — even if the immediate neighbour on one side
// happens to be an empty macro or a dropped group — is kept exactly as
// written. This is what keeps the deliberate two spaces of `{fullName}
// {file}:{lines}` (both sides always have content) while still dropping a
// leading `{step} ` when there is no step, or a trailing `  ` before a
// `{relations: ...}` block that turned out empty.
func renderTopLevel(nodes []tplNode, m NodeMacros, rel *RelationMacros) string {
	texts := make([]string, len(nodes))
	blank := make([]bool, len(nodes))
	for i, n := range nodes {
		if lit, ok := n.(literalNode); ok {
			s := string(lit)
			texts[i] = s
			blank[i] = isSpacesOnly(s)
			continue
		}
		var b strings.Builder
		n.render(&b, m, rel)
		texts[i] = b.String()
		blank[i] = n.empty(m, rel)
	}
	lead := 0
	for lead < len(nodes) && blank[lead] {
		lead++
	}
	trail := len(nodes)
	for trail > lead && blank[trail-1] {
		trail--
	}
	var b strings.Builder
	for i, n := range nodes {
		if lit, ok := n.(literalNode); ok && isSpacesOnly(string(lit)) && (i < lead || i >= trail) {
			continue // a space-only literal in the leading/trailing blank run
		}
		b.WriteString(texts[i])
	}
	return b.String()
}

// ParseTemplate parses a template string (part 3). A malformed template
// names the position and, where relevant, the macro.
func ParseTemplate(src string) (*Template, error) {
	r := []rune(src)
	nodes, pos, err := parseTemplateBody(r, 0, -1)
	if err != nil {
		return nil, err
	}
	if pos != len(r) {
		return nil, fmt.Errorf("template: unexpected %q at position %d", string(r[pos]), pos)
	}
	return &Template{nodes: nodes, src: src}, nil
}

// parseTemplateBody parses until `closer` (a rune) is seen and consumed, or,
// when closer < 0, until the end of the input.
func parseTemplateBody(r []rune, i int, closer rune) ([]tplNode, int, error) {
	var nodes []tplNode
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			nodes = append(nodes, literalNode(lit.String()))
			lit.Reset()
		}
	}
	for i < len(r) {
		c := r[i]
		// \[ and \] are literal brackets (defect 4d): [/] are the
		// optional-group syntax, so a template that wants one printed has to
		// escape it. Recognised before the closer/group checks, so `\]`
		// never closes an enclosing group and `\[` never opens one.
		if c == '\\' && i+1 < len(r) && (r[i+1] == '[' || r[i+1] == ']') {
			lit.WriteRune(r[i+1])
			i += 2
			continue
		}
		if closer >= 0 && c == closer {
			flush()
			return nodes, i + 1, nil
		}
		switch c {
		case '{':
			flush()
			node, next, err := parseBraces(r, i)
			if err != nil {
				return nil, 0, err
			}
			nodes = append(nodes, node)
			i = next
		case '[':
			flush()
			inner, next, err := parseTemplateBody(r, i+1, ']')
			if err != nil {
				return nil, 0, fmt.Errorf("template: unmatched '[' at position %d", i)
			}
			nodes = append(nodes, optionalNode{inner: inner})
			i = next
		case ']':
			return nil, 0, fmt.Errorf("template: unmatched ']' at position %d", i)
		default:
			lit.WriteRune(c)
			i++
		}
	}
	if closer >= 0 {
		return nil, 0, fmt.Errorf("template: unmatched '[' before position %d", i)
	}
	flush()
	return nodes, i, nil
}

// parseBraces parses `{...}` starting at r[open]=='{': either a plain macro
// or a `{relations: TEMPLATE | SEPARATOR}` block.
func parseBraces(r []rune, open int) (tplNode, int, error) {
	depth := 1
	j := open + 1
	for depth > 0 {
		if j >= len(r) {
			return nil, 0, fmt.Errorf("template: unmatched '{' at position %d", open)
		}
		switch r[j] {
		case '{':
			depth++
		case '}':
			depth--
		}
		j++
	}
	content := string(r[open+1 : j-1])
	if strings.HasPrefix(content, "relations:") {
		rest := content[len("relations:"):]
		splitAt := findTopLevelPipe(rest)
		if splitAt < 0 {
			return nil, 0, fmt.Errorf("template: relations block at position %d is missing its '|' separator", open)
		}
		inner, sep := rest[:splitAt], rest[splitAt+1:]
		innerNodes, pos, err := parseTemplateBody([]rune(inner), 0, -1)
		if err != nil {
			return nil, 0, err
		}
		if pos != len([]rune(inner)) {
			return nil, 0, fmt.Errorf("template: malformed relations block at position %d", open)
		}
		return relationsBlockNode{inner: innerNodes, sep: sep}, j, nil
	}
	name := strings.TrimSpace(content)
	if !nodeMacroNames[name] && !relationMacroNames[name] {
		return nil, 0, fmt.Errorf("template: unknown macro %q at position %d", name, open)
	}
	return macroNode(name), j, nil
}

// findTopLevelPipe: the index of the first '|' in `s` not inside a `[...]`
// group (a relations block's separator never contains a raw '|').
func findTopLevelPipe(s string) int {
	depth := 0
	for i, c := range s {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
		case '|':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// MustTemplate parses a template known to be well-formed (a stored format's
// own template): a parse failure there is a programming error, not a caller
// mistake.
func MustTemplate(src string) *Template {
	t, err := ParseTemplate(src)
	if err != nil {
		panic(err)
	}
	return t
}
