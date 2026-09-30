package core

import (
	"encoding/json"
	"slices"
)

type ViewHead struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	// Detail is how the placements are given: DetailFull (every one, nested) or
	// DetailTree (containers only, with a count of the blocks in each).
	Detail string `json:"detail"`
	Axis   string `json:"axis,omitempty"`
	// AxisInherited: the view declares no axis, Axis is project.defaultAxis (CONTRACT §8.1).
	AxisInherited bool   `json:"axisInherited,omitempty"`
	Relations     any    `json:"relations,omitempty"`
	Routing       string `json:"routing,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

// PlacementInfo is one placement as a human sees it: the entity's name and
// kind, whether it is a container, an absolute rectangle; a container carries
// what lies in it.
type PlacementInfo struct {
	Entity    string  `json:"entity"`
	Name      string  `json:"name,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	Container bool    `json:"container"`
	Parent    *string `json:"parent"`
	Rect
	StyleID   string           `json:"styleId,omitempty"`
	Override  json.RawMessage  `json:"override,omitempty"`
	Template  string           `json:"template,omitempty"`
	Collapsed bool             `json:"collapsed,omitempty"`
	Children  []*PlacementInfo `json:"children,omitempty"`
	// Blocks: in the tree detail, how many non-container placements lie directly
	// in this container (they are not listed). Nil otherwise.
	Blocks *int `json:"blocks,omitempty"`
}

// MarshalJSON: a container always has children (an empty list when it holds
// nothing); a block has none.
func (p PlacementInfo) MarshalJSON() ([]byte, error) {
	type plain PlacementInfo
	children := p.Children
	if p.Container && children == nil {
		children = []*PlacementInfo{}
	}
	if !p.Container {
		return json.Marshal(struct {
			plain
			Children []*PlacementInfo `json:"children,omitempty"`
		}{plain(p), nil})
	}
	return json.Marshal(struct {
		plain
		Children []*PlacementInfo `json:"children"`
	}{plain(p), children})
}

