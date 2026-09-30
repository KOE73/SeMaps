package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"semaps/core"
)

type modelEvent struct {
	Client          string            `json:"client"`
	Author          string            `json:"author"`
	Changed         []core.Ref        `json:"changed"`
	Dirty           core.DirtySummary `json:"dirty"`
	ProjectReloaded *projectReloaded  `json:"projectReloaded,omitempty"`
	// Graph carries what changed in the live code graph after a run
	// (PLAN_20260928_host_graph-provider.md step 5), never the graph
	// itself. It is a field the current editor's ModelEvent interface does
	// not declare (editor/src/editor/io/HostModelStore.ts): a TypeScript
	// `as ModelEvent` cast does not validate at runtime, so an event with
	// only `graph` set and no other content the editor understands is read
	// as an ordinary (empty) change and safely ignored.
	Graph *core.GraphDiff `json:"graph,omitempty"`
	// Render asks an editor that subscribed with render=1 to draw a picture of
	// a view (the render_view tool, host/render.go). Only those clients get
	// such an event.
	Render *renderRequest `json:"render,omitempty"`
}

type projectReloaded struct {
	OldProject string `json:"oldProject"`
	NewProject string `json:"newProject"`
	OldView    string `json:"oldView,omitempty"`
	NewView    string `json:"newView,omitempty"`
}

type modelService struct {
	workspace   string
	key         string
	mu          sync.Mutex
	structureMu sync.RWMutex
	models      map[string]*core.Model
	clients     map[string]map[chan modelEvent]struct{}
	// renderers: the subscribed channels (a subset of clients) of editors that
	// can render; guarded by mu, like clients.
	renderers map[chan modelEvent]bool
	// pending render requests by id (render.go), guarded by renderMu.
	renderMu sync.Mutex
	pending  map[string]*renderCall
}

func newModelService(workspace string) (*modelService, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &modelService{workspace: workspace, key: hex.EncodeToString(b), models: map[string]*core.Model{}, clients: map[string]map[chan modelEvent]struct{}{},
		renderers: map[chan modelEvent]bool{}, pending: map[string]*renderCall{}}, nil
}

func (s *modelService) get(id string) (*core.Model, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("invalid project id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.models[id]; m != nil {
		m.SetCanvas(loadCanvas(s.workspace))
		return m, nil
	}
	m, err := core.LoadModel(s.workspace, id, defaultKinds())
	if err != nil {
		return nil, err
	}
	// the canvas is read each time a model is handed out, so a workspace
	// canvas.json edited meanwhile is picked up
	m.SetCanvas(loadCanvas(s.workspace))
	s.models[id] = m
	return m, nil
}

func (s *modelService) evict(id string) {
	s.mu.Lock()
	delete(s.models, id)
	s.mu.Unlock()
}

func (s *modelService) clean(id string) error {
	m, err := s.get(id)
	if err != nil {
		return err
	}
	d := m.Dirty()
	if len(d.Registry) > 0 || len(d.Views) > 0 {
		return fmt.Errorf("%w %s", errUnsaved, id)
	}
	return nil
}

// errUnsaved: a structural change waits for Save. The editor matches its text.
var errUnsaved = errors.New("сначала сохраните проект")

func (s *modelService) reload(change projectReloaded) error {
	s.evict(change.OldProject)
	s.evict(change.NewProject)
	m, err := s.get(change.NewProject)
	if err != nil {
		return err
	}
	ev := modelEvent{Author: "host", Changed: []core.Ref{}, Dirty: m.Dirty(), ProjectReloaded: &change}
	s.publish(change.OldProject, ev)
	if change.NewProject != change.OldProject {
		s.publish(change.NewProject, ev)
	}
	return nil
}

// The workspace as every client sees it — the editor's catalog
// (/api/workspace) and MCP list_projects/list_views alike: what exists from
// disk, names and looks from the working model, unsaved edits included.
func (s *modelService) index() core.WorkspaceIndex {
	return core.LiveIndex(s.workspace, s.get)
}

// createProject and createView are the one way a project or a view comes
// into being, for the editor (HTTP) and for agents (MCP): the file rule is
// core's, the lock and the reload that tells every open editor are here
// (ADR_20260926).
func (s *modelService) createProject(p core.NewProject) error {
	s.structureMu.Lock()
	defer s.structureMu.Unlock()
	if err := core.CreateProject(s.workspace, p); err != nil {
		return err
	}
	return s.reload(projectReloaded{OldProject: p.ID, NewProject: p.ID})
}

