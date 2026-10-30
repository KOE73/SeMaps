package core

// WorkspaceModel owns the shared stylesheet and coordinates its references in
// project models. A checkpoint replaces the journals of attached projects so a
// rename is durable as one transaction, including projects nobody has open.
import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	stdsync "sync"
)

type workspaceWork struct {
	Revision uint64            `json:"revision"`
	Styles   json.RawMessage   `json:"styles"`
	Kinds    json.RawMessage   `json:"kinds,omitempty"`
	Dirty    []Ref             `json:"dirty"`
	Projects map[string][][]Op `json:"projects"`
}

type WorkspaceModel struct {
	mu                                     stdsync.Mutex
	root                                   string
	defaults, defaultKinds                 []byte
	styles, savedStyles, kinds, savedKinds *object
	dirty                                  []Ref
	journals                               map[string][][]Op
	models                                 map[string]*Model
	revision                               uint64
}

type styleReferencePatch struct {
	NewID       string                `json:"newId"`
	References  []Ref                 `json:"references"`
	Field       string                `json:"field,omitempty"`
	Replacement bool                  `json:"replacement,omitempty"`
	Values      []styleReferenceValue `json:"values,omitempty"`
}

type styleReferenceValue struct {
	View   string   `json:"view"`
	Kind   string   `json:"kind"`
	ID     string   `json:"id"`
	Field  string   `json:"field"`
	Before []string `json:"before"`
	After  []string `json:"after"`
}

func decodeStyleReference(op Op) styleReferencePatch {
	var patch styleReferencePatch
	if json.Unmarshal(op.Value, &patch) != nil {
		_ = json.Unmarshal(op.Value, &patch.NewID)
	}
	return patch
}

// Full placement values from ordinary edits also contain styleId. Transform
// that one field when a workspace rename is saved/undone; keep everything else.
func projectJournalWithoutStyleRefs(journal [][]Op, undo bool) [][]Op {
	changes := []struct {
		old, next string
		refs      []Ref
		field     string
		patch     styleReferencePatch
	}{}
	for _, batch := range journal {
		for _, op := range batch {
			if op.Kind == "style_ref" {
				patch := decodeStyleReference(op)
				changes = append(changes, struct {
					old, next string
					refs      []Ref
					field     string
					patch     styleReferencePatch
				}{op.ID, patch.NewID, patch.References, patch.Field, patch})
			}
		}
	}
	if undo {
		slices.Reverse(changes)
		for i := range changes {
			changes[i].old, changes[i].next = changes[i].next, changes[i].old
		}
	}
	result := [][]Op{}
	for _, batch := range journal {
		keep := []Op{}
		for _, op := range batch {
			if op.Kind == "style_ref" {
				continue
			}
			if (op.Kind == "placement" || op.Kind == "view") && string(op.Value) != "null" {
				doc := newObject()
				if json.Unmarshal(op.Value, doc) == nil {
					for _, change := range changes {
						if undo && change.patch.Replacement && change.field != "" {
							// A replacement already existed: reversing its ID globally would
							// change unrelated assignments and lose deduplicated original lists.
							for _, value := range change.patch.Values {
								if value.View != op.View {
									continue
								}
								target := doc
								edges := viewItems(doc, "edges")
								if value.Kind == "placement" && (op.Kind != "placement" || op.ID != value.ID) {
									continue
								}
								if value.Kind == "view" && op.Kind != "view" {
									continue
								}
								if value.Kind == "edge" {
									if op.Kind != "view" {
										continue
									}
									target = findByID(edges, value.ID)
								}
								// An explicitly edited assignment stays as the human chose it.
								if has(target, value.Field) && slices.Equal(stringList(target, value.Field), value.After) {
									target.set(value.Field, value.Before)
									if value.Kind == "edge" {
										doc.set("edges", edges)
									}
								}
							}
							continue
						}
						if change.field != "" {
							if op.Kind == "placement" && change.field == "modifier" {
								replaceReferenceList(doc, "modifiers", change.old, change.next)
								replaceReferenceList(doc, "disabledModifiers", change.old, change.next)
							}
							if op.Kind == "view" {
								if change.field == "rule" {
									replaceReferenceList(doc, "disabledStyleRules", change.old, change.next)
								}
								if change.field == "modifier" {
									edges := viewItems(doc, "edges")
									for _, edge := range edges {
										replaceReferenceList(edge, "modifiers", change.old, change.next)
										replaceReferenceList(edge, "disabledModifiers", change.old, change.next)
									}
									if len(edges) > 0 {
										doc.set("edges", edges)
									}
								}
							}
							continue
						}
						for _, ref := range change.refs {
							if ref.View != op.View {
								continue
							}
							if op.Kind == "placement" && ref.Kind == "placement" && ref.ID == op.ID && doc.str("styleId") == change.old {
								doc.set("styleId", change.next)
							}
							if op.Kind == "view" && ref.Kind == "view" {
								edges := viewItems(doc, "edges")
								if edge := findByID(edges, ref.ID); edge != nil && edge.str("styleId") == change.old {
									edge.set("styleId", change.next)
									doc.set("edges", edges)
								}
							}
						}
					}
					op.Value = encodeJSON(doc)
				}
			}
			keep = append(keep, op)
		}
		if len(keep) > 0 {
			result = append(result, keep)
		}
	}
	return result
}

