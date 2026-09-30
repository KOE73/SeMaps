package core

// Sync reconciles one project's registry (entities.json, relations.json) with
// extractor facts. The rules are normative in
// docs/EXTRACTOR.md §5; why they are what they are:
// ADR_20260923-9_core_sync-symbol-mapping-and-containment.
//
// Sync never writes texts or views, never touches an `authored`
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
	// DefaultKinds is the tool's dictionary, for SyncWorkspace's load
	// (LoadModel); sync itself never reads it.
	DefaultKinds []byte
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
	Added     []string `json:"added"`     // не хватает: new entities and relations
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
		fmt.Fprintln(w, "  Сущность: поставьте старой сущности в code[] `symbol` нового символа (и `name`, если имя сменилось).")
		fmt.Fprintln(w, "  Членская связь: установите `via.member` в evidence[] старой связи на новое имя члена и повторите сверку.")
		fmt.Fprintln(w, "  Не переименование — повторите с --no-renames.")
	}
}

// dropMethodsAndCalls removes methods and calls (ADR_20260928-4 §4) before
// sync does anything else with the facts: they are dynamic data of the live
// graph and must never reach the registry, git, the workspace, or a missing
// mark. Dropped by kind, never by extractor: symbols of kind "method", edges
// of kind "calls"/"constructs"/"overrides", and "contains"/"implements" edges
// with a method at either end. `calls` in edgeKinds is likewise ignored:
// nothing is ever marked missing for it.
func dropMethodsAndCalls(facts *Facts) *Facts {
	out := *facts
	out.Symbols = make([]Symbol, 0, len(facts.Symbols))
	isMethod := map[string]bool{}
	for _, s := range facts.Symbols {
		if s.Kind == "method" {
			isMethod[s.ID] = true
			continue
		}
		out.Symbols = append(out.Symbols, s)
	}
	out.Edges = make([]Edge, 0, len(facts.Edges))
	for _, e := range facts.Edges {
		switch e.Kind {
		case "calls", "constructs", "overrides":
			continue
		}
		if (e.Kind == "contains" || e.Kind == "implements") && (isMethod[e.From] || isMethod[e.To]) {
			continue
		}
		out.Edges = append(out.Edges, e)
	}
	var edgeKinds []string
	for _, k := range facts.EdgeKinds {
		if k == "calls" {
			continue
		}
		edgeKinds = append(edgeKinds, k)
	}
	out.EdgeKinds = edgeKinds
	return &out
}

// structuralTypes are the edge kinds sync writes as relations: all organic ones.
// Member relations (holds, uses) are not listed here; they are created per-member.
// Which ones a view shows is the view's decision (CONTRACT.md §8.5).
var structuralTypes = []string{"extends", "implements", "contains", "depends"}

