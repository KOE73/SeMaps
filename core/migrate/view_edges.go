package migrate

// The `edges` of a view is an overlay on the registry, not a copy of it
// (ADR_20260930-7). A view of the earlier form kept a full list — `id`, `from`,
// `to`, `type` of every line — that replaced the registry. This step brings such
// a view to the current shape, by the content, so a second run finds nothing to do:
//
//   - an entry whose `id` is not a relation of the registry becomes a relation
//     `origin: authored`, `status: present` when both its ends are entities of the
//     registry; otherwise it is dropped and named in the report;
//   - `relations.except` of the view is recomputed so that, among the relations
//     with both ends placed on the view, exactly those that were in the list stay
//     visible — by the same rule as core: the dictionary's visibility of the type,
//     else the view's `relations.default`, else visible;
//   - `edges` keeps only the entries that have a styleId, an override or a routing
//     of their own, stripped of from/to/type; an emptied `edges` key is removed.
//
// An `edges: []` was "no lines": the recomputed `except` hides them all. A view
// that had no list of the earlier form keeps its picture too: a relation this
// step adds to the registry is hidden there when it would show.

import (
	"fmt"
	"strings"
)

// relationKeyType is the type of a relation record: `type`, else the older `relation`.
func relationKeyType(r *obj) string {
	if s, _ := asStr(r.get("type")); s != "" {
		return s
	}
	s, _ := asStr(r.get("relation"))
	return s
}

// oldEdgeList tells whether the `edges` of a view is a list of the earlier form:
// empty (it meant no lines) or with a from, to or type in an entry.
func oldEdgeList(arr []any) bool {
	if len(arr) == 0 {
		return true
	}
	for _, it := range arr {
		if e, ok := asObj(it); ok && (e.has("from") || e.has("to") || e.has("type")) {
			return true
		}
	}
	return false
}

// ownEdgeFields is what an entry of `edges` may hold besides its id.
var ownEdgeFields = []string{"styleId", "override", "routing"}

// hasOwnEdgeField: the entry has a styleId, an override or a routing that says something.
func hasOwnEdgeField(e *obj) bool {
	for _, k := range ownEdgeFields {
		v, ok := e.lookup(k)
		if !ok || isNull(v) {
			continue
		}
		if s, isStr := asStr(v); isStr && s == "" {
			continue
		}
		return true
	}
	return false
}

// edgeViewState is one view of the project with what the first pass learned of it.
type edgeViewState struct {
	v       *viewData
	id      string
	placed  map[string]bool
	old     bool            // its `edges` is a list of the earlier form
	listed  map[string]bool // ids of the old list that stay drawn
	kept    []any
	entries int
	dropped int
	differ  int
	added   int
}

