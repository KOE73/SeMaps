package main

// `semaps mcp`: the registry as MCP tools over stdio (ADR_20260924-5,
// docs/API.md §6). The tools are thin: every rule of writing lives in core.
// stdout carries the protocol, so extractor logs go to stderr.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"semaps/core"
)

type mcpServer struct {
	proj       project // the .semaps file: extractors
	workspace  string
	sourceRoot string
	project    string // --project; "" — the only one
	models     *modelService
	// onRunFinish, when set, is attached to every run store this server
	// creates for `extract`/`sync` (s.runs), so the graph service hears
	// about MCP-triggered runs too, not only ones started through the tool
	// API (PLAN_20260928_host_graph-provider.md step 3/5).
	onRunFinish func(*runInfo)
}

// Every structured answer is a JSON object: MCP clients (Claude Code among
// them) reject an array in structuredContent, so lists come wrapped —
// {views: [...]}, {relationTypes: [...]}, {runs: [...]}, {reports: [...]}.

// Tool inputs. `project` is optional everywhere: --project, else the only one.

type projectIn struct {
	Project string `json:"project,omitempty" jsonschema:"project id; default: --project or the only project"`
}

type entityIn struct {
	Project string `json:"project,omitempty"`
	ID      string `json:"id,omitempty" jsonschema:"entity id (e_...)"`
	Symbol  string `json:"symbol,omitempty" jsonschema:"extractor symbol id, when the entity id is not known"`
}

type findIn struct {
	Project string `json:"project,omitempty"`
	Query   string `json:"query,omitempty" jsonschema:"substring of name, symbol or namespace, any case"`
	Kind    string `json:"kind,omitempty" jsonschema:"entity kind: class, interface, assembly..."`
	Status  string `json:"status,omitempty" jsonschema:"present, missing or planned"`
	Limit   int    `json:"limit,omitempty" jsonschema:"at most this many; default 50"`
}

type relationsIn struct {
	Project   string `json:"project,omitempty"`
	Entity    string `json:"entity,omitempty" jsonschema:"entity id; empty: all relations"`
	Direction string `json:"direction,omitempty" jsonschema:"out, in or both (default)"`
	Type      string `json:"type,omitempty" jsonschema:"relation type, or a prefix ending with a dot: holds."`
	Status    string `json:"status,omitempty" jsonschema:"present or missing"`
	Limit     int    `json:"limit,omitempty" jsonschema:"at most this many; default 200"`
}

type textIn struct {
	Project string `json:"project,omitempty"`
	Lang    string `json:"lang" jsonschema:"language of text.<lang>.json: ru, en..."`
	Key     string `json:"key" jsonschema:"e_, c_, rt_, r_, v_ or z_ id"`
}

type setTextIn struct {
	Project string `json:"project,omitempty"`
	Lang    string `json:"lang"`
	Key     string `json:"key"`
	Field   string `json:"field" jsonschema:"name, title, description, doc, fromLabel or toLabel"`
	Value   string `json:"value"`
}

type addRelationIn struct {
	Project string `json:"project,omitempty"`
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type" jsonschema:"an id from relation-types.json"`
}

type addTypeIn struct {
	Project    string `json:"project,omitempty"`
	ID         string `json:"id"`
	Visibility string `json:"visibility,omitempty" jsonschema:"visible or hidden: the default on views"`
	StyleID    string `json:"styleId,omitempty"`
}

type visibleIn struct {
	Project  string `json:"project,omitempty"`
	View     string `json:"view"`
	Relation string `json:"relation"`
	Visible  bool   `json:"visible"`
}

type renameIn struct {
	Project  string `json:"project,omitempty"`
	Entity   string `json:"entity,omitempty" jsonschema:"old entity id: takes the new symbol"`
	Symbol   string `json:"symbol,omitempty"`
	Relation string `json:"relation,omitempty" jsonschema:"old member relation id: takes the new member name"`
	Member   string `json:"member,omitempty"`
}

type extractIn struct {
	Extractor string `json:"extractor,omitempty" jsonschema:"extractor id from the .semaps file; default: all"`
}

type syncIn struct {
	Extractor string `json:"extractor,omitempty"`
	Run       string `json:"run,omitempty" jsonschema:"use the facts of this run (from extract) instead of extracting again"`
	NoRenames bool   `json:"noRenames,omitempty"`
}

type placeIn struct {
	Project          string           `json:"project,omitempty"`
	View             string           `json:"view"`
	Entities         []core.Placement `json:"entities"`
	RequestedByHuman bool             `json:"requestedByHuman" jsonschema:"true only when a human asked for this layout in so many words"`
}

