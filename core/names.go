package core

// The name an entity is shown by (CONTRACT §3, §7; ADR_20260930-5). An entity
// of origin `authored` — drawn by a person or an agent — has its name in
// text.<lang>.json under its id, per language, and may change; an entity of any
// other origin keeps the name of its code in entities.json, not translated.
// The id never changes either way.

import (
	"encoding/json"
	"strings"
)

// isAuthored: the entity is drawn by hand, so its name is a text.
func isAuthored(e *object) bool { return e.str("origin") == "authored" }

// textLanguages are the languages of the project's texts, the first one being
// the main language (project.json → languages, `ru` when it has none).
func (m *Model) textLanguages() []string {
	var langs []string
	_ = json.Unmarshal(m.manifest.vals["languages"], &langs)
	if len(langs) == 0 {
		langs = []string{"ru"}
	}
	return langs
}

// textValue is the `v` of a provenance field of a text entry, "" when absent.
func textV(entry *object, field string) string {
	raw, ok := entry.vals[field]
	if !ok {
		return ""
	}
	var v struct {
		V string `json:"v"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return strings.TrimSpace(v.V)
}

// nameLocked is the name to show for e: an authored entity takes its text
// `name` in the main language, else in any other language of the project in
// the order of `languages`, else its id; any other entity its name in
// entities.json. The caller holds m.mu.
func (m *Model) nameLocked(e *object) string {
	if !isAuthored(e) {
		return e.str("name")
	}
	id := e.str("id")
	for _, lang := range m.textLanguages() {
		doc, err := m.loadText(lang)
		if err != nil || doc == nil {
			continue
		}
		if entry := textEntry(doc, id); entry != nil {
			if name := textV(entry, "name"); name != "" {
				return name
			}
		}
	}
	return id
}

// EntityNames maps every entity id to the name it is shown by.
func (m *Model) EntityNames() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	if r := m.registries["entity"]; r != nil {
		for _, e := range r.items {
			out[e.str("id")] = m.nameLocked(e)
		}
	}
	return out
}

// EntityName is the name an entity is shown by; its id when it is not there.
func (m *Model) EntityName(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.registries["entity"]; r != nil {
		if e := findByID(r.items, id); e != nil {
			return m.nameLocked(e)
		}
	}
	return id
}

// hasNameText: some language of the project has a `name` text for the entity.
// It reads the model as it is now, unsaved texts included; a language that has
// no catalogue yet counts as having none.
func (m *Model) hasNameText(id string) bool {
	for _, lang := range m.textLanguages() {
		doc, err := m.loadTextNoCache(lang)
		if err != nil || doc == nil {
			continue
		}
		if entry := textEntry(doc, id); entry != nil && textV(entry, "name") != "" {
			return true
		}
	}
	return false
}
