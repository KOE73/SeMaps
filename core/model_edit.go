package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func (m *Model) record(kind, id string) *object {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.registries[kind]; r != nil {
		if o := findByID(r.items, id); o != nil {
			return cloneObject(o)
		}
	}
	return nil
}

func (m *Model) records(kind string) []*object {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.registries[kind]
	if r == nil {
		return nil
	}
	out := make([]*object, len(r.items))
	for i, o := range r.items {
		out[i] = cloneObject(o)
	}
	return out
}

func (m *Model) view(id string) (*object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.loadView(id)
	if err != nil {
		return nil, err
	}
	return cloneObject(v.doc), nil
}

func (m *Model) text(lang, key string) (*object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.loadText(lang)
	if err != nil {
		return nil, err
	}
	entries, err := child(d, "entries")
	if err != nil {
		return nil, err
	}
	return child(entries, key)
}

func (m *Model) SetText(lang, key, field, value, author string) error {
	if lang == "" || strings.ContainsAny(lang, `/\.`) {
		return refuse("language %q: expected a code like ru or en", lang)
	}
	if !slices.Contains(TextFields, field) {
		return refuse("field %q: allowed %s (CONTRACT §7.2)", field, strings.Join(TextFields, ", "))
	}
	if strings.TrimSpace(value) == "" {
		return refuse("empty text; there is no deleting a text")
	}
	switch {
	case strings.HasPrefix(key, "e_"):
		if field == "name" || field == "title" {
			return refuse("an entity's name is not translated and lives in entities.json (CONTRACT §7.1)")
		}
	case strings.HasPrefix(key, "r_"):
		r := m.record("relation", key)
		if r == nil {
			return refuse("no relation %s", key)
		}
		if r.str("origin") == "code" {
			return refuse("relation %s comes from code: it has no texts, its label is drawn from `via` (CONTRACT §4)", key)
		}
	case strings.HasPrefix(key, "c_"), strings.HasPrefix(key, "rt_"), strings.HasPrefix(key, "v_"), strings.HasPrefix(key, "z_"):
	default:
		return refuse("key %q: expected a prefix e_, c_, rt_, r_, v_ or z_ (CONTRACT §7.1)", key)
	}
	entry, err := m.text(lang, key)
	if err != nil {
		return err
	}
	v := newObject()
	v.set("v", value)
	v.set("at", now().Format("2006-01-02T15:04:05Z"))
	v.set("origin", "authored")
	entry.set(field, v)
	b, _ := entry.MarshalJSON()
	_, err = m.Apply([]Op{{Kind: "text", ID: key, Lang: lang, Value: b}}, author)
	return err
}

func (m *Model) AddRelation(from, to, relType, author string) (string, error) {
	ents := m.records("entity")
	for _, e := range []string{from, to} {
		if findByID(ents, e) == nil {
			return "", refuse("no entity %s", e)
		}
	}
	if from == to {
		return "", refuse("a relation of an entity to itself is not drawn")
	}
	if findByID(m.records("relationType"), relType) == nil {
		return "", refuse("no relation type %s; add it first (add_relation_type)", relType)
	}
	taken := map[string]bool{}
	for _, r := range m.records("relation") {
		taken[r.str("id")] = true
		if r.str("from") == from && r.str("to") == to && relationType(r) == relType && r.str("origin") != "code" {
			return "", refuse("relation %s already says %s → %s (%s)", r.str("id"), from, to, relType)
		}
	}
	id := mint("r_"+strings.TrimPrefix(from, "e_")+"_"+strings.TrimPrefix(to, "e_")+"_"+slug(relType), taken)
	o := newObject()
	o.set("id", id)
	o.set("from", from)
	o.set("to", to)
	o.set("type", relType)
	o.set("origin", "authored")
	b, _ := o.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "relation", ID: id, Value: b}}, author)
	return id, err
}

func (m *Model) AddRelationType(id, visibility, styleID, author string) error {
	if id == "" || strings.ContainsAny(id, " \t\"") {
		return refuse("type id %q: a word without spaces", id)
	}
	if visibility != "" && visibility != "visible" && visibility != "hidden" {
		return refuse("visibility %q: visible, hidden or nothing", visibility)
	}
	if m.record("relationType", id) != nil {
		return refuse("type %s exists", id)
	}
	o := newObject()
	o.set("id", id)
	o.set("origin", "authored")
	if visibility != "" {
		o.set("visibility", visibility)
	}
	if styleID != "" {
		o.set("styleId", styleID)
	}
	b, _ := o.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "relationType", ID: id, Value: b}}, author)
	return err
}

func (m *Model) SetRelationVisible(viewID, relationID string, visible bool, author string) error {
	r := m.record("relation", relationID)
	if r == nil {
		return refuse("no relation %s", relationID)
	}
	view, err := m.view(viewID)
	if err != nil {
		return err
	}
	policy, err := child(view, "relations")
	if err != nil {
		return fmt.Errorf("%s: relations: %w", viewID, err)
	}
	def := policy.str("default")
	if t := m.record("relationType", relationType(r)); t != nil && t.str("visibility") != "" {
		def = t.str("visibility")
	}
	if def == "" {
		def = "visible"
	}
	var except []string
	if raw, ok := policy.vals["except"]; ok {
		if err := json.Unmarshal(raw, &except); err != nil {
			return fmt.Errorf("%s: relations.except: %w", viewID, err)
		}
	}
	except = slices.DeleteFunc(except, func(s string) bool { return s == relationID })
	if visible != (def == "visible") {
		except = append(except, relationID)
	}
	if except == nil {
		except = []string{}
	}
	policy.set("except", except)
	props := newObject()
	props.set("id", viewID)
	props.set("relations", policy)
	b, _ := props.MarshalJSON()
	_, err = m.Apply([]Op{{Kind: "view", ID: viewID, View: viewID, Value: b}}, author)
	return err
}

