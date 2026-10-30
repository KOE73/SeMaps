package core

import (
	"encoding/json"
	"slices"
	"strings"
)

type StyleFilter struct {
	AppliesTo string   `json:"appliesTo,omitempty"`
	ForKinds  []string `json:"forKinds,omitempty"`
	Name      string   `json:"name,omitempty"`
	ID        string   `json:"id,omitempty"`
}
type StyleUse struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Project string `json:"project,omitempty"`
	View    string `json:"view,omitempty"`
	Target  string `json:"appliesTo,omitempty"`
	ForKind string `json:"forKind,omitempty"`
}
type StyleDetail struct {
	Definition json.RawMessage `json:"definition"`
	Resolved   json.RawMessage `json:"resolved"`
	Dirty      []Ref           `json:"dirty"`
	Usage      []StyleUse      `json:"usage"`
	Revision   uint64          `json:"revision"`
}
type StyleEdit struct {
	Action      string         `json:"action"`
	ID          string         `json:"id"`
	Definition  map[string]any `json:"definition,omitempty"`
	Patch       map[string]any `json:"patch,omitempty"`
	NewID       string         `json:"newId,omitempty"`
	Replacement string         `json:"replacement,omitempty"`
	Expected    *uint64        `json:"expectedRevision,omitempty"`
}

func styleItems(sheet *object) []*object { return viewItems(sheet, "styles") }
func targetOfStyle(s *object) string {
	if s == nil {
		return ""
	}
	if t := s.str("appliesTo"); t != "" {
		return t
	}
	return "block"
}
func styleKinds(s *object) []string {
	var kinds []string
	_ = json.Unmarshal(s.vals["forKinds"], &kinds)
	return kinds
}
func fallbackStyle(id string) bool {
	return id == "default.node" || id == "default.container" || id == "default.edge"
}
func mapObject(v any) *object { o := newObject(); _ = json.Unmarshal(encodeJSON(v), o); return o }

// Recursive merge patch: absent keys survive; null removes an own value,
// including a nested one. Arrays replace the previous array.
func mergeStylePatch(dst *object, patch *object) *object {
	out := cloneObject(dst)
	for _, key := range patch.keys {
		raw := patch.vals[key]
		if string(raw) == "null" {
			out.del(key)
			continue
		}
		if len(raw) > 0 && raw[0] == '{' {
			p := newObject()
			_ = json.Unmarshal(raw, p)
			old, _ := child(out, key)
			if old == nil {
				old = newObject()
			}
			merged := mergeStylePatch(old, p)
			if len(merged.keys) == 0 {
				out.del(key)
			} else {
				out.set(key, merged)
			}
		} else {
			out.set(key, raw)
		}
	}
	return out
}

