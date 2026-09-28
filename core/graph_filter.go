// Graph filters: pure functions over *Graph, PLAN_20260928_host_graph-provider.md
// step 4. host/graph.go only parses query parameters and calls these; the
// filtering logic itself lives here so the HTTP handler and the MCP tool
// (step 6) share it verbatim.
package core

import (
	"fmt"
	"sort"
)

// FilterLevel is `level=types` (drop nodes of kind `function`, `value` and
// `method`, and every edge touching one) or `level=all` (no change). `types`
// is the default of a whole-graph request (no `around`/`container`,
// ADR_20260928-3 §7); `all` is the default of a neighbourhood request.
// Any other value is a usage mistake, not "no match": callers turn it into
// 400.
func FilterLevel(g *Graph, level string) (*Graph, error) {
	switch level {
	case "", "all":
		return g, nil
	case "types":
		drop := map[string]bool{}
		nodes := make([]GraphNode, 0, len(g.Nodes))
		for _, n := range g.Nodes {
			if n.Kind == "function" || n.Kind == "value" || n.Kind == "method" {
				drop[n.ID] = true
				continue
			}
			nodes = append(nodes, n)
		}
		edges := make([]GraphEdge, 0, len(g.Edges))
		for _, e := range g.Edges {
			if drop[e.From] || drop[e.To] {
				continue
			}
			edges = append(edges, e)
		}
		return &Graph{Nodes: nodes, Edges: edges}, nil
	default:
		return nil, fmt.Errorf("level: %q is neither %q nor %q", level, "types", "all")
	}
}

// FilterEdgeKinds keeps only edges whose Kind is one of `kinds` (applied
// after level, per the plan). An empty list is "no filter", not "keep
// nothing": the query parameter is absent, not empty-valued.
func FilterEdgeKinds(g *Graph, kinds []string) *Graph {
	if len(kinds) == 0 {
		return g
	}
	keep := map[string]bool{}
	for _, k := range kinds {
		keep[k] = true
	}
	edges := make([]GraphEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if keep[e.Kind] {
			edges = append(edges, e)
		}
	}
	return &Graph{Nodes: g.Nodes, Edges: edges}
}

// FanoutNote is what Walk reports when `fanout` left neighbours out: for
// `Node` and `Relation` (a follow name, from the point of view of `Node`),
// `Kept` were taken into the walk and `Left` were not.
type FanoutNote struct {
	Node     string
	Relation string
	Kept     int
	Left     int
}

// Walk is the neighbourhood of `around`, following exactly the directed
// relations named in `follow` (part 1 of the agent-answers rework: no more
// undirected `kinds`/named `set` for this case — see core/graph_relations.go).
// `fanout`, when > 0, caps how many neighbours a single (node, relation) pair
// contributes to the walk's growth; the excess is reported in the returned
// notes, never silently dropped from the count. The returned graph's nodes
// carry `Step`, their BFS distance from `around`; edges are every edge of `g`
// between two nodes of the resulting set that satisfies one of `follow`'s
// relations (so several relations to the same neighbour all appear, not just
// the one that first reached it). `around` naming no node in `g` is an error
// naming it, for callers to turn into 404.
func Walk(g *Graph, around string, depth int, follow []Relation, fanout int) (*Graph, []FanoutNote, error) {
	found := false
	for _, n := range g.Nodes {
		if n.ID == around {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("no such node: %q", around)
	}

	type candidate struct {
		other    string
		relation string
	}
	// neighboursOf: every (other, relationName) this node reaches by walking
	// one step along `follow`, deterministically ordered (by relation name,
	// then other id) so fanout's cut and the "kept" set are reproducible.
	neighboursOf := func(id string) []candidate {
		var out []candidate
		for _, r := range follow {
			for _, e := range g.Edges {
				if e.Kind != r.Kind || !typeMatches(e.Type, r.TypeMatch) {
					continue
				}
				if r.Forward && e.From == id {
					out = append(out, candidate{e.To, r.Name})
				}
				if !r.Forward && e.To == id {
					out = append(out, candidate{e.From, r.Name})
				}
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].relation != out[j].relation {
				return out[i].relation < out[j].relation
			}
			return out[i].other < out[j].other
		})
		return out
	}

	step := map[string]int{around: 0}
	frontier := []string{around}
	var notes []FanoutNote
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range frontier {
			cands := neighboursOf(id)
			// group by relation, so fanout counts per (node, relation), and
			// only NEW nodes count toward the cap and the note (a relation
			// that only re-reaches an already-visited node is never why a
			// caller lost a neighbour).
			byRel := map[string][]string{}
			var order []string
			for _, c := range cands {
				if _, ok := byRel[c.relation]; !ok {
					order = append(order, c.relation)
				}
				byRel[c.relation] = append(byRel[c.relation], c.other)
			}
			for _, rel := range order {
				var fresh []string
				for _, other := range byRel[rel] {
					if _, ok := step[other]; !ok {
						fresh = append(fresh, other)
					}
				}
				kept := fresh
				if fanout > 0 && len(fresh) > fanout {
					kept = fresh[:fanout]
					notes = append(notes, FanoutNote{Node: id, Relation: rel, Kept: fanout, Left: len(fresh) - fanout})
				}
				for _, other := range kept {
					step[other] = d + 1
					next = append(next, other)
				}
			}
		}
		frontier = next
	}

	nodes := make([]GraphNode, 0, len(step))
	for _, n := range g.Nodes {
		if s, ok := step[n.ID]; ok {
			nCopy := n
			sCopy := s
			nCopy.Step = &sCopy
			nodes = append(nodes, nCopy)
		}
	}
	edges := make([]GraphEdge, 0)
	for _, e := range g.Edges {
		if _, ok := step[e.From]; !ok {
			continue
		}
		if _, ok := step[e.To]; !ok {
			continue
		}
		for _, r := range follow {
			if e.Kind == r.Kind && typeMatches(e.Type, r.TypeMatch) {
				edges = append(edges, e)
				break
			}
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}, notes, nil
}

