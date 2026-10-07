package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LibraryFile is a deterministic, portable filename. The hash prevents two
// assembly identities with the same slug from sharing a file.
func LibraryFile(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "external/" + slug(id) + fmt.Sprintf("-%x.json", sum[:4])
}

func entityLibrary(e *object) string {
	for _, c := range entries(e, "code") {
		if id := c.str("library"); id != "" {
			return id
		}
	}
	return ""
}

// loadExternalRegistries joins storage shards into the ordinary registries.
// Metadata lives separately, so clients continue to read entity/relation.
func (m *Model) loadExternalRegistries() error {
	files, err := filepath.Glob(filepath.Join(m.dir, "external", "*.json"))
	if err != nil {
		return err
	}
	files = append([]string{filepath.Join(m.dir, "relations.external.json")}, files...)
	seen := map[string]map[string]bool{"entity": {}, "relation": {}}
	relationFiles := map[string]string{}
	for kind, r := range m.registries {
		for _, e := range r.items {
			seen[kind][e.str("id")] = true
			if kind == "relation" {
				relationFiles[e.str("id")] = r.file
			} else if entityLibrary(e) != "" {
				return fmt.Errorf("%s: library entity %s belongs in external/", r.file, e.str("id"))
			}
		}
	}
	for _, file := range files {
		doc, err := loadDoc(file)
		if err != nil {
			return err
		}
		if doc == nil {
			continue
		}
		library := ""
		if filepath.Base(file) != "relations.external.json" {
			header, err := child(doc, "library")
			if err != nil {
				return err
			}
			library = header.str("id")
			if library == "" || header.str("name") == "" || header.str("version") == "" {
				return fmt.Errorf("%s: library needs id, name and version", file)
			}
			if err := checkLibraryHeader(header); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			if m.libraries[library] != nil {
				return fmt.Errorf("%s: duplicate library %s", file, library)
			}
			// Require the deterministic filename: avoids leaving an obsolete duplicate on Save.
			if filepath.ToSlash(file) != filepath.ToSlash(filepath.Join(m.dir, LibraryFile(library))) {
				return fmt.Errorf("%s: expected library file %s", file, LibraryFile(library))
			}
			m.libraries[library] = header
		}
		for kind, spec := range modelRegistries {
			raw, present := doc.vals[spec.key]
			if !present && library == "" && kind == "entity" {
				continue
			}
			var items []*object
			if !present || len(raw) == 0 || raw[0] != '[' {
				return fmt.Errorf("%s: %s must be an array", file, spec.key)
			}
			if err := json.Unmarshal(raw, &items); err != nil {
				return fmt.Errorf("%s: %s: %w", file, spec.key, err)
			}
			for _, item := range items {
				if item == nil {
					return fmt.Errorf("%s: %s contains null", file, spec.key)
				}
				id := item.str("id")
				if id == "" || seen[kind][id] {
					return fmt.Errorf("%s: duplicate or empty %s id %q", file, kind, id)
				}
				if old := oldShape(kind, item); old != "" {
					return fmt.Errorf("%s: %s: %s", file, id, old)
				}
				if kind == "entity" && (library == "" || entityLibrary(item) != library) {
					return fmt.Errorf("%s: entity %s has inconsistent library", file, id)
				}
				seen[kind][id] = true
				if kind == "relation" {
					relationFiles[id] = file
				}
				r := m.registries[kind]
				r.items = append(r.items, item)
				r.saved[id] = true
			}
		}
	}
	owners := entityOwners(m.registries["entity"].items)
	for _, r := range m.registries["relation"].items {
		if file := relationFiles[r.str("id")]; file != "" && filepath.ToSlash(file) != filepath.ToSlash(filepath.Join(m.dir, relationFile(r, owners))) {
			return fmt.Errorf("%s: relation %s belongs in %s", file, r.str("id"), relationFile(r, owners))
		}
	}
	return m.checkLibraryRecords()
}

func (m *Model) checkLibraryRecords() error {
	for _, e := range m.registries["entity"].items {
		owner := ""
		for _, c := range entries(e, "code") {
			id := c.str("library")
			if id == "" {
				continue
			}
			if c.str("ref") != "" || c.str("symbol") == "" || m.libraries[id] == nil {
				return refuse("entity %s: library realization needs a known library and symbol, without source ref", e.str("id"))
			}
			if owner != "" && owner != id {
				return refuse("entity %s belongs to multiple libraries", e.str("id"))
			}
			owner = id
		}
	}
	for _, r := range m.registries["relation"].items {
		for _, c := range entries(r, "evidence") {
			if id := c.str("library"); id != "" && (c.str("ref") != "" || c.str("symbol") == "" || m.libraries[id] == nil) {
				return refuse("relation %s: library evidence needs a known library and symbol, without source ref", r.str("id"))
			}
		}
	}
	return nil
}

