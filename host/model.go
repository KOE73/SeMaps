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
	"strings"
	"sync"

	"semaps/core"
)

type modelEvent struct {
	Client  string            `json:"client"`
	Author  string            `json:"author"`
	Changed []core.Ref        `json:"changed"`
	Dirty   core.DirtySummary `json:"dirty"`
}

type modelService struct {
	workspace string
	key       string
	mu        sync.Mutex
	models    map[string]*core.Model
	clients   map[string]map[chan modelEvent]struct{}
}

func newModelService(workspace string) (*modelService, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &modelService{workspace: workspace, key: hex.EncodeToString(b), models: map[string]*core.Model{}, clients: map[string]map[chan modelEvent]struct{}{}}, nil
}

func (s *modelService) get(id string) (*core.Model, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("invalid project id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.models[id]; m != nil {
		return m, nil
	}
	m, err := core.LoadModel(s.workspace, id)
	if err != nil {
		return nil, err
	}
	s.models[id] = m
	return m, nil
}

func (s *modelService) evict(id string) {
	s.mu.Lock()
	delete(s.models, id)
	s.mu.Unlock()
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
	mux.HandleFunc("GET /api/model/{project}", s.snapshot)
	mux.HandleFunc("GET /api/model/{project}/views/{id}", s.view)
	mux.HandleFunc("POST /api/model/{project}/ops", s.ops)
	mux.HandleFunc("GET /api/model/{project}/save", s.summary)
	mux.HandleFunc("POST /api/model/{project}/save", s.save)
	mux.HandleFunc("POST /api/model/{project}/discard", s.discard)
	mux.HandleFunc("GET /api/events", s.events)
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
	manifest, err := os.ReadFile(filepath.Join(m.ProjectDir(), "project.json"))
	if err != nil {
		modelError(w, err)
		return
	}
	texts, err := m.TextSnapshot()
	if err != nil {
		modelError(w, err)
		return
	}
	views := map[string]json.RawMessage{}
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
				delete(props, "zones")
				delete(props, "nodes")
				delete(props, "placements")
				views[v.ID], _ = json.Marshal(props)
			}
		}
	}
	writeJSON(w, map[string]any{"project": json.RawMessage(manifest), "registry": m.RegistrySnapshot(), "texts": texts, "views": views, "dirty": m.Dirty()})
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
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.clients[id], ch); s.mu.Unlock() }()
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