// LiftNoneHint is step 3d: asked with lift=none, a type/interface/module
// whose own edges (after `follow`) come back empty while its methods (found
// in `full`, the graph as it stood right before Walk ran) have some of the
// relations `follow` asked for — the answer would otherwise look like the
// node simply has no such relation, when really it has them one level down.
// `full` must still carry method nodes (lift=none never lifted them away).
// Returns "" when there is nothing to say: the focus is a method itself, it
// already has edges of its own in `walked`, or its methods have none either.
func LiftNoneHint(full, walked *Graph, focusID string, follow []Relation) string {
	byID := make(map[string]*GraphNode, len(full.Nodes))
	for i := range full.Nodes {
		byID[full.Nodes[i].ID] = &full.Nodes[i]
	}
	focus := byID[focusID]
	if focus == nil || focus.Kind == "method" {
		return ""
	}
	for _, e := range walked.Edges {
		if e.From == focusID || e.To == focusID {
			return "" // the focus already has relations of its own
		}
	}
	methods := map[string]bool{}
	for _, e := range full.Edges {
		if e.Kind == "contains" && e.From == focusID {
			if m := byID[e.To]; m != nil && m.Kind == "method" {
				methods[e.To] = true
			}
		}
	}
	if len(methods) == 0 {
		return ""
	}
	for _, e := range full.Edges {
		for _, r := range follow {
			if e.Kind != r.Kind || !typeMatches(e.Type, r.TypeMatch) {
				continue
			}
			if (r.Forward && methods[e.From]) || (!r.Forward && methods[e.To]) {
				return fmt.Sprintf("%s has no relations of its own; its methods do — ask again with lift=types (the default) to see them.", FullName(focus))
			}
		}
	}
	return ""
}

