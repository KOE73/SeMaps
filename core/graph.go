// Graph joins extractor facts with a model into the live code graph:
// ADR_20260928_host_live-code-graph, docs/plans/PLAN_20260928_host_graph-provider.md
// step 1. It is read-only and derived: nothing here writes to the model or to
// disk, and nothing here is persisted — a fresh Graph is built from the same
// facts and model state every time.
package core

import (
	"encoding/json"
	"sort"
	"strings"
)

// FactsSource is one extractor's latest facts, as the host's graph service
// collects them (PLAN_20260928_host_graph-provider.md step 3): one project
// model may be fed by several extractors (`.semaps` → `extractors[].project`).
type FactsSource struct {
	Extractor string
	Facts     *Facts
}

// GraphNode is a symbol, an entity, or both, joined by entity.symbol
// (CONTRACT.md §3). Named GraphNode, not Node: core.Edge/core.Symbol already
// name the facts types this joins.
type GraphNode struct {
	ID         string   `json:"id"`
	Symbol     string   `json:"symbol,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	NativeKind string   `json:"nativeKind,omitempty"`
	Name       string   `json:"name,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
	File       string   `json:"file,omitempty"`
	Line       int      `json:"line,omitempty"`
	EndLine    int      `json:"endLine,omitempty"`
	Spans      []Span   `json:"spans,omitempty"` // every declaration, when there is more than one
	// MemberLines: member name -> line, in File (docs/EXTRACTOR.md §2.1).
	MemberLines map[string]int `json:"memberLines,omitempty"`
	Language    string         `json:"language,omitempty"`
	Extractor  string   `json:"extractor,omitempty"`
	Entity     string   `json:"entity,omitempty"`
	Status     string   `json:"status,omitempty"`
	Containers []string `json:"containers,omitempty"`
	// Presence: "both" (symbol and entity), "code" (symbol only), "model"
	// (entity only).
	Presence string `json:"presence"`
	// Members is the joined entity's own `members` field (docs/CONTRACT.md),
	// as sync already copied it from the extractor's facts — never
	// recomputed here. Filled only when asked (`fields=members`,
	// PLAN_20260928_host_graph-provider.md step 4) by AttachMembers.
	Members json.RawMessage `json:"members,omitempty"`
}

// GraphEdge is a fact edge, a registry relation, or both.
type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"` // facts vocabulary; of a model-only edge, the first segment of its type
	Type     string `json:"type"` // relation type as the registry names it: holds.many, injects, extends…
	Via      *Via   `json:"via,omitempty"`
	Line     int    `json:"line,omitempty"` // where the edge comes from, in File or in the file of `from`
	File     string `json:"file,omitempty"`
	Relation string `json:"relation,omitempty"` // registry relation id, or empty
	Presence string `json:"presence"`           // "both", "code" or "model"
}

// Graph is the join of extractor facts and a model, docs/adr/ADR_20260928 §2.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// nodeKey is the id a node gets in the graph. Symbol ids are only unique
// *within one extractor's output* (EXTRACTOR.md §2.1): a C# symbol id is a
// dotted metadata name with no "/" or "#" in it, while a TypeScript module
// id is a slash path with no "#" (declarations get one). A root-level C#
// namespace or type (e.g. `App`) and a root-level TypeScript module
// (`App.ts` → id `App`) can print the identical string, so two extractors
// feeding the same project model can collide on a bare symbol id. The node
// key is therefore always `<extractor>:<symbol>`, never the bare symbol.
func nodeKey(extractor, symbol string) string {
	return extractor + ":" + symbol
}