func LoadWorkspaceModel(root string, defaultStyles, defaultKinds []byte) (*WorkspaceModel, error) {
	w := &WorkspaceModel{root: root, defaults: defaultStyles, defaultKinds: defaultKinds, models: map[string]*Model{}, journals: map[string][][]Op{}}
	var err error
	w.savedStyles, err = loadDoc(filepath.Join(root, "styles.json"))
	if err != nil {
		return nil, err
	}
	if w.savedStyles == nil {
		w.savedStyles = newObject()
		if err = json.Unmarshal(defaultStyles, w.savedStyles); err != nil {
			return nil, err
		}
	}
	w.savedKinds, err = loadDoc(filepath.Join(root, KindsFile))
	if err != nil {
		return nil, err
	}
	if w.savedKinds == nil {
		w.savedKinds = newObject()
	}
	w.styles, w.kinds = cloneObject(w.savedStyles), cloneObject(w.savedKinds)
	if err = checkStyleExtensions(w.styles); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(w.workFile())
	if errors.Is(err, fs.ErrNotExist) {
		return w, nil
	}
	if err != nil {
		return nil, err
	}
	var work workspaceWork
	if err = json.Unmarshal(b, &work); err != nil {
		return nil, fmt.Errorf("workspace work: %w", err)
	}
	if err = json.Unmarshal(work.Styles, w.styles); err != nil {
		return nil, err
	}
	if err = checkStyleExtensions(w.styles); err != nil {
		return nil, err
	}
	if len(work.Kinds) > 0 {
		if err = json.Unmarshal(work.Kinds, w.kinds); err != nil {
			return nil, err
		}
	}
	w.dirty = work.Dirty
	w.revision = work.Revision
	if work.Projects != nil {
		w.journals = work.Projects
	}
	return w, nil
}

func (w *WorkspaceModel) workFile() string {
	return filepath.Join(w.root, ".semaps", "work", "workspace.json")
}

