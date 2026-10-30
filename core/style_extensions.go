package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const ModifiersCollection = "modifiers"
const StyleRulesCollection = "styleRules"

type StyleExtensionUse struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Project string `json:"project,omitempty"`
	View    string `json:"view,omitempty"`
	Family  string `json:"family,omitempty"`
	Field   string `json:"field"`
}
type StyleExtensionDetail struct {
	Definition json.RawMessage     `json:"definition"`
	Usage      []StyleExtensionUse `json:"usage"`
	Dirty      []Ref               `json:"dirty"`
	Revision   uint64              `json:"revision"`
}

func validStyleCollection(collection string) bool {
	return collection == ModifiersCollection || collection == StyleRulesCollection
}

func (w *WorkspaceModel) ListStyleExtensions(collection string) ([]json.RawMessage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !validStyleCollection(collection) {
		return nil, refuse("unknown style collection")
	}
	out := []json.RawMessage{}
	for _, item := range viewItems(w.styles, collection) {
		out = append(out, encodeJSON(item))
	}
	return out, nil
}
func (w *WorkspaceModel) GetStyleExtension(collection, id string) (StyleExtensionDetail, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.allProjects(); err != nil {
		return StyleExtensionDetail{}, err
	}
	return w.extensionDetail(collection, id)
}
func (w *WorkspaceModel) extensionDetail(collection, id string) (StyleExtensionDetail, error) {
	item := findByID(viewItems(w.styles, collection), id)
	if item == nil {
		return StyleExtensionDetail{}, refuse("unknown %s id %q", collection, id)
	}
	uses, err := w.extensionUses(collection, id)
	dirty := []Ref{}
	for _, ref := range w.dirty {
		if ref.Kind == collection && ref.ID == id {
			dirty = append(dirty, ref)
		}
	}
	return StyleExtensionDetail{encodeJSON(item), uses, dirty, w.revision}, err
}

func (w *WorkspaceModel) extensionUses(collection, id string) ([]StyleExtensionUse, error) {
	out := []StyleExtensionUse{}
	if collection == ModifiersCollection {
		for _, rule := range viewItems(w.styles, StyleRulesCollection) {
			if slices.Contains(stringList(rule, "modifierIds"), id) {
				out = append(out, StyleExtensionUse{Kind: "rule", ID: rule.str("id"), Field: "modifierIds"})
			}
		}
	}
	for pid, m := range w.models {
		m.mu.Lock()
		sources := []*Model{m}
		saved, err := LoadModelWithoutJournal(w.root, pid, w.defaultKinds)
		if err == nil {
			sources = append(sources, saved)
		}
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		for si, source := range sources {
			for _, vid := range source.allViewIDs() {
				v, err := source.loadView(vid)
				if err != nil {
					m.mu.Unlock()
					return nil, err
				}
				if collection == StyleRulesCollection {
					if slices.Contains(stringList(v.doc, "disabledStyleRules"), id) {
						out = append(out, StyleExtensionUse{Kind: "view", ID: vid, Project: pid, View: vid, Field: "disabledStyleRules"})
					}
					continue
				}
				for _, key := range []string{"placements", "edges"} {
					for _, p := range viewItems(v.doc, key) {
						for _, field := range []string{"modifiers", "disabledModifiers"} {
							if !slices.Contains(stringList(p, field), id) {
								continue
							}
							family := "edge"
							recordID := p.str("id")
							if key == "placements" {
								recordID = p.str("entity")
								e := findByID(source.registries["entity"].items, recordID)
								family = "block"
								if e != nil && source.kinds.IsContainer(e.str("kind")) {
									family = "container"
								}
							}
							kind := key
							if si == 1 {
								kind = "saved-" + kind
							}
							out = append(out, StyleExtensionUse{kind, recordID, pid, vid, family, field})
						}
					}
				}
			}
		}
		m.mu.Unlock()
	}
	return out, nil
}