// BuildGraph joins facts from one or more extractors with a model's current
// working state (registry, not the contract files on disk — the same state
// Records/RegistrySnapshot expose).
func BuildGraph(sources []FactsSource, model *Model) (*Graph, error) {
	entities, err := parseObjects(model, "entities.json")
	if err != nil {
		return nil, err
	}
	relations, err := parseObjects(model, "relations.json")
	if err != nil {
		return nil, err
	}
	containers, err := LoadContainers(model.ProjectDir())
	if err != nil {
		return nil, err
	}
	var defs []Container
	overrides := map[string]string{}
	if containers != nil {
		defs, overrides = containers.List, containers.Overrides
	}

	// entity by symbol: first entity claiming a symbol wins (CONTRACT §3's
	// invariant says this map has at most one entry per symbol among
	// non-authored entities; a broken registry is not this function's job to
	// catch — sync/check already do).
	entityBySymbol := map[string]*object{}
	for _, e := range entities {
		if e.str("origin") == "authored" {
			continue
		}
		if s := e.str("symbol"); s != "" {
			if _, ok := entityBySymbol[s]; !ok {
				entityBySymbol[s] = e
			}
		}
	}

	nodes := []GraphNode{}
	subjects := make([]ContainerSubject, 0, len(entities))
	nodeIndexBySubject := map[string]int{} // subject id -> index in nodes
	entityNodeKey := map[string]string{}   // entity id -> node key
	consumed := map[*object]bool{}         // entities joined to a symbol

	// -------------------------------------------------- code and both nodes
	symbolNodeKey := map[[2]string]string{} // (extractor, symbol id) -> node key
	for _, src := range sources {
		for _, s := range src.Facts.Symbols {
			key := nodeKey(src.Extractor, s.ID)
			n := GraphNode{
				ID: key, Symbol: s.ID, Kind: s.Kind, NativeKind: s.NativeKind,
				Name: s.Name, Namespace: s.Namespace, Visibility: s.Visibility,
				File: s.File, Line: s.Line, EndLine: s.EndLine, Spans: s.Spans, MemberLines: s.MemberLines, Language: src.Facts.Language, Extractor: src.Extractor,
			}
			subjectID := key
			if e := entityBySymbol[s.ID]; e != nil && !consumed[e] {
				consumed[e] = true
				n.Entity, n.Status, n.Presence = e.str("id"), e.str("status"), "both"
				entityNodeKey[e.str("id")] = key
				subjectID = e.str("id") // so overrides (keyed by entity id) apply
			} else {
				n.Presence = "code"
			}
			nodes = append(nodes, n)
			nodeIndexBySubject[subjectID] = len(nodes) - 1
			subjects = append(subjects, ContainerSubject{ID: subjectID, Name: n.Name, File: n.File})
			symbolNodeKey[[2]string{src.Extractor, s.ID}] = key
		}
	}

	// ------------------------------------------------------- model-only nodes
	for _, e := range entities {
		if consumed[e] {
			continue
		}
		id, name, file := e.str("id"), e.str("name"), codeRefFile(e.str("codeRef"))
		n := GraphNode{ID: id, Kind: e.str("kind"), Name: name, Status: e.str("status"), Presence: "model"}
		if ns := e.str("namespace"); ns != "" {
			n.Namespace = ns
		}
		if file != "" {
			n.File = file
		}
		entityNodeKey[id] = id
		nodes = append(nodes, n)
		nodeIndexBySubject[id] = len(nodes) - 1
		subjects = append(subjects, ContainerSubject{ID: id, Name: name, File: file})
	}

	// ------------------------------------------------------------ containers
	resolved := ResolveContainers(defs, overrides, subjects)
	for subjectID, idx := range nodeIndexBySubject {
		nodes[idx].Containers = resolved[subjectID]
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	// ------------------------------------------------------------------ edges
	relByKey := map[string][]int{}
	for i, r := range relations {
		relByKey[relationKey(r)] = append(relByKey[relationKey(r)], i)
	}
	confirmed := map[int]bool{}

	edges := []GraphEdge{}
	for _, src := range sources {
		for _, e := range src.Facts.Edges {
			from, ok1 := symbolNodeKey[[2]string{src.Extractor, e.From}]
			to, ok2 := symbolNodeKey[[2]string{src.Extractor, e.To}]
			if !ok1 || !ok2 {
				continue // never happens: facts.Validate already requires this
			}
			relType := e.Kind
			if (e.Kind == "holds" || e.Kind == "uses") && e.Via != nil {
				relType = deriveRelationType(e.Kind, e.Via)
			}
			ge := GraphEdge{From: from, To: to, Kind: e.Kind, Type: relType, Via: e.Via, Line: e.Line, File: e.File, Presence: "code"}

			fromEnt, toEnt := entityBySymbol[e.From], entityBySymbol[e.To]
			if fromEnt != nil && toEnt != nil {
				var key string
				if e.Kind == "holds" || e.Kind == "uses" {
					key = memberRelationKey(fromEnt.str("id"), toEnt.str("id"), relType, e.Via)
				} else {
					key = triple(fromEnt.str("id"), toEnt.str("id"), relType)
				}
				if idxs := relByKey[key]; len(idxs) > 0 {
					ge.Relation, ge.Presence = relations[idxs[0]].str("id"), "both"
					for _, i := range idxs {
						confirmed[i] = true
					}
				}
			}
			edges = append(edges, ge)
		}
	}

	// ------------------------------------------------------ model-only edges
	for i, r := range relations {
		if confirmed[i] {
			continue
		}
		from, ok1 := entityNodeKey[r.str("from")]
		to, ok2 := entityNodeKey[r.str("to")]
		if !ok1 || !ok2 {
			continue // dangling relation; not this function's job to flag
		}
		var via *Via
		if raw, ok := r.vals["via"]; ok && len(raw) > 0 && string(raw) != "null" {
			var v Via
			if json.Unmarshal(raw, &v) == nil {
				via = &v
			}
		}
		relType := relationType(r)
		kind, _, _ := strings.Cut(relType, ".")
		edges = append(edges, GraphEdge{From: from, To: to, Kind: kind, Type: relType, Via: via, Relation: r.str("id"), Presence: "model"})
	}

	sort.Slice(edges, func(i, j int) bool { return compareGraphEdges(edges[i], edges[j]) < 0 })

	return &Graph{Nodes: nodes, Edges: edges}, nil
}

func compareGraphEdges(a, b GraphEdge) int {
	for _, pair := range [][2]string{{a.From, b.From}, {a.To, b.To}, {a.Kind, b.Kind}} {
		if c := strings.Compare(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	aMember, bMember := "", ""
	if a.Via != nil {
		aMember = a.Via.Member
	}
	if b.Via != nil {
		bMember = b.Via.Member
	}
	if c := strings.Compare(aMember, bMember); c != 0 {
		return c
	}
	aPath, bPath := "", ""
	if a.Via != nil {
		aPath = strings.Join(a.Via.Path, ",")
	}
	if b.Via != nil {
		bPath = strings.Join(b.Via.Path, ",")
	}
	return strings.Compare(aPath, bPath)
}

// AttachMembers fills GraphNode.Members for every node joined to an entity,
// from entities.json's own `members` field: sync already copies the
// extractor's `members` there (docs/CONTRACT.md), so this reads it back
// rather than re-deriving it from facts. A node with no entity, or an
// entity with no `members`, is left as is.
func AttachMembers(graph *Graph, model *Model) error {
	entities, err := parseObjects(model, "entities.json")
	if err != nil {
		return err
	}
	byID := map[string]*object{}
	for _, e := range entities {
		byID[e.str("id")] = e
	}
	for i, n := range graph.Nodes {
		if n.Entity == "" {
			continue
		}
		e := byID[n.Entity]
		if e == nil {
			continue
		}
		if raw, ok := e.vals["members"]; ok && len(raw) > 0 && string(raw) != "null" {
			graph.Nodes[i].Members = raw
		}
	}
	return nil
}

// parseObjects reads one registry file's items as ordered objects, the same
// shape sync.go and check.go work with, so this package's key helpers
// (relationKey, relationType, triple, memberRelationKey…) can be reused
// as-is instead of copied onto a second, typed representation.
func parseObjects(model *Model, file string) ([]*object, error) {
	raws, err := model.Records(file)
	if err != nil {
		return nil, err
	}
	out := make([]*object, len(raws))
	for i, raw := range raws {
		o := newObject()
		if err := json.Unmarshal(raw, o); err != nil {
			return nil, err
		}
		out[i] = o
	}
	return out, nil
}
