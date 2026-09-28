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
func relationMacrosFor(e GraphEdge, fromID string, byID map[string]*GraphNode) RelationMacros {
	names := edgeRelationNames(e, fromID)
	name := strings.Join(names, "/")
	rm := RelationMacros{Relation: name, RelationLine: e.Line}
	if len(e.Lines) > 0 {
		rm.RelationLines = e.Lines
	}
	if e.Via != nil {
		rm.Member = e.Via.Member
		rm.MemberKind = e.Via.MemberKind
		rm.Modifiers = e.Via.Modifiers
		rm.Cardinality = e.Via.Cardinality
		rm.Text = e.Via.Text
		if n := byID[fromID]; n != nil && n.MemberLines != nil {
			if l, ok := n.MemberLines[e.Via.Member]; ok {
				rm.MemberLine = l
			}
		}
	}
	rm.Type = e.Type
	return rm
}

// combineHoldsAndInjects folds a `holds` and the `injects` of the same
// member of the same pair into one relation, printed as `holds … (injected)`
// (the `facts` format's rule, part 3).
func combineHoldsAndInjects(rels []RelationMacros) []RelationMacros {
	type key struct{ member, relation string }
	holdsIdx := map[string]int{}
	out := make([]RelationMacros, 0, len(rels))
	for _, r := range rels {
		if strings.HasPrefix(r.Relation, "holds") || strings.HasPrefix(r.Relation, "held-by") {
			holdsIdx[r.Member] = len(out)
			out = append(out, r)
			continue
		}
		if strings.HasPrefix(r.Relation, "injects") || strings.HasPrefix(r.Relation, "injected-into") {
			if i, ok := holdsIdx[r.Member]; ok {
				out[i].Relation += " (injected)"
				continue
			}
			r.Relation += " (injected)"
		}
		out = append(out, r)
	}
	return out
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
	var parents []string
	if n.Step != nil {
		want := *n.Step - 1
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
	}
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

// FactsTemplate is the stored template of the `facts` format (part 3): one
// line per neighbour, focus node first, relations to the node it was reached
// from folded into the same line.
const FactsTemplate = "{step} {fullName}  {file}:{lines}  [{relations: {relation} {member}[:{memberLine}] [{memberKind}] [{modifiers}] | ; }]"

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
			var rb strings.Builder
			rb.WriteString("  ")
			// relation line uses both node macros (of the neighbour) and
			// relation macros: render manually since it mixes both spaces.
			t := MustTemplate(LinesRelationTemplate)
			for _, tn := range t.nodes {
				tn.render(&rb, om, &rm)
			}
			b.WriteString(rb.String())
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
				var rb strings.Builder
				kRel := k.rm
				for _, tn := range relTpl.nodes {
					tn.render(&rb, NodeMacrosOf(n), &kRel)
				}
				b.WriteString(rb.String())
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
