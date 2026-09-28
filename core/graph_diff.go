// GraphDiff: what changed between two builds of the same project's graph,
// PLAN_20260928_host_graph-provider.md step 5. A pure function so host/graph.go
// can call it without duplicating comparison logic, and so subscribers of
// /api/events get told what changed rather than the whole graph.
package core

import (
	"encoding/json"
	"sort"
)

// GraphEdgeKey identifies an edge for a diff, without its content.
type GraphEdgeKey struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// GraphDiff is added/removed/changed node ids, and added/removed edges, of
// one project's graph between two builds. It never carries the graphs
// themselves — a subscriber that wants the new content asks for it (GET
// /api/graph/{project}); the diff only says that it should.
type GraphDiff struct {
	AddedNodes   []string       `json:"addedNodes,omitempty"`
	RemovedNodes []string       `json:"removedNodes,omitempty"`
	ChangedNodes []string       `json:"changedNodes,omitempty"`
	AddedEdges   []GraphEdgeKey `json:"addedEdges,omitempty"`
	RemovedEdges []GraphEdgeKey `json:"removedEdges,omitempty"`
}

// Empty: nothing worth telling a subscriber.
func (d GraphDiff) Empty() bool {
	return len(d.AddedNodes) == 0 && len(d.RemovedNodes) == 0 && len(d.ChangedNodes) == 0 &&
		len(d.AddedEdges) == 0 && len(d.RemovedEdges) == 0
}

func edgeKey(e GraphEdge) GraphEdgeKey { return GraphEdgeKey{From: e.From, To: e.To, Kind: e.Kind} }

// nodeSignature is what "changed" means for a node: anything a viewer of
// that node would see differently. Comparing the marshaled JSON is simpler
// than listing fields by hand and cannot drift out of sync with GraphNode.
func nodeSignature(n GraphNode) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func edgeKeyLess(a, b GraphEdgeKey) bool {
	if a.From != b.From {
		return a.From < b.From
	}
	if a.To != b.To {
		return a.To < b.To
	}
	return a.Kind < b.Kind
}

// DiffGraphs compares two builds of the same project's graph: node ids
// added, removed, or present in both but changed; edges added or removed
// (identified by from/to/kind, since an edge has no id of its own). Both
// nil old and a new graph are read literally — old with no nodes means
// everything in new is "added".
func DiffGraphs(old, new *Graph) GraphDiff {
	oldNodes := map[string]GraphNode{}
	if old != nil {
		for _, n := range old.Nodes {
			oldNodes[n.ID] = n
		}
	}
	newNodes := map[string]GraphNode{}
	if new != nil {
		for _, n := range new.Nodes {
			newNodes[n.ID] = n
		}
	}
	var diff GraphDiff
	for id, n := range newNodes {
		if o, ok := oldNodes[id]; !ok {
			diff.AddedNodes = append(diff.AddedNodes, id)
		} else if nodeSignature(o) != nodeSignature(n) {
			diff.ChangedNodes = append(diff.ChangedNodes, id)
		}
	}
	for id := range oldNodes {
		if _, ok := newNodes[id]; !ok {
			diff.RemovedNodes = append(diff.RemovedNodes, id)
		}
	}

	oldEdges := map[GraphEdgeKey]bool{}
	if old != nil {
		for _, e := range old.Edges {
			oldEdges[edgeKey(e)] = true
		}
	}
	newEdges := map[GraphEdgeKey]bool{}
	if new != nil {
		for _, e := range new.Edges {
			newEdges[edgeKey(e)] = true
		}
	}
	for k := range newEdges {
		if !oldEdges[k] {
			diff.AddedEdges = append(diff.AddedEdges, k)
		}
	}
	for k := range oldEdges {
		if !newEdges[k] {
			diff.RemovedEdges = append(diff.RemovedEdges, k)
		}
	}

	sort.Strings(diff.AddedNodes)
	sort.Strings(diff.RemovedNodes)
	sort.Strings(diff.ChangedNodes)
	sort.Slice(diff.AddedEdges, func(i, j int) bool { return edgeKeyLess(diff.AddedEdges[i], diff.AddedEdges[j]) })
	sort.Slice(diff.RemovedEdges, func(i, j int) bool { return edgeKeyLess(diff.RemovedEdges[i], diff.RemovedEdges[j]) })
	return diff
}