// getGraphIn: same parameters as GET /api/graph/{project} (host/graph.go),
// plus `limit` so an agent never gets the whole graph by accident
// (docs/plans/PLAN_20260928_host_graph-provider.md step 6).
type getGraphIn struct {
	Project string `json:"project,omitempty"`
	Level   string `json:"level,omitempty" jsonschema:"types (drop function/value/method nodes) or all; default types without around, all with around"`
	Lift    string `json:"lift,omitempty" jsonschema:"types (fold method-level edges up to their containing types, ADR_20260928-3 §6) or none; default types unless around is a method, or (without around) unless level=all"`
	// Kinds is only for a whole-graph request (no Around): a neighbourhood
	// names Follow instead (part 1 of the agent-answers rework).
	Kinds     string   `json:"kinds,omitempty" jsonschema:"whole-graph only (no around): comma list of edge kinds to keep; default: all"`
	Around    string   `json:"around,omitempty" jsonschema:"a node id or a name (part 2: resolved by id, then name, case-insensitively as a last resort) — only its neighbourhood"`
	Follow    []string `json:"follow,omitempty" jsonschema:"neighbourhood only: directed relation names to walk (call graph_formats for the vocabulary); default: every relation but containment"`
	Depth     int      `json:"depth,omitempty" jsonschema:"with around: 1-5, default 1"`
	Fanout    int      `json:"fanout,omitempty" jsonschema:"with around: at most this many neighbours per node per relation; 0 (default) = unlimited"`
	Container string   `json:"container,omitempty" jsonschema:"a containers.json id: only nodes in it, or in a descendant of it"`
	// Fields is a list, not a comma string, so null/absent (the default,
	// via+position) can be told apart from an explicit empty list (nothing
	// extra): a plain string can't carry that distinction over JSON.
	Fields   []string `json:"fields,omitempty" jsonschema:"members, via, position, memberLines; default (omitted/null) via,position; [] for none"`
	Missing  bool     `json:"missing,omitempty" jsonschema:"include model-only nodes/edges whose entity/relation has status missing; default false"`
	Limit    int      `json:"limit,omitempty" jsonschema:"without around or container, cut to this many nodes; default 200"`
	Format   string   `json:"format,omitempty" jsonschema:"answer format name, or call graph_formats to see them; default: facts (provisional)"`
	Template string   `json:"template,omitempty" jsonschema:"a template of your own (see graph_formats for the macro dictionary and rules), instead of a named format's"`
}

type findNodeIn struct {
	Project string `json:"project,omitempty"`
	Q       string `json:"q" jsonschema:"substring of a node's name or id, case-insensitive"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 50"`
}

type getViewIn struct {
	Project string `json:"project,omitempty"`
	View    string `json:"view" jsonschema:"a view id, or a zone reference view#zone to read only that subtree"`
	Lang    string `json:"lang,omitempty" jsonschema:"language of names; default ru"`
}

type geomIn struct {
	Project          string   `json:"project,omitempty"`
	View             string   `json:"view,omitempty" jsonschema:"view id; may be left out when elements are references view#id"`
	Elements         []string `json:"elements" jsonschema:"zone ids or entity ids of nodes, or references view#id"`
	DX               *float64 `json:"dx,omitempty"`
	DY               *float64 `json:"dy,omitempty"`
	X                *float64 `json:"x,omitempty" jsonschema:"move: the top-left corner of the common box of the elements goes here"`
	Y                *float64 `json:"y,omitempty"`
	Width            *float64 `json:"width,omitempty"`
	Height           *float64 `json:"height,omitempty"`
	Zone             string   `json:"zone,omitempty" jsonschema:"set_zone: the zone to put them into; empty takes them out of any"`
	Mode             string   `json:"mode,omitempty" jsonschema:"align: left, right, top, bottom, width or height, to the first element"`
	RequestedByHuman bool     `json:"requestedByHuman" jsonschema:"true only when a human asked for this layout in so many words"`
}

type arrangeIn struct {
	Project          string   `json:"project,omitempty"`
	Reference        string   `json:"reference" jsonschema:"the zone to copy the layout from: view#zone"`
	Targets          []string `json:"targets,omitempty" jsonschema:"zones to lay out like the reference: references or zone ids"`
	Parent           string   `json:"parent,omitempty" jsonschema:"instead of targets: every zone next to the reference inside this zone; they are also restacked with the reference gap"`
	DryRun           bool     `json:"dryRun,omitempty" jsonschema:"only report what would be done; nothing is written"`
	RequestedByHuman bool     `json:"requestedByHuman"`
}

type addZoneIn struct {
	Project          string  `json:"project,omitempty"`
	View             string  `json:"view"`
	ID               string  `json:"id" jsonschema:"z_<name>"`
	Parent           string  `json:"parent,omitempty"`
	Container        string  `json:"container,omitempty" jsonschema:"a container id of containers.json; empty for a frame that asserts nothing"`
	StyleID          string  `json:"styleId,omitempty"`
	Name             string  `json:"name,omitempty" jsonschema:"caption, written as a text under the zone id"`
	Lang             string  `json:"lang,omitempty"`
	X                float64 `json:"x"`
	Y                float64 `json:"y"`
	Width            float64 `json:"width"`
	Height           float64 `json:"height"`
	RequestedByHuman bool    `json:"requestedByHuman"`
}

type saveIn struct {
	Project          string `json:"project,omitempty"`
	RequestedByHuman bool   `json:"requestedByHuman"`
}

