// Graph filters: pure functions over *Graph, PLAN_20260928_host_graph-provider.md
// step 4. host/graph.go only parses query parameters and calls these; the
// filtering logic itself lives here so the HTTP handler and the MCP tool
// (step 6) share it verbatim.
package core

import "fmt"

// FilterLevel is `level=types` (drop nodes of kind `function` and `value`,
// and every edge touching one) or `level=all` (default, no change). Any
// other value is a usage mistake, not "no match": callers turn it into 400.
func FilterLevel(g *Graph, level string) (*Graph, error) {
	switch level {
	case "", "all":
		return g, nil
	case "types":
		drop := map[string]bool{}
		nodes := make([]GraphNode, 0, len(g.Nodes))
		for _, n := range g.Nodes {
			if n.Kind == "function" || n.Kind == "value" {
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

// Neighborhood is the subgraph reachable from `around` within `depth` hops,
// counting edges in either direction (breadth-first, as the plan asks).
// `around` naming no node in `g` is an error naming it, for callers to turn
// into 404.
func Neighborhood(g *Graph, around string, depth int) (*Graph, error) {
	found := false
	for _, n := range g.Nodes {
		if n.ID == around {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("no such node: %q", around)
	}
	adjacent := map[string][]int{} // node id -> indexes of edges touching it
	for i, e := range g.Edges {
		adjacent[e.From] = append(adjacent[e.From], i)
		adjacent[e.To] = append(adjacent[e.To], i)
	}
	visited := map[string]bool{around: true}
	frontier := []string{around}
	edgeSet := map[int]bool{}
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range frontier {
			for _, ei := range adjacent[id] {
				edgeSet[ei] = true
				e := g.Edges[ei]
				other := e.To
				if other == id {
					other = e.From
				}
				if !visited[other] {
					visited[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	nodes := make([]GraphNode, 0, len(visited))
	for _, n := range g.Nodes {
		if visited[n.ID] {
			nodes = append(nodes, n)
		}
	}
	edges := make([]GraphEdge, 0, len(edgeSet))
	for i, e := range g.Edges {
		if edgeSet[i] {
			edges = append(edges, e)
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}, nil
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
var validFieldNames = map[string]bool{"via": true, "position": true, "members": true}

// ParseFields turns an already-split list of field names into the set
// StripFields/AttachMembers expect, rejecting a name that is none of
// "via"/"position"/"members" — a caller's typo, not an empty selection.
// An empty (but non-nil, from an explicit `fields=`) or nil list both come
// back as an empty set; distinguishing "absent" (the default) from
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
			nodes[i].Spans, nodes[i].MemberLines = nil, nil
		}
		for i := range edges {
			edges[i].File, edges[i].Line = "", 0
		}
	}
	if !fields["via"] {
		for i := range edges {
			edges[i].Via = nil
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}
}
