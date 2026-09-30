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
	"slices"
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
		case "entity", "relation":
			// what the batch leaves out of the registry, it withdrew
			switch {
			case findByID(after.registries[op.Kind].items, op.ID) == nil:
				err = after.checkWithdrawn(op.Kind, op.ID)
			case op.Kind == "entity":
				err = after.checkEntity(before, op.ID)
			default:
				err = after.checkRelation(before, op.ID)
			}
		case "text":
			err = after.checkText(before, op.Lang, op.ID)
		case "placement":
			err = after.checkPlacement(before, op.View, op.ID)
		case "view":
			err = after.checkViewEdges(before, op.View)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// checkWithdrawn: a record withdrawn by the batch is named nowhere (ADR_20260930-8).
// A withdrawn entity stands on no view and is no end of a relation; a withdrawn
// relation is in no view's `edges` or `relations.except`. Every view of the
// project is looked at, not only those the batch touched: the same id may stand
// on a view the editor never opened.
func (after *Model) checkWithdrawn(kind, id string) error {
	if kind == "entity" {
		for _, r := range after.registries["relation"].items {
			if r.str("from") == id || r.str("to") == id {
				return refuse("cannot withdraw entity %s: relation %s still has it as an end; withdraw the relation in the same batch", id, r.str("id"))
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(after.dir, "views", "*.view.json"))
	for _, file := range files {
		doc, err := loadDoc(file)
		if err != nil {
			return err
		}
		view := doc.str("id")
		if view == "" {
			view = strings.TrimSuffix(filepath.Base(file), ".view.json")
		}
		if v := after.views[view]; v != nil {
			doc = v.doc // the batch's own state of the view wins over the file
		}
		if kind == "entity" {
			for _, p := range viewItems(doc, "placements") {
				if p.str("entity") == id {
					return refuse("cannot withdraw entity %s: it still stands on %s; remove its placement in the same batch", id, view)
				}
			}
			continue
		}
		for _, e := range viewItems(doc, "edges") {
			if e.str("id") == id {
				return refuse("cannot withdraw relation %s: %s still has an entry for it in edges; remove it in the same batch", id, view)
			}
		}
		var rel struct {
			Except []string `json:"except"`
		}
		if raw, ok := doc.vals["relations"]; ok {
			_ = json.Unmarshal(raw, &rel)
		}
		for _, x := range rel.Except {
			if x == id {
				return refuse("cannot withdraw relation %s: %s still names it in relations.except; remove it in the same batch", id, view)
			}
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
	authored := isAuthored(o)
	switch {
	case old == nil && !hasIDPrefix(id, "e_"):
		return refuse("entity id %q: starts with e_ (CONTRACT §3)", id)
	case authored && has(o, "name") && (changed(old, o, "name") || changed(old, o, "origin")):
		return refuse("entity %s: an authored entity has no name in entities.json — its name is a text under its id (CONTRACT §3, §7; ADR_20260930-5)", id)
	case !authored && (changed(old, o, "name") || changed(old, o, "origin")) && strings.TrimSpace(o.str("name")) == "":
		return refuse("entity %s: name is empty (CONTRACT §3)", id)
	case changed(old, o, "kind") && strings.TrimSpace(o.str("kind")) == "":
		return refuse("entity %s: kind is empty (CONTRACT §3)", id)
	case changed(old, o, "origin") && !oneOf(o, "origin", "code", "authored"):
		return refuse("entity %s: origin %q: code or authored (CONTRACT §3)", id, o.str("origin"))
	case changed(old, o, "status") && !oneOf(o, "status", "present", "missing", "planned"):
		return refuse("entity %s: status %q: present, missing or planned (CONTRACT §3)", id, o.str("status"))
	}
	if s := oldShape("entity", o); s != "" {
		return refuse("entity %s: %s (CONTRACT §3; `semaps migrate`)", id, s)
	}
	if changed(old, o, "code") {
		if err := checkCode("entity", id, entries(o, "code")); err != nil {
			return err
		}
	}
	if authored && (old == nil || changed(old, o, "origin")) && !after.hasNameText(id) {
		return refuse("entity %s: an authored entity needs a name: a `name` text under its id in at least one language (CONTRACT §7; ADR_20260930-5)", id)
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
		// the set of types is open: any word is a type, the dictionary describes the known ones (CONTRACT §5, §6)
		t := relationType(o)
		if strings.TrimSpace(t) == "" || strings.ContainsAny(t, " \t\"") {
			return refuse("relation %s: type %q: a word without spaces (CONTRACT §4)", id, t)
		}
	}
	if changed(old, o, "origin") && !oneOf(o, "origin", "code", "authored") {
		return refuse("relation %s: origin %q: code or authored (CONTRACT §4)", id, o.str("origin"))
	}
	if s := oldShape("relation", o); s != "" {
		return refuse("relation %s: %s (CONTRACT §4; `semaps migrate`)", id, s)
	}
	if changed(old, o, "evidence") {
		return checkCode("relation", id, entries(o, "evidence"))
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
		if strings.HasPrefix(key, "e_") && (field == "name" || field == "title") {
			e := findByID(after.registries["entity"].items, key)
			switch {
			case field == "title":
				return refuse("%s.title (%s): an entity has a name, not a title (CONTRACT §7.1)", key, lang)
			case e == nil:
				return refuse("%s.name (%s): no entity %s", key, lang, key)
			case !isAuthored(e):
				return refuse("%s.name (%s): the name of an entity that comes from code is its code name in entities.json and is not translated (CONTRACT §7.1)", key, lang)
			}
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

// placementsOf: the placements of a view doc by entity.
func placementsOf(doc *object) map[string]*object {
	out := map[string]*object{}
	if doc == nil {
		return out
	}
	for _, p := range viewItems(doc, "placements") {
		out[p.str("entity")] = p
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

// checkPlacement: a placement places an entity of entities.json, names its
// parent (null for none) and only a container placement of the same view as
// the parent, without a loop; its override has only the fields of the table
// (CONTRACT §8.2, §11.6). A removed container must not leave placements
// pointing at it.
func (after *Model) checkPlacement(before *Model, view, id string) error {
	all := placementsOf(after.views[view].doc)
	o := all[id]
	if o == nil {
		for _, p := range viewItems(after.views[view].doc, "placements") {
			if p.str("parent") == id {
				return refuse("%s on %s still lies in %s: move it out or remove it in the same batch", p.str("entity"), view, id)
			}
		}
		return nil
	}
	if s := oldPlacementShape(o); s != "" {
		return refuse("placement %s on %s: %s — форма контракта 3, нужен %d (CONTRACT §8.2)", id, view, s, ContractVersion)
	}
	if _, ok := o.vals["parent"]; !ok {
		return refuse("placement %s on %s: parent is required; null for none (CONTRACT §8.2)", id, view)
	}
	old := placementsOf(before.viewDocNoCache(view))[id]
	entities := after.registries["entity"].items
	if old == nil && findByID(entities, id) == nil {
		return refuse("no entity %s: a placement places an entity of entities.json (CONTRACT §8.2)", id)
	}
	if changed(old, o, "parent") {
		for p, hops := o.str("parent"), 0; p != ""; p, hops = all[p].str("parent"), hops+1 {
			if all[p] == nil {
				return refuse("no container %s on %s", p, view)
			}
			if e := findByID(entities, p); e == nil || !after.kinds.IsContainer(e.str("kind")) {
				return refuse("%s is not a container: its kind is not a container kind of kinds.json (CONTRACT §8.2)", p)
			}
			if p == id || hops > len(all) {
				return refuse("%s cannot go into itself or its own content", id)
			}
		}
	}
	if changed(old, o, "override") {
		if err := CheckOverride(o.vals["override"]); err != nil {
			return refuse("placement %s on %s: %v", id, view, err)
		}
	}
	for _, field := range []string{"styleId", "template"} {
		if raw, ok := o.vals[field]; ok && changed(old, o, field) {
			var s string
			if json.Unmarshal(raw, &s) != nil || strings.TrimSpace(s) == "" {
				return refuse("placement %s on %s: %s is a non-empty string; to drop it, take the field out (CONTRACT §8.2)", id, view, field)
			}
		}
	}
	if raw, ok := o.vals["collapsed"]; ok && changed(old, o, "collapsed") {
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return refuse("placement %s on %s: collapsed is true or false (CONTRACT §8.2)", id, view)
		}
		if e := findByID(entities, id); e == nil || !after.kinds.IsContainer(e.str("kind")) {
			return refuse("placement %s on %s: collapsed belongs to a container placement; %s is not a container (CONTRACT §8.2)", id, view, id)
		}
	}
	return nil
}

// checkRouting: a `routing` is one of the modes of CONTRACT §11.3.
func checkRouting(raw json.RawMessage, on string) error {
	var s string
	if json.Unmarshal(raw, &s) != nil || !slices.Contains(RoutingModes, s) {
		return refuse("%s: routing %s: one of %s (CONTRACT §11.3)", on, string(raw), strings.Join(RoutingModes, ", "))
	}
	return nil
}

// checkViewEdges: an entry of the view's `edges` refers to a relation of the
// registry and holds only what is its own — styleId, override, routing
// (CONTRACT §8.5). A new or changed entry has no from/to/type, names a relation
// that exists after the batch, has at least one own field and an id that no other
// entry has; its `override` has only the fields of the edge table (§11.6).
// Entries that the batch did not change are not looked at.
func (after *Model) checkViewEdges(before *Model, view string) error {
	if after.views[view] == nil {
		return nil
	}
	// the view's own routing, when the batch changed it
	if raw, ok := after.views[view].doc.vals["routing"]; ok {
		var was json.RawMessage
		if doc := before.viewDocNoCache(view); doc != nil {
			was = doc.vals["routing"]
		}
		if !bytes.Equal(was, raw) {
			if err := checkRouting(raw, "view "+view); err != nil {
				return err
			}
		}
	}
	old := map[string]string{}
	if doc := before.viewDocNoCache(view); doc != nil {
		for _, e := range viewItems(doc, "edges") {
			b, _ := e.MarshalJSON()
			old[e.str("id")] = string(b)
		}
	}
	entries := viewItems(after.views[view].doc, "edges")
	count := map[string]int{}
	for _, e := range entries {
		count[e.str("id")]++
	}
	relations := after.registries["relation"]
	for _, e := range entries {
		id := e.str("id")
		b, _ := e.MarshalJSON()
		if prev, ok := old[id]; ok && prev == string(b) {
			continue
		}
		if id == "" {
			return refuse("an entry of edges on %s has no id: it names a relation of the registry (CONTRACT §8.5)", view)
		}
		for _, key := range []string{"from", "to", "type"} {
			if _, ok := e.vals[key]; ok {
				return refuse("edge %s on %s: `%s` is the relation's, not the view's: an entry of edges is {id, styleId?, override?, routing?} (CONTRACT §8.5)", id, view, key)
			}
		}
		if relations == nil || findByID(relations.items, id) == nil {
			return refuse("edge %s on %s: no relation %s in the registry; a line is a relation, add it with add_relation (CONTRACT §8.5)", id, view, id)
		}
		if count[id] > 1 {
			return refuse("edge %s is listed twice on %s", id, view)
		}
		override, hasOverride := e.vals["override"]
		hasOverride = hasOverride && string(override) != "null"
		if e.str("styleId") == "" && e.str("routing") == "" && !hasOverride {
			return refuse("edge %s on %s has none of styleId, override, routing: an entry with nothing of its own is not written (CONTRACT §8.5)", id, view)
		}
		if raw, ok := e.vals["routing"]; ok {
			if err := checkRouting(raw, "edge "+id+" on "+view); err != nil {
				return err
			}
		}
		if err := CheckEdgeOverride(override); err != nil {
			return refuse("edge %s on %s: %v", id, view, err)
		}
	}
	return nil
}

// hasIDPrefix: the id starts with prefix and has something after it.
func hasIDPrefix(id, prefix string) bool {
	return strings.HasPrefix(id, prefix) && len(id) > len(prefix)
}
