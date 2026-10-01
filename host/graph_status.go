package main

import (
	"net/http"
	"time"
)

// graph_status (MCP) and GET /api/graph/{project}/status: per extractor entry
// of the .semaps file that feeds a project, the plain facts needed to judge
// whether the live graph can be trusted and what to do about it — the newest
// run and its state, the run the graph's facts come from, the edge kinds the
// entry asks for against those the facts have, and whether a watcher is on.
// It judges nothing itself: "stale" is for the reader to conclude.

// runStatus is one run, as graph_status shows it.
type runStatus struct {
	Run     string `json:"run"`
	Trigger string `json:"trigger,omitempty"` // "watch" for a run the host started on a file change; empty: a person or an agent
	State   string `json:"state"`             // running | done | failed
	Started string `json:"started"`
	// Finished and Seconds are absent while the run is going.
	Finished string  `json:"finished,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	// AgeSeconds: since the run finished, or since it started while it goes.
	AgeSeconds int    `json:"ageSeconds"`
	Error      string `json:"error,omitempty"`
}

func newRunStatus(r *runInfo) *runStatus {
	s := &runStatus{Run: r.ID, Trigger: r.Trigger, State: r.State, Started: r.Started.UTC().Format(time.RFC3339), Seconds: r.Seconds, Error: r.Error}
	from := r.Started
	if !r.Finished.IsZero() {
		s.Finished = r.Finished.UTC().Format(time.RFC3339)
		from = r.Finished
	}
	s.AgeSeconds = max(0, int(nowFunc().Sub(from).Seconds()))
	return s
}

// extractorStatus is one entry of the .semaps file.
type extractorStatus struct {
	Extractor string `json:"extractor"`
	Language  string `json:"language"`
	// Watch: the entry has `watch: true`; Watching: a watcher is really on
	// (false with --workspace or when it could not start); RunPending: a file
	// change was seen and its run has not started yet.
	Watch      bool `json:"watch"`
	Watching   bool `json:"watching"`
	RunPending bool `json:"runPending"`
	// LastRun is the newest run in any state; FactsRun the newest successful
	// one — the run the graph's facts come from. Empty FactsRun: the graph has
	// no facts from this entry.
	LastRun  *runStatus `json:"lastRun,omitempty"`
	FactsRun string     `json:"factsRun,omitempty"`
	// EdgesAsked is `edges:` of the entry now; EdgesInFacts the edge kinds
	// the facts of FactsRun cover; EdgesMissing the asked ones they lack.
	EdgesAsked   []string `json:"edgesAsked"`
	EdgesInFacts []string `json:"edgesInFacts"`
	EdgesMissing []string `json:"edgesMissing"`
}

// graphStatus: the status of every extractor entry of `proj` that feeds
// `projectID`. `watch` may be nil (--workspace, tests).
func graphStatus(proj project, projectID string, watch *watchManager) []extractorStatus {
	out := []extractorStatus{}
	if proj.File == "" {
		return out
	}
	runs := newRunStore(proj.File)
	for _, e := range proj.Extractors {
		if e.Project != projectID {
			continue
		}
		ws := watch.state(e.ID)
		st := extractorStatus{
			Extractor: e.ID, Language: e.Language, Watch: e.Watch, Watching: ws.Watching, RunPending: ws.RunPending,
			EdgesAsked: nonNil(e.Edges), EdgesInFacts: []string{}, EdgesMissing: []string{},
		}
		all := runs.list(e.ID)
		if len(all) > 0 {
			st.LastRun = newRunStatus(all[0])
		}
		if r := latestDone(all); r != nil {
			st.FactsRun = r.ID
			var declared []string
			if f, err := readFacts(runs.path(r.ID, "facts.json")); err == nil {
				declared = f.EdgeKinds
			}
			st.EdgesInFacts = nonNil(edgeKindsOf(r, declared))
		}
		st.EdgesMissing = nonNil(missingEdgeKinds(e.Edges, st.EdgesInFacts))
		out = append(out, st)
	}
	return out
}

// latestDone: the newest successful run of `runs` (newest first), or nil.
func latestDone(runs []*runInfo) *runInfo {
	for _, r := range runs {
		if r.State == "done" {
			return r
		}
	}
	return nil
}

// serveStatus: GET /api/graph/{project}/status.
func (g *graphService) serveStatus(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if _, err := g.models.get(project); err != nil {
		modelError(w, err)
		return
	}
	writeJSON(w, map[string]any{"extractors": graphStatus(g.proj, project, g.watch)})
}
