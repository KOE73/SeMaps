// Package core is the language-neutral part of SeMaps: what is checked and
// synced against a workspace, independent of the editor and of any extractor.
//
// Check reports, per project (contract v3):
//
//  1. stale translations — `fromHash` no longer matches its source
//  2. divergences — two languages both `authored`, neither derived from the
//     other: not translations at all
//  3. missing text — an id used by the structure has no text
//  4. views without an axis
//  5. containment contradictions — one node placed in different containers
//     by two views declaring the *same* axis
//  6. broken codeRef — a file that is no longer there
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

type placement struct {
	Entity    string `json:"entity"`
	ID        string `json:"id"`
	Container string `json:"container"`
	Zone      string `json:"zone"`
}

type view struct {
	ID         string                `json:"id"`
	Axis       string                `json:"axis"`
	Zones      []struct{ ID string } `json:"zones"`
	Nodes      []placement           `json:"nodes"`
	Placements []placement           `json:"placements"`
}

type viewFile struct {
	name string
	view view
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
// against sourceRoot. A file that cannot be parsed is itself a finding, not a
// crash: the rest of the workspace is still checked.
func Check(workspace, sourceRoot string) []Finding {
	var findings []Finding
	report := func(project, kind, msg string) {
		findings = append(findings, Finding{project, kind, msg})
	}
	// node id -> axis -> placement, gathered across every project.
	byAxis := map[string]map[string]seenPlacement{}

	if exists(filepath.Join(workspace, "catalog.json")) {
		report("workspace", "устарело", "catalog.json больше не читается (ADR_20260923-7): имя вида — `name` под его id в text.<lang>.json, icon/theme — в самом .view.json; затем удалите файл")
	}

	root := filepath.Join(workspace, "projects")
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return findings // an empty workspace is legal: projects are created from the editor
	}
	if err != nil {
		report("workspace", "workspace", fmt.Sprintf("%s: %v", root, err))
		return findings
	}

	for _, entry := range entries {
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
		languages := manifest.Languages
		if len(languages) == 0 {
			languages = []string{"ru"}
		}

		var ents struct {
			Entities []struct{ ID, CodeRef string } `json:"entities"`
		}
		load("entities.json", &ents)
		var rels struct {
			Relations []struct{ ID, Type, Relation string } `json:"relations"`
		}
		load("relations.json", &rels)
		var types struct {
			RelationTypes []struct{ ID string } `json:"relationTypes"`
		}
		declared := map[string]bool{}
		if load("relation-types.json", &types) {
			for _, t := range types.RelationTypes {
				declared[t.ID] = true
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
		// Only what is actually authored is expected to carry text. An entity's
		// name is canonical and lives in entities.json — not translated.
		// Containers, views and relation types are named by a human, so
		// silence there is a real gap.
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
				var v view
				if err := readJSON(filepath.Join(viewsDir, f.Name()), &v); err != nil {
					report(project, "не читается", fmt.Sprintf("views/%s: %v", f.Name(), err))
					continue
				}
				views = append(views, viewFile{f.Name(), v})
				id := v.ID
				if id == "" {
					id = strings.TrimSuffix(f.Name(), ".view.json")
				}
				mustBeNamed[id] = true
				// A zone id is `z_<x>` for the container `c_<x>` it renders; its
				// text is keyed to the container. Same shim as in ProjectStore.
				for _, z := range v.Zones {
					if strings.HasPrefix(z.ID, "z_") {
						mustBeNamed["c_"+z.ID[2:]] = true
					} else {
						mustBeNamed[z.ID] = true
					}
				}
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
		}

		// ------------------------------------------------ типы связей объявлены
		for _, r := range rels.Relations {
			t := r.Type
			if t == "" {
				t = r.Relation
			}
			if t != "" && len(declared) > 0 && !declared[t] {
				report(project, "тип связи", fmt.Sprintf("%s: тип %q не объявлен в relation-types.json", r.ID, t))
			}
		}

		// -------------------------------------------------- 4 & 5: виды и оси
		for _, vf := range views {
			axis := vf.view.Axis
			if axis == "" {
				axis = manifest.DefaultAxis
			}
			if axis == "" {
				report(project, "вид без оси", fmt.Sprintf("%s: и у проекта нет defaultAxis", vf.name))
				continue
			}
			nodes := vf.view.Nodes
			if nodes == nil {
				nodes = vf.view.Placements
			}
			for _, p := range nodes {
				id, container := p.Entity, p.Container
				if id == "" {
					id = p.ID
				}
				if container == "" {
					container = p.Zone
				}
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

		// ------------------------------------------------------------ 6: codeRef
		for _, e := range ents.Entities {
			if e.CodeRef == "" {
				continue
			}
			file := e.CodeRef
			if i := strings.IndexAny(file, "#:"); i >= 0 {
				file = file[:i]
			}
			if !exists(filepath.Join(sourceRoot, filepath.FromSlash(file))) {
				report(project, "битый codeRef", fmt.Sprintf("%s: %s", e.ID, e.CodeRef))
			}
		}
	}
	return findings
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
