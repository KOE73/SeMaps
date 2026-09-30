package migrate

import (
	"encoding/json"
	"strings"
)

// builtinZoneStyles is a snapshot of the colour styles the tool shipped in
// host/defaults/styles.json before contract v5 (copied at HEAD of the branch
// this package was written on; it is not read at runtime). A zone whose style
// the workspace styles.json does not define is resolved against it.
type builtinStyle struct {
	id, basedOn, fill, border, dash, header string
}

var builtinZoneStyles = []builtinStyle{
	{id: "default.zone", fill: "#f8fafc", border: "#cbd5e1", header: "#e2e8f0"},
	{id: "default.container", fill: "#f8fafc", border: "#cbd5e1", header: "#e2e8f0"},
	{id: "zone.green", basedOn: "default.zone", fill: "#f0fdf4", border: "#86efac", header: "#dcfce7"},
	{id: "zone.blue", basedOn: "default.zone", fill: "#eff6ff", border: "#93c5fd", header: "#dbeafe"},
	{id: "zone.fuchsia", basedOn: "default.zone", fill: "#fdf4ff", border: "#f0abfc", header: "#fae8ff"},
	{id: "zone.red", basedOn: "default.zone", fill: "#fff1f2", border: "#fca5a5", header: "#ffe4e6"},
	{id: "zone.yellow", basedOn: "default.zone", fill: "#fefce8", border: "#fde047", header: "#fef9c3"},
	{id: "zone.slate", basedOn: "default.zone", fill: "#f8fafc", border: "#cbd5e1", header: "#e2e8f0"},
	{id: "zone.violet", basedOn: "default.zone", fill: "#f5f3ff", border: "#c4b5fd", header: "#ede9fe"},
	{id: "zone.amber", basedOn: "default.zone", fill: "#fffbeb", border: "#fde68a", header: "#fef3c7"},
	{id: "zone.sky", basedOn: "default.zone", fill: "#f0f9ff", border: "#7dd3fc", header: "#e0f2fe"},
	{id: "zone.cyan", basedOn: "default.zone", fill: "#ecfeff", border: "#67e8f9", header: "#cffafe"},
	{id: "zone.lime", basedOn: "default.zone", fill: "#f7fee7", border: "#bef264", header: "#ecfccb"},
	{id: "zone.pink", basedOn: "default.zone", fill: "#fdf2f8", border: "#f9a8d4", header: "#fce7f3"},
	{id: "zone.gray", basedOn: "default.zone", fill: "#f1f5f9", border: "#94a3b8", header: "#e2e8f0"},
	{id: "boundary", basedOn: "default.zone"},
	{id: "subsystem", basedOn: "zone.blue"},
	{id: "zone.red.dashed", basedOn: "zone.red", dash: "6,4"},
}

// styleDef is the part of a style a container override needs.
type styleDef struct {
	basedOn string
	fill    any // raw JSON value (a colour string or a gradient object)
	border  any
	dash    any
	header  any
}

func builtinDef(id string) (styleDef, bool) {
	for _, b := range builtinZoneStyles {
		if b.id != id {
			continue
		}
		d := styleDef{basedOn: b.basedOn}
		if b.fill != "" {
			d.fill = jsonStr(b.fill)
		}
		if b.border != "" {
			d.border = jsonStr(b.border)
		}
		if b.dash != "" {
			d.dash = jsonStr(b.dash)
		}
		if b.header != "" {
			d.header = jsonStr(b.header)
		}
		return d, true
	}
	return styleDef{}, false
}

func defFromObj(o *obj) styleDef {
	d := styleDef{}
	d.basedOn, _, _ = strField(o, "basedOn")
	d.fill = o.get("fill")
	if b, ok := asObj(o.get("border")); ok {
		d.border = b.get("color")
		d.dash = b.get("dash")
	}
	if h, ok := asObj(o.get("header")); ok {
		d.header = h.get("fill")
	}
	return d
}

// styleLib resolves style ids against the workspace styles.json first, then
// the built-in snapshot.
type styleLib struct {
	ws map[string]*obj
}

func newStyleLib(styles []any) *styleLib {
	l := &styleLib{ws: map[string]*obj{}}
	for _, s := range styles {
		o, ok := asObj(s)
		if !ok {
			continue
		}
		if id, ok := asStr(o.get("id")); ok {
			if _, dup := l.ws[id]; !dup {
				l.ws[id] = o
			}
		}
	}
	return l
}

func (l *styleLib) lookup(id string) (styleDef, bool) {
	if l != nil {
		if o, ok := l.ws[id]; ok {
			return defFromObj(o), true
		}
	}
	return builtinDef(id)
}

func isContainerBase(id string) bool {
	return id == "default.zone" || id == "default.container" ||
		strings.HasPrefix(id, "zone.") || strings.HasPrefix(id, "container.")
}

// resolveColour follows basedOn upward (nearer wins). colour reports whether id
// is a colour style of a container: by its name or because its chain reaches
// default.zone / default.container. found is false when no definition exists.
func (l *styleLib) resolveColour(id string) (res styleDef, colour, found bool) {
	seen := map[string]bool{}
	cur := id
	for cur != "" && !seen[cur] {
		seen[cur] = true
		if isContainerBase(cur) {
			colour = true
		}
		d, ok := l.lookup(cur)
		if !ok {
			break
		}
		found = true
		if res.fill == nil {
			res.fill = d.fill
		}
		if res.border == nil {
			res.border = d.border
		}
		if res.dash == nil {
			res.dash = d.dash
		}
		if res.header == nil {
			res.header = d.header
		}
		cur = d.basedOn
	}
	return res, colour, found
}

// override builds the v5 override object of a resolved style; nil when the
// style yields none of the overridable colour fields.
func (d styleDef) override() *obj {
	o := newObj()
	if d.fill != nil {
		o.set("fill", deepCopy(d.fill))
	}
	if d.border != nil || d.dash != nil {
		b := newObj()
		if d.border != nil {
			b.set("color", deepCopy(d.border))
		}
		if d.dash != nil {
			b.set("dash", deepCopy(d.dash))
		}
		o.set("border", b)
	}
	if d.header != nil {
		h := newObj()
		h.set("fill", deepCopy(d.header))
		o.set("header", h)
	}
	if len(o.keys) == 0 {
		return nil
	}
	return o
}

// rawEqual compares two raw JSON leaves.
func rawEqual(a, b any) bool {
	x, ok1 := a.(json.RawMessage)
	y, ok2 := b.(json.RawMessage)
	return ok1 && ok2 && string(x) == string(y)
}
