package core

// The contract rules every writer obeys — editor, agent, sync (ADR_20260926).
// A batch is checked as a whole, after all its operations, and only for what it
// creates or changes: a new object entirely, a changed one by its changed
// fields. Data already on disk that breaks a rule does not block other edits.
// Policy that binds only the agent (requestedByHuman, no moving someone's
// placement, minted ids) stays in the MCP verbs.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
)

type opKey struct{ kind, id, view, lang string }

// validate checks the objects ops touched in after (the model with the batch
// applied) against before (the model without it).
func (after *Model) validate(before *Model, ops []Op) error {
	seen := map[opKey]bool{}
	for _, op := range ops {
		k := opKey{op.Kind, op.ID, op.View, op.Lang}
		if seen[k] {
			continue
		}
		seen[k] = true
		var err error
		switch op.Kind {
		case "entity":
			err = after.checkEntity(before, op.ID)
		case "relation":
			err = after.checkRelation(before, op.ID)
		case "relationType":
			err = after.checkRelationType(before, op.ID)
		case "text":
			err = after.checkText(before, op.Lang, op.ID)
		case "zone":
			err = after.checkZone(before, op.View, op.ID)
		case "node":
			err = after.checkNode(before, op.View, op.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// changed: o is new, or field differs from what old had.
func changed(old, o *object, field string) bool {
	if old == nil {
		return true
	}
	return !bytes.Equal(old.vals[field], o.vals[field])
}

func oneOf(o *object, field string, allowed ...string) bool {
	if _, ok := o.vals[field]; !ok {
		return true
	}
	v := o.str(field)
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func (after *Model) checkEntity(before *Model, id string) error {
	old, o := findByID(before.registries["entity"].items, id), findByID(after.registries["entity"].items, id)
	switch {
	case old == nil && !hasIDPrefix(id, "e_"):
		return refuse("entity id %q: starts with e_ (CONTRACT §3)", id)
	case changed(old, o, "name") && strings.TrimSpace(o.str("name")) == "":
		return refuse("entity %s: name is empty (CONTRACT §3)", id)
	case changed(old, o, "kind") && strings.TrimSpace(o.str("kind")) == "":
		return refuse("entity %s: kind is empty (CONTRACT §3)", id)
	case changed(old, o, "origin") && !oneOf(o, "origin", "code", "authored"):
		return refuse("entity %s: origin %q: code or authored (CONTRACT §3)", id, o.str("origin"))
	case changed(old, o, "status") && !oneOf(o, "status", "present", "missing", "planned"):
		return refuse("entity %s: status %q: present, missing or planned (CONTRACT §3)", id, o.str("status"))
	}
	return nil
}

func (after *Model) checkRelation(before *Model, id string) error {
	old, o := findByID(before.registries["relation"].items, id), findByID(after.registries["relation"].items, id)
	if old == nil && !hasIDPrefix(id, "r_") {
		return refuse("relation id %q: starts with r_ (CONTRACT §4)", id)
	}
	entities := after.registries["entity"].items
	for _, end := range []string{"from", "to"} {
		if changed(old, o, end) && findByID(entities, o.str(end)) == nil {
			return refuse("relation %s: %s: no entity %s (CONTRACT §4)", id, end, o.str(end))
		}
	}
	if changed(old, o, "type") || changed(old, o, "relation") {
		t := relationType(o)
		if findByID(after.registries["relationType"].items, t) == nil {
			return refuse("relation %s: no relation type %q in relation-types.json (CONTRACT §4)", id, t)
		}
	}
	if changed(old, o, "origin") && !oneOf(o, "origin", "code", "authored") {
		return refuse("relation %s: origin %q: code or authored (CONTRACT §4)", id, o.str("origin"))
	}
	return nil
}

func (after *Model) checkRelationType(before *Model, id string) error {
	old, o := findByID(before.registries["relationType"].items, id), findByID(after.registries["relationType"].items, id)
	if old == nil && strings.ContainsAny(id, " \t\"") {
		return refuse("type id %q: a word without spaces", id)
	}
	if changed(old, o, "visibility") && !oneOf(o, "visibility", "visible", "hidden") {
		return refuse("type %s: visibility %q: visible, hidden or nothing", id, o.str("visibility"))
	}
	return nil
}

// checkText: every changed field is a value with provenance and a non-empty
// text; a text is never cleared (CONTRACT §7).
func (after *Model) checkText(before *Model, lang, key string) error {
	entry := textEntry(after.texts[lang], key)
	var old *object
	if d, err := before.loadTextNoCache(lang); err == nil {
		old = textEntry(d, key)
	}
	for _, field := range entry.keys {
		if old != nil && bytes.Equal(old.vals[field], entry.vals[field]) {
			continue
		}
		var v struct {
			V      *string `json:"v"`
			Origin string  `json:"origin"`
			At     string  `json:"at"`
		}
		if json.Unmarshal(entry.vals[field], &v) != nil || v.V == nil {
			return refuse("%s.%s (%s): a value {v, origin, at} (CONTRACT §7.2)", key, field, lang)
		}
		switch {
		case strings.TrimSpace(*v.V) == "":
			return refuse("%s.%s (%s): empty text; there is no deleting a text", key, field, lang)
		case v.Origin != "authored" && v.Origin != "translated":
			return refuse("%s.%s (%s): origin %q: authored or translated (CONTRACT §7.2)", key, field, lang, v.Origin)
		case v.At == "":
			return refuse("%s.%s (%s): at is missing (CONTRACT §7.2)", key, field, lang)
		}
	}
	return nil
}

func textEntry(doc *object, key string) *object {
	if doc == nil {
		return nil
	}
	entries, err := child(doc, "entries")
	if err != nil {
		return nil
	}
	e, err := child(entries, key)
	if err != nil {
		return nil
	}
	return e
}

// loadTextNoCache is the catalogue as before holds it, without loading it into
// before: Apply holds before's lock and must leave it untouched.
func (m *Model) loadTextNoCache(lang string) (*object, error) {
	if m.loaded[lang] {
		return m.texts[lang], nil
	}
	return loadDoc(filepath.Join(m.dir, "text."+lang+".json"))
}

// viewObjects: zones or nodes of a view doc by id (a node by its entity).
func viewObjects(doc *object, kind string) map[string]*object {
	out := map[string]*object{}
	if doc == nil {
		return out
	}
	if kind == "zone" {
		for _, z := range viewItems(doc, "zones") {
			out[z.str("id")] = z
		}
		return out
	}
	for _, key := range []string{"nodes", "placements"} {
		for _, n := range viewItems(doc, key) {
			out[nodeID(n)] = n
		}
	}
	return out
}

func (m *Model) viewDocNoCache(view string) *object {
	if v := m.views[view]; v != nil {
		return v.doc
	}
	_, doc, err := loadView(m.dir, view)
	if err != nil {
		return nil
	}
	return doc
}

func (after *Model) checkZone(before *Model, view, id string) error {
	o := viewObjects(after.views[view].doc, "zone")[id]
	if o == nil {
		return nil // removed
	}
	old := viewObjects(before.viewDocNoCache(view), "zone")[id]
	if old == nil && !hasIDPrefix(id, "z_") {
		return refuse("zone id %q: starts with z_ (CONTRACT §8.2)", id)
	}
	if !changed(old, o, "parent") {
		return nil
	}
	zones := viewObjects(after.views[view].doc, "zone")
	for p, hops := o.str("parent"), 0; p != ""; p, hops = zones[p].str("parent"), hops+1 {
		if zones[p] == nil {
			return refuse("%s is not a zone", p)
		}
		if p == id || hops > len(zones) {
			return refuse("zone %s cannot go into itself or its own content", id)
		}
	}
	return nil
}

func (after *Model) checkNode(before *Model, view, id string) error {
	o := viewObjects(after.views[view].doc, "node")[id]
	if o == nil {
		return nil // removed
	}
	old := viewObjects(before.viewDocNoCache(view), "node")[id]
	if (changed(old, o, "entity") || changed(old, o, "id")) && findByID(after.registries["entity"].items, id) == nil {
		return refuse("no entity %s: a node places an entity of entities.json (CONTRACT §8.2)", id)
	}
	zoneKey := "zone"
	if _, ok := o.vals["zone"]; !ok {
		zoneKey = "container" // legacy spelling (CONTRACT §8.2)
	}
	if z := o.str(zoneKey); z != "" && changed(old, o, zoneKey) && viewObjects(after.views[view].doc, "zone")[z] == nil {
		return refuse("no zone %s on %s", z, view)
	}
	return nil
}

// hasIDPrefix: the id starts with prefix and has something after it.
func hasIDPrefix(id, prefix string) bool {
	return strings.HasPrefix(id, prefix) && len(id) > len(prefix)
}