// createView refuses while the project has unsaved changes: SetDefault rewrites
// project.json on disk, and the reload drops the working model. names (language
// to caption) become texts of the working model, unsaved: a language left empty
// is the project's first one.
func (s *modelService) createView(project string, v core.NewView, names map[string]string, author string) error {
	s.structureMu.Lock()
	if err := s.clean(project); err != nil {
		s.structureMu.Unlock()
		return err
	}
	err := core.CreateView(s.workspace, project, v)
	if err == nil {
		err = s.reload(projectReloaded{OldProject: project, NewProject: project, NewView: v.ID})
	}
	s.structureMu.Unlock()
	if err != nil {
		return err
	}
	langs := make([]string, 0, len(names))
	for lang, name := range names {
		if strings.TrimSpace(name) != "" {
			langs = append(langs, lang)
		}
	}
	if len(langs) == 0 {
		return nil
	}
	m, err := s.get(project)
	if err != nil {
		return err
	}
	sort.Strings(langs)
	defer s.publishDirty(m, author)
	for _, lang := range langs {
		name := names[lang]
		if lang == "" {
			lang = m.Languages()[0]
		}
		if err := m.SetText(lang, v.ID, "name", name, author); err != nil {
			return fmt.Errorf("view %s created, its name not written: %w", v.ID, err)
		}
	}
	return nil
}

func (s *modelService) publishDirty(m *core.Model, author string) {
	dirty := m.Dirty()
	refs := append([]core.Ref{}, dirty.Registry...)
	for _, viewRefs := range dirty.Views {
		refs = append(refs, viewRefs...)
	}
	s.publish(m.ProjectID(), modelEvent{Author: author, Changed: refs, Dirty: dirty})
}

func (s *modelService) postProject(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	var p core.NewProject
	if err := readJSON(r, &p); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.createProject(p); err != nil {
		createError(w, err)
		return
	}
	writeJSON(w, map[string]string{"id": p.ID})
}

