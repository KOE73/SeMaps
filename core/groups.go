package core

import (
	"path/filepath"
	"sort"
)

// GroupData is what the graph mode can group nodes by beyond the facts
// themselves: the containers of the graph with their nesting (`contains`
// between container nodes, GraphContainers) and, per axis, which container
// placement of the project's views each entity sits in. It is read-only and
// built from the working model (unsaved edits included); nothing in it is ever
// written back.
type GroupData struct {
	Containers []GroupContainer `json:"containers"`
	Axes       []GroupAxis      `json:"axes"`
}

// GroupAxis is one axis (`axis_*`, CONTRACT §8.1) the project's views declare —
// or inherit from `defaultAxis` — with the containers placed on those views and,
// per entity, the container it is placed in (`parent` of its placement). An
// entity a view places outside any container (`parent: null`), or no view of
// the axis places at all, is absent from `Of`. When views of one axis place the
// same entity in different containers (which CONTRACT §8.1 forbids), the first
// view in file order wins.
type GroupAxis struct {
	Axis       string               `json:"axis"`
	Containers []GroupAxisContainer `json:"containers"`
	Of         map[string]string    `json:"of"`
}

// GroupAxisContainer is a container entity placed on a view of the axis, with
// the container it is placed in on that view. Ids are entity ids.
type GroupAxisContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// Groups is the GroupData of the model's project for a graph built from it.
func (m *Model) Groups(g *Graph) (*GroupData, error) {
	axes, err := m.AxisGroups()
	if err != nil {
		return nil, err
	}
	return &GroupData{Containers: GraphContainers(g), Axes: axes}, nil
}

// AxisGroups reads the views of the project, an unsaved edit of a view winning
// over its file.
func (m *Model) AxisGroups() ([]GroupAxis, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := map[string]string{}
	if r := m.registries["entity"]; r != nil {
		for _, e := range r.items {
			names[e.str("id")] = e.str("name")
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
		if old := oldViewShape(doc); old != "" {
			return nil, refuse("%s", oldShapeError("views/"+filepath.Base(file), old))
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
			a = &GroupAxis{Axis: axis, Containers: []GroupAxisContainer{}, Of: map[string]string{}}
			byAxis[axis] = a
			order = append(order, axis)
		}
		seen := map[string]bool{}
		for _, c := range a.Containers {
			seen[c.ID] = true
		}
		items := viewItems(doc, "placements")
		isContainer := map[string]bool{}
		if r := m.registries["entity"]; r != nil {
			for _, e := range r.items {
				isContainer[e.str("id")] = m.kinds.IsContainer(e.str("kind"))
			}
		}
		for _, p := range items {
			entity, parent := p.str("entity"), p.str("parent")
			if entity == "" {
				continue
			}
			if isContainer[entity] && !seen[entity] {
				a.Containers = append(a.Containers, GroupAxisContainer{ID: entity, Name: names[entity], Parent: parent})
				seen[entity] = true
			}
			if parent == "" {
				continue
			}
			if _, placed := a.Of[entity]; !placed {
				a.Of[entity] = parent
			}
		}
	}
	out := []GroupAxis{}
	for _, axis := range order {
		out = append(out, *byAxis[axis])
	}
	return out, nil
}

func filepathBase(file string) string {
	base := filepath.Base(file)
	const suffix = ".view.json"
	if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
		return base[:len(base)-len(suffix)]
	}
	return base
}
