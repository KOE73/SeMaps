package core

import (
	"math"
	"slices"
	"strings"
)

// Geometry of a view, in steps a person would name: move, resize, put into a
// container, add a container, fit a container to its content, align. Each step
// reads the view, changes copies of its placements, and applies all of them as
// one batch — everything or nothing. Every step needs requestedByHuman
// (CONTRACT §8.2 p. 4, ADR_20260924).

// GeomReport says what a geometry step did. Touched are references `view#entity`
// of every placement whose object changed.
type GeomReport struct {
	Touched []string `json:"touched"`
}

const needHuman = "geometry of a view is written only on a human's direct request (CONTRACT §8.2 p. 4)"

// layout is a view's placements by entity id, in file order. Whether a
// placement is a container is its entity's kind (CONTRACT §8.2).
type layout struct {
	view      string
	items     map[string]*object
	order     []string
	container map[string]bool
	before    map[string]string // entity -> the placement as loaded
	cv        Canvas
}

func (m *Model) loadLayout(viewID string) (*layout, error) {
	cv, err := m.Canvas()
	if err != nil {
		return nil, err
	}
	doc, err := m.view(viewID)
	if err != nil {
		return nil, refuse("no view %s", viewID)
	}
	kinds := map[string]string{}
	for _, e := range m.records("entity") {
		kinds[e.str("id")] = e.str("kind")
	}
	l := &layout{view: viewID, cv: cv, items: map[string]*object{}, container: map[string]bool{}, before: map[string]string{}}
	for _, p := range viewItems(doc, "placements") {
		id := p.str("entity")
		l.items[id] = p
		l.order = append(l.order, id)
		l.container[id] = m.kinds.IsContainer(kinds[id])
		l.before[id] = objString(p)
	}
	return l, nil
}

func objString(o *object) string { b, _ := o.MarshalJSON(); return string(b) }

