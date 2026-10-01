package core

// Removing authored records from the registry (ADR_20261001). A record of the
// registry is named by views (placements, edges, relations.except) and, for an
// entity, by relations; a removal that left those names behind would break the
// batch rule (checkWithdrawn). The batch that takes the names away together
// with the record is built here, once, for every writer: the editor and the MCP
// tool only call it (ADR_20260926).

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Removal is what RemoveRecords took away, for an answer to the one who asked.
type Removal struct {
	Entities  []string `json:"entities"`
	Relations []string `json:"relations"`
	// Placements, Edges and Except are `view#id`: the view that lost the entity's
	// placement, the entry of the relation in `edges`, its name in `relations.except`.
	Placements []string `json:"placements"`
	Edges      []string `json:"edges"`
	Except     []string `json:"except"`
}

// RemoveRecords removes entities and relations (ids) from the registry as one
// batch, all or nothing. Only authored records go: a record of code is the next
// sync's, and references to it must live. A removed entity takes its placements
// off every view of the project; a removed relation its `edges` entry and its
// name in `relations.except` on every view. The relations an entity stands in
// as an end go with it when cascade is set and every one of them is authored;
// otherwise the call is refused and says what holds the entity.
func (m *Model) RemoveRecords(ids []string, cascade bool, author string) (Removal, error) {
	out := Removal{Entities: []string{}, Relations: []string{}, Placements: []string{}, Edges: []string{}, Except: []string{}}
	if len(ids) == 0 {
		return out, refuse("ids: give at least one entity or relation id")
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()

	var unknown, notAuthored []string
	entities, relations := map[string]bool{}, map[string]bool{}
	for _, id := range ids {
		var kind string
		switch {
		case strings.HasPrefix(id, "e_"):
			kind = "entity"
		case strings.HasPrefix(id, "r_"):
			kind = "relation"
		default:
			unknown = append(unknown, id)
			continue
		}
		rec := m.record(kind, id)
		switch {
		case rec == nil:
			unknown = append(unknown, id)
		case !isAuthored(rec):
			notAuthored = append(notAuthored, id)
		case kind == "entity":
			entities[id] = true
		default:
			relations[id] = true
		}
	}
	if len(unknown) > 0 {
		return out, refuse("no entity or relation %s (an id starts with e_ or r_); nothing removed", strings.Join(unknown, ", "))
	}
	if len(notAuthored) > 0 {
		return out, refuse("%s come from code, not authored: only an authored record can be removed from the registry, the next sync would bring a code one back; nothing removed", strings.Join(notAuthored, ", "))
	}

	// the relations that stand on a removed entity
	var code, held []string
	for _, r := range m.records("relation") {
		id := r.str("id")
		if relations[id] || !(entities[r.str("from")] || entities[r.str("to")]) {
			continue
		}
		switch {
		case !isAuthored(r):
			code = append(code, id)
		case cascade:
			relations[id] = true
		default:
			held = append(held, id)
		}
	}
	if len(code) > 0 {
		return out, refuse("relation %s comes from code and has a removed entity as an end: the entity cannot be removed while the code says so; nothing removed", strings.Join(code, ", "))
	}
	if len(held) > 0 {
		return out, refuse("entity still has relations: %s; remove them too (cascade: true) or name them in ids; nothing removed", strings.Join(held, ", "))
	}

	var ops []Op
	for _, view := range m.viewIDs() {
		doc, err := m.view(view)
		if err != nil {
			return out, err
		}
		for _, p := range viewItems(doc, "placements") {
			if id := p.str("entity"); entities[id] {
				ops = append(ops, Op{Kind: "placement", ID: id, View: view, Value: json.RawMessage("null")})
				out.Placements = append(out.Placements, view+"#"+id)
			}
		}
		props := newObject()
		edges := viewItems(doc, "edges")
		keptEdges := slices.DeleteFunc(slices.Clone(edges), func(e *object) bool { return relations[e.str("id")] })
		if len(keptEdges) != len(edges) {
			for _, e := range edges {
				if relations[e.str("id")] {
					out.Edges = append(out.Edges, view+"#"+e.str("id"))
				}
			}
			props.set("edges", keptEdges)
		}
		policy, err := child(doc, "relations")
		if err != nil {
			return out, err
		}
		var except []string
		if raw, ok := policy.vals["except"]; ok {
			if err := json.Unmarshal(raw, &except); err != nil {
				return out, err
			}
		}
		keptExcept := slices.DeleteFunc(slices.Clone(except), func(id string) bool { return relations[id] })
		if len(keptExcept) != len(except) {
			for _, id := range except {
				if relations[id] {
					out.Except = append(out.Except, view+"#"+id)
				}
			}
			policy.set("except", keptExcept)
			props.set("relations", policy)
		}
		if len(props.keys) > 0 {
			props.set("id", view)
			b, _ := props.MarshalJSON()
			ops = append(ops, Op{Kind: "view", ID: view, View: view, Value: b})
		}
	}
	// relations before the entities that were their ends, each list in a stable order
	for _, set := range []struct {
		kind string
		ids  map[string]bool
		list *[]string
	}{{"relation", relations, &out.Relations}, {"entity", entities, &out.Entities}} {
		for id := range set.ids {
			*set.list = append(*set.list, id)
		}
		sort.Strings(*set.list)
		for _, id := range *set.list {
			ops = append(ops, Op{Kind: set.kind, ID: id, Value: json.RawMessage("null")})
		}
	}
	if _, err := m.Apply(ops, author); err != nil {
		return Removal{Entities: []string{}, Relations: []string{}, Placements: []string{}, Edges: []string{}, Except: []string{}}, err
	}
	return out, nil
}

// viewIDs: every view of the project — the files on disk and the ones only the
// working state has — in a stable order. A removal must look at all of them, not
// only those an editor has open.
func (m *Model) viewIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(m.dir, "views", "*.view.json"))
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".view.json")
		if doc, err := loadDoc(file); err == nil && doc != nil && doc.str("id") != "" {
			id = doc.str("id")
		}
		seen[id] = true
	}
	for id := range m.views {
		seen[id] = true
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
