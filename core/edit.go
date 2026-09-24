package core

// Edits of the registry made on request, by an agent through `semaps mcp` or
// by any other frontend (ADR_20260924-5). Every promise the server makes lives
// here, not in the transport:
//
//   - ids are minted by core, never passed in;
//   - texts are written with origin: authored and `at` in UTC (CONTRACT §7.3),
//     never under a code relation (§4);
//   - nothing is deleted: there is no function for it;
//   - geometry in views/ is written only on a human's request (§8.2 p. 3).
//
// Files are read and written as ordered objects, like sync does: every key a
// file had stays where it was.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// EditError is a refused edit: the request breaks a rule of the contract.
// The message says which one; nothing was written.
type EditError struct{ msg string }

func (e *EditError) Error() string { return e.msg }

func refuse(format string, a ...any) error { return &EditError{fmt.Sprintf(format, a...)} }

// ProjectDir resolves the project an edit goes to: the one named, or the only
// one in the workspace.
func ProjectDir(workspace, project string) (string, error) {
	id, err := pickProject(workspace, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspace, "projects", id), nil
}

// now is the `at` of a text written now; a variable for tests.
var now = func() time.Time { return time.Now().UTC() }

// TextFields are the fields a text entry may carry (CONTRACT §7.2).
var TextFields = []string{"name", "title", "description", "doc", "fromLabel", "toLabel"}

// SetText writes one field of one key in text.<lang>.json as authored: `v`,
// `at` now, `origin: authored`; `from`/`fromHash` of a former translation go.
func SetText(workspace, project, lang, key, field, value string) error {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	if lang == "" || strings.ContainsAny(lang, `/\.`) {
		return refuse("language %q: expected a code like ru or en", lang)
	}
	if !slices.Contains(TextFields, field) {
		return refuse("field %q: allowed %s (CONTRACT §7.2)", field, strings.Join(TextFields, ", "))
	}
	if strings.TrimSpace(value) == "" {
		return refuse("empty text; there is no deleting a text")
	}
	switch {
	case strings.HasPrefix(key, "e_"):
		if field == "name" || field == "title" {
			return refuse("an entity's name is not translated and lives in entities.json (CONTRACT §7.1)")
		}
	case strings.HasPrefix(key, "r_"):
		rels, err := loadRegistry(dir, "relations.json", "relations", freshList("relations"))
		if err != nil {
			return err
		}
		r := findByID(rels.items, key)
		if r == nil {
			return refuse("no relation %s", key)
		}
		if r.str("origin") == "code" {
			return refuse("relation %s comes from code: it has no texts, its label is drawn from `via` (CONTRACT §4)", key)
		}
	case strings.HasPrefix(key, "c_"), strings.HasPrefix(key, "rt_"), strings.HasPrefix(key, "v_"), strings.HasPrefix(key, "z_"):
	default:
		return refuse("key %q: expected a prefix e_, c_, rt_, r_, v_ or z_ (CONTRACT §7.1)", key)
	}

	file := filepath.Join(dir, "text."+lang+".json")
	doc, err := loadDoc(file)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = newObject()
		doc.set("contractVersion", 3)
		doc.set("language", lang)
	}
	entries, err := child(doc, "entries")
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	entry, err := child(entries, key)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", filepath.Base(file), key, err)
	}
	v := newObject()
	v.set("v", value)
	v.set("at", now().Format("2006-01-02T15:04:05Z"))
	v.set("origin", "authored")
	entry.set(field, v)
	entries.set(key, entry)
	doc.set("entries", entries)
	return saveDoc(file, doc)
}

