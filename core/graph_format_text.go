// The plain-text graph formats: lines, locations, tree. Shared helpers
// (location printing, relation wording, the trailing counts line) live here
// so the three formats stay consistent with each other.
package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// place prints a node's position exactly as the facts give it: forward
// slashes, relative to the source root, so an agent can hand it straight to
// a file-reading tool. "" when the node carries no position at all.
func place(file string, line, endLine int) string {
	if file == "" {
		return ""
	}
	if line == 0 {
		return file
	}
	if endLine != 0 && endLine != line {
		return fmt.Sprintf("%s:%d-%d", file, line, endLine)
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// countsLine is the short trailing line every text format ends with, plus a
// note when the answer was cut by a limit.
func countsLine(g *Graph, o FormatOptions) string {
	s := fmt.Sprintf("%d nodes, %d edges", len(g.Nodes), len(g.Edges))
	if o.Truncated {
		s += fmt.Sprintf(" (cut to a limit: %d of %d nodes, %d of %d edges)", len(g.Nodes), o.FullNodes, len(g.Edges), o.FullEdges)
	}
	return s
}

// forward/reverse relation wording, a person would use it: docs/EXTRACTOR.md
// §2 lists the edge kinds this covers (extends, implements, holds, uses,
// depends, contains); `injects`/`holds.*` are the Type nuance of a `holds`/
// `uses` Kind edge and are folded into the same wording as their Kind.
var relationWords = map[string][2]string{
	"extends":    {"extends", "extended by"},
	"implements": {"implements", "implemented by"},
	"holds":      {"holds", "held by"},
	"uses":       {"uses", "used by"},
	"depends":    {"depends on", "depended on by"},
	"contains":   {"contains", "inside"},
}

// direction: word for `kind` when the node in hand is `from` (forward=true)
// or `to` (forward=false) of the edge.
func relationWord(kind string, forward bool) string {
	w, ok := relationWords[kind]
	if !ok {
		return kind
	}
	if forward {
		return w[0]
	}
	return w[1]
}

func nodeByID(g *Graph) map[string]*GraphNode {
	m := make(map[string]*GraphNode, len(g.Nodes))
	for i := range g.Nodes {
		m[g.Nodes[i].ID] = &g.Nodes[i]
	}
	return m
}

// nodeLabel: a node's name, falling back to its id when it has none (a
// dangling reference should never print as an empty string).
func nodeLabel(n *GraphNode) string {
	if n == nil {
		return "?"
	}
	if n.Name != "" {
		return n.Name
	}
	return n.ID
}

func whereItLies(n *GraphNode) string {
	if n == nil || n.Namespace == "" {
		return ""
	}
	return n.Namespace
}

// ---------------------------------------------------------------- lines

// linesFormat: one line per neighbour, grouped by relation+direction in
// words, with file:line-endLine. Ignores Fields (position is always shown;
// via/members are not separate data in this format).
type linesFormat struct{}

func (linesFormat) Name() string { return "lines" }
func (linesFormat) Description() string {
	return "plain text, one line per neighbour, grouped by relation and direction in words"
}
func (linesFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (linesFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	byID := nodeByID(g)
	var order []string
	if o.Focus != "" {
		order = append(order, o.Focus)
	}
	for _, n := range g.Nodes {
		if n.ID != o.Focus {
			order = append(order, n.ID)
		}
	}

	var b strings.Builder
	for _, id := range order {
		n := byID[id]
		b.WriteString(nodeLabel(n))
		if p := place(n.File, n.Line, n.EndLine); p != "" {
			b.WriteString("  ")
			b.WriteString(p)
		}
		if ns := whereItLies(n); ns != "" {
			b.WriteString("  in ")
			b.WriteString(ns)
		}
		b.WriteString("\n")

		type nb struct {
			word  string
			label string
			place string
		}
		var group []nb
		for _, e := range g.Edges {
			switch {
			case e.From == id:
				other := byID[e.To]
				group = append(group, nb{relationWord(e.Kind, true), nodeLabel(other), place(other.File, other.Line, other.EndLine)})
			case e.To == id:
				other := byID[e.From]
				group = append(group, nb{relationWord(e.Kind, false), nodeLabel(other), place(other.File, other.Line, other.EndLine)})
			}
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].word != group[j].word {
				return group[i].word < group[j].word
			}
			return group[i].label < group[j].label
		})
		for _, nb := range group {
			b.WriteString("  ")
			b.WriteString(nb.word)
			b.WriteString(": ")
			b.WriteString(nb.label)
			if nb.place != "" {
				b.WriteString("  ")
				b.WriteString(nb.place)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// ------------------------------------------------------------ locations

// locationsFormat: the degenerate one — name, file, line, endLine, tab
// separated, one line per node, no relations at all.
type locationsFormat struct{}

func (locationsFormat) Name() string { return "locations" }
func (locationsFormat) Description() string {
	return "name<TAB>file<TAB>line<TAB>endLine, one line per node, no relations"
}
func (locationsFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (locationsFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	var b strings.Builder
	for _, n := range g.Nodes {
		line, endLine := "", ""
		if n.Line != 0 {
			line = strconv.Itoa(n.Line)
		}
		if n.EndLine != 0 {
			endLine = strconv.Itoa(n.EndLine)
		}
		b.WriteString(nodeLabel(&n))
		b.WriteString("\t")
		b.WriteString(n.File)
		b.WriteString("\t")
		b.WriteString(line)
		b.WriteString("\t")
		b.WriteString(endLine)
		b.WriteString("\n")
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// ------------------------------------------------------------------ tree

// treeFormat: indented by walk depth from the focus node. Without a focus
// this format is refused — there is no "depth from" without one.
type treeFormat struct{}

func (treeFormat) Name() string { return "tree" }
func (treeFormat) Description() string {
	return "indented by depth from the focus node: relation → name  file:line; needs `around`"
}
func (treeFormat) MediaType() string { return "text/plain; charset=utf-8" }

func (treeFormat) Format(g *Graph, o FormatOptions) ([]byte, error) {
	if o.Focus == "" {
		return nil, fmt.Errorf("tree needs a focus node (around=...): there is no depth to indent by without one")
	}
	byID := nodeByID(g)
	if byID[o.Focus] == nil {
		return nil, fmt.Errorf("tree: focus node %q not in the graph", o.Focus)
	}
	adjacent := map[string][]int{}
	for i, e := range g.Edges {
		adjacent[e.From] = append(adjacent[e.From], i)
		adjacent[e.To] = append(adjacent[e.To], i)
	}

	var b strings.Builder
	printed := map[string]bool{o.Focus: true}
	root := byID[o.Focus]
	b.WriteString(nodeLabel(root))
	if p := place(root.File, root.Line, root.EndLine); p != "" {
		b.WriteString("  ")
		b.WriteString(p)
	}
	b.WriteString("\n")

	type item struct {
		id, word string
		depth    int
	}
	// BFS, but each depth's items are emitted sorted, deterministic.
	frontier := []item{{o.Focus, "", 0}}
	visitedAsParent := map[string]bool{}
	for len(frontier) > 0 {
		var next []item
		type child struct {
			id, word string
		}
		byParent := map[string][]child{}
		var parents []string
		for _, it := range frontier {
			if visitedAsParent[it.id] {
				continue
			}
			visitedAsParent[it.id] = true
			parents = append(parents, it.id)
			for _, ei := range adjacent[it.id] {
				e := g.Edges[ei]
				var otherID, word string
				if e.From == it.id {
					otherID, word = e.To, relationWord(e.Kind, true)
				} else {
					otherID, word = e.From, relationWord(e.Kind, false)
				}
				byParent[it.id] = append(byParent[it.id], child{otherID, word})
			}
		}
		sort.Strings(parents)
		for _, pid := range parents {
			kids := byParent[pid]
			sort.Slice(kids, func(i, j int) bool {
				if kids[i].word != kids[j].word {
					return kids[i].word < kids[j].word
				}
				return nodeLabel(byID[kids[i].id]) < nodeLabel(byID[kids[j].id])
			})
			depth := 0
			for _, it := range frontier {
				if it.id == pid {
					depth = it.depth
					break
				}
			}
			for _, k := range kids {
				n := byID[k.id]
				b.WriteString(strings.Repeat("  ", depth+1))
				b.WriteString(k.word)
				b.WriteString(" → ")
				b.WriteString(nodeLabel(n))
				if !printed[k.id] {
					printed[k.id] = true
					if p := place(n.File, n.Line, n.EndLine); p != "" {
						b.WriteString("  ")
						b.WriteString(p)
					}
					next = append(next, item{k.id, k.word, depth + 1})
				} else {
					b.WriteString("  (see above)")
				}
				b.WriteString("\n")
			}
		}
		frontier = next
	}
	b.WriteString(countsLine(g, o))
	b.WriteString("\n")
	return []byte(b.String()), nil
}
