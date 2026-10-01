package core

// Model is the unsaved, ordered project document shared by all frontends.
// Contract files are written only by Save; an accepted batch is durable in the
// work journal before it becomes visible to callers.
import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	stdsync "sync"
)

type Op struct {
	Kind   string          `json:"kind"`
	ID     string          `json:"id"`
	View   string          `json:"view,omitempty"`
	Lang   string          `json:"lang,omitempty"`
	Value  json.RawMessage `json:"value"`
	Author string          `json:"author,omitempty"`
}

type Ref struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	View   string `json:"view,omitempty"`
	Lang   string `json:"lang,omitempty"`
	Author string `json:"author"`
}

type DirtySummary struct {
	Registry []Ref            `json:"registry"`
	Views    map[string][]Ref `json:"views"`
}

type modelView struct {
	file string
	doc  *object
}

type Model struct {
	mu stdsync.Mutex
	// editMu serialises the edits that read a value, change it and write it back
	// through Apply (which takes mu only for the write): without it two calls at
	// once both read the same old value and the later write loses the earlier
	// one (SetRelationsVisible).
	editMu stdsync.Mutex

	workspace  string
	project    string
	dir        string
	manifest   *object
	registries map[string]*registry
	texts      map[string]*object
	views      map[string]*modelView
	loaded     map[string]bool
	journal    [][]Op
	dirty      DirtySummary
	canvas     Canvas // given by the host (SetCanvas); geometry needs it
	// kinds is the merged dictionary (CONTRACT §6); defaultKinds the tool's
	// default it was merged from, kept for Discard's reload.
	kinds        *KindCatalog
	defaultKinds []byte
}

var modelRegistries = map[string]struct{ file, key string }{
	"entity":   {"entities.json", "entities"},
	"relation": {"relations.json", "relations"},
}

// LoadModel loads a project with its work journal. defaultKinds is the tool's
// dictionary (host/defaults/kinds.json); <workspace>/kinds.json adds to it.
func LoadModel(workspace, project string, defaultKinds []byte) (*Model, error) {
	m, err := LoadModelWithoutJournal(workspace, project, defaultKinds)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(m.journalFile())
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for s.Scan() {
		var batch []Op
		if err := json.Unmarshal(s.Bytes(), &batch); err != nil {
			return nil, fmt.Errorf("work journal: %w", err)
		}
		if _, err := m.apply(batch, "", false); err != nil {
			return nil, fmt.Errorf("work journal: %w", err)
		}
		m.journal = append(m.journal, batch)
	}
	return m, s.Err()
}

func (m *Model) journalFile() string {
	return filepath.Join(m.workspace, ".semaps", "work", m.project+".jsonl")
}

func cloneObject(o *object) *object {
	b, _ := o.MarshalJSON()
	c := newObject()
	_ = json.Unmarshal(b, c)
	return c
}

func (m *Model) copy() *Model {
	c := &Model{workspace: m.workspace, project: m.project, dir: m.dir, manifest: cloneObject(m.manifest), canvas: m.canvas, kinds: m.kinds, defaultKinds: m.defaultKinds,
		registries: map[string]*registry{}, texts: map[string]*object{}, views: map[string]*modelView{}, loaded: map[string]bool{},
		journal: append([][]Op(nil), m.journal...), dirty: DirtySummary{Registry: append([]Ref(nil), m.dirty.Registry...), Views: map[string][]Ref{}}}
	for k, r := range m.registries {
		n := *r
		n.top = cloneObject(r.top)
		n.items = make([]*object, len(r.items))
		for i, o := range r.items {
			n.items[i] = cloneObject(o)
		}
		c.registries[k] = &n
	}
	for k, o := range m.texts {
		c.texts[k] = cloneObject(o)
	}
	for k, v := range m.views {
		c.views[k] = &modelView{file: v.file, doc: cloneObject(v.doc)}
	}
	for k, v := range m.loaded {
		c.loaded[k] = v
	}
	for k, v := range m.dirty.Views {
		c.dirty.Views[k] = append([]Ref(nil), v...)
	}
	return c
}

