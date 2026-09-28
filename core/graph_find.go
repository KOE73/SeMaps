// Finding a node by name (part 2 of the agent-answers rework, then defects
// 8/9/11 of the agent-answers-graph task): `around` accepts a full id or a
// name, and the standalone find endpoint/tool does a plain substring search.
// Pure functions, host/mcp.go only parses/formats.
package core

import (
	"regexp"
	"sort"
	"strings"
)

// NodeMatch is one candidate, as listed to a caller that must pick among
// several, or search by substring.
type NodeMatch struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

func toMatch(n *GraphNode) NodeMatch {
	return NodeMatch{ID: n.ID, Kind: n.Kind, File: n.File, Line: n.Line}
}

// NodeResolution is what ResolveNode found for one query string.
type NodeResolution struct {
	// Node is set only when resolution found exactly one candidate.
	Node *GraphNode
	// Notice is non-empty whenever the resolved id differs from the literal
	// query text — "what was asked, what was taken" (part 2, then defect 8:
	// it says WHAT happened, not just "no such id"). Empty on an exact id
	// hit.
	Notice string
	// MissingHidden is true when the sole candidate exists but is a
	// model-only node with status "missing" (hidden by `missing=0`, the
	// default): the caller should refuse it and say so, not silently use it.
	MissingHidden bool
	// Candidates is filled (len > 1) when the query is ambiguous.
	Candidates []NodeMatch
	// Suggestions is filled when nothing matched: up to 5 short names
	// nearest by edit distance, each with its full id (defect 9).
	Suggestions []string
}

var arityPattern = regexp.MustCompile("`\\d+$")

func stripArity(s string) string {
	return arityPattern.ReplaceAllString(s, "")
}

func idWithoutExtractorPrefix(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
}

// isType: kind/interface/module/function/value — everything ResolveNode and
// FindNodes try BEFORE a method (defect 11: with ~1900 methods in a real
// project's graph, `Runner` must resolve to the type `Runner`, never an
// ambiguity with a constructor or a method of a similar name).
func isType(n *GraphNode) bool { return n.Kind != "method" }

// looksLikeMethodQuery: the asked text is method-shaped — it names a call
// (`(`) or has the `Type.Member` form (a dot, with the part after the last
// one starting like an identifier) — the two cases defect 11 says a method
// is tried for even when a type also happens to match, or instead of one.
func looksLikeMethodQuery(q string) bool {
	if strings.Contains(q, "(") {
		return true
	}
	if i := strings.LastIndexByte(q, '.'); i > 0 && i < len(q)-1 {
		return true
	}
	return false
}

// matchKey identifies which of resolveKeys/methodKeys matched, and at which
// case sensitivity, so ResolveNode can word its notice precisely (defect 8)
// instead of a blanket "no such id".
type matchKey int

const (
	keyID matchKey = iota
	keyIDNoPrefix
	keyFullNameNoArity
	keyName
	keyNameNoArity
	keyMethodSignature
	keyMethodTypeMember
)

// resolveKeys: the resolution order of part 2, exact id first, then
// progressively looser names. Each is tried case-sensitively across all
// keys, then (only if nothing at all matched) case-insensitively.
func resolveKeys() []struct {
	key matchKey
	f   func(*GraphNode) string
} {
	return []struct {
		key matchKey
		f   func(*GraphNode) string
	}{
		{keyID, func(n *GraphNode) string { return n.ID }},
		{keyIDNoPrefix, func(n *GraphNode) string { return idWithoutExtractorPrefix(n.ID) }},
		{keyFullNameNoArity, func(n *GraphNode) string { return stripArity(FullName(n)) }},
		{keyName, func(n *GraphNode) string { return n.Name }},
		{keyNameNoArity, func(n *GraphNode) string { return stripArity(n.Name) }},
	}
}