type discardIn struct {
	Project          string `json:"project,omitempty"`
	Scope            string `json:"scope" jsonschema:"registry, view or all"`
	View             string `json:"view,omitempty"`
	RequestedByHuman bool   `json:"requestedByHuman"`
}

func (s *mcpServer) server() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "semaps", Version: "1"}, &mcp.ServerOptions{
		Instructions: "SeMaps registry of this repository. Read with list_*/get_*/find_*; write only through these tools. " +
			"Nothing can be deleted; view geometry only with requestedByHuman when a human asked. See docs/ADOPTING.md.",
	})
	// Reading tools say so (readOnlyHint): a client may run them without asking,
	// and the editor's sandbox asks before any other.
	read := func(name, desc string) *mcp.Tool {
		return &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
	}
	notDestructive := false
	write := func(name, desc string) *mcp.Tool {
		return &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive}}
	}

	mcp.AddTool(srv, read("list_projects", "Projects of the workspace and their views."), s.listProjects)
	mcp.AddTool(srv, read("list_views", "Views of one project."), s.listViews)
	mcp.AddTool(srv, read("get_entity", "One entity by id or by symbol."), s.getEntity)
	mcp.AddTool(srv, read("find_entities", "Entities by name/symbol/namespace substring, kind, status."), s.findEntities)
	mcp.AddTool(srv, read("get_relations", "Relations of an entity (or all), by direction, type, status."), s.getRelations)
	mcp.AddTool(srv, read("get_relation_types", "The relation-type vocabulary with default visibility."), s.getRelationTypes)
	mcp.AddTool(srv, read("get_text", "Text entry of a key in one language."), s.getText)
	mcp.AddTool(srv, read("get_view", "A view with geometry as a tree: zones with their nodes, absolute rectangles, visible lines, what is unsaved. A zone reference reads only its subtree."), s.getView)
	mcp.AddTool(srv, read("doctor", "Extractors and runtimes found, and the model check of the workspace."), s.doctor)
	mcp.AddTool(srv, read("get_graph", "The live code graph: symbols and their code positions joined with entities and containers, docs/adr/ADR_20260928_host_live-code-graph.md. Bound it with around+depth or container, or it is cut to limit nodes (truncated: true, with the full counts). format/set pick the answer shape and which edges count; see graph_formats."), s.getGraph)
	mcp.AddTool(srv, read("graph_formats", "The answer formats, the relation vocabulary follow accepts, the template macro dictionary and rules, and the defaults."), s.graphFormats)
	mcp.AddTool(srv, read("find_node", "Substring search over graph node names/ids: candidates to pass as get_graph's around."), s.findNode)
	mcp.AddTool(srv, read("sync_preview", "What sync would change, writing nothing (= semaps sync --dry-run)."), s.syncPreview)

	mcp.AddTool(srv, write("set_text", "Write one text field as authored, with at = now."), s.setText)
	mcp.AddTool(srv, write("add_relation", "Add an authored relation; the id is minted and returned."), s.addRelation)
	mcp.AddTool(srv, write("add_relation_type", "Add an authored relation type."), s.addRelationType)
	mcp.AddTool(srv, write("set_relation_visible", "Show or hide one relation on one view (relations.except)."), s.setRelationVisible)
	mcp.AddTool(srv, write("confirm_rename", "Answer a sync rename candidate: entity + symbol, or relation + member."), s.confirmRename)
	mcp.AddTool(srv, write("extract", "Run the extractors of the .semaps file; returns run ids."), s.extract)
	mcp.AddTool(srv, write("sync", "Reconcile the registry with the code (extracts first unless run is given)."), s.sync)
	mcp.AddTool(srv, write("place_entities", "Put entities on a view. Only when a human asked for it: requestedByHuman."), s.placeEntities)
	mcp.AddTool(srv, write("move_elements", "Move nodes and zones by dx/dy or to x/y; a zone goes with its content. Only when a human asked: requestedByHuman."), s.moveElements)
	mcp.AddTool(srv, write("resize_elements", "Set width/height of nodes and zones, not below the minimum nor, for a zone, below its content. requestedByHuman."), s.resizeElements)
	mcp.AddTool(srv, write("set_zone", "Put nodes and zones into a zone, coordinates untouched. requestedByHuman."), s.setZone)
	mcp.AddTool(srv, write("add_zone", "Add a zone with a rectangle, optional parent, container, style and caption. requestedByHuman."), s.addZone)
	mcp.AddTool(srv, write("fit_zone", "Fit zones to their content (caption strip and padding); ancestors grow if they no longer hold it. requestedByHuman."), s.fitZone)
	mcp.AddTool(srv, write("align_elements", "Align elements to the first one: left, right, top, bottom, width, height. requestedByHuman."), s.alignElements)
	mcp.AddTool(srv, write("arrange_like", "Lay out target zones like a reference zone: node positions and sizes, template, style, zone size; nodes are paired by their part in the inheritance and by variant words. dryRun shows the plan. requestedByHuman."), s.arrangeLike)
	mcp.AddTool(srv, write("save", "Save all unsaved project changes only when a human explicitly requested it."), s.save)
	mcp.AddTool(srv, write("discard", "Discard unsaved changes only when a human explicitly requested it."), s.discard)
	return srv
}