// viewEdges is the step of one project. It runs after the dictionary of the
// workspace is decided (the visibility of a relation type is read from it).
func (m *projectMigration) viewEdges() error {
	var (
		entIDs = map[string]bool{}
		rels   = map[string]*obj{}
		relArr []any
	)
	if m.ents != nil {
		arr, err := m.ents.array("entities")
		if err != nil {
			return err
		}
		for _, it := range arr {
			if o, ok := asObj(it); ok {
				if id, _ := asStr(o.get("id")); id != "" {
					entIDs[id] = true
				}
			}
		}
	}
	if m.rels != nil {
		var err error
		if relArr, err = m.rels.array("relations"); err != nil {
			return err
		}
		for _, it := range relArr {
			if o, ok := asObj(it); ok {
				if id, _ := asStr(o.get("id")); id != "" {
					rels[id] = o
				}
			}
		}
	}

	// first pass: read every view, add the relations the lists had of their own
	var views []*edgeViewState
	var addedRels []*obj
	oldAny := false
	for _, v := range m.views {
		ev := &edgeViewState{v: v, placed: map[string]bool{}, listed: map[string]bool{}}
		ev.id, _ = asStr(v.f.root.get("id"))
		if ev.id == "" {
			ev.id = v.f.rel
		}
		if pv, ok := v.f.root.lookup("placements"); ok {
			pa, ok := asArr(pv)
			if !ok {
				return ferr(v.f.rel, "placements", "ожидался массив")
			}
			for _, it := range pa {
				if p, ok := asObj(it); ok {
					if id, _ := asStr(p.get("entity")); id != "" {
						ev.placed[id] = true
					}
				}
			}
		}
		views = append(views, ev)
		lv, ok := v.f.root.lookup("edges")
		if !ok {
			continue
		}
		arr, ok := asArr(lv)
		if !ok {
			return ferr(v.f.rel, "edges", "ожидался массив")
		}
		if !oldEdgeList(arr) {
			continue
		}
		ev.old, ev.entries = true, len(arr)
		oldAny = true
		for i, it := range arr {
			e, ok := asObj(it)
			if !ok {
				return ferr(v.f.rel, fmt.Sprintf("edges[%d]", i), "ожидался объект")
			}
			id, _ := asStr(e.get("id"))
			if id == "" {
				ev.dropped++
				m.rep.Notes = append(m.rep.Notes, fmt.Sprintf("вид %s: запись edges[%d] без id отброшена", ev.id, i))
				continue
			}
			from, _ := asStr(e.get("from"))
			to, _ := asStr(e.get("to"))
			typ, _ := asStr(e.get("type"))
			if r := rels[id]; r != nil {
				rf, _ := asStr(r.get("from"))
				rt, _ := asStr(r.get("to"))
				if (from != "" && from != rf) || (to != "" && to != rt) || (typ != "" && typ != relationKeyType(r)) {
					ev.differ++
				}
			} else {
				if !entIDs[from] || !entIDs[to] || typ == "" {
					ev.dropped++
					m.rep.Notes = append(m.rep.Notes, fmt.Sprintf(
						"вид %s: линия %s (%s → %s, %s) отброшена: её нет среди связей реестра, а концы — не сущности реестра", ev.id, id, from, to, typ))
					continue
				}
				r := newObj()
				r.set("id", jsonStr(id))
				r.set("from", jsonStr(from))
				r.set("to", jsonStr(to))
				r.set("type", jsonStr(typ))
				r.set("origin", jsonStr("authored"))
				r.set("status", jsonStr("present"))
				relArr = append(relArr, r)
				rels[id] = r
				addedRels = append(addedRels, r)
				ev.added++
			}
			ev.listed[id] = true
			if !hasOwnEdgeField(e) {
				continue
			}
			ne := newObj()
			ne.set("id", jsonStr(id))
			for _, k := range e.keys {
				switch k {
				case "id", "from", "to", "type":
				default:
					ne.set(k, e.m[k])
				}
			}
			ev.kept = append(ev.kept, ne)
		}
	}
	if !oldAny {
		return nil
	}
	if len(addedRels) > 0 {
		if m.rels == nil {
			m.rels = newFile(m.ws, m.rel+"/relations.json")
		}
		m.rels.root.set("relations", relArr)
		m.rels.dirty = true
	}

	// second pass: the rule of core decides what is visible, so that the list
	// decides only once, here
	for _, ev := range views {
		v := ev.v
		var ro *obj
		if rv, ok := v.f.root.lookup("relations"); ok {
			o, ok := asObj(rv)
			if !ok {
				return ferr(v.f.rel, "relations", "ожидался объект")
			}
			ro = o
		}
		var oldExcept []any
		if ro != nil {
			if xv, ok := ro.lookup("except"); ok {
				xa, ok := asArr(xv)
				if !ok {
					return ferr(v.f.rel, "relations.except", "ожидался массив")
				}
				oldExcept = xa
			}
		}
		visibleBy := func(r *obj) bool { // the rule of CONTRACT §8.5 without `except`
			d := m.viewDefault(v)
			if t, _ := m.lib.effective(relationKeyType(r)); t != nil {
				if vis, _, _ := strField(t, "visibility"); vis != "" {
					d = vis
				}
			}
			return d == "visible"
		}
		bothPlaced := func(r *obj) bool {
			from, _ := asStr(r.get("from"))
			to, _ := asStr(r.get("to"))
			return ev.placed[from] && ev.placed[to]
		}

		var except []any
		switch {
		case ev.old:
			except = []any{}
			for _, it := range relArr {
				r, ok := asObj(it)
				if !ok || !bothPlaced(r) {
					continue
				}
				id, _ := asStr(r.get("id"))
				if visibleBy(r) != ev.listed[id] {
					except = append(except, jsonStr(id))
				}
			}
		default:
			// no list of the earlier form: what was drawn stays, the relations added
			// above are new to this view and are hidden when they would show
			except = append([]any(nil), oldExcept...)
			have := map[string]bool{}
			for _, x := range oldExcept {
				if s, ok := asStr(x); ok {
					have[s] = true
				}
			}
			for _, r := range addedRels {
				id, _ := asStr(r.get("id"))
				if bothPlaced(r) && visibleBy(r) && !have[id] {
					except = append(except, jsonStr(id))
				}
			}
			if len(except) == len(oldExcept) {
				continue
			}
		}
		switch {
		case ro != nil:
			ro.set("except", except)
		case len(except) > 0:
			ro = newObj()
			ro.set("default", jsonStr("visible"))
			ro.set("except", except)
			v.f.root.insertBefore("placements", "relations", ro)
		}
		v.f.dirty = true

		if !ev.old {
			continue
		}
		if len(ev.kept) > 0 {
			v.f.root.set("edges", ev.kept)
		} else {
			v.f.root.del("edges")
		}
		unplaced := 0
		for id := range ev.listed {
			if r := rels[id]; r != nil && !bothPlaced(r) {
				unplaced++
			}
		}
		parts := []string{fmt.Sprintf("edges: %d → %d", ev.entries, len(ev.kept)), fmt.Sprintf("relations.except: %d → %d", len(oldExcept), len(except))}
		if ev.added > 0 {
			parts = append(parts, fmt.Sprintf("связей реестра добавлено: %d", ev.added))
		}
		if ev.dropped > 0 {
			parts = append(parts, fmt.Sprintf("отброшено: %d", ev.dropped))
		}
		if unplaced > 0 {
			parts = append(parts, fmt.Sprintf("линий списка без обоих концов на виде (не рисуются): %d", unplaced))
		}
		if ev.differ > 0 {
			parts = append(parts, fmt.Sprintf("концы или тип записи расходились со связью реестра (взята связь реестра): %d", ev.differ))
		}
		m.rep.ViewEdges = append(m.rep.ViewEdges, ev.id+": "+strings.Join(parts, ", "))
	}
	return nil
}
