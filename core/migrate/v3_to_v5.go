package migrate

// The migration of one project (ADR_20260927-3, ADR_20260927-6, ADR_20260930-4,
// ADR_20260930-5): from contract 3, zones and containers become group entities
// (the ones a view shows; a container no view shows gets none), a view keeps one
// `placements` array, containment becomes `contains` relations, colour styles of
// zones become an `override`; then, for a project of contract 3 and for one already
// at 5 alike, the final shape — an entity's realizations in `code[]`, a relation's
// in `evidence[]`, the name of an authored entity a text under its id. Every
// function works on the in-memory JSON trees; nothing is written here.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// now is replaceable in tests.
var nowUTC = func() string { return timeNow().UTC().Format("2006-01-02T15:04:05Z") }

type newEnt struct {
	id       string
	srcID    string // zone or container id the entity comes from
	fallback string // id tail: the name when no text names it
	zoneKeys []string
	baseKeys []string
	// names is the entity's name per language: the `name` value (with its
	// provenance) of the first text key of its zones or container that has one.
	names map[string]any
}

// keys lists the text keys that describe the entity: the keys of the zones that
// show it first, then those of its container.
func (e *newEnt) keys() []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range append(append([]string(nil), e.zoneKeys...), e.baseKeys...) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func (e *newEnt) addZoneKeys(ks ...string) {
	for _, k := range ks {
		found := false
		for _, x := range e.zoneKeys {
			if x == k {
				found = true
			}
		}
		if !found {
			e.zoneKeys = append(e.zoneKeys, k)
		}
	}
}

type zoneInfo struct {
	idx        int
	id         string
	o          *obj
	container  string // container id shown by the zone ("" when none)
	parentZone string // zone id of the parent zone ("" when none)
	ent        *newEnt
}

type nodeInfo struct {
	src string // "nodes" or "placements"
	idx int
	o   *obj
}

type viewData struct {
	f         *file
	hasLayout bool
	zones     []*zoneInfo
	zoneByID  map[string]*zoneInfo
	nodes     []nodeInfo
	placed    map[string]bool // entity ids placed on the view after conversion
}

type textData struct {
	f       *file
	lang    string
	entries *obj
	changed bool
}

type containRel struct{ id, from, to string }

type projectMigration struct {
	ws, id, rel string
	rep         *ProjectReport
	styles      *styleLib
	langs       []string
	opt         Options
	fromV3      bool // the project is below contract 5: the v3 steps run

	// needLang lists what needs a language of the realization and has none to
	// take: the project has not exactly one extractor in the .semaps file.
	needLang []string

	proj, ents, rels, rtypes, conts *file
	views                           []*viewData
	texts                           []*textData
	textByLang                      map[string]*textData

	origEnts map[string]bool // entity ids of the input entities.json
	entIDs   map[string]bool // ... plus the ids minted in this run
	relIDs   map[string]bool
	relPairs map[string]bool // "from|to" of existing contains relations

	newEnts []*newEnt
	zoneEnt map[string]*newEnt // zone id → entity (zones without a container)
	contEnt map[string]*newEnt // container id → entity

	added         []containRel
	newRels       []any
	containsType  *obj   // the `contains` type of relation-types.json, if any
	addContainsRT bool   // relation-types.json needs the type
	containsVis   string // "visible", "hidden" or ""
}

