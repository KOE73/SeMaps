package migrate

// Relation types live in the dictionary (ADR_20260930-6): the project file
// relation-types.json and the `rt_<id>` texts are gone. This step removes them
// and carries over what the dictionary does not already say — the default
// visibility of a type, its name and description — into the WORKSPACE kinds.json,
// where the entry of a type replaces the dictionary's (CONTRACT §6):
//
//   - a type of relation-types.json (and a type only an `rt_` text names) that the
//     dictionary — the tool's, supplemented by the workspace kinds.json — has, with
//     the same visibility and the same texts, needs nothing;
//   - one that has a different visibility or different texts gets its entry in the
//     workspace kinds.json: the workspace's own entry is changed in place, else a
//     copy of the dictionary's entry (or a new one for a type the dictionary does
//     not have) goes into the group of project relation types;
//   - the default visibility is one per workspace, not per project: when projects
//     disagree, the first project wins and the report says so.
//
// Everything is decided by the content, so a second run finds nothing to do. What
// is written to the workspace kinds.json is listed in the report, never silent.

import (
	"fmt"
	"sort"
	"strings"
)

// relTypeGroupID is the group of the workspace kinds.json that receives the
// entries this step writes.
const (
	relTypeGroupID = "relations.project"
	kindsFileName  = "kinds.json"
)

// rtEntry is one type of a project's relation-types.json: what the project said of
// it, for the report and for the decision.
type rtEntry struct {
	id  string
	vis string // "visible", "hidden" or "" (none: the view decided)
}

// rtClaim is what one project says of one relation type: its entry in
// relation-types.json (has) with the visibility it recorded, and the `name` and
// `description` texts under `rt_<id>` by language.
type rtClaim struct {
	project string
	lang0   string // the project's main language: the name of a type nobody named
	has     bool
	vis     string
	texts   map[string]map[string]string // language → field → value
}

// relTypeLib is the dictionary as this migration sees it — the tool's default and
// the workspace's kinds.json — and the claims of every project on it.
type relTypeLib struct {
	ws     *file // <workspace>/kinds.json; nil when the workspace has none, made on the first entry
	wsRoot string
	// base and wsTypes are the relation types of the default and of the
	// workspace dictionary by id; a workspace entry shadows a default one.
	base    map[string]*obj
	wsTypes map[string]*obj

	claims map[string][]*rtClaim
	order  []string // type ids in the order they were first claimed
}

// typesOf lists the relation type entries of a dictionary document, by id.
func typesOf(root *obj, rel string) (map[string]*obj, error) {
	out := map[string]*obj{}
	gv, ok := root.lookup("relationGroups")
	if !ok {
		return out, nil
	}
	groups, ok := asArr(gv)
	if !ok {
		return nil, ferr(rel, "relationGroups", "ожидался массив")
	}
	for gi, g := range groups {
		grp, ok := asObj(g)
		if !ok {
			return nil, ferr(rel, fmt.Sprintf("relationGroups[%d]", gi), "ожидался объект")
		}
		tv, ok := grp.lookup("types")
		if !ok {
			continue
		}
		types, ok := asArr(tv)
		if !ok {
			return nil, ferr(rel, fmt.Sprintf("relationGroups[%d].types", gi), "ожидался массив")
		}
		for ti, t := range types {
			to, ok := asObj(t)
			if !ok {
				return nil, ferr(rel, fmt.Sprintf("relationGroups[%d].types[%d]", gi, ti), "ожидался объект")
			}
			if id, ok := asStr(to.get("id")); ok && id != "" {
				out[id] = to
			}
		}
	}
	return out, nil
}

// newRelTypeLib reads the workspace kinds.json and parses the tool's default
// dictionary (empty: the dictionary has no relation type).
func newRelTypeLib(ws string, defaultKinds []byte) (*relTypeLib, error) {
	l := &relTypeLib{claims: map[string][]*rtClaim{}, base: map[string]*obj{}, wsTypes: map[string]*obj{}}
	if len(defaultKinds) > 0 {
		t, err := parseTree(defaultKinds)
		if err != nil {
			return nil, fmt.Errorf("словарь по умолчанию: неверный JSON: %w", err)
		}
		root, ok := t.(*obj)
		if !ok {
			return nil, fmt.Errorf("словарь по умолчанию: ожидался JSON-объект верхнего уровня")
		}
		if l.base, err = typesOf(root, "словарь по умолчанию"); err != nil {
			return nil, err
		}
	}
	f, err := loadFile(ws, kindsFileName, false)
	if err != nil {
		return nil, err
	}
	if f != nil {
		l.ws = f
		if l.wsTypes, err = typesOf(f.root, f.rel); err != nil {
			return nil, err
		}
	}
	l.wsRoot = ws
	return l, nil
}

func (l *relTypeLib) claim(id string, c *rtClaim) {
	if _, ok := l.claims[id]; !ok {
		l.order = append(l.order, id)
	}
	l.claims[id] = append(l.claims[id], c)
}

