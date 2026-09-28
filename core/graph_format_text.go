// The plain-text graph formats: facts, lines, tree, locations. Each is
// nothing but a stored template plus, for tree/lines, the walk that decides
// which node comes next and which edges are "the relations it was reached
// by" (part 3 of the agent-answers rework); the actual line text always
// comes out of the one template engine (core/graph_template.go).
package core

import (
	"fmt"
	"sort"
	"strings"
)

// clearContainerPosition blanks a namespace/assembly node's file/lines
// macros: a container has no one file (part 3).
func clearContainerPosition(m *NodeMacros, n *GraphNode) {
	if isContainerNode(n) {
		m.File, m.Line, m.EndLine = "", 0, 0
	}
}

func isContainerNode(n *GraphNode) bool {
	return n.Kind == "module" && (n.NativeKind == "namespace" || n.NativeKind == "assembly")
}

// countsLine is the short trailing line every text format ends with, plus a
// note when the answer was cut by a limit or by fanout.
func countsLine(g *Graph, o FormatOptions) string {
	s := fmt.Sprintf("%d nodes, %d edges", len(g.Nodes), len(g.Edges))
	if o.Truncated {
		s += fmt.Sprintf(" (cut to a limit: %d of %d nodes, %d of %d edges)", len(g.Nodes), o.FullNodes, len(g.Edges), o.FullEdges)
	}
	for _, n := range o.FanoutNotes {
		s += fmt.Sprintf("\n(fanout: %s %s cut to %d, %d more not taken)", n.Node, n.Relation, n.Kept, n.Left)
	}
	return s
}

// noticePrefix: the name-resolution notice of part 2, as the first line of a
// text answer when there is one.
func noticePrefix(o FormatOptions) string {
	if o.Notice == "" {
		return ""
	}
	return o.Notice + "\n"
}

func nodeByID(g *Graph) map[string]*GraphNode {
	m := make(map[string]*GraphNode, len(g.Nodes))
	for i := range g.Nodes {
		m[g.Nodes[i].ID] = &g.Nodes[i]
	}
	return m
}

// relationMacrosFor builds a nested {relations: …} entry for edge `e`, seen
// from the point of view of `fromID` (the node it was reached FROM): the
// relation name is directed from there, per core/graph_relations.go.
//
// {memberLine} and {relationLine}/{relationLines} are two different numbers,
// not two names for one (defect 2 of the agent-answers-graph task):
//   - for a `holds`/`uses` edge (which covers `injects`, a `uses` narrowed to
//     a constructor parameter), the member is always declared on e.From,
//     whichever direction the relation is being read from (`holds` or its
//     reverse `held-by`, `injects` or `injected-into`) — and EXTRACTOR.md §2.2
//     says e.Line is exactly that member's declaration line for this Kind.
//     So {memberLine} is simply e.Line, computed once, not looked up via a
//     MemberLines map keyed by whichever node happened to be "fromID" (the
//     walk-parent) — that lookup broke whenever fromID was the reverse
//     direction's parent (the held/injected TYPE, not its holder), which is
//     exactly the `held-by`/`injected-into` case the real project surfaced.
//   - for a `calls`/`constructs` edge, e.Line/e.Lines are the call/construct
//     site(s), reported as {relationLine}/{relationLines} — never as
//     {memberLine}, which stays empty there (no member is involved).
func relationMacrosFor(e GraphEdge, fromID string, byID map[string]*GraphNode) RelationMacros {
	names := edgeRelationNames(e, fromID)
	name := strings.Join(names, "/")
	rm := RelationMacros{Relation: name, other: otherEnd(e, fromID)}
	if e.Kind == "calls" || e.Kind == "constructs" {
		rm.RelationLine = e.Line
		if len(e.Lines) > 0 {
			rm.RelationLines = e.Lines
		}
	}
	if e.Via != nil {
		rm.Member = e.Via.Member
		rm.MemberKind = e.Via.MemberKind
		rm.Modifiers = e.Via.Modifiers
		rm.Cardinality = e.Via.Cardinality
		rm.Text = e.Via.Text
		rm.MemberLine = e.Line
	}
	rm.Type = e.Type
	rm.Count = e.Count
	rm.FromMethods = e.FromMethods
	rm.ToMethods = e.ToMethods
	return rm
}

