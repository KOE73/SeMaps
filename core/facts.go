package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Facts is what an extractor prints: docs/EXTRACTOR.md §2. The extractor
// knows nothing about the registry; Sync turns these into entities.json and
// relations.json.
type Facts struct {
	Language  string   `json:"language"`
	Root      string   `json:"root"`
	EdgeKinds []string `json:"edgeKinds,omitempty"` // explicit list of edge kinds covered by these facts
	Symbols   []Symbol `json:"symbols"`
	Edges     []Edge   `json:"edges"`
}

// Symbol is one declaration. ID is the key within one output (ADR_20260923-5),
// never a registry id.
type Symbol struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	NativeKind string `json:"nativeKind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	Visibility string `json:"visibility,omitempty"`
	// Members keeps the extractor's array as is: its shape is the `members`
	// of CONTRACT.md §3, and sync copies it into the entity unchanged.
	Members json.RawMessage `json:"members,omitempty"`
}

// Edge joins two symbols of the same output.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
	Via  *Via   `json:"via,omitempty"` // signature of member relation
}

// Via is the signature of a member relation: docs/EXTRACTOR.md §2.2a.
// All fields are optional: absent means "not known".
type Via struct {
	Member      string   `json:"member,omitempty"`      // name of the member
	MemberKind  string   `json:"memberKind,omitempty"`  // field, property, event, indexer, parameter, return, constructor, self
	Modifiers   []string `json:"modifiers,omitempty"`   // public, private, readonly, static, …
	Text        string   `json:"text,omitempty"`        // the member type as written in code
	Path        []string `json:"path,omitempty"`        // path inside the member type to the target symbol
	Cardinality string   `json:"cardinality,omitempty"` // one, optional, many, keyed
	Mutability  string   `json:"mutability,omitempty"`  // mutable, readonly
	Deferred    bool     `json:"deferred,omitempty"`    // true if the object comes later
}

// SymbolKinds and EdgeKinds are the closed vocabularies of EXTRACTOR.md §2.2–2.3.
var (
	SymbolKinds = []string{"type", "interface", "function", "module", "value"}
	EdgeKinds   = []string{"extends", "implements", "contains", "depends", "holds", "uses"}
)

// FactsError lists every problem found in a facts document, not just the first.
type FactsError struct{ Problems []string }

func (e *FactsError) Error() string {
	const shown = 15
	var b strings.Builder
	fmt.Fprintf(&b, "facts do not follow EXTRACTOR.md §2 (%d problems):", len(e.Problems))
	for i, p := range e.Problems {
		if i == shown {
			fmt.Fprintf(&b, "\n  ... and %d more", len(e.Problems)-shown)
			break
		}
		b.WriteString("\n  ")
		b.WriteString(p)
	}
	return b.String()
}

var (
	languagePattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	symbolIDPattern = regexp.MustCompile(`^\S+$`)
)

// ReadFacts decodes and validates one facts document. Fields it does not know
// are ignored, so an extractor may print more than this version reads.
func ReadFacts(r io.Reader) (*Facts, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return nil, fmt.Errorf("facts are not a JSON object: %w", err)
	}
	var f Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("facts: %w", err)
	}
	var problems []string
	for _, key := range []string{"language", "root", "symbols", "edges"} {
		if _, ok := present[key]; !ok {
			problems = append(problems, fmt.Sprintf("`%s` is required", key))
		}
	}
	problems = append(problems, f.problems()...)
	if len(problems) > 0 {
		return nil, &FactsError{problems}
	}
	return &f, nil
}

// Validate checks the rules of EXTRACTOR.md §2–3 that a reader can see:
// vocabularies, required fields, unique ids, edges only between printed
// symbols, and the sort order that makes the output deterministic.
func (f *Facts) Validate() error {
	if p := f.problems(); len(p) > 0 {
		return &FactsError{p}
	}
	return nil
}

func (f *Facts) problems() []string {
	var out []string
	bad := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	if !languagePattern.MatchString(f.Language) {
		bad("language %q: lower-case latin letters and digits", f.Language)
	}
	if f.Root == "" {
		bad("root is empty")
	}

	kinds := setOf(SymbolKinds)
	ids := make(map[string]bool, len(f.Symbols))
	for i, s := range f.Symbols {
		where := fmt.Sprintf("symbols[%d] %s", i, s.ID)
		if !symbolIDPattern.MatchString(s.ID) {
			bad("symbols[%d]: id %q is empty or has whitespace", i, s.ID)
		} else if ids[s.ID] {
			bad("%s: id repeats", where)
		}
		ids[s.ID] = true
		if !kinds[s.Kind] {
			bad("%s: kind %q is not one of %s", where, s.Kind, strings.Join(SymbolKinds, ", "))
		}
		if s.NativeKind == "" {
			bad("%s: nativeKind is empty", where)
		}
		if s.Name == "" {
			bad("%s: name is empty", where)
		}
		switch {
		case s.File == "":
			bad("%s: file is empty", where)
		case strings.Contains(s.File, `\`):
			bad("%s: file %q must use forward slashes", where, s.File)
		case strings.HasPrefix(s.File, "/") || (len(s.File) > 1 && s.File[1] == ':'):
			bad("%s: file %q must be relative to root", where, s.File)
		}
		if s.Line < 0 {
			bad("%s: line %d, lines start at 1", where, s.Line)
		}
		if len(s.Members) > 0 {
			var members []struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(s.Members, &members); err != nil {
				bad("%s: members is not an array of objects: %v", where, err)
			} else {
				for j, m := range members {
					if m.Name == "" {
						bad("%s: members[%d] has no name", where, j)
					}
				}
			}
		}
		if i > 0 && f.Symbols[i-1].ID > s.ID {
			bad("%s: symbols are not sorted by id (after %s)", where, f.Symbols[i-1].ID)
		}
	}

	edgeKinds := setOf(EdgeKinds)
	for i, e := range f.Edges {
		where := fmt.Sprintf("edges[%d] %s -%s-> %s", i, e.From, e.Kind, e.To)
		if !edgeKinds[e.Kind] {
			bad("%s: kind %q is not one of %s", where, e.Kind, strings.Join(EdgeKinds, ", "))
		}
		if !ids[e.From] {
			bad("%s: from is not a symbol of this output", where)
		}
		if !ids[e.To] {
			bad("%s: to is not a symbol of this output", where)
		}
		if e.From == e.To {
			bad("%s: an edge to itself", where)
		}
		if e.Via != nil && (e.Kind == "extends" || e.Kind == "implements" || e.Kind == "contains" || e.Kind == "depends") {
			bad("%s: `via` is only for holds/uses edges", where)
		}
		if e.Via != nil {
			if e.Via.Cardinality != "" && e.Via.Cardinality != "one" && e.Via.Cardinality != "optional" && e.Via.Cardinality != "many" && e.Via.Cardinality != "keyed" {
				bad("%s: via.cardinality %q must be one of: one, optional, many, keyed", where, e.Via.Cardinality)
			}
			if e.Via.Mutability != "" && e.Via.Mutability != "mutable" && e.Via.Mutability != "readonly" {
				bad("%s: via.mutability %q must be mutable or readonly", where, e.Via.Mutability)
			}
		}
		if i > 0 {
			switch prev := f.Edges[i-1]; compareEdges(prev, e) {
			case 0:
				bad("%s: edge repeats", where)
			case 1:
				bad("%s: edges are not sorted by (from, to, kind, via.member, via.path)", where)
			}
		}
	}
	return out
}

func compareEdges(a, b Edge) int {
	for _, pair := range [][2]string{{a.From, b.From}, {a.To, b.To}, {a.Kind, b.Kind}} {
		if c := strings.Compare(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	// Member and path are only for member relations (holds/uses).
	// Missing fields sort before present ones.
	aMember := ""
	if a.Via != nil {
		aMember = a.Via.Member
	}
	bMember := ""
	if b.Via != nil {
		bMember = b.Via.Member
	}
	if c := strings.Compare(aMember, bMember); c != 0 {
		return c
	}
	aPath := ""
	if a.Via != nil {
		aPath = strings.Join(a.Via.Path, ",")
	}
	bPath := ""
	if b.Via != nil {
		bPath = strings.Join(b.Via.Path, ",")
	}
	return strings.Compare(aPath, bPath)
}

func setOf(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, s := range list {
		m[s] = true
	}
	return m
}

// compactJSON is the canonical form two JSON values are compared in.
func compactJSON(raw json.RawMessage) string {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return string(raw)
	}
	return b.String()
}