// The temp file and rename ensure a failed write never exposes half a batch.
func (w *WorkspaceModel) persist(styles, kinds *object, dirty []Ref, journals map[string][][]Op) error {
	dir := filepath.Dir(w.workFile())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	EnsureSelfIgnore(filepath.Dir(dir))
	work := workspaceWork{Styles: encodeJSON(styles), Kinds: encodeJSON(kinds), Dirty: dirty, Projects: journals, Revision: w.revision + 1}
	f, err := os.CreateTemp(dir, "workspace-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(encodeJSON(work)); err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(name, w.workFile())
}

func copyJournals(src map[string][][]Op) map[string][][]Op {
	out := map[string][][]Op{}
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Called with the workspace gate held, before the project becomes visible.
func (w *WorkspaceModel) persistProject(id string, journal [][]Op) error {
	next := copyJournals(w.journals)
	next[id] = journal
	if err := w.persist(w.styles, w.kinds, w.dirty, next); err != nil {
		return err
	}
	w.journals = next
	return nil
}

func (w *WorkspaceModel) Project(id string) (*Model, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.project(id)
}
func (w *WorkspaceModel) project(id string) (*Model, error) {
	if m := w.models[id]; m != nil {
		return m, nil
	}
	var m *Model
	var err error
	if journal, ok := w.journals[id]; ok {
		m, err = LoadModelWithoutJournal(w.root, id, w.defaultKinds)
		if err != nil {
			return nil, err
		}
		for _, ops := range journal {
			if _, err = m.apply(ops, "", true); err != nil {
				return nil, err
			}
		}
	} else {
		m, err = LoadModel(w.root, id, w.defaultKinds)
		if err != nil {
			return nil, err
		}
	}
	m.shared = w
	w.models[id] = m
	return m, nil
}

func (w *WorkspaceModel) Evict(id string) { w.mu.Lock(); defer w.mu.Unlock(); delete(w.models, id) }

func (w *WorkspaceModel) allProjects() error {
	for _, p := range Index(w.root).Projects {
		if _, err := w.project(p.ID); err != nil {
			return err
		}
	}
	return nil
}

type StyleSnapshot struct {
	Kinds    *KindCatalog    `json:"kinds"`
	Sheet    json.RawMessage `json:"sheet"`
	Dirty    []Ref           `json:"dirty"`
	Revision uint64          `json:"revision"`
}

func (w *WorkspaceModel) Styles() StyleSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.snapshot()
}

type StyledView struct {
	View        json.RawMessage            `json:"view"`
	Styles      StyleSnapshot              `json:"styles"`
	Appearances map[string]StyleAppearance `json:"appearances"`
}

func (w *WorkspaceModel) View(project, view string) (StyledView, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	m, err := w.project(project)
	if err != nil {
		return StyledView{}, err
	}
	data, err := m.View(view)
	if err != nil {
		return StyledView{}, err
	}
	appearances, err := w.viewAppearances(m, view, nil)
	return StyledView{View: data, Styles: w.snapshot(), Appearances: appearances}, err
}
func (w *WorkspaceModel) snapshot() StyleSnapshot {
	catalog, _ := w.catalog()
	return StyleSnapshot{Kinds: catalog, Sheet: encodeJSON(w.styles), Dirty: append([]Ref{}, w.dirty...), Revision: w.revision}
}

// Style references are patches, never whole placements: rename/save/discard
// cannot replace geometry, parent, content templates or another user's edits.
func (m *Model) styleReference(old, next string) ([]Ref, error) {
	refs := []Ref{}
	for _, v := range m.allViewIDs() {
		view, err := m.loadView(v)
		if err != nil {
			return nil, err
		}
		for _, key := range []string{"placements", "edges"} {
			items := viewItems(view.doc, key)
			for _, item := range items {
				if item.str("styleId") == old {
					item.set("styleId", next)
					id := item.str("entity")
					kind := "placement"
					if key == "edges" {
						id = item.str("id")
						kind = "view"
					}
					refs = append(refs, Ref{Kind: kind, ID: id, View: v})
				}
			}
			view.doc.set(key, items)
		}
	}
	return refs, nil
}

func (m *Model) applyStyleReference(op Op) ([]Ref, error) {
	patch := decodeStyleReference(op)
	if patch.Field != "" {
		return m.extensionReference(op.ID, patch.NewID, patch.Field)
	}
	return m.styleReference(op.ID, patch.NewID)
}
func (m *Model) allViewIDs() []string {
	ids := []string{}
	files, _ := filepath.Glob(filepath.Join(m.dir, "views", "*"+ViewSuffix))
	for _, file := range files {
		doc, err := loadDoc(file)
		if err == nil {
			ids = append(ids, doc.str("id"))
		}
	}
	return ids
}

func (w *WorkspaceModel) SaveStyles() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.allProjects(); err != nil {
		return err
	}
	// Save only reference patches from workspace transactions, never unrelated
	// unsaved project edits. Then replay the remaining project journal.
	next := copyJournals(w.journals)
	freshModels := map[string]*Model{}
	for id, m := range w.models {
		m.mu.Lock()
		fresh, err := LoadModelWithoutJournal(w.root, id, w.defaultKinds)
		keep := projectJournalWithoutStyleRefs(m.journal, false)
		refs := []Ref{}
		if err == nil {
			for _, batch := range m.journal {
				for _, op := range batch {
					if op.Kind == "style_ref" {
						var changed []Ref
						changed, err = fresh.applyStyleReference(op)
						refs = append(refs, changed...)
					}
				}
			}
		}
		if err == nil {
			seen := map[string]bool{}
			for _, ref := range refs {
				if !seen[ref.View] {
					seen[ref.View] = true
					err = saveDoc(fresh.views[ref.View].file, fresh.views[ref.View].doc)
					if err != nil {
						break
					}
				}
			}
		}
		if err == nil {
			for _, ops := range keep {
				_, err = fresh.apply(ops, "", true)
				if err != nil {
					break
				}
			}
		}
		m.mu.Unlock()
		if err != nil {
			return err
		}
		next[id] = keep
		freshModels[id] = fresh
	}
	if err := saveDoc(filepath.Join(w.root, "styles.json"), w.styles); err != nil {
		return err
	}
	if string(encodeJSON(w.kinds)) != string(encodeJSON(w.savedKinds)) {
		if err := saveDoc(filepath.Join(w.root, KindsFile), w.kinds); err != nil {
			return err
		}
	}
	if err := w.persist(w.styles, w.kinds, []Ref{}, next); err != nil {
		return err
	}
	w.savedStyles, w.savedKinds = cloneObject(w.styles), cloneObject(w.kinds)
	w.dirty = nil
	w.journals = next
	for id, fresh := range freshModels {
		m := w.models[id]
		m.mu.Lock()
		m.adopt(fresh)
		m.mu.Unlock()
	}
	w.revision++
	return nil
}

