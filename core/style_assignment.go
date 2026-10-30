package core

import "encoding/json"

func (m *Model) SetStyleRuleExclusions(view string, ids []string, human bool, author string) ([]Ref, error) {
	if !human {
		return nil, refuse(needHuman)
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	m.mu.Lock()
	_, err := m.loadView(view)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	doc := newObject()
	doc.set("id", view)
	if len(ids) == 0 {
		doc.set("disabledStyleRules", nil)
	} else {
		doc.set("disabledStyleRules", ids)
	}
	m.mu.Unlock()
	return m.Apply([]Op{{Kind: "view", ID: view, View: view, Value: encodeJSON(doc)}}, author)
}

// SetStyleModifiers edits only reference/exclusion fields. It never materialises
// automatic formatting or rewrites geometry, parents, templates or overrides.
func (m *Model) SetStyleModifiers(view string, elements, relations []string, fields map[string]json.RawMessage, human bool, author string) ([]Ref, error) {
	if !human {
		return nil, refuse(needHuman)
	}
	if len(elements) == 0 && len(relations) == 0 {
		return nil, refuse("give elements or relations")
	}
	if len(fields) == 0 {
		return nil, refuse("give modifiers, disabledModifiers or automaticModifiers")
	}
	for key := range fields {
		if key != "modifiers" && key != "disabledModifiers" && key != "automaticModifiers" {
			return nil, refuse("unsupported modifier assignment field %s", key)
		}
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	m.mu.Lock()
	v, err := m.loadView(view)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	doc := cloneObject(v.doc)
	ops := []Op{}
	apply := func(p *object) {
		for key, raw := range fields {
			if string(raw) == "null" {
				p.del(key)
			} else {
				p.set(key, raw)
			}
		}
	}
	for _, id := range elements {
		p := findPlacement(doc, id)
		if p == nil {
			m.mu.Unlock()
			return nil, refuse("%s is not placed on %s", id, view)
		}
		p = cloneObject(p)
		apply(p)
		ops = append(ops, Op{Kind: "placement", ID: id, View: view, Value: encodeJSON(p)})
	}
	if len(relations) > 0 {
		edges := viewItems(doc, "edges")
		for _, id := range relations {
			if findByID(m.registries["relation"].items, id) == nil {
				m.mu.Unlock()
				return nil, refuse("unknown relation %s", id)
			}
			p := findByID(edges, id)
			if p == nil {
				p = newObject()
				p.set("id", id)
				edges = append(edges, p)
			}
			apply(p)
		}
		edges = filterOwnEdges(edges)
		doc.set("edges", edges)
		doc.del("placements")
		ops = append(ops, Op{Kind: "view", ID: view, View: view, Value: encodeJSON(doc)})
	}
	m.mu.Unlock()
	return m.Apply(ops, author)
}

func filterOwnEdges(edges []*object) []*object {
	out := []*object{}
	for _, e := range edges {
		if e.str("styleId") != "" || has(e, "override") || has(e, "routing") || has(e, "modifiers") || has(e, "disabledModifiers") || has(e, "automaticModifiers") {
			out = append(out, e)
		}
	}
	return out
}