// effective is the dictionary entry of a type as the workspace sees it: the
// workspace's own, else the default's; inWS tells which.
func (l *relTypeLib) effective(id string) (o *obj, inWS bool) {
	if o, ok := l.wsTypes[id]; ok {
		return o, true
	}
	return l.base[id], false
}

// textOf is a text value: a value with provenance ({"v": …}) or a bare string.
func textOf(v any) string {
	if s, ok := asStr(v); ok {
		return s
	}
	return valueText(v)
}

// entryText is the text of an entry of the dictionary: name and description are
// objects by language.
func entryText(o *obj, field, lang string) string {
	if o == nil {
		return ""
	}
	m, ok := asObj(o.get(field))
	if !ok {
		return ""
	}
	s, _ := asStr(m.get(lang))
	return s
}

// projectGroup is the group of the workspace kinds.json for the relation types of
// the projects; it is made when there is none.
func (l *relTypeLib) projectGroup() (*obj, error) {
	if l.ws == nil {
		l.ws = newFile(l.wsRoot, kindsFileName)
	}
	var groups []any
	if gv, ok := l.ws.root.lookup("relationGroups"); ok {
		var isArr bool
		if groups, isArr = asArr(gv); !isArr {
			return nil, ferr(l.ws.rel, "relationGroups", "ожидался массив")
		}
	}
	for _, g := range groups {
		if o, ok := asObj(g); ok {
			if id, _ := asStr(o.get("id")); id == relTypeGroupID {
				return o, nil
			}
		}
	}
	g := newObj()
	g.set("id", jsonStr(relTypeGroupID))
	name := newObj()
	name.set("ru", jsonStr("Типы связей проектов"))
	name.set("en", jsonStr("Relation types of the projects"))
	g.set("name", name)
	desc := newObj()
	desc.set("ru", jsonStr("Типы связей, которые проекты объявляли у себя до того, как типы связей перешли в словарь."))
	desc.set("en", jsonStr("Relation types the projects declared themselves before relation types moved to the dictionary."))
	g.set("description", desc)
	g.set("types", []any{})
	l.ws.root.set("relationGroups", append(groups, g))
	l.ws.dirty = true
	return g, nil
}

func (l *relTypeLib) addToProjectGroup(t *obj) error {
	g, err := l.projectGroup()
	if err != nil {
		return err
	}
	types, _ := asArr(g.get("types"))
	g.set("types", append(types, t))
	l.ws.dirty = true
	return nil
}

var rtFields = []string{"name", "description"}

// resolve decides every claimed type and writes the workspace entries it needs
// (in memory); the report lists each of them.
func (l *relTypeLib) resolve(rep *Report) error {
	for _, id := range l.order {
		claims := l.claims[id]
		eff, inWS := l.effective(id)
		effVis := ""
		if eff != nil {
			effVis, _, _ = strField(eff, "visibility")
		}

		// visibility: one per workspace; the first project that recorded it wins
		wantVis, decided, first := effVis, false, ""
		for _, c := range claims {
			if !c.has {
				continue
			}
			switch {
			case !decided:
				wantVis, decided, first = c.vis, true, c.project
			case c.vis != wantVis:
				rep.WorkspaceNotes = append(rep.WorkspaceNotes, fmt.Sprintf(
					"тип связи «%s»: проект %s записывал visibility %s, проект %s — %s; в рабочем пространстве она одна, оставлена %s (проекта %s)",
					id, first, visLabel(wantVis), c.project, visLabel(c.vis), visLabel(wantVis), first))
			}
		}
		visChanged := decided && wantVis != effVis

		// texts: per language and field; the first project that has one wins
		type change struct{ field, lang, from, to string }
		var changes []change
		seen := map[string]string{}
		for _, c := range claims {
			var langs []string
			for lang := range c.texts {
				langs = append(langs, lang)
			}
			sort.Strings(langs)
			for _, lang := range langs {
				for _, field := range rtFields {
					v := c.texts[lang][field]
					if v == "" {
						continue
					}
					key := lang + "\x00" + field
					if prev, ok := seen[key]; ok {
						if prev != v {
							rep.WorkspaceNotes = append(rep.WorkspaceNotes, fmt.Sprintf(
								"тип связи «%s»: %s.%s проекта %s — «%s», у первого проекта «%s»; оставлено первое", id, field, lang, c.project, v, prev))
						}
						continue
					}
					seen[key] = v
					if cur := entryText(eff, field, lang); cur != v {
						changes = append(changes, change{field, lang, cur, v})
					}
				}
			}
		}
		if eff != nil && !visChanged && len(changes) == 0 {
			continue
		}

		// the entry to write: the workspace's own, else a copy of the default's, else a new one
		var target *obj
		var what []string
		if inWS {
			target = eff
		} else {
			if eff != nil {
				target = deepCopy(eff).(*obj)
				what = append(what, "запись словаря по умолчанию заменена целиком")
			} else {
				target = newObj()
				target.set("id", jsonStr(id))
				target.set("name", newObj())
				what = append(what, "типа нет в словаре")
			}
			if err := l.addToProjectGroup(target); err != nil {
				return err
			}
			l.wsTypes[id] = target
		}
		if visChanged {
			switch {
			case wantVis == "":
				target.del("visibility")
			case target.has("visibility"):
				target.set("visibility", jsonStr(wantVis))
			default:
				target.insertBefore("name", "visibility", jsonStr(wantVis))
			}
			what = append(what, fmt.Sprintf("visibility %s → %s", visLabel(effVis), visLabel(wantVis)))
		}
		for _, ch := range changes {
			m, ok := asObj(target.get(ch.field))
			if !ok {
				m = newObj()
				target.set(ch.field, m)
			}
			m.set(ch.lang, jsonStr(ch.to))
			if ch.from == "" {
				what = append(what, fmt.Sprintf("%s.%s «%s»", ch.field, ch.lang, ch.to))
			} else {
				what = append(what, fmt.Sprintf("%s.%s «%s» → «%s»", ch.field, ch.lang, ch.from, ch.to))
			}
		}
		if nm, _ := asObj(target.get("name")); nm == nil || len(nm.keys) == 0 {
			// the dictionary wants a name: nobody named the type, so its id is
			lang := claims[0].lang0
			if nm == nil {
				nm = newObj()
				target.set("name", nm)
			}
			nm.set(lang, jsonStr(id))
			what = append(what, fmt.Sprintf("name.%s «%s» (имени не было, взят id)", lang, id))
		}
		l.ws.dirty = true
		rep.WorkspaceNotes = append(rep.WorkspaceNotes, fmt.Sprintf(
			"тип связи «%s» → %s, группа %s: %s", id, kindsFileName, l.groupOf(id), strings.Join(what, "; ")))
	}
	return nil
}