// ResolveNode resolves `query` against every node of `g` (callers pass the
// graph including nodes hidden by `missing=0`, so a hit on a missing node
// can be reported as such rather than silently treated as "no such node").
// Types/interfaces/modules (and function/value symbols) are tried before
// methods; a method is tried only when nothing else matched, or when the
// query is method-shaped (defect 11).
func ResolveNode(g *Graph, query string) NodeResolution {
	if query == "" {
		return NodeResolution{}
	}
	byID := nodeByID(g)
	methodOf := methodContainerMap(g)

	methodShaped := looksLikeMethodQuery(query)

	if methodShaped {
		if res, ok := resolveMethods(g, query, byID, methodOf); ok {
			return res
		}
	}
	if res, ok := resolveAmong(g, query, isType); ok {
		return res
	}
	if !methodShaped {
		if res, ok := resolveMethods(g, query, byID, methodOf); ok {
			return res
		}
	}
	return NodeResolution{Suggestions: nearestNames(g, query, byID, methodOf, 5)}
}

// resolveAmong tries resolveKeys() against every node for which `keep`
// returns true, case-sensitively then (only if nothing matched at all)
// case-insensitively, and builds the defect-8 notice from which key/case
// level actually matched.
func resolveAmong(g *Graph, query string, keep func(*GraphNode) bool) (NodeResolution, bool) {
	keys := resolveKeys()
	for _, ci := range []bool{false, true} {
		q := query
		if ci {
			q = strings.ToLower(q)
		}
		for _, k := range keys {
			var matches []*GraphNode
			seen := map[string]bool{}
			for i := range g.Nodes {
				n := &g.Nodes[i]
				if !keep(n) {
					continue
				}
				v := k.f(n)
				if v == "" {
					continue
				}
				if ci {
					v = strings.ToLower(v)
				}
				if v == q && !seen[n.ID] {
					seen[n.ID] = true
					matches = append(matches, n)
				}
			}
			if len(matches) == 0 {
				continue
			}
			if len(matches) > 1 {
				out := make([]NodeMatch, len(matches))
				for i, m := range matches {
					out[i] = toMatch(m)
				}
				sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
				return NodeResolution{Candidates: out}, true
			}
			m := matches[0]
			res := NodeResolution{Node: m, Notice: notice(query, m, k.key, ci)}
			if m.Presence == "model" && m.Status == "missing" {
				res.MissingHidden = true
			}
			return res, true
		}
	}
	return NodeResolution{}, false
}

// notice: what happened (defect 8) — empty for an exact id hit; otherwise
// says what kind of match this was.
func notice(query string, m *GraphNode, key matchKey, caseInsensitive bool) string {
	if m.ID == query {
		return ""
	}
	switch {
	case caseInsensitive:
		return "asked " + quote(query) + " (case differs); taken " + quote(m.ID)
	case key == keyNameNoArity && m.Name != stripArity(m.Name):
		return "asked " + quote(query) + "; no " + quote(query) + " without type parameters; taken " + quote(m.ID)
	case key == keyFullNameNoArity && FullName(m) != stripArity(FullName(m)):
		return "asked " + quote(query) + "; no " + quote(query) + " without type parameters; taken " + quote(m.ID)
	case key == keyName || key == keyNameNoArity:
		return "asked " + quote(query) + " (a short name); taken " + quote(m.ID)
	case key == keyMethodSignature || key == keyMethodTypeMember:
		return "asked " + quote(query) + " (a method); taken " + quote(m.ID)
	default:
		return "asked " + quote(query) + ", no such id; taken " + quote(m.ID)
	}
}

func quote(s string) string { return "`" + s + "`" }

