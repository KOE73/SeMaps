package core

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
)

// Geometry of a view, in steps a person would name: move, resize, put into a
// zone, add a zone, fit a zone to its content, align. Each step reads the view,
// changes copies of the zone and node objects, and applies all of them as one
// batch — everything or nothing. Every step needs requestedByHuman
// (CONTRACT §8.2 p. 3, ADR_20260924).

// GeomReport says what a geometry step did. Touched are references `view#id`
// of every zone and node whose object changed.
type GeomReport struct {
	Touched []string `json:"touched"`
}

const needHuman = "geometry of a view is written only on a human's direct request (CONTRACT §8.2 p. 3)"

type layout struct {
	view      string
	nodeKey   string
	zones     map[string]*object
	nodes     map[string]*object
	zoneOrder []string
	nodeOrder []string
	before    map[string]string // "zone:<id>" / "node:<id>" -> the object as loaded
	parents   map[string]string // effective parent of each zone, see zoneParents
}

func (m *Model) loadLayout(viewID string) (*layout, error) {
	doc, err := m.view(viewID)
	if err != nil {
		return nil, refuse("no view %s", viewID)
	}
	l := &layout{view: viewID, nodeKey: "nodes", zones: map[string]*object{}, nodes: map[string]*object{}, before: map[string]string{}}
	for _, z := range viewItems(doc, "zones") {
		id := z.str("id")
		l.zones[id] = z
		l.zoneOrder = append(l.zoneOrder, id)
		l.before["zone:"+id] = objString(z)
	}
	if _, ok := doc.vals["nodes"]; !ok {
		if _, ok := doc.vals["placements"]; ok {
			l.nodeKey = "placements"
		}
	}
	for _, n := range viewItems(doc, l.nodeKey) {
		id := nodeID(n)
		l.nodes[id] = n
		l.nodeOrder = append(l.nodeOrder, id)
		l.before["node:"+id] = objString(n)
	}
	l.parents = zoneParents(viewItems(doc, "zones"))
	return l, nil
}

// zoneParents is where each zone nests: its `parent`, else a zone named by its
// `container` (older files do that), else — files that only draw the frames
// inside each other — the smallest zone whose rectangle strictly holds it.
func zoneParents(list []*object) map[string]string {
	ids := map[string]bool{}
	for _, z := range list {
		ids[z.str("id")] = true
	}
	out := map[string]string{}
	for _, z := range list {
		id := z.str("id")
		p := z.str("parent")
		if p == "" || !ids[p] || p == id {
			p = z.str("container")
			if !ids[p] || p == id {
				p = ""
			}
		}
		if p == "" {
			rz, best, bestArea := zoneRect(z), "", 0.0
			for _, o := range list {
				ro := zoneRect(o)
				if o == z || ro == rz || !ro.Contains(rz) {
					continue
				}
				if a := ro.Width * ro.Height; best == "" || a < bestArea {
					best, bestArea = o.str("id"), a
				}
			}
			p = best
		}
		if p != "" {
			out[id] = p
		}
	}
	return out
}

func objString(o *object) string { b, _ := o.MarshalJSON(); return string(b) }

// id turns a reference `view#id` or a bare id into a bare id of this layout.
func (l *layout) id(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "#") || strings.Contains(s, "/") {
		r, err := ParseRef(s)
		if err != nil {
			return "", err
		}
		if r.View != l.view || r.ID == "" {
			return "", refuse("%s is not an object of view %s", s, l.view)
		}
		s = r.ID
	}
	if l.zones[s] == nil && l.nodes[s] == nil {
		return "", refuse("%s is not on view %s", s, l.view)
	}
	return s, nil
}