func (w *WorkspaceModel) ListStyles(f StyleFilter) ([]json.RawMessage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f.AppliesTo != "" && !slices.Contains([]string{"block", "container", "edge"}, f.AppliesTo) {
		return nil, refuse("appliesTo must be block, container or edge")
	}
	out := []json.RawMessage{}
	for _, s := range styleItems(w.styles) {
		if f.ID != "" && s.str("id") != f.ID {
			continue
		}
		if f.AppliesTo != "" && targetOfStyle(s) != f.AppliesTo {
			continue
		}
		if f.Name != "" && !strings.Contains(strings.ToLower(s.str("name")), strings.ToLower(f.Name)) {
			continue
		}
		if len(f.ForKinds) > 0 {
			match := false
			for _, k := range f.ForKinds {
				match = match || slices.Contains(styleKinds(s), k)
			}
			if !match {
				continue
			}
		}
		out = append(out, encodeJSON(s))
	}
	return out, nil
}
func (w *WorkspaceModel) GetStyle(id string) (StyleDetail, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.allProjects(); err != nil {
		return StyleDetail{}, err
	}
	return w.detail(id)
}
func (w *WorkspaceModel) detail(id string) (StyleDetail, error) {
	s := findByID(styleItems(w.styles), id)
	if s == nil {
		return StyleDetail{}, refuse("unknown styleId %q", id)
	}
	if err := checkStyleChain(s, w.styles); err != nil {
		return StyleDetail{}, err
	}
	flat := newObject()
	chain := []*object{}
	seen := map[string]bool{}
	for at := s; at != nil; at = findByID(styleItems(w.styles), at.str("basedOn")) {
		if seen[at.str("id")] {
			return StyleDetail{}, refuse("style inheritance cycle at %s", at.str("id"))
		}
		seen[at.str("id")] = true
		chain = append(chain, at)
		if at.str("basedOn") == "" {
			break
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		flat = mergeStylePatch(flat, chain[i])
	}
	flat.del("forKinds")
	if raw, ok := s.vals["forKinds"]; ok {
		flat.set("forKinds", raw)
	}
	resolved := mergeStylePatch(styleFloor(targetOfStyle(s)), flat)
	dirty := []Ref{}
	for _, ref := range w.dirty {
		if seen[ref.ID] {
			dirty = append(dirty, ref)
		}
	}
	uses, err := w.uses(id)
	return StyleDetail{encodeJSON(s), encodeJSON(resolved), dirty, uses, w.revision}, err
}

func (w *WorkspaceModel) catalog() (*KindCatalog, error) {
	if string(encodeJSON(w.kinds)) == string(encodeJSON(w.savedKinds)) {
		return LoadKinds(w.root, w.defaultKinds)
	}
	return ParseKinds(w.defaultKinds, encodeJSON(w.kinds))
}
func (w *WorkspaceModel) uses(id string) ([]StyleUse, error) {
	out := []StyleUse{}
	for _, s := range styleItems(w.styles) {
		if s.str("basedOn") == id {
			out = append(out, StyleUse{Kind: "basedOn", ID: s.str("id")})
		}
	}
	catalog, err := w.catalog()
	if err != nil {
		return nil, err
	}
	base := func(kind, target string) string {
		named := kind
		if target == "edge" {
			k, _ := catalog.LookupRelation(kind)
			if k.Style != "" {
				named = k.Style
			}
		} else {
			k, _ := catalog.Lookup(kind)
			if k.Style != "" {
				named = k.Style
			}
		}
		if s := findByID(styleItems(w.styles), named); s != nil && targetOfStyle(s) == target {
			return named
		}
		if target == "edge" {
			return "default.edge"
		}
		if target == "container" {
			return "default.container"
		}
		return "default.node"
	}
	for _, g := range catalog.Groups {
		for _, k := range g.Kinds {
			target := "block"
			if k.Container {
				target = "container"
			}
			if base(k.ID, target) == id {
				out = append(out, StyleUse{Kind: "kind", ID: k.ID, Target: target, ForKind: k.ID})
			}
		}
	}
	for _, g := range catalog.RelationGroups {
		for _, k := range g.Types {
			if base(k.ID, "edge") == id {
				out = append(out, StyleUse{Kind: "relationType", ID: k.ID, Target: "edge", ForKind: k.ID})
			}
		}
	}
	for pid, m := range w.models {
		m.mu.Lock()
		for _, v := range m.allViewIDs() {
			view, err := m.loadView(v)
			if err != nil {
				m.mu.Unlock()
				return nil, err
			}
			for _, key := range []string{"placements", "edges"} {
				for _, p := range viewItems(view.doc, key) {
					eid, kind, target := p.str("entity"), "placement", "block"
					var entityKind string
					if key == "edges" {
						eid = p.str("id")
						kind = "edge"
						target = "edge"
						if rel := findByID(m.registries["relation"].items, eid); rel != nil {
							entityKind = rel.str("type")
						}
					} else {
						if entity := findByID(m.registries["entity"].items, eid); entity != nil {
							entityKind = entity.str("kind")
							if catalog.IsContainer(entityKind) {
								target = "container"
							}
						}
					}
					styleID := p.str("styleId")
					if styleID == "" {
						styleID = base(entityKind, target)
					}
					if styleID == id {
						out = append(out, StyleUse{kind, eid, pid, v, target, entityKind})
					}
				}
			}
		}
		m.mu.Unlock()
	}
	// A pending project assignment can hide a saved reference. Saving workspace
	// definitions must keep that saved view valid without saving the assignment.
	saved, err := w.savedUses(id)
	if err != nil {
		return nil, err
	}
	for _, use := range saved {
		duplicate := slices.ContainsFunc(out, func(other StyleUse) bool {
			return other.Project == use.Project && other.View == use.View && other.ID == use.ID
		})
		if !duplicate {
			out = append(out, use)
		}
	}
	return out, nil
}

func (w *WorkspaceModel) savedUses(id string) ([]StyleUse, error) {
	uses := []StyleUse{}
	for pid := range w.models {
		m, err := LoadModelWithoutJournal(w.root, pid, w.defaultKinds)
		if err != nil {
			return nil, err
		}
		for _, vid := range m.allViewIDs() {
			view, err := m.loadView(vid)
			if err != nil {
				return nil, err
			}
			for _, key := range []string{"placements", "edges"} {
				for _, item := range viewItems(view.doc, key) {
					if item.str("styleId") != id {
						continue
					}
					use := StyleUse{Kind: "saved-placement", ID: item.str("entity"), Project: pid, View: vid, Target: "block"}
					if key == "edges" {
						use.Kind, use.ID, use.Target = "saved-edge", item.str("id"), "edge"
						if r := findByID(m.registries["relation"].items, use.ID); r != nil {
							use.ForKind = r.str("type")
						}
					} else if e := findByID(m.registries["entity"].items, use.ID); e != nil {
						use.ForKind = e.str("kind")
						if m.kinds.IsContainer(use.ForKind) {
							use.Target = "container"
						}
					}
					uses = append(uses, use)
				}
			}
		}
	}
	return uses, nil
}

func (w *WorkspaceModel) EditStyle(edit StyleEdit, author string) (StyleDetail, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if edit.Expected != nil && *edit.Expected != w.revision {
		return StyleDetail{}, refuse("style conflict: expected revision %d, current %d; reload styles", *edit.Expected, w.revision)
	}
	if author == "" {
		return StyleDetail{}, refuse("style author is empty")
	}
	if err := w.allProjects(); err != nil {
		return StyleDetail{}, err
	}
	sheet := cloneObject(w.styles)
	items := styleItems(sheet)
	current := findByID(items, edit.ID)
	kinds := cloneObject(w.kinds)
	dirty := append([]Ref{}, w.dirty...)
	next := copyJournals(w.journals)
	copies := map[string]*Model{}
	id := edit.ID
	replacement := ""
	switch edit.Action {
	case "create":
		if current != nil {
			return StyleDetail{}, refuse("style %s already exists", id)
		}
		current = mapObject(edit.Definition)
		current.set("id", id)
		items = append(items, current)
	case "update":
		if current == nil {
			return StyleDetail{}, refuse("unknown styleId %q", id)
		}
		if _, ok := edit.Patch["id"]; ok {
			return StyleDetail{}, refuse("use rename_style to change id")
		}
		updated := mergeStylePatch(current, mapObject(edit.Patch))
		for i, s := range items {
			if s.str("id") == id {
				items[i] = updated
				current = updated
			}
		}
	case "rename", "delete":
		if current == nil {
			return StyleDetail{}, refuse("unknown styleId %q", id)
		}
		if fallbackStyle(id) {
			return StyleDetail{}, refuse("fallback %s cannot be renamed or deleted", id)
		}
		uses, err := w.uses(id)
		if err != nil {
			return StyleDetail{}, err
		}
		replacement = edit.NewID
		if edit.Action == "delete" {
			replacement = edit.Replacement
			if len(uses) > 0 && replacement == "" {
				return StyleDetail{}, refuse("style %s is used; provide replacement explicitly. references: %s", id, string(encodeJSON(uses)))
			}
		}
		if edit.Action == "rename" {
			if replacement == id {
				return w.detail(id)
			}
			if findByID(items, replacement) != nil {
				return StyleDetail{}, refuse("style %s already exists", replacement)
			}
			current.set("id", replacement)
			id = replacement
		} else {
			items = slices.DeleteFunc(items, func(s *object) bool { return s.str("id") == id })
		}
		sheet.set("styles", items)
		if replacement != "" {
			dest := findByID(items, replacement)
			if dest == nil {
				return StyleDetail{}, refuse("unknown replacement style %s", replacement)
			}
			if targetOfStyle(dest) != targetOfStyle(current) {
				return StyleDetail{}, refuse("replacement style target differs")
			}
			for _, use := range uses {
				if use.ForKind != "" && !fallbackStyle(replacement) && !slices.Contains(styleKinds(dest), use.ForKind) {
					return StyleDetail{}, refuse("replacement %s does not belong to %s", replacement, use.ForKind)
				}
			}
			for _, s := range items {
				if s.str("basedOn") == edit.ID {
					s.set("basedOn", replacement)
					dirty = upsertRef(dirty, Ref{Kind: "style", ID: s.str("id"), Author: author})
				}
			}
			catalog, _ := w.catalog()
			changedGroups := map[string]bool{}
			changedRelations := map[string]bool{}
			for gi, g := range catalog.Groups {
				for ki, k := range g.Kinds {
					if k.Style == edit.ID || (k.Style == "" && k.ID == edit.ID) {
						catalog.Groups[gi].Kinds[ki].Style = replacement
						changedGroups[g.ID] = true
					}
				}
			}
			for gi, g := range catalog.RelationGroups {
				for ki, k := range g.Types {
					if k.Style == edit.ID || (k.Style == "" && k.ID == edit.ID) {
						catalog.RelationGroups[gi].Types[ki].Style = replacement
						changedRelations[g.ID] = true
					}
				}
			}
			groups := viewItems(kinds, "groups")
			for _, group := range catalog.Groups {
				if changedGroups[group.ID] {
					groups = replaceGroup(groups, mapObject(group))
				}
			}
			if len(changedGroups) > 0 {
				kinds.set("groups", groups)
			}
			relationGroups := viewItems(kinds, "relationGroups")
			for _, group := range catalog.RelationGroups {
				if changedRelations[group.ID] {
					relationGroups = replaceGroup(relationGroups, mapObject(group))
				}
			}
			if len(changedRelations) > 0 {
				kinds.set("relationGroups", relationGroups)
			}
			for pid, m := range w.models {
				m.mu.Lock()
				c := m.copy()
				refs, err := c.styleReference(edit.ID, replacement)
				savedReference := slices.ContainsFunc(uses, func(use StyleUse) bool { return use.Project == pid })
				if err == nil && (len(refs) > 0 || savedReference) {
					op := Op{Kind: "style_ref", ID: edit.ID, Value: encodeJSON(styleReferencePatch{NewID: replacement, References: refs}), Author: author}
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
					return StyleDetail{}, err
				}
			}
		}
	default:
		return StyleDetail{}, refuse("unknown style action %q", edit.Action)
	}
	sheet.set("styles", items)
	if edit.Action != "delete" {
		if err := w.validateStyle(current, sheet, kinds); err != nil {
			return StyleDetail{}, err
		}
	}
	// A target/ownership edit cannot invalidate placements or descendants.
	if edit.Action == "update" {
		uses, err := w.uses(id)
		if err != nil {
			return StyleDetail{}, err
		}
		for _, use := range uses {
			if use.Target != "" && (use.Target != targetOfStyle(current) || (!fallbackStyle(id) && !slices.Contains(styleKinds(current), use.ForKind))) {
				return StyleDetail{}, refuse("style update conflicts with reference %s", string(encodeJSON(use)))
			}
		}
	}
	for _, s := range items {
		if s.str("basedOn") != "" {
			if err := checkStyleChain(s, sheet); err != nil {
				return StyleDetail{}, err
			}
		}
	}
	dirty = upsertRef(dirty, Ref{Kind: "style", ID: edit.ID, Author: author, Removed: edit.Action == "delete" || edit.Action == "rename"})
	if edit.Action == "rename" {
		dirty = upsertRef(dirty, Ref{Kind: "style", ID: id, Author: author})
	}
	if err := w.persist(sheet, kinds, dirty, next); err != nil {
		return StyleDetail{}, err
	}
	w.styles, w.kinds, w.dirty, w.journals = sheet, kinds, dirty, next
	w.revision++
	for pid, c := range copies {
		m := w.models[pid]
		m.mu.Lock()
		m.adopt(c)
		m.mu.Unlock()
	}
	if edit.Action == "delete" {
		return StyleDetail{Dirty: dirty, Revision: w.revision}, nil
	}
	return w.detail(id)
}

func (w *WorkspaceModel) validateStyle(s, sheet, kinds *object) error {
	id := s.str("id")
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, " \t\r\n/\\#") {
		return refuse("style id must be a non-empty word without paths or whitespace")
	}
	if strings.TrimSpace(s.str("name")) == "" {
		return refuse("style %s requires name", id)
	}
	target := targetOfStyle(s)
	if fallbackStyle(id) {
		expected := map[string]string{"default.node": "block", "default.container": "container", "default.edge": "edge"}[id]
		if target != expected {
			return refuse("fallback %s requires appliesTo %s", id, expected)
		}
	}
	if raw, ok := s.vals["appliesTo"]; ok {
		var targetValue string
		if json.Unmarshal(raw, &targetValue) != nil || !slices.Contains([]string{"block", "container", "edge"}, targetValue) {
			return refuse("style %s: invalid appliesTo", id)
		}
	}
	if !slices.Contains([]string{"block", "container", "edge"}, target) {
		return refuse("style %s: appliesTo must be block, container or edge", id)
	}
	catalog, err := ParseKinds(w.defaultKinds, encodeJSON(kinds))
	if err != nil {
		return err
	}
	ks := styleKinds(s)
	if len(ks) == 0 && !fallbackStyle(id) {
		return refuse("style %s requires own forKinds", id)
	}
	for _, k := range ks {
		if target == "edge" {
			if _, ok := catalog.LookupRelation(k); !ok {
				return refuse("style %s: unknown relation type %s", id, k)
			}
		} else {
			kind, ok := catalog.Lookup(k)
			if !ok || kind.Container != (target == "container") {
				return refuse("style %s: kind %s is not of target %s", id, k, target)
			}
		}
	}
	return checkStyleFields(s, target)
}
func checkStyleChain(s, sheet *object) error {
	seen := map[string]bool{}
	target := targetOfStyle(s)
	for s != nil {
		id := s.str("id")
		if seen[id] {
			return refuse("style inheritance cycle at %s", id)
		}
		seen[id] = true
		if targetOfStyle(s) != target {
			return refuse("style inheritance crosses appliesTo at %s", id)
		}
		base := s.str("basedOn")
		if base == "" {
			return nil
		}
		s = findByID(styleItems(sheet), base)
		if s == nil {
			return refuse("unknown basedOn style %s", base)
		}
	}
	return nil
}