// otherEnd: the id at the far end of `e` from `fromID` — used only to pair a
// `holds`/`held-by` relation with the `injects`/`injected-into` of the same
// member of the same pair (combineHoldsAndInjects): "same pair" means the
// same two nodes, not merely the same member name anywhere on the line.
func otherEnd(e GraphEdge, fromID string) string {
	if e.From == fromID {
		return e.To
	}
	return e.From
}

// normalizeMemberName: for matching a holds/injects pair (defect 3): a
// field, a property and a constructor parameter for the same thing are
// often spelled differently (`_context` the field, `context` the parameter,
// `Context` the property) — compared case-insensitively, with at most one
// leading underscore stripped.
func normalizeMemberName(s string) string {
	s = strings.TrimPrefix(s, "_")
	return strings.ToLower(s)
}

// combineHoldsAndInjects folds a `holds`/`held-by` and the
// `injects`/`injected-into` of the same member (normalizeMemberName) of the
// same pair (otherEnd) into one relation — the holds one, its Injected flag
// set so a template can print "(injected)" wherever it likes (defect 3): an
// `injects`/`injected-into` with no matching holds relation is left plain,
// with Injected still false.
func combineHoldsAndInjects(rels []RelationMacros) []RelationMacros {
	type pairKey struct{ other, member string }
	holdsIdx := map[pairKey]int{}
	out := make([]RelationMacros, 0, len(rels))
	for _, r := range rels {
		switch {
		case strings.HasPrefix(r.Relation, "holds") || strings.HasPrefix(r.Relation, "held-by"):
			holdsIdx[pairKey{r.other, normalizeMemberName(r.Member)}] = len(out)
			out = append(out, r)
		case strings.HasPrefix(r.Relation, "injects") || strings.HasPrefix(r.Relation, "injected-into"):
			if i, ok := holdsIdx[pairKey{r.other, normalizeMemberName(r.Member)}]; ok {
				out[i].Injected = true
				continue
			}
			out = append(out, r)
		default:
			out = append(out, r)
		}
	}
	return out
}

// stepParents: the id(s) of the node(s) one step closer to the focus than
// `id` that reach it directly by an edge — `id`'s walk parent(s). Nil when
// `id` carries no Step (a plain whole-graph list, no walk) or is the focus
// itself (step 0). Shared by relationsReachingNode (the "reached from" set
// a relation is named from) and the {via}/{viaFullName} macros (defect 6).
func stepParents(g *Graph, id string, byID map[string]*GraphNode) []string {
	n := byID[id]
	if n == nil || n.Step == nil || *n.Step == 0 {
		return nil
	}
	want := *n.Step - 1
	var parents []string
	for _, e := range g.Edges {
		var other string
		switch {
		case e.From == id:
			other = e.To
		case e.To == id:
			other = e.From
		default:
			continue
		}
		if on := byID[other]; on != nil && on.Step != nil && *on.Step == want {
			parents = append(parents, other)
		}
	}
	return parents
}

