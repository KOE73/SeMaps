package core

import (
	"encoding/json"
	"slices"
)

type ViewHead struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Axis    string `json:"axis,omitempty"`
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

// EdgeInfo is a line of a view. StyleID, Override and Routing are what the
// view's own `edges` entry says (CONTRACT §8.5, §11.6); the registry-driven
// lines carry none.
type EdgeInfo struct {
	ID       string          `json:"id"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Type     string          `json:"type"`
	StyleID  string          `json:"styleId,omitempty"`
	Override json.RawMessage `json:"override,omitempty"`
	Routing  string          `json:"routing,omitempty"`
}

type ViewInfo struct {
	View       ViewHead         `json:"view"`
	Placements []*PlacementInfo `json:"placements"`
	Edges      []EdgeInfo       `json:"edges"`
	Unsaved    []Ref            `json:"unsaved"`
}

// GetView reads a view the way a human sees it: placements nest in their
// containers, coordinates are absolute. ref is a view, or a container of it
// (`v_ops#e_a`): then only that container's subtree and the lines touching it
// come back.
func (m *Model) GetView(ref string) (ViewInfo, error) {
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
	}
	for _, e := range m.visibleEdges(doc, placed) {
		if inScope(e.From) || inScope(e.To) {
			out.Edges = append(out.Edges, e)
		}
	}
	return out, nil
}

// visibleEdges: the view's own `edges` when it has the key (an empty list means
// none), else the registry's relations by CONTRACT §8.5 — both ends on the view,
// the type's default or the view's, and `except` flipping it.
func (m *Model) visibleEdges(doc *object, placed map[string]bool) []EdgeInfo {
	var out []EdgeInfo
	if _, ok := doc.vals["edges"]; ok {
		for _, e := range viewItems(doc, "edges") {
			info := EdgeInfo{ID: e.str("id"), From: e.str("from"), To: e.str("to"), Type: e.str("type"), StyleID: e.str("styleId"), Routing: e.str("routing")}
			if raw, ok := e.vals["override"]; ok && string(raw) != "null" {
				info.Override = append(json.RawMessage(nil), raw...)
			}
			out = append(out, info)
		}
		return out
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
	types := map[string]*object{}
	for _, t := range m.records("relationType") {
		types[t.str("id")] = t
	}
	for _, r := range m.records("relation") {
		from, to := r.str("from"), r.str("to")
		if !placed[from] || !placed[to] {
			continue
		}
		d := def
		if t := types[relationType(r)]; t != nil && t.str("visibility") != "" {
			d = t.str("visibility")
		}
		if (d == "visible") != slices.Contains(except, r.str("id")) {
			out = append(out, EdgeInfo{ID: r.str("id"), From: from, To: to, Type: relationType(r)})
		}
	}
	return out
}
