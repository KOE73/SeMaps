package core

import (
	"encoding/json"
	"slices"
	"strings"
)

type AppliedStyleModifier struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage"`
	Rule  string `json:"rule,omitempty"`
}
type StyleAppearance struct {
	ID          string                 `json:"id"`
	Family      string                 `json:"family"`
	BaseStyleID string                 `json:"baseStyleId"`
	Applied     []AppliedStyleModifier `json:"applied"`
	Skipped     []AppliedStyleModifier `json:"skipped"`
	Patch       json.RawMessage        `json:"patch"`
	Resolved    json.RawMessage        `json:"resolved"`
	Origins     map[string]string      `json:"origins"`
	Facts       map[string][]string    `json:"facts"`
}

func (w *WorkspaceModel) PreviewStyleRule(project, view string, raw json.RawMessage) ([]StyleAppearance, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	rule := newObject()
	if json.Unmarshal(raw, rule) != nil {
		return nil, refuse("rule must be an object")
	}
	if err := checkStyleRule(rule, w.styles); err != nil {
		return nil, err
	}
	m, err := w.project(project)
	if err != nil {
		return nil, err
	}
	appearances, err := w.viewAppearances(m, view, rule)
	if err != nil {
		return nil, err
	}
	out := []StyleAppearance{}
	for _, a := range appearances {
		if slices.ContainsFunc(a.Applied, func(e AppliedStyleModifier) bool { return e.Rule == rule.str("id") }) || slices.ContainsFunc(a.Skipped, func(e AppliedStyleModifier) bool { return e.Rule == rule.str("id") }) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b StyleAppearance) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func appearanceFacts(m *Model, project, view string) map[string]selectorContext {
	entities := map[string]*object{}
	parents := map[string][]string{}
	for _, e := range m.registries["entity"].items {
		entities[e.str("id")] = e
	}
	for _, r := range m.registries["relation"].items {
		if strings.SplitN(r.str("type"), ".", 2)[0] == "contains" {
			parents[r.str("to")] = appendUnique(parents[r.str("to")], r.str("from"))
		}
	}
	contexts := map[string]selectorContext{}
	for id, e := range entities {
		values := map[string][]string{"project": {project}, "view": {view}, "id": {id}, "kind": {e.str("kind")}}
		for _, field := range []string{"name", "namespace", "origin", "status"} {
			if value := e.str(field); value != "" {
				values[field] = []string{value}
			}
		}
		for _, code := range viewItems(e, "code") {
			for field, key := range map[string]string{"library": "library", "language": "lang", "code.ref": "ref"} {
				if value := code.str(key); value != "" {
					values[field] = appendUnique(values[field], value)
				}
			}
		}
		visited := map[string]bool{}
		var visit func(string)
		visit = func(at string) {
			if visited[at] {
				return
			}
			visited[at] = true
			if parent := entities[at]; parent != nil && parent.str("kind") == "assembly" {
				values["assembly"] = appendUnique(values["assembly"], at)
			}
			for _, p := range parents[at] {
				visit(p)
			}
		}
		visit(id)
		contexts[id] = selectorContext{values}
	}
	return contexts
}

func ruleInScope(rule *object, project, view string) bool {
	if raw, ok := rule.vals["enabled"]; ok && string(raw) == "false" {
		return false
	}
	scope, _ := child(rule, "scope")
	for key, value := range map[string]string{"projects": project, "views": view} {
		list := stringList(scope, key)
		if len(list) > 0 && !slices.Contains(list, value) {
			return false
		}
	}
	return true
}

func (w *WorkspaceModel) viewAppearances(m *Model, vid string, preview *object) (map[string]StyleAppearance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.loadView(vid)
	if err != nil {
		return nil, err
	}
	contexts := appearanceFacts(m, m.project, vid)
	rules := viewItems(w.styles, StyleRulesCollection)
	if preview != nil {
		rules = slices.DeleteFunc(rules, func(r *object) bool { return r.str("id") == preview.str("id") })
		rules = append(rules, preview)
	}
	order := func(r *object) int { var n int; _ = json.Unmarshal(r.vals["order"], &n); return n }
	slices.SortStableFunc(rules, func(a, b *object) int {
		if order(a) < order(b) {
			return -1
		}
		if order(a) > order(b) {
			return 1
		}
		return strings.Compare(a.str("id"), b.str("id"))
	})
	selectors := map[string]StyleSelector{}
	for _, rule := range rules {
		selector, err := parseSelector(rule.vals["selector"])
		if err != nil {
			return nil, refuse("rule %s: %v", rule.str("id"), err)
		}
		selectors[rule.str("id")] = selector
	}
	modifiers := map[string]*object{}
	for _, modifier := range viewItems(w.styles, ModifiersCollection) {
		modifiers[modifier.str("id")] = modifier
	}
	catalog, err := w.catalog()
	if err != nil {
		return nil, err
	}
	baseID := func(kind, family string, placement *object) string {
		if id := placement.str("styleId"); id != "" {
			return id
		}
		id := kind
		if family == "edge" {
			k, _ := catalog.LookupRelation(kind)
			if k.Style != "" {
				id = k.Style
			}
		} else {
			k, _ := catalog.Lookup(kind)
			if k.Style != "" {
				id = k.Style
			}
		}
		if findByID(styleItems(w.styles), id) != nil {
			return id
		}
		switch family {
		case "edge":
			return "default.edge"
		case "container":
			return "default.container"
		default:
			return "default.node"
		}
	}
	build := func(id, kind, family string, p *object, context selectorContext) (StyleAppearance, error) {
		if context.values == nil {
			context.values = map[string][]string{"project": {m.project}, "view": {vid}, "id": {id}, "kind": {kind}}
		}
		context.values["family"] = []string{family}
		base := baseID(kind, family, p)
		flat := styleFloor(family)
		seen := map[string]bool{}
		chain := []*object{}
		for at := base; at != ""; {
			if seen[at] {
				return StyleAppearance{}, refuse("style inheritance cycle at %s", at)
			}
			seen[at] = true
			s := findByID(styleItems(w.styles), at)
			if s == nil {
				break
			}
			chain = append(chain, s)
			at = s.str("basedOn")
		}
		for i := len(chain) - 1; i >= 0; i-- {
			flat = mergeStylePatch(flat, chain[i])
		}
		a := StyleAppearance{ID: id, Family: family, BaseStyleID: base, Applied: []AppliedStyleModifier{}, Skipped: []AppliedStyleModifier{}, Origins: map[string]string{}, Facts: context.values}
		markOrigins(a.Origins, flat, "", "style:"+base)
		patch := newObject()
		apply := func(mid, stage, rid string) error {
			modifier := modifiers[mid]
			if modifier == nil {
				return refuse("%s: unknown modifier %s", id, mid)
			}
			patches, _ := child(modifier, "patches")
			own, _ := child(patches, family)
			if own == nil {
				if stage == "manual" {
					return refuse("modifier %s has no %s patch", mid, family)
				}
				return nil
			}
			entry := AppliedStyleModifier{mid, modifier.str("name"), stage, rid}
			if stage == "automatic" && (string(p.vals["automaticModifiers"]) == "false" || slices.Contains(stringList(p, "disabledModifiers"), mid) || slices.Contains(stringList(v.doc, "disabledStyleRules"), rid)) {
				a.Skipped = append(a.Skipped, entry)
				return nil
			}
			patch = mergeStylePatch(patch, own)
			flat = mergeStylePatch(flat, own)
			origin := stage + ":" + mid
			if rid != "" {
				origin = "rule:" + rid + "/" + mid
			}
			markOrigins(a.Origins, own, "", origin)
			a.Applied = append(a.Applied, entry)
			return nil
		}
		for _, rule := range rules {
			if !ruleInScope(rule, m.project, vid) || selectorMatch(selectors[rule.str("id")], context) != selectorTrue {
				continue
			}
			for _, mid := range stringList(rule, "modifierIds") {
				if err := apply(mid, "automatic", rule.str("id")); err != nil {
					return a, err
				}
			}
		}
		{
			for _, mid := range stringList(p, "modifiers") {
				if err := apply(mid, "manual", ""); err != nil {
					return a, err
				}
			}
		}
		if own, _ := child(p, "override"); own != nil {
			flat = mergeStylePatch(flat, own)
			markOrigins(a.Origins, own, "", "override")
		}
		a.Patch = encodeJSON(patch)
		a.Resolved = encodeJSON(flat)
		return a, nil
	}
	out := map[string]StyleAppearance{}
	placed := map[string]bool{}
	for _, p := range viewItems(v.doc, "placements") {
		id := p.str("entity")
		placed[id] = true
		e := findByID(m.registries["entity"].items, id)
		if e == nil {
			continue
		}
		family := "block"
		if m.kinds.IsContainer(e.str("kind")) {
			family = "container"
		}
		a, err := build(id, e.str("kind"), family, p, contexts[id])
		if err != nil {
			return nil, err
		}
		out[id] = a
	}
	shown, _ := m.edgesByRuleRecords(v.doc, placed, m.registries["relation"].items)
	for _, edge := range shown {
		r := findByID(m.registries["relation"].items, edge.ID)
		if r == nil {
			continue
		}
		p := findByID(viewItems(v.doc, "edges"), edge.ID)
		if p == nil {
			p = newObject()
		}
		values := map[string][]string{"project": {m.project}, "view": {vid}, "id": {edge.ID}, "kind": {r.str("type")}}
		for _, key := range []string{"name", "origin", "status"} {
			if value := r.str(key); value != "" {
				values[key] = []string{value}
			}
		}
		for prefix, endpoint := range map[string]string{"source.": r.str("from"), "target.": r.str("to")} {
			for key, value := range contexts[endpoint].values {
				if key != "project" && key != "view" {
					values[prefix+key] = value
				}
			}
		}
		a, err := build(edge.ID, r.str("type"), "edge", p, selectorContext{values})
		if err != nil {
			return nil, err
		}
		out[edge.ID] = a
	}
	return out, nil
}

func markOrigins(out map[string]string, o *object, prefix, source string) {
	for _, key := range o.keys {
		if key == "id" || key == "name" || key == "forKinds" || key == "basedOn" || key == "appliesTo" || key == "description" || key == "tags" {
			continue
		}
		nested, _ := child(o, key)
		if nested != nil {
			markOrigins(out, nested, prefix+key+".", source)
		} else {
			out[prefix+key] = source
		}
	}
}
