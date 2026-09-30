// Package core is the language-neutral part of SeMaps: what is checked and
// synced against a workspace, independent of the editor and of any extractor.
//
// Check reports, per project (contract v5):
//
//  1. stale translations — `fromHash` no longer matches its source
//  2. divergences — two languages both `authored`, neither derived from the
//     other: not translations at all
//  3. missing text — an id used by the structure has no text; an authored
//     entity has no `name` text in any language (ADR_20260930-5); a `name`
//     text under the id of an entity from code is «лишнее имя» — nobody reads it
//  4. views without an axis
//  5. containment contradictions — one block placed in different containers
//     by two views declaring the *same* axis
//  6. broken code[].ref — a file that is no longer there; a malformed code[]
//     or evidence[] entry (ADR_20260930-4)
//  7. the old shape — project.json of another contractVersion, a view with
//     `zones`/`nodes`, containers.json, c_/z_ text keys, `kinds` or a zone key
//     in a workspace file, a top-level `codeRef`/`symbol`/`via`, the `name` of
//     an authored entity in entities.json (ADR_20260927-3): the same text the
//     loader answers with
//  8. placements — of no entity, in a parent that is not a container placement
//     of the view or in a loop, an override field outside CONTRACT §11.6
//  9. entity kinds and relation types not in the dictionary («не из словаря»):
//     not an error of the model, a list for a human to look at (CONTRACT §6)
//  10. styles of the workspace styles.json without `forKinds` («без типа»)
//
// Plus two cheap ones that catch the same rot earlier: placeholder names (a
// name equal to the id says nothing) and relation types used but not declared.
//
// This check is the only reason the provenance fields are worth writing:
// unchecked, they decay into optional fields nobody fills in.
// See ADR_20260831_diagrams_text-provenance-and-view-axes.
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

// textFields is kept in step with TEXT_FIELDS in editor/src/model/text-provenance.ts
// by hand. `fromLabel`/`toLabel` are the end-label captions on a relation
// (ADR_20260903 §2.6) — same shape, same provenance mechanism as every other
// field, so the checks apply to them without change.
var textFields = []string{"name", "title", "description", "doc", "fromLabel", "toLabel"}

// Finding is one observation about one project.
type Finding struct {
	Project string
	Kind    string
	Message string
}

type textValue struct {
	V        string `json:"v"`
	Origin   string `json:"origin"`
	From     string `json:"from"`
	FromHash string `json:"fromHash"`
}

type textRecord map[string]json.RawMessage

func (r textRecord) field(name string) (textValue, bool) {
	raw, ok := r[name]
	if !ok || len(raw) == 0 || raw[0] != '{' {
		return textValue{}, false
	}
	var v textValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return textValue{}, false
	}
	return v, true
}

type viewFile struct {
	name string
	id   string
	doc  *object
}

var (
	trailingSpace = regexp.MustCompile(`(?m)[ \t]+$`)
	blankRuns     = regexp.MustCompile(`\n{3,}`)
)