// resolveMethods tries a query against method-kind nodes only, by their
// printed short signature (methodDisplayName — what facts/lines/tree print,
// and what must resolve back, defect 11) and, for the coarser `Type.Member`
// form with no parameter list, by container-short-name + method name alone
// (several overloads there are all returned as candidates).
func resolveMethods(g *Graph, query string, byID map[string]*GraphNode, methodOf map[string]string) (NodeResolution, bool) {
	for _, ci := range []bool{false, true} {
		q := query
		if ci {
			q = strings.ToLower(q)
		}
		// exact printed short signature (round-trips a printed name).
		if res, ok := matchMethods(g, q, ci, byID, methodOf, keyMethodSignature, func(n *GraphNode) string {
			return methodDisplayName(n, byID, methodOf)
		}); ok {
			return res, true
		}
		// Type.Member, no parameter list: every overload matches.
		if res, ok := matchMethods(g, q, ci, byID, methodOf, keyMethodTypeMember, func(n *GraphNode) string {
			return methodTypeMember(n, byID, methodOf)
		}); ok {
			return res, true
		}
	}
	return NodeResolution{}, false
}

func matchMethods(g *Graph, q string, ci bool, byID map[string]*GraphNode, methodOf map[string]string, key matchKey, keyFn func(*GraphNode) string) (NodeResolution, bool) {
	var matches []*GraphNode
	seen := map[string]bool{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Kind != "method" {
			continue
		}
		v := keyFn(n)
		if v == "" {
			continue
		}
		if ci {
			v = strings.ToLower(v)
		}
		if v == q && !seen[n.ID] {
			seen[n.ID] = true
			matches = append(matches, n)
		}
	}
	if len(matches) == 0 {
		return NodeResolution{}, false
	}
	if len(matches) > 1 {
		out := make([]NodeMatch, len(matches))
		for i, m := range matches {
			out[i] = toMatch(m)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return NodeResolution{Candidates: out}, true
	}
	m := matches[0]
	res := NodeResolution{Node: m, Notice: notice(q, m, key, ci)}
	if m.Presence == "model" && m.Status == "missing" {
		res.MissingHidden = true
	}
	return res, true
}

// nearestNames: up to `limit` short-name suggestions ("name (fullID)"),
// nearest to `query` by edit distance, types first (defect 9). A candidate
// is kept only when its distance is within a third of the asked name's
// length (at least 1, at most 3); methods are only considered when the
// query is method-shaped or no type matched at all — since this is only
// called once every other resolution attempt failed, that condition is
// already satisfied, so every kind is eligible here.
func nearestNames(g *Graph, query string, byID map[string]*GraphNode, methodOf map[string]string, limit int) []string {
	maxDist := len(query) / 3
	if maxDist < 1 {
		maxDist = 1
	}
	if maxDist > 3 {
		maxDist = 3
	}
	type scored struct {
		short  string
		full   string
		dist   int
		isType bool
	}
	seen := map[string]bool{}
	var all []scored
	q := strings.ToLower(query)
	for i := range g.Nodes {
		n := &g.Nodes[i]
		short := n.Name
		if n.Kind == "method" {
			short = methodTypeMember(n, byID, methodOf)
		}
		short = stripArity(short)
		if short == "" || seen[short+"|"+n.ID] {
			continue
		}
		seen[short+"|"+n.ID] = true
		dist := levenshtein(q, strings.ToLower(short))
		if dist > maxDist {
			continue
		}
		all = append(all, scored{short: short, full: n.ID, dist: dist, isType: isType(n)})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].isType != all[j].isType {
			return all[i].isType // types first
		}
		if all[i].dist != all[j].dist {
			return all[i].dist < all[j].dist
		}
		return all[i].short < all[j].short
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.short + " (" + s.full + ")"
	}
	return out
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			cur[j] = m
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// FindNodes is the plain substring search behind `GET
// /api/graph/{project}/find` and the `find_node` MCP tool: every node whose
// id or name contains `q`, case-insensitively, up to `limit` (0 = 50).
// Types/interfaces/modules are listed before methods (defect 11).
func FindNodes(g *Graph, q string, limit int) []NodeMatch {
	if limit <= 0 {
		limit = 50
	}
	ql := strings.ToLower(q)
	var out []NodeMatch
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if ql == "" || strings.Contains(strings.ToLower(n.ID), ql) || strings.Contains(strings.ToLower(FullName(n)), ql) {
			out = append(out, toMatch(n))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].Kind != "method", out[j].Kind != "method"
		return ti && !tj
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ------------------------------------------------------- method short names

// methodContainerMap: method id -> its containing node's id, from `contains`
// edges (From = container, To = the method). Shared by resolution and by
// the method display name printed in facts/lines/tree.
func methodContainerMap(g *Graph) map[string]string {
	byID := nodeByID(g)
	out := map[string]string{}
	for _, e := range g.Edges {
		if e.Kind != "contains" {
			continue
		}
		n := byID[e.To]
		if n == nil || n.Kind != "method" {
			continue
		}
		if _, ok := out[e.To]; !ok {
			out[e.To] = e.From
		}
	}
	return out
}

// namespacePrefixPattern strips a dotted namespace/type qualifier
// (`Foo.Bar.Baz` -> `Baz`), applied globally so it also shortens generic
// type arguments (`System.Collections.Generic.List<System.String>` ->
// `List<String>`).
var namespacePrefixPattern = regexp.MustCompile(`(?:[A-Za-z_][A-Za-z0-9_]*\.)+`)

func shortenTypeRef(s string) string { return namespacePrefixPattern.ReplaceAllString(s, "") }

// containerShortName: the short name of the node containing method `n`, via
// methodContainerMap, or "" when it cannot be found (contains edges dropped
// by `fields`, or a method with no discoverable container).
func containerShortName(n *GraphNode, byID map[string]*GraphNode, methodOf map[string]string) string {
	if c, ok := methodOf[n.ID]; ok {
		if cn := byID[c]; cn != nil {
			return cn.Name
		}
	}
	return ""
}

// methodDisplayName is the short signature a method prints as in
// facts/lines/tree (defect 11): `Container.Name\`arity(ShortParamTypes)`,
// never the full id (namespace-qualified container, full parameter type
// names — up to 250 characters). Falls back to the bare id when the
// container cannot be found.
func methodDisplayName(n *GraphNode, byID map[string]*GraphNode, methodOf map[string]string) string {
	container := containerShortName(n, byID, methodOf)
	if container == "" {
		return n.ID
	}
	name, params := splitMethodSignature(n)
	if params == "" && !strings.Contains(n.Symbol, "(") {
		return container + "." + name
	}
	return container + "." + name + "(" + shortenTypeRef(params) + ")"
}

// methodTypeMember is the coarser `Type.Member` form (no parameter list) —
// what several overloads share, and the form resolveMethods matches a
// query like `YoloObbFactory.CreateRunner` against.
func methodTypeMember(n *GraphNode, byID map[string]*GraphNode, methodOf map[string]string) string {
	container := containerShortName(n, byID, methodOf)
	name, _ := splitMethodSignature(n)
	name = stripArity(name)
	if container == "" {
		return name
	}
	return container + "." + name
}

// splitMethodSignature pulls the method's own name (with generic arity, as
// in the id) and its raw parameter-type text out of its id
// (`<typeID>.<name>(<paramTypes>)`, EXTRACTOR.md §2.1) — falling back to the
// symbol's plain Name when the id doesn't parse as expected.
func splitMethodSignature(n *GraphNode) (name, params string) {
	id := n.Symbol
	if id == "" {
		id = n.ID
	}
	body := id
	if open := strings.LastIndexByte(id, '('); open >= 0 && strings.HasSuffix(id, ")") {
		body = id[:open]
		params = id[open+1 : len(id)-1]
	}
	if dot := strings.LastIndexByte(body, '.'); dot >= 0 {
		name = body[dot+1:]
	} else {
		name = body
	}
	if name == "" {
		name = n.Name
	}
	return name, params
}