// prepareProject computes every new content of one project in memory. It does
// not collect the outputs: the workspace styles are decided after all projects
// are prepared, and a style dropped there is dropped from the views here first
// (dropStyleRefs), so collect comes last.
func prepareProject(ws, id string, styles *styleLib, opt Options) (*projectMigration, error) {
	rel := "projects/" + id
	pr := &ProjectReport{ID: id}
	proj, err := loadFile(ws, rel+"/project.json", true)
	if err != nil {
		return nil, err
	}
	ver, has, err := versionOf(proj.root)
	if err != nil {
		return nil, ferr(proj.rel, "contractVersion", "%v", err)
	}
	if ver > targetVersion {
		return nil, ferr(proj.rel, "contractVersion", "%d новее поддерживаемого %d", ver, targetVersion)
	}
	pr.FromVersion = ver
	m := &projectMigration{
		ws: ws, id: id, rel: rel, rep: pr, styles: styles, proj: proj, opt: opt,
		fromV3:     !(has && ver >= targetVersion),
		textByLang: map[string]*textData{},
		origEnts:   map[string]bool{}, entIDs: map[string]bool{},
		relIDs: map[string]bool{}, relPairs: map[string]bool{},
		zoneEnt: map[string]*newEnt{}, contEnt: map[string]*newEnt{},
	}
	steps := []func() error{m.loadAll}
	if m.fromV3 {
		steps = append(steps, m.parseViews, m.makeEntities, m.containment,
			m.convertTexts, m.rewriteViews, m.exceptions, m.appendRegistry)
	}
	steps = append(steps, m.finalShape)
	for _, s := range steps {
		if err := s(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// ---------------------------------------------------------------- loading

func (m *projectMigration) loadAll() error {
	m.langs = []string{"ru"}
	if lv, ok := m.proj.root.lookup("languages"); ok {
		arr, ok := asArr(lv)
		if !ok {
			return ferr(m.proj.rel, "languages", "ожидался массив строк")
		}
		m.langs = nil
		for i, x := range arr {
			s, ok := asStr(x)
			if !ok {
				return ferr(m.proj.rel, fmt.Sprintf("languages[%d]", i), "ожидалась строка")
			}
			m.langs = append(m.langs, s)
		}
	}

	var err error
	if m.ents, err = loadFile(m.ws, m.rel+"/entities.json", false); err != nil {
		return err
	}
	if m.ents != nil {
		arr, err := m.ents.array("entities")
		if err != nil {
			return err
		}
		for i, it := range arr {
			o, ok := asObj(it)
			if !ok {
				return ferr(m.ents.rel, fmt.Sprintf("entities[%d]", i), "ожидался объект")
			}
			if id, ok := asStr(o.get("id")); ok {
				m.origEnts[id] = true
				m.entIDs[id] = true
			}
		}
	}
	if m.rels, err = loadFile(m.ws, m.rel+"/relations.json", false); err != nil {
		return err
	}
	if m.rels != nil {
		arr, err := m.rels.array("relations")
		if err != nil {
			return err
		}
		for i, it := range arr {
			o, ok := asObj(it)
			if !ok {
				return ferr(m.rels.rel, fmt.Sprintf("relations[%d]", i), "ожидался объект")
			}
			if id, ok := asStr(o.get("id")); ok {
				m.relIDs[id] = true
			}
			t, _ := asStr(o.get("type"))
			f, _ := asStr(o.get("from"))
			to, _ := asStr(o.get("to"))
			if t == "contains" {
				m.relPairs[f+"|"+to] = true
			}
		}
	}
	if m.rtypes, err = loadFile(m.ws, m.rel+"/relation-types.json", false); err != nil {
		return err
	}
	if m.rtypes != nil {
		arr, err := m.rtypes.array("relationTypes")
		if err != nil {
			return err
		}
		for i, it := range arr {
			o, ok := asObj(it)
			if !ok {
				return ferr(m.rtypes.rel, fmt.Sprintf("relationTypes[%d]", i), "ожидался объект")
			}
			id, _ := asStr(o.get("id"))
			if id == "contains" && m.containsType == nil {
				m.containsType = o
			}
			// A relation type has no style of its own in the project: the base style of
			// a type is in the dictionary and the style names its types in forKinds
			// (ADR_20260930-2). The old link is dropped and named for a human.
			if sid, ok := asStr(o.get("styleId")); ok {
				o.del("styleId")
				m.rtypes.dirty = true
				m.rep.Notes = append(m.rep.Notes, fmt.Sprintf("тип связи %s: styleId %q снят из relation-types.json — назовите %q в forKinds стиля %s (или укажите style у типа в kinds.json)", id, sid, id, sid))
			}
		}
	}
	if m.conts, err = loadFile(m.ws, m.rel+"/containers.json", false); err != nil {
		return err
	}

	dirEntries, err := os.ReadDir(filepath.Join(m.ws, filepath.FromSlash(m.rel), "views"))
	if err == nil {
		var names []string
		for _, e := range dirEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".view.json") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			f, err := loadFile(m.ws, m.rel+"/views/"+n, true)
			if err != nil {
				return err
			}
			m.views = append(m.views, &viewData{f: f, placed: map[string]bool{}})
		}
	}
	dirEntries, err = os.ReadDir(filepath.Join(m.ws, filepath.FromSlash(m.rel)))
	if err != nil {
		return ferr(m.rel, "", "не читается: %v", err)
	}
	var tnames []string
	for _, e := range dirEntries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "text.") && strings.HasSuffix(n, ".json") && len(n) > len("text..json") {
			tnames = append(tnames, n)
		}
	}
	sort.Strings(tnames)
	for _, n := range tnames {
		f, err := loadFile(m.ws, m.rel+"/"+n, true)
		if err != nil {
			return err
		}
		td := &textData{f: f, lang: strings.TrimSuffix(strings.TrimPrefix(n, "text."), ".json")}
		if ev, ok := f.root.lookup("entries"); ok {
			o, ok := asObj(ev)
			if !ok {
				return ferr(f.rel, "entries", "ожидался объект")
			}
			td.entries = o
		}
		m.texts = append(m.texts, td)
		m.textByLang[td.lang] = td
	}
	return nil
}

