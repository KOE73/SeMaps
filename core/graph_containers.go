package core

import "sort"

// ContainsKind is the edge kind — the family of a relation type, before its
// first dot — of "a container holds a member": a `contains` relation goes from
// the container to what lies in it.
const ContainsKind = "contains"

// Containers of the graph: ADR_20260930_contract_graph-containers-from-contains.
// A container is a node whose kind is a container kind of the dictionary
// (kinds.json, `container: true`); the members of a container are the nodes it
// `contains` — a `contains` edge from the container, whether the extractor
// reported it or a person wrote it as a relation. There is no other source: no
// rules by name or path, no neighbour in the same file, no guessing.
//
// A node has the innermost container among those that contain it. When that is
// not one node — two unrelated containers contain it — it belongs to none.

// assignContainers fills GraphNode.Container and GraphNode.Containers.
// kindOf is the kind each node's entity has, or would have when sync creates
// it (the symbol's nativeKind); a node it does not know is no container.
func assignContainers(nodes []GraphNode, edges []GraphEdge, kindOf map[string]string, kinds *KindCatalog) {
	isContainer := func(id string) bool { return kinds.IsContainer(kindOf[id]) }
	for i := range nodes {
		nodes[i].Container = isContainer(nodes[i].ID)
	}
	// contained node -> the containers that contain it directly
	direct := map[string][]string{}
	for _, e := range edges {
		if e.Kind != ContainsKind || e.From == e.To || !isContainer(e.From) {
			continue
		}
		direct[e.To] = appendUnique(direct[e.To], e.From)
	}
	// outer[c]: every container above c, through any of its containers
	outer := map[string]map[string]bool{}
	var above func(c string) map[string]bool
	above = func(c string) map[string]bool {
		if got, ok := outer[c]; ok {
			return got
		}
		set := map[string]bool{}
		outer[c] = set // a cycle sees the set so far and stops
		for _, p := range direct[c] {
			set[p] = true
			for q := range above(p) {
				set[q] = true
			}
		}
		return set
	}
	for i := range nodes {
		cands := direct[nodes[i].ID]
		if len(cands) == 0 {
			continue
		}
		// a candidate that another candidate lies in is not the innermost
		var innermost []string
		for _, c := range cands {
			dominated := false
			for _, other := range cands {
				if other != c && above(other)[c] {
					dominated = true
					break
				}
			}
			if !dominated {
				innermost = append(innermost, c)
			}
		}
		if len(innermost) == 1 {
			nodes[i].Containers = []string{innermost[0]}
		}
	}
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// FilterContainer keeps the container `id` — a node id, resolved by the caller
// — and every node inside it, at any depth, and the edges between two kept
// nodes. A node id that is no container is an error, not an empty answer. A
// node's own Containers list is left as it is: this only widens what counts as
// a match, it never adds ancestors to a node.
func FilterContainer(g *Graph, id string) (*Graph, error) {
	var root *GraphNode
	children := map[string][]string{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.ID == id {
			root = n
		}
		if len(n.Containers) == 1 {
			children[n.Containers[0]] = append(children[n.Containers[0]], n.ID)
		}
	}
	if root == nil {
		return nil, refuse("no such container: %q", id)
	}
	if !root.Container {
		return nil, refuse("%s is not a container: its kind %q is not a container kind of kinds.json", id, root.Kind)
	}
	inside := map[string]bool{id: true}
	frontier := []string{id}
	for len(frontier) > 0 { // a node is visited once, so a cycle ends the walk
		var next []string
		for _, cur := range frontier {
			for _, child := range children[cur] {
				if !inside[child] {
					inside[child] = true
					next = append(next, child)
				}
			}
		}
		frontier = next
	}
	nodes := make([]GraphNode, 0)
	for _, n := range g.Nodes {
		if inside[n.ID] {
			nodes = append(nodes, n)
		}
	}
	edges := make([]GraphEdge, 0)
	for _, e := range g.Edges {
		if inside[e.From] && inside[e.To] {
			edges = append(edges, e)
		}
	}
	return &Graph{Nodes: nodes, Edges: edges}, nil
}

// GroupContainer is one container of the graph with the container it lies in
// (ids are graph node ids). Name is the node's name.
type GroupContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// GraphContainers lists every container node of the graph with its own
// container, in node order. It is what the graph mode nests groups by.
func GraphContainers(g *Graph) []GroupContainer {
	out := []GroupContainer{}
	for _, n := range g.Nodes {
		if !n.Container {
			continue
		}
		c := GroupContainer{ID: n.ID, Name: n.Name}
		if len(n.Containers) == 1 {
			c.Parent = n.Containers[0]
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
