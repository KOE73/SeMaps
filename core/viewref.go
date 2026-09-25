package core

import (
	"encoding/json"
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
// names an object, that the object is on it. kind is "view", "zone" or "node".
func (m *Model) ResolveRef(r ObjectRef) (string, error) {
	doc, err := m.view(r.View)
	if err != nil {
		return "", refuse("no view %s", r.View)
	}
	if r.ID == "" {
		return "view", nil
	}
	for _, z := range viewItems(doc, "zones") {
		if z.str("id") == r.ID {
			return "zone", nil
		}
	}
	for _, key := range []string{"nodes", "placements"} {
		for _, n := range viewItems(doc, key) {
			if nodeID(n) == r.ID {
				return "node", nil
			}
		}
	}
	return "", refuse("%s is not on view %s", r.ID, r.View)
}

func nodeID(n *object) string { return orDefault(n.str("entity"), n.str("id")) }

// viewItems is the objects under a view key, in file order.
func viewItems(doc *object, key string) []*object {
	var items []*object
	if raw := doc.vals[key]; len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &items)
	}
	return items
}
