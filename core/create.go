package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ids of projects and views are folder and file names; one rule for every
// caller (editor dialogs, HTTP, MCP) and for creating as well as renaming.
var (
	ProjectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	ViewIDPattern    = regexp.MustCompile(`^v_[a-z0-9_]+$`)
)

// ErrExists: a create or a rename would land on a project or view that is
// already there. The same error as the renames' fs.ErrExist.
var ErrExists = fs.ErrExist

// NewProject is what a new, empty, hand-authored project starts with (CONTRACT §2).
type NewProject struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle,omitempty"`
	DefaultAxis string `json:"defaultAxis,omitempty"`
	Language    string `json:"language,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Theme       string `json:"theme,omitempty"`
}

// NewView is what a new, empty view starts with (CONTRACT §8). Its name is a
// text and goes through the model (Model.SetText), not into the file.
type NewView struct {
	ID         string `json:"id"`
	Axis       string `json:"axis,omitempty"`
	Icon       string `json:"icon,omitempty"`
	Theme      string `json:"theme,omitempty"`
	SetDefault bool   `json:"setDefault,omitempty"`
}

// createOnly writes a brand-new file and refuses to overwrite anything:
// a create that replaces is a delete.
func createOnly(file string, o *object) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := encodeDoc(o)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExists, filepath.Base(file))
	}
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// CreateProject writes project.json of a new project: no views, no entities,
// nothing extracted. A structural change: written at once, not journaled.
func CreateProject(workspace string, p NewProject) error {
	if !ProjectIDPattern.MatchString(p.ID) {
		return refuse("project id %q: expected %s", p.ID, ProjectIDPattern)
	}
	if strings.TrimSpace(p.Title) == "" {
		return refuse("project title is empty")
	}
	lang := orDefault(p.Language, "ru")
	if strings.ContainsAny(lang, `/\.`) {
		return refuse("language %q: expected a code like ru or en", lang)
	}
	if exists(filepath.Join(workspace, "projects", p.ID)) {
		return fmt.Errorf("%w: project %s", ErrExists, p.ID)
	}
	o := newObject()
	o.set("id", p.ID)
	o.set("title", p.Title)
	if p.Subtitle != "" {
		o.set("subtitle", p.Subtitle)
	}
	o.set("contractVersion", ContractVersion)
	if p.DefaultAxis != "" {
		o.set("defaultAxis", p.DefaultAxis)
	}
	o.set("languages", []string{lang})
	if p.Icon != "" {
		o.set("icon", p.Icon)
	}
	if p.Theme != "" {
		o.set("theme", p.Theme)
	}
	return createOnly(filepath.Join(workspace, "projects", p.ID, "project.json"), o)
}

// CreateView writes the file of a new, empty view into an existing project.
// With SetDefault, project.json's defaultView is rewritten in place.
func CreateView(workspace, project string, v NewView) error {
	if !ViewIDPattern.MatchString(v.ID) {
		return refuse("view id %q: expected %s", v.ID, ViewIDPattern)
	}
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	if _, _, err := loadView(dir, v.ID); err == nil {
		return fmt.Errorf("%w: view %s", ErrExists, v.ID)
	}
	manifest, err := loadDoc(filepath.Join(dir, "project.json"))
	if err != nil {
		return err
	}
	if manifest == nil {
		return refuse("no project manifest %s", filepath.Base(dir))
	}
	// `axis` is required of a view; one that names none takes project.defaultAxis (CONTRACT §8.1).
	if v.Axis == "" && manifest.str("defaultAxis") == "" {
		return refuse("give axis: the project has no defaultAxis (CONTRACT §8.1)")
	}
	o := newObject()
	o.set("id", v.ID)
	o.set("project", filepath.Base(dir))
	if v.Axis != "" {
		o.set("axis", v.Axis)
	}
	if v.Icon != "" {
		o.set("icon", v.Icon)
	}
	if v.Theme != "" {
		o.set("theme", v.Theme)
	}
	// No `edges` key: a view without one shows the project's relations (CONTRACT §8.5).
	o.set("placements", []any{})
	if err := createOnly(filepath.Join(dir, "views", v.ID+ViewSuffix), o); err != nil {
		return err
	}
	if !v.SetDefault {
		return nil
	}
	manifest.set("defaultView", v.ID)
	return saveDoc(filepath.Join(dir, "project.json"), manifest)
}

// Languages of the project manifest; ["ru"] when it names none.
func (m *Model) Languages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.textLanguages()
}
