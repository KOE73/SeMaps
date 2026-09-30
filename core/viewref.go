package core

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ObjectRef points at a view or at one object of it. The text form is
// `<view>#<id>`, or `<project>/<view>#<id>` when the workspace has several
// projects; a bare `<view>` names the view itself. The editor's own link,
// `/app/#<view>?highlight=<id>`, is accepted too.
type ObjectRef struct {
	Project string
	View    string
	ID      string
}

func (r ObjectRef) String() string {
	s := r.View
	if r.Project != "" {
		s = r.Project + "/" + s
	}
	if r.ID != "" {
		s += "#" + r.ID
	}
	return s
}

func ParseRef(ref string) (ObjectRef, error) {
	s := strings.TrimSpace(ref)
	if i := strings.Index(s, "#"); i >= 0 && strings.Contains(s[:i], "/app") {
		s = s[i+1:]
		if head, query, ok := strings.Cut(s, "?"); ok {
			id := ""
			if q, err := url.ParseQuery(query); err == nil {
				id = q.Get("highlight")
			}
			s = head
			if id != "" {
				s += "#" + id
			}
		}
	}
	left, id, hasID := strings.Cut(s, "#")
	var out ObjectRef
	if p, v, ok := strings.Cut(left, "/"); ok {
		out.Project, out.View = p, v
	} else {
		out.View = left
	}
	if out.View == "" || strings.Contains(out.View, "/") || (hasID && id == "") || strings.ContainsAny(id, "#, ") {
		return ObjectRef{}, refuse("invalid object reference %q: want <view>#<id>", ref)
	}
	out.ID = id
	return out, nil
}

// ResolveRef checks that the referenced view exists and, when the reference
// names an object, that the object is on it: "view", "container" (a placement
// of a container entity) or "block" (any other placement).
func (m *Model) ResolveRef(r ObjectRef) (string, error) {
	doc, err := m.view(r.View)
	if err != nil {
		return "", err
	}
	if r.ID == "" {
		return "view", nil
	}
	for _, p := range viewItems(doc, "placements") {
		if p.str("entity") == r.ID {
			if m.isContainerEntity(r.ID) {
				return "container", nil
			}
			return "block", nil
		}
	}
	return "", refuse("%s is not on view %s", r.ID, r.View)
}

// isContainerEntity: the entity's kind is a container kind of the dictionary.
func (m *Model) isContainerEntity(id string) bool {
	e := m.record("entity", id)
	return e != nil && m.kinds.IsContainer(e.str("kind"))
}

// viewItems is the objects under a view key, in file order.
func viewItems(doc *object, key string) []*object {
	var items []*object
	if raw := doc.vals[key]; len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &items)
	}
	return items
}

// oldViewShape names what makes a view a shape of contract 3 or older —
// `zones`, `nodes`, a placement with `zone`/`container` or `id` instead of
// `entity`; "" when the view is of the current shape (CONTRACT §8). The loader
// does not read it; `semaps migrate` rewrites it (ADR_20260927-3).
func oldViewShape(doc *object) string {
	for _, key := range []string{"zones", "nodes"} {
		if _, ok := doc.vals[key]; ok {
			return "`" + key + "`"
		}
	}
	for i, p := range viewItems(doc, "placements") {
		if s := oldPlacementShape(p); s != "" {
			return fmt.Sprintf("placements[%d]: %s", i, s)
		}
	}
	return ""
}

func oldPlacementShape(p *object) string {
	for _, key := range []string{"zone", "container"} {
		if _, ok := p.vals[key]; ok {
			return "`" + key + "`"
		}
	}
	if _, ok := p.vals["id"]; ok && p.str("entity") == "" {
		return "`id` вместо `entity`"
	}
	return ""
}

// containersFileError is the answer of the loader and of semaps check to a
// project directory that still has a containers.json: a container is an entity now.
var containersFileError = fmt.Sprintf("containers.json — контейнер это сущность с типом-контейнером, файл упразднён в контракте %d (`semaps migrate`, ADR_20260927-6)", ContractVersion)

// oldShapeError is the one text of the loader and of semaps check about a view
// of an old contract (ADR_20260927-3).
func oldShapeError(file, what string) string {
	return fmt.Sprintf("%s: %s — форма контракта 3, нужен %d (`semaps migrate`, ADR_20260927-3)", file, what, ContractVersion)
}
