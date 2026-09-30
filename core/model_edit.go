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

// SetText writes one field as authored, at = now. What a text may be is Apply's
// rule (ADR_20260926), the same for the editor.
func (m *Model) SetText(lang, key, field, value, author string) error {
	op, err := m.textOp(lang, key, field, value)
	if err != nil {
		return err
	}
	_, err = m.Apply([]Op{op}, author)
	return err
}

// textOp is the op that sets one field of a text entry as authored, at = now.
func (m *Model) textOp(lang, key, field, value string) (Op, error) {
	entry, err := m.text(lang, key)
	if err != nil {
		return Op{}, err
	}
	v := newObject()
	v.set("v", value)
	v.set("at", now().Format("2006-01-02T15:04:05Z"))
	v.set("origin", "authored")
	entry.set(field, v)
	b, _ := entry.MarshalJSON()
	return Op{Kind: "text", ID: key, Lang: lang, Value: b}, nil
}

// AddEntity adds an authored entity — a part of the system no extractor
// reports (CONTRACT §3: kind app, external, database…). The id is id when
// given, else minted from the name; an existing one is refused, not replaced.
// The name is the entity's `name` text in lang (the project's main language
// when empty), written in the same batch as the entity: an authored entity has
// no name in entities.json (CONTRACT §3, §7; ADR_20260930-5).
func (m *Model) AddEntity(id, name, kind, lang, author string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", refuse("name is empty")
	}
	taken := map[string]bool{}
	for _, e := range m.records("entity") {
		taken[e.str("id")] = true
	}
	if id == "" {
		id = mint("e_"+slug(name), taken)
	} else if taken[id] {
		return "", refuse("entity %s exists", id)
	} else if !hasIDPrefix(id, "e_") {
		return "", refuse("entity id %q: starts with e_ (CONTRACT §3)", id)
	}
	o := newObject()
	o.set("id", id)
	o.set("kind", kind)
	o.set("origin", "authored")
	o.set("status", "present")
	b, _ := o.MarshalJSON()
	if lang == "" {
		lang = m.Languages()[0]
	}
	nameOp, err := m.textOp(lang, id, "name", name)
	if err != nil {
		return "", err
	}
	_, err = m.Apply([]Op{{Kind: "entity", ID: id, Value: b}, nameOp}, author)
	return id, err
}

// AddRelation adds an authored relation. That both ends and the type exist is
// Apply's rule; not a self-relation and not a duplicate is the agent's policy.
func (m *Model) AddRelation(from, to, relType, author string) (string, error) {
	if from == to {
		return "", refuse("a relation of an entity to itself is not drawn")
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

// AddRelationType declares an authored relation type of the project. Its name,
// description and base style live in the dictionary (kinds.json, relationGroups),
// not here (ADR_20260930-2).
func (m *Model) AddRelationType(id, visibility, author string) error {
	if m.record("relationType", id) != nil {
		return refuse("type %s exists", id)
	}
	o := newObject()
	o.set("id", id)
	o.set("origin", "authored")
	if visibility != "" {
		o.set("visibility", visibility)
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

// ConfirmEntityRename points the entity's realization in code — the one entry
// of its code[] bound to a symbol — at the symbol it was renamed to. An entity
// realized in several languages is not decided here (ADR_20260930-4).
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
	list := entries(e, "code")
	k := -1
	for i, c := range list {
		if c.str("symbol") == "" {
			continue
		}
		if k >= 0 {
			return refuse("entity %s is realized in several languages: a rename is not confirmed for it yet (ADR_20260930-4)", entityID)
		}
		k = i
	}
	if k < 0 {
		return refuse("entity %s has no realization bound to a symbol: nothing to rename", entityID)
	}
	list[k].set("symbol", symbol)
	setEntries(e, "code", list)
	b, _ := e.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "entity", ID: entityID, Value: b}}, author)
	return err
}

// ConfirmRelationRename renames the member in the `via` of the relation's
// evidence.
func (m *Model) ConfirmRelationRename(relationID, member, author string) error {
	if member == "" {
		return refuse("member is empty")
	}
	r := m.record("relation", relationID)
	if r == nil {
		return refuse("no relation %s", relationID)
	}
	list := entries(r, "evidence")
	k := -1
	for i, c := range list {
		if evidenceVia(c) != nil {
			k = i
			break
		}
	}
	if k < 0 {
		return refuse("relation %s has no via in its evidence: it is not a member relation", relationID)
	}
	via, _ := child(list[k], "via")
	via.set("member", member)
	list[k].set("via", via)
	setEntries(r, "evidence", list)
	b, _ := r.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "relation", ID: relationID, Value: b}}, author)
	return err
}

// PlaceEntities puts entities on a view, each at the end of its placements.
// A parent must be a container placement of the view, or one placed by the
// same call before it.
func (m *Model) PlaceEntities(viewID string, list []Placement, requestedByHuman bool, author string) error {
	if !requestedByHuman {
		return refuse(needHuman)
	}
	view, err := m.view(viewID)
	if err != nil {
		return err
	}
	placed := map[string]bool{}
	for _, p := range viewItems(view, "placements") {
		placed[p.str("entity")] = true
	}
	ops := make([]Op, 0, len(list))
	for _, p := range list {
		if placed[p.Entity] {
			return refuse("%s is already on %s; a placement made by someone is not moved", p.Entity, viewID)
		}
		placed[p.Entity] = true
		n := newObject()
		n.set("entity", p.Entity)
		setParent(n, p.Parent)
		n.set("x", p.X)
		n.set("y", p.Y)
		if p.Width > 0 {
			n.set("width", p.Width)
		}
		if p.Height > 0 {
			n.set("height", p.Height)
		}
		b, _ := n.MarshalJSON()
		ops = append(ops, Op{Kind: "placement", ID: p.Entity, View: viewID, Value: b})
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
