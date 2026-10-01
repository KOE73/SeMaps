package migrate

// The final shape of a project's registry (ADR_20260930-4, ADR_20260930-5),
// reached from contract 3 and from an already-5 project of the earlier form
// alike; it is decided by the content, so a second run finds nothing to do:
//
//   - an entity's `codeRef` + `symbol` become `code: [{lang, ref, symbol}]`;
//   - a relation's top-level `via` and `evidence[].codeRef` become `evidence:
//     [{lang, ref, symbol, via}]`;
//   - the `name` of an authored entity leaves entities.json and becomes a `name`
//     text under its id.
//
// The language of a realization is the language of the project's extractor in the
// .semaps file when exactly one language covers the project; there is no
// guessing otherwise — the run stops and asks.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// realizationLang is the language of the project's realizations: the one
// language of the extractors that write into the project.
func (m *projectMigration) realizationLang() (string, bool) {
	langs := map[string]bool{}
	for _, x := range m.opt.Extractors {
		if x.Project == m.id && x.Language != "" {
			langs[x.Language] = true
		}
	}
	if len(langs) != 1 {
		return "", false
	}
	for l := range langs {
		return l, true
	}
	return "", false
}

// replaceKey renames a key keeping its position and gives it a new value.
func (o *obj) replaceKey(oldK, newK string, v any) {
	for i, x := range o.keys {
		if x == oldK {
			o.keys[i] = newK
			break
		}
	}
	delete(o.m, oldK)
	o.m[newK] = v
}

func (m *projectMigration) finalShape() error {
	lang, langOK := m.realizationLang()
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
			if err := m.entityCode(o, lang, langOK); err != nil {
				return err
			}
			if err := m.entityName(o); err != nil {
				return err
			}
		}
	}
	m.dropCodeNameTexts()
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
			if err := m.relationEvidence(o, lang, langOK); err != nil {
				return err
			}
		}
	}
	if len(m.needLang) > 0 {
		head := m.needLang
		more := ""
		if len(head) > 5 {
			head, more = head[:5], fmt.Sprintf(" и ещё %d", len(m.needLang)-5)
		}
		var have []string
		for _, x := range m.opt.Extractors {
			if x.Project == m.id {
				have = append(have, x.Language)
			}
		}
		sort.Strings(have)
		found := "в файле .semaps нет извлекателя этого проекта"
		if len(have) > 0 {
			found = "в файле .semaps у этого проекта извлекатели языков: " + strings.Join(have, ", ")
		}
		return fmt.Errorf("проект %s: язык реализаций не определён — %s, а нужен ровно один язык: %d записей со `symbol` (%s%s). "+
			"Язык не угадывается: добавьте в .semaps извлекатель проекта с его языком (`extractors: - id: … language: … project: %s`) и повторите; ничего не записано",
			m.id, found, len(m.needLang), strings.Join(head, "; "), more, m.id)
	}
	return nil
}

// entityCode turns `codeRef` and `symbol` into one entry of `code[]`.
func (m *projectMigration) entityCode(o *obj, lang string, langOK bool) error {
	if !o.has("codeRef") && !o.has("symbol") {
		return nil
	}
	id, _ := asStr(o.get("id"))
	ref, _, err := strField(o, "codeRef")
	if err != nil {
		return ferr(m.ents.rel, "entities["+id+"].codeRef", "%v", err)
	}
	symbol, _, err := strField(o, "symbol")
	if err != nil {
		return ferr(m.ents.rel, "entities["+id+"].symbol", "%v", err)
	}
	if o.has("code") {
		return ferr(m.ents.rel, "entities["+id+"].code", "и code, и codeRef/symbol сразу: какая форма верна, не угадывается")
	}
	var entry *obj
	switch {
	case symbol != "":
		if !langOK {
			m.needLang = append(m.needLang, id+": symbol "+symbol)
			return nil
		}
		entry = newObj()
		entry.set("lang", jsonStr(lang))
		if ref != "" {
			entry.set("ref", jsonStr(ref))
		}
		entry.set("symbol", jsonStr(symbol))
	case ref != "":
		entry = newObj()
		entry.set("ref", jsonStr(ref))
	}
	anchor := "symbol"
	if o.has("codeRef") {
		anchor = "codeRef"
	}
	if entry != nil {
		o.replaceKey(anchor, "code", []any{entry})
		m.rep.CodeEntries++
	}
	o.del("codeRef")
	o.del("symbol")
	m.ents.dirty = true
	return nil
}

