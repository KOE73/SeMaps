package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// New projects and views. The ids follow the editor's own rules
// (editor/src/editor/io/WorkspaceStore.ts).
var (
	projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	viewIDPattern    = regexp.MustCompile(`^v_[a-z0-9_]+$`)
)

// CreateProject writes `<workspace>/projects/<id>/project.json` — the same
// manifest the editor writes for a new project. A project is a folder, not a
// document of another model, so this cannot be an unsaved model change: it is
// written at once, and refused when the id is taken.
func CreateProject(workspace, id, title, language string) error {
	if !projectIDPattern.MatchString(id) {
		return refuse("project id %q: lowercase letters, digits and _, starting with a letter", id)
	}
	if language == "" {
		language = "ru"
	}
	if strings.ContainsAny(language, `/\.`) {
		return refuse("language %q: expected a code like ru or en", language)
	}
	if strings.TrimSpace(title) == "" {
		title = id
	}
	dir := filepath.Join(workspace, "projects", id)
	file := filepath.Join(dir, "project.json")
	if _, err := os.Stat(file); err == nil {
		return refuse("project %s already exists", id)
	}
	o := newObject()
	o.set("id", id)
	o.set("title", title)
	o.set("contractVersion", 3)
	o.set("languages", []string{language})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return refuse("project %s already exists", id)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Languages of the project manifest; ["ru"] when it names none.
func (m *Model) Languages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var langs []string
	if raw, ok := m.manifest.vals["languages"]; ok {
		_ = json.Unmarshal(raw, &langs)
	}
	if len(langs) == 0 {
		return []string{"ru"}
	}
	return langs
}

// DefaultAxis is `defaultAxis` of the project manifest, or "".
func (m *Model) DefaultAxis() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.manifest.str("defaultAxis")
}

// newView creates the view a `view` op names when it does not exist yet. It
// reports false, and does nothing, when the op is an ordinary property change.
func (m *Model) newView(op Op) (bool, error) {
	n := newObject()
	if err := json.Unmarshal(op.Value, n); err != nil {
		return false, err
	}
	_, hasZones := n.vals["zones"]
	_, hasNodes := n.vals["nodes"]
	if !hasZones || !hasNodes {
		return false, nil
	}
	if !viewIDPattern.MatchString(op.View) {
		return false, refuse("view id %q: v_ and lowercase letters, digits, _", op.View)
	}
	if n.str("id") != op.View {
		return false, refuse("view id differs from operation id")
	}
	file := filepath.Join(m.dir, "views", op.View+ViewSuffix)
	if _, err := os.Stat(file); err == nil {
		return false, refuse("file of view %s already exists under another id", op.View)
	}
	m.views[op.View] = &modelView{file: file, doc: n}
	return true, nil
}

// CreateView adds a view — id, axis (the project's defaultAxis when empty) and
// its name in each language of names — as one unsaved batch. The file appears
// on Save; until then the view lives in the shared model only.
func (m *Model) CreateView(id, axis string, names map[string]string, requestedByHuman bool, author string) error {
	if !requestedByHuman {
		return refuse("a new view is created only on a human's direct request")
	}
	if !viewIDPattern.MatchString(id) {
		return refuse("view id %q: v_ and lowercase letters, digits, _", id)
	}
	if _, err := m.view(id); err == nil {
		return refuse("view %s already exists", id)
	}
	if axis == "" {
		if axis = m.DefaultAxis(); axis == "" {
			return refuse("give axis: the project has no defaultAxis")
		}
	}
	doc := newObject()
	doc.set("id", id)
	doc.set("project", m.project)
	doc.set("axis", axis)
	doc.set("zones", []*object{})
	doc.set("nodes", []*object{})
	b, _ := doc.MarshalJSON()
	ops := []Op{{Kind: "view", ID: id, View: id, Value: b}}
	for lang, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if lang == "" || strings.ContainsAny(lang, `/\.`) {
			return refuse("language %q: expected a code like ru or en", lang)
		}
		entry := newObject()
		v := newObject()
		v.set("v", name)
		v.set("at", now().Format("2006-01-02T15:04:05Z"))
		v.set("origin", "authored")
		entry.set("name", v)
		tb, _ := entry.MarshalJSON()
		ops = append(ops, Op{Kind: "text", ID: id, Lang: lang, Value: tb})
	}
	if _, err := m.Apply(ops, author); err != nil {
		return fmt.Errorf("create view: %w", err)
	}
	return nil
}