func (s *mcpServer) save(_ context.Context, _ *mcp.CallToolRequest, in saveIn) (*mcp.CallToolResult, any, error) {
	if !in.RequestedByHuman {
		return nil, nil, errors.New("save requires requestedByHuman: true")
	}
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.Save(); err != nil {
		return nil, nil, err
	}
	if s.models != nil {
		s.models.publish(m.ProjectID(), modelEvent{Author: "agent", Changed: []core.Ref{}, Dirty: m.Dirty()})
	}
	return done("project saved")
}

func (s *mcpServer) discard(_ context.Context, _ *mcp.CallToolRequest, in discardIn) (*mcp.CallToolResult, any, error) {
	if !in.RequestedByHuman {
		return nil, nil, errors.New("discard requires requestedByHuman: true")
	}
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.Discard(in.Scope, in.View); err != nil {
		return nil, nil, err
	}
	if s.models != nil {
		s.models.publish(m.ProjectID(), modelEvent{Author: "agent", Changed: []core.Ref{}, Dirty: m.Dirty()})
	}
	return done("unsaved changes discarded")
}

func (s *mcpServer) pick(p string) string {
	if p != "" {
		return p
	}
	return s.project
}

// done is the result of a write: a line of text for the agent.
func done(format string, a ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}}}, nil, nil
}

// ------------------------------------------------------------------ reading

func (s *mcpServer) listProjects(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, core.Index(s.workspace), nil
}

func (s *mcpServer) listViews(_ context.Context, _ *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, any, error) {
	dir, err := core.ProjectDir(s.workspace, s.pick(in.Project))
	if err != nil {
		return nil, nil, err
	}
	for _, p := range core.Index(s.workspace).Projects {
		if p.ID == filepath.Base(dir) {
			return nil, map[string]any{"views": p.Views}, nil
		}
	}
	return nil, map[string]any{"views": []any{}}, nil
}

// record is one registry item, read for filtering.
type record struct {
	raw    json.RawMessage
	fields map[string]any
}

func (r record) str(k string) string { s, _ := r.fields[k].(string); return s }

func (s *mcpServer) records(project, file string) ([]record, error) {
	m, err := s.model(project)
	if err != nil {
		return nil, err
	}
	raws, err := m.Records(file)
	if err != nil {
		return nil, err
	}
	out := make([]record, len(raws))
	for i, raw := range raws {
		out[i].raw = raw
		_ = json.Unmarshal(raw, &out[i].fields)
	}
	return out, nil
}

func (s *mcpServer) model(project string) (*core.Model, error) {
	if s.models == nil {
		return core.LoadModel(s.workspace, s.pick(project))
	}
	id := s.pick(project)
	if id == "" {
		dir, err := core.ProjectDir(s.workspace, "")
		if err != nil {
			return nil, err
		}
		id = filepath.Base(dir)
	}
	return s.models.get(id)
}

func (s *mcpServer) changed(project string, m *core.Model) {
	if s.models == nil {
		return
	}
	dirty := m.Dirty()
	refs := append([]core.Ref{}, dirty.Registry...)
	for _, viewRefs := range dirty.Views {
		refs = append(refs, viewRefs...)
	}
	s.models.publish(m.ProjectID(), modelEvent{Author: "agent", Changed: refs, Dirty: dirty})
}

func (s *mcpServer) reviewLink(m *core.Model, id string) string {
	for _, p := range core.Index(s.workspace).Projects {
		if p.ID != m.ProjectID() {
			continue
		}
		for _, v := range p.Views {
			if v.ID == id {
				return "/app/#" + v.ID
			}
			b, err := m.View(v.ID)
			if err != nil {
				continue
			}
			if strings.Contains(string(b), `"`+id+`"`) {
				return "/app/#" + v.ID + "?highlight=" + url.QueryEscape(id)
			}
		}
		if len(p.Views) > 0 {
			return "/app/#" + p.Views[0].ID + "?highlight=" + url.QueryEscape(id)
		}
	}
	return "/app/"
}

func raws(list []record) []json.RawMessage {
	out := make([]json.RawMessage, len(list))
	for i, r := range list {
		out[i] = r.raw
	}
	return out
}

func (s *mcpServer) getEntity(_ context.Context, _ *mcp.CallToolRequest, in entityIn) (*mcp.CallToolResult, any, error) {
	if (in.ID == "") == (in.Symbol == "") {
		return nil, nil, errors.New("pass id or symbol")
	}
	ents, err := s.records(in.Project, "entities.json")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range ents {
		if (in.ID != "" && e.str("id") == in.ID) || (in.Symbol != "" && e.str("symbol") == in.Symbol) {
			return nil, e.raw, nil
		}
	}
	return nil, nil, fmt.Errorf("no entity %s%s", in.ID, in.Symbol)
}

