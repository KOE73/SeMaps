// LiftToTypes: subject of ADR_20260928-3 §6 — the graph can, on request,
// fold method-level edges up to the types that contain them, so "who calls
// class X" can be asked about the type directly instead of walking every one
// of its methods. Pure and deterministic; the input graph is never mutated.
package core

import "sort"

// LiftToTypes returns a new graph where every edge with a `method`-kind node
// at either end is rewritten to use that method's containing type instead
// (found via `contains` edges, built once here). Method nodes, and the
// `contains` edges reaching them, are not part of the result — methods are
// not nodes of a lifted answer. An edge that becomes a type to itself after
// lifting (recursion, or two methods of the same type calling each other) is
// dropped. Edges that become equal (same From, To, Kind, Type) after lifting
// are merged into one that keeps Count (how many were merged), FromMethods
// and ToMethods (short names of the methods lifted from each end, sorted and
// de-duplicated) and Lines (sorted, de-duplicated, but only when every merged
// edge shares one File — otherwise left empty, since a line number without a
// file would be misleading). An edge between two nodes that were already
// types (no method end at all) is passed through exactly as it was, even
// when several such edges share a key, since there was nothing to lift.
func LiftToTypes(g *Graph) *Graph {
	byID := make(map[string]*GraphNode, len(g.Nodes))
	for i := range g.Nodes {
		byID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	isMethod := func(id string) bool {
		n := byID[id]
		return n != nil && n.Kind == "method"
	}

	// containerOf: method id -> its containing node's id, from `contains`
	// edges (From = container, To = contained). First one wins when a method
	// is somehow contained more than once, for determinism.
	containerOf := map[string]string{}
	for _, e := range g.Edges {
		if e.Kind != "contains" || !isMethod(e.To) {
			continue
		}
		if _, ok := containerOf[e.To]; !ok {
			containerOf[e.To] = e.From
		}
	}
	liftedID := func(id string) string {
		if isMethod(id) {
			if c, ok := containerOf[id]; ok {
				return c
			}
		}
		return id
	}

	type mergeKey struct{ from, to, kind, typ string }
	type bucket struct {
		originals   []GraphEdge
		liftedAny   bool
		fromMethods map[string]bool
		toMethods   map[string]bool
		files       map[string]bool
		lines       map[int]bool
	}
	buckets := map[mergeKey]*bucket{}
	var order []mergeKey

	for _, e := range g.Edges {
		if e.Kind == "contains" && isMethod(e.To) {
			continue // methods, and the edges that reach them, are not lifted-answer nodes
		}
		from, to := liftedID(e.From), liftedID(e.To)
		lifted := from != e.From || to != e.To
		if lifted && from == to {
			continue // recursion inside one type disappears on lifting
		}
		key := mergeKey{from, to, e.Kind, e.Type}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{fromMethods: map[string]bool{}, toMethods: map[string]bool{}, files: map[string]bool{}, lines: map[int]bool{}}
			buckets[key] = b
			order = append(order, key)
		}
		b.originals = append(b.originals, e)
		if lifted {
			b.liftedAny = true
		}
		if isMethod(e.From) {
			if n := byID[e.From]; n != nil {
				b.fromMethods[n.Name] = true
			}
		}
		if isMethod(e.To) {
			if n := byID[e.To]; n != nil {
				b.toMethods[n.Name] = true
			}
		}
		if lines := edgeLines(e); len(lines) > 0 {
			if f := edgeCallFile(e, byID); f != "" {
				b.files[f] = true
			}
			for _, l := range lines {
				b.lines[l] = true
			}
		}
	}

	edges := make([]GraphEdge, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		if !b.liftedAny {
			// Nothing here was a method: pass every original edge through
			// unchanged, in the order they were found.
			edges = append(edges, b.originals...)
			continue
		}
		base := b.originals[0]
		merged := GraphEdge{
			From: key.from, To: key.to, Kind: key.kind, Type: key.typ,
			Presence: base.Presence, Count: len(b.originals),
			FromMethods: sortedStringSet(b.fromMethods),
			ToMethods:   sortedStringSet(b.toMethods),
		}
		if len(b.files) == 1 && len(b.lines) > 0 {
			lines := sortedIntSet(b.lines)
			var callFile string
			for f := range b.files {
				callFile = f
			}
			// GraphEdge.File is "only if different from file of `from`"
			// (EXTRACTOR.md §2.2): after lifting, `from` is the type, so the
			// call sites' actual file is only stored explicitly when it
			// differs from the type's own file — otherwise left implicit,
			// exactly like an ordinary unlifted calls/constructs edge.
			if fromNode := byID[key.from]; fromNode == nil || fromNode.File != callFile {
				merged.File = callFile
			}
			merged.Line = lines[0]
			if len(lines) > 1 {
				merged.Lines = lines
			}
		}
		edges = append(edges, merged)
	}

	nodes := make([]GraphNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.Kind == "method" {
			continue
		}
		nodes = append(nodes, n)
	}

	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Type < b.Type
	})

	return &Graph{Nodes: nodes, Edges: edges}
}

// edgeLines: every line an edge carries — Lines when set (a folded `calls`
// edge, ADR_20260928-4 §3), else just Line when it is set, else none.
func edgeLines(e GraphEdge) []int {
	if len(e.Lines) > 0 {
		return e.Lines
	}
	if e.Line != 0 {
		return []int{e.Line}
	}
	return nil
}

// edgeCallFile: the file a `calls`/`constructs` edge's line(s) actually lie
// in — e.File when the extractor printed one (it differed from the caller's
// own file), else the caller's (e.From's) own declared file (EXTRACTOR.md
// §2.2's "only if different from file of `from`" convention).
func edgeCallFile(e GraphEdge, byID map[string]*GraphNode) string {
	if e.File != "" {
		return e.File
	}
	if n := byID[e.From]; n != nil {
		return n.File
	}
	return ""
}

func sortedStringSet(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func sortedIntSet(m map[int]bool) []int {
	if len(m) == 0 {
		return nil
	}
	out := make([]int, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