// dropCodeNameTexts removes the `name` text of an entity from code that only
// repeats the name in entities.json, word for word: the name of such an entity is
// the code's and is not read from a text (ADR_20260930-5), so the copy is dead. A
// text that differs is kept — it says something the registry does not — and is
// listed by `semaps check` («лишнее имя»).
func (m *projectMigration) dropCodeNameTexts() {
	if m.ents == nil {
		return
	}
	arr, _ := m.ents.array("entities")
	names := map[string]string{}
	for _, it := range arr {
		o, ok := asObj(it)
		if !ok {
			continue
		}
		origin, _, _ := strField(o, "origin")
		id, _ := asStr(o.get("id"))
		if name, ok := asStr(o.get("name")); ok && id != "" && origin != "authored" {
			names[id] = name
		}
	}
	for _, td := range m.texts {
		if td.entries == nil {
			continue
		}
		for _, id := range append([]string(nil), td.entries.keys...) {
			name, ok := names[id]
			en, isObj := asObj(td.entries.get(id))
			if !ok || !isObj || !en.has("name") || valueText(en.get("name")) != name {
				continue
			}
			en.del("name")
			if len(en.keys) == 0 {
				td.entries.del(id)
			}
			td.changed, td.f.dirty = true, true
			m.rep.CodeNameTexts++
		}
	}
}

// entriesOf is the `entries` object of a text catalogue, made when it has none.
func (td *textData) entriesOf() *obj {
	if td.entries == nil {
		td.entries = newObj()
		td.f.root.set("entries", td.entries)
	}
	return td.entries
}

// entityName moves the `name` of an authored entity into its text: the main
// language of the project, provenance authored, at = now. The id stays.
func (m *projectMigration) entityName(o *obj) error {
	origin, _, _ := strField(o, "origin")
	nv, has := o.lookup("name")
	if origin != "authored" || !has {
		return nil
	}
	id, _ := asStr(o.get("id"))
	name, ok := asStr(nv)
	if !ok {
		return ferr(m.ents.rel, "entities["+id+"].name", "ожидалась строка")
	}
	o.del("name")
	m.ents.dirty = true
	if strings.TrimSpace(name) == "" {
		m.rep.Notes = append(m.rep.Notes, fmt.Sprintf("сущность %s: пустое name убрано, имени в текстах нет — задайте его", id))
		return nil
	}
	td := m.ensureText(m.langs[0])
	entries := td.entriesOf()
	var entry *obj
	if ev, ok := entries.lookup(id); ok {
		if entry, ok = asObj(ev); !ok {
			return ferr(td.f.rel, "entries."+id, "ожидался объект")
		}
	}
	if entry != nil && entry.has("name") {
		m.rep.Notes = append(m.rep.Notes,
			fmt.Sprintf("сущность %s: имя уже есть в text.%s.json, name «%s» из entities.json отброшено", id, td.lang, name))
		return nil
	}
	if entry == nil {
		entry = newObj()
		entries.set(id, entry)
	}
	entry.setFirst("name", provenance(name))
	td.changed, td.f.dirty = true, true
	m.rep.NamesMoved++
	return nil
}

