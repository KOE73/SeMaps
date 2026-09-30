package main

// render_view: a request/response bridge from the host to an open editor
// (docs/API.md §6, "render_view"). The host draws nothing itself — only the
// editor has the real router and text measurement — so it asks:
//
//  1. an editor subscribes to `GET /api/events?project=<id>&render=1`;
//  2. the tool sends those clients (and only those) an SSE event whose
//     `render` field is a renderRequest;
//  3. the editor answers `POST /api/render/{id}` with a renderAnswer.
//
// The host waits for the first successful answer.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"semaps/core"
)

// renderTimeout is how long render_view waits for an editor; a variable so a
// test can shorten it.
var renderTimeout = 20 * time.Second

// renderRequest is the `render` field of a modelEvent.
type renderRequest struct {
	ID      string     `json:"id"`
	View    string     `json:"view"`
	Ref     string     `json:"ref,omitempty"`
	Rect    *core.Rect `json:"rect,omitempty"`
	Scale   float64    `json:"scale"`
	MaxSize int        `json:"maxSize"`
}

// renderProblem is one finding of the editor's own checks: kind is one of
// overlap, clipped-caption, clipped-rows, line-through-box, line-crossing,
// outside-container.
type renderProblem struct {
	Kind string   `json:"kind"`
	IDs  []string `json:"ids"`
	Text string   `json:"text"`
}

// renderAnswer is the body of POST /api/render/{id}; on failure only Error.
type renderAnswer struct {
	PNG      string          `json:"png,omitempty"` // base64, no prefix
	Width    int             `json:"width,omitempty"`
	Height   int             `json:"height,omitempty"`
	Rect     *core.Rect      `json:"rect,omitempty"` // the model rectangle drawn, margin included
	Problems []renderProblem `json:"problems,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type renderCall struct {
	answers chan renderAnswer
}

var errNoRenderClient = errors.New("no render client")

func newRenderID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// publishRender sends ev to the render clients of a project only and says how
// many there were.
func (s *modelService) publishRender(project string, ev modelEvent) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for ch := range s.clients[project] {
		if !s.renderers[ch] {
			continue
		}
		select {
		case ch <- ev:
			n++
		default:
			close(ch)
			delete(s.clients[project], ch)
		}
	}
	return n
}

// render asks the open editors of a project for a picture and waits for the
// first successful answer. An error answer from one editor is kept while the
// others are still awaited; when every render client has answered with an
// error, the first one is returned.
func (s *modelService) render(project string, req renderRequest) (renderAnswer, error) {
	req.ID = newRenderID()
	call := &renderCall{answers: make(chan renderAnswer, 64)}
	s.renderMu.Lock()
	s.pending[req.ID] = call
	s.renderMu.Unlock()
	defer func() {
		s.renderMu.Lock()
		delete(s.pending, req.ID)
		s.renderMu.Unlock()
	}()

	n := s.publishRender(project, modelEvent{Author: "host", Changed: []core.Ref{}, Render: &req})
	if n == 0 {
		return renderAnswer{}, errNoRenderClient
	}
	timer := time.NewTimer(renderTimeout)
	defer timer.Stop()
	var first *renderAnswer
	failed := 0
	for {
		select {
		case a := <-call.answers:
			if a.Error == "" {
				return a, nil
			}
			if first == nil {
				first = &a
			}
			if failed++; failed >= n {
				return renderAnswer{}, errors.New(first.Error)
			}
		case <-timer.C:
			return renderAnswer{}, fmt.Errorf("the editor did not answer within %d seconds: is an editor open on the project?", int(renderTimeout/time.Second))
		}
	}
}

// answerRender is POST /api/render/{id}: authorised like /api/model/*.
func (s *modelService) answerRender(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	var a renderAnswer
	if err := readJSON(r, &a); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.renderMu.Lock()
	call := s.pending[r.PathValue("id")]
	s.renderMu.Unlock()
	if call == nil {
		http.Error(w, "no such render request (answered already, or timed out)", http.StatusNotFound)
		return
	}
	select {
	case call.answers <- a:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}
