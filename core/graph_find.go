// Finding a node by name (part 2 of the agent-answers rework): `around`
// accepts a full id or a name, and the standalone find endpoint/tool does a
// plain substring search. Pure functions, host/mcp.go only parses/formats.
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
	// query text — "what was asked, what was taken" (part 2). Empty on an
	// exact id hit.
	Notice string
	// MissingHidden is true when the sole candidate exists but is a
	// model-only node with status "missing" (hidden by `missing=0`, the
	// default): the caller should refuse it and say so, not silently use it.
	MissingHidden bool
	// Candidates is filled (len > 1) when the query is ambiguous.
	Candidates []NodeMatch
	// Suggestions is filled when nothing matched: up to 5 names nearest by
	// edit distance.
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

// resolveKeys: the resolution order of part 2, exact id first, then
// progressively looser names. Each is tried case-sensitively across all
// keys, then (only if nothing at all matched) case-insensitively.
func resolveKeys() []func(*GraphNode) string {
	return []func(*GraphNode) string{
		func(n *GraphNode) string { return n.ID },
		func(n *GraphNode) string { return idWithoutExtractorPrefix(n.ID) },
		func(n *GraphNode) string { return stripArity(FullName(n)) },
		func(n *GraphNode) string { return n.Name },
		func(n *GraphNode) string { return stripArity(n.Name) },
	}
}

// ResolveNode resolves `query` against every node of `g` (callers pass the
// graph including nodes hidden by `missing=0`, so a hit on a missing node
// can be reported as such rather than silently treated as "no such node").
func ResolveNode(g *Graph, query string) NodeResolution {
	if query == "" {
		return NodeResolution{}
	}
	keys := resolveKeys()
	for _, ci := range []bool{false, true} {
		q := query
		if ci {
			q = strings.ToLower(q)
		}
		for _, key := range keys {
			seen := map[string]bool{}
			var matches []*GraphNode
			for i := range g.Nodes {
				n := &g.Nodes[i]
				k := key(n)
				if k == "" {
					continue
				}
				if ci {
					k = strings.ToLower(k)
				}
				if k == q && !seen[n.ID] {
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
				return NodeResolution{Candidates: out}
			}
			m := matches[0]
			res := NodeResolution{Node: m}
			if m.ID != query {
				res.Notice = "asked " + quote(query) + ", no such id; taken " + quote(m.ID)
			}
			if m.Presence == "model" && m.Status == "missing" {
				res.MissingHidden = true
			}
			return res
		}
	}
	return NodeResolution{Suggestions: nearestNames(g, query, 5)}
}

func quote(s string) string { return "`" + s + "`" }

// nearestNames: up to `limit` distinct full names of `g`, nearest to `query`
// by Levenshtein edit distance, closest first (ties broken alphabetically).
func nearestNames(g *Graph, query string, limit int) []string {
	type scored struct {
		name string
		dist int
	}
	seen := map[string]bool{}
	var all []scored
	q := strings.ToLower(query)
	for i := range g.Nodes {
		n := &g.Nodes[i]
		name := FullName(n)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		dist := levenshtein(q, strings.ToLower(name))
		if n.Name != "" && n.Name != name {
			if d2 := levenshtein(q, strings.ToLower(n.Name)); d2 < dist {
				dist = d2
			}
		}
		all = append(all, scored{name, dist})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].dist != all[j].dist {
			return all[i].dist < all[j].dist
		}
		return all[i].name < all[j].name
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.name
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
func FindNodes(g *Graph, q string, limit int) []NodeMatch {
	if limit <= 0 {
		limit = 50
	}
	q = strings.ToLower(q)
	var out []NodeMatch
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if q == "" || strings.Contains(strings.ToLower(n.ID), q) || strings.Contains(strings.ToLower(FullName(n)), q) {
			out = append(out, toMatch(n))
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
