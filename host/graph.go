// The live code graph: host/graph.go joins a project's model with its
// extractors' latest facts (docs/adr/ADR_20260928_host_live-code-graph.md,
// docs/plans/PLAN_20260928_host_graph-provider.md steps 3-5). It is
// read-only and derived: nothing here writes to the model or caches the
// graph across requests — only enough of the previous build is kept, per
// project, to publish a diff instead of the whole graph when a run finishes
// (ADR §9: indexes and caches wait for a measured need).
package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"semaps/core"
)

type graphService struct {
	proj   project
	models *modelService

	mu   sync.Mutex
	prev map[string]*core.Graph // project id -> the last graph built for it
}

func newGraphService(proj project, models *modelService) *graphService {
	return &graphService{proj: proj, models: models, prev: map[string]*core.Graph{}}
}

// graphFactsInfo is one extractor's contribution, as the API and the MCP
// tool report it (docs/plans/PLAN_20260928_host_graph-provider.md step 4).
type graphFactsInfo struct {
	Extractor string `json:"extractor"`
	Run       string `json:"run"`
	Finished  string `json:"finished"`
	Language  string `json:"language"`
}

// sources finds, for every extractor of the .semaps file that feeds
// `project`, the facts of its latest successful run. No .semaps file
// (--workspace) or no successful run at all: empty — BuildGraph then joins
// the model alone, exactly as step 3 asks.
func (g *graphService) sources(project string) ([]core.FactsSource, []graphFactsInfo) {
	return sourcesFor(g.proj, project)
}

// sourcesFor is the free-standing form: every extractor of `proj`'s
// `.semaps` file that feeds `project`, with the facts of its latest
// successful run. Used directly by the MCP tool (get_graph, step 6), which
// has a project and a model but not always a graphService.
func sourcesFor(proj project, projectID string) ([]core.FactsSource, []graphFactsInfo) {
	sources := []core.FactsSource{}
	facts := []graphFactsInfo{}
	if proj.File == "" {
		return sources, facts
	}
	runs := newRunStore(proj.File)
	for _, e := range proj.Extractors {
		if e.Project != projectID {
			continue
		}
		for _, r := range runs.list(e.ID) {
			if r.State != "done" {
				continue
			}
			f, err := readFacts(runs.path(r.ID, "facts.json"))
			if err != nil {
				break // newest done run is unreadable: no facts for this extractor, not older ones
			}
			sources = append(sources, core.FactsSource{Extractor: e.ID, Facts: f})
			facts = append(facts, graphFactsInfo{
				Extractor: e.ID, Run: r.ID, Language: e.Language,
				Finished: r.Finished.UTC().Format(time.RFC3339),
			})
			break
		}
	}
	return sources, facts
}

// build joins the live working model with the latest facts on disk. Nothing
// here is persisted: a fresh core.Graph every call.
func (g *graphService) build(project string) (*core.Graph, []graphFactsInfo, error) {
	m, err := g.models.get(project)
	if err != nil {
		return nil, nil, err
	}
	sources, facts := g.sources(project)
	graph, err := core.BuildGraph(sources, m)
	if err != nil {
		return nil, nil, err
	}
	return graph, facts, nil
}

// notifyRunFinished is runStore's onFinish callback: a successful run may
// change its project's graph, so /api/events subscribers are told what
// changed (step 5). It keeps the built graph only to diff against the next
// one — never served from here, never treated as a cache of an API answer.
func (g *graphService) notifyRunFinished(info *runInfo) {
	if info == nil || info.State != "done" {
		return
	}
	graph, _, err := g.build(info.Project)
	if err != nil {
		return
	}
	g.mu.Lock()
	old := g.prev[info.Project]
	g.prev[info.Project] = graph
	g.mu.Unlock()
	if old == nil {
		return // first graph built for this project in this process: nothing to diff against yet
	}
	diff := core.DiffGraphs(old, graph)
	if diff.Empty() {
		return
	}
	g.models.publish(info.Project, modelEvent{Author: "extract", Changed: []core.Ref{}, Graph: &diff})
}

// remember keeps the graph a reader was first given, so that the first run
// after it has something to be compared with. It is built apart from the
// answer: the answer is filtered and may carry members.
func (g *graphService) remember(project string) {
	g.mu.Lock()
	known := g.prev[project] != nil
	g.mu.Unlock()
	if known {
		return
	}
	graph, _, err := g.build(project)
	if err != nil {
		return
	}
	g.mu.Lock()
	if g.prev[project] == nil {
		g.prev[project] = graph
	}
	g.mu.Unlock()
}

func (g *graphService) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph/{project}", g.guard(g.serve))
}

// guard: the graph needs the .semaps file to know a project's extractors,
// so --workspace answers 409 like the other tool-only endpoints
// (host/api_tools.go, docs/API.md §5). Unlike those, this is a read with no
// side effect, so it needs no host key — matching /api/model/{project},
// whose GET does not call authorize either (only ops/save/discard do).
func (g *graphService) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.proj.File == "" {
			http.Error(w, "Started with --workspace: no .semaps file, no extractors", http.StatusConflict)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		h(w, r)
	}
}

func (g *graphService) serve(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	graph, facts, err := g.build(project)
	if err != nil {
		modelError(w, err)
		return
	}
	m, err := g.models.get(project)
	if err != nil {
		modelError(w, err)
		return
	}
	g.remember(project)

	fields := parseFieldSet(r.URL.Query().Get("fields"))
	if fields["members"] {
		if err := core.AttachMembers(graph, m); err != nil {
			modelError(w, err)
			return
		}
	}

	graph, err = core.FilterLevel(graph, r.URL.Query().Get("level"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if kinds := splitCSV(r.URL.Query().Get("kinds")); len(kinds) > 0 {
		graph = core.FilterEdgeKinds(graph, kinds)
	}

	if around := r.URL.Query().Get("around"); around != "" {
		depth, err := parseDepth(r.URL.Query().Get("depth"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		graph, err = core.Neighborhood(graph, around, depth)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	if container := r.URL.Query().Get("container"); container != "" {
		known, err := knownContainers(m)
		if err != nil {
			modelError(w, err)
			return
		}
		graph, err = core.FilterContainer(graph, container, known)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	graph = core.StripFields(graph, fields)

	writeJSON(w, map[string]any{
		"nodes": graph.Nodes,
		"edges": graph.Edges,
		"facts": facts,
		"stats": graphStats(graph),
	})
}

func knownContainers(m *core.Model) (map[string]bool, error) {
	containers, err := core.LoadContainers(m.ProjectDir())
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	if containers != nil {
		for _, c := range containers.List {
			known[c.ID] = true
		}
	}
	return known, nil
}

// parseDepth: default 1, must be in 1..5 (docs/plans/PLAN_20260928_host_graph-provider.md step 4).
func parseDepth(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	d, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("depth: not a number")
	}
	if d <= 0 || d > 5 {
		return 0, errors.New("depth: must be between 1 and 5")
	}
	return d, nil
}

func graphStats(g *core.Graph) map[string]any {
	byPresence := map[string]int{}
	for _, n := range g.Nodes {
		byPresence[n.Presence]++
	}
	return map[string]any{"nodes": len(g.Nodes), "edges": len(g.Edges), "byPresence": byPresence}
}

// parseFieldSet: default "via,position" (members off by default — it can be
// large and most consumers do not need it).
func parseFieldSet(s string) map[string]bool {
	if s == "" {
		s = "via,position"
	}
	out := map[string]bool{}
	for _, f := range splitCSV(s) {
		out[f] = true
	}
	return out
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