func (m *Model) loadView(id string) (*modelView, error) {
	if v := m.views[id]; v != nil {
		return v, nil
	}
	file, doc, err := loadView(m.dir, id)
	if err != nil {
		return nil, err
	}
	if old := oldViewShape(doc); old != "" {
		return nil, refuse("%s", oldShapeError("views/"+filepath.Base(file), old))
	}
	v := &modelView{file: file, doc: doc}
	m.views[id] = v
	return v, nil
}

func (m *Model) loadText(lang string) (*object, error) {
	if strings.ContainsAny(lang, `/\.`) || lang == "" {
		return nil, refuse("language %q: expected a code like ru or en", lang)
	}
	if m.loaded[lang] {
		return m.texts[lang], nil
	}
	o, err := loadDoc(filepath.Join(m.dir, "text."+lang+".json"))
	if err != nil {
		return nil, err
	}
	if o == nil {
		o = newObject()
		o.set("contractVersion", ContractVersion)
		o.set("language", lang)
		o.set("entries", newObject())
	}
	m.texts[lang], m.loaded[lang] = o, true
	return o, nil
}

// Apply validates a complete batch on a copy — its operations, then the
// contract rules for what the batch created or changed (rules.go,
// ADR_20260926). No partial mutation or journal line is produced when one
// operation or rule fails.
func (m *Model) Apply(ops []Op, author string) ([]Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(ops) == 0 {
		return []Ref{}, nil
	}
	ops = append([]Op(nil), ops...)
	if author != "" {
		for i := range ops {
			ops[i].Author = author
		}
	}
	c := m.copy()
	refs, err := c.apply(ops, "", true)
	if err != nil {
		return nil, err
	}
	if err := c.validate(m, ops); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(m.journalFile()), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(m.journalFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	b := append(encodeJSON(ops), '\n')
	_, err = f.Write(b)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return nil, err
	}
	m.manifest, m.registries, m.texts, m.views, m.loaded, m.journal, m.dirty = c.manifest, c.registries, c.texts, c.views, c.loaded, c.journal, c.dirty
	return refs, nil
}

func (m *Model) apply(ops []Op, author string, record bool) ([]Ref, error) {
	refs := make([]Ref, 0, len(ops))
	batch := make([]Op, len(ops))
	for i, op := range ops {
		if author != "" {
			op.Author = author
		}
		if op.Author == "" {
			return nil, refuse("operation author is empty")
		}
		if op.ID == "" {
			return nil, refuse("operation id is empty")
		}
		if len(op.Value) == 0 {
			return nil, refuse("operation value is missing")
		}
		if !json.Valid(op.Value) {
			return nil, refuse("operation value is invalid JSON")
		}
		if err := m.applyOne(op); err != nil {
			return nil, err
		}
		ref := Ref{Kind: op.Kind, ID: op.ID, View: op.View, Lang: op.Lang, Author: op.Author}
		refs = append(refs, ref)
		if _, reg := modelRegistries[op.Kind]; reg && bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
			// a withdrawn record is not unsaved: withdraw took it off the list
		} else if op.View == "" {
			m.dirty.Registry = upsertRef(m.dirty.Registry, ref)
		} else {
			m.dirty.Views[op.View] = upsertRef(m.dirty.Views[op.View], ref)
		}
		batch[i] = op
	}
	if record {
		m.journal = append(m.journal, batch)
	}
	return refs, nil
}

func upsertRef(list []Ref, ref Ref) []Ref {
	for i, old := range list {
		if old.Kind == ref.Kind && old.ID == ref.ID && old.Lang == ref.Lang {
			list[i] = ref
			return list
		}
	}
	return append(list, ref)
}

// orderedReplacement keeps the surviving keys in their original positions.
// This is only serialization order: values still come entirely from next.
func orderedReplacement(old, next *object) *object {
	out := newObject()
	for _, key := range old.keys {
		if value, ok := next.vals[key]; ok {
			out.keys = append(out.keys, key)
			out.vals[key] = value
		}
	}
	for _, key := range next.keys {
		if _, ok := out.vals[key]; !ok {
			out.keys = append(out.keys, key)
			out.vals[key] = next.vals[key]
		}
	}
	return out
}