// normalise strips what is noise before hashing — line endings, trailing
// spaces, runs of blank lines — and keeps indentation, which carries meaning
// in markdown. Must stay in step with whatever writes `fromHash`.
func normalise(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = trailingSpace.ReplaceAllString(s, "")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// Hash is FNV-1a, 32 bit, over UTF-16 code units — the same units the editor
// hashes in JavaScript. Change detection, not security.
func Hash(text string) string {
	h := uint32(0x811c9dc5)
	for _, u := range utf16.Encode([]rune(normalise(text))) {
		h ^= uint32(u)
		h *= 0x01000193
	}
	return fmt.Sprintf("%08x", h)
}

func readJSON(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, into)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type seenPlacement struct{ container, where string }

// Check walks every project under <workspace>/projects and resolves codeRef
// against sourceRoot. defaultKinds is the tool's dictionary, which
// <workspace>/kinds.json adds to. A file that cannot be parsed is itself a
// finding, not a crash: the rest of the workspace is still checked.
func Check(workspace, sourceRoot string, defaultKinds []byte) []Finding {
	var findings []Finding
	report := func(project, kind, msg string) {
		findings = append(findings, Finding{project, kind, msg})
	}
	// node id -> axis -> placement, gathered across every project.
	byAxis := map[string]map[string]seenPlacement{}

	if exists(filepath.Join(workspace, "catalog.json")) {
		report("workspace", "устарело", "catalog.json больше не читается (ADR_20260923-7): имя вида — `name` под его id в text.<lang>.json, icon/theme — в самом .view.json; затем удалите файл")
	}

	kinds, err := LoadKinds(workspace, defaultKinds)
	if err != nil {
		report("workspace", "не читается", err.Error())
	}
	checkWorkspaceFiles(workspace, kinds, report)

	root := filepath.Join(workspace, "projects")
	projectDirs, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return findings // an empty workspace is legal: projects are created from the editor
	}
	if err != nil {
		report("workspace", "workspace", fmt.Sprintf("%s: %v", root, err))
		return findings
	}

	for _, entry := range projectDirs {
		if !entry.IsDir() {
			continue
		}
		project := entry.Name()
		dir := filepath.Join(root, project)
		if !exists(filepath.Join(dir, "project.json")) {
			continue
		}
		load := func(name string, into any) bool {
			path := filepath.Join(dir, name)
			if !exists(path) {
				return false
			}
			if err := readJSON(path, into); err != nil {
				report(project, "не читается", fmt.Sprintf("%s: %v", name, err))
				return false
			}
			return true
		}

		var manifest struct {
			Languages   []string `json:"languages"`
			DefaultAxis string   `json:"defaultAxis"`
		}
		if !load("project.json", &manifest) {
			continue
		}
		if doc, err := loadDoc(filepath.Join(dir, "project.json")); err == nil && doc != nil {
			if err := checkContractVersion(doc); err != nil {
				report(project, "форма контракта", err.Error())
				continue
			}
		}
		if exists(filepath.Join(dir, "containers.json")) {
			report(project, "форма контракта", containersFileError)
		}
		languages := manifest.Languages
		if len(languages) == 0 {
			languages = []string{"ru"}
		}

		// the registries as ordered objects, so that the old shape can be named
		// by its keys (oldShape); the loader answers with the same text
		var ents struct {
			Entities []*object `json:"entities"`
		}
		load("entities.json", &ents)
		var rels struct {
			Relations []*object `json:"relations"`
		}
		load("relations.json", &rels)
		for _, e := range ents.Entities {
			if old := oldShape("entity", e); old != "" {
				report(project, "форма контракта", fmt.Sprintf("entities.json: %s: %s (`semaps migrate`, ADR_20260930-4/5)", e.str("id"), old))
			}
		}
		for _, r := range rels.Relations {
			if old := oldShape("relation", r); old != "" {
				report(project, "форма контракта", fmt.Sprintf("relations.json: %s: %s (`semaps migrate`, ADR_20260930-4)", r.str("id"), old))
			}
		}
		var types struct {
			RelationTypes []struct {
				ID      string
				StyleID string `json:"styleId"`
			} `json:"relationTypes"`
		}
		declared := map[string]bool{}
		if load("relation-types.json", &types) {
			for _, t := range types.RelationTypes {
				declared[t.ID] = true
				if t.StyleID != "" {
					report(project, "форма контракта", fmt.Sprintf("relation-types.json: %s: `styleId` — стиль связи принадлежит типу: базовый стиль задаёт kinds.json, стиль называет тип в forKinds (`semaps migrate`, ADR_20260930-2)", t.ID))
				}
			}
		}

		catalogues := map[string]map[string]textRecord{}
		for _, lang := range languages {
			var cat struct {
				Entries map[string]textRecord `json:"entries"`
			}
			load("text."+lang+".json", &cat)
			if cat.Entries == nil {
				cat.Entries = map[string]textRecord{}
			}
			catalogues[lang] = cat.Entries
		}

		// ---------------------------------------------- 1 & 2: provenance health
		for _, lang := range languages {
			for _, id := range sortedKeys(catalogues[lang]) {
				record := catalogues[lang][id]
				for _, field := range textFields {
					value, ok := record.field(field)
					if !ok {
						continue
					}
					if value.Origin == "translated" {
						source, ok := catalogues[value.From][id].field(field)
						if !ok {
							report(project, "протух", fmt.Sprintf("%s.%s@%s: источник %s исчез", id, field, lang, value.From))
						} else if Hash(source.V) != value.FromHash {
							report(project, "протух", fmt.Sprintf("%s.%s@%s: источник %s изменился", id, field, lang, value.From))
						}
					}
					if value.Origin == "authored" && len(languages) > 1 {
						for _, other := range languages {
							if lang >= other {
								continue
							}
							if rival, ok := catalogues[other][id].field(field); ok && rival.Origin == "authored" {
								report(project, "расхождение", fmt.Sprintf("%s.%s: %s и %s написаны независимо, связи между ними нет", id, field, lang, other))
							}
						}
					}
				}
				if name, ok := record.field("name"); ok && name.V != "" && name.V == id {
					report(project, "имя-заглушка", fmt.Sprintf("%s@%s: имя совпадает с идентификатором", id, lang))
				}
			}
		}

		// ------------------------------------------------------- 3: недостача
		// Only what is actually authored is expected to carry text. The name of
		// an entity from code is canonical and lives in entities.json — not
		// translated. The name of an authored entity (a container is one too,
		// CONTRACT §8.2) is a text under its id; a language without it falls back
		// to another one, so the gap is an entity with no name in any language
		// (ADR_20260930-5). Views and relation types are named by a human in every
		// language, so silence there is a real gap.
		mustBeNamed := map[string]bool{}
		for id := range declared {
			mustBeNamed[id] = true
		}
		viewsDir := filepath.Join(dir, "views")
		var views []viewFile
		if files, err := os.ReadDir(viewsDir); err == nil {
			for _, f := range files {
				if !strings.HasSuffix(f.Name(), ViewSuffix) {
					continue
				}
				doc, err := loadDoc(filepath.Join(viewsDir, f.Name()))
				if err != nil {
					report(project, "не читается", fmt.Sprintf("views/%s: %v", f.Name(), err))
					continue
				}
				id := doc.str("id")
				if id == "" {
					id = strings.TrimSuffix(f.Name(), ViewSuffix)
				}
				mustBeNamed[id] = true
				if old := oldViewShape(doc); old != "" {
					report(project, "форма контракта", oldShapeError("views/"+f.Name(), old))
					continue
				}
				views = append(views, viewFile{f.Name(), id, doc})
			}
		}
		for _, lang := range languages {
			for _, id := range sortedKeys(mustBeNamed) {
				key := id
				if declared[id] {
					key = "rt_" + id
				}
				record, ok := catalogues[lang][key]
				if !ok || (record["name"] == nil && record["title"] == nil) {
					report(project, "недостача", fmt.Sprintf("%s: нет имени в %s", key, lang))
				}
			}
			for _, key := range sortedKeys(catalogues[lang]) {
				if strings.HasPrefix(key, "c_") || strings.HasPrefix(key, "z_") {
					report(project, "форма контракта", fmt.Sprintf("text.%s.json: %s — ключи c_/z_ упразднены, имя контейнера — текст name под id его сущности (CONTRACT §7.1)", lang, key))
				}
				// a bare string instead of a value with provenance is the shape of contract 2; nobody reads it
				for _, field := range textFields {
					if raw, ok := catalogues[lang][key][field]; ok && len(raw) > 0 && raw[0] != '{' {
						report(project, "форма контракта", fmt.Sprintf("text.%s.json: %s.%s — строка вместо значения с происхождением, форма контракта 2, нужен %d (CONTRACT §7)", lang, key, field, ContractVersion))
					}
				}
			}
		}

		for _, e := range ents.Entities {
			if !isAuthored(e) {
				// the name of an entity from code is the code's, in entities.json; a `name` text under
				// its id is left over from before ADR_20260930-5 and is never read
				for _, lang := range languages {
					if record, ok := catalogues[lang][e.str("id")]; ok {
						if _, ok := record.field("name"); ok {
							report(project, "лишнее имя", fmt.Sprintf("%s@%s: текст name у сущности из кода не читается — её имя в entities.json (ADR_20260930-5)", e.str("id"), lang))
						}
					}
				}
				continue
			}
			named := false
			for _, lang := range languages {
				if record, ok := catalogues[lang][e.str("id")]; ok {
					if v, ok := record.field("name"); ok && strings.TrimSpace(v.V) != "" {
						named = true
					}
				}
			}
			if !named {
				report(project, "недостача", fmt.Sprintf("%s: у нарисованной сущности нет имени ни в одном языке (`name` в text.<lang>.json, CONTRACT §7.1)", e.str("id")))
			}
		}

		// ---------------------------------------- 9: типы не из словаря
		entityKind := map[string]string{}
		outside := map[string][]string{}
		for _, e := range ents.Entities {
			id, kind := e.str("id"), e.str("kind")
			entityKind[id] = kind
			if _, ok := kinds.Lookup(kind); !ok && kind != "" {
				outside[kind] = append(outside[kind], id)
			}
		}
		for _, k := range sortedKeys(outside) {
			ids := outside[k]
			more := ""
			if len(ids) > 3 {
				ids, more = ids[:3], fmt.Sprintf(" и ещё %d", len(outside[k])-3)
			}
			report(project, "тип не из словаря", fmt.Sprintf("%q: %s%s", k, strings.Join(ids, ", "), more))
		}
		for _, id := range sortedKeys(declared) {
			if _, ok := kinds.LookupRelation(id); !ok {
				report(project, "тип связи не из словаря", fmt.Sprintf("%q объявлен в relation-types.json, но его нет в словаре (kinds.json, relationGroups)", id))
			}
		}

		// ---------------------------------------------------- 8: размещения
		for _, vf := range views {
			for _, msg := range checkPlacements(vf.doc, entityKind, kinds) {
				report(project, "размещение", fmt.Sprintf("views/%s: %s", vf.name, msg))
			}
		}

		// ------------------------------------------------ типы связей объявлены
		for _, r := range rels.Relations {
			if t := relationType(r); t != "" && len(declared) > 0 && !declared[t] {
				report(project, "тип связи", fmt.Sprintf("%s: тип %q не объявлен в relation-types.json", r.str("id"), t))
			}
			if err := checkCode("relation", r.str("id"), entries(r, "evidence")); err != nil {
				report(project, "реализация", err.Error())
			}
		}

		// -------------------------------------------------- 4 & 5: виды и оси
		for _, vf := range views {
			axis := vf.doc.str("axis")
			if axis == "" {
				axis = manifest.DefaultAxis
			}
			if axis == "" {
				report(project, "вид без оси", fmt.Sprintf("%s: и у проекта нет defaultAxis", vf.name))
				continue
			}
			for _, p := range viewItems(vf.doc, "placements") {
				id, container := p.str("entity"), p.str("parent")
				if id == "" || container == "" {
					continue
				}
				perNode := byAxis[axis]
				if perNode == nil {
					perNode = map[string]seenPlacement{}
					byAxis[axis] = perNode
				}
				where := project + "/" + vf.name
				if seen, ok := perNode[id]; ok {
					if seen.container != container {
						report(project, "противоречие", fmt.Sprintf("%s лежит в %s (%s) и в %s (%s) — одна ось %s", id, seen.container, seen.where, container, where, axis))
					}
				} else {
					perNode[id] = seenPlacement{container, where}
				}
			}
		}

		// ------------------------------------------------------- 6: code[].ref
		for _, e := range ents.Entities {
			if err := checkCode("entity", e.str("id"), entries(e, "code")); err != nil {
				report(project, "реализация", err.Error())
			}
			for _, c := range entries(e, "code") {
				ref := c.str("ref")
				// a realization of an external symbol has no file (ADR_20260927-4)
				if ref != "" && !exists(filepath.Join(sourceRoot, filepath.FromSlash(refFile(ref)))) {
					report(project, "битая ссылка на код", fmt.Sprintf("%s: %s", e.str("id"), ref))
				}
			}
		}
	}
	return findings
}