func (m *Model) adopt(c *Model) {
	m.manifest, m.registries, m.texts, m.views, m.loaded, m.journal, m.dirty = c.manifest, c.registries, c.texts, c.views, c.loaded, c.journal, c.dirty
	m.libraries = c.libraries
}

func (w *WorkspaceModel) DiscardStyles() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.allProjects(); err != nil {
		return err
	}
	next := copyJournals(w.journals)
	copies := map[string]*Model{}
	for id, m := range w.models {
		m.mu.Lock()
		fresh, err := LoadModelWithoutJournal(w.root, id, w.defaultKinds)
		if err == nil {
			for _, keep := range projectJournalWithoutStyleRefs(m.journal, true) {
				if len(keep) > 0 {
					_, err = fresh.apply(keep, "", true)
					if err != nil {
						break
					}
				}
			}
		}
		m.mu.Unlock()
		if err != nil {
			return err
		}
		// A newly assigned style has no saved definition: report the references,
		// require the project's assignment to be discarded first.
		if err = w.checkAllReferences(fresh, w.savedStyles); err != nil {
			return refuse("discard styles: %v; discard the project assignments first", err)
		}
		copies[id] = fresh
		next[id] = fresh.journal
	}
	if err := w.persist(w.savedStyles, w.savedKinds, []Ref{}, next); err != nil {
		return err
	}
	w.styles, w.kinds = cloneObject(w.savedStyles), cloneObject(w.savedKinds)
	w.dirty = nil
	w.journals = next
	for id, c := range copies {
		m := w.models[id]
		m.mu.Lock()
		m.adopt(c)
		m.mu.Unlock()
	}
	w.revision++
	return nil
}
