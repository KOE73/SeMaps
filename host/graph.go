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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"semaps/core"
)

type graphService struct {
	proj     project
	models   *modelService
	settings *mcpSettingsBox // list_cap/limit defaults, live (step 2)

	mu   sync.Mutex
	prev map[string]*core.Graph // project id -> the last graph built for it
}

func newGraphService(proj project, models *modelService, settings *mcpSettingsBox) *graphService {
	return &graphService{proj: proj, models: models, settings: settings, prev: map[string]*core.Graph{}}
}

// graphFactsInfo is one extractor's contribution, as the API and the MCP
// tool report it (docs/plans/PLAN_20260928_host_graph-provider.md step 4).
type graphFactsInfo struct {
	Extractor string `json:"extractor"`
	Run       string `json:"run,omitempty"`
	Finished  string `json:"finished,omitempty"`
	Language  string `json:"language"`
	// LastRun and LastRunFailed are about the newest run of this extractor,
	// in any state — not necessarily the one Run/Finished/the graph's facts
	// come from. A failed run never touches the graph (runStore.onFinish
	// fires only for "done"); this is how a reader of GET /api/graph learns
	// that anyway (PLAN_20260928-4_host_watch-sources.md step 2).
	LastRun       string `json:"lastRun,omitempty"`
	LastRunFailed bool   `json:"lastRunFailed,omitempty"`
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
		all := runs.list(e.ID)
		if len(all) == 0 {
			continue // never run: nothing to say about it yet
		}
		info := graphFactsInfo{Extractor: e.ID, Language: e.Language, LastRun: all[0].ID, LastRunFailed: all[0].State == "failed"}
		for _, r := range all {
			if r.State != "done" {
				continue
			}
			f, err := readFacts(runs.path(r.ID, "facts.json"))
			if err != nil {
				break // newest done run is unreadable: no facts for this extractor, not older ones
			}
			sources = append(sources, core.FactsSource{Extractor: e.ID, Facts: f})
			info.Run, info.Finished = r.ID, r.Finished.UTC().Format(time.RFC3339)
			break
		}
		facts = append(facts, info)
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
	mux.HandleFunc("GET /api/graph/{project}/find", g.guard(g.serveFind))
	mux.HandleFunc("GET /api/graph/{project}/groups", g.guard(g.serveGroups))
	mux.HandleFunc("GET /api/graph-formats", g.serveFormats)
}

// serveFormats: GET /api/graph-formats, the list a caller needs to build a
// `format=`/`set=` request without guessing (docs/API.md §5).
func (g *graphService) serveFormats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, graphFormatsPayload())
}

// graphTerms: the words of the registry's relation types and statuses that
// `relations` (the directed names `follow` takes) does not explain, one line
// each — what an agent meets in `get_relations`, `get_relation_types` and a
// node's `presence`. The server's Instructions point here instead of
// repeating them (docs/EXTRACTOR.md §5 and CONTRACT.md §4/§5 are the
// sources).
var graphTerms = []map[string]string{
	{"name": "holds.one", "description": "a public field/property/parameter holds exactly one value of the target type"},
	{"name": "holds.optional", "description": "holds zero or one value of the target type (nullable)"},
	{"name": "holds.many", "description": "holds a collection of the target type (list, array, set)"},
	{"name": "holds.many.ro", "description": "a collection of the target type that cannot be changed through that member (read-only)"},
	{"name": "holds.keyed", "description": "holds a dictionary whose key or value is the target type; `.ro`: read-only"},
	{"name": "holds.*.internal", "description": "the same, through a member that is not public"},
	{"name": "injects", "description": "a constructor parameter takes the target type (dependency injection)"},
	{"name": "uses", "description": "a signature (parameter, return value) or type alias mentions the target type, other than by holding or injecting it"},
	{"name": "references", "description": "an old generic relation type; sync does not produce it and marks such a relation `missing` — the code-derived types are holds.*, uses, injects"},
	{"name": "present", "description": "status: the entity or relation was found in the code at the last sync"},
	{"name": "missing", "description": "status: still in the registry (nothing is deleted, the id stays valid) but no longer found in the code; get_graph leaves it out unless `missing` is asked for"},
	{"name": "planned", "description": "status of an entity: declared before its code exists"},
	{"name": "code / model / both", "description": "a graph node's presence: only in the code facts / only in the registry / in both"},
}