func checkLibraryHeader(header *object) error {
	if raw, ok := header.vals["complete"]; ok {
		var complete bool
		if string(raw) == "null" || json.Unmarshal(raw, &complete) != nil {
			return refuse("library %s: complete must be a boolean", header.str("id"))
		}
	}
	return nil
}

func entityOwners(items []*object) map[string]string {
	owners := make(map[string]string, len(items))
	for _, e := range items {
		owners[e.str("id")] = entityLibrary(e)
	}
	return owners
}

func relationFile(r *object, owners map[string]string) string {
	if owner := owners[r.str("from")]; owner != "" {
		return LibraryFile(owner)
	}
	if owners[r.str("to")] != "" {
		return "relations.external.json"
	}
	return "relations.json"
}

// saveRegistries partitions the logical model and writes only changed bytes.
// This prevents a source edit from rewriting every unchanged library snapshot.
func (m *Model) saveRegistries() error {
	changed := m.registries["entity"].dirty || m.registries["relation"].dirty
	for _, ref := range m.dirty.Registry {
		changed = changed || ref.Kind == "library"
	}
	if !changed {
		return nil
	}
	if err := m.checkLibraryRecords(); err != nil {
		return err
	}
	docs := map[string]*object{}
	for _, spec := range modelRegistries {
		top := cloneObject(m.registries[map[string]string{"entities": "entity", "relations": "relation"}[spec.key]].top)
		top.set(spec.key, []*object{})
		docs[spec.file] = top
	}
	boundary := newObject()
	boundary.set("relations", []*object{})
	docs["relations.external.json"] = boundary
	for id, header := range m.libraries {
		d := newObject()
		d.set("library", header)
		d.set("entities", []*object{})
		d.set("relations", []*object{})
		docs[LibraryFile(id)] = d
	}
	owners := entityOwners(m.registries["entity"].items)
	entityLists := map[string][]*object{}
	relationLists := map[string][]*object{}
	for _, e := range m.registries["entity"].items {
		owner := entityLibrary(e)
		file := "entities.json"
		if owner != "" {
			file = LibraryFile(owner)
		}
		entityLists[file] = append(entityLists[file], e)
	}
	for _, r := range m.registries["relation"].items {
		file := relationFile(r, owners)
		relationLists[file] = append(relationLists[file], r)
	}
	for file, list := range entityLists {
		docs[file].set("entities", list)
	}
	for file, list := range relationLists {
		docs[file].set("relations", list)
	}
	var files []string
	for f := range docs {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, name := range files {
		file := filepath.Join(m.dir, filepath.FromSlash(name))
		doc := docs[name]
		// A new workspace has no reason to acquire an empty boundary file.
		if name == "relations.external.json" && len(entries(doc, "relations")) == 0 && !exists(file) {
			continue
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return err
		}
		if old, err := os.ReadFile(file); err == nil && bytes.Equal(old, buf.Bytes()) {
			continue
		}
		// Preserve the original own file when only a different shard changed.
		{
			old, err := loadDoc(file)
			if err != nil {
				return err
			}
			if old != nil {
				a, _ := old.MarshalJSON()
				b, _ := doc.MarshalJSON()
				if compactJSON(a) == compactJSON(b) {
					continue
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(file, buf.Bytes(), 0644); err != nil {
			return err
		}
	}
	return nil
}

type libraryCoverage struct {
	complete map[string]bool
	expanded map[string]string
}

func newLibraryCoverage(facts *Facts) libraryCoverage {
	scope := libraryCoverage{map[string]bool{}, map[string]string{}}
	for _, l := range facts.Libraries {
		scope.complete[l.ID] = l.Complete
		for _, id := range l.Expanded {
			scope.expanded[id] = l.ID
		}
	}
	return scope
}

func (scope libraryCoverage) covers(library, symbol string) bool {
	return scope.complete[library] || scope.expanded[symbol] == library
}

func (scope libraryCoverage) symbolExpanded(s Symbol) bool {
	return s.Library == "" || scope.covers(s.Library, s.ID)
}

func (scope libraryCoverage) entityCovered(facts *Facts, e *object) bool {
	for _, c := range entries(e, "code") {
		if c.str("lang") == facts.Language && c.str("library") != "" {
			return scope.covers(c.str("library"), c.str("symbol"))
		}
	}
	return true
}

// LibrarySnapshots returns metadata without exposing mutable model objects.
func (m *Model) LibrarySnapshots() []Library {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Library{}
	for _, doc := range m.libraries {
		var l Library
		raw, _ := doc.MarshalJSON()
		_ = json.Unmarshal(raw, &l)
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].ID, out[j].ID) < 0 })
	return out
}