// parseViews reads zones and nodes of every view in the old shape.
func (m *projectMigration) parseViews() error {
	for _, v := range m.views {
		root := v.f.root
		zv, hasZ := root.lookup("zones")
		nv, hasN := root.lookup("nodes")
		pv, hasP := root.lookup("placements")
		v.hasLayout = hasZ || hasN || hasP
		if !v.hasLayout {
			continue
		}
		v.zoneByID = map[string]*zoneInfo{}
		if hasZ {
			arr, ok := asArr(zv)
			if !ok {
				return ferr(v.f.rel, "zones", "ожидался массив")
			}
			for i, it := range arr {
				fld := fmt.Sprintf("zones[%d]", i)
				o, ok := asObj(it)
				if !ok {
					return ferr(v.f.rel, fld, "ожидался объект")
				}
				id, ok := asStr(o.get("id"))
				if !ok || id == "" {
					return ferr(v.f.rel, fld+".id", "нужна непустая строка")
				}
				if _, dup := v.zoneByID[id]; dup {
					return ferr(v.f.rel, fld+".id", "зона %q уже есть на этом виде", id)
				}
				z := &zoneInfo{idx: i, id: id, o: o}
				v.zones = append(v.zones, z)
				v.zoneByID[id] = z
			}
		}
		for _, src := range []struct {
			key string
			val any
			has bool
		}{{"nodes", nv, hasN}, {"placements", pv, hasP}} {
			if !src.has {
				continue
			}
			arr, ok := asArr(src.val)
			if !ok {
				return ferr(v.f.rel, src.key, "ожидался массив")
			}
			for i, it := range arr {
				o, ok := asObj(it)
				if !ok {
					return ferr(v.f.rel, fmt.Sprintf("%s[%d]", src.key, i), "ожидался объект")
				}
				v.nodes = append(v.nodes, nodeInfo{src: src.key, idx: i, o: o})
			}
		}
		// Nesting of zones: the `parent` field when it names a zone of the view,
		// else a `container` value that names a zone (very old files). Geometry
		// is never consulted.
		for _, z := range v.zones {
			fld := fmt.Sprintf("zones[%d]", z.idx)
			parent, _, err := strField(z.o, "parent")
			if err != nil {
				return ferr(v.f.rel, fld+".parent", "%v", err)
			}
			cont, _, err := strField(z.o, "container")
			if err != nil {
				return ferr(v.f.rel, fld+".container", "%v", err)
			}
			if parent != "" {
				if _, ok := v.zoneByID[parent]; ok && parent != z.id {
					z.parentZone = parent
				} else {
					m.rep.UnknownZones = append(m.rep.UnknownZones,
						fmt.Sprintf("%s: %s.parent «%s» — такой зоны нет на виде, вложенность потеряна", v.f.rel, fld, parent))
				}
			}
			containerIsZone := false
			if cont != "" {
				if _, ok := v.zoneByID[cont]; ok && cont != z.id {
					containerIsZone = true
					if z.parentZone == "" {
						z.parentZone = cont
					}
				}
			}
			if cont != "" && !containerIsZone {
				z.container = cont
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- entities

func tailOf(id string) string {
	if strings.HasPrefix(id, "z_") || strings.HasPrefix(id, "c_") {
		return id[2:]
	}
	return id
}

func slugify(s string) string {
	var sb strings.Builder
	sep := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			sep = false
		} else if !sep {
			sb.WriteByte('_')
			sep = true
		}
	}
	out := strings.Trim(sb.String(), "_")
	if out == "" {
		return "group"
	}
	return out
}

func (m *projectMigration) mint(srcID string) *newEnt {
	slug := slugify(tailOf(srcID))
	id := "e_" + slug
	for n := 2; m.entIDs[id]; n++ {
		id = fmt.Sprintf("e_%s_%d", slug, n)
	}
	m.entIDs[id] = true
	e := &newEnt{id: id, srcID: srcID, fallback: tailOf(srcID), names: map[string]any{}}
	m.newEnts = append(m.newEnts, e)
	return e
}

// makeEntities creates the group entities of the zones of the views, in file
// order. A container of containers.json that no zone shows gets no entity: it is
// only listed in the report, with its `match` rules (containment).
func (m *projectMigration) makeEntities() error {
	for _, v := range m.views {
		for _, z := range v.zones {
			m.rep.Zones++
			keys := []string{z.id}
			if z.container != "" {
				keys = append(keys, z.container)
			}
			if strings.HasPrefix(z.id, "z_") {
				keys = append(keys, "c_"+z.id[2:])
			}
			var e *newEnt
			if z.container != "" {
				e = m.contEnt[z.container]
				if e == nil {
					e = m.mint(z.container)
					e.baseKeys = []string{z.container}
					m.contEnt[z.container] = e
					m.rep.ContainerEntities++
				}
			} else {
				e = m.zoneEnt[z.id]
				if e == nil {
					e = m.mint(z.id)
					m.zoneEnt[z.id] = e
					m.rep.ZoneEntities++
				}
			}
			e.addZoneKeys(keys...)
			z.ent = e
		}
	}
	return nil
}

// ------------------------------------------------------------- containment

func compactJSON(v any) string {
	var b bytes.Buffer
	if err := writeValue(&b, v); err != nil {
		return "?"
	}
	return b.String()
}

func (m *projectMigration) addContains(from, to *newEnt) {
	if m.relPairs[from.id+"|"+to.id] {
		return
	}
	m.relPairs[from.id+"|"+to.id] = true
	base := "r_" + strings.TrimPrefix(from.id, "e_") + "_" + strings.TrimPrefix(to.id, "e_") + "_contains"
	id := base
	for n := 2; m.relIDs[id]; n++ {
		id = fmt.Sprintf("%s_%d", base, n)
	}
	m.relIDs[id] = true
	r := newObj()
	r.set("id", jsonStr(id))
	r.set("from", jsonStr(from.id))
	r.set("to", jsonStr(to.id))
	r.set("type", jsonStr("contains"))
	r.set("origin", jsonStr("authored"))
	m.newRels = append(m.newRels, r)
	m.added = append(m.added, containRel{id: id, from: from.id, to: to.id})
}

func (m *projectMigration) containment() error {
	if m.conts != nil {
		arr, err := m.conts.array("containers")
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for i, it := range arr {
			fld := fmt.Sprintf("containers[%d]", i)
			co, ok := asObj(it)
			if !ok {
				return ferr(m.conts.rel, fld, "ожидался объект")
			}
			cid, ok := asStr(co.get("id"))
			if !ok || cid == "" {
				return ferr(m.conts.rel, fld+".id", "нужна непустая строка")
			}
			if seen[cid] {
				m.rep.Notes = append(m.rep.Notes, fmt.Sprintf("containers.json: контейнер %s повторён, взят первый", cid))
				continue
			}
			seen[cid] = true
			parent, _, err := strField(co, "parent")
			if err != nil {
				return ferr(m.conts.rel, fld+".parent", "%v", err)
			}
			// A container gets an entity only where a zone of a view shows it
			// (makeEntities); the rest of containers.json is only listed here.
			child := m.contEnt[cid]
			target := "без сущности"
			if child != nil {
				target = "→ " + child.id
			}
			var extra []string
			if mv, ok := co.lookup("match"); ok && !isNull(mv) {
				extra = append(extra, "match "+compactJSON(mv))
			}
			if av, ok := co.lookup("axis"); ok && !isNull(av) {
				extra = append(extra, "axis "+compactJSON(av))
			}
			if tv, ok := co.lookup("theme"); ok && !isNull(tv) {
				extra = append(extra, "theme "+compactJSON(tv))
			}
			if len(extra) > 0 {
				m.rep.MatchRules = append(m.rep.MatchRules, fmt.Sprintf("%s %s: %s", cid, target, strings.Join(extra, "; ")))
			}
			if child == nil {
				line := cid
				if parent != "" {
					line += " (parent " + parent + ")"
				}
				m.rep.UnplacedContainers = append(m.rep.UnplacedContainers, line)
			}
			if parent == "" {
				continue
			}
			pe := m.contEnt[parent]
			switch {
			case parent == cid:
				m.rep.Notes = append(m.rep.Notes,
					fmt.Sprintf("containers.json: %s.parent — сам контейнер, связь contains не создана", cid))
			case child == nil:
				// the container itself has no entity: nothing to contain
			case pe == nil:
				m.rep.Notes = append(m.rep.Notes,
					fmt.Sprintf("containers.json: %s.parent «%s» — такого контейнера нет или он не размещён ни на одном виде, связь contains не создана", cid, parent))
			default:
				m.addContains(pe, child)
			}
		}
		if ov, ok := m.conts.root.lookup("overrides"); ok && !isNull(ov) {
			oo, ok := asObj(ov)
			if !ok {
				return ferr(m.conts.rel, "overrides", "ожидался объект")
			}
			for _, eid := range oo.keys {
				cid, ok := asStr(oo.m[eid])
				if !ok {
					return ferr(m.conts.rel, "overrides."+eid, "ожидалась строка — id контейнера")
				}
				pe := m.contEnt[cid]
				switch {
				case !m.origEnts[eid]:
					m.rep.UnappliedOverrides = append(m.rep.UnappliedOverrides,
						fmt.Sprintf("%s → %s: сущности %s нет в entities.json", eid, cid, eid))
				case pe == nil:
					m.rep.UnappliedOverrides = append(m.rep.UnappliedOverrides,
						fmt.Sprintf("%s → %s: контейнер %s не размещён ни на одном виде, сущности у него нет", eid, cid, cid))
				default:
					m.addContains(pe, &newEnt{id: eid})
				}
			}
		}
	}
	m.rep.ContainsAdded = len(m.added)
	if len(m.added) > 0 && m.containsType == nil {
		m.addContainsRT = true
		m.containsVis = "hidden"
	}
	if m.containsType != nil {
		if s, ok, _ := strField(m.containsType, "visibility"); ok && (s == "visible" || s == "hidden") {
			m.containsVis = s
		}
	}
	return nil
}

// ------------------------------------------------------------------- texts

func entryObj(td *textData, key string) *obj {
	if td.entries == nil {
		return nil
	}
	o, _ := asObj(td.entries.get(key))
	return o
}

// nameOf reads name.v of an entry (a plain string is a v2-style name).
func nameOf(td *textData, key string) string {
	en := entryObj(td, key)
	if en == nil {
		return ""
	}
	nv := en.get("name")
	if s, ok := asStr(nv); ok {
		return s
	}
	if no, ok := asObj(nv); ok {
		s, _ := asStr(no.get("v"))
		return s
	}
	return ""
}

func sameValue(a, b any) bool {
	val := func(x any) string {
		if o, ok := asObj(x); ok {
			if s, ok := asStr(o.get("v")); ok {
				return "v:" + s
			}
		}
		return "j:" + compactJSON(x)
	}
	return val(a) == val(b)
}

// provenance is a text value written by the migration.
func provenance(v string) *obj {
	o := newObj()
	o.set("v", jsonStr(v))
	o.set("at", jsonStr(nowUTC()))
	o.set("origin", jsonStr("authored"))
	return o
}

// nameValue is the `name` value of an entry with its provenance, as it is (a
// plain string of contract 2 becomes an authored value); nil when it has none.
func nameValue(td *textData, key string) any {
	en := entryObj(td, key)
	if en == nil {
		return nil
	}
	nv, ok := en.lookup("name")
	if !ok {
		return nil
	}
	if s, ok := asStr(nv); ok {
		if s == "" {
			return nil
		}
		return provenance(s)
	}
	if no, ok := asObj(nv); ok {
		if s, _ := asStr(no.get("v")); s != "" {
			return deepCopy(no)
		}
	}
	return nil
}

// chooseNames sets the name of a new entity in every language that names it: the
// first text key of its zones, then of its container, that has one (nothing is
// lost by language; the entity's name is a text per language, ADR_20260930-5).
// Another name of the same language under a later key is reported. No language
// names it: the tail of its id becomes the name in the main language.
func (m *projectMigration) chooseNames(e *newEnt) {
	keys := e.keys()
	for _, lang := range m.langs {
		td := m.textByLang[lang]
		if td == nil {
			continue
		}
		for _, k := range keys {
			v := nameValue(td, k)
			if v == nil {
				continue
			}
			if e.names[lang] == nil {
				e.names[lang] = v
				continue
			}
			if !sameValue(e.names[lang], v) {
				m.rep.NameConflicts = append(m.rep.NameConflicts,
					fmt.Sprintf("%s %s: «%s» — у сущности %s взято другое имя: «%s»", lang, k, valueText(v), e.id, valueText(e.names[lang])))
			}
		}
	}
	if len(e.names) == 0 {
		e.names[m.langs[0]] = provenance(e.fallback)
		m.rep.NoName = append(m.rep.NoName, fmt.Sprintf("%s → %s: имя «%s» (хвост id)", e.srcID, e.id, e.fallback))
	}
}

// valueText is the `v` of a provenance value.
func valueText(v any) string {
	if o, ok := asObj(v); ok {
		s, _ := asStr(o.get("v"))
		return s
	}
	return ""
}

// ensureText is the text catalogue of a language, made when the project has none:
// a name has to be written somewhere.
func (m *projectMigration) ensureText(lang string) *textData {
	if td := m.textByLang[lang]; td != nil {
		return td
	}
	f := newFile(m.ws, m.rel+"/text."+lang+".json")
	f.root.set("language", jsonStr(lang))
	entries := newObj()
	f.root.set("entries", entries)
	td := &textData{f: f, lang: lang, entries: entries, changed: true}
	m.texts = append(m.texts, td)
	m.textByLang[lang] = td
	return td
}

func (m *projectMigration) convertTexts() error {
	consumed := map[string]bool{}
	for _, e := range m.newEnts {
		for _, k := range e.keys() {
			consumed[k] = true
		}
	}
	for _, v := range m.views {
		for _, z := range v.zones {
			consumed[z.id] = true
		}
	}
	for _, e := range m.newEnts {
		m.chooseNames(e)
		// the fallback name of an entity nobody names is written to the main
		// language, whose catalogue may not exist yet
		langs := make([]string, 0, len(e.names))
		for lang := range e.names {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		for _, lang := range langs {
			m.ensureText(lang)
		}
	}
	seen := map[string]bool{}
	lost := func(list *[]string, s string) {
		if !seen[s] {
			seen[s] = true
			*list = append(*list, s)
		}
	}
	isName := map[string]bool{"name": true, "description": true, "doc": true}

	for _, td := range m.texts {
		if td.entries == nil {
			continue
		}
		newEntries := map[string]*obj{}
		for _, e := range m.newEnts {
			keys := e.keys()
			var target *obj
			existing := false
			if ev, ok := td.entries.lookup(e.id); ok {
				o, ok := asObj(ev)
				if !ok {
					return ferr(td.f.rel, "entries."+e.id, "ожидался объект")
				}
				target, existing = o, true
			}
			// the name of the entity in this language, first among its fields
			if nv := e.names[td.lang]; nv != nil && (target == nil || !target.has("name")) {
				if target == nil {
					target = newObj()
				}
				target.setFirst("name", nv)
				m.rep.NamesMoved++
				td.changed = true
			}
			for _, field := range []string{"description", "doc"} {
				var chosen any
				src := ""
				if target != nil && target.has(field) {
					chosen = target.get(field)
				} else {
					for _, k := range keys {
						if en := entryObj(td, k); en != nil && en.has(field) {
							chosen, src = deepCopy(en.get(field)), k
							break
						}
					}
					if chosen != nil {
						if target == nil {
							target = newObj()
						}
						target.set(field, chosen)
						m.rep.TextsMoved++
						td.changed = true
					}
				}
				if chosen == nil {
					continue
				}
				for _, k := range keys {
					if k == src {
						continue
					}
					if en := entryObj(td, k); en != nil && en.has(field) && !sameValue(en.get(field), chosen) {
						lost(&m.rep.LostText, fmt.Sprintf("%s %s.%s: отброшено, у %s уже есть другой текст", td.lang, k, field, e.id))
					}
				}
			}
			for _, k := range keys {
				en := entryObj(td, k)
				if en == nil {
					continue
				}
				for _, fk := range en.keys {
					if !isName[fk] {
						lost(&m.rep.LostText, fmt.Sprintf("%s %s.%s: поле не переносится", td.lang, k, fk))
					}
				}
			}
			if target != nil && !existing {
				newEntries[e.id] = target
			}
		}

		removed := map[string]bool{}
		for _, k := range td.entries.keys {
			if m.origEnts[k] || m.entIDs[k] {
				continue
			}
			if consumed[k] {
				removed[k] = true
			} else if strings.HasPrefix(k, "z_") || strings.HasPrefix(k, "c_") {
				removed[k] = true
				nm := nameOf(td, k)
				line := fmt.Sprintf("%s %s", td.lang, k)
				if nm != "" {
					line = fmt.Sprintf("%s %s «%s»", td.lang, k, nm)
				}
				// what else the deleted entry held, so that nothing goes silently
				if en := entryObj(td, k); en != nil {
					var rest []string
					for _, fk := range en.keys {
						if fk != "name" {
							rest = append(rest, fk)
						}
					}
					if len(rest) > 0 {
						line += " + " + strings.Join(rest, ", ")
					}
				}
				m.rep.UnconsumedText = append(m.rep.UnconsumedText, line)
			}
		}
		m.rep.TextsRemoved += len(removed)

		var rt *obj
		if m.addContainsRT && !td.entries.has("rt_contains") {
			name := ""
			switch td.lang {
			case "ru":
				name = "содержит"
			case "en":
				name = "contains"
			default:
				m.rep.Notes = append(m.rep.Notes,
					fmt.Sprintf("text.%s.json: имя типа связи contains не добавлено — язык «%s» не ru/en", td.lang, td.lang))
			}
			if name != "" {
				prov := newObj()
				prov.set("v", jsonStr(name))
				prov.set("at", jsonStr(nowUTC()))
				prov.set("origin", jsonStr("authored"))
				rt = newObj()
				rt.set("name", prov)
			}
		}

		if !td.changed && len(removed) == 0 && rt == nil {
			continue
		}
		anchor := map[string][]*newEnt{}
		for _, e := range m.newEnts {
			if newEntries[e.id] == nil {
				continue
			}
			ks := map[string]bool{}
			for _, k := range e.keys() {
				ks[k] = true
			}
			for _, k := range td.entries.keys {
				if ks[k] {
					anchor[k] = append(anchor[k], e)
					break
				}
			}
		}
		out := newObj()
		emitted := map[string]bool{}
		for _, k := range td.entries.keys {
			if removed[k] {
				for _, e := range anchor[k] {
					out.set(e.id, newEntries[e.id])
					emitted[e.id] = true
				}
				continue
			}
			out.set(k, td.entries.m[k])
		}
		for _, e := range m.newEnts {
			if ne := newEntries[e.id]; ne != nil && !emitted[e.id] {
				out.set(e.id, ne)
			}
		}
		if rt != nil {
			out.set("rt_contains", rt)
		}
		td.f.root.set("entries", out)
		td.entries = out // what later steps add goes into the object that is written
		td.f.dirty = true
	}
	return nil
}

// ------------------------------------------------------------------- views

func (m *projectMigration) zoneOverride(v *viewData, z *zoneInfo, kept map[string]int) (*obj, bool) {
	sid, ok := asStr(z.o.get("styleId"))
	if !ok || sid == "" {
		return nil, false
	}
	if z.o.has("override") {
		kept[fmt.Sprintf("«%s» — у зоны уже есть override, styleId оставлен", sid)]++
		return nil, false
	}
	res, colour, found := m.styles.resolveColour(sid)
	switch {
	case !colour:
		kept[fmt.Sprintf("«%s» — не цветовой стиль контейнера, оставлен", sid)]++
	case !found:
		kept[fmt.Sprintf("«%s» — цветовой стиль не найден ни в styles.json, ни среди встроенных, оставлен", sid)]++
	default:
		if o := res.override(); o != nil {
			return o, true
		}
		kept[fmt.Sprintf("«%s» — стиль не задаёт ни заливки, ни рамки, ни заголовка, оставлен", sid)]++
	}
	return nil, false
}

func (m *projectMigration) rewriteViews() error {
	kept := map[string]int{}
	var keptOrder []string
	for _, v := range m.views {
		if !v.hasLayout {
			continue
		}
		root := v.f.root
		var placements []any

		for _, z := range v.zones {
			p := newObj()
			p.set("entity", jsonStr(z.ent.id))
			if z.parentZone != "" {
				p.set("parent", jsonStr(v.zoneByID[z.parentZone].ent.id))
			} else {
				p.set("parent", jsonNull)
			}
			ov, useOverride := m.zoneOverride(v, z, kept)
			for _, k := range z.o.keys {
				switch k {
				case "id", "container", "parent":
				case "styleId":
					if useOverride {
						p.set("override", ov)
						m.rep.Overrides++
					} else {
						p.set(k, deepCopy(z.o.m[k]))
					}
				default:
					p.set(k, deepCopy(z.o.m[k]))
				}
			}
			placements = append(placements, p)
			v.placed[z.ent.id] = true
		}
		dup := map[string]bool{}
		for _, z := range v.zones {
			if dup[z.ent.id] {
				continue
			}
			n := 0
			for _, z2 := range v.zones {
				if z2.ent == z.ent {
					n++
				}
			}
			if n > 1 {
				dup[z.ent.id] = true
				m.rep.Notes = append(m.rep.Notes,
					fmt.Sprintf("%s: сущность %s размещена %d раза (несколько зон одного контейнера)", v.f.rel, z.ent.id, n))
			}
		}

		for _, n := range v.nodes {
			fld := fmt.Sprintf("%s[%d]", n.src, n.idx)
			// The primary spelling wins when present; `id` and `container`
			// are the legacy aliases of `entity` and `zone`.
			ekey, zkey := "id", "container"
			if n.o.has("entity") {
				ekey = "entity"
			}
			if n.o.has("zone") {
				zkey = "zone"
			}
			eid, _, err := strField(n.o, ekey)
			if err != nil {
				return ferr(v.f.rel, fld+"."+ekey, "%v", err)
			}
			if eid == "" {
				return ferr(v.f.rel, fld+".entity", "нет ни entity, ни id")
			}
			zid, _, err := strField(n.o, zkey)
			if err != nil {
				return ferr(v.f.rel, fld+"."+zkey, "%v", err)
			}
			p := newObj()
			p.set("entity", jsonStr(eid))
			p.set("parent", jsonNull)
			if zid != "" {
				if z := v.zoneByID[zid]; z != nil {
					p.set("parent", jsonStr(z.ent.id))
				} else {
					m.rep.UnknownZones = append(m.rep.UnknownZones,
						fmt.Sprintf("%s: %s (%s): зона «%s» не найдена на виде, parent = null", v.f.rel, fld, eid, zid))
				}
			}
			for _, k := range n.o.keys {
				switch k {
				case "entity", "id", "zone", "container", "parent":
				default:
					p.set(k, deepCopy(n.o.m[k]))
				}
			}
			placements = append(placements, p)
			v.placed[eid] = true
		}
		m.rep.Placements += len(placements)
		if placements == nil {
			placements = []any{}
		}

		nv := newObj()
		done := false
		for _, k := range root.keys {
			switch k {
			case "zones", "nodes", "placements":
				if !done {
					nv.set("placements", placements)
					done = true
				}
			default:
				nv.set(k, root.m[k])
			}
		}
		v.f.root = nv
		v.f.dirty = true
	}
	// Deterministic order of the "kept" list: sorted by text.
	for k := range kept {
		keptOrder = append(keptOrder, k)
	}
	sort.Strings(keptOrder)
	for _, k := range keptOrder {
		m.rep.KeptStyleIDs = append(m.rep.KeptStyleIDs, fmt.Sprintf("%s, зон: %d", k, kept[k]))
	}
	return nil
}

func (m *projectMigration) viewDefault(v *viewData) string {
	if ro, ok := asObj(v.f.root.get("relations")); ok {
		if s, ok, _ := strField(ro, "default"); ok && s != "" {
			return s
		}
	}
	return "visible"
}

// exceptions hides the new containment relations on the views where nesting
// already shows them.
func (m *projectMigration) exceptions() error {
	if len(m.added) == 0 {
		return nil
	}
	for _, v := range m.views {
		if v.f.root.has("edges") {
			continue
		}
		visible := m.containsVis == "visible" || (m.containsVis == "" && m.viewDefault(v) != "hidden")
		if !visible {
			continue
		}
		var ids []string
		for _, c := range m.added {
			if v.placed[c.from] && v.placed[c.to] {
				ids = append(ids, c.id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		var ro *obj
		if rv, ok := v.f.root.lookup("relations"); ok {
			o, ok := asObj(rv)
			if !ok {
				return ferr(v.f.rel, "relations", "ожидался объект")
			}
			ro = o
		} else {
			ro = newObj()
			ro.set("default", jsonStr("visible"))
			v.f.root.insertBefore("placements", "relations", ro)
		}
		var ex []any
		if xv, ok := ro.lookup("except"); ok {
			a, ok := asArr(xv)
			if !ok {
				return ferr(v.f.rel, "relations.except", "ожидался массив")
			}
			ex = a
		}
		have := map[string]bool{}
		for _, x := range ex {
			if s, ok := asStr(x); ok {
				have[s] = true
			}
		}
		for _, id := range ids {
			if !have[id] {
				ex = append(ex, jsonStr(id))
			}
		}
		if ex == nil {
			ex = []any{}
		}
		ro.set("except", ex)
		v.f.dirty = true
	}
	return nil
}

// --------------------------------------------------------------- registry

func (m *projectMigration) appendRegistry() error {
	if len(m.newEnts) > 0 {
		if m.ents == nil {
			m.ents = newFile(m.ws, m.rel+"/entities.json")
		}
		arr, err := m.ents.array("entities")
		if err != nil {
			return err
		}
		for _, e := range m.newEnts {
			o := newObj()
			o.set("id", jsonStr(e.id))
			// an authored entity has no name here: its name is a text under its id
			o.set("kind", jsonStr("group"))
			o.set("origin", jsonStr("authored"))
			o.set("status", jsonStr("present"))
			arr = append(arr, o)
		}
		m.ents.root.set("entities", arr)
		m.ents.dirty = true
	}
	if len(m.newRels) > 0 {
		if m.rels == nil {
			m.rels = newFile(m.ws, m.rel+"/relations.json")
		}
		arr, err := m.rels.array("relations")
		if err != nil {
			return err
		}
		m.rels.root.set("relations", append(arr, m.newRels...))
		m.rels.dirty = true
	}
	if m.addContainsRT {
		if m.rtypes == nil {
			m.rtypes = newFile(m.ws, m.rel+"/relation-types.json")
		}
		arr, err := m.rtypes.array("relationTypes")
		if err != nil {
			return err
		}
		t := newObj()
		t.set("id", jsonStr("contains"))
		t.set("origin", jsonStr("authored"))
		t.set("visibility", jsonStr("hidden"))
		m.rtypes.root.set("relationTypes", append(arr, t))
		m.rtypes.dirty = true
	}
	return nil
}

func (m *projectMigration) collect() ([]fileOut, error) {
	m.proj.bump, m.proj.ensureVersion = true, true
	var files []*file
	files = append(files, m.proj)
	for _, f := range []*file{m.ents, m.rels, m.rtypes} {
		if f != nil {
			f.bump = true
			files = append(files, f)
		}
	}
	for _, v := range m.views {
		v.f.bump = true
		files = append(files, v.f)
	}
	for _, td := range m.texts {
		td.f.bump = true
		files = append(files, td.f)
	}
	if m.conts != nil && m.fromV3 {
		m.conts.remove = true
		files = append(files, m.conts)
	}
	var outs []fileOut
	for _, f := range files {
		o, changed, err := f.output()
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		outs = append(outs, o)
		if o.remove {
			m.rep.Removed = append(m.rep.Removed, f.rel)
		} else {
			m.rep.Files = append(m.rep.Files, f.rel)
		}
	}
	m.rep.Skipped = !m.fromV3 && len(outs) == 0
	return outs, nil
}