// AddRelation adds an authored relation and returns its id,
// r_<from>_<to>_<type>, `_2` on collision. Both entities and the type must be
// in the registry: a relation of an unknown type has no name and helps nobody.
func AddRelation(workspace, project, from, to, relType string) (string, error) {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return "", err
	}
	ents, err := loadRegistry(dir, "entities.json", "entities", freshList("entities"))
	if err != nil {
		return "", err
	}
	for _, e := range []string{from, to} {
		if findByID(ents.items, e) == nil {
			return "", refuse("no entity %s", e)
		}
	}
	if from == to {
		return "", refuse("a relation of an entity to itself is not drawn")
	}
	types, err := loadRegistry(dir, "relation-types.json", "relationTypes", freshList("relationTypes"))
	if err != nil {
		return "", err
	}
	if findByID(types.items, relType) == nil {
		return "", refuse("no relation type %s; add it first (add_relation_type)", relType)
	}
	rels, err := loadRegistry(dir, "relations.json", "relations", freshList("relations"))
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, r := range rels.items {
		taken[r.str("id")] = true
		if r.str("from") == from && r.str("to") == to && relationType(r) == relType && r.str("origin") != "code" {
			return "", refuse("relation %s already says %s → %s (%s)", r.str("id"), from, to, relType)
		}
	}
	id := mint("r_"+strings.TrimPrefix(from, "e_")+"_"+strings.TrimPrefix(to, "e_")+"_"+slug(relType), taken)
	o := newObject()
	o.set("id", id)
	o.set("from", from)
	o.set("to", to)
	o.set("type", relType)
	o.set("origin", "authored")
	rels.items = append(rels.items, o)
	return id, rels.save()
}

// AddRelationType adds an authored type. visibility is "", visible or hidden.
func AddRelationType(workspace, project, id, visibility, styleID string) error {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	if id == "" || strings.ContainsAny(id, " \t\"") {
		return refuse("type id %q: a word without spaces", id)
	}
	if visibility != "" && visibility != "visible" && visibility != "hidden" {
		return refuse("visibility %q: visible, hidden or nothing", visibility)
	}
	types, err := loadRegistry(dir, "relation-types.json", "relationTypes", freshList("relationTypes"))
	if err != nil {
		return err
	}
	if findByID(types.items, id) != nil {
		return refuse("type %s exists", id)
	}
	o := newObject()
	o.set("id", id)
	o.set("origin", "authored")
	if visibility != "" {
		o.set("visibility", visibility)
	}
	if styleID != "" {
		o.set("styleId", styleID)
	}
	types.items = append(types.items, o)
	return types.save()
}

// SetRelationVisible decides one relation on one view: it goes into
// relations.except when the wish is against the default (the type's
// visibility, else the view's relations.default), and out of it otherwise
// (CONTRACT §8.5). Geometry is not touched.
func SetRelationVisible(workspace, project, viewID, relationID string, visible bool) error {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	rels, err := loadRegistry(dir, "relations.json", "relations", freshList("relations"))
	if err != nil {
		return err
	}
	r := findByID(rels.items, relationID)
	if r == nil {
		return refuse("no relation %s", relationID)
	}
	file, view, err := loadView(dir, viewID)
	if err != nil {
		return err
	}
	policy, err := child(view, "relations")
	if err != nil {
		return fmt.Errorf("%s: relations: %w", filepath.Base(file), err)
	}
	def := policy.str("default")
	types, err := loadRegistry(dir, "relation-types.json", "relationTypes", freshList("relationTypes"))
	if err != nil {
		return err
	}
	if t := findByID(types.items, relationType(r)); t != nil && t.str("visibility") != "" {
		def = t.str("visibility")
	}
	if def == "" {
		def = "visible"
	}
	var except []string
	if raw, ok := policy.vals["except"]; ok {
		if err := json.Unmarshal(raw, &except); err != nil {
			return fmt.Errorf("%s: relations.except: %w", filepath.Base(file), err)
		}
	}
	except = slices.DeleteFunc(except, func(s string) bool { return s == relationID })
	if visible != (def == "visible") {
		except = append(except, relationID)
	}
	if except == nil {
		except = []string{}
	}
	policy.set("except", except)
	view.set("relations", policy)
	return saveDoc(file, view)
}

