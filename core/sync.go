package core

// Sync reconciles one project's registry (entities.json, relations.json,
// relation-types.json) with extractor facts. The rules are normative in
// docs/EXTRACTOR.md §5; why they are what they are:
// ADR_20260923-9_core_sync-symbol-mapping-and-containment.
//
// Sync never writes texts, views or containers, never touches an `authored`
// record, never deletes and never changes an id it has handed out.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// SyncOptions selects the project and how far sync may go.
type SyncOptions struct {
	// Project is the folder name under <workspace>/projects. Empty: the only
	// project there is; several projects without a choice is a UsageError.
	Project string
	// DryRun reports without writing anything.
	DryRun bool
	// NoRenames treats every rename candidate as what it literally is — one
	// entity gone and one new — instead of holding both for a human decision.
	NoRenames bool
}

// UsageError is a mistake in how sync was called, not in the data.
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

// SyncReport is what sync found and, unless DryRun, did.
type SyncReport struct {
	Project  string `json:"project"`
	Language string `json:"language"`
	Symbols  int    `json:"symbols"` // after the project's sources.include filter
	Edges    int    `json:"edges"`
	DryRun   bool   `json:"dryRun"`

	Broken    []string `json:"broken"`    // сломано: the registry contradicts itself; nothing is written
	Ambiguous []string `json:"ambiguous"` // неоднозначно: an entity without `symbol` fits several symbols, or the reverse
	Renames   []string `json:"renames"`   // переименование?: gone and new in one file, same kind
	Gone      []string `json:"gone"`      // лишнее: marked `status: missing`
	Added     []string `json:"added"`     // не хватает: new entities, relations, relation types
	Changed   []string `json:"changed"`   // изменилось: fields updated, entities adopted, returned from missing

	Written []string `json:"written"` // files written, workspace-relative
}

// Empty is true when the registry already matches the code.
func (r *SyncReport) Empty() bool {
	return len(r.Broken)+len(r.Ambiguous)+len(r.Renames)+len(r.Gone)+len(r.Added)+len(r.Changed) == 0
}

// ExitCode: 1 when the registry contradicts itself or a decision is left to a
// human; with DryRun also when anything would change. 0 otherwise.
func (r *SyncReport) ExitCode() int {
	switch {
	case len(r.Broken) > 0:
		return 1
	case r.DryRun && !r.Empty():
		return 1
	case len(r.Ambiguous)+len(r.Renames) > 0:
		return 1
	}
	return 0
}

// Print writes the report in the style of `semaps check`: groups, at most 15
// lines each.
func (r *SyncReport) Print(w io.Writer) {
	fmt.Fprintf(w, "Сверка проекта %s с фактами %s: %d символов, %d рёбер\n\n", r.Project, r.Language, r.Symbols, r.Edges)
	groups := []struct {
		title string
		items []string
	}{
		{"сломано", r.Broken},
		{"неоднозначно", r.Ambiguous},
		{"переименование?", r.Renames},
		{"лишнее", r.Gone},
		{"не хватает", r.Added},
		{"изменилось", r.Changed},
	}
	for _, g := range groups {
		if len(g.items) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s (%d)\n", g.title, len(g.items))
		for i, item := range g.items {
			if i == 15 {
				fmt.Fprintf(w, "  ... и ещё %d\n", len(g.items)-15)
				break
			}
			fmt.Fprintf(w, "  %s\n", item)
		}
		fmt.Fprintln(w)
	}
	switch {
	case len(r.Broken) > 0:
		fmt.Fprintln(w, "Ничего не записано: реестр противоречит себе.")
	case r.Empty():
		fmt.Fprintln(w, "Реестр совпадает с кодом.")
	case r.DryRun:
		fmt.Fprintln(w, "--dry-run: ничего не записано.")
	case len(r.Written) > 0:
		fmt.Fprintf(w, "Записано: %s\n", strings.Join(r.Written, ", "))
	}
	if len(r.Renames) > 0 {
		fmt.Fprintln(w, "Переименование:")
		fmt.Fprintln(w, "  Сущность: поставьте старой сущности `symbol` нового символа (и `name`, если имя сменилось).")
		fmt.Fprintln(w, "  Членская связь: установите `via.member` старой связи на новое имя члена и повторите сверку.")
		fmt.Fprintln(w, "  Не переименование — повторите с --no-renames.")
	}
}

// structuralTypes are the edge kinds sync writes as relations: all organic ones.
// Member relations (holds, uses) are not listed here; they are created per-member.
// Which ones a view shows is the view's decision (CONTRACT.md §8.5).
var structuralTypes = []string{"extends", "implements", "contains", "depends"}

