package core

import (
	"encoding/json"
	"slices"
	"strings"
)

type ViewHead struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Axis      string `json:"axis,omitempty"`
	Relations any    `json:"relations,omitempty"`
	Routing   string `json:"routing,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type NodeInfo struct {
	Entity string `json:"entity"`
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Zone   string `json:"zone,omitempty"`
	Rect
	StyleID  string `json:"styleId,omitempty"`
	Template string `json:"template,omitempty"`
}

type ZoneInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Container string `json:"container,omitempty"`
	Parent    string `json:"parent,omitempty"`
	Rect
	StyleID   string      `json:"styleId,omitempty"`
	Collapsed bool        `json:"collapsed,omitempty"`
	Zones     []*ZoneInfo `json:"zones"`
	Nodes     []NodeInfo  `json:"nodes"`
}

type EdgeInfo struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type ViewInfo struct {
	View    ViewHead    `json:"view"`
	Zones   []*ZoneInfo `json:"zones"`
	Nodes   []NodeInfo  `json:"nodes"`
	Edges   []EdgeInfo  `json:"edges"`
	Unsaved []Ref       `json:"unsaved"`
}

// GetView reads a view the way a human sees it: zones nest, nodes sit in zones,
// coordinates are absolute. ref is a view, or a zone of it (`v_ops#z_a`): then
// only that zone's subtree and the lines touching it come back.
func (m *Model) GetView(ref, lang string) (ViewInfo, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return ViewInfo{}, err
	}
	kind, err := m.ResolveRef(r)
	if err != nil {
		return ViewInfo{}, err
	}
	if kind == "node" {
		return ViewInfo{}, refuse("%s is a node; pass a view or a zone", ref)
	}
	doc, err := m.view(r.View)
	if err != nil {
		return ViewInfo{}, err
	}
	if lang == "" {
		lang = "ru"
	}
	name := func(keys ...string) string {
		for _, k := range keys {
			e, err := m.text(lang, k)
			if err != nil {
				continue
			}
			if v, ok := e.vals["name"]; ok {
				o := newObject()
				if json.Unmarshal(v, o) == nil && o.str("v") != "" {
					return o.str("v")
				}
			}
		}
		return ""
	}
	ents := map[string]*object{}
	for _, e := range m.records("entity") {
		ents[e.str("id")] = e
	}

	zoneObjs := viewItems(doc, "zones")
	parents := zoneParents(zoneObjs)
	zones := map[string]*ZoneInfo{}
	var order []*ZoneInfo
	for _, z := range zoneObjs {
		info := &ZoneInfo{ID: z.str("id"), Container: z.str("container"), Parent: parents[z.str("id")], Rect: zoneRect(z),
			StyleID: z.str("styleId"), Zones: []*ZoneInfo{}, Nodes: []NodeInfo{}}
		if raw, ok := z.vals["collapsed"]; ok {
			_ = json.Unmarshal(raw, &info.Collapsed)
		}
		keys := []string{info.ID}
		if info.Container != "" {
			keys = append(keys, info.Container)
		} else if tail, ok := strings.CutPrefix(info.ID, "z_"); ok {
			keys = append(keys, "c_"+tail)
		}
		info.Name = name(keys...)
		zones[info.ID] = info
		order = append(order, info)
	}
	var top []*ZoneInfo
	for _, z := range order {
		if p := zones[z.Parent]; p != nil && z.Parent != z.ID {
			p.Zones = append(p.Zones, z)
		} else {
			top = append(top, z)
		}
	}
	loose := []NodeInfo{}
	placed := map[string]bool{}
	for _, key := range []string{"nodes", "placements"} {
		for _, n := range viewItems(doc, key) {
			id := nodeID(n)
			info := NodeInfo{Entity: id, Zone: nodeZone(n), Rect: nodeRect(n), StyleID: n.str("styleId"), Template: n.str("template")}
			if e := ents[id]; e != nil {
				info.Name, info.Kind = e.str("name"), e.str("kind")
			}
			placed[id] = true
			if z := zones[info.Zone]; z != nil {
				z.Nodes = append(z.Nodes, info)
			} else {
				info.Zone = ""
				loose = append(loose, info)
			}
		}
	}

	out := ViewInfo{View: ViewHead{ID: r.View, Project: m.project, Axis: doc.str("axis"), Routing: doc.str("routing")},
		Zones: top, Nodes: loose, Edges: []EdgeInfo{}, Unsaved: append([]Ref{}, m.Dirty().Views[r.View]...)}
	if top == nil {
		out.Zones = []*ZoneInfo{}
	}
	if raw, ok := doc.vals["relations"]; ok {
		_ = json.Unmarshal(raw, &out.View.Relations)
	}

	inScope := func(string) bool { return true }
	if kind == "zone" {
		root := zones[r.ID]
		out.View.Scope = ref
		out.Zones, out.Nodes = []*ZoneInfo{root}, []NodeInfo{}
		inside := map[string]bool{}
		var walk func(z *ZoneInfo)
		walk = func(z *ZoneInfo) {
			for _, n := range z.Nodes {
				inside[n.Entity] = true
			}
			for _, c := range z.Zones {
				walk(c)
			}
		}
		walk(root)
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
			out = append(out, EdgeInfo{e.str("id"), e.str("from"), e.str("to"), e.str("type")})
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
			out = append(out, EdgeInfo{r.str("id"), from, to, relationType(r)})
		}
	}
	return out
}
