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

// FilterContainer keeps only nodes in container `id`, and edges between two
// kept nodes. `known` is the set of container ids that actually exist
// (containers.json, regardless of whether anything currently resolves into
// them) — an id absent from it is a typo, not an empty container, so callers
// turn it into 404.
func FilterContainer(g *Graph, id string, known map[string]bool) (*Graph, error) {
	if !known[id] {
		return nil, fmt.Errorf("no such container: %q", id)
	}
	nodes := make([]GraphNode, 0)
	keep := map[string]bool{}
	for _, n := range g.Nodes {
		for _, c := range n.Containers {
			if c == id {
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

// StripFields removes GraphEdge.Via when `fields` lacks "via", and
// GraphNode.File/Line when it lacks "position". "members" is handled
// earlier, by AttachMembers (graph.go): there is nothing to strip for it
// here, since a node without it asked for was never given one.
func StripFields(g *Graph, fields map[string]bool) *Graph {
	nodes := make([]GraphNode, len(g.Nodes))
	copy(nodes, g.Nodes)
	if !fields["position"] {
		for i := range nodes {
			nodes[i].File, nodes[i].Line = "", 0
		}
	}
	edges := make([]GraphEdge, len(g.Edges))
	copy(edges, g.Edges)
	if !fields["via"] {
		for i := range edges {
			edges[i].Via = nil
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}
}