func (l *layout) ids(list []string) ([]string, error) {
	if len(list) == 0 {
		return nil, refuse("no elements given")
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		id, err := l.id(s)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (l *layout) isZone(id string) bool { return l.zones[id] != nil }

func (l *layout) rect(id string) Rect {
	if z := l.zones[id]; z != nil {
		return zoneRect(z)
	}
	return nodeRect(l.nodes[id])
}

// setRect writes only the fields that differ, so a node that never had a size
// does not get one for being moved.
func (l *layout) setRect(id string, r Rect) {
	o := l.zones[id]
	if o == nil {
		o = l.nodes[id]
	}
	cur := l.rect(id)
	for _, f := range []struct {
		key      string
		old, new float64
	}{{"x", cur.X, r.X}, {"y", cur.Y, r.Y}, {"width", cur.Width, r.Width}, {"height", cur.Height, r.Height}} {
		if f.old == f.new {
			continue
		}
		o.set(f.key, f.new)
	}
}

func has(o *object, key string) bool { _, ok := o.vals[key]; return ok }

func (l *layout) parentOf(id string) string {
	if l.zones[id] != nil {
		return l.parents[id]
	}
	if n := l.nodes[id]; n != nil {
		return nodeZone(n)
	}
	return ""
}

// children of a zone: zones whose parent it is, nodes in it.
func (l *layout) children(zone string) []string {
	var out []string
	for _, id := range l.zoneOrder {
		if id != zone && l.parents[id] == zone {
			out = append(out, id)
		}
	}
	for _, id := range l.nodeOrder {
		if nodeZone(l.nodes[id]) == zone {
			out = append(out, id)
		}
	}
	return out
}

// subtree is the element and everything inside it.
func (l *layout) subtree(id string) []string {
	seen := map[string]bool{}
	var walk func(id string) []string
	walk = func(id string) []string {
		if seen[id] {
			return nil
		}
		seen[id] = true
		out := []string{id}
		if l.isZone(id) {
			for _, c := range l.children(id) {
				out = append(out, walk(c)...)
			}
		}
		return out
	}
	return walk(id)
}

func (l *layout) content(zone string) (Rect, bool) {
	var box Rect
	found := false
	for _, c := range l.children(zone) {
		if found {
			box = box.Union(l.rect(c))
		} else {
			box, found = l.rect(c), true
		}
	}
	return box, found
}

func (l *layout) shift(ids []string, dx, dy float64) {
	seen := map[string]bool{}
	for _, id := range ids {
		for _, t := range l.subtree(id) {
			if seen[t] {
				continue
			}
			seen[t] = true
			r := l.rect(t)
			r.X, r.Y = Snap(r.X+dx), Snap(r.Y+dy)
			l.setRect(t, r)
		}
	}
}

func (l *layout) union(ids []string) Rect {
	box := l.rect(ids[0])
	for _, id := range ids[1:] {
		box = box.Union(l.rect(id))
	}
	return box
}

// keepContent grows a zone's right and bottom so its children stay inside.
func (l *layout) keepContent(zone string, r Rect) Rect {
	if c, ok := l.content(zone); ok {
		r.Width = math.Max(r.Width, c.Right()+ZonePadding-r.X)
		r.Height = math.Max(r.Height, c.Bottom()+ZonePadding-r.Y)
	}
	r.Width, r.Height = math.Max(r.Width, MinZoneWidth), math.Max(r.Height, MinZoneHeight)
	r.Width, r.Height = math.Ceil(r.Width/GridStep)*GridStep, math.Ceil(r.Height/GridStep)*GridStep
	return r
}

// fit sets a zone to its content — caption strip and padding around it — and
// then makes every ancestor still hold what is in it.
func (l *layout) fit(zone string) {
	if c, ok := l.content(zone); ok {
		r := Rect{
			X:     math.Floor((c.X-ZonePadding)/GridStep) * GridStep,
			Y:     math.Floor((c.Y-ZonePadding-ZoneHeader)/GridStep) * GridStep,
			Width: 0, Height: 0,
		}
		r.Width = math.Max(math.Ceil((c.Right()+ZonePadding-r.X)/GridStep)*GridStep, MinZoneWidth)
		r.Height = math.Max(math.Ceil((c.Bottom()+ZonePadding-r.Y)/GridStep)*GridStep, MinZoneHeight)
		l.setRect(zone, r)
	}
	l.growAncestors(zone)
}

func (l *layout) growAncestors(id string) {
	seen := map[string]bool{id: true}
	for p := l.parentOf(id); p != "" && l.zones[p] != nil && !seen[p]; p = l.parentOf(p) {
		seen[p] = true
		c, _ := l.content(p)
		cur := l.rect(p)
		need := Rect{
			X:      math.Min(cur.X, math.Floor((c.X-ZonePadding)/GridStep)*GridStep),
			Y:      math.Min(cur.Y, math.Floor((c.Y-ZonePadding-ZoneHeader)/GridStep)*GridStep),
			Width:  0,
			Height: 0,
		}
		need.Width = math.Max(cur.Right(), math.Ceil((c.Right()+ZonePadding)/GridStep)*GridStep) - need.X
		need.Height = math.Max(cur.Bottom(), math.Ceil((c.Bottom()+ZonePadding)/GridStep)*GridStep) - need.Y
		l.setRect(p, need)
	}
}

// commit applies every changed object as one batch.
func (m *Model) commit(l *layout, extra []Op, author string, human bool) (GeomReport, error) {
	return m.commitMode(l, extra, author, human, false)
}

// commitMode with dry set reports what would change and writes nothing.
func (m *Model) commitMode(l *layout, extra []Op, author string, human, dry bool) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	var ops []Op
	var touched []string
	add := func(kind, id string, o *object, was string) {
		if objString(o) == was {
			return
		}
		b, _ := o.MarshalJSON()
		ops = append(ops, Op{Kind: kind, ID: id, View: l.view, Value: b})
		touched = append(touched, l.view+"#"+id)
	}
	ops = append(ops, extra...)
	for _, id := range l.zoneOrder {
		add("zone", id, l.zones[id], l.before["zone:"+id])
	}
	for _, id := range l.nodeOrder {
		add("node", id, l.nodes[id], l.before["node:"+id])
	}
	for _, op := range extra {
		if op.Kind == "zone" {
			touched = append(touched, l.view+"#"+op.ID)
		}
	}
	if len(ops) == 0 || dry {
		return GeomReport{Touched: append([]string{}, touched...)}, nil
	}
	if _, err := m.Apply(ops, author); err != nil {
		return GeomReport{}, err
	}
	return GeomReport{Touched: touched}, nil
}

// ---------------------------------------------------------------- the steps

// MoveElements shifts by (dx, dy), or puts the top-left corner of the elements'
// common box at (x, y). A zone goes with everything inside it.
func (m *Model) MoveElements(view string, elements []string, dx, dy, x, y *float64, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(elements)
	if err != nil {
		return GeomReport{}, err
	}
	switch {
	case (dx != nil || dy != nil) && (x != nil || y != nil):
		return GeomReport{}, refuse("give dx/dy or x/y, not both")
	case dx != nil || dy != nil:
		l.shift(ids, deref(dx), deref(dy))
	case x != nil || y != nil:
		box := l.union(ids)
		var mx, my float64
		if x != nil {
			mx = *x - box.X
		}
		if y != nil {
			my = *y - box.Y
		}
		l.shift(ids, mx, my)
	default:
		return GeomReport{}, refuse("give dx/dy or x/y")
	}
	for _, id := range ids {
		l.growAncestors(id)
	}
	return m.commit(l, nil, author, human)
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ResizeElements sets width and/or height, not below the minimum of the sort
// and, for a zone, not below what it holds.
func (m *Model) ResizeElements(view string, elements []string, width, height *float64, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	if width == nil && height == nil {
		return GeomReport{}, refuse("give width and/or height")
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(elements)
	if err != nil {
		return GeomReport{}, err
	}
	for _, id := range ids {
		r := l.rect(id)
		if width != nil {
			r.Width = *width
		}
		if height != nil {
			r.Height = *height
		}
		if l.isZone(id) {
			r = l.keepContent(id, r)
		} else {
			r.Width, r.Height = math.Max(Snap(r.Width), MinNodeWidth), math.Max(Snap(r.Height), MinNodeHeight)
		}
		l.setRect(id, r)
		l.growAncestors(id)
	}
	return m.commit(l, nil, author, human)
}

// SetZone puts nodes and zones into a zone (zone "" takes them out of any).
// Coordinates stay as they are.
func (m *Model) SetZone(view string, elements []string, zone string, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(elements)
	if err != nil {
		return GeomReport{}, err
	}
	if zone != "" {
		if zone, err = l.id(zone); err != nil {
			return GeomReport{}, err
		}
		if !l.isZone(zone) {
			return GeomReport{}, refuse("%s is not a zone", zone)
		}
	}
	for _, id := range ids {
		if l.isZone(id) {
			if zone != "" && slices.Contains(l.subtree(id), zone) {
				return GeomReport{}, refuse("zone %s cannot go into itself or its own content", id)
			}
			setOrDrop(l.zones[id], "parent", zone)
			if c := l.zones[id].str("container"); l.zones[c] != nil {
				l.zones[id].set("container", nil)
				if zone != "" {
					l.zones[id].set("container", zone)
				}
			}
		} else {
			key := "zone"
			if !has(l.nodes[id], "zone") && has(l.nodes[id], "container") {
				key = "container"
			}
			if zone == "" {
				l.nodes[id].set(key, nil)
			} else {
				l.nodes[id].set(key, zone)
			}
		}
	}
	return m.commit(l, nil, author, human)
}

func setOrDrop(o *object, key, value string) {
	if value == "" {
		o.del(key)
		return
	}
	o.set(key, value)
}

// ZoneSpec is a new zone. Container is a container id of containers.json or empty.
type ZoneSpec struct {
	ID        string
	Parent    string
	Container string
	StyleID   string
	Name      string
	Lang      string
	Rect
}

// AddZone adds a zone. Its name, when given, is a text under the zone's id.
func (m *Model) AddZone(view string, spec ZoneSpec, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	if !strings.HasPrefix(spec.ID, "z_") || len(spec.ID) == 2 {
		return GeomReport{}, refuse("a zone id starts with z_: %q", spec.ID)
	}
	if l.zones[spec.ID] != nil {
		return GeomReport{}, refuse("zone %s already exists on %s", spec.ID, view)
	}
	if spec.Parent != "" {
		if spec.Parent, err = l.id(spec.Parent); err != nil {
			return GeomReport{}, err
		}
		if !l.isZone(spec.Parent) {
			return GeomReport{}, refuse("%s is not a zone", spec.Parent)
		}
	}
	z := newObject()
	z.set("id", spec.ID)
	if spec.Container != "" {
		z.set("container", spec.Container)
	} else {
		z.set("container", nil)
	}
	if spec.Parent != "" {
		z.set("parent", spec.Parent)
	}
	r := Rect{Snap(spec.X), Snap(spec.Y), math.Max(Snap(spec.Width), MinZoneWidth), math.Max(Snap(spec.Height), MinZoneHeight)}
	z.set("x", r.X)
	z.set("y", r.Y)
	z.set("width", r.Width)
	z.set("height", r.Height)
	if spec.StyleID != "" {
		z.set("styleId", spec.StyleID)
	}
	b, _ := z.MarshalJSON()
	extra := []Op{{Kind: "zone", ID: spec.ID, View: view, Value: b}}
	if spec.Name != "" {
		lang := spec.Lang
		if lang == "" {
			lang = m.firstLanguage()
		}
		txt := newObject()
		entry := newObject()
		entry.set("v", spec.Name)
		entry.set("at", now().Format("2006-01-02T15:04:05Z"))
		entry.set("origin", "authored")
		txt.set("name", entry)
		tb, _ := txt.MarshalJSON()
		extra = append(extra, Op{Kind: "text", ID: spec.ID, Lang: lang, Value: tb})
	}
	l.zones[spec.ID] = z
	l.zoneOrder = append(l.zoneOrder, spec.ID)
	l.before["zone:"+spec.ID] = objString(z)
	rep, err := m.commit(l, extra, author, human)
	if err != nil {
		return rep, err
	}
	// the new zone's own op was counted from extra; a parent that had to grow is in Touched already
	return rep, nil
}

func (m *Model) firstLanguage() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var langs []string
	if raw, ok := m.manifest.vals["languages"]; ok {
		_ = json.Unmarshal(raw, &langs)
	}
	if len(langs) > 0 {
		return langs[0]
	}
	return "ru"
}

// FitZone sets each zone to its content and grows the ancestors that stop holding it.
func (m *Model) FitZone(view string, zones []string, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(zones)
	if err != nil {
		return GeomReport{}, err
	}
	for _, id := range ids {
		if !l.isZone(id) {
			return GeomReport{}, refuse("%s is not a zone", id)
		}
		l.fit(id)
	}
	return m.commit(l, nil, author, human)
}

var alignModes = []string{"left", "right", "top", "bottom", "width", "height"}

// AlignElements lines the elements up on the first one, as the editor's align buttons do.
func (m *Model) AlignElements(view string, elements []string, mode string, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	if !slices.Contains(alignModes, mode) {
		return GeomReport{}, refuse("mode is one of %s", strings.Join(alignModes, ", "))
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(elements)
	if err != nil {
		return GeomReport{}, err
	}
	if len(ids) < 2 {
		return GeomReport{}, refuse("align needs at least two elements")
	}
	ref := l.rect(ids[0])
	for _, id := range ids[1:] {
		r := l.rect(id)
		switch mode {
		case "left":
			l.shift([]string{id}, ref.X-r.X, 0)
		case "right":
			l.shift([]string{id}, ref.Right()-r.Right(), 0)
		case "top":
			l.shift([]string{id}, 0, ref.Y-r.Y)
		case "bottom":
			l.shift([]string{id}, 0, ref.Bottom()-r.Bottom())
		case "width":
			r.Width = ref.Width
			if l.isZone(id) {
				r = l.keepContent(id, r)
			}
			l.setRect(id, r)
		case "height":
			r.Height = ref.Height
			if l.isZone(id) {
				r = l.keepContent(id, r)
			}
			l.setRect(id, r)
		}
		l.growAncestors(id)
	}
	return m.commit(l, nil, author, human)
}