// Sync reconciles facts into the working model. Persisting the contract is a
// separate, project-wide Save decision.
func Sync(model *Model, facts *Facts, opt SyncOptions) (*SyncReport, error) {
	if err := facts.Validate(); err != nil {
		return nil, err
	}
	facts = dropMethodsAndCalls(facts)
	if model == nil {
		return nil, errors.New("nil model")
	}
	if opt.Project != "" && opt.Project != model.project {
		return nil, &UsageError{fmt.Sprintf("model project %q differs from %q", model.project, opt.Project)}
	}
	model.mu.Lock()
	base := model.copy()
	model.mu.Unlock()
	working := base.copy()
	project := model.project
	var manifest struct {
		Sources struct {
			Include []string `json:"include"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(model.Manifest(), &manifest); err != nil {
		return nil, fmt.Errorf("project.json: %w", err)
	}
	rep := &SyncReport{Project: project, Language: facts.Language, DryRun: opt.DryRun}

	// ------------------------------------------------ facts of this project
	symbols := map[string]Symbol{}
	var order []string // symbol ids, in the facts' own (sorted) order
	for _, s := range facts.Symbols {
		// An external symbol has no file to filter by (ADR_20260927-4); it
		// stays whatever sources.include says.
		if s.NativeKind == ExternalKind || included(s.File, manifest.Sources.Include) {
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

	ents, rels := working.registries["entity"], working.registries["relation"]

	// ------------------------------------------------ registry must be sane
	taken := map[string]bool{}   // every entity id, any origin
	bySymbol := map[string]int{} // symbol of this language's realization -> index, non-authored only
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
		// One realization per entity per language (CONTRACT §3): the run of
		// this facts' language sees only the entry of its own language.
		langs := map[string]bool{}
		for _, c := range entries(e, "code") {
			if l := c.str("lang"); l != "" {
				if langs[l] {
					rep.Broken = append(rep.Broken, fmt.Sprintf("%s: в code[] два элемента языка %s", id, l))
				}
				langs[l] = true
			}
		}
		if k := entryFor(entries(e, "code"), facts.Language); k >= 0 {
			s := entries(e, "code")[k].str("symbol")
			if j, dup := bySymbol[s]; dup {
				rep.Broken = append(rep.Broken, fmt.Sprintf("%s и %s: у обеих symbol %s (%s)", ents.items[j].str("id"), id, s, facts.Language))
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
	lang := facts.Language
	adopt(ents.items, symbols, order, match, matched, held, heldSym, rep)
	if !opt.NoRenames {
		renames(ents.items, symbols, order, match, matched, held, heldSym, lang, rep)
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
		if msg := updateEntity(e, symbols[sid], lang); msg != "" {
			rep.Changed = append(rep.Changed, msg)
			ents.dirty = true
		}
	}
	gone := map[string]bool{}
	for i, e := range ents.items {
		if !managed(e, lang) || held[i] {
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
		e.set("code", []*object{codeEntry(lang, s)}) // an external symbol has no file, so no ref (ADR_20260927-4)
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
			relType := deriveRelationType(edge.Kind, edge.Via)
			key := memberRelationKey(from, to, relType, edge.Via)
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
					// Update the member signature in the evidence of this language
					hadVia := relationVia(r) != nil
					if putVia(r, lang, fromSym, edge.Via) {
						changes = append(changes, map[bool]string{true: "via", false: "evidence"}[hadVia])
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
			r.set("evidence", []*object{evidenceEntry(lang, fromSym, edge.Via)})
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
			r.set("evidence", []*object{evidenceEntry(lang, fromSym, nil)})
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
			from    string
			to      string
			path    string
			relType string // include type to distinguish holds from uses
		}
		missingMembers := make(map[memberKey][]int)  // key -> missing relation indices
		newMembers := make(map[memberKey][]string)   // key -> new member names
		confirmedMembers := make(map[memberKey]bool) // key -> any confirmed

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
			v := relationVia(r)
			if v == nil {
				continue
			}
			path := strings.Join(v.Path, ",")
			key := memberKey{from, to, path, relationFamily(relType)}
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
			v := relationVia(r)
			if v == nil {
				continue
			}
			path := strings.Join(v.Path, ",")
			key := memberKey{from, to, path, relationFamily(relType)}
			confirmedMembers[key] = true
			newMembers[key] = append(newMembers[key], v.Member)
		}

		// Report rename candidates
		for key, missingIndices := range missingMembers {
			if confirmedMembers[key] && len(newMembers[key]) > 0 {
				// Potential rename: same (from, to, path, type) but different member name
				for _, mi := range missingIndices {
					r := rels.items[mi]
					memberRenameHolds[mi] = true
					oldMember := relationVia(r).Member
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
			// Unknown/legacy types (e.g. old "references") should be marked missing
			isCovered = true
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

	// A relation type needs no record: it is a string on the relation, and what
	// it means — name, base style, default visibility — is the dictionary's
	// (CONTRACT §5, §6).
	if opt.DryRun {
		return rep, nil
	}
	var ops []Op
	for _, kind := range []string{"entity", "relation"} {
		reg := working.registries[kind]
		if !reg.dirty {
			continue
		}
		previous := map[string][]byte{}
		for _, item := range base.registries[kind].items {
			previous[item.str("id")], _ = item.MarshalJSON()
		}
		for _, item := range reg.items {
			value, _ := item.MarshalJSON()
			if !bytes.Equal(previous[item.str("id")], value) {
				ops = append(ops, Op{Kind: kind, ID: item.str("id"), Value: value})
			}
		}
		rep.Written = append(rep.Written, path.Join("projects", project, reg.name))
	}
	if _, err := model.Apply(ops, "sync"); err != nil {
		return rep, err
	}
	return rep, nil
}

// SyncWorkspace is the non-host CLI path: load, reconcile, and save. Hosts use
// Sync on their already loaded model and leave Save to the human.
func SyncWorkspace(workspace string, facts *Facts, opt SyncOptions) (*SyncReport, error) {
	m, err := LoadModel(workspace, opt.Project, opt.DefaultKinds)
	if err != nil {
		return nil, err
	}
	rep, err := Sync(m, facts, opt)
	if err != nil || opt.DryRun || rep.ExitCode() != 0 && len(rep.Broken) > 0 {
		return rep, err
	}
	if len(rep.Written) > 0 {
		err = m.Save()
	}
	return rep, err
}

// managed: an entity this run answers for — one realized in the facts'
// language (its `code[]` has an entry of that language bound to a symbol), or
// `origin: code` with no symbol bound anywhere. Authored entities, entities
// realized only in another language, and hand-made ones without origin that
// matched nothing are not this run's business (ADR_20260930-4).
func managed(e *object, lang string) bool {
	if e.str("origin") == "authored" {
		return false
	}
	list := entries(e, "code")
	if entryFor(list, lang) >= 0 {
		return true
	}
	return e.str("origin") == "code" && !hasBound(list)
}

// adopt matches entities made without sync (no realization bound to a symbol)
// to symbols, so that a first run keeps their ids instead of minting
// duplicates. Two rules, in order; each must be unique both ways, otherwise
// both sides are held:
//
//  1. same file (a `ref` of code[] without its #/: anchor) and same name; several
//     symbols there (a TS file module and its class) are narrowed by kind;
//  2. same namespace, same name and the same kind family.
//
// Names are compared by baseName (no generic parameter list, no arity), kinds
// by normKind (modifiers stripped): a hand-written registry says
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
				if f := refFile(firstRef(entries(e, "code"))); f != "" {
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
				return e.str("namespace") + "\x00" + baseName(e.str("name")) + "\x00" + normKind(e.str("kind"))
			},
			symbolKey: func(s Symbol) string {
				return s.Namespace + "\x00" + baseName(s.Name) + "\x00" + normKind(s.NativeKind)
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
			if e.str("origin") == "authored" || hasBound(entries(e, "code")) || e.str("id") == "" || held[i] {
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
				// Exact kind first, then the base kind; neither — leave all.
				for _, same := range []func(string) bool{
					func(nk string) bool { return strings.EqualFold(nk, e.str("kind")) },
					func(nk string) bool { return normKind(nk) == normKind(e.str("kind")) },
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
	match map[string]int, matched map[int]string, held map[int]bool, heldSym map[string]bool, lang string, rep *SyncReport) {

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
		if !managed(e, lang) || held[i] || e.str("status") == "missing" {
			continue
		}
		if _, ok := matched[i]; ok {
			continue
		}
		if f := refFile(entityRef(e, lang)); f != "" {
			g := at(f + "\x00" + normKind(e.str("kind")))
			g.ents = append(g.ents, i)
		}
	}
	for _, sid := range order {
		if _, ok := match[sid]; ok || heldSym[sid] {
			continue
		}
		s := symbols[sid]
		if g := groups[s.File+"\x00"+normKind(s.NativeKind)]; g != nil {
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
// kind are the human's once the entity exists; namespace, the realization of
// this language in code[] (lang, ref, symbol), members and status follow the
// code. Returns a report line, or "" if nothing changed.
func updateEntity(e *object, s Symbol, lang string) string {
	var changes []string
	list := entries(e, "code")
	k := entryFor(list, lang)
	adopted := k < 0
	if adopted {
		// The hand-written link to a file (a `ref` only) becomes the realization,
		// preferring the one that names the symbol's own file; none: a new entry.
		for i, c := range list {
			if c.str("symbol") == "" && c.str("lang") == "" && (k < 0 || refFile(c.str("ref")) == s.File) {
				k = i
			}
		}
		if k < 0 {
			list = append(list, newObject())
			k = len(list) - 1
		}
	}
	old, _ := list[k].MarshalJSON()
	setCodeEntry(list[k], lang, s)
	if now, _ := list[k].MarshalJSON(); adopted || compactJSON(old) != compactJSON(now) {
		setEntries(e, "code", list) // written only when it changed: a rewrite would reformat the file's own bytes
		if !adopted {
			changes = append(changes, "code")
		}
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
	v := relationVia(r)
	if v == nil {
		// Organic relation
		return triple(r.str("from"), r.str("to"), relationType(r))
	}
	// Member relation: key is (from, to, family, via.member, via.path)
	return memberRelationKey(r.str("from"), r.str("to"), relationType(r), v)
}

// memberRelationKey creates the identification key for a member relation:
// (from, to, family, member, path). The family — holds, injects or uses — is
// part of it: a field and the constructor parameter that fills it share a
// name (`options`, a record's positional property) and are two relations; a
// wrapper change within a family (List → IReadOnlyList) keeps the relation.
func memberRelationKey(from, to, relType string, via *Via) string {
	member := ""
	if via != nil {
		member = via.Member
	}
	path := ""
	if via != nil {
		path = strings.Join(via.Path, ",")
	}
	return from + "\x00" + to + "\x00" + relationFamily(relType) + "\x00" + member + "\x00" + path
}

// relationFamily: holds for every holds.* type, the type itself otherwise.
func relationFamily(relType string) string {
	if relType == "holds" || strings.HasPrefix(relType, "holds.") {
		return "holds"
	}
	return relType
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
// named by its short name (`e_invoiceservice`). A module is named by its whole
// symbol id with its native kind in front — `e_namespace_shop_billing_diagnostics`,
// `e_assembly_shop_billing`, `e_file_web_src_canvas_drawing`:
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

// kindModifiers are the prefixes an extractor puts in front of a base kind in
// `nativeKind` (docs/extractors/csharp.md, "Модификаторы в nativeKind");
// the same list as KIND_MODIFIERS in editor/src/ui/kindIcons.ts.
var kindModifiers = map[string]bool{"abstract": true, "static": true, "sealed": true, "readonly": true, "ref": true}

// normKind is the base kind: case and surrounding space ignored, leading
// known modifiers (`abstract-class` → `class`) stripped. Compound kinds such
// as `record-struct` stay whole and are not a `struct`.
func normKind(kind string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(kind)), "-")
	for len(parts) > 1 && kindModifiers[parts[0]] {
		parts = parts[1:]
	}
	return strings.Join(parts, "-")
}

// slug is the lower-cased name with every run of non-letters and non-digits
// turned into one `_`: `Shop.Billing` → `shop_billing`.
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
	// saved: the ids the file held when it was loaded or last saved. A record
	// outside it was created in the unsaved working state and can be withdrawn
	// (ADR_20260930-8). Replaced, never mutated, so a copy may share it.
	saved map[string]bool
}

// markSaved remembers the ids now in items as the saved ones.
func (r *registry) markSaved() {
	r.saved = make(map[string]bool, len(r.items))
	for _, o := range r.items {
		r.saved[o.str("id")] = true
	}
}

func loadRegistry(dir, name, listKey string, fresh func() *object) (*registry, error) {
	r := &registry{file: filepath.Join(dir, name), name: name, listKey: listKey, saved: map[string]bool{}}
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
	r.markSaved()
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