// FilterContainer keeps only nodes whose resolved containers include `id`
// itself or a descendant of it (by `parent`, containers.json), and edges
// between two kept nodes. `containers` is the full containers.json list, not
// just the set of known ids: descendants can only be found by walking
// `parent`. An id absent from it is a typo, not an empty container, so
// callers turn that into 404. A node's own Containers list is left as
// resolved — this only widens which containers count as a match, it never
// adds ancestors to a node.
//
// A cycle in `parent` cannot make this loop forever: the descendant walk
// tracks visited ids, exactly like Neighborhood's frontier walk.
func FilterContainer(g *Graph, id string, containers []Container) (*Graph, error) {
	known := map[string]bool{}
	children := map[string][]string{}
	for _, c := range containers {
		known[c.ID] = true
		if c.Parent != "" {
			children[c.Parent] = append(children[c.Parent], c.ID)
		}
	}
	if !known[id] {
		return nil, fmt.Errorf("no such container: %q", id)
	}
	inSubtree := map[string]bool{id: true}
	frontier := []string{id}
	for len(frontier) > 0 {
		var next []string
		for _, cur := range frontier {
			for _, child := range children[cur] {
				if !inSubtree[child] {
					inSubtree[child] = true
					next = append(next, child)
				}
			}
		}
		frontier = next
	}

	nodes := make([]GraphNode, 0)
	keep := map[string]bool{}
	for _, n := range g.Nodes {
		for _, c := range n.Containers {
			if inSubtree[c] {
				nodes = append(nodes, n)
				keep[n.ID] = true
				break
			}
		}
	}
	edges := make([]GraphEdge, 0)
	for _, e := range g.Edges {
		if keep[e.From] && keep[e.To] {
			edges = append(edges, e)
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}, nil
}

// FilterMissing drops model-only nodes whose entity has status "missing",
// and model-only edges whose relation has status "missing" — old relations
// and entities the registry has kept from before, which most callers do not
// want mixed into a live graph (a third of all edges, on a real project).
// Edges touching a dropped node are dropped too, whatever their own status.
// Nodes and edges with Presence "both" or "code" are never dropped here,
// even if Status happens to be "missing" for another reason. `include: true`
// is a no-op, and the two counts are the number of nodes/edges left out —
// for the answer's stats.hiddenMissing — always 0 when include is true.
func FilterMissing(g *Graph, include bool) (out *Graph, hiddenNodes int, hiddenEdges int) {
	if include {
		return g, 0, 0
	}
	dropNode := map[string]bool{}
	nodes := make([]GraphNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.Presence == "model" && n.Status == "missing" {
			dropNode[n.ID] = true
			continue
		}
		nodes = append(nodes, n)
	}
	edges := make([]GraphEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if dropNode[e.From] || dropNode[e.To] {
			continue
		}
		if e.Presence == "model" && e.Status == "missing" {
			continue
		}
		edges = append(edges, e)
	}
	return &Graph{Nodes: nodes, Edges: edges}, len(g.Nodes) - len(nodes), len(g.Edges) - len(edges)
}

// validFieldNames are the values `fields=` accepts (docs/API.md §5).
// `memberLines` is its own name, off by default: `position` no longer
// includes it (part 3 of the agent-answers rework).
var validFieldNames = map[string]bool{"via": true, "position": true, "members": true, "memberLines": true}

// ParseFields turns an already-split list of field names into the set
// StripFields/AttachMembers expect, rejecting a name that is none of
// "via"/"position"/"members"/"memberLines" — a caller's typo, not an empty
// selection. An empty (but non-nil, from an explicit `fields=`) or nil list
// both come back as an empty set; distinguishing "absent" (the default) from
// "explicitly empty" is the caller's job, not this function's.
func ParseFields(names []string) (map[string]bool, error) {
	out := make(map[string]bool, len(names))
	for _, f := range names {
		if !validFieldNames[f] {
			return nil, fmt.Errorf("fields: unknown field %q", f)
		}
		out[f] = true
	}
	return out, nil
}

// StripFields removes GraphEdge.Via when `fields` lacks "via", and
// every place in the code (file and lines of nodes and edges) when it lacks
// "position". "members" is handled earlier, by AttachMembers (graph.go):
// there is nothing to strip for it here, since a node without it asked for
// was never given one.
func StripFields(g *Graph, fields map[string]bool) *Graph {
	nodes := make([]GraphNode, len(g.Nodes))
	copy(nodes, g.Nodes)
	edges := make([]GraphEdge, len(g.Edges))
	copy(edges, g.Edges)
	if !fields["position"] {
		for i := range nodes {
			nodes[i].File, nodes[i].Line, nodes[i].EndLine = "", 0, 0
			nodes[i].Spans = nil
		}
		for i := range edges {
			edges[i].File, edges[i].Line = "", 0
		}
	}
	if !fields["memberLines"] {
		for i := range nodes {
			nodes[i].MemberLines = nil
		}
	}
	if !fields["via"] {
		for i := range edges {
			edges[i].Via = nil
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}
}
