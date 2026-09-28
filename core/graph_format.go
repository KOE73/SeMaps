// GraphFormatter turns a (already filtered) graph answer into bytes for an
// agent to read, docs/API.md §5/§6. Several formats exist side by side on
// purpose: nobody yet knows which one serves an agent best, so the registry
// makes adding one a matter of writing a new file and registering it here,
// nothing hard-wired to any single format.
package core

import "fmt"

// FormatOptions is everything a formatter needs besides the graph itself.
// A formatter must not filter the graph on its own — Focus/Truncated etc.
// describe filtering the caller already did.
type FormatOptions struct {
	// Focus is the `around` node id, or "" when the request had none.
	Focus string
	// Facts and Stats are carried through opaquely (already the shape the
	// JSON formats and the API have always returned them in); a text format
	// ignores them beyond the counts it prints itself.
	Facts any
	Stats any
	// Truncated says the node list was cut to a limit (MCP's `limit`); when
	// true, FullNodes/FullEdges are the untruncated counts.
	Truncated            bool
	FullNodes, FullEdges int
	// Fields is the `fields` selection, applied by JSON formats only; text
	// formats always show position (it is their point) and never show
	// via/members as separate data.
	Fields map[string]bool
}

// GraphFormatter is one named answer format.
type GraphFormatter interface {
	Name() string
	Description() string
	MediaType() string
	// Format renders `g` (already filtered: level, kinds/set, around,
	// container, missing — the formatter never filters). `g.Nodes`/`g.Edges`
	// are already in the order the caller wants printed.
	Format(g *Graph, opts FormatOptions) ([]byte, error)
}

// graphFormats is the registry, in declaration order (also the order they
// are listed to a caller). Add a new format here and nowhere else.
var graphFormats = []GraphFormatter{
	jsonFormat{},
	jsonCompactFormat{},
	linesFormat{},
	locationsFormat{},
	treeFormat{},
}

// DefaultGraphFormat is the HTTP endpoint's default: today's JSON, unchanged.
const DefaultGraphFormat = "json"

// DefaultToolFormat is get_graph's default. Deliberately NOT "json": the
// whole point of this task is to have several real formats side by side so
// a real agent can be asked which one serves it best. Set to "lines" for
// now; change this one constant once that question has an answer.
const DefaultToolFormat = "lines"

// GraphFormats lists the registry, in declaration order.
func GraphFormats() []GraphFormatter {
	out := make([]GraphFormatter, len(graphFormats))
	copy(out, graphFormats)
	return out
}

// GetGraphFormat looks a format up by name.
func GetGraphFormat(name string) (GraphFormatter, bool) {
	for _, f := range graphFormats {
		if f.Name() == name {
			return f, true
		}
	}
	return nil, false
}

func graphFormatNames() string {
	names := make([]string, len(graphFormats))
	for i, f := range graphFormats {
		names[i] = f.Name()
	}
	return joinStrings(names)
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// UnknownFormatError is what a caller turns into 400, naming the formats
// that do exist.
func UnknownFormatError(name string) error {
	return fmt.Errorf("format: %q is not one of: %s", name, graphFormatNames())
}