func (w *WorkspaceModel) checkStyleRef(sheet *object, id, kind, target, on string) error {
	if id == "" {
		return nil
	}
	s := findByID(styleItems(sheet), id)
	if s == nil {
		return refuse("%s: unknown styleId %q", on, id)
	}
	if targetOfStyle(s) != target || (!fallbackStyle(id) && !slices.Contains(styleKinds(s), kind)) {
		return refuse("%s: style %s does not belong to %s %s", on, id, target, kind)
	}
	return nil
}
func (w *WorkspaceModel) checkAllReferences(m *Model, sheet *object) error {
	if err := w.checkAppearanceReferences(m, sheet); err != nil {
		return err
	}
	for _, id := range m.allViewIDs() {
		v, err := m.loadView(id)
		if err != nil {
			return err
		}
		for _, p := range viewItems(v.doc, "placements") {
			e := findByID(m.registries["entity"].items, p.str("entity"))
			if e == nil {
				continue
			}
			target := "block"
			if m.kinds.IsContainer(e.str("kind")) {
				target = "container"
			}
			if err := w.checkStyleRef(sheet, p.str("styleId"), e.str("kind"), target, m.project+"/"+id+"#"+p.str("entity")); err != nil {
				return err
			}
		}
		for _, p := range viewItems(v.doc, "edges") {
			r := findByID(m.registries["relation"].items, p.str("id"))
			if r != nil {
				if err := w.checkStyleRef(sheet, p.str("styleId"), r.str("type"), "edge", m.project+"/"+id+"#"+p.str("id")); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (w *WorkspaceModel) checkChangedReferences(before, after *Model, ops []Op) error {
	if err := w.checkAppearanceReferences(after, w.styles); err != nil {
		return err
	}
	for _, op := range ops {
		if op.Kind == "placement" {
			v, err := after.loadView(op.View)
			if err != nil {
				return err
			}
			p := findPlacement(v.doc, op.ID)
			if p == nil {
				continue
			}
			e := findByID(after.registries["entity"].items, op.ID)
			if e == nil {
				continue
			}
			target := "block"
			if after.kinds.IsContainer(e.str("kind")) {
				target = "container"
			}
			oldView, _ := before.loadView(op.View)
			old := findPlacement(oldView.doc, op.ID)
			if changed(old, p, "styleId") {
				if err := w.checkStyleRef(w.styles, p.str("styleId"), e.str("kind"), target, op.View+"#"+op.ID); err != nil {
					return err
				}
			}
		}
		if op.Kind == "view" {
			v, err := after.loadView(op.View)
			if err != nil {
				return err
			}
			old, _ := before.loadView(op.View)
			for _, edge := range viewItems(v.doc, "edges") {
				previous := findByID(viewItems(old.doc, "edges"), edge.str("id"))
				if !changed(previous, edge, "styleId") {
					continue
				}
				r := findByID(after.registries["relation"].items, edge.str("id"))
				if r != nil {
					if err := w.checkStyleRef(w.styles, edge.str("styleId"), r.str("type"), "edge", op.View+"#"+edge.str("id")); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func styleFloor(target string) *object {
	// Same terminal defaults as StyleLibrary's fully resolved canvas styles.
	sans := "Inter, system-ui, -apple-system, Segoe UI, sans-serif"
	mono := "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
	text := func(family string, size, weight float64, color, align string) map[string]any {
		return map[string]any{"family": family, "size": size, "weight": weight, "italic": false, "color": color, "align": align, "opacity": 1, "show": true}
	}
	stroke := map[string]any{"color": "#cbd5e1", "width": 1.5, "dash": "none", "opacity": 1}
	if target == "edge" {
		return mapObject(map[string]any{"line": stroke, "source": map[string]any{"shape": "none", "size": 10, "color": nil}, "target": map[string]any{"shape": "arrow", "size": 10, "color": nil}, "label": text(sans, 10, 500, "#475569", "middle"), "family": "flow", "overview": false, "routing": nil})
	}
	return mapObject(map[string]any{"fill": "#ffffff", "border": stroke, "radius": 8, "shadow": true, "header": map[string]any{"fill": "#e2e8f0", "height": 34, "text": text(sans, 13, 700, "#334155", "start")}, "title": text(sans, 12.5, 600, "#1e293b", "start"), "subtitle": text(mono, 10.5, 400, "#64748b", "start"), "icon": map[string]any{"glyph": "file-text", "show": true}, "shape": "rect", "template": nil})
}

func checkStyleFields(s *object, target string) error {
	common := []string{"id", "name", "appliesTo", "description", "tags", "forKinds", "basedOn"}
	block := []string{"fill", "border", "radius", "shadow", "header", "title", "subtitle", "icon", "shape", "template"}
	edge := []string{"line", "routing", "source", "target", "label", "family", "overview"}
	allowed := block
	if target == "edge" {
		allowed = edge
	}
	for _, key := range s.keys {
		if !slices.Contains(common, key) && !slices.Contains(allowed, key) {
			return refuse("style %s: field %s is not supported for %s", s.str("id"), key, target)
		}
	}
	for _, key := range []string{"name", "description", "basedOn", "template"} {
		if raw, ok := s.vals[key]; ok {
			var v string
			if json.Unmarshal(raw, &v) != nil || v == "" {
				return refuse("style %s: %s must be a non-empty string", s.str("id"), key)
			}
		}
	}
	for _, key := range []string{"forKinds", "tags"} {
		if raw, ok := s.vals[key]; ok {
			var v []string
			if json.Unmarshal(raw, &v) != nil || v == nil {
				return refuse("style %s: %s must be a string array", s.str("id"), key)
			}
		}
	}
	for _, key := range []string{"shadow", "overview"} {
		if raw, ok := s.vals[key]; ok {
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return refuse("%s must be boolean", key)
			}
		}
	}
	if raw, ok := s.vals["radius"]; ok {
		var n float64
		if json.Unmarshal(raw, &n) != nil || n < 0 {
			return refuse("radius must be a non-negative number")
		}
	}
	for _, key := range []string{"border", "line", "header", "title", "subtitle", "icon", "source", "target", "label"} {
		if raw, ok := s.vals[key]; ok {
			var v map[string]any
			if json.Unmarshal(raw, &v) != nil || v == nil {
				return refuse("%s must be an object", key)
			}
			if err := validateStyleGroup(key, v); err != nil {
				return err
			}
		}
	}
	if raw, ok := s.vals["fill"]; ok {
		var v any
		_ = json.Unmarshal(raw, &v)
		if err := validatePaint(v); err != nil {
			return err
		}
	}
	for key, options := range map[string][]string{"routing": RoutingModes, "shape": {"rect", "ellipse", "diamond", "hexagon", "actor", "cylinder"}, "family": {"structure", "flow"}} {
		if has(s, key) && !slices.Contains(options, s.str(key)) {
			return refuse("invalid %s %s", key, s.str(key))
		}
	}
	return nil
}
func validateStyleGroup(group string, v map[string]any) error {
	allowed := map[string]string{}
	switch group {
	case "border", "line":
		allowed = map[string]string{"color": "string", "width": "number", "dash": "string", "opacity": "opacity"}
	case "title", "subtitle", "label", "text":
		allowed = map[string]string{"family": "string", "size": "number", "weight": "number", "italic": "bool", "color": "string", "align": "align", "opacity": "opacity", "show": "bool"}
	case "icon":
		allowed = map[string]string{"glyph": "string", "show": "bool"}
	case "header":
		allowed = map[string]string{"fill": "paint", "height": "number", "text": "text"}
	case "source", "target":
		allowed = map[string]string{"shape": "string", "size": "number", "color": "string"}
	}
	for key, value := range v {
		typ, ok := allowed[key]
		if !ok {
			return refuse("unknown style field %s.%s", group, key)
		}
		valid := false
		switch typ {
		case "string":
			s, ok := value.(string)
			valid = ok && s != ""
		case "number", "opacity":
			n, ok := value.(float64)
			valid = ok && n >= 0 && (typ != "opacity" || n <= 1)
		case "bool":
			_, valid = value.(bool)
		case "align":
			s, _ := value.(string)
			valid = slices.Contains([]string{"start", "middle", "end"}, s)
		case "paint":
			valid = validatePaint(value) == nil
		case "text":
			o, ok := value.(map[string]any)
			valid = ok && validateStyleGroup("text", o) == nil
		}
		if !valid {
			return refuse("invalid style field %s.%s", group, key)
		}
	}
	return nil
}
func validatePaint(value any) error {
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return nil
	}
	g, ok := value.(map[string]any)
	if !ok {
		return refuse("fill must be a colour or gradient")
	}
	kind, _ := g["kind"].(string)
	if kind != "linear" && kind != "radial" {
		return refuse("gradient kind must be linear or radial")
	}
	stops, ok := g["stops"].([]any)
	if !ok || len(stops) < 2 {
		return refuse("gradient requires at least two stops")
	}
	for _, v := range stops {
		o, ok := v.(map[string]any)
		if !ok {
			return refuse("invalid gradient stop")
		}
		offset, ok := o["offset"].(float64)
		color, _ := o["color"].(string)
		if !ok || offset < 0 || offset > 1 || color == "" {
			return refuse("invalid gradient stop")
		}
	}
	return nil
}

func findPlacement(doc *object, id string) *object { return placementsOf(doc)[id] }

func replaceGroup(groups []*object, group *object) []*object {
	for i, g := range groups {
		if g.str("id") == group.str("id") {
			groups[i] = group
			return groups
		}
	}
	return append(groups, group)
}