// Sync reconciles <workspace>/projects/<project> with facts.
func Sync(workspace string, facts *Facts, opt SyncOptions) (*SyncReport, error) {
	if err := facts.Validate(); err != nil {
		return nil, err
	}
	project, err := pickProject(workspace, opt.Project)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(workspace, "projects", project)
	var manifest struct {
		Sources struct {
			Include []string `json:"include"`
		} `json:"sources"`
	}
	if err := readJSON(filepath.Join(dir, "project.json"), &manifest); err != nil {
		return nil, fmt.Errorf("project.json: %w", err)
	}
	rep := &SyncReport{Project: project, Language: facts.Language, DryRun: opt.DryRun}

	// ------------------------------------------------ facts of this project
	symbols := map[string]Symbol{}
	var order []string // symbol ids, in the facts' own (sorted) order
	for _, s := range facts.Symbols {
		if included(s.File, manifest.Sources.Include) {
			symbols[s.ID] = s
			order = append(order, s.ID)
		}
	}
	var edges []Edge
	for _, e := range facts.Edges {
		if _, ok := symbols[e.From]; !ok {
			continue
		}
		if _, ok := symbols[e.To]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	rep.Symbols, rep.Edges = len(order), len(edges)

	// covered holds which edge kinds the facts declare they cover (for missing marking)
	covered := map[string]bool{}
	for _, k := range facts.EdgeKinds {
		covered[k] = true
	}

	withVersion := func() *object {
		o := newObject()
		o.set("contractVersion", 3)
		return o
	}
	ents, err := loadRegistry(dir, "entities.json", "entities", newObject)
	if err != nil {
		return nil, err
	}
	rels, err := loadRegistry(dir, "relations.json", "relations", withVersion)
	if err != nil {
		return nil, err
	}
	types, err := loadRegistry(dir, "relation-types.json", "relationTypes", withVersion)
	if err != nil {
		return nil, err
	}

	// ------------------------------------------------ registry must be sane
	taken := map[string]bool{}   // every entity id, any origin
	bySymbol := map[string]int{} // symbol -> index, non-authored only
	for i, e := range ents.items {
		id := e.str("id")
		if id == "" {
			rep.Broken = append(rep.Broken, fmt.Sprintf("entities.json[%d]: нет id", i))
			continue
		}
		if taken[id] {
			rep.Broken = append(rep.Broken, fmt.Sprintf("%s: id повторяется в entities.json", id))
			continue
		}
		taken[id] = true
		if e.str("origin") == "authored" {
			continue
		}
		if s := e.str("symbol"); s != "" {
			if j, dup := bySymbol[s]; dup {
				rep.Broken = append(rep.Broken, fmt.Sprintf("%s и %s: у обеих symbol %s", ents.items[j].str("id"), id, s))
				continue
			}
			bySymbol[s] = i
		}
	}
	relIDs := map[string]bool{}
	for i, r := range rels.items {
		id := r.str("id")
		if id == "" {
			rep.Broken = append(rep.Broken, fmt.Sprintf("relations.json[%d]: нет id", i))
		} else if relIDs[id] {
			rep.Broken = append(rep.Broken, fmt.Sprintf("%s: id повторяется в relations.json", id))
		}
		relIDs[id] = true
	}
	if len(rep.Broken) > 0 {
		return rep, nil // nothing is decided on a registry that contradicts itself
	}

	// ------------------------------------------------ match symbols to entities
	match := map[string]int{}   // symbol -> entity index
	matched := map[int]string{} // entity index -> symbol
	for s, i := range bySymbol {
		if _, ok := symbols[s]; ok {
			match[s], matched[i] = i, s
		}
	}
	held := map[int]bool{}       // entities waiting for a human
	heldSym := map[string]bool{} // symbols waiting for a human
	adopt(ents.items, symbols, order, match, matched, held, heldSym, rep)
	if !opt.NoRenames {
		renames(ents.items, symbols, order, match, matched, held, heldSym, rep)
	}

	// ------------------------------------------------ apply to entities
	inCode := map[string]bool{} // entity ids backed by a symbol after this run
	for _, sid := range order {
		i, ok := match[sid]
		if !ok {
			continue
		}
		e := ents.items[i]
		inCode[e.str("id")] = true
		if msg := updateEntity(e, symbols[sid]); msg != "" {
			rep.Changed = append(rep.Changed, msg)
			ents.dirty = true
		}
	}
	gone := map[string]bool{}
	for i, e := range ents.items {
		if !managed(e) || held[i] {
			continue
		}
		if _, ok := matched[i]; ok {
			continue
		}
		gone[e.str("id")] = true
		if e.str("status") != "missing" {
			e.set("status", "missing")
			ents.dirty = true
			rep.Gone = append(rep.Gone, fmt.Sprintf("%s (%s): нет в коде", e.str("id"), e.str("name")))
		}
	}
	for _, sid := range order {
		if _, ok := match[sid]; ok || heldSym[sid] {
			continue
		}
		s := symbols[sid]
		id := mint(newEntityID(s), taken)
		e := newObject()
		e.set("id", id)
		e.set("name", s.Name)
		e.set("kind", s.NativeKind)
		e.set("origin", "code")
		e.set("status", "present")
		if s.Namespace != "" {
			e.set("namespace", s.Namespace)
		}
		e.set("codeRef", s.File)
		e.set("symbol", s.ID)
		if len(s.Members) > 0 {
			e.vals["members"] = s.Members
			e.keys = append(e.keys, "members")
		}
		ents.items = append(ents.items, e)
		ents.dirty = true
		match[sid] = len(ents.items) - 1
		inCode[id] = true
		rep.Added = append(rep.Added, fmt.Sprintf("%s ← %s (%s)", id, sid, s.NativeKind))
	}

	// ------------------------------------------------ relations
	structural := setOf(structuralTypes)
	byKey := map[string][]int{} // relation key -> relation indices
	for i, r := range rels.items {
		key := relationKey(r)
		byKey[key] = append(byKey[key], i)
	}
	confirmed := map[int]bool{}

	for _, edge := range edges {
		fi, ok1 := match[edge.From]
		ti, ok2 := match[edge.To]
		if !ok1 || !ok2 {
			continue // an end is waiting for a human
		}
		from, to := ents.items[fi].str("id"), ents.items[ti].str("id")
		fromSym := symbols[edge.From]

		// Organic edges and member relations are handled differently
		if edge.Kind == "holds" || edge.Kind == "uses" {
			// Member relation: one edge per member occurrence
			if edge.Via == nil {
				// Skip member edges without via information
				continue
			}
			key := memberRelationKey(from, to, edge.Via)
			relType := deriveRelationType(edge.Kind, edge.Via)
			existing := byKey[key]
			if len(existing) > 0 {
				for _, ri := range existing {
					r := rels.items[ri]
					if r.str("origin") == "authored" {
						continue
					}
					confirmed[ri] = true
					var changes []string
					if r.str("origin") != "code" {
						r.set("origin", "code")
						changes = append(changes, "origin")
					}
					if r.str("type") != relType {
						r.set("type", relType)
						changes = append(changes, "type")
					}
					if r.str("status") == "missing" {
						r.set("status", "present")
						changes = append(changes, "status")
					}
					// Update via
					existingViaJSON, hasVia := r.vals["via"]
					newViaJSON := encodeJSON(edge.Via)
					if !hasVia || compactJSON(existingViaJSON) != compactJSON(newViaJSON) {
						r.set("via", edge.Via)
						if hasVia {
							changes = append(changes, "via")
						}
					}
					if len(changes) > 0 {
						rels.dirty = true
						rep.Changed = append(rep.Changed, fmt.Sprintf("%s: %s", r.str("id"), strings.Join(changes, ", ")))
					}
				}
				continue
			}
			// New member relation
			id := mintMemberRelationID(from, to, edge.Via, relIDs)
			r := newObject()
			r.set("id", id)
			r.set("from", from)
			r.set("to", to)
			r.set("type", relType)
			r.set("origin", "code")
			r.set("status", "present")
			r.set("via", edge.Via)
			r.set("evidence", []map[string]string{{"codeRef": fromSym.File, "symbol": fromSym.ID}})
			rels.items = append(rels.items, r)
			rels.dirty = true
			confirmed[len(rels.items)-1] = true
			byKey[key] = append(byKey[key], len(rels.items)-1)
			rep.Added = append(rep.Added, id)
		} else {
			// Organic edge (extends, implements, contains, depends)
			key := triple(from, to, edge.Kind)
			existing := byKey[key]
			if len(existing) > 0 {
				for _, ri := range existing {
					r := rels.items[ri]
					if r.str("origin") == "authored" {
						continue
					}
					confirmed[ri] = true
					var changes []string
					if r.str("origin") != "code" {
						r.set("origin", "code")
						changes = append(changes, "origin")
					}
					if r.str("status") == "missing" {
						r.set("status", "present")
						changes = append(changes, "status")
					}
					if len(changes) > 0 {
						rels.dirty = true
						rep.Changed = append(rep.Changed, fmt.Sprintf("%s: %s", r.str("id"), strings.Join(changes, ", ")))
					}
				}
				continue
			}
			id := mint("r_"+strings.TrimPrefix(from, "e_")+"_"+strings.TrimPrefix(to, "e_")+"_"+edge.Kind, relIDs)
			r := newObject()
			r.set("id", id)
			r.set("from", from)
			r.set("to", to)
			r.set("type", edge.Kind)
			r.set("origin", "code")
			r.set("status", "present")
			r.set("evidence", []map[string]string{{"codeRef": fromSym.File, "symbol": fromSym.ID}})
			rels.items = append(rels.items, r)
			rels.dirty = true
			confirmed[len(rels.items)-1] = true
			byKey[key] = append(byKey[key], len(rels.items)-1)
			rep.Added = append(rep.Added, id)
		}
	}

	// Detect member relation rename candidates: same (from, to, path, type) but different member
	memberRenameHolds := map[int]bool{} // relations held due to rename candidate
	if !opt.NoRenames {
		type memberKey struct {
			from     string
			to       string
			path     string
			relType  string // include type to distinguish holds from uses
		}
		missingMembers := make(map[memberKey][]int)   // key -> missing relation indices
		newMembers := make(map[memberKey][]string)    // key -> new member names
		confirmedMembers := make(map[memberKey]bool)  // key -> any confirmed

		// Collect missing member relations
		for i, r := range rels.items {
			if confirmed[i] || r.str("origin") != "code" {
				continue
			}
			relType := relationType(r)
			if !(strings.HasPrefix(relType, "holds") || relType == "uses" || relType == "injects") {
				continue // not a member relation
			}
			from, to := r.str("from"), r.str("to")
			if !(inCode[from] || gone[from]) || !(inCode[to] || gone[to]) {
				continue
			}
			// This is a missing member relation
			via, hasVia := r.vals["via"]
			if !hasVia || len(via) == 0 || string(via) == "null" {
				continue
			}
			var v Via
			if err := json.Unmarshal(via, &v); err != nil {
				continue
			}
			path := strings.Join(v.Path, ",")
			key := memberKey{from, to, path, relType}
			missingMembers[key] = append(missingMembers[key], i)
		}

		// Collect new confirmed member relations
		for i, r := range rels.items {
			if !confirmed[i] || r.str("origin") != "code" {
				continue
			}
			relType := relationType(r)
			if !(strings.HasPrefix(relType, "holds") || relType == "uses" || relType == "injects") {
				continue
			}
			from, to := r.str("from"), r.str("to")
			via, hasVia := r.vals["via"]
			if !hasVia || len(via) == 0 || string(via) == "null" {
				continue
			}
			var v Via
			if err := json.Unmarshal(via, &v); err != nil {
				continue
			}
			path := strings.Join(v.Path, ",")
			key := memberKey{from, to, path, relType}
			confirmedMembers[key] = true
			newMembers[key] = append(newMembers[key], v.Member)
		}

		// Report rename candidates
		for key, missingIndices := range missingMembers {
			if confirmedMembers[key] && len(newMembers[key]) > 0 {
				// Potential rename: same (from, to, path, type) but different member name
				for _, mi := range missingIndices {
					r := rels.items[mi]
					via, _ := r.vals["via"]
					var v Via
					json.Unmarshal(via, &v)
					memberRenameHolds[mi] = true
					oldMember := v.Member
					for _, newMember := range newMembers[key] {
						if oldMember != newMember {
							rep.Renames = append(rep.Renames, fmt.Sprintf("%s (%s) → %s", r.str("id"), oldMember, newMember))
							break
						}
					}
				}
			}
		}
	}

	// Mark relations as missing if they're not in the facts
	for i, r := range rels.items {
		if confirmed[i] || r.str("origin") != "code" || memberRenameHolds[i] {
			continue
		}
		relType := relationType(r)

		// Check if this relation type's kind is covered by the facts
		var isCovered bool
		if strings.HasPrefix(relType, "holds") {
			isCovered = covered["holds"]
		} else if relType == "injects" {
			// injects is covered if "injects" or "uses" are in edgeKinds
			isCovered = covered["injects"] || covered["uses"]
		} else if relType == "uses" {
			isCovered = covered["uses"]
		} else if structural[relType] {
			// organic relation: always marked as missing if not confirmed
			isCovered = true
		} else {
			continue
		}

		if !isCovered {
			continue // kind not covered by the facts, don't mark as missing
		}

		from, to := r.str("from"), r.str("to")
		// Only when sync can see both ends
		if !(inCode[from] || gone[from]) || !(inCode[to] || gone[to]) {
			continue
		}
		if r.str("status") != "missing" {
			r.set("status", "missing")
			rels.dirty = true
			rep.Gone = append(rep.Gone, fmt.Sprintf("%s: связи нет в коде", r.str("id")))
		}
	}

	// ------------------------------------------------ relation types
	declared := map[string]bool{}
	for _, t := range types.items {
		declared[t.str("id")] = true
	}
	used := map[string]bool{}
	for _, r := range rels.items {
		if r.str("origin") == "code" {
			used[relationType(r)] = true
		}
	}

	// Organic types
	for _, t := range structuralTypes {
		if used[t] && !declared[t] {
			o := newObject()
			o.set("id", t)
			o.set("origin", "code")
			o.set("visibility", "visible")
			types.items = append(types.items, o)
			types.dirty = true
			rep.Added = append(rep.Added, "relation-types.json: "+t)
		}
	}

	// Member relation types derived from holds/uses edges
	for _, typeID := range []string{
		"holds.one", "holds.optional", "holds.many", "holds.many.ro", "holds.keyed", "holds.keyed.ro",
		"holds.one.internal", "holds.optional.internal", "holds.many.internal", "holds.many.ro.internal",
		"holds.keyed.internal", "holds.keyed.ro.internal",
		"uses", "injects",
	} {
		if used[typeID] && !declared[typeID] {
			o := newObject()
			o.set("id", typeID)
			o.set("origin", "code")
			visibility := "visible"
			if strings.HasSuffix(typeID, ".internal") || typeID == "uses" || typeID == "injects" {
				visibility = "hidden"
			}
			o.set("visibility", visibility)
			types.items = append(types.items, o)
			types.dirty = true
			rep.Added = append(rep.Added, "relation-types.json: "+typeID)
		}
	}

	if opt.DryRun {
		return rep, nil
	}
	for _, reg := range []*registry{ents, rels, types} {
		if !reg.dirty {
			continue
		}
		if err := reg.save(); err != nil {
			return rep, err
		}
		rep.Written = append(rep.Written, path.Join("projects", project, reg.name))
	}
	return rep, nil
}

// managed: an entity sync answers for — `origin: code`, or adopted earlier.
// Authored entities and hand-made ones without origin that matched nothing
// are not sync's business.
func managed(e *object) bool {
	origin := e.str("origin")
	return origin != "authored" && (origin == "code" || e.str("symbol") != "")
}

// adopt matches entities made without sync (no `symbol`) to symbols, so that
// a first run keeps their ids instead of minting duplicates. Two rules, in
// order; each must be unique both ways, otherwise both sides are held:
//
//  1. same file (codeRef without its #/: anchor) and same name; several
//     symbols there (a TS file module and its class) are narrowed by kind;
//  2. same namespace, same name and the same kind family.
//
// Names are compared by baseName (no generic parameter list, no arity), kinds
// by kindFamily (`abstract-class` ~ `class`): a hand-written registry says
// `IRunner<in TIn, out TOut>` / `class` where the extractor says `IRunner` /
// `abstract-class`.
func adopt(items []*object, symbols map[string]Symbol, order []string,
	match map[string]int, matched map[int]string, held map[int]bool, heldSym map[string]bool, rep *SyncReport) {

	type rule struct {
		entityKey func(*object) string
		symbolKey func(Symbol) string
		narrow    bool
	}
	rules := []rule{
		{
			entityKey: func(e *object) string {
				if f := codeRefFile(e.str("codeRef")); f != "" {
					return f + "\x00" + baseName(e.str("name"))
				}
				return ""
			},
			symbolKey: func(s Symbol) string { return s.File + "\x00" + baseName(s.Name) },
			narrow:    true,
		},
		{
			entityKey: func(e *object) string {
				if e.str("kind") == "" {
					return ""
				}
				return e.str("namespace") + "\x00" + baseName(e.str("name")) + "\x00" + kindFamily(e.str("kind"))
			},
			symbolKey: func(s Symbol) string {
				return s.Namespace + "\x00" + baseName(s.Name) + "\x00" + kindFamily(s.NativeKind)
			},
		},
	}
	for _, r := range rules {
		free := map[string][]string{} // key -> unmatched symbols
		for _, sid := range order {
			if _, ok := match[sid]; ok || heldSym[sid] {
				continue
			}
			k := r.symbolKey(symbols[sid])
			free[k] = append(free[k], sid)
		}
		proposals := map[int][]string{}
		claims := map[string]int{}
		var candidates []int
		for i, e := range items {
			if e.str("origin") == "authored" || e.str("symbol") != "" || e.str("id") == "" || held[i] {
				continue
			}
			if _, ok := matched[i]; ok {
				continue
			}
			k := r.entityKey(e)
			if k == "" || len(free[k]) == 0 {
				continue
			}
			found := free[k]
			if r.narrow && len(found) > 1 {
				// Exact kind first, then the family; neither — leave all.
				for _, same := range []func(string) bool{
					func(nk string) bool { return strings.EqualFold(nk, e.str("kind")) },
					func(nk string) bool { return kindFamily(nk) == kindFamily(e.str("kind")) },
				} {
					var narrowed []string
					for _, sid := range found {
						if same(symbols[sid].NativeKind) {
							narrowed = append(narrowed, sid)
						}
					}
					if len(narrowed) > 0 {
						found = narrowed
						break
					}
				}
			}
			proposals[i] = found
			candidates = append(candidates, i)
			for _, sid := range found {
				claims[sid]++
			}
		}
		for _, i := range candidates {
			found := proposals[i]
			if len(found) == 1 && claims[found[0]] == 1 {
				match[found[0]], matched[i] = i, found[0]
				continue
			}
			held[i] = true
			for _, sid := range found {
				heldSym[sid] = true
			}
			rep.Ambiguous = append(rep.Ambiguous, fmt.Sprintf("%s (%s): подходят символы %s — поставьте `symbol` руками", items[i].str("id"), items[i].str("name"), strings.Join(found, ", ")))
		}
	}
}

// renames pairs entities about to go missing with new symbols of the same
// file and kind. Every such pair is only a candidate: held entities are
// neither marked missing, held symbols are not minted, until a human decides.
func renames(items []*object, symbols map[string]Symbol, order []string,
	match map[string]int, matched map[int]string, held map[int]bool, heldSym map[string]bool, rep *SyncReport) {

	type group struct {
		ents []int
		syms []string
	}
	groups := map[string]*group{}
	at := func(k string) *group {
		if groups[k] == nil {
			groups[k] = &group{}
		}
		return groups[k]
	}
	for i, e := range items {
		if !managed(e) || held[i] || e.str("status") == "missing" {
			continue
		}
		if _, ok := matched[i]; ok {
			continue
		}
		if f := codeRefFile(e.str("codeRef")); f != "" {
			g := at(f + "\x00" + kindFamily(e.str("kind")))
			g.ents = append(g.ents, i)
		}
	}
	for _, sid := range order {
		if _, ok := match[sid]; ok || heldSym[sid] {
			continue
		}
		s := symbols[sid]
		if g := groups[s.File+"\x00"+kindFamily(s.NativeKind)]; g != nil {
			g.syms = append(g.syms, sid)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		if len(g.ents) == 0 || len(g.syms) == 0 {
			continue
		}
		file := k[:strings.IndexByte(k, 0)]
		for _, i := range g.ents {
			held[i] = true
			rep.Renames = append(rep.Renames, fmt.Sprintf("%s (%s) → %s в %s", items[i].str("id"), items[i].str("name"), strings.Join(g.syms, " | "), file))
		}
		for _, sid := range g.syms {
			heldSym[sid] = true
		}
	}
}

// updateEntity brings a matched entity in line with its symbol. id, name and
// kind are the human's once the entity exists; namespace, codeRef, members
// and status follow the code. Returns a report line, or "" if nothing changed.
func updateEntity(e *object, s Symbol) string {
	var changes []string
	adopted := e.str("symbol") == ""
	if e.str("symbol") != s.ID {
		e.set("symbol", s.ID)
	}
	if e.str("origin") != "code" {
		e.set("origin", "code")
		changes = append(changes, "origin")
	}
	if e.str("namespace") != s.Namespace {
		if s.Namespace == "" {
			e.del("namespace")
		} else {
			e.set("namespace", s.Namespace)
		}
		changes = append(changes, "namespace")
	}
	if e.str("codeRef") != s.File {
		e.set("codeRef", s.File)
		changes = append(changes, "codeRef")
	}
	if len(s.Members) > 0 {
		if raw, ok := e.vals["members"]; !ok || compactJSON(raw) != compactJSON(s.Members) {
			if !ok {
				e.keys = append(e.keys, "members")
			}
			e.vals["members"] = s.Members
			changes = append(changes, "members")
		}
	}
	if e.str("status") == "missing" {
		e.set("status", "present")
		changes = append(changes, "вернулась")
	}
	switch {
	case adopted:
		msg := fmt.Sprintf("%s ← %s: принята", e.str("id"), s.ID)
		if len(changes) > 0 {
			msg += "; " + strings.Join(changes, ", ")
		}
		return msg
	case len(changes) > 0:
		return fmt.Sprintf("%s: %s", e.str("id"), strings.Join(changes, ", "))
	}
	return ""
}

func pickProject(workspace, id string) (string, error) {
	if id != "" {
		if !exists(filepath.Join(workspace, "projects", id, "project.json")) {
			return "", &UsageError{fmt.Sprintf("no project %q in %s", id, workspace)}
		}
		return id, nil
	}
	projects := Index(workspace).Projects
	switch len(projects) {
	case 0:
		return "", &UsageError{fmt.Sprintf("no projects in %s", workspace)}
	case 1:
		return projects[0].ID, nil
	}
	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	return "", &UsageError{fmt.Sprintf("several projects in %s (%s): pass --project", workspace, strings.Join(ids, ", "))}
}

// included: a symbol belongs to the project when its file is under one of
// project.json → sources.include. No include: the whole source root.
func included(file string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, inc := range include {
		inc = strings.Trim(path.Clean(strings.ReplaceAll(inc, `\`, "/")), "/")
		if inc == "" || inc == "." || file == inc || strings.HasPrefix(file, inc+"/") {
			return true
		}
	}
	return false
}

func codeRefFile(ref string) string {
	if i := strings.IndexAny(ref, "#:"); i >= 0 {
		return ref[:i]
	}
	return ref
}

func relationType(r *object) string {
	if t := r.str("type"); t != "" {
		return t
	}
	return r.str("relation")
}

func triple(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }

// relationKey returns the identification key for any relation: either triple
// (from, to, type) for organic edges or member key for member relations.
func relationKey(r *object) string {
	via, ok := r.vals["via"]
	if !ok || len(via) == 0 || string(via) == "null" {
		// Organic relation
		return triple(r.str("from"), r.str("to"), relationType(r))
	}
	// Member relation: key is (from, to, via.member, via.path)
	var v Via
	if err := json.Unmarshal(via, &v); err != nil {
		// Malformed via; treat as organic for ordering
		return triple(r.str("from"), r.str("to"), relationType(r))
	}
	return memberRelationKey(r.str("from"), r.str("to"), &v)
}

// memberRelationKey creates the identification key for a member relation:
// (from, to, member, path).
func memberRelationKey(from, to string, via *Via) string {
	member := ""
	if via != nil {
		member = via.Member
	}
	path := ""
	if via != nil {
		path = strings.Join(via.Path, ",")
	}
	return from + "\x00" + to + "\x00" + member + "\x00" + path
}

// deriveRelationType derives the relation type from edge kind and via features.
// For holds: type is based on cardinality and mutability.
// For uses: always "uses" unless it's a constructor, which gives "injects".
func deriveRelationType(kind string, via *Via) string {
	if kind == "uses" {
		if via != nil && via.MemberKind == "constructor" {
			return "injects"
		}
		return "uses"
	}
	if kind != "holds" {
		return kind // extends, implements, contains, depends
	}

	// Derive holds type from cardinality and mutability
	var base string
	if via != nil {
		switch via.Cardinality {
		case "optional":
			base = "holds.optional"
		case "many":
			if via.Mutability == "readonly" {
				base = "holds.many.ro"
			} else {
				base = "holds.many"
			}
		case "keyed":
			if via.Mutability == "readonly" {
				base = "holds.keyed.ro"
			} else {
				base = "holds.keyed"
			}
		default:
			base = "holds.one"
		}
	} else {
		base = "holds.one"
	}

	// Add .internal suffix if member is not public
	if !isPublicMember(via) {
		base += ".internal"
	}
	return base
}

// isPublicMember checks if a member is public (has "public" modifier or no access-limiting modifiers).
// Extractors emit effective accessibility: C# emits DeclaredAccessibility, so all members have
// an accessibility modifier in the list. Empty/nil modifiers shouldn't happen, but we default
// to public for defensive reasons.
func isPublicMember(via *Via) bool {
	if via == nil || len(via.Modifiers) == 0 {
		return true // no modifiers declared; assume public
	}
	hasPublic := false
	hasPrivate := false
	for _, mod := range via.Modifiers {
		switch strings.ToLower(mod) {
		case "public":
			hasPublic = true
		case "private", "internal", "protected":
			hasPrivate = true
		}
	}
	if hasPublic {
		return true
	}
	if hasPrivate {
		return false
	}
	return true // no access modifier found; assume public
}

// mintMemberRelationID creates an ID for a new member relation.
// Format: r_<from>_<to>_<member>[_<path>], collision -> _2
func mintMemberRelationID(from, to string, via *Via, taken map[string]bool) string {
	member := ""
	if via != nil {
		member = via.Member
	}
	base := "r_" + strings.TrimPrefix(from, "e_") + "_" + strings.TrimPrefix(to, "e_") + "_" + slug(member)
	if via != nil && len(via.Path) > 0 {
		base += "_" + strings.Join(via.Path, "_")
	}
	return mint(base, taken)
}

// newEntityID is the base of a new entity's id. A type, function or value is
// named by its short name (`e_repetitionguard`). A module is named by its whole
// symbol id with its native kind in front — `e_namespace_neuromodflownet_onnx_diagnostics`,
// `e_assembly_neuromodflownet_onnx`, `e_file_editor_src_canvas_diagramcanvas`:
// module short names (`Diagnostics`, `Tracking`) repeat across assemblies and
// read as something else, and a namespace and an assembly often share a name.
func newEntityID(s Symbol) string {
	if s.Kind == "module" {
		if id := slug(s.ID); id != "" {
			if kind := slug(s.NativeKind); kind != "" {
				return "e_" + kind + "_" + id
			}
			return "e_" + id
		}
	}
	return "e_" + orDefault(slug(baseName(s.Name)), "symbol")
}

// baseName drops what a hand-written name may carry and a symbol name does
// not: a trailing generic parameter list (`IRunner<in TIn, out TOut>`) and a
// backtick arity (`IRunner`2`). Used only to compare, never to write.
func baseName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, ">") {
		depth := 0
		for i := len(name) - 1; i >= 0; i-- {
			switch name[i] {
			case '>':
				depth++
			case '<':
				depth--
			}
			if depth == 0 {
				name = strings.TrimSpace(name[:i])
				break
			}
		}
	}
	if i := strings.LastIndexByte(name, '`'); i > 0 && i < len(name)-1 && strings.Trim(name[i+1:], "0123456789") == "" {
		name = name[:i]
	}
	return name
}

// kindFamily compares kinds without their modifiers: a native kind written as
// `<modifier>-<kind>` (`abstract-class`, `static-class`, `record-struct`) is
// the family of its last segment. The registry's `class` then matches the
// extractor's `abstract-class`; `record-struct` ~ `struct`.
func kindFamily(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if i := strings.LastIndexByte(kind, '-'); i >= 0 {
		return kind[i+1:]
	}
	return kind
}

// slug is the lower-cased name with every run of non-letters and non-digits
// turned into one `_`: `NeuroModFlowNet.ONNX` → `neuromodflownet_onnx`.
func slug(name string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if gap && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			gap = false
		} else {
			gap = true
		}
	}
	return b.String()
}

// mint returns base, or base_2, base_3… — the first one not taken — and
// takes it. An id is minted once; later runs find the entity by `symbol`.
func mint(base string, taken map[string]bool) string {
	id := base
	for n := 2; taken[id]; n++ {
		id = fmt.Sprintf("%s_%d", base, n)
	}
	taken[id] = true
	return id
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---------------------------------------------------------------------------
// Registry files are read into ordered objects and written back with every
// key they had, in the order they had it: sync owns a few fields, the editor
// and humans own the rest.

type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *object { return &object{vals: map[string]json.RawMessage{}} }

func (o *object) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return errors.New("expected an object")
	}
	o.keys, o.vals = nil, map[string]json.RawMessage{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		key := t.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		if _, dup := o.vals[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.vals[key] = raw
	}
	_, err = dec.Token()
	return err
}

func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encodeJSON(k))
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// str is the key's value when it is a string, "" otherwise.
func (o *object) str(key string) string {
	var s string
	if raw, ok := o.vals[key]; ok {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

func (o *object) set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = encodeJSON(v)
}

func (o *object) del(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// encodeJSON marshals without HTML escaping: `Task<int>` stays readable, as
// the editor's JSON.stringify writes it.
func encodeJSON(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // only plain values and objects reach here
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}

type registry struct {
	file    string
	name    string
	listKey string
	top     *object
	items   []*object
	dirty   bool
}

func loadRegistry(dir, name, listKey string, fresh func() *object) (*registry, error) {
	r := &registry{file: filepath.Join(dir, name), name: name, listKey: listKey}
	data, err := os.ReadFile(r.file)
	if errors.Is(err, fs.ErrNotExist) {
		r.top = fresh()
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	r.top = newObject()
	if err := json.Unmarshal(data, r.top); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if raw, ok := r.top.vals[listKey]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &r.items); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", name, listKey, err)
		}
	}
	return r, nil
}

// save writes the file with two-space indentation, like the editor does.
func (r *registry) save() error {
	items := r.items
	if items == nil {
		items = []*object{}
	}
	r.top.set(r.listKey, items)
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r.top); err != nil {
		return err
	}
	return os.WriteFile(r.file, b.Bytes(), 0o644)
}