// graphFormatsPayload is shared by the HTTP list and the MCP graph_formats
// tool, so the two never drift apart: formats (with a text format's own
// template), the relation vocabulary `follow` accepts, the macro dictionary
// and template rules (part 3, so an agent can write its own template after
// reading only this), and the defaults.
func graphFormatsPayload() map[string]any {
	formats := []map[string]any{}
	for _, f := range core.GraphFormats() {
		m := map[string]any{"name": f.Name(), "description": f.Description(), "mediaType": f.MediaType()}
		if t, ok := graphFormatTemplates[f.Name()]; ok {
			m["template"] = t
		}
		formats = append(formats, m)
	}
	relations := []map[string]any{}
	for _, r := range core.RelationVocabulary() {
		relations = append(relations, map[string]any{
			"name": r.Name, "inverse": r.Inverse, "kind": r.Kind, "typeMatch": r.TypeMatch, "description": r.Description,
		})
	}
	return map[string]any{
		"formats":       formats,
		"relations":     relations,
		"terms":         graphTerms,
		"defaultFollow": core.DefaultFollow,
		"template":      templateRulesPayload(),
		"defaults": map[string]any{
			"format": core.DefaultGraphFormat,
			"level":  map[string]any{"wholeGraph": "types", "neighbourhood": "all"},
		},
	}
}

// graphFormatTemplates: the stored template string of each text format, so
// graph_formats shows exactly what a caller could pass as `template` to get
// the same shape (part 3).
var graphFormatTemplates = map[string]string{
	"facts":     core.FactsTemplate,
	"lines":     core.LinesTemplate,
	"locations": core.LocationsTemplate,
	"tree":      core.TreeNodeTemplate,
}