func (m *Model) applyOne(op Op) error {
	if op.Kind == "project" {
		if op.View != "" || op.ID != m.project || bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
			return refuse("invalid project operation")
		}
		o := newObject()
		if err := json.Unmarshal(op.Value, o); err != nil {
			return err
		}
		if o.str("id") != m.project {
			return refuse("project id differs from folder; use structural rename")
		}
		// contractVersion is the loader's, not the editor's: kept when left out
		if _, ok := o.vals["contractVersion"]; !ok {
			if cv, ok := m.manifest.vals["contractVersion"]; ok {
				o.set("contractVersion", cv)
			}
		} else if err := checkContractVersion(o); err != nil {
			return err
		}
		m.manifest = orderedReplacement(m.manifest, o)
		return nil
	}
	if spec, ok := modelRegistries[op.Kind]; ok {
		if op.View != "" {
			return refuse("registry %s cannot be scoped to a view", op.Kind)
		}
		if bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
			return m.withdraw(op)
		}
		o := newObject()
		if err := json.Unmarshal(op.Value, o); err != nil {
			return err
		}
		if o.str("id") != op.ID {
			return refuse("%s id differs from operation id", spec.key)
		}
		r := m.registries[op.Kind]
		for i, item := range r.items {
			if item.str("id") == op.ID {
				r.items[i] = orderedReplacement(item, o)
				r.dirty = true
				return nil
			}
		}
		r.items = append(r.items, o)
		r.dirty = true
		return nil
	}
	if op.Kind == "text" {
		if op.View != "" || bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
			return refuse("registry text cannot be removed or scoped to a view")
		}
		switch {
		case strings.HasPrefix(op.ID, "e_"):
		case strings.HasPrefix(op.ID, "r_"):
			r := findByID(m.registries["relation"].items, op.ID)
			if r == nil {
				return refuse("no relation %s", op.ID)
			}
			if r.str("origin") == "code" {
				return refuse("relation %s comes from code: it has no texts, its label is drawn from the `via` of its evidence (CONTRACT §4)", op.ID)
			}
		case strings.HasPrefix(op.ID, "v_"):
		default:
			return refuse("key %q: expected a prefix e_, r_ or v_ (CONTRACT §7.1)", op.ID)
		}
		doc, err := m.loadText(op.Lang)
		if err != nil {
			return err
		}
		entry := newObject()
		if err := json.Unmarshal(op.Value, entry); err != nil {
			return err
		}
		entries, err := child(doc, "entries")
		if err != nil {
			return err
		}
		old, err := child(entries, op.ID)
		if err != nil {
			return err
		}
		for _, field := range entry.keys {
			if !slices.Contains(TextFields, field) {
				return refuse("field %q: allowed %s (CONTRACT §7.2)", field, strings.Join(TextFields, ", "))
			}
		}
		if len(old.keys) > 0 {
			entry = orderedReplacement(old, entry)
		}
		entries.set(op.ID, entry)
		doc.set("entries", entries)
		return nil
	}
	if op.Kind != "view" && op.Kind != "placement" {
		return refuse("unknown operation kind %q", op.Kind)
	}
	if op.View == "" {
		return refuse("%s needs a view", op.Kind)
	}
	v, err := m.loadView(op.View)
	if err != nil {
		return err
	}
	if op.Kind == "view" {
		if op.ID != op.View || bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
			return refuse("invalid view operation")
		}
		n := newObject()
		if err := json.Unmarshal(op.Value, n); err != nil {
			return err
		}
		if n.str("id") != op.View {
			return refuse("view id differs from operation id")
		}
		for _, key := range []string{"placements", "zones", "nodes"} {
			if _, ok := n.vals[key]; ok {
				return refuse("view properties cannot carry `%s`: placements change one by one, op kind placement", key)
			}
		}
		for _, key := range n.keys {
			if string(n.vals[key]) == "null" {
				v.doc.del(key)
			} else {
				v.doc.set(key, n.vals[key])
			}
		}
		return nil
	}
	items := viewItems(v.doc, "placements")
	index := -1
	for i, item := range items {
		if item.str("entity") == op.ID {
			index = i
			break
		}
	}
	if bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
		if index >= 0 {
			items = append(items[:index], items[index+1:]...)
		}
	} else {
		o := newObject()
		if err := json.Unmarshal(op.Value, o); err != nil {
			return err
		}
		if o.str("entity") != op.ID {
			return refuse("placement entity differs from operation id")
		}
		if index >= 0 {
			items[index] = orderedReplacement(items[index], o)
		} else {
			items = append(items, o)
		}
	}
	if items == nil {
		items = []*object{}
	}
	v.doc.set("placements", items)
	return nil
}