// id turns a reference `view#entity` or a bare entity id into an entity placed on this view.
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
	if l.items[s] == nil {
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

func (l *layout) isContainer(id string) bool { return l.container[id] }

func (l *layout) rect(id string) Rect { return placementRect(l.items[id], l.container[id], l.cv) }

// setRect writes only the fields that differ, so a placement that never had a
// size does not get one for being moved.
func (l *layout) setRect(id string, r Rect) {
	o := l.items[id]
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

// parentOf is the container a placement lies in on this view; "" when none.
func (l *layout) parentOf(id string) string {
	if o := l.items[id]; o != nil {
		if p := o.str("parent"); p != id && l.container[p] {
			return p
		}
	}
	return ""
}

// children of a container: the placements whose parent it is, in file order.
func (l *layout) children(c string) []string {
	var out []string
	for _, id := range l.order {
		if id != c && l.parentOf(id) == c {
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
		if l.isContainer(id) {
			for _, c := range l.children(id) {
				out = append(out, walk(c)...)
			}
		}
		return out
	}
	return walk(id)
}

func (l *layout) content(c string) (Rect, bool) {
	var box Rect
	found := false
	for _, ch := range l.children(c) {
		if found {
			box = box.Union(l.rect(ch))
		} else {
			box, found = l.rect(ch), true
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
			r.X, r.Y = r.X+dx, r.Y+dy
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

// keepContent grows a container's right and bottom so its children stay inside.
func (l *layout) keepContent(c string, r Rect) Rect {
	if box, ok := l.content(c); ok {
		r.Width = math.Max(r.Width, box.Right()+l.cv.Container.Padding-r.X)
		r.Height = math.Max(r.Height, box.Bottom()+l.cv.Container.Padding-r.Y)
	}
	r.Width, r.Height = math.Max(r.Width, l.cv.Container.MinWidth), math.Max(r.Height, l.cv.Container.MinHeight)
	return r
}

// fit sets a container to its content — caption strip and padding around it —
// and then makes every ancestor still hold what is in it.
func (l *layout) fit(c string) {
	if box, ok := l.content(c); ok {
		cc := l.cv.Container
		r := Rect{X: box.X - cc.Padding, Y: box.Y - cc.Padding - cc.HeaderHeight}
		r.Width = math.Max(box.Right()+cc.Padding-r.X, cc.MinWidth)
		r.Height = math.Max(box.Bottom()+cc.Padding-r.Y, cc.MinHeight)
		l.setRect(c, r)
	}
	l.growAncestors(c)
}

func (l *layout) growAncestors(id string) {
	seen := map[string]bool{id: true}
	cc := l.cv.Container
	for p := l.parentOf(id); p != "" && !seen[p]; p = l.parentOf(p) {
		seen[p] = true
		c, _ := l.content(p)
		cur := l.rect(p)
		need := Rect{
			X: math.Min(cur.X, c.X-cc.Padding),
			Y: math.Min(cur.Y, c.Y-cc.Padding-cc.HeaderHeight),
		}
		need.Width = math.Max(cur.Right(), c.Right()+cc.Padding) - need.X
		need.Height = math.Max(cur.Bottom(), c.Bottom()+cc.Padding) - need.Y
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
	ops = append(ops, extra...)
	for _, id := range l.order {
		o := l.items[id]
		if objString(o) == l.before[id] {
			continue
		}
		b, _ := o.MarshalJSON()
		ops = append(ops, Op{Kind: "placement", ID: id, View: l.view, Value: b})
		touched = append(touched, l.view+"#"+id)
	}
	for _, op := range extra {
		if op.Kind == "placement" {
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
// common box at (x, y). A container goes with everything inside it.
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
// and, for a container, not below what it holds.
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
		if l.isContainer(id) {
			r = l.keepContent(id, r)
		} else {
			r.Width, r.Height = math.Max(r.Width, l.cv.Node.MinWidth), math.Max(r.Height, l.cv.Node.MinHeight)
		}
		l.setRect(id, r)
		l.growAncestors(id)
	}
	return m.commit(l, nil, author, human)
}

// SetParent puts placements into a container (parent "" takes them out of
// any). Coordinates stay as they are.
func (m *Model) SetParent(view string, elements []string, parent string, human bool, author string) (GeomReport, error) {
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
	if parent != "" {
		if parent, err = l.id(parent); err != nil {
			return GeomReport{}, err
		}
		if !l.isContainer(parent) {
			return GeomReport{}, refuse("%s is not a container: its kind is not a container kind of kinds.json", parent)
		}
	}
	for _, id := range ids {
		if parent != "" && slices.Contains(l.subtree(id), parent) {
			return GeomReport{}, refuse("%s cannot go into itself or its own content", id)
		}
		setParent(l.items[id], parent)
	}
	return m.commit(l, nil, author, human)
}

// setParent writes `parent`, null for none: the field is required (CONTRACT §8.2).
func setParent(o *object, parent string) {
	if parent == "" {
		o.set("parent", nil)
		return
	}
	o.set("parent", parent)
}

// ContainerSpec is a new container placement: of an existing entity (Entity),
// or of a new authored entity (Name, Kind; Kind defaults to "group").
type ContainerSpec struct {
	Entity  string
	Name    string
	Kind    string
	Parent  string
	StyleID string
	Rect
}

// AddContainer places a container entity on a view — creating the entity
// first when the spec names none — as one batch. It returns the entity id.
func (m *Model) AddContainer(view string, spec ContainerSpec, human bool, author string) (string, GeomReport, error) {
	if !human {
		return "", GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return "", GeomReport{}, err
	}
	var extra []Op
	id := spec.Entity
	if id != "" {
		if spec.Name != "" || spec.Kind != "" {
			return "", GeomReport{}, refuse("give entity, or name and kind for a new one, not both")
		}
		e := m.record("entity", id)
		if e == nil {
			return "", GeomReport{}, refuse("no entity %s", id)
		}
		if !m.kinds.IsContainer(e.str("kind")) {
			return "", GeomReport{}, refuse("entity %s is of kind %q, which is not a container kind of kinds.json", id, e.str("kind"))
		}
	} else {
		if strings.TrimSpace(spec.Name) == "" {
			return "", GeomReport{}, refuse("give entity, or name (and kind) for a new one")
		}
		kind := orDefault(spec.Kind, "group")
		if !m.kinds.IsContainer(kind) {
			return "", GeomReport{}, refuse("kind %q is not a container kind of kinds.json (get_kinds)", kind)
		}
		taken := map[string]bool{}
		for _, e := range m.records("entity") {
			taken[e.str("id")] = true
		}
		id = mint("e_"+slug(spec.Name), taken)
		e := newObject()
		e.set("id", id)
		e.set("kind", kind)
		e.set("origin", "authored")
		e.set("status", "present")
		b, _ := e.MarshalJSON()
		// its name is a text of the main language, in the same batch (ADR_20260930-5)
		nameOp, err := m.textOp(m.Languages()[0], id, "name", spec.Name)
		if err != nil {
			return "", GeomReport{}, err
		}
		extra = append(extra, Op{Kind: "entity", ID: id, Value: b}, nameOp)
	}
	if l.items[id] != nil {
		return "", GeomReport{}, refuse("%s is already on %s", id, view)
	}
	if spec.Parent != "" {
		if spec.Parent, err = l.id(spec.Parent); err != nil {
			return "", GeomReport{}, err
		}
		if !l.isContainer(spec.Parent) {
			return "", GeomReport{}, refuse("%s is not a container", spec.Parent)
		}
	}
	p := newObject()
	p.set("entity", id)
	setParent(p, spec.Parent)
	r := Rect{spec.X, spec.Y, math.Max(spec.Width, l.cv.Container.MinWidth), math.Max(spec.Height, l.cv.Container.MinHeight)}
	p.set("x", r.X)
	p.set("y", r.Y)
	p.set("width", r.Width)
	p.set("height", r.Height)
	if spec.StyleID != "" {
		p.set("styleId", spec.StyleID)
	}
	b, _ := p.MarshalJSON()
	extra = append(extra, Op{Kind: "placement", ID: id, View: view, Value: b})
	// the new placement is the last of the view; it is written by its own op,
	// its parent grows around it if it must
	l.items[id], l.container[id], l.before[id] = p, true, objString(p)
	l.order = append(l.order, id)
	l.growAncestors(id)
	rep, err := m.commit(l, extra, author, human)
	return id, rep, err
}

// FitContainer sets each container to its content and grows the ancestors
// that stop holding it.
func (m *Model) FitContainer(view string, containers []string, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(containers)
	if err != nil {
		return GeomReport{}, err
	}
	for _, id := range ids {
		if !l.isContainer(id) {
			return GeomReport{}, refuse("%s is not a container", id)
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
			if l.isContainer(id) {
				r = l.keepContent(id, r)
			}
			l.setRect(id, r)
		case "height":
			r.Height = ref.Height
			if l.isContainer(id) {
				r = l.keepContent(id, r)
			}
			l.setRect(id, r)
		}
		l.growAncestors(id)
	}
	return m.commit(l, nil, author, human)
}