func (s *mcpServer) findEntities(_ context.Context, _ *mcp.CallToolRequest, in findIn) (*mcp.CallToolResult, any, error) {
	ents, err := s.records(in.Project, "entities.json")
	if err != nil {
		return nil, nil, err
	}
	q := strings.ToLower(in.Query)
	limit := orInt(in.Limit, 50)
	var out []record
	total := 0
	for _, e := range ents {
		if in.Kind != "" && e.str("kind") != in.Kind {
			continue
		}
		if in.Status != "" && orStr(e.str("status"), "present") != in.Status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.str("name")+"\x00"+e.str("symbol")+"\x00"+e.str("namespace")+"\x00"+e.str("id")), q) {
			continue
		}
		total++
		if len(out) < limit {
			out = append(out, e)
		}
	}
	return nil, map[string]any{"total": total, "entities": raws(out)}, nil
}

func (s *mcpServer) getRelations(_ context.Context, _ *mcp.CallToolRequest, in relationsIn) (*mcp.CallToolResult, any, error) {
	rels, err := s.records(in.Project, "relations.json")
	if err != nil {
		return nil, nil, err
	}
	dir := orStr(in.Direction, "both")
	if dir != "in" && dir != "out" && dir != "both" {
		return nil, nil, fmt.Errorf("direction %q: out, in or both", dir)
	}
	limit := orInt(in.Limit, 200)
	var out []record
	total := 0
	for _, r := range rels {
		if in.Entity != "" {
			out1 := r.str("from") == in.Entity && dir != "in"
			in1 := r.str("to") == in.Entity && dir != "out"
			if !out1 && !in1 {
				continue
			}
		}
		t := orStr(r.str("type"), r.str("relation"))
		if in.Type != "" && t != in.Type && !(strings.HasSuffix(in.Type, ".") && strings.HasPrefix(t, in.Type)) {
			continue
		}
		if in.Status != "" && orStr(r.str("status"), "present") != in.Status {
			continue
		}
		total++
		if len(out) < limit {
			out = append(out, r)
		}
	}
	return nil, map[string]any{"total": total, "relations": raws(out)}, nil
}

func (s *mcpServer) getRelationTypes(_ context.Context, _ *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, any, error) {
	types, err := s.records(in.Project, "relation-types.json")
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"relationTypes": raws(types)}, nil
}

