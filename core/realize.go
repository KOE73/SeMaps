package core

// The shape of code realizations (ADR_20260927-2, ADR_20260930-4). An entity
// is the intent; its optional `code[]` lists where it is realized in code, and
// a relation's optional `evidence[]` lists the same for the relation. Today
// sync reconciles ONE realization per entity per extractor run — the entry of
// the facts' language — and never binds a second language; these helpers read
// and write the lists as ordered objects, so keys sync does not know survive.

import (
	"bytes"
	"encoding/json"
	"strings"
)

// entries is o[key] as a list of ordered objects; nil when absent or not a list.
func entries(o *object, key string) []*object {
	raw, ok := o.vals[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var list []*object
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	return list
}

// setEntries writes the list back; an empty list removes the key — no code is
// no `code`, not `code: []`.
func setEntries(o *object, key string, list []*object) {
	if len(list) == 0 {
		o.del(key)
		return
	}
	o.set(key, list)
}

// entryFor is the index of the entry bound to a symbol in this language, -1
// when there is none.
func entryFor(list []*object, lang string) int {
	for i, c := range list {
		if c.str("lang") == lang && c.str("symbol") != "" {
			return i
		}
	}
	return -1
}

// hasBound: some entry is bound to a symbol, in any language.
func hasBound(list []*object) bool {
	for _, c := range list {
		if c.str("symbol") != "" {
			return true
		}
	}
	return false
}

// firstRef is the `ref` of the first entry that has one ("" when none): the
// file the entity is shown by when no language is asked for.
func firstRef(list []*object) string {
	for _, c := range list {
		if r := c.str("ref"); r != "" {
			return r
		}
	}
	return ""
}

// entityRef is the file of the entity's realization in this language, else the
// first file it has.
func entityRef(e *object, lang string) string {
	list := entries(e, "code")
	if k := entryFor(list, lang); k >= 0 {
		if r := list[k].str("ref"); r != "" {
			return r
		}
	}
	return firstRef(list)
}

// entrySymbols are the symbols of all the entity's realizations.
func entrySymbols(e *object) []string {
	var out []string
	for _, c := range entries(e, "code") {
		if s := c.str("symbol"); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// codeEntry is the realization of a symbol in a language: lang, ref, symbol in
// this order. An external symbol has no file (ADR_20260927-4), so no `ref`.
func codeEntry(lang string, s Symbol) *object {
	c := newObject()
	setCodeEntry(c, lang, s)
	return c
}

// setCodeEntry points the entry at the symbol. Keys it does not own (`status`,
// whatever a human wrote) are kept; lang, ref, symbol come first.
func setCodeEntry(c *object, lang string, s Symbol) {
	rest := newObject()
	for _, k := range c.keys {
		if k != "lang" && k != "ref" && k != "symbol" {
			rest.keys, rest.vals[k] = append(rest.keys, k), c.vals[k]
		}
	}
	c.keys, c.vals = nil, map[string]json.RawMessage{}
	c.set("lang", lang)
	if s.File != "" {
		c.set("ref", s.File)
	}
	c.set("symbol", s.ID)
	for _, k := range rest.keys {
		c.keys, c.vals[k] = append(c.keys, k), rest.vals[k]
	}
}

// evidenceEntry is the realization of an edge in one language: where it is
// (the file and symbol of its `from` end) and, for a member relation, through
// which member. `native` is not written (ADR_20260930-3 §4).
func evidenceEntry(lang string, from Symbol, via *Via) *object {
	c := codeEntry(lang, from)
	if via != nil {
		c.set("via", via)
	}
	return c
}

func evidenceVia(ev *object) *Via {
	raw, ok := ev.vals["via"]
	if !ok || len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		return nil
	}
	var v Via
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return &v
}

// relationVia is the member signature of a relation: the `via` of the first
// evidence entry that has one; nil for an organic relation.
func relationVia(r *object) *Via {
	for _, ev := range entries(r, "evidence") {
		if v := evidenceVia(ev); v != nil {
			return v
		}
	}
	return nil
}

// putVia sets the member signature in the relation's evidence: in the entry of
// this language, else in the first entry that has a via, else in a new entry.
// It reports whether the relation changed.
func putVia(r *object, lang string, from Symbol, via *Via) bool {
	list := entries(r, "evidence")
	k := entryFor(list, lang)
	if k < 0 {
		for i, c := range list {
			if evidenceVia(c) != nil {
				k = i
				break
			}
		}
	}
	if k < 0 {
		setEntries(r, "evidence", append(list, evidenceEntry(lang, from, via)))
		return true
	}
	old, _ := list[k].MarshalJSON()
	list[k].set("via", via)
	now, _ := list[k].MarshalJSON()
	if compactJSON(old) == compactJSON(now) {
		return false
	}
	setEntries(r, "evidence", list)
	return true
}

// oldShape names what a record still carries from before ADR_20260930-4 and
// ADR_20260930-5, "" if nothing. The loader knows only the current shape and
// says so instead of guessing (ADR_20260927-3); `semaps migrate` converts.
func oldShape(kind string, o *object) string {
	switch kind {
	case "entity":
		for _, key := range []string{"codeRef", "symbol"} {
			if has(o, key) {
				return "`" + key + "` верхнего уровня — реализации в коде лежат в code[]"
			}
		}
		if o.str("origin") == "authored" && has(o, "name") {
			return "`name` у authored-сущности — её имя это текст под её id"
		}
	case "relation":
		if has(o, "via") {
			return "`via` верхнего уровня — подпись члена лежит в evidence[]"
		}
		for _, ev := range entries(o, "evidence") {
			for _, key := range []string{"codeRef", "line"} {
				if has(ev, key) {
					return "`evidence[]." + key + "` — ссылка на файл называется ref"
				}
			}
		}
	}
	return ""
}

// checkCode: a realization has a file, a symbol or (evidence) a member
// signature; `lang` and `symbol` come together; one language once; `status` is
// `missing` or left out (CONTRACT §3, §4).
func checkCode(what, id string, list []*object) error {
	field := "code[]"
	if what == "relation" {
		field = "evidence[]"
	}
	langs := map[string]bool{}
	for _, c := range list {
		switch {
		case c.str("ref") == "" && c.str("symbol") == "" && !(what == "relation" && has(c, "via")):
			return refuse("%s %s: %s entry has neither ref nor symbol (CONTRACT §3)", what, id, field)
		case (c.str("lang") == "") != (c.str("symbol") == ""):
			return refuse("%s %s: %s entry needs lang and symbol together (CONTRACT §3)", what, id, field)
		case c.str("lang") != "" && langs[c.str("lang")]:
			return refuse("%s %s: two %s entries of language %s (CONTRACT §3)", what, id, field, c.str("lang"))
		case has(c, "via") && what == "entity":
			return refuse("entity %s: via belongs to a relation's evidence, not to code[] (CONTRACT §3)", id)
		case !oneOf(c, "status", "missing"):
			return refuse("%s %s: %s entry status %q: missing or nothing (CONTRACT §3)", what, id, field, c.str("status"))
		}
		if has(c, "via") {
			if _, err := child(c, "via"); err != nil {
				return refuse("%s %s: %s via: an object (CONTRACT §4)", what, id, field)
			}
		}
		if c.str("lang") != "" {
			langs[c.str("lang")] = true
		}
	}
	return nil
}

// refFile is a `ref` without its `#…` or `:…` anchor.
func refFile(ref string) string {
	if i := strings.IndexAny(ref, "#:"); i >= 0 {
		return ref[:i]
	}
	return ref
}