func (m *Model) ConfirmEntityRename(entityID, symbol, author string) error {
	if symbol == "" {
		return refuse("symbol is empty")
	}
	e := m.record("entity", entityID)
	if e == nil {
		return refuse("no entity %s", entityID)
	}
	if e.str("origin") == "authored" {
		return refuse("entity %s is authored: it has no symbol", entityID)
	}
	e.set("symbol", symbol)
	b, _ := e.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "entity", ID: entityID, Value: b}}, author)
	return err
}

func (m *Model) ConfirmRelationRename(relationID, member, author string) error {
	if member == "" {
		return refuse("member is empty")
	}
	r := m.record("relation", relationID)
	if r == nil {
		return refuse("no relation %s", relationID)
	}
	via, err := child(r, "via")
	if err != nil || len(via.keys) == 0 {
		return refuse("relation %s has no via: it is not a member relation", relationID)
	}
	via.set("member", member)
	r.set("via", via)
	b, _ := r.MarshalJSON()
	_, err = m.Apply([]Op{{Kind: "relation", ID: relationID, Value: b}}, author)
	return err
}

func (m *Model) PlaceEntities(viewID string, list []Placement, requestedByHuman bool, author string) error {
	if !requestedByHuman {
		return refuse("geometry of a view is written only on a human's direct request (CONTRACT §8.2 p. 3)")
	}
	view, err := m.view(viewID)
	if err != nil {
		return err
	}
	key := "nodes"
	if _, ok := view.vals["nodes"]; !ok {
		if _, ok := view.vals["placements"]; ok {
			key = "placements"
		}
	}
	var nodes []*object
	if raw := view.vals[key]; len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return fmt.Errorf("%s: %s: %w", viewID, key, err)
		}
	}
	placed := map[string]bool{}
	for _, n := range nodes {
		placed[orDefault(n.str("entity"), n.str("id"))] = true
	}
	zones := map[string]bool{}
	if raw := view.vals["zones"]; len(raw) > 0 {
		var zs []*object
		if err := json.Unmarshal(raw, &zs); err == nil {
			for _, z := range zs {
				zones[z.str("id")] = true
			}
		}
	}
	ents := m.records("entity")
	ops := make([]Op, 0, len(list))
	for _, p := range list {
		switch {
		case findByID(ents, p.Entity) == nil:
			return refuse("no entity %s", p.Entity)
		case placed[p.Entity]:
			return refuse("%s is already on %s; a placement made by someone is not moved", p.Entity, viewID)
		case p.Zone != "" && !zones[p.Zone]:
			return refuse("no zone %s on %s", p.Zone, viewID)
		}
		placed[p.Entity] = true
		n := newObject()
		n.set("entity", p.Entity)
		if p.Zone != "" {
			n.set("zone", p.Zone)
		} else {
			n.set("zone", nil)
		}
		n.set("x", p.X)
		n.set("y", p.Y)
		if p.Width > 0 {
			n.set("width", p.Width)
		}
		if p.Height > 0 {
			n.set("height", p.Height)
		}
		b, _ := n.MarshalJSON()
		ops = append(ops, Op{Kind: "node", ID: p.Entity, View: viewID, Value: b})
	}
	_, err = m.Apply(ops, author)
	return err
}

// Records and Text expose the current working state to API and MCP readers.
func (m *Model) Records(file string) ([]json.RawMessage, error) {
	kind := map[string]string{"entities.json": "entity", "relations.json": "relation", "relation-types.json": "relationType"}[file]
	if kind == "" {
		return nil, fmt.Errorf("not a registry file: %s", file)
	}
	items := m.records(kind)
	out := make([]json.RawMessage, len(items))
	for i, o := range items {
		out[i], _ = o.MarshalJSON()
	}
	return out, nil
}

func (m *Model) Text(lang, key string) (json.RawMessage, error) {
	o, err := m.text(lang, key)
	if err != nil {
		return nil, err
	}
	if len(o.keys) == 0 {
		return nil, nil
	}
	return o.MarshalJSON()
}

func (m *Model) View(id string) (json.RawMessage, error) {
	o, err := m.view(id)
	if err != nil {
		return nil, err
	}
	return o.MarshalJSON()
}

func (m *Model) RegistrySnapshot() map[string]json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]json.RawMessage{}
	for kind, spec := range modelRegistries {
		r := m.registries[kind]
		top := cloneObject(r.top)
		items := r.items
		if items == nil {
			items = []*object{}
		}
		top.set(spec.key, items)
		out[spec.file], _ = top.MarshalJSON()
	}
	return out
}

func (m *Model) TextSnapshot() (map[string]json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var langs []string
	_ = json.Unmarshal(m.manifest.vals["languages"], &langs)
	if len(langs) == 0 {
		langs = []string{"ru"}
	}
	for lang := range m.loaded {
		if !slices.Contains(langs, lang) {
			langs = append(langs, lang)
		}
	}
	out := map[string]json.RawMessage{}
	for _, lang := range langs {
		doc, err := m.loadText(lang)
		if err != nil {
			return nil, err
		}
		out[lang], _ = doc.MarshalJSON()
	}
	return out, nil
}

func (m *Model) Manifest() json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := m.manifest.MarshalJSON()
	return b
}

func (m *Model) ProjectID() string  { return m.project }
func (m *Model) ProjectDir() string { return filepath.Clean(m.dir) }