// checkPlacements reads the placements of a view of the current shape: each
// places an entity of entities.json at most once, names its parent (null for
// none), a parent is a container placement of the same view and there is no
// loop, an override has only the fields of the table (CONTRACT §8.2, §11.6);
// the same for the override of an edge entry of the view's own `edges`.
func checkPlacements(doc *object, entityKind map[string]string, kinds *KindCatalog) []string {
	var out []string
	items := viewItems(doc, "placements")
	byEntity := map[string]*object{}
	for i, p := range items {
		id := p.str("entity")
		switch {
		case id == "":
			out = append(out, fmt.Sprintf("placements[%d]: нет entity", i))
			continue
		case byEntity[id] != nil:
			out = append(out, fmt.Sprintf("%s размещена дважды", id))
		}
		byEntity[id] = p
		if _, ok := entityKind[id]; !ok {
			out = append(out, fmt.Sprintf("%s: нет такой сущности в entities.json", id))
		}
		if _, ok := p.vals["parent"]; !ok {
			out = append(out, fmt.Sprintf("%s: нет поля parent (null — вне контейнеров)", id))
		}
		if err := CheckOverride(p.vals["override"]); err != nil {
			out = append(out, fmt.Sprintf("%s: %v", id, err))
		}
	}
	for _, p := range items {
		id := p.str("entity")
		parent := p.str("parent")
		if id == "" || parent == "" {
			continue
		}
		if byEntity[parent] == nil {
			out = append(out, fmt.Sprintf("%s: parent %s не размещён на этом виде", id, parent))
			continue
		}
		if !kinds.IsContainer(entityKind[parent]) {
			out = append(out, fmt.Sprintf("%s: parent %s типа %q — не контейнер по словарю", id, parent, entityKind[parent]))
			continue
		}
		for q, hops := parent, 0; q != "" && byEntity[q] != nil; q, hops = byEntity[q].str("parent"), hops+1 {
			if q == id || hops > len(items) {
				out = append(out, fmt.Sprintf("%s: вложенность по parent замыкается в цикл", id))
				break
			}
		}
	}
	for _, e := range viewItems(doc, "edges") {
		if err := CheckEdgeOverride(e.vals["override"]); err != nil {
			out = append(out, fmt.Sprintf("связь %s: %v", e.str("id"), err))
		}
	}
	return out
}

