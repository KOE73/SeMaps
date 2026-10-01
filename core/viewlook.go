package core

import (
	"encoding/json"
	"slices"
	"strings"
)

// The look of a view, in steps a person would name: the style, override,
// template and collapsed state of placements; the shape of lines. Like the
// geometry steps, each is one batch through Apply, so the write rules of
// rules.go decide what is valid; a step needs requestedByHuman.

// PlacementLookFields are the fields of a placement that SetPlacement changes.
var PlacementLookFields = []string{"styleId", "override", "template", "collapsed"}

// SetPlacement changes the look of placements already on the view. fields holds
// only the fields the caller gave, each as its JSON value; `null` drops the field.
// Geometry and parent are not touched. Whether a value is fine (an override of the
// table, a collapsed container) is the rule's (checkPlacement).
func (m *Model) SetPlacement(view string, elements []string, fields map[string]json.RawMessage, human bool, author string) (GeomReport, error) {
	if !human {
		return GeomReport{}, refuse(needHuman)
	}
	for key := range fields {
		if !slices.Contains(PlacementLookFields, key) {
			return GeomReport{}, refuse("field %q: set_placement changes %s", key, strings.Join(PlacementLookFields, ", "))
		}
	}
	if len(fields) == 0 {
		return GeomReport{}, refuse("give at least one of %s", strings.Join(PlacementLookFields, ", "))
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	l, err := m.loadLayout(view)
	if err != nil {
		return GeomReport{}, err
	}
	ids, err := l.ids(elements)
	if err != nil {
		return GeomReport{}, err
	}
	for _, id := range ids {
		o := l.items[id]
		for _, key := range PlacementLookFields { // the contract's field order
			raw, ok := fields[key]
			if !ok {
				continue
			}
			if string(raw) == "null" {
				o.del(key)
			} else {
				o.set(key, raw)
			}
		}
	}
	return m.commit(l, nil, author, human)
}

// RoutingModes: the values of `routing` (CONTRACT §11.3).
var RoutingModes = []string{"bezier", "orthogonal", "tree-horizontal", "tree-vertical"}

// RoutingReport says what SetRouting did.
type RoutingReport struct {
	// Relations: the relations whose entry on the view changed.
	Relations []string `json:"relations,omitempty"`
	// NotDrawn: given relations that the view does not draw now (an end not placed, or hidden).
	NotDrawn []string `json:"notDrawn,omitempty"`
}

// SetRouting sets the shape of lines on a view: without relations, the view's
// own `routing` (nil removes the key, the relation type's or style's choice
// applies); with relations, the `routing` of the view's `edges` entry of each
// (CONTRACT §8.5, ADR_20260930-7) — the entry is created when missing, an entry
// left with nothing of its own is removed, and so is the `edges` key when the
// list empties.
func (m *Model) SetRouting(view string, routing *string, relations []string, human bool, author string) (RoutingReport, error) {
	if !human {
		return RoutingReport{}, refuse(needHuman)
	}
	if routing != nil && !slices.Contains(RoutingModes, *routing) {
		return RoutingReport{}, refuse("routing %q: one of %s, or null (CONTRACT §11.3)", *routing, strings.Join(RoutingModes, ", "))
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	doc, err := m.view(view)
	if err != nil {
		return RoutingReport{}, refuse("no view %s", view)
	}
	props := newObject()
	props.set("id", view)
	var rep RoutingReport
	if len(relations) == 0 {
		if routing == nil {
			props.set("routing", nil)
		} else {
			props.set("routing", *routing)
		}
	} else {
		var ids []string
		for _, id := range relations {
			id = strings.TrimSpace(id)
			if m.record("relation", id) == nil {
				return RoutingReport{}, refuse("no relation %s in the registry: a line is a relation, and its shape is set on it (CONTRACT §8.5)", id)
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		list := viewItems(doc, "edges")
		for _, id := range ids {
			at := slices.IndexFunc(list, func(e *object) bool { return e.str("id") == id })
			if at < 0 {
				if routing == nil {
					continue // no entry, nothing to remove
				}
				e := newObject()
				e.set("id", id)
				list = append(list, e)
				at = len(list) - 1
			}
			if routing == nil {
				list[at].del("routing")
			} else {
				list[at].set("routing", *routing)
			}
			rep.Relations = append(rep.Relations, id)
		}
		// an entry with nothing of its own is not written
		kept := list[:0:0]
		for _, e := range list {
			override, hasOverride := e.vals["override"]
			if e.str("styleId") == "" && e.str("routing") == "" && !(hasOverride && string(override) != "null") {
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			props.set("edges", nil)
		} else {
			props.set("edges", kept)
		}
		if info, err := m.GetView(view); err == nil {
			drawn := map[string]bool{}
			for _, e := range info.Edges {
				drawn[e.ID] = true
			}
			for _, id := range ids {
				if !drawn[id] {
					rep.NotDrawn = append(rep.NotDrawn, id)
				}
			}
		}
	}
	b, _ := props.MarshalJSON()
	if _, err := m.Apply([]Op{{Kind: "view", ID: view, View: view, Value: b}}, author); err != nil {
		return RoutingReport{}, err
	}
	return rep, nil
}