// getGraph is get_graph (docs/plans/PLAN_20260928_host_graph-provider.md step
// 6): the same filters as GET /api/graph/{project} (host/graph.go), reusing
// the same core filter functions, plus a `limit` on the node count so an
// agent asking for a whole project's graph without `around` or `container`
// gets a cut answer with `truncated: true` and the full counts, not
// everything at once.
func (s *mcpServer) getGraph(_ context.Context, _ *mcp.CallToolRequest, in getGraphIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	sources, facts := sourcesFor(s.proj, m.ProjectID())
	graph, err := core.BuildGraph(sources, m)
	if err != nil {
		return nil, nil, err
	}

	fields, err := fieldSetParam(in.Fields != nil, in.Fields)
	if err != nil {
		return nil, nil, err
	}
	if fields["members"] {
		if err := core.AttachMembers(graph, m); err != nil {
			return nil, nil, err
		}
	}

	formatName := in.Format
	if formatName == "" && in.Template == "" {
		formatName = core.DefaultToolFormat
	}
	var formatter core.GraphFormatter
	if in.Template == "" {
		var ok bool
		formatter, ok = core.GetGraphFormat(formatName)
		if !ok {
			return nil, nil, core.UnknownFormatError(formatName)
		}
	}
	if formatName == "tree" && in.Around == "" {
		return nil, nil, errors.New("format=tree needs around=... (a focus node)")
	}
	if in.Around != "" && in.Kinds != "" {
		return nil, nil, errors.New("kinds is only for a whole-graph request (no around); a neighbourhood names follow")
	}

	var notice string
	var around, aroundKind string
	if in.Around != "" {
		res := core.ResolveNode(graph, in.Around)
		switch {
		case len(res.Candidates) > 0:
			return nil, map[string]any{"error": "ambiguous", "asked": in.Around, "candidates": res.Candidates}, nil
		case res.Node == nil:
			return nil, nil, fmt.Errorf("no such node: %q (nearest: %s)", in.Around, strings.Join(res.Suggestions, ", "))
		case res.MissingHidden && !in.Missing:
			return nil, nil, fmt.Errorf("node %q is hidden as missing; pass missing:true to include it", res.Node.ID)
		}
		around, aroundKind, notice = res.Node.ID, res.Node.Kind, res.Notice
	}

	var hiddenNodes, hiddenEdges int
	graph, hiddenNodes, hiddenEdges = core.FilterMissing(graph, in.Missing)

	level := in.Level
	if level == "" {
		if around == "" {
			level = "types"
		} else {
			level = "all"
		}
	}

	lift := in.Lift
	if lift == "" {
		lift = defaultLift(around != "", aroundKind, level)
	}
	if lift != "types" && lift != "none" {
		return nil, nil, fmt.Errorf("lift: %q is neither %q nor %q", lift, "types", "none")
	}
	if lift == "types" {
		graph = core.LiftToTypes(graph)
	}

	if graph, err = core.FilterLevel(graph, level); err != nil {
		return nil, nil, err
	}

	bounded := false
	var fanoutNotes []core.FanoutNote
	if around != "" {
		follow, err := core.ParseFollow(in.Follow)
		if err != nil {
			return nil, nil, err
		}
		depth := in.Depth
		if depth == 0 {
			depth = 1
		}
		if depth < 1 || depth > 5 {
			return nil, nil, errors.New("depth: must be between 1 and 5")
		}
		if graph, fanoutNotes, err = core.Walk(graph, around, depth, follow, in.Fanout); err != nil {
			return nil, nil, err
		}
		bounded = true
	} else if in.Kinds != "" {
		graph = core.FilterEdgeKinds(graph, splitCSV(in.Kinds))
	}
	if in.Container != "" {
		defs, err := containerDefs(m)
		if err != nil {
			return nil, nil, err
		}
		if graph, err = core.FilterContainer(graph, in.Container, defs); err != nil {
			return nil, nil, err
		}
		bounded = true
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 200
	}
	fullNodes, fullEdges := len(graph.Nodes), len(graph.Edges)
	truncated := false
	if !bounded && len(graph.Nodes) > limit {
		keep := map[string]bool{}
		nodes := append([]core.GraphNode{}, graph.Nodes[:limit]...)
		for _, n := range nodes {
			keep[n.ID] = true
		}
		var edges []core.GraphEdge
		for _, e := range graph.Edges {
			if keep[e.From] && keep[e.To] {
				edges = append(edges, e)
			}
		}
		graph, truncated = &core.Graph{Nodes: nodes, Edges: edges}, true
	}

	stats := graphStats(graph, hiddenNodes, hiddenEdges)
	opts := core.FormatOptions{
		Focus: around, Facts: facts, Stats: stats, Fields: fields,
		Truncated: truncated, FullNodes: fullNodes, FullEdges: fullEdges,
		FanoutNotes: fanoutNotes, Notice: notice,
	}
	var body []byte
	if in.Template != "" {
		body, err = core.FormatWithTemplate(graph, in.Template, opts)
	} else {
		body, err = formatter.Format(graph, opts)
	}
	if err != nil {
		return nil, nil, err
	}

	// A JSON format also carries structuredContent, as before (the same
	// bytes, parsed back, so text and structured content never disagree); a
	// text format is returned as the tool's text content only
	// (docs/API.md §6).
	if in.Template == "" && (formatName == core.DefaultGraphFormat || formatName == "json-compact") {
		var structured map[string]any
		if err := json.Unmarshal(body, &structured); err != nil {
			return nil, nil, err
		}
		// The tool always reports truncated (true/false), unlike the HTTP
		// JSON body, which (unchanged from before this task) only carries it
		// when true.
		structured["truncated"] = truncated
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, structured, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

// findNode is find_node (part 2): a plain substring search over names and
// ids, for a caller unsure of the exact spelling to give `around`.
func (s *mcpServer) findNode(_ context.Context, _ *mcp.CallToolRequest, in findNodeIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	sources, _ := sourcesFor(s.proj, m.ProjectID())
	graph, err := core.BuildGraph(sources, m)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"candidates": core.FindNodes(graph, in.Q, in.Limit)}, nil
}

// graphFormats is graph_formats: the same list GET /api/graph-formats gives,
// so a client (or an agent) can see the available formats/sets and defaults
// without guessing.
func (s *mcpServer) graphFormats(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, graphFormatsPayload(), nil
}

func (s *mcpServer) getText(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	entry, err := m.Text(in.Lang, in.Key)
	if err != nil {
		return nil, nil, err
	}
	if entry == nil {
		return nil, map[string]any{}, nil
	}
	return nil, entry, nil
}

func (s *mcpServer) doctor(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	var b bytes.Buffer
	code := runDoctor(&b, s.proj)
	return nil, map[string]any{
		"extractors":   b.String(),
		"extractorsOK": code == 0,
		"findings":     core.Check(s.workspace, s.sourceRoot),
	}, nil
}

// ------------------------------------------------------------------ writing

func (s *mcpServer) setText(_ context.Context, _ *mcp.CallToolRequest, in setTextIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.SetText(in.Lang, in.Key, in.Field, in.Value, "agent"); err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	return done("%s.%s (%s) changed, not saved; review and Save: %s", in.Key, in.Field, in.Lang, s.reviewLink(m, in.Key))
}

func (s *mcpServer) addRelation(_ context.Context, _ *mcp.CallToolRequest, in addRelationIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	id, err := m.AddRelation(in.From, in.To, in.Type, "agent")
	if err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	return done("%s added, not saved; review and Save: %s", id, s.reviewLink(m, id))
}

func (s *mcpServer) addRelationType(_ context.Context, _ *mcp.CallToolRequest, in addTypeIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.AddRelationType(in.ID, in.Visibility, in.StyleID, "agent"); err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	return done("type %s added, not saved; give it a name: set_text key rt_%s; review: %s", in.ID, in.ID, s.reviewLink(m, "rt_"+in.ID))
}

func (s *mcpServer) setRelationVisible(_ context.Context, _ *mcp.CallToolRequest, in visibleIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.SetRelationVisible(in.View, in.Relation, in.Visible, "agent"); err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	return done("%s on %s: visible=%v, not saved; review and Save: %s", in.Relation, in.View, in.Visible, s.reviewLink(m, in.Relation))
}

func (s *mcpServer) confirmRename(_ context.Context, _ *mcp.CallToolRequest, in renameIn) (*mcp.CallToolResult, any, error) {
	p := s.pick(in.Project)
	switch {
	case in.Entity != "" && in.Relation == "":
		m, err := s.model(p)
		if err != nil {
			return nil, nil, err
		}
		if err := m.ConfirmEntityRename(in.Entity, in.Symbol, "agent"); err != nil {
			return nil, nil, err
		}
		s.changed(p, m)
		return done("%s takes symbol %s, not saved; run sync again; review: %s", in.Entity, in.Symbol, s.reviewLink(m, in.Entity))
	case in.Relation != "" && in.Entity == "":
		m, err := s.model(p)
		if err != nil {
			return nil, nil, err
		}
		if err := m.ConfirmRelationRename(in.Relation, in.Member, "agent"); err != nil {
			return nil, nil, err
		}
		s.changed(p, m)
		return done("%s takes member %s, not saved; run sync again; review: %s", in.Relation, in.Member, s.reviewLink(m, in.Relation))
	}
	return nil, nil, errors.New("pass entity+symbol or relation+member")
}

// runs starts the extractors one by one and waits; logs go to stderr.
func (s *mcpServer) runs(id string) ([]*runInfo, error) {
	list, err := selectExtractors(s.proj, id)
	if err != nil {
		return nil, err
	}
	store := newRunStore(s.proj.File)
	store.onFinish = s.onRunFinish
	var out []*runInfo
	for _, e := range list {
		info, wait, err := store.start(s.proj, e, os.Stderr, "")
		if err != nil {
			return out, fmt.Errorf("%s: %w", e.ID, err)
		}
		<-wait
		info, err = store.get(info.ID)
		if err != nil {
			return out, err
		}
		if info.State != "done" {
			return out, fmt.Errorf("%s: run %s ended %s; its log: %s", e.ID, info.ID, info.State, store.path(info.ID, "log.txt"))
		}
		out = append(out, info)
	}
	return out, nil
}

func (s *mcpServer) extract(_ context.Context, _ *mcp.CallToolRequest, in extractIn) (*mcp.CallToolResult, any, error) {
	runs, err := s.runs(in.Extractor)
	if err != nil {
		return nil, nil, err
	}
	out := make([]map[string]string, len(runs))
	for i, r := range runs {
		out[i] = map[string]string{"run": r.ID, "extractor": r.Extractor, "project": r.Project}
	}
	return nil, map[string]any{"runs": out}, nil
}

func (s *mcpServer) reconcile(in syncIn, dryRun bool) (*mcp.CallToolResult, any, error) {
	store := newRunStore(s.proj.File)
	var runs []*runInfo
	if in.Run != "" {
		info, err := store.get(in.Run)
		if err != nil {
			return nil, nil, fmt.Errorf("run %s: %w", in.Run, err)
		}
		runs = []*runInfo{info}
	} else {
		var err error
		if runs, err = s.runs(in.Extractor); err != nil {
			return nil, nil, err
		}
	}
	var b strings.Builder
	var reports []*core.SyncReport
	for _, info := range runs {
		m, err := s.model(info.Project)
		if err != nil {
			return nil, nil, err
		}
		rep, err := store.syncRunModel(m, info, core.SyncOptions{DryRun: dryRun, NoRenames: in.NoRenames})
		if err != nil {
			return nil, nil, err
		}
		fmt.Fprintf(&b, "== %s → project %s (run %s)\n", info.Extractor, info.Project, info.ID)
		rep.Print(&b)
		reports = append(reports, rep)
		if !dryRun && len(rep.Written) > 0 {
			s.changed(info.Project, m)
		}
	}
	if !dryRun {
		b.WriteString("Changes are not saved; review in the editor and Save.\n")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, map[string]any{"reports": reports}, nil
}

func (s *mcpServer) syncPreview(_ context.Context, _ *mcp.CallToolRequest, in syncIn) (*mcp.CallToolResult, any, error) {
	return s.reconcile(in, true)
}

func (s *mcpServer) sync(_ context.Context, _ *mcp.CallToolRequest, in syncIn) (*mcp.CallToolResult, any, error) {
	return s.reconcile(in, false)
}

func (s *mcpServer) placeEntities(_ context.Context, _ *mcp.CallToolRequest, in placeIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if err := m.PlaceEntities(in.View, in.Entities, in.RequestedByHuman, "agent"); err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	return done("%d placed on %s, not saved; review and Save: %s", len(in.Entities), in.View, s.reviewLink(m, in.View))
}

// modelOfRef picks the model a reference belongs to: its own project prefix, else project.
func (s *mcpServer) modelOfRef(project, ref string) (*core.Model, error) {
	r, err := core.ParseRef(ref)
	if err != nil {
		return nil, err
	}
	if r.Project != "" {
		project = r.Project
	}
	return s.model(project)
}

func (s *mcpServer) getView(_ context.Context, _ *mcp.CallToolRequest, in getViewIn) (*mcp.CallToolResult, any, error) {
	m, err := s.modelOfRef(in.Project, in.View)
	if err != nil {
		return nil, nil, err
	}
	info, err := m.GetView(in.View, in.Lang)
	if err != nil {
		return nil, nil, err
	}
	return nil, info, nil
}

// geomTarget picks the model and the view of a geometry step: view, or the view of the first reference.
func (s *mcpServer) geomTarget(project, view string, elements []string) (*core.Model, string, error) {
	ref := view
	if ref == "" && len(elements) > 0 {
		ref = elements[0]
	}
	if ref == "" {
		return nil, "", errors.New("give view or elements")
	}
	r, err := core.ParseRef(ref)
	if err != nil {
		return nil, "", err
	}
	m, err := s.modelOfRef(project, ref)
	return m, r.View, err
}

// geomDone answers a geometry step: what was touched, not saved, and one link that lights it up.
func (s *mcpServer) geomDone(m *core.Model, view, project string, rep core.GeomReport) (*mcp.CallToolResult, any, error) {
	s.changed(project, m)
	ids := make([]string, len(rep.Touched))
	for i, t := range rep.Touched {
		ids[i] = t[strings.Index(t, "#")+1:]
	}
	link := "/app/#" + view
	if len(ids) > 0 {
		link += "?highlight=" + url.QueryEscape(strings.Join(ids, ","))
	}
	text := fmt.Sprintf("%d changed on %s, not saved; review and Save: %s", len(rep.Touched), view, link)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, map[string]any{"touched": rep.Touched, "saved": false, "link": link}, nil
}

func (s *mcpServer) moveElements(_ context.Context, _ *mcp.CallToolRequest, in geomIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.MoveElements(view, in.Elements, in.DX, in.DY, in.X, in.Y, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) resizeElements(_ context.Context, _ *mcp.CallToolRequest, in geomIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.ResizeElements(view, in.Elements, in.Width, in.Height, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) setZone(_ context.Context, _ *mcp.CallToolRequest, in geomIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.SetZone(view, in.Elements, in.Zone, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) fitZone(_ context.Context, _ *mcp.CallToolRequest, in geomIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.FitZone(view, in.Elements, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) alignElements(_ context.Context, _ *mcp.CallToolRequest, in geomIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.AlignElements(view, in.Elements, in.Mode, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) addZone(_ context.Context, _ *mcp.CallToolRequest, in addZoneIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, nil)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.AddZone(view, core.ZoneSpec{ID: in.ID, Parent: in.Parent, Container: in.Container, StyleID: in.StyleID, Name: in.Name, Lang: in.Lang,
		Rect: core.Rect{X: in.X, Y: in.Y, Width: in.Width, Height: in.Height}}, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) arrangeLike(_ context.Context, _ *mcp.CallToolRequest, in arrangeIn) (*mcp.CallToolResult, any, error) {
	r, err := core.ParseRef(in.Reference)
	if err != nil {
		return nil, nil, err
	}
	if r.ID == "" {
		return nil, nil, errors.New("reference must be view#zone")
	}
	m, err := s.modelOfRef(in.Project, in.Reference)
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.ArrangeLike(r.View, core.ArrangeOptions{Reference: in.Reference, Targets: in.Targets, Parent: in.Parent, DryRun: in.DryRun}, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	var b strings.Builder
	for _, t := range rep.Targets {
		fmt.Fprintf(&b, "%s: %d paired", t.Zone, len(t.Pairs))
		if len(t.Unpaired) > 0 {
			fmt.Fprintf(&b, ", no pair for %s (put in a free row)", strings.Join(t.Unpaired, ", "))
		}
		if len(t.Missing) > 0 {
			fmt.Fprintf(&b, ", nothing to pair with %s", strings.Join(t.Missing, ", "))
		}
		b.WriteString("\n")
	}
	if in.DryRun {
		fmt.Fprintf(&b, "dryRun: nothing written; %d objects would change", len(rep.Touched))
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, rep, nil
	}
	s.changed(in.Project, m)
	ids := make([]string, len(rep.Touched))
	for i, t := range rep.Touched {
		ids[i] = t[strings.Index(t, "#")+1:]
	}
	link := "/app/#" + r.View
	if len(ids) > 0 {
		link += "?highlight=" + url.QueryEscape(strings.Join(ids, ","))
	}
	fmt.Fprintf(&b, "%d changed on %s, not saved; review and Save: %s", len(rep.Touched), r.View, link)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, map[string]any{"targets": rep.Targets, "touched": rep.Touched, "saved": false, "dryRun": false, "link": link}, nil
}

func orInt(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