// checkWorkspaceFiles reads the workspace-level files the contract changed:
// canvas.json (`zone` became `container`) and styles.json (`kinds` became
// `forKinds`, mandatory for block, container and edge styles, ADR_20260927-7,
// ADR_20260930-2).
func checkWorkspaceFiles(workspace string, kinds *KindCatalog, report func(project, kind, msg string)) {
	if doc, err := loadDoc(filepath.Join(workspace, CanvasFile)); err == nil && doc != nil {
		gap, _ := child(doc, "gap")
		if has(doc, "zone") || has(gap, "zone") {
			report("workspace", "форма контракта", fmt.Sprintf("%s: `zone` — в контракте %d ключ называется `container` (`semaps migrate`)", CanvasFile, ContractVersion))
		}
	}
	doc, err := loadDoc(filepath.Join(workspace, "styles.json"))
	if err != nil {
		report("workspace", "не читается", err.Error())
		return
	}
	if doc == nil {
		return
	}
	for _, s := range viewItems(doc, "styles") {
		id, appliesTo := s.str("id"), s.str("appliesTo")
		if has(s, "kinds") {
			report("workspace", "форма контракта", fmt.Sprintf("styles.json: %s: `kinds` — в контракте %d поле называется `forKinds` (`semaps migrate`)", id, ContractVersion))
		}
		if id == "default.node" || id == "default.container" || id == "default.edge" {
			continue
		}
		var forKinds []string
		if raw, ok := s.vals["forKinds"]; ok {
			_ = json.Unmarshal(raw, &forKinds)
		}
		if len(forKinds) == 0 {
			what := "стиль без типа"
			if appliesTo == "edge" {
				what = "стиль связи без типа"
			}
			report("workspace", "без типа", fmt.Sprintf("styles.json: %s — %s: нет forKinds (ADR_20260927-7)", id, what))
			continue
		}
		for _, k := range forKinds {
			if appliesTo == "edge" {
				if _, ok := kinds.LookupRelation(k); !ok {
					report("workspace", "тип не из словаря", fmt.Sprintf("styles.json: %s: forKinds %q — нет такого типа связи в словаре", id, k))
				}
				continue
			}
			if _, ok := kinds.Lookup(k); !ok {
				report("workspace", "тип не из словаря", fmt.Sprintf("styles.json: %s: forKinds %q — нет такого типа в словаре", id, k))
			} else if isContainer := kinds.IsContainer(k); isContainer != (appliesTo == "container") {
				report("workspace", "стиль и тип", fmt.Sprintf("styles.json: %s: appliesTo %q, а тип %q %s", id, appliesTo, k, map[bool]string{true: "контейнер", false: "не контейнер"}[isContainer]))
			}
		}
	}
}

// Report prints findings grouped by kind, at most 15 per kind, and returns
// the exit code: 0 when clean, 1 otherwise.
func Report(w io.Writer, findings []Finding) int {
	if len(findings) == 0 {
		fmt.Fprintln(w, "Модель схем: замечаний нет.")
		return 0
	}
	fmt.Fprintf(w, "Модель схем: %d замечаний\n\n", len(findings))
	var order []string
	byKind := map[string][]Finding{}
	for _, f := range findings {
		if _, ok := byKind[f.Kind]; !ok {
			order = append(order, f.Kind)
		}
		byKind[f.Kind] = append(byKind[f.Kind], f)
	}
	for _, kind := range order {
		items := byKind[kind]
		fmt.Fprintf(w, "%s (%d)\n", kind, len(items))
		for i, item := range items {
			if i == 15 {
				fmt.Fprintf(w, "  ... и ещё %d\n", len(items)-15)
				break
			}
			fmt.Fprintf(w, "  %s: %s\n", item.Project, item.Message)
		}
		fmt.Fprintln(w)
	}
	return 1
}