// templateRulesPayload: the explanation and worked examples part 3 (then
// defects 2-6 of the agent-answers-graph task) ask for, so an agent can
// write a template after reading only this.
func templateRulesPayload() map[string]any {
	return map[string]any{
		"rules": []string{
			"Literal text is copied as is, subject to the whitespace rule below.",
			"{macro} is replaced by that macro's value, or nothing when it has none.",
			"[...] is an optional group: printed as is when every macro directly inside it (or a nested {relations: ...} block) has a value; dropped whole, literal text included, when any one of them is empty. A nested [...] group is opaque to this check.",
			"\\[ and \\] are literal '[' and ']': since [ and ] are the optional-group syntax, a literal bracket (the default `facts` template wraps its relations in one) must be escaped this way. \\{ and \\} are literal '{' and '}' the same way, since {...} is the macro syntax (the default `facts`/`lines`/`tree` templates wrap {dynamic} in literal braces of their own: `{dynamic: create @57 in CreateRunner}`).",
			"Whitespace rule: after rendering a template's own top-level pieces, a run made only of the space character ' ' is dropped when it falls in the LEADING run (everything from the start up to and including it renders empty) or the TRAILING run (everything from it to the end renders empty); a space run with real content on both sides — even next to an immediately empty macro or dropped group — is kept exactly as written. A tab or a newline is never touched by this rule (so `locations`' tab-separated columns stay put even when a value is empty). This applies once, to the template as a whole (and, the same way, to a `lines`/`tree` relation line) — not separately inside every nested [...] group, so a group that intentionally carries its own leading or trailing separator (e.g. a lifted relation's `×{count} `) keeps it.",
			"{relations: TEMPLATE | SEPARATOR} renders TEMPLATE once per relation reaching the node from the node it was reached from, joined by SEPARATOR.",
			"Node macros: step name fullName id kind nativeKind visibility file line endLine lines namespace assembly containers presence status entity via viaFullName dynamic.",
			"{dynamic}: blind-spot marks (ADR_20260928-5 §4) as 'kind @line[; kind @line...]', capped like {fromMethods} then '+N'. On a `method` node, its own marks; on a `type`/`interface`/`module` node after `lift=types`, the marks gathered from its methods, each followed by 'in MethodName' (and, for a partial type whose method lies in another file, that file's name in parentheses). Empty, so the optional group around it disappears, when there are none.",
			"Relation macros (inside a relations block only): relation member memberKind memberLine modifiers cardinality type text relationLine relationLines relationLinesFile count fromMethods toMethods injected.",
			"`lines` is `line-endLine`, or just `line` when there is no end.",
			"`relation` is the directed name, from the point of view of the node it was reached FROM (e.g. `holds`, not `held-by`, when read from the holder) — the SAME rule in `facts`, `lines` and `tree` alike, and the same word a `follow=` request would use to continue the walk in that direction: what an agent reads in any text answer is what it can hand back to keep walking.",
			"{memberLine} is where the member is declared (holds/uses/injects: the edge's own line); {relationLine}/{relationLines} is the call/construct site(s) (calls/constructs only) — two different numbers, never printed for the other edge kind.",
			"{relationLinesFile}: only set when the call/construct site(s) are NOT in the file already printed on this line — the file of the node the relation starts FROM (e.g.From), which for a `calls`/`constructs` line is the parent (a file not otherwise shown on this line) and for a `called-by`/`constructed-by` line is the node the line is about (already shown, so empty here).",
			"{via}/{viaFullName} (short name / full name of the node(s) this node was reached from, comma-separated) are non-empty only at step >= 2 — at step 0 there is nothing to name, at step 1 it is the focus, which the line already gives.",
			"{count} is the number of method-level edges LiftToTypes (lift=types) merged into a relation; empty (no '×N') when <= 1. {fromMethods}/{toMethods} (comma-separated short method names, capped at 5 then '+N') are set whenever the relation was lifted, even at count 1 — only the '×N' is skipped there.",
			"{injected} is the literal '(injected)' on a `holds`/`held-by` relation that the same member of the same pair's `injects`/`injected-into` was folded into (member names compared case-insensitively, ignoring one leading underscore); empty on every other relation, including a lone `injects`/`injected-into` with no matching holds.",
			"A `method`-kind node's {fullName} is its short printed signature (container.Name`arity(ShortParamTypes)), never its full id (which can run to 250 characters) — {id} still gives the full id. The printed short signature always resolves back via `around` to that same method (or lists every overload sharing it).",
			"A malformed template (unmatched `{`, `[` or `]`, or an unknown macro) is an error naming the position and the macro.",
			"A node macro may also be used inside a relations block, to show the neighbour's own data next to the relation.",
		},
		"examples": []map[string]string{
			{"template": core.FactsTemplate, "example": "1 Billing.InvoiceService  src/Billing/InvoiceService.cs:10-40  [held-by Context:7 property protected readonly (injected)]"},
			{"template": "{relations: {relation} via {member} | ; }", "example": "holds via Log; extends via "},
			{"template": core.FactsTemplate, "example": "1 Shop.Order  src/Order.cs:14-164  [calls ×3 from Create to Id,Total,Lines @38,41,44 in OrderFactory.cs]"},
		},
	}
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
	settings := mcpSettings{}.withDefaults()
	if g.settings != nil {
		settings = g.settings.Get()
	}
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

	fields, err := fieldSetParam(r.URL.Query().Has("fields"), splitCSV(r.URL.Query().Get("fields")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if fields["members"] {
		if err := core.AttachMembers(graph, m); err != nil {
			modelError(w, err)
			return
		}
	}

	template := r.URL.Query().Get("template")
	formatName := r.URL.Query().Get("format")
	if formatName == "" && template == "" {
		formatName = core.DefaultGraphFormat
	}
	var formatter core.GraphFormatter
	if template == "" {
		var ok bool
		formatter, ok = core.GetGraphFormat(formatName)
		if !ok {
			http.Error(w, core.UnknownFormatError(formatName).Error(), http.StatusBadRequest)
			return
		}
	}
	aroundQuery := r.URL.Query().Get("around")
	if formatName == "tree" && aroundQuery == "" {
		http.Error(w, "format=tree needs around=... (a focus node): see docs/API.md §5", http.StatusBadRequest)
		return
	}
	if aroundQuery != "" && r.URL.Query().Get("kinds") != "" {
		http.Error(w, "kinds is only for a whole-graph request (no around); a neighbourhood names follow=...", http.StatusBadRequest)
		return
	}

	// Name resolution (part 2) happens against the graph as built, before
	// `missing` hides anything, so a hit on a hidden node can be reported as
	// such rather than as "no such node".
	missing := parseBoolParam(r.URL.Query().Get("missing"))
	var notice string
	var around, aroundKind string
	if aroundQuery != "" {
		res := core.ResolveNode(graph, aroundQuery)
		switch {
		case len(res.Candidates) > 0:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]any{"error": "ambiguous", "asked": aroundQuery, "candidates": res.Candidates})
			return
		case res.Node == nil:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{"error": "no such node", "asked": aroundQuery, "suggestions": res.Suggestions})
			return
		case res.MissingHidden && !missing:
			http.Error(w, fmt.Sprintf("node %q is hidden as missing; pass missing=1 to include it", res.Node.ID), http.StatusNotFound)
			return
		}
		around, aroundKind, notice = res.Node.ID, res.Node.Kind, res.Notice
	}

	graph, hiddenNodes, hiddenEdges := core.FilterMissing(graph, missing)

	levelParam := r.URL.Query().Get("level")
	if levelParam == "" {
		if around == "" {
			levelParam = "types"
		} else {
			levelParam = "all"
		}
	}

	liftParam := r.URL.Query().Get("lift")
	if liftParam == "" {
		liftParam = defaultLift(around != "", aroundKind, levelParam)
	}
	if liftParam != "types" && liftParam != "none" {
		http.Error(w, fmt.Sprintf("lift: %q is neither %q nor %q", liftParam, "types", "none"), http.StatusBadRequest)
		return
	}
	if liftParam == "types" {
		// Before the walk (and before level=types would otherwise just drop
		// method nodes outright): ADR_20260928-3 §6, so `follow=calls` from a
		// type walks type to type.
		graph = core.LiftToTypes(graph)
	}

	graph, err = core.FilterLevel(graph, levelParam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var fanoutNotes []core.FanoutNote
	bounded := false
	var liftHint string
	if around != "" {
		follow, err := core.ParseFollow(splitCSV(r.URL.Query().Get("follow")))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		depth, err := parseDepth(r.URL.Query().Get("depth"))
		if err == nil {
			err = checkWalkAll(depth, follow, len(splitCSV(r.URL.Query().Get("follow"))) > 0)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fanout, err := parseFanout(r.URL.Query().Get("fanout"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		preWalk := graph
		graph, fanoutNotes, err = core.Walk(graph, around, depth, follow, fanout)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		bounded = true
		if liftParam == "none" {
			liftHint = core.LiftNoneHint(preWalk, graph, around, follow)
		}
	} else if kinds := splitCSV(r.URL.Query().Get("kinds")); len(kinds) > 0 {
		graph = core.FilterEdgeKinds(graph, kinds)
	}

	if container := r.URL.Query().Get("container"); container != "" {
		defs, err := containerDefs(m)
		if err != nil {
			modelError(w, err)
			return
		}
		graph, err = core.FilterContainer(graph, container, defs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		bounded = true
	}

	// The endpoint cuts only when asked to: the graph page draws the whole
	// graph. `mcp.limit` is the default of the tool, not of this endpoint.
	var truncated bool
	var fullNodes, fullEdges int
	if r.URL.Query().Has("limit") {
		limit := intParam(r.URL.Query().Get("limit"), settings.Limit)
		graph, truncated, fullNodes, fullEdges = truncateGraph(graph, bounded, limit)
	}

	opts := core.FormatOptions{
		Focus:       around,
		Facts:       facts,
		Stats:       graphStats(graph, hiddenNodes, hiddenEdges),
		Fields:      fields,
		FanoutNotes: fanoutNotes,
		Notice:      notice,
		Truncated:   truncated,
		FullNodes:   fullNodes,
		FullEdges:   fullEdges,
		ListCap:     intParam(r.URL.Query().Get("list_cap"), settings.ListCap),
		LiftHint:    liftHint,
	}
	var body []byte
	mediaType := "text/plain; charset=utf-8"
	if template != "" {
		body, err = core.FormatWithTemplate(graph, template, opts)
	} else {
		body, err = formatter.Format(graph, opts)
		mediaType = formatter.MediaType()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	_, _ = w.Write(body)
}

// serveFind: GET /api/graph/{project}/find?q=&limit= — part 2's plain
// substring search over names and ids, so a caller unsure of the exact
// `around` spelling gets candidates instead of guessing.
// serveGroups answers the graph mode's «группировать по»: containers' nesting and,
// per axis, the zone of the project's views each entity sits in. Read-only.
func (g *graphService) serveGroups(w http.ResponseWriter, r *http.Request) {
	m, err := g.models.get(r.PathValue("project"))
	if err != nil {
		modelError(w, err)
		return
	}
	data, err := m.Groups()
	if err != nil {
		modelError(w, err)
		return
	}
	writeJSON(w, data)
}

func (g *graphService) serveFind(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	graph, _, err := g.build(project)
	if err != nil {
		modelError(w, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, map[string]any{"candidates": core.FindNodes(graph, r.URL.Query().Get("q"), limit)})
}

// defaultLift is `lift`'s default when the parameter is absent
// (ADR_20260928-3 §6): with a focus node (`hasAround`), `types` unless that
// node is itself a method (methods are already the finest level, so there is
// nothing to lift); without one (a whole-graph request), `types` exactly when
// `level` is also `types` (the whole-graph default), `none` when it is `all`.
func defaultLift(hasAround bool, aroundKind, level string) string {
	if hasAround {
		if aroundKind == "method" {
			return "none"
		}
		return "types"
	}
	if level == "types" {
		return "types"
	}
	return "none"
}

// truncateGraph is the `limit` cut shared by GET /api/graph/{project} and the
// MCP get_graph tool (step 2): unbounded requests (no around, no container)
// are cut to `limit` nodes, edges between two cut nodes kept, edges to a
// dropped node dropped. `bounded` requests (around or container) are never
// cut here — their own size is the point of asking for them.
func truncateGraph(g *core.Graph, bounded bool, limit int) (out *core.Graph, truncated bool, fullNodes, fullEdges int) {
	fullNodes, fullEdges = len(g.Nodes), len(g.Edges)
	if bounded || len(g.Nodes) <= limit {
		return g, false, fullNodes, fullEdges
	}
	keep := map[string]bool{}
	nodes := append([]core.GraphNode{}, g.Nodes[:limit]...)
	for _, n := range nodes {
		keep[n.ID] = true
	}
	var edges []core.GraphEdge
	for _, e := range g.Edges {
		if keep[e.From] && keep[e.To] {
			edges = append(edges, e)
		}
	}
	return &core.Graph{Nodes: nodes, Edges: edges}, true, fullNodes, fullEdges
}

// intParam parses a query/tool integer parameter that falls back to
// `def` (the effective mcp.list_cap/limit setting) when absent or <= 0 — the
// request wins over the setting exactly when it actually names one (step 2).
func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// parseFanout: 0 (off, the default) or a positive integer.
func parseFanout(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, errors.New("fanout: must be a non-negative number")
	}
	return n, nil
}

// containerDefs is the full containers.json list, as core.FilterContainer
// needs it to find descendants by `parent` — not just which ids exist.
func containerDefs(m *core.Model) ([]core.Container, error) {
	containers, err := core.LoadContainers(m.ProjectDir())
	if err != nil {
		return nil, err
	}
	if containers == nil {
		return nil, nil
	}
	return containers.List, nil
}

// parseBoolParam: the `missing` query/tool parameter. Absent, "0" and
// "false" are all "leave missing stuff out" (the default); "1" or "true"
// include it.
func parseBoolParam(s string) bool {
	return s == "1" || strings.EqualFold(s, "true")
}

// fieldSetParam resolves `fields=`, shared by the HTTP endpoint and the MCP
// tool: the parameter absent means the default (via,position,dynamic);
// present — even as an empty list or an empty string — means exactly what
// was listed, nothing added back. `present` and `names` must agree: `names`
// is only consulted when `present` is true.
func fieldSetParam(present bool, names []string) (map[string]bool, error) {
	if !present {
		return map[string]bool{"via": true, "position": true, "dynamic": true}, nil
	}
	return core.ParseFields(names)
}

// parseDepth: default 1, must be in 1..5 (docs/plans/PLAN_20260928_host_graph-provider.md step 4),
// or "all" (core.WalkAll) — checked against `follow` by checkWalkAll.
func parseDepth(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	if strings.EqualFold(s, "all") {
		return core.WalkAll, nil
	}
	d, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New(`depth: not a number (1-5, or "all" along inheritance relations)`)
	}
	if d <= 0 || d > 5 {
		return 0, errors.New(`depth: must be between 1 and 5, or "all" along inheritance relations`)
	}
	return d, nil
}

// checkWalkAll: `depth: "all"` walks until nothing new is reached, which is
// only a bounded question along inheritance; with any other relation in
// `follow` (or with `follow` absent, whose default is everything but
// containment) it is refused, saying which relations it takes.
func checkWalkAll(depth int, follow []core.Relation, followGiven bool) error {
	if depth != core.WalkAll || core.AllInheritance(follow) {
		return nil
	}
	got := "follow is absent (the default is every relation but containment)"
	if followGiven {
		names := make([]string, len(follow))
		for i, r := range follow {
			names[i] = r.Name
		}
		got = "follow has " + strings.Join(names, ", ")
	}
	return fmt.Errorf(`depth "all" needs every relation in follow to be an inheritance relation (%s); %s — give a number 1-5, or narrow follow`, strings.Join(core.InheritanceRelationNames(), ", "), got)
}

// graphStats: hiddenMissing is how many nodes/edges FilterMissing left out
// (0/0 when `missing=1` asked to include them), so a reader knows they exist
// even though this answer does not carry them.
func graphStats(g *core.Graph, hiddenNodes, hiddenEdges int) map[string]any {
	byPresence := map[string]int{}
	for _, n := range g.Nodes {
		byPresence[n.Presence]++
	}
	return map[string]any{
		"nodes": len(g.Nodes), "edges": len(g.Edges), "byPresence": byPresence,
		"hiddenMissing": map[string]any{"nodes": hiddenNodes, "edges": hiddenEdges},
	}
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