// withdraw removes a record created in the unsaved working state, as if it had
// never been created (ADR_20260930-8): from the registry, from the dirty list,
// and with it every text under its id in the loaded catalogues (a text has no
// life of its own: the same batch takes it away). A record of the saved file is
// never removed. Withdrawing what is not there is not an error: a journal replay
// after a partial Discard meets it. Whether a view or a relation still names
// the record is the batch rule's (checkWithdrawn), not this one's.
func (m *Model) withdraw(op Op) error {
	r := m.registries[op.Kind]
	if r.saved[op.ID] {
		return refuse("%s %s is in the saved registry: a saved record is never removed; only one created in the unsaved state can be withdrawn (API.md §3.4)", op.Kind, op.ID)
	}
	for i, item := range r.items {
		if item.str("id") == op.ID {
			r.items = append(r.items[:i:i], r.items[i+1:]...)
			break
		}
	}
	keep := m.dirty.Registry[:0:0]
	for _, ref := range m.dirty.Registry {
		if ref.ID == op.ID && (ref.Kind == op.Kind || ref.Kind == "text") {
			continue
		}
		keep = append(keep, ref)
	}
	m.dirty.Registry = keep
	// the registry is dirty while something else of its kind is still unsaved
	r.dirty = false
	for _, ref := range keep {
		if ref.Kind == op.Kind {
			r.dirty = true
		}
	}
	for _, doc := range m.texts {
		if entries, err := child(doc, "entries"); err == nil {
			if _, ok := entries.vals[op.ID]; ok {
				entries.del(op.ID)
				doc.set("entries", entries)
			}
		}
	}
	return nil
}

func (m *Model) Dirty() DirtySummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := DirtySummary{Registry: append([]Ref{}, m.dirty.Registry...), Views: map[string][]Ref{}}
	for k, v := range m.dirty.Views {
		d.Views[k] = append([]Ref{}, v...)
	}
	return d
}

func (m *Model) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ref := range m.dirty.Registry {
		if ref.Kind == "project" {
			if err := saveDoc(filepath.Join(m.dir, "project.json"), m.manifest); err != nil {
				return err
			}
			break
		}
	}
	for kind, r := range m.registries {
		if r.dirty {
			if err := r.save(); err != nil {
				return fmt.Errorf("%s: %w", kind, err)
			}
		}
	}
	for _, ref := range m.dirty.Registry {
		if ref.Kind == "text" {
			if err := saveDoc(filepath.Join(m.dir, "text."+ref.Lang+".json"), m.texts[ref.Lang]); err != nil {
				return err
			}
		}
	}
	for id := range m.dirty.Views {
		v := m.views[id]
		if err := os.MkdirAll(filepath.Dir(v.file), 0o755); err != nil {
			return err
		}
		if err := saveDoc(v.file, v.doc); err != nil {
			return err
		}
	}
	if err := os.Remove(m.journalFile()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	m.journal = nil
	m.dirty = DirtySummary{Views: map[string][]Ref{}}
	for _, r := range m.registries {
		r.dirty = false
		r.markSaved()
	}
	return nil
}

