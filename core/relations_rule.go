package core

// The rule that decides which relations a view draws (CONTRACT §8.5): one copy,
// used by get_view and by SetRelationsVisible.

import (
	"encoding/json"
	"slices"
)

// relationRule is how one view decides, read from its `relations`.
type relationRule struct {
	kinds  *KindCatalog
	def    string
	except map[string]bool
}

// relationRule reads the rule of a view doc. A part that is not of the shape is
// read as absent.
func (m *Model) relationRule(doc *object) relationRule {
	r := relationRule{kinds: m.kinds, def: "visible", except: map[string]bool{}}
	raw, ok := doc.vals["relations"]
	if !ok {
		return r
	}
	var parts map[string]json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return r
	}
	var def string
	if json.Unmarshal(parts["default"], &def) == nil && def != "" {
		r.def = def
	}
	var except []string
	_ = json.Unmarshal(parts["except"], &except)
	for _, id := range except {
		r.except[id] = true
	}
	return r
}

// defaultVisible is what the view shows of a relation type before `except`: the
// type's visibility in the dictionary, else the view's `relations.default`,
// else visible.
func (r relationRule) defaultVisible(relType string) bool {
	if v := r.kinds.RelationVisibility(relType); v != "" {
		return v == VisibilityVisible
	}
	return r.def == VisibilityVisible
}

// visible: a relation with both ends placed is drawn when this says so.
func (r relationRule) visible(id, relType string) bool {
	return r.defaultVisible(relType) != r.except[id]
}

// RelationsOfTypes lists the relations of the registry whose type is one of
// types (exact ids) and whose two ends are both placed on the view now, in
// registry order. It is a snapshot: a relation of such a type added to the
// registry later is not in it.
func (m *Model) RelationsOfTypes(viewID string, types []string) ([]string, error) {
	view, err := m.view(viewID)
	if err != nil {
		return nil, err
	}
	placed := placementsOf(view)
	out := []string{}
	for _, r := range m.records("relation") {
		if slices.Contains(types, relationType(r)) && placed[r.str("from")] != nil && placed[r.str("to")] != nil {
			out = append(out, r.str("id"))
		}
	}
	return out, nil
}

// RelationRef is a relation as a tool lists it back: id, ends and type.
type RelationRef struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// RelationRefs describes the relations of ids, in the order given; an id the
// registry does not have is skipped.
func (m *Model) RelationRefs(ids []string) []RelationRef {
	out := []RelationRef{}
	for _, id := range ids {
		if r := m.record("relation", id); r != nil {
			out = append(out, RelationRef{ID: id, From: r.str("from"), To: r.str("to"), Type: relationType(r)})
		}
	}
	return out
}