// ConfirmEntityRename answers a «переименование?» of sync for an entity: the
// old entity takes the new symbol and keeps its id (EXTRACTOR §5).
func ConfirmEntityRename(workspace, project, entityID, symbol string) error {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	if symbol == "" {
		return refuse("symbol is empty")
	}
	ents, err := loadRegistry(dir, "entities.json", "entities", freshList("entities"))
	if err != nil {
		return err
	}
	e := findByID(ents.items, entityID)
	if e == nil {
		return refuse("no entity %s", entityID)
	}
	if e.str("origin") == "authored" {
		return refuse("entity %s is authored: it has no symbol", entityID)
	}
	e.set("symbol", symbol)
	return ents.save()
}

// ConfirmRelationRename answers a «переименование?» for a member relation:
// the old relation takes the new member name and keeps its id (ADR_20260924-4 §6).
func ConfirmRelationRename(workspace, project, relationID, member string) error {
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	if member == "" {
		return refuse("member is empty")
	}
	rels, err := loadRegistry(dir, "relations.json", "relations", freshList("relations"))
	if err != nil {
		return err
	}
	r := findByID(rels.items, relationID)
	if r == nil {
		return refuse("no relation %s", relationID)
	}
	via, err := child(r, "via")
	if err != nil || len(via.keys) == 0 {
		return refuse("relation %s has no via: it is not a member relation", relationID)
	}
	via.set("member", member)
	r.set("via", via)
	return rels.save()
}

// Placement is one entity put on a view.
type Placement struct {
	Entity string  `json:"entity"`
	Zone   string  `json:"zone,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

// PlaceEntities puts entities on a view — only on a human's request
// (ADR_20260924_contract). What is already placed stays where it is: an
// entity that is on the view is refused, not moved.
func PlaceEntities(workspace, project, viewID string, list []Placement, requestedByHuman bool) error {
	if !requestedByHuman {
		return refuse("geometry of a view is written only on a human's direct request (CONTRACT §8.2 p. 3)")
	}
	dir, err := ProjectDir(workspace, project)
	if err != nil {
		return err
	}
	ents, err := loadRegistry(dir, "entities.json", "entities", freshList("entities"))
	if err != nil {
		return err
	}
	file, view, err := loadView(dir, viewID)
	if err != nil {
		return err
	}
	key := "nodes"
	if _, ok := view.vals["nodes"]; !ok {
		if _, ok := view.vals["placements"]; ok {
			key = "placements"
		}
	}
	var nodes []*object
	if raw, ok := view.vals[key]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return fmt.Errorf("%s: %s: %w", filepath.Base(file), key, err)
		}
	}
	placed := map[string]bool{}
	for _, n := range nodes {
		placed[orDefault(n.str("entity"), n.str("id"))] = true
	}
	zones := map[string]bool{}
	if raw, ok := view.vals["zones"]; ok {
		var zs []*object
		if err := json.Unmarshal(raw, &zs); err == nil {
			for _, z := range zs {
				zones[z.str("id")] = true
			}
		}
	}
	for _, p := range list {
		switch {
		case findByID(ents.items, p.Entity) == nil:
			return refuse("no entity %s", p.Entity)
		case placed[p.Entity]:
			return refuse("%s is already on %s; a placement made by someone is not moved", p.Entity, viewID)
		case p.Zone != "" && !zones[p.Zone]:
			return refuse("no zone %s on %s", p.Zone, viewID)
		}
		placed[p.Entity] = true
		n := newObject()
		n.set("entity", p.Entity)
		if p.Zone != "" {
			n.set("zone", p.Zone)
		} else {
			n.set("zone", nil)
		}
		n.set("x", p.X)
		n.set("y", p.Y)
		if p.Width > 0 {
			n.set("width", p.Width)
		}
		if p.Height > 0 {
			n.set("height", p.Height)
		}
		nodes = append(nodes, n)
	}
	view.set(key, nodes)
	return saveDoc(file, view)
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