// Discard reloads contract files, then replays only operations outside scope.
func (m *Model) Discard(scope, view string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if scope != "all" && scope != "registry" && scope != "view" {
		return refuse("unknown discard scope %q", scope)
	}
	if scope == "view" && view == "" {
		return refuse("view id is empty")
	}
	fresh, err := LoadModelWithoutJournal(m.workspace, m.project, m.defaultKinds)
	if err != nil {
		return err
	}
	for _, batch := range m.journal {
		keep := []Op{}
		for _, op := range batch {
			if scope == "all" || (scope == "registry" && op.View == "") || (scope == "view" && op.View == view) {
				continue
			}
			keep = append(keep, op)
		}
		if len(keep) > 0 {
			if _, err := fresh.apply(keep, "", true); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.journalFile()), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	for _, batch := range fresh.journal {
		b.Write(encodeJSON(batch))
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		if err := os.Remove(m.journalFile()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	} else if err := os.WriteFile(m.journalFile(), b.Bytes(), 0o644); err != nil {
		return err
	}
	m.manifest, m.registries, m.texts, m.views, m.loaded, m.journal, m.dirty = fresh.manifest, fresh.registries, fresh.texts, fresh.views, fresh.loaded, fresh.journal, fresh.dirty
	return nil
}

func LoadModelWithoutJournal(workspace, project string, defaultKinds []byte) (*Model, error) {
	id, err := pickProject(workspace, project)
	if err != nil {
		return nil, err
	}
	kinds, err := LoadKinds(workspace, defaultKinds)
	if err != nil {
		return nil, err
	}
	m := &Model{workspace: workspace, project: id, dir: filepath.Join(workspace, "projects", id), registries: map[string]*registry{}, texts: map[string]*object{}, views: map[string]*modelView{}, loaded: map[string]bool{}, dirty: DirtySummary{Views: map[string][]Ref{}},
		kinds: kinds, defaultKinds: defaultKinds}
	m.manifest, err = loadDoc(filepath.Join(m.dir, "project.json"))
	if err != nil {
		return nil, err
	}
	if m.manifest == nil {
		return nil, refuse("no project manifest %s", id)
	}
	if err := checkContractVersion(m.manifest); err != nil {
		return nil, err
	}
	if exists(filepath.Join(m.dir, "containers.json")) {
		return nil, refuse("%s", containersFileError)
	}
	if exists(filepath.Join(m.dir, RelationTypesFile)) {
		return nil, refuse("%s", relationTypesFileError)
	}
	for kind, spec := range modelRegistries {
		r, err := loadRegistry(m.dir, spec.file, spec.key, freshList(spec.key))
		if err != nil {
			return nil, err
		}
		m.registries[kind] = r
		for _, item := range r.items {
			if old := oldShape(kind, item); old != "" {
				return nil, refuse("%s: %s: %s — форма до ADR_20260930-4/5, нужна текущая (`semaps migrate`, ADR_20260927-3)", spec.file, item.str("id"), old)
			}
		}
	}
	return m, nil
}

// checkContractVersion refuses a project.json of any other contract version,
// naming the file, the field and the version (ADR_20260927-3).
func checkContractVersion(manifest *object) error {
	var v struct {
		ContractVersion *int `json:"contractVersion"`
	}
	raw, _ := manifest.MarshalJSON()
	if err := json.Unmarshal(raw, &v); err != nil {
		return refuse("project.json: contractVersion: %v", err)
	}
	if v.ContractVersion == nil {
		return refuse("project.json: contractVersion is missing — нужен %d (`semaps migrate`, ADR_20260927-3)", ContractVersion)
	}
	if *v.ContractVersion != ContractVersion {
		return refuse("project.json: contractVersion %d — форма контракта %d, нужен %d (`semaps migrate`, ADR_20260927-3)", *v.ContractVersion, *v.ContractVersion, ContractVersion)
	}
	return nil
}

// Kinds is the merged dictionary of the workspace.
func (m *Model) Kinds() *KindCatalog { return m.kinds }
