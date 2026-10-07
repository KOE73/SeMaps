package main

import (
	"net/http"

	"semaps/core"
)

func (s *modelService) registerStyles(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/model/{project}/views/{id}/styled", func(w http.ResponseWriter, r *http.Request) {
		value, err := s.shared.View(r.PathValue("project"), r.PathValue("id"))
		if err != nil {
			modelError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, value)
	})
	mux.HandleFunc("GET /styles.json", func(w http.ResponseWriter, r *http.Request) {
		snapshot := s.shared.Styles()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(snapshot.Sheet)
	})
	mux.HandleFunc("GET /api/styles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, s.shared.Styles())
	})
	mux.HandleFunc("POST /api/styles", s.editStyleHTTP)
	mux.HandleFunc("POST /api/styles/save", s.saveStylesHTTP)
	mux.HandleFunc("POST /api/styles/discard", s.discardStylesHTTP)
}

func (s *modelService) publishStyles(author string, clients ...string) {
	s.publishStyleChange(author, nil, nil, clients...)
}

func (s *modelService) publishStyleEdit(author string, edit core.StyleEdit, clients ...string) {
	var renamed *styleRename
	if edit.Action == "rename" {
		renamed = &styleRename{OldID: edit.ID, NewID: edit.NewID}
	}
	s.publishStyleChange(author, renamed, nil, clients...)
}

// Capture before Discard clears dirt: its reverted view references must still
// ask both editors to reload, even when the resulting project is clean.
func (s *modelService) styleViewChanges() map[string][]core.Ref {
	s.mu.Lock()
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	changes := map[string][]core.Ref{}
	for _, id := range ids {
		if m, err := s.get(id); err == nil {
			for _, refs := range m.Dirty().Views {
				changes[id] = append(changes[id], refs...)
			}
		}
	}
	return changes
}

func (s *modelService) publishStyleChange(author string, renamed *styleRename, changes map[string][]core.Ref, clients ...string) {
	snapshot := s.shared.Styles()
	s.mu.Lock()
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		m, err := s.get(id)
		if err != nil {
			continue
		}
		client := ""
		if len(clients) > 0 {
			client = clients[0]
		}
		dirty := m.Dirty()
		refs := append([]core.Ref{}, changes[id]...)
		for _, viewRefs := range dirty.Views {
			refs = append(refs, viewRefs...)
		}
		s.publish(id, modelEvent{Client: client, Author: author, Changed: refs, Dirty: dirty, Styles: &snapshot, StyleRenamed: renamed})
	}
}

func (s *modelService) editStyleHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	s.structureMu.Lock()
	defer s.structureMu.Unlock()
	var body struct {
		core.StyleEdit
		Client string `json:"client"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	detail, err := s.shared.EditStyle(body.StyleEdit, "human")
	if err != nil {
		modelError(w, err)
		return
	}
	s.publishStyleEdit("human", body.StyleEdit, body.Client)
	writeJSON(w, struct {
		core.StyleDetail
		State core.StyleSnapshot `json:"state"`
	}{detail, s.shared.Styles()})
}
func (s *modelService) saveStylesHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	s.structureMu.Lock()
	defer s.structureMu.Unlock()
	if err := s.shared.SaveStyles(); err != nil {
		modelError(w, err)
		return
	}
	s.publishStyles("human")
	writeJSON(w, s.shared.Styles())
}
func (s *modelService) discardStylesHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	s.structureMu.Lock()
	defer s.structureMu.Unlock()
	changes := s.styleViewChanges()
	if err := s.shared.DiscardStyles(); err != nil {
		modelError(w, err)
		return
	}
	s.publishStyleChange("human", nil, changes)
	writeJSON(w, s.shared.Styles())
}
