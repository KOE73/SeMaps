package core

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ViewSuffix names a view file: `<workspace>/projects/<project>/views/<id>.view.json`.
const ViewSuffix = ".view.json"

// WorkspaceIndex is what the workspace holds, as found on disk: no list is
// kept anywhere else. See ADR_20260923-7_contract_projects-and-views-found-not-listed.
type WorkspaceIndex struct {
	Projects []ProjectIndex `json:"projects"`
}

type ProjectIndex struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Subtitle  string      `json:"subtitle,omitempty"`
	Icon      string      `json:"icon,omitempty"`
	Theme     string      `json:"theme,omitempty"`
	Order     *float64    `json:"order,omitempty"`
	Languages []string    `json:"languages"`
	Views     []ViewIndex `json:"views"`
	Error     string      `json:"error,omitempty"`
}

type ViewIndex struct {
	ID    string   `json:"id"`
	File  string   `json:"file"` // workspace-relative, slash-separated
	Axis  string   `json:"axis,omitempty"`
	Icon  string   `json:"icon,omitempty"`
	Theme string   `json:"theme,omitempty"`
	Order *float64 `json:"order,omitempty"`
	// Names by language, from `v_<id>.name` in text.<lang>.json.
	Names map[string]string `json:"names"`
	Error string            `json:"error,omitempty"`
}

type listed struct {
	Icon  string   `json:"icon"`
	Theme string   `json:"theme"`
	Order *float64 `json:"order"`
}

// Index lists every project under <workspace>/projects that has a project.json,
// and every view file in it. A missing projects folder is an empty workspace,
// not an error. A file that does not parse is reported on its entry, so one
// broken file hides nothing else.
func Index(workspace string) WorkspaceIndex {
	index := WorkspaceIndex{Projects: []ProjectIndex{}}
	root := filepath.Join(workspace, "projects")
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if !exists(filepath.Join(dir, "project.json")) {
			continue
		}
		index.Projects = append(index.Projects, indexProject(dir, entry.Name()))
	}
	sort.SliceStable(index.Projects, func(i, j int) bool {
		a, b := index.Projects[i], index.Projects[j]
		return less(a.Order, a.ID, b.Order, b.ID)
	})
	return index
}

func indexProject(dir, id string) ProjectIndex {
	p := ProjectIndex{ID: id, Title: id, Languages: []string{"ru"}, Views: []ViewIndex{}}
	var manifest struct {
		listed
		Title     string   `json:"title"`
		Subtitle  string   `json:"subtitle"`
		Languages []string `json:"languages"`
	}
	if err := readJSON(filepath.Join(dir, "project.json"), &manifest); err != nil {
		p.Error = "project.json: " + err.Error()
		return p
	}
	if manifest.Title != "" {
		p.Title = manifest.Title
	}
	p.Subtitle, p.Icon, p.Theme, p.Order = manifest.Subtitle, manifest.Icon, manifest.Theme, manifest.Order
	if len(manifest.Languages) > 0 {
		p.Languages = manifest.Languages
	}

	names := map[string]map[string]string{} // lang -> key -> name
	for _, lang := range p.Languages {
		var cat struct {
			Entries map[string]textRecord `json:"entries"`
		}
		_ = readJSON(filepath.Join(dir, "text."+lang+".json"), &cat)
		names[lang] = map[string]string{}
		for key, record := range cat.Entries {
			if v, ok := record.field("name"); ok {
				names[lang][key] = v.V
			} else if raw, ok := record["name"]; ok {
				var s string // v2 catalogue: a bare string
				if json.Unmarshal(raw, &s) == nil {
					names[lang][key] = s
				}
			}
		}
	}

	files, _ := os.ReadDir(filepath.Join(dir, "views"))
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ViewSuffix) {
			continue
		}
		v := ViewIndex{
			ID:    strings.TrimSuffix(f.Name(), ViewSuffix),
			File:  path.Join("projects", id, "views", f.Name()),
			Names: map[string]string{},
		}
		var doc struct {
			listed
			ID   string `json:"id"`
			Axis string `json:"axis"`
		}
		if err := readJSON(filepath.Join(dir, "views", f.Name()), &doc); err != nil {
			v.Error = err.Error()
		} else {
			if doc.ID != "" {
				v.ID = doc.ID
			}
			v.Axis, v.Icon, v.Theme, v.Order = doc.Axis, doc.Icon, doc.Theme, doc.Order
		}
		for lang, byKey := range names {
			if name, ok := byKey[v.ID]; ok && name != "" {
				v.Names[lang] = name
			}
		}
		p.Views = append(p.Views, v)
	}
	sort.SliceStable(p.Views, func(i, j int) bool {
		a, b := p.Views[i], p.Views[j]
		return less(a.Order, a.ID, b.Order, b.ID)
	})
	return p
}

// less orders by `order` where given, entries without one after, then by id.
func less(ao *float64, aid string, bo *float64, bid string) bool {
	switch {
	case ao != nil && bo != nil && *ao != *bo:
		return *ao < *bo
	case ao != nil && bo == nil:
		return true
	case ao == nil && bo != nil:
		return false
	}
	return aid < bid
}