// relationEvidence turns the top-level `via` and the `codeRef` of the evidence
// entries into the entries of `evidence[]` of the current shape.
func (m *projectMigration) relationEvidence(r *obj, lang string, langOK bool) error {
	id, _ := asStr(r.get("id"))
	viaV, hasVia := r.lookup("via")
	evV, hasEv := r.lookup("evidence")
	var old []any
	oldShape := false
	if hasEv {
		arr, ok := asArr(evV)
		if !ok {
			return ferr(m.rels.rel, "relations["+id+"].evidence", "ожидался массив")
		}
		old = arr
		for i, it := range arr {
			e, ok := asObj(it)
			if !ok {
				return ferr(m.rels.rel, fmt.Sprintf("relations[%s].evidence[%d]", id, i), "ожидался объект")
			}
			// an entry that names a symbol has to name its language too
			if s, _ := asStr(e.get("symbol")); e.has("codeRef") || e.has("line") || (s != "" && !e.has("lang")) {
				oldShape = true
			}
		}
	}
	if !hasVia && !oldShape {
		return nil
	}
	list := make([]any, 0, len(old)+1)
	for i, it := range old {
		e := it.(*obj)
		fld := fmt.Sprintf("relations[%s].evidence[%d]", id, i)
		langV, _, err := strField(e, "lang")
		if err != nil {
			return ferr(m.rels.rel, fld+".lang", "%v", err)
		}
		ref, _, err := strField(e, "ref")
		if err != nil {
			return ferr(m.rels.rel, fld+".ref", "%v", err)
		}
		codeRef, _, err := strField(e, "codeRef")
		if err != nil {
			return ferr(m.rels.rel, fld+".codeRef", "%v", err)
		}
		symbol, _, err := strField(e, "symbol")
		if err != nil {
			return ferr(m.rels.rel, fld+".symbol", "%v", err)
		}
		if symbol != "" && langV == "" {
			if !langOK {
				m.needLang = append(m.needLang, id+": evidence symbol "+symbol)
				return nil
			}
			langV = lang
		}
		if ref == "" {
			ref = codeRef
		}
		if line, ok := e.get("line").(json.RawMessage); ok && ref != "" && !strings.ContainsAny(ref, "#:") {
			ref += ":" + string(line)
		}
		ne := newObj()
		if langV != "" {
			ne.set("lang", jsonStr(langV))
		}
		if ref != "" {
			ne.set("ref", jsonStr(ref))
		}
		if symbol != "" {
			ne.set("symbol", jsonStr(symbol))
		}
		for _, k := range e.keys {
			switch k {
			case "lang", "ref", "codeRef", "symbol", "line":
			default:
				ne.set(k, deepCopy(e.m[k]))
			}
		}
		list = append(list, ne)
	}
	if hasVia && !isNull(viaV) {
		target := -1
		for i, it := range list {
			if target < 0 {
				target = i
			}
			if s, _ := asStr(it.(*obj).get("symbol")); s != "" {
				target = i
				break
			}
		}
		if target < 0 {
			list = append(list, newObj())
			target = 0
		}
		te := list[target].(*obj)
		if te.has("via") {
			return ferr(m.rels.rel, "relations["+id+"].via", "и via на связи, и via в evidence: какое верно, не угадывается")
		}
		// via goes after the file and the symbol, before the keys of no meaning here
		te.set("via", deepCopy(viaV))
	}
	switch {
	case len(list) == 0: // no realization is no `evidence`
		r.del("via")
		r.del("evidence")
	case hasEv:
		r.set("evidence", list)
		r.del("via")
	default:
		r.replaceKey("via", "evidence", list)
	}
	m.rels.dirty = true
	m.rep.EvidenceConverted++
	return nil
}

// dropStyleRefs takes the styles dropped from the workspace's styles.json off
// the placements and the edge entries of the project's views: a style that is
// gone must not stay named. Every removal is reported.
func (m *projectMigration) dropStyleRefs(dropped map[string]bool) {
	if len(dropped) == 0 {
		return
	}
	type ref struct{ view, what string }
	refs := map[string][]ref{}
	for _, v := range m.views {
		viewID, _ := asStr(v.f.root.get("id"))
		if viewID == "" {
			viewID = v.f.rel
		}
		for _, list := range []struct{ key, what, id string }{{"placements", "размещение", "entity"}, {"edges", "связь", "id"}} {
			lv, ok := v.f.root.lookup(list.key)
			if !ok {
				continue
			}
			arr, ok := asArr(lv)
			if !ok {
				continue
			}
			for _, it := range arr {
				p, ok := asObj(it)
				if !ok {
					continue
				}
				sid, ok := asStr(p.get("styleId"))
				if !ok || !dropped[sid] {
					continue
				}
				what, _ := asStr(p.get(list.id))
				p.del("styleId")
				v.f.dirty = true
				refs[sid] = append(refs[sid], ref{viewID, list.what + " " + what})
			}
		}
	}
	ids := make([]string, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		byView := map[string][]string{}
		var order []string
		for _, r := range refs[id] {
			if _, ok := byView[r.view]; !ok {
				order = append(order, r.view)
			}
			byView[r.view] = append(byView[r.view], r.what)
		}
		var parts []string
		for _, v := range order {
			list := byView[v]
			shown := list
			tail := ""
			if len(shown) > 4 {
				shown, tail = shown[:4], fmt.Sprintf(", … (+%d)", len(list)-4)
			}
			parts = append(parts, fmt.Sprintf("%s: %s%s", v, strings.Join(shown, ", "), tail))
		}
		m.rep.DroppedStyleRefs = append(m.rep.DroppedStyleRefs,
			fmt.Sprintf("«%s» — %d: %s", id, len(refs[id]), strings.Join(parts, "; ")))
	}
}