// EdgeInfo is a line of a view: a relation of the registry that the view shows
// (CONTRACT §8.5). StyleID, Override and Routing are what the view's `edges`
// entry of the same id says (§8.5, §11.6); a line without such an entry has none.
type EdgeInfo struct {
	ID       string          `json:"id"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Type     string          `json:"type"`
	StyleID  string          `json:"styleId,omitempty"`
	Override json.RawMessage `json:"override,omitempty"`
	Routing  string          `json:"routing,omitempty"`
}

// The details of a read of a view.
const (
	DetailFull = "full"
	DetailTree = "tree"
)

// ViewOptions shape what GetViewWith returns.
type ViewOptions struct {
	// Detail: DetailFull, DetailTree, or "" to let the size decide: full when the
	// scope holds at most FullMax placements (FullMax <= 0: always), else tree.
	Detail  string
	FullMax int
	// Hidden also lists the relations both of whose ends are placed and that the
	// rule hides.
	Hidden bool
}

// ViewInfo is a read of a view. In the full detail Edges lists the visible
// lines; in the tree detail only their count is given (Lines) and the
// placements are containers with a count of their blocks.
type ViewInfo struct {
	View       ViewHead
	Placements []*PlacementInfo
	Edges      []EdgeInfo
	// Lines is the number of visible lines in scope (tree detail).
	Lines int
	// HiddenEdges is non-nil only when they were asked for (ViewOptions.Hidden).
	HiddenEdges []EdgeInfo
	Unsaved     []Ref
	// Scoped is the number of placements in scope (the view, or the container's
	// subtree); FellBack says the tree detail was chosen because of it, not asked.
	Scoped   int
	FellBack bool
}

// MarshalJSON: `edges` in the full detail, `lines` (a count) in the tree one;
// `hiddenEdges` only when asked for.
func (v ViewInfo) MarshalJSON() ([]byte, error) {
	w := struct {
		View        ViewHead         `json:"view"`
		Placements  []*PlacementInfo `json:"placements"`
		Edges       *[]EdgeInfo      `json:"edges,omitempty"`
		Lines       *int             `json:"lines,omitempty"`
		HiddenEdges *[]EdgeInfo      `json:"hiddenEdges,omitempty"`
		Unsaved     []Ref            `json:"unsaved"`
	}{View: v.View, Placements: v.Placements, Unsaved: v.Unsaved}
	if v.View.Detail == DetailTree {
		w.Lines = &v.Lines
	} else {
		w.Edges = &v.Edges
	}
	if v.HiddenEdges != nil {
		w.HiddenEdges = &v.HiddenEdges
	}
	return json.Marshal(w)
}

// GetView reads a view in full: see GetViewWith.
func (m *Model) GetView(ref string) (ViewInfo, error) {
	return m.GetViewWith(ref, ViewOptions{Detail: DetailFull})
}

// GetViewWith reads a view the way a human sees it: placements nest in their
// containers, coordinates are absolute. ref is a view, or a container of it
// (`v_ops#e_a`): then only that container's subtree and the lines touching it
// come back.
func (m *Model) GetViewWith(ref string, opts ViewOptions) (ViewInfo, error) {
	if !slices.Contains([]string{"", DetailFull, DetailTree}, opts.Detail) {
		return ViewInfo{}, refuse("detail is %q or %q", DetailFull, DetailTree)
	}
	r, err := ParseRef(ref)
	if err != nil {
		return ViewInfo{}, err
	}
	cv, err := m.Canvas()
	if err != nil {
		return ViewInfo{}, err
	}
	kind, err := m.ResolveRef(r)
	if err != nil {
		return ViewInfo{}, err
	}
	if kind == "block" {
		return ViewInfo{}, refuse("%s is not a container; pass a view or a container", ref)
	}
	doc, err := m.view(r.View)
	if err != nil {
		return ViewInfo{}, err
	}
	ents := map[string]*object{}
	for _, e := range m.records("entity") {
		ents[e.str("id")] = e
	}
	names := m.EntityNames()

	items := viewItems(doc, "placements")
	byID := map[string]*PlacementInfo{}
	var order []*PlacementInfo
	placed := map[string]bool{}
	for _, p := range items {
		id := p.str("entity")
		info := &PlacementInfo{Entity: id, StyleID: p.str("styleId"), Template: p.str("template")}
		if e := ents[id]; e != nil {
			info.Name, info.Kind = names[id], e.str("kind")
		}
		info.Container = m.kinds.IsContainer(info.Kind)
		info.Rect = placementRect(p, info.Container, cv)
		if raw, ok := p.vals["override"]; ok && string(raw) != "null" {
			info.Override = append(json.RawMessage(nil), raw...)
		}
		if raw, ok := p.vals["collapsed"]; ok {
			_ = json.Unmarshal(raw, &info.Collapsed)
		}
		if parent := p.str("parent"); parent != "" {
			info.Parent = &parent
		}
		byID[id] = info
		order = append(order, info)
		placed[id] = true
	}
	// nest by parent; a parent that is not a container placement of this view,
	// or a loop, leaves the placement at the top
	var top []*PlacementInfo
	reached := map[string]bool{}
	for _, p := range order {
		if p.Parent == nil || byID[*p.Parent] == nil || !byID[*p.Parent].Container || *p.Parent == p.Entity {
			top = append(top, p)
		}
	}
	var walk func(p *PlacementInfo)
	walk = func(p *PlacementInfo) {
		reached[p.Entity] = true
		for _, c := range order {
			if c.Parent != nil && *c.Parent == p.Entity && p.Container && !reached[c.Entity] && c != p {
				p.Children = append(p.Children, c)
				walk(c)
			}
		}
	}
	for _, p := range top {
		walk(p)
	}
	for _, p := range order {
		if !reached[p.Entity] {
			top = append(top, p)
			walk(p)
		}
	}
	if top == nil {
		top = []*PlacementInfo{}
	}

	out := ViewInfo{View: ViewHead{ID: r.View, Project: m.project, Axis: doc.str("axis"), Routing: doc.str("routing")},
		Placements: top, Edges: []EdgeInfo{}, Unsaved: append([]Ref{}, m.Dirty().Views[r.View]...)}
	if out.View.Axis == "" {
		var manifest struct {
			DefaultAxis string `json:"defaultAxis"`
		}
		_ = json.Unmarshal(m.Manifest(), &manifest)
		out.View.Axis, out.View.AxisInherited = manifest.DefaultAxis, manifest.DefaultAxis != ""
	}
	if raw, ok := doc.vals["relations"]; ok {
		_ = json.Unmarshal(raw, &out.View.Relations)
	}

	inScope := func(string) bool { return true }
	out.Scoped = len(order)
	if kind == "container" {
		root := byID[r.ID]
		out.View.Scope = ref
		out.Placements = []*PlacementInfo{root}
		inside := map[string]bool{}
		var mark func(p *PlacementInfo)
		mark = func(p *PlacementInfo) {
			inside[p.Entity] = true
			for _, c := range p.Children {
				mark(c)
			}
		}
		mark(root)
		inScope = func(entity string) bool { return inside[entity] }
		out.Scoped = len(inside)
	}
	shown, hidden := m.edgesByRule(doc, placed)
	if opts.Hidden {
		out.HiddenEdges = []EdgeInfo{}
	}
	for _, e := range hidden {
		if opts.Hidden && (inScope(e.From) || inScope(e.To)) {
			out.HiddenEdges = append(out.HiddenEdges, e)
		}
	}
	for _, e := range shown {
		if inScope(e.From) || inScope(e.To) {
			out.Edges = append(out.Edges, e)
		}
	}

	out.View.Detail = opts.Detail
	if out.View.Detail == "" {
		out.View.Detail = DetailFull
		if opts.FullMax > 0 && out.Scoped > opts.FullMax {
			out.View.Detail, out.FellBack = DetailTree, true
		}
	}
	if out.View.Detail == DetailTree {
		out.Lines, out.Edges = len(out.Edges), nil
		for i, p := range out.Placements {
			if p.Container {
				out.Placements[i] = treeOf(p)
			}
		}
	}
	return out, nil
}

// treeOf is a container without its blocks: its container children, nested the
// same way, and how many blocks lay directly in it.
func treeOf(p *PlacementInfo) *PlacementInfo {
	c := *p
	c.Children = nil
	blocks := 0
	for _, ch := range p.Children {
		if ch.Container {
			c.Children = append(c.Children, treeOf(ch))
		} else {
			blocks++
		}
	}
	c.Blocks = &blocks
	return &c
}

// edgesByRule splits the relations of the registry with both ends on the view
// into the lines the view shows and the ones it hides. What is visible is always
// the registry's relations by CONTRACT §8.5 — the type's default or the view's,
// and `except` flipping it. The view's own `edges` decide nothing about
// visibility: an entry of the same id only adds its own styleId, override and
// routing to the line.
func (m *Model) edgesByRule(doc *object, placed map[string]bool) (shown, hidden []EdgeInfo) {
	own := map[string]*object{}
	for _, e := range viewItems(doc, "edges") {
		own[e.str("id")] = e
	}
	def, except := "visible", []string(nil)
	if raw, ok := doc.vals["relations"]; ok {
		p := newObject()
		if json.Unmarshal(raw, p) == nil {
			def = orDefault(p.str("default"), def)
			if x, ok := p.vals["except"]; ok {
				_ = json.Unmarshal(x, &except)
			}
		}
	}
	for _, r := range m.records("relation") {
		from, to := r.str("from"), r.str("to")
		if !placed[from] || !placed[to] {
			continue
		}
		d := def
		if v := m.kinds.RelationVisibility(relationType(r)); v != "" {
			d = v
		}
		info := EdgeInfo{ID: r.str("id"), From: from, To: to, Type: relationType(r)}
		if e := own[info.ID]; e != nil {
			info.StyleID, info.Routing = e.str("styleId"), e.str("routing")
			if raw, ok := e.vals["override"]; ok && string(raw) != "null" {
				info.Override = append(json.RawMessage(nil), raw...)
			}
		}
		if (d == "visible") != slices.Contains(except, r.str("id")) {
			shown = append(shown, info)
		} else {
			hidden = append(hidden, info)
		}
	}
	return shown, hidden
}
