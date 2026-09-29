package core

import (
	"encoding/json"
	"path/filepath"
	"sort"
)

// GroupData is what the graph mode can group nodes by beyond the facts
// themselves: the containers' nesting (containers.json `parent`) and, per
// axis, which zone of the project's views each entity sits in. It is read-only
// and built from the working model (unsaved edits included); nothing in it is
// ever written back.
type GroupData struct {
	Containers []GroupContainer `json:"containers"`
	Axes       []GroupAxis      `json:"axes"`
}

type GroupContainer struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
}

// GroupAxis is one axis (`axis_*`, CONTRACT §8.1) the project's views declare —
// or inherit from `defaultAxis` — with the zones of those views and, per
// entity, the zone it is placed in. An entity a view places outside any zone
// (`zone: null`), or no view of the axis places at all, is absent from `Of`.
// When views of one axis place the same entity in different zones (which
// CONTRACT §8.1 forbids), the first view in file order wins.
type GroupAxis struct {
	Axis  string            `json:"axis"`
	Zones []GroupZone       `json:"zones"`
	Of    map[string]string `json:"of"`
}

type GroupZone struct {
	ID        string `json:"id"`
	Container string `json:"container,omitempty"`
	Parent    string `json:"parent,omitempty"`
}

// Groups builds the GroupData of the model's project.
func (m *Model) Groups() (*GroupData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := &GroupData{Containers: []GroupContainer{}, Axes: []GroupAxis{}}
	if containers, err := LoadContainers(m.dir); err != nil {
		return nil, err
	} else if containers != nil {
		for _, c := range containers.List {
			data.Containers = append(data.Containers, GroupContainer{ID: c.ID, Parent: c.Parent})
		}
	}

	defaultAxis := m.manifest.str("defaultAxis")
	files, _ := filepath.Glob(filepath.Join(m.dir, "views", "*.view.json"))
	sort.Strings(files)
	byAxis := map[string]*GroupAxis{}
	var order []string
	for _, file := range files {
		doc, err := loadDoc(file)
		if err != nil {
			return nil, err
		}
		id := doc.str("id")
		if id == "" {
			id = filepathBase(file)
		}
		if cached := m.views[id]; cached != nil {
			doc = cached.doc // an unsaved edit of the view wins over the file
		}
		axis := doc.str("axis")
		if axis == "" {
			axis = defaultAxis
		}
		if axis == "" {
			continue // the layout of a view without an axis says nothing (CONTRACT §8.1)
		}
		a := byAxis[axis]
		if a == nil {
			a = &GroupAxis{Axis: axis, Zones: []GroupZone{}, Of: map[string]string{}}
			byAxis[axis] = a
			order = append(order, axis)
		}
		var zones []struct {
			ID        string `json:"id"`
			Container string `json:"container"`
			Parent    string `json:"parent"`
		}
		_ = json.Unmarshal(doc.vals["zones"], &zones)
		seen := map[string]bool{}
		for _, z := range a.Zones {
			seen[z.ID] = true
		}
		for _, z := range zones {
			if z.ID != "" && !seen[z.ID] {
				a.Zones = append(a.Zones, GroupZone{ID: z.ID, Container: z.Container, Parent: z.Parent})
				seen[z.ID] = true
			}
		}
		// `nodes`/`placements`, `entity`/`id`, `zone`/`container`: the loader takes both spellings (CONTRACT §8.3).
		raw := doc.vals["nodes"]
		if raw == nil {
			raw = doc.vals["placements"]
		}
		var nodes []struct {
			Entity    string `json:"entity"`
			ID        string `json:"id"`
			Zone      string `json:"zone"`
			Container string `json:"container"`
		}
		_ = json.Unmarshal(raw, &nodes)
		for _, n := range nodes {
			entity, zone := n.Entity, n.Zone
			if entity == "" {
				entity = n.ID
			}
			if zone == "" {
				zone = n.Container
			}
			if entity == "" || zone == "" {
				continue
			}
			if _, placed := a.Of[entity]; !placed {
				a.Of[entity] = zone
			}
		}
	}
	for _, axis := range order {
		data.Axes = append(data.Axes, *byAxis[axis])
	}
	return data, nil
}

func filepathBase(file string) string {
	base := filepath.Base(file)
	const suffix = ".view.json"
	if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
		return base[:len(base)-len(suffix)]
	}
	return base
}