// relationsReachingNode: every edge of `g` connecting `id` to a node one
// step closer to the focus (its "reached from" set), or — with no focus/step
// data — every edge touching `id` at all (a plain per-node relation list).
func relationsReachingNode(g *Graph, id string, byID map[string]*GraphNode) []RelationMacros {
	n := byID[id]
	if n == nil {
		return nil
	}
	if n.Step != nil && *n.Step == 0 {
		return nil // the focus node itself: nothing reached it from anywhere
	}
	parents := stepParents(g, id, byID)
	// perspective: whose point of view {relation} is named from. With step
	// data, that is the node it was reached FROM (the parent); with none (a
	// plain whole-graph list, no walk), there is no "reached from", so each
	// node's own perspective is used instead (part 3's rule only applies to
	// a walk's result).
	perspective := id
	haveParents := n.Step != nil
	var rels []RelationMacros
	for _, e := range g.Edges {
		var other string
		switch {
		case e.From == id:
			other = e.To
		case e.To == id:
			other = e.From
		default:
			continue
		}
		if len(parents) > 0 && !contains(parents, other) {
			continue
		}
		if haveParents {
			perspective = other
		}
		rels = append(rels, relationMacrosFor(e, perspective, byID))
	}
	sort.Slice(rels, func(i, j int) bool {
		if rels[i].Relation != rels[j].Relation {
			return rels[i].Relation < rels[j].Relation
		}
		return rels[i].Member < rels[j].Member
	})
	return rels
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// -------------------------------------------------------------------- facts

// FactsTemplate is the stored template of the `facts` format (part 3, then
// defects 2-6 of the agent-answers-graph task): one line per neighbour,
// focus node first, relations to the node it was reached from folded into
// the same line, in square brackets (escaped: `\[`/`\]`), separated by "; ".
// Every optional piece after {relation} carries its own leading space, so
// the whitespace rule (core/graph_template.go) cleans up whichever pieces
// are absent without any manual spacing per case: a bare "extends", a
// "holds Context:7 property protected readonly (injected)", a lifted
// "calls ×3 from Create,List to Widget,Gadget" (or, unmerged, just
// "calls from Create to Widget" — {count} is empty at 1) all come out of the
// same template. {via} is only ever non-empty at step >= 2 (defect 6), so
// the trailing " via {via}" group only shows up there.
const FactsTemplate = `{step} {fullName}  {file}:{lines}  [\[{relations:{relation}[ {member}[:{memberLine}]][ {memberKind}][ {modifiers}][ [×{count} ]from {fromMethods}[ to {toMethods}]][ {injected}]|; }\]][ via {via}]`

type factsFormat struct{}

func (factsFormat) Name() string { return "facts" }
func (factsFormat) Description() string {
	return "one line per neighbour: step, full name, position, relations to the node it was reached from"
}
func (factsFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (factsFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	return renderPerNodeTemplate(g, MustTemplate(FactsTemplate), o)
}

// FormatWithTemplate renders `g` with a caller-supplied template (part 3:
// an agent's own `template`, one line per node, in the same shape as
// `facts` — step/fullName/position plus the relations reaching each node
// from the node it was reached from). A malformed template is a parse
// error naming the position and the macro.
func FormatWithTemplate(g *Graph, templateSrc string, o FormatOptions) ([]byte, error) {
	tpl, err := ParseTemplate(templateSrc)
	if err != nil {
		return nil, err
	}
	return renderPerNodeTemplate(g, tpl, o)
}

// viaOf: the {via}/{viaFullName} macro values for a node reached from
// `parents` (its walk parents, stepParents) — short names and full names,
// sorted and de-duplicated, comma-joined (defect 6: several parents at the
// same step are all named, once each).
func viaOf(parents []string, byID map[string]*GraphNode) (short, full string) {
	seenS, seenF := map[string]bool{}, map[string]bool{}
	var shorts, fulls []string
	for _, p := range parents {
		n := byID[p]
		if n == nil {
			continue
		}
		if s := n.Name; s != "" && !seenS[s] {
			seenS[s] = true
			shorts = append(shorts, s)
		}
		if f := FullName(n); f != "" && !seenF[f] {
			seenF[f] = true
			fulls = append(fulls, f)
		}
	}
	sort.Strings(shorts)
	sort.Strings(fulls)
	return strings.Join(shorts, ","), strings.Join(fulls, ",")
}

func renderPerNodeTemplate(g *Graph, tpl *Template, o FormatOptions) ([]byte, error) {
	byID := nodeByID(g)
	order := orderedIDs(g, o.Focus)
	var b strings.Builder
	b.WriteString(noticePrefix(o))
	for _, id := range order {
		n := byID[id]
		m := NodeMacrosOf(n)
		clearContainerPosition(&m, n)
		m.Relations = combineHoldsAndInjects(relationsReachingNode(g, id, byID))
		if n.Step != nil && *n.Step >= 2 {
			m.Via, m.ViaFullName = viaOf(stepParents(g, id, byID), byID)
		}
		b.WriteString(tpl.Render(m))
		b.WriteString("\n")
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// orderedIDs: the focus node first (when there is one and it is in `g`),
// then every other node by step (ascending) then full name.
func orderedIDs(g *Graph, focus string) []string {
	byID := nodeByID(g)
	ids := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool {
		ni, nj := byID[ids[i]], byID[ids[j]]
		si, sj := stepOf(ni), stepOf(nj)
		if si != sj {
			return si < sj
		}
		return FullName(ni) < FullName(nj)
	})
	if focus == "" {
		return ids
	}
	out := []string{focus}
	for _, id := range ids {
		if id != focus {
			out = append(out, id)
		}
	}
	return out
}

func stepOf(n *GraphNode) int {
	if n == nil || n.Step == nil {
		return 0
	}
	return *n.Step
}

// ---------------------------------------------------------------- lines

// LinesTemplate is the stored template of the `lines` format's header line
// (one per node); its relations print one per following line, indented,
// via LinesRelationTemplate.
const LinesTemplate = "[{step}: ]{fullName}[  {file}:{lines}][  in {namespace}]"
const LinesRelationTemplate = "{relation}[: {member}[ ({memberKind})][:{memberLine}]]  {fullName}[  {file}:{lines}]"

type linesFormat struct{}

func (linesFormat) Name() string { return "lines" }
func (linesFormat) Description() string {
	return "plain text, one block per node: header line, then its relations to the node it was reached from"
}
func (linesFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (linesFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	byID := nodeByID(g)
	order := orderedIDs(g, o.Focus)
	header := MustTemplate(LinesTemplate)

	var b strings.Builder
	b.WriteString(noticePrefix(o))
	for _, id := range order {
		n := byID[id]
		m := NodeMacrosOf(n)
		clearContainerPosition(&m, n)
		b.WriteString(header.Render(m))
		b.WriteString("\n")

		rels := relationsReachingNodeAllDirections(g, id, byID)
		for _, rel := range rels {
			other := byID[rel.otherID]
			om := NodeMacrosOf(other)
			clearContainerPosition(&om, other)
			rm := rel.rm
			// relation line uses both node macros (of the neighbour) and
			// relation macros: mixes both, through the same engine (and its
			// whitespace rule) as every other template.
			t := MustTemplate(LinesRelationTemplate)
			b.WriteString("  ")
			b.WriteString(t.RenderRelation(om, &rm))
			b.WriteString("\n")
		}
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

type namedRelation struct {
	otherID string
	rm      RelationMacros
}

// relationsReachingNodeAllDirections: every edge touching `id`, in both
// directions (the classic `lines` behaviour — every direct neighbour, not
// only the walk's parent), sorted by relation name then neighbour name.
func relationsReachingNodeAllDirections(g *Graph, id string, byID map[string]*GraphNode) []namedRelation {
	var out []namedRelation
	for _, e := range g.Edges {
		var other, fromID string
		switch {
		case e.From == id:
			other, fromID = e.To, id
		case e.To == id:
			other, fromID = e.From, id
		default:
			continue
		}
		out = append(out, namedRelation{otherID: other, rm: relationMacrosFor(e, fromID, byID)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rm.Relation != out[j].rm.Relation {
			return out[i].rm.Relation < out[j].rm.Relation
		}
		return FullName(byID[out[i].otherID]) < FullName(byID[out[j].otherID])
	})
	return out
}

// ------------------------------------------------------------ locations

// LocationsTemplate is the stored template of the `locations` format.
const LocationsTemplate = "{fullName}\t{file}\t{line}\t{endLine}"

type locationsFormat struct{}

func (locationsFormat) Name() string { return "locations" }
func (locationsFormat) Description() string {
	return "fullName<TAB>file<TAB>line<TAB>endLine, one line per node, no relations"
}
func (locationsFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (locationsFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	tpl := MustTemplate(LocationsTemplate)
	var b strings.Builder
	b.WriteString(noticePrefix(o))
	for _, n := range g.Nodes {
		nn := n
		if isContainerNode(&nn) {
			nn.File = ""
		}
		b.WriteString(tpl.Render(NodeMacrosOf(&nn)))
		b.WriteString("\n")
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// ------------------------------------------------------------------ tree

// TreeNodeTemplate/TreeRelationTemplate: the stored templates of the `tree`
// format. Indentation and "(see above)" de-duplication are the walk's job
// (they are not expressible as a per-line template), the line text itself
// is not.
const TreeNodeTemplate = "{fullName}[  {file}:{lines}]"
const TreeRelationTemplate = "{relation}[ {member}][:{memberLine}] → "

// treeFormat: indented by walk depth from the focus node. Without a focus
// this format is refused — there is no "depth from" without one.
type treeFormat struct{}

func (treeFormat) Name() string { return "tree" }
func (treeFormat) Description() string {
	return "indented by depth from the focus node: relation [member] → name  file:line; needs `around`"
}
func (treeFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (treeFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	if o.Focus == "" {
		return nil, fmt.Errorf("tree needs a focus node (around=...): there is no depth to indent by without one")
	}
	byID := nodeByID(g)
	root := byID[o.Focus]
	if root == nil {
		return nil, fmt.Errorf("tree: focus node %q not in the graph", o.Focus)
	}
	nodeTpl, relTpl := MustTemplate(TreeNodeTemplate), MustTemplate(TreeRelationTemplate)

	adjacent := map[string][]int{}
	for i, e := range g.Edges {
		adjacent[e.From] = append(adjacent[e.From], i)
		adjacent[e.To] = append(adjacent[e.To], i)
	}

	var b strings.Builder
	b.WriteString(noticePrefix(o))
	rm := NodeMacrosOf(root)
	clearContainerPosition(&rm, root)
	b.WriteString(nodeTpl.Render(rm))
	b.WriteString("\n")

	printed := map[string]bool{o.Focus: true}
	type item struct {
		id    string
		depth int
	}
	type child struct {
		id string
		rm RelationMacros
	}
	frontier := []item{{o.Focus, 0}}
	visitedAsParent := map[string]bool{}
	for len(frontier) > 0 {
		var next []item
		byParent := map[string][]child{}
		var parents []string
		for _, it := range frontier {
			if visitedAsParent[it.id] {
				continue
			}
			visitedAsParent[it.id] = true
			parents = append(parents, it.id)
			for _, ei := range adjacent[it.id] {
				e := g.Edges[ei]
				var otherID string
				switch {
				case e.From == it.id:
					otherID = e.To
				case e.To == it.id:
					otherID = e.From
				default:
					continue
				}
				byParent[it.id] = append(byParent[it.id], child{otherID, relationMacrosFor(e, it.id, byID)})
			}
		}
		sort.Strings(parents)
		for _, pid := range parents {
			kids := byParent[pid]
			sort.Slice(kids, func(i, j int) bool {
				if kids[i].rm.Relation != kids[j].rm.Relation {
					return kids[i].rm.Relation < kids[j].rm.Relation
				}
				return FullName(byID[kids[i].id]) < FullName(byID[kids[j].id])
			})
			depth := 0
			for _, it := range frontier {
				if it.id == pid {
					depth = it.depth
					break
				}
			}
			for _, k := range kids {
				n := byID[k.id]
				b.WriteString(strings.Repeat("  ", depth+1))
				kRel := k.rm
				b.WriteString(relTpl.RenderRelation(NodeMacrosOf(n), &kRel))
				if !printed[k.id] {
					printed[k.id] = true
					nm := NodeMacrosOf(n)
					clearContainerPosition(&nm, n)
					b.WriteString(nodeTpl.Render(nm))
					next = append(next, item{k.id, depth + 1})
				} else {
					b.WriteString(FullName(n))
					b.WriteString("  (see above)")
				}
				b.WriteString("\n")
			}
		}
		frontier = next
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}
