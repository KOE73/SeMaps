package core

import (
	"bytes"
	"encoding/json"
	"sort"
)

// jsonFormat is what GET /api/graph/{project} and get_graph have always
// returned: unchanged, and it stays the default of the HTTP endpoint.
type jsonFormat struct{}

func (jsonFormat) Name() string { return "json" }
func (jsonFormat) Description() string {
	return "the full graph as JSON: one object per node/edge, nothing shortened"
}
func (jsonFormat) MediaType() string { return "application/json; charset=utf-8" }

func (jsonFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	g = StripFields(g, o.Fields)
	m := map[string]any{"nodes": g.Nodes, "edges": g.Edges, "facts": o.Facts, "stats": o.Stats}
	if o.Truncated {
		m["truncated"] = true
		m["fullNodes"], m["fullEdges"] = o.FullNodes, o.FullEdges
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// jsonCompactFormat: JSON with nothing repeated. `language`/`extractor` are
// the same for every node of one project's graph, so they go once at the
// head, not per node. A node's `symbol` already contains its `namespace` as
// a prefix, so namespace is left out of the compact node (it is still the
// full node's own field — this format just does not repeat it). File paths
// go in a table (`files`), referenced by index (`fi`), so a path used by 50
// nodes is written once. Edges refer to nodes by index into `nodes`, not by
// the (longer, repeated) id string.
type jsonCompactFormat struct{}

func (jsonCompactFormat) Name() string { return "json-compact" }
func (jsonCompactFormat) Description() string {
	return "JSON with short keys, no repeated fields, file paths and node ids interned"
}
func (jsonCompactFormat) MediaType() string { return "application/json; charset=utf-8" }

// compactNode is one node's shape in json-compact. Short keys, chosen so an
// agent can still guess them: i=id, s=symbol, k=kind, n=name, fi=file index,
// l=line, el=endLine, c=containers, e=entity, st=status, p=presence.
type compactNode struct {
	I  string   `json:"i"`
	S  string   `json:"s,omitempty"`
	K  string   `json:"k,omitempty"`
	N  string   `json:"n,omitempty"`
	FI int      `json:"fi,omitempty"`
	L  int      `json:"l,omitempty"`
	EL int      `json:"el,omitempty"`
	C  []string `json:"c,omitempty"`
	E  string   `json:"e,omitempty"`
	St string   `json:"st,omitempty"`
	P  string   `json:"p,omitempty"`
}

// compactEdge: f/t are node ids (not indexes: an agent misreads an index
// across a large answer — part 1 of the agent-answers rework).
type compactEdge struct {
	F  string `json:"f"`
	T  string `json:"t"`
	K  string `json:"k"`
	Ty string `json:"ty,omitempty"`
	Vi *Via   `json:"vi,omitempty"`
	FI int    `json:"fi,omitempty"`
	L  int    `json:"l,omitempty"`
	R  string `json:"r,omitempty"`
	St string `json:"st,omitempty"`
	P  string `json:"p,omitempty"`
}

func (jsonCompactFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	g = StripFields(g, o.Fields)

	// files table: 0 means "no file", so real files start at index 1.
	fileIndex := map[string]int{}
	files := []string{}
	fileIdx := func(f string) int {
		if f == "" {
			return 0
		}
		if i, ok := fileIndex[f]; ok {
			return i
		}
		files = append(files, f)
		i := len(files) // 1-based
		fileIndex[f] = i
		return i
	}

	nodes := make([]compactNode, len(g.Nodes))
	languages := map[string]bool{}
	extractors := map[string]bool{}
	for i, n := range g.Nodes {
		nodes[i] = compactNode{
			I: n.ID, S: n.Symbol, K: n.Kind, N: n.Name,
			FI: fileIdx(n.File), L: n.Line, EL: n.EndLine,
			C: n.Containers, E: n.Entity, St: n.Status, P: n.Presence,
		}
		if n.Language != "" {
			languages[n.Language] = true
		}
		if n.Extractor != "" {
			extractors[n.Extractor] = true
		}
	}

	edges := make([]compactEdge, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = compactEdge{
			F: e.From, T: e.To, K: e.Kind, Ty: e.Type,
			Vi: e.Via, FI: fileIdx(e.File), L: e.Line, R: e.Relation, St: e.Status, P: e.Presence,
		}
	}

	m := map[string]any{
		"language":  boolMapKeys(languages),
		"extractor": boolMapKeys(extractors),
		"files":     files,
		"nodes":     nodes,
		"edges":     edges,
		"facts":     o.Facts,
		"stats":     o.Stats,
	}
	if o.Truncated {
		m["truncated"] = true
		m["fullNodes"], m["fullEdges"] = o.FullNodes, o.FullEdges
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func boolMapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
