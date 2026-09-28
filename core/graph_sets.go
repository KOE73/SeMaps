// Named sets of edge kinds a graph request can walk or keep, in one place so
// a new set is one entry here (docs/API.md §5/§6, part 1 of the formatter
// task). GraphEdge.Kind is always one of extends, implements, holds, uses,
// depends, contains (docs/EXTRACTOR.md §2); a set names a subset of those.
package core

import (
	"fmt"
	"sort"
	"strings"
)

// RelationSet is one named, described subset of edge kinds.
type RelationSet struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kinds       []string `json:"kinds"`
}

// relationSets is the registry, in declaration order (also the order they
// are listed to a caller). Add a new set here and nowhere else.
var relationSets = []RelationSet{
	{
		Name:        "links",
		Description: "direct references: extends, implements, holds, uses, depends — the default for a neighbourhood",
		Kinds:       []string{"extends", "implements", "holds", "uses", "depends"},
	},
	{
		Name:        "inheritance",
		Description: "extends, implements",
		Kinds:       []string{"extends", "implements"},
	},
	{
		Name:        "members",
		Description: "holds, uses",
		Kinds:       []string{"holds", "uses"},
	},
	{
		Name:        "containment",
		Description: "contains: who is inside a namespace/assembly, and where a thing lies",
		Kinds:       []string{"contains"},
	},
	{
		Name:        "all",
		Description: "every edge kind",
		Kinds:       []string{"extends", "implements", "holds", "uses", "depends", "contains"},
	},
}

// DefaultNeighborhoodSet is the set a neighbourhood request (`around`) uses
// when neither `set` nor `kinds` was given: `contains` is not walked, so an
// interface's neighbourhood does not pull in its whole assembly
// (docs/plans/PLAN_20260928_host_graph-provider.md, "Экономия для агента").
const DefaultNeighborhoodSet = "links"

// DefaultWholeGraphSet is what a whole-graph request (no `around`) keeps
// using when neither `set` nor `kinds` was given: "all", unchanged from
// before named sets existed — the graph page depends on this.
const DefaultWholeGraphSet = "all"

// RelationSets lists the registry, in declaration order, for a caller that
// wants to show what is available (GET /api/graph-formats, graph_formats).
func RelationSets() []RelationSet {
	out := make([]RelationSet, len(relationSets))
	copy(out, relationSets)
	return out
}

// GetRelationSet looks a set up by name.
func GetRelationSet(name string) (RelationSet, bool) {
	for _, s := range relationSets {
		if s.Name == name {
			return s, true
		}
	}
	return RelationSet{}, false
}

func relationSetNames() string {
	names := make([]string, len(relationSets))
	for i, s := range relationSets {
		names[i] = s.Name
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ResolveEdgeKinds turns the `set`/`kinds` request parameters into the edge
// kinds to filter on, or nil for "no filtering" (every kind kept, exactly
// today's behaviour with neither parameter given). Both `set` and explicit
// `kinds` given is an error naming the conflict. An unknown `set` name is an
// error naming the available ones. `around` says whether this resolution is
// for a neighbourhood walk (its default set is DefaultNeighborhoodSet) or a
// whole-graph request (its default is "no filtering": DefaultWholeGraphSet
// applied would be a no-op anyway, since it lists every kind that exists,
// but the graph page must see literally the same request path as before, so
// we take the no-op shortcut and never call FilterEdgeKinds in that case).
func ResolveEdgeKinds(set string, kinds []string, around bool) ([]string, error) {
	if set != "" && len(kinds) > 0 {
		return nil, fmt.Errorf("set and kinds both given: name one (set=%q or kinds=%q)", set, strings.Join(kinds, ","))
	}
	if set != "" {
		rs, ok := GetRelationSet(set)
		if !ok {
			return nil, fmt.Errorf("set: %q is not one of: %s", set, relationSetNames())
		}
		return rs.Kinds, nil
	}
	if len(kinds) > 0 {
		return kinds, nil
	}
	if around {
		rs, _ := GetRelationSet(DefaultNeighborhoodSet)
		return rs.Kinds, nil
	}
	return nil, nil // whole graph, no set/kinds given: unchanged behaviour
}