func visLabel(v string) string {
	if v == "" {
		return "не задана"
	}
	return v
}

// groupOf is the id of the workspace group that holds the entry of a type.
func (l *relTypeLib) groupOf(id string) string {
	if l.ws == nil {
		return relTypeGroupID
	}
	gv, _ := l.ws.root.lookup("relationGroups")
	groups, _ := asArr(gv)
	for _, g := range groups {
		grp, ok := asObj(g)
		if !ok {
			continue
		}
		types, _ := asArr(grp.get("types"))
		for _, t := range types {
			if to, ok := asObj(t); ok {
				if tid, _ := asStr(to.get("id")); tid == id {
					gid, _ := asStr(grp.get("id"))
					return gid
				}
			}
		}
	}
	return relTypeGroupID
}

// output is the workspace kinds.json as it has to be written; changed is false
// when nothing was carried over.
func (l *relTypeLib) output() (fileOut, bool, error) {
	if l.ws == nil || (!l.ws.dirty && !l.ws.created) {
		return fileOut{}, false, nil
	}
	return l.ws.output()
}

// relationTypes is the step of one project: its relation-types.json and its `rt_`
// texts are claimed on the dictionary and removed.
func (m *projectMigration) relationTypes() error {
	claims := map[string]*rtClaim{}
	var order []string
	get := func(id string) *rtClaim {
		c := claims[id]
		if c == nil {
			c = &rtClaim{project: m.id, lang0: m.langs[0], texts: map[string]map[string]string{}}
			claims[id] = c
			order = append(order, id)
		}
		return c
	}
	for _, e := range m.rtEntries {
		c := get(e.id)
		c.has, c.vis = true, e.vis
	}
	for _, td := range m.texts {
		if td.entries == nil {
			continue
		}
		for _, key := range append([]string(nil), td.entries.keys...) {
			if !strings.HasPrefix(key, "rt_") || len(key) == len("rt_") {
				continue
			}
			id := strings.TrimPrefix(key, "rt_")
			en, ok := asObj(td.entries.get(key))
			if !ok {
				return ferr(td.f.rel, "entries."+key, "ожидался объект")
			}
			c := get(id)
			c.texts[td.lang] = map[string]string{}
			for _, field := range en.keys {
				switch field {
				case "name", "description":
					c.texts[td.lang][field] = textOf(en.get(field))
				default:
					m.rep.Notes = append(m.rep.Notes, fmt.Sprintf("text.%s.json: %s.%s не переносится: у типа связи в словаре только name и description", td.lang, key, field))
				}
			}
			td.entries.del(key)
			td.changed, td.f.dirty = true, true
			m.rep.RelationTypeTexts++
		}
	}
	for _, id := range order {
		m.lib.claim(id, claims[id])
	}
	if m.rtypes != nil {
		m.rtypes.remove = true
	}
	return nil
}
