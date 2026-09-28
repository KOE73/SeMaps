// The graph's relation vocabulary (part 1 of the agent-answers rework,
// docs/plans/PLAN_20260928-5_host_graph-answers-for-agents.md step 3):
// every relation a `follow` request can walk, named once per direction, in
// ONE table. Replaces the named sets of core/graph_sets.go (removed): a
// neighbourhood walk now names exactly the relations and directions it
// wants, never an undirected "kind".
package core

import (
	"fmt"
	"sort"
	"strings"
)

// Relation is one direction of one named relation. `Kind` is the
// GraphEdge.Kind it walks; `TypeMatch`, when set, narrows to edges whose
// Type is that value or has it as a dotted prefix (docs/EXTRACTOR.md's
// `holds.*` nuance) — "" means every Type of that Kind counts (so `holds`
// covers `holds.one`, `holds.many`, ... at once, while `uses` and `injects`,
// which share Kind "uses", must each name their own Type).
type Relation struct {
	Name        string
	Inverse     string
	Kind        string
	TypeMatch   string
	Forward     bool // true: this name reads the edge as From (the node in hand) -> To; false: To -> From
	Description string
}

// relationVocabulary is the one table part 1 asks for. Add a relation here
// and nowhere else; every row's inverse is also its own row.
var relationVocabulary = []Relation{
	{Name: "extends", Inverse: "extended-by", Kind: "extends", Forward: true, Description: "a type extends its base type"},
	{Name: "extended-by", Inverse: "extends", Kind: "extends", Forward: false, Description: "a type is the base that another extends"},

	{Name: "implements", Inverse: "implemented-by", Kind: "implements", Forward: true, Description: "a type implements an interface, or a method implements an interface method"},
	{Name: "implemented-by", Inverse: "implements", Kind: "implements", Forward: false, Description: "an interface (or interface method) is implemented"},

	{Name: "overrides", Inverse: "overridden-by", Kind: "overrides", Forward: true, Description: "a method overrides a base virtual method"},
	{Name: "overridden-by", Inverse: "overrides", Kind: "overrides", Forward: false, Description: "a base virtual method is overridden by another"},

	{Name: "holds", Inverse: "held-by", Kind: "holds", Forward: true, Description: "a field/property/parameter holds a value of the target type, any cardinality"},
	{Name: "held-by", Inverse: "holds", Kind: "holds", Forward: false, Description: "a type is held by a field/property/parameter"},
	{Name: "holds.many", Inverse: "held-by.many", Kind: "holds", TypeMatch: "holds.many", Forward: true, Description: "holds, narrowed to cardinality many"},
	{Name: "held-by.many", Inverse: "holds.many", Kind: "holds", TypeMatch: "holds.many", Forward: false, Description: "held by, narrowed to cardinality many"},

	{Name: "uses", Inverse: "used-by", Kind: "uses", TypeMatch: "uses", Forward: true, Description: "a member's type refers to the target, other than by holding or injecting it"},
	{Name: "used-by", Inverse: "uses", Kind: "uses", TypeMatch: "uses", Forward: false, Description: "a type is used by a member of another"},

	{Name: "injects", Inverse: "injected-into", Kind: "uses", TypeMatch: "injects", Forward: true, Description: "a constructor parameter injects the target type"},
	{Name: "injected-into", Inverse: "injects", Kind: "uses", TypeMatch: "injects", Forward: false, Description: "a type is injected into another through its constructor"},

	{Name: "calls", Inverse: "called-by", Kind: "calls", Forward: true, Description: "a method calls another method (or reads/writes a property)"},
	{Name: "called-by", Inverse: "calls", Kind: "calls", Forward: false, Description: "a method is called by another"},

	{Name: "constructs", Inverse: "constructed-by", Kind: "constructs", Forward: true, Description: "a method constructs (`new`) the target type"},
	{Name: "constructed-by", Inverse: "constructs", Kind: "constructs", Forward: false, Description: "a type is constructed by a method"},

	{Name: "depends", Inverse: "depended-on-by", Kind: "depends", Forward: true, Description: "a module-level dependency, direction as extracted"},
	{Name: "depended-on-by", Inverse: "depends", Kind: "depends", Forward: false, Description: "the inverse of a module-level dependency"},

	{Name: "contains", Inverse: "inside", Kind: "contains", Forward: true, Description: "a namespace/assembly/type contains a member"},
	{Name: "inside", Inverse: "contains", Kind: "contains", Forward: false, Description: "a member is inside its container"},
}

// DefaultFollow is the `follow` list a neighbourhood request uses when none
// was given: everything except containment (part 1).
var DefaultFollow = []string{
	"extends", "extended-by", "implements", "implemented-by",
	"holds", "held-by", "uses", "used-by", "injects", "injected-into",
	"calls", "called-by", "constructs", "constructed-by",
	"overrides", "overridden-by", "depends", "depended-on-by",
}

// RelationVocabulary lists the table, in declaration order (GET
// /api/graph-formats, graph_formats).
func RelationVocabulary() []Relation {
	out := make([]Relation, len(relationVocabulary))
	copy(out, relationVocabulary)
	return out
}

// GetRelation looks a follow/relation name up by exact name.
func GetRelation(name string) (Relation, bool) {
	for _, r := range relationVocabulary {
		if r.Name == name {
			return r, true
		}
	}
	return Relation{}, false
}

func relationNames() string {
	names := make([]string, len(relationVocabulary))
	for i, r := range relationVocabulary {
		names[i] = r.Name
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ParseFollow resolves the `follow` request parameter (a list of relation
// names) into vocabulary entries. An empty list means DefaultFollow. An
// unknown name is an error listing the vocabulary, for callers to turn into
// 400.
func ParseFollow(names []string) ([]Relation, error) {
	if len(names) == 0 {
		names = DefaultFollow
	}
	out := make([]Relation, 0, len(names))
	for _, n := range names {
		r, ok := GetRelation(n)
		if !ok {
			return nil, fmt.Errorf("follow: %q is not one of: %s", n, relationNames())
		}
		out = append(out, r)
	}
	return out, nil
}

// typeMatches: an edge counts for a relation whose TypeMatch is "" (any
// Type of that Kind) or equal to the edge's Type, or a dotted prefix of it
// (holds.many matches holds.many and holds.many.ro).
func typeMatches(edgeType, match string) bool {
	if match == "" {
		return true
	}
	return edgeType == match || strings.HasPrefix(edgeType, match+".")
}

// edgeRelationNames: every relation name (both directions considered
// separately) that edge `e` satisfies when the node in hand is `nodeID`.
// Used by templates to print "the directed name, from the point of view of
// the node it was reached FROM" (part 3): a caller with the reached-from
// node as `nodeID` gets the name looking outward from it.
func edgeRelationNames(e GraphEdge, nodeID string) []string {
	var out []string
	for _, r := range relationVocabulary {
		if r.Kind != e.Kind || !typeMatches(e.Type, r.TypeMatch) {
			continue
		}
		if r.Forward && e.From == nodeID {
			out = append(out, r.Name)
		}
		if !r.Forward && e.To == nodeID {
			out = append(out, r.Name)
		}
	}
	return out
}