func (s *modelService) postView(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	var body struct {
		core.NewView
		Name     string `json:"name"`
		Language string `json:"language"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	project := r.PathValue("project")
	if err := s.createView(project, body.NewView, map[string]string{body.Language: body.Name}, "human"); err != nil {
		createError(w, err)
		return
	}
	writeJSON(w, map[string]string{"id": body.ID, "file": "projects/" + project + "/views/" + body.ID + core.ViewSuffix})
}

// createError: a taken id or unsaved changes are a conflict (409), as the
// editor's dialogs expect; everything else is modelError.
func createError(w http.ResponseWriter, err error) {
	if errors.Is(err, core.ErrExists) || errors.Is(err, errUnsaved) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	modelError(w, err)
}

func (s *modelService) publish(id string, ev modelEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients[id] {
		select {
		case ch <- ev:
		default:
			close(ch)
			delete(s.clients[id], ch)
		}
	}
}

func (s *modelService) authorize(w http.ResponseWriter, r *http.Request) bool {
	if !sameOrigin(r) {
		http.Error(w, "Cross-origin request refused", http.StatusForbidden)
		return false
	}
	if r.Header.Get("Authorization") != "Bearer "+s.key {
		http.Error(w, "Missing or invalid host key", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *modelService) hostFile(root string, port int) error {
	path := filepath.Join(root, ".semaps", "host.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(struct {
		PID  int    `json:"pid"`
		Port int    `json:"port"`
		Key  string `json:"key"`
	}{os.Getpid(), port, s.key})
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func (s *modelService) register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects", s.postProject)
	mux.HandleFunc("POST /api/model/{project}/views", s.postView)
	mux.HandleFunc("GET /api/model/{project}", s.snapshot)
	mux.HandleFunc("GET /api/model/{project}/views/{id}", s.view)
	mux.HandleFunc("POST /api/model/{project}/ops", s.ops)
	mux.HandleFunc("GET /api/model/{project}/save", s.summary)
	mux.HandleFunc("POST /api/model/{project}/save", s.save)
	mux.HandleFunc("POST /api/model/{project}/discard", s.discard)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/render/{id}", s.answerRender)
	mux.HandleFunc("GET /api/kinds", s.kinds)
}

// kinds answers GET /api/kinds: the dictionary — the tool's default with the
// workspace kinds.json added (CONTRACT §6). Without `lang` every text comes in
// all its languages; with it, names and descriptions in that one (as get_kinds).
func (s *modelService) kinds(w http.ResponseWriter, r *http.Request) {
	catalog, err := core.LoadKinds(s.workspace, defaultKinds())
	if err != nil {
		modelError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if lang := r.URL.Query().Get("lang"); lang != "" {
		writeJSON(w, dictionaryOut(catalog, lang))
		return
	}
	writeJSON(w, catalog)
}

func modelError(w http.ResponseWriter, err error) {
	var edit *core.EditError
	if errors.As(err, &edit) {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	var usage *core.UsageError
	if errors.As(err, &usage) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (s *modelService) snapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("project")
	m, err := s.get(id)
	if err != nil {
		modelError(w, err)
		return
	}
	manifest := m.Manifest()
	texts, err := m.TextSnapshot()
	if err != nil {
		modelError(w, err)
		return
	}
	views := map[string]json.RawMessage{}
	viewFiles := map[string]string{}
	for _, p := range core.Index(s.workspace).Projects {
		if p.ID == id {
			for _, v := range p.Views {
				b, err := m.View(v.ID)
				if err != nil {
					modelError(w, err)
					return
				}
				var props map[string]json.RawMessage
				if err := json.Unmarshal(b, &props); err != nil {
					modelError(w, err)
					return
				}
				delete(props, "placements")
				views[v.ID], _ = json.Marshal(props)
				viewFiles[v.ID] = v.File
			}
		}
	}
	writeJSON(w, map[string]any{"project": json.RawMessage(manifest), "registry": m.RegistrySnapshot(), "texts": texts, "views": views, "viewFiles": viewFiles, "dirty": m.Dirty(), "unsaved": m.Unsaved()})
}

func (s *modelService) view(w http.ResponseWriter, r *http.Request) {
	m, err := s.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	b, err := m.View(r.PathValue("id"))
	if err != nil {
		modelError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}

func (s *modelService) ops(w http.ResponseWriter, r *http.Request) {
	s.structureMu.RLock()
	defer s.structureMu.RUnlock()
	if !s.authorize(w, r) {
		return
	}
	var body struct {
		Client string    `json:"client"`
		Ops    []core.Op `json:"ops"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m, err := s.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	changed, err := m.Apply(body.Ops, "human")
	if err != nil {
		modelError(w, err)
		return
	}
	dirty := m.Dirty()
	s.publish(m.ProjectID(), modelEvent{Client: body.Client, Author: "human", Changed: changed, Dirty: dirty})
	writeJSON(w, map[string]any{"changed": changed, "dirty": dirty})
}

func (s *modelService) summary(w http.ResponseWriter, r *http.Request) {
	m, err := s.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	writeJSON(w, m.Dirty())
}

func (s *modelService) save(w http.ResponseWriter, r *http.Request) {
	s.structureMu.RLock()
	defer s.structureMu.RUnlock()
	if !s.authorize(w, r) {
		return
	}
	m, err := s.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	if err := m.Save(); err != nil {
		modelError(w, err)
		return
	}
	ev := modelEvent{Author: "human", Changed: []core.Ref{}, Dirty: m.Dirty()}
	s.publish(m.ProjectID(), ev)
	writeJSON(w, ev.Dirty)
}

func (s *modelService) discard(w http.ResponseWriter, r *http.Request) {
	s.structureMu.RLock()
	defer s.structureMu.RUnlock()
	if !s.authorize(w, r) {
		return
	}
	var body struct {
		Scope string `json:"scope"`
		ID    string `json:"id"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m, err := s.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	before := m.Dirty()
	if err := m.Discard(body.Scope, body.ID); err != nil {
		modelError(w, err)
		return
	}
	changed := append([]core.Ref{}, before.Registry...)
	for _, refs := range before.Views {
		changed = append(changed, refs...)
	}
	ev := modelEvent{Author: "human", Changed: changed, Dirty: m.Dirty()}
	s.publish(m.ProjectID(), ev)
	writeJSON(w, ev.Dirty)
}

func (s *modelService) events(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("project"))
	if id == "" {
		http.Error(w, "project required", 400)
		return
	}
	if _, err := s.get(id); err != nil {
		modelError(w, err)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := make(chan modelEvent, 16)
	s.mu.Lock()
	if s.clients[id] == nil {
		s.clients[id] = map[chan modelEvent]struct{}{}
	}
	s.clients[id][ch] = struct{}{}
	if r.URL.Query().Get("render") == "1" {
		s.renderers[ch] = true
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients[id], ch)
		delete(s.renderers, ch)
		s.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	f.Flush()
	for {
		select {
		case ev, open := <-ch:
			if !open {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
