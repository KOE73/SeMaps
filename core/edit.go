package core

// Editing rules are methods of Model. These workspace helpers serve callers
// that intentionally make a complete load/edit/save transaction (CLI and
// compatibility tests); the running host holds one Model across requests.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type EditError struct{ msg string }

func (e *EditError) Error() string         { return e.msg }
func refuse(format string, a ...any) error { return &EditError{fmt.Sprintf(format, a...)} }

func ProjectDir(workspace, project string) (string, error) {
	id, err := pickProject(workspace, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspace, "projects", id), nil
}

var now = func() time.Time { return time.Now().UTC() }
var TextFields = []string{"name", "title", "description", "doc", "fromLabel", "toLabel"}

type Placement struct {
	Entity string  `json:"entity"`
	Zone   string  `json:"zone,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

func editAndSave(workspace, project string, edit func(*Model) error) error {
	m, err := LoadModel(workspace, project)
	if err != nil {
		return err
	}
	if err := edit(m); err != nil {
		return err
	}
	return m.Save()
}

func SetText(workspace, project, lang, key, field, value string) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.SetText(lang, key, field, value, "agent") })
}
func AddRelation(workspace, project, from, to, relType string) (string, error) {
	m, err := LoadModel(workspace, project)
	if err != nil {
		return "", err
	}
	id, err := m.AddRelation(from, to, relType, "agent")
	if err != nil {
		return "", err
	}
	return id, m.Save()
}
func AddRelationType(workspace, project, id, visibility, styleID string) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.AddRelationType(id, visibility, styleID, "agent") })
}
func SetRelationVisible(workspace, project, viewID, relationID string, visible bool) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.SetRelationVisible(viewID, relationID, visible, "agent") })
}
func ConfirmEntityRename(workspace, project, entityID, symbol string) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.ConfirmEntityRename(entityID, symbol, "agent") })
}
func ConfirmRelationRename(workspace, project, relationID, member string) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.ConfirmRelationRename(relationID, member, "agent") })
}
func PlaceEntities(workspace, project, viewID string, list []Placement, requestedByHuman bool) error {
	return editAndSave(workspace, project, func(m *Model) error { return m.PlaceEntities(viewID, list, requestedByHuman, "agent") })
}

// ------------------------------------------------------------------ reading

// Records returns the items of a registry file of the project as raw JSON
// objects, in file order: entities.json → "entities", relations.json →
// "relations", relation-types.json → "relationTypes".
func Records(workspace, project, file string) ([]json.RawMessage, error) {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return nil, err
	}
	key := map[string]string{"entities.json": "entities", "relations.json": "relations", "relation-types.json": "relationTypes"}[file]
	if key == "" {
		return nil, fmt.Errorf("not a registry file: %s", file)
	}
	reg, err := loadRegistry(dir, file, key, freshList(key))
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, len(reg.items))
	for i, o := range reg.items {
		out[i], _ = o.MarshalJSON()
	}
	return out, nil
}

// Text returns the entry of a key in text.<lang>.json; nil when there is none.
func Text(workspace, project, lang, key string) (json.RawMessage, error) {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return nil, err
	}
	doc, err := loadDoc(filepath.Join(dir, "text."+lang+".json"))
	if err != nil || doc == nil {
		return nil, err
	}
	entries, err := child(doc, "entries")
	if err != nil {
		return nil, err
	}
	return entries.vals[key], nil
}

// ------------------------------------------------------------------ helpers

func findByID(items []*object, id string) *object {
	for _, o := range items {
		if o.str("id") == id {
			return o
		}
	}
	return nil
}

func freshList(key string) func() *object {
	return func() *object {
		o := newObject()
		o.set("contractVersion", 3)
		o.set(key, []any{})
		return o
	}
}

// child is o[key] as an ordered object; an empty one when absent.
func child(o *object, key string) (*object, error) {
	c := newObject()
	if raw, ok := o.vals[key]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// loadDoc reads a JSON file as an ordered object; nil when it does not exist.
func loadDoc(file string) (*object, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o := newObject()
	if err := json.Unmarshal(data, o); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	return o, nil
}

func saveDoc(file string, o *object) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(o); err != nil {
		return err
	}
	return os.WriteFile(file, b.Bytes(), 0o644)
}

// loadView finds a view by its id: the `id` inside wins over the file name
// (CONTRACT §8.1).
func loadView(dir, viewID string) (string, *object, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "views", "*.view.json"))
	for _, f := range files {
		v, err := loadDoc(f)
		if err != nil {
			return "", nil, err
		}
		id := v.str("id")
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(f), ".view.json")
		}
		if id == viewID {
			return f, v, nil
		}
	}
	return "", nil, refuse("no view %s", viewID)
}