func stringList(o *object, key string) []string {
	var list []string
	if o != nil {
		_ = json.Unmarshal(o.vals[key], &list)
	}
	return list
}
func checkStringList(o *object, key string) error {
	if !has(o, key) {
		return nil
	}
	var list []string
	if json.Unmarshal(o.vals[key], &list) != nil || list == nil {
		return refuse("%s must be a string array", key)
	}
	seen := map[string]bool{}
	for _, id := range list {
		if id == "" || seen[id] {
			return refuse("%s needs distinct non-empty identifiers", key)
		}
		seen[id] = true
	}
	return nil
}

func checkExtensionMetadata(item *object) error {
	id := item.str("id")
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, " \t\r\n/\\#,") {
		return refuse("extension id must be non-empty and contain no whitespace, path separators, # or comma")
	}
	if strings.TrimSpace(item.str("name")) == "" {
		return refuse("extension name must be a non-empty string")
	}
	if has(item, "description") {
		var description string
		if json.Unmarshal(item.vals["description"], &description) != nil || string(item.vals["description"]) == "null" {
			return refuse("description must be a string")
		}
	}
	return nil
}

func checkStyleExtensions(sheet *object) error {
	for _, collection := range []string{ModifiersCollection, StyleRulesCollection} {
		if !has(sheet, collection) {
			continue
		}
		var entries []json.RawMessage
		if json.Unmarshal(sheet.vals[collection], &entries) != nil || entries == nil {
			return refuse("%s must be an array", collection)
		}
		seen := map[string]bool{}
		for _, raw := range entries {
			item := newObject()
			if json.Unmarshal(raw, item) != nil || string(raw) == "null" {
				return refuse("%s entries must be objects", collection)
			}
			if seen[item.str("id")] {
				return refuse("duplicate %s id %s", collection, item.str("id"))
			}
			seen[item.str("id")] = true
			var err error
			if collection == ModifiersCollection {
				err = checkStyleModifier(item)
			} else {
				err = checkStyleRule(item, sheet)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func checkStyleModifier(item *object) error {
	for _, key := range item.keys {
		if !slices.Contains([]string{"id", "name", "description", "patches"}, key) {
			return refuse("unknown modifier field %s", key)
		}
	}
	if err := checkExtensionMetadata(item); err != nil {
		return err
	}
	patches, err := child(item, "patches")
	if err != nil || patches == nil || len(patches.keys) == 0 {
		return refuse("modifier requires patches for block, container or edge")
	}
	for _, family := range patches.keys {
		if !slices.Contains([]string{"block", "container", "edge"}, family) {
			return refuse("unknown modifier family %s", family)
		}
		patch, err := child(patches, family)
		if err != nil || patch == nil {
			return refuse("modifier patch %s must be an object", family)
		}
		for _, field := range patch.keys {
			allowed := []string{"fill", "border", "radius", "shadow", "header", "title", "subtitle", "icon"}
			if family == "edge" {
				allowed = []string{"line", "label", "source", "target"}
			}
			if !slices.Contains(allowed, field) {
				return refuse("modifier cannot change %s for %s", field, family)
			}
			group, _ := child(patch, field)
			if field == "header" && has(group, "height") {
				return refuse("modifier cannot change header height")
			}
			if (field == "source" || field == "target") && has(group, "shape") {
				return refuse("modifier cannot change endpoint shape")
			}
		}
		if err := checkStyleFields(patch, family); err != nil {
			return err
		}
	}
	return nil
}
func checkStyleRule(item, sheet *object) error {
	for _, key := range item.keys {
		if !slices.Contains([]string{"id", "name", "description", "enabled", "order", "scope", "selector", "modifierIds", "selectorVersion"}, key) {
			return refuse("unknown style rule field %s", key)
		}
	}
	if err := checkExtensionMetadata(item); err != nil {
		return err
	}
	var version int
	if json.Unmarshal(item.vals["selectorVersion"], &version) != nil || version != 1 {
		return refuse("selectorVersion must be 1")
	}
	if _, err := parseSelector(item.vals["selector"]); err != nil {
		return err
	}
	if !has(item, "modifierIds") {
		return refuse("rule requires modifierIds")
	}
	if err := checkStringList(item, "modifierIds"); err != nil {
		return err
	}
	if len(stringList(item, "modifierIds")) == 0 {
		return refuse("rule requires at least one modifier")
	}
	for _, id := range stringList(item, "modifierIds") {
		if findByID(viewItems(sheet, ModifiersCollection), id) == nil {
			return refuse("rule %s: unknown modifier %s", item.str("id"), id)
		}
	}
	if raw, ok := item.vals["enabled"]; ok {
		var v bool
		if json.Unmarshal(raw, &v) != nil || string(raw) == "null" {
			return refuse("enabled must be boolean")
		}
	}
	if raw, ok := item.vals["order"]; ok {
		var v int
		if json.Unmarshal(raw, &v) != nil || string(raw) == "null" {
			return refuse("order must be an integer")
		}
	}
	if has(item, "scope") {
		scope, err := child(item, "scope")
		if err != nil || scope == nil {
			return refuse("scope must be an object")
		}
		for _, key := range scope.keys {
			if key != "projects" && key != "views" {
				return refuse("unknown rule scope %s", key)
			}
			if err := checkStringList(scope, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *WorkspaceModel) EditStyleExtension(collection string, edit StyleEdit, author string) (StyleExtensionDetail, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !validStyleCollection(collection) {
		return StyleExtensionDetail{}, refuse("unknown style collection")
	}
	if edit.Expected != nil && *edit.Expected != w.revision {
		return StyleExtensionDetail{}, refuse("style revision conflict: expected %d, current %d", *edit.Expected, w.revision)
	}
	if edit.ID == "" || author == "" {
		return StyleExtensionDetail{}, refuse("id and author required")
	}
	if err := w.allProjects(); err != nil {
		return StyleExtensionDetail{}, err
	}
	sheet := cloneObject(w.styles)
	items := viewItems(sheet, collection)
	current := findByID(items, edit.ID)
	id := edit.ID
	next := copyJournals(w.journals)
	copies := map[string]*Model{}
	dirty := append([]Ref{}, w.dirty...)
	switch edit.Action {
	case "create":
		if current != nil {
			return StyleExtensionDetail{}, refuse("%s %s already exists", collection, id)
		}
		current = mapObject(edit.Definition)
		current.set("id", id)
		items = append(items, current)
	case "update":
		if current == nil {
			return StyleExtensionDetail{}, refuse("unknown %s %s", collection, id)
		}
		if _, ok := edit.Patch["id"]; ok {
			return StyleExtensionDetail{}, refuse("use rename to change id")
		}
		current = mergeStylePatch(current, mapObject(edit.Patch))
		for i, item := range items {
			if item.str("id") == id {
				items[i] = current
			}
		}
	case "rename", "delete":
		if current == nil {
			return StyleExtensionDetail{}, refuse("unknown %s %s", collection, id)
		}
		uses, err := w.extensionUses(collection, id)
		if err != nil {
			return StyleExtensionDetail{}, err
		}
		replacement := edit.NewID
		if edit.Action == "delete" {
			replacement = edit.Replacement
			if len(uses) > 0 && replacement == "" {
				return StyleExtensionDetail{}, refuse("%s %s is used; provide replacement explicitly. references: %s", collection, id, encodeJSON(uses))
			}
		}
		if replacement == id {
			return StyleExtensionDetail{}, refuse("replacement must differ")
		}
		if edit.Action == "rename" {
			if replacement == "" || findByID(items, replacement) != nil {
				return StyleExtensionDetail{}, refuse("new id empty or already exists")
			}
			current.set("id", replacement)
			id = replacement
		} else {
			if replacement != "" && findByID(items, replacement) == nil {
				return StyleExtensionDetail{}, refuse("unknown replacement %s", replacement)
			}
			items = slices.DeleteFunc(items, func(i *object) bool { return i.str("id") == edit.ID })
		}
		if replacement != "" {
			if collection == ModifiersCollection {
				rules := viewItems(sheet, StyleRulesCollection)
				for _, rule := range rules {
					if replaceReferenceList(rule, "modifierIds", edit.ID, replacement) {
						dirty = upsertRef(dirty, Ref{Kind: StyleRulesCollection, ID: rule.str("id"), Author: author})
					}
				}
				if has(sheet, StyleRulesCollection) {
					sheet.set(StyleRulesCollection, rules)
				}
			}
			field := "modifier"
			if collection == StyleRulesCollection {
				field = "rule"
			}
			for pid, m := range w.models {
				m.mu.Lock()
				c := m.copy()
				values := []styleReferenceValue{}
				refs, err := c.extensionReference(edit.ID, replacement, field, &values)
				if err == nil && (len(refs) > 0 || slices.ContainsFunc(uses, func(u StyleExtensionUse) bool { return u.Project == pid })) {
					op := Op{Kind: "style_ref", ID: edit.ID, Value: encodeJSON(styleReferencePatch{NewID: replacement, References: refs, Field: field, Replacement: edit.Action == "delete", Values: values}), Author: author}
					c.journal = append(c.journal, []Op{op})
					for _, ref := range refs {
						ref.Author = author
						c.dirty.Views[ref.View] = upsertRef(c.dirty.Views[ref.View], ref)
					}
					copies[pid] = c
					next[pid] = c.journal
				}
				m.mu.Unlock()
				if err != nil {
					return StyleExtensionDetail{}, err
				}
			}
		}
	default:
		return StyleExtensionDetail{}, refuse("unknown extension action")
	}
	sheet.set(collection, items)
	if err := checkStyleExtensions(sheet); err != nil {
		return StyleExtensionDetail{}, err
	}
	for pid, m := range w.models {
		candidate := m
		if c := copies[pid]; c != nil {
			candidate = c
		}
		candidate.mu.Lock()
		err := w.checkAppearanceReferences(candidate, sheet)
		candidate.mu.Unlock()
		if err != nil {
			return StyleExtensionDetail{}, err
		}
		// A pending assignment can hide a reference that SaveStyles must leave valid.
		saved, err := LoadModelWithoutJournal(w.root, pid, w.defaultKinds)
		if err != nil {
			return StyleExtensionDetail{}, err
		}
		for _, batch := range next[pid] {
			for _, op := range batch {
				if op.Kind == "style_ref" {
					if _, err := saved.applyStyleReference(op); err != nil {
						return StyleExtensionDetail{}, err
					}
				}
			}
		}
		if err := w.checkAppearanceReferences(saved, sheet); err != nil {
			return StyleExtensionDetail{}, err
		}
	}
	dirty = upsertRef(dirty, Ref{Kind: collection, ID: edit.ID, Author: author, Removed: edit.Action == "delete" || edit.Action == "rename"})
	if edit.Action == "rename" {
		dirty = upsertRef(dirty, Ref{Kind: collection, ID: id, Author: author})
	}
	if err := w.persist(sheet, w.kinds, dirty, next); err != nil {
		return StyleExtensionDetail{}, err
	}
	w.styles, w.dirty, w.journals = sheet, dirty, next
	w.revision++
	for pid, c := range copies {
		m := w.models[pid]
		m.mu.Lock()
		m.adopt(c)
		m.mu.Unlock()
	}
	if edit.Action == "delete" {
		return StyleExtensionDetail{Dirty: dirty, Revision: w.revision}, nil
	}
	return w.extensionDetail(collection, id)
}

func replaceReferenceList(o *object, field, old, next string) bool {
	list := stringList(o, field)
	changed := false
	out := []string{}
	for _, id := range list {
		if id == old {
			id = next
			changed = true
		}
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if changed {
		if len(out) == 0 {
			o.del(field)
		} else {
			o.set(field, out)
		}
	}
	return changed
}
func (m *Model) extensionReference(old, next, field string, captures ...*[]styleReferenceValue) ([]Ref, error) {
	refs := []Ref{}
	replace := func(o *object, key, view, kind, id string) bool {
		before := stringList(o, key)
		changed := replaceReferenceList(o, key, old, next)
		if changed && len(captures) > 0 {
			*captures[0] = append(*captures[0], styleReferenceValue{view, kind, id, key, before, stringList(o, key)})
		}
		return changed
	}
	for _, vid := range m.allViewIDs() {
		v, err := m.loadView(vid)
		if err != nil {
			return nil, err
		}
		if field == "rule" {
			if replace(v.doc, "disabledStyleRules", vid, "view", vid) {
				refs = append(refs, Ref{Kind: "view", ID: vid, View: vid})
			}
			continue
		}
		for _, key := range []string{"placements", "edges"} {
			items := viewItems(v.doc, key)
			for _, p := range items {
				kind, id := "placement", p.str("entity")
				if key == "edges" {
					kind, id = "edge", p.str("id")
				}
				changed := replace(p, "modifiers", vid, kind, id)
				changed = replace(p, "disabledModifiers", vid, kind, id) || changed
				if changed {
					kind, id := "placement", p.str("entity")
					if key == "edges" {
						kind, id = "view", p.str("id")
					}
					refs = append(refs, Ref{Kind: kind, ID: id, View: vid})
				}
			}
			if has(v.doc, key) {
				v.doc.set(key, items)
			}
		}
	}
	return refs, nil
}
func (w *WorkspaceModel) checkAppearanceReferences(m *Model, sheet *object) error {
	for _, vid := range m.allViewIDs() {
		v, err := m.loadView(vid)
		if err != nil {
			return err
		}
		if err := checkStringList(v.doc, "disabledStyleRules"); err != nil {
			return err
		}
		for _, id := range stringList(v.doc, "disabledStyleRules") {
			if findByID(viewItems(sheet, StyleRulesCollection), id) == nil {
				return refuse("%s/%s: unknown style rule %s", m.project, vid, id)
			}
		}
		for _, key := range []string{"placements", "edges"} {
			for _, p := range viewItems(v.doc, key) {
				family := "edge"
				id := p.str("id")
				if key == "placements" {
					family = "block"
					id = p.str("entity")
					e := findByID(m.registries["entity"].items, id)
					if e != nil && m.kinds.IsContainer(e.str("kind")) {
						family = "container"
					}
				}
				for _, field := range []string{"modifiers", "disabledModifiers"} {
					if err := checkStringList(p, field); err != nil {
						return err
					}
					for _, mid := range stringList(p, field) {
						modifier := findByID(viewItems(sheet, ModifiersCollection), mid)
						if modifier == nil {
							return refuse("%s/%s#%s: unknown modifier %s", m.project, vid, id, mid)
						}
						patches, _ := child(modifier, "patches")
						if field == "modifiers" && !has(patches, family) {
							return refuse("modifier %s has no %s patch for %s", mid, family, id)
						}
					}
				}
				if raw, ok := p.vals["automaticModifiers"]; ok {
					var value bool
					if json.Unmarshal(raw, &value) != nil || string(raw) == "null" {
						return refuse("automaticModifiers must be boolean")
					}
				}
			}
		}
	}
	return nil
}

// CheckStyleExtensions validates saved contract files only, without replaying journals.
func CheckStyleExtensions(root string, kinds []byte) error {
	sheet, err := loadDoc(filepath.Join(root, "styles.json"))
	if err != nil {
		return err
	}
	if sheet == nil {
		sheet = newObject()
	}
	if err := checkStyleExtensions(sheet); err != nil {
		return err
	}
	dirs, err := os.ReadDir(filepath.Join(root, "projects"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	w := &WorkspaceModel{}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		m, err := LoadModelWithoutJournal(root, dir.Name(), kinds)
		if err != nil {
			continue
		} // General model validation already reports malformed projects.
		if err := w.checkAppearanceReferences(m, sheet); err != nil {
			return err
		}
	}
	return nil
}
