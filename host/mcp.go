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
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// settings: the mcp: section of the .semaps file, live (PLAN_20260928-7
	// step 2) — nil in tests that build an mcpServer directly, in which case
	// getGraph falls back to the file's own defaults.
	settings *mcpSettingsBox
}

// mcpSettingsNow: s.settings.Get(), or the plain defaults when s.settings is
// nil (a test-constructed mcpServer, or --workspace with no .semaps file).
func (s *mcpServer) mcpSettingsNow() mcpSettings {
	if s.settings != nil {
		return s.settings.Get()
	}
	return s.proj.Mcp.withDefaults()
}

// Every structured answer is a JSON object: MCP clients (Claude Code among
// them) reject an array in structuredContent, so lists come wrapped —
// {views: [...]}, {relations: [...]}, {runs: [...]}, {reports: [...]}.

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
	Key     string `json:"key" jsonschema:"e_, r_ or v_ id"`
}

type setTextIn struct {
	Project string `json:"project,omitempty"`
	Lang    string `json:"lang"`
	Key     string `json:"key"`
	Field   string `json:"field" jsonschema:"name, title, description, doc, fromLabel or toLabel"`
	Value   string `json:"value"`
}

type addEntityIn struct {
	Project     string `json:"project,omitempty"`
	ID          string `json:"id,omitempty" jsonschema:"e_<name>; default: minted from name"`
	Name        string `json:"name" jsonschema:"the name the entity is shown by; written as its name text in lang (an authored entity has no name in entities.json, CONTRACT §7.1); change it later with set_text field name; the id does not change"`
	Kind        string `json:"kind" jsonschema:"a kind of get_kinds (app, service, component, external, database…); a kind outside it is allowed and flagged by check"`
	Description string `json:"description,omitempty" jsonschema:"written as a text, like set_text"`
	Lang        string `json:"lang,omitempty" jsonschema:"language of name and description; default: the project's first language"`
}

type addRelationIn struct {
	Project string `json:"project,omitempty"`
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type" jsonschema:"a relation type: an id of get_kinds (relationGroups), or a word of your own (the dictionary is open; flagged by check until kinds.json describes it)"`
}

type visibleIn struct {
	Project   string   `json:"project,omitempty"`
	View      string   `json:"view"`
	Relations []string `json:"relations,omitempty" jsonschema:"ids of relations to show or hide on this view, at least one; all or nothing: an unknown id refuses the whole call. Exactly one of relations or types"`
	Types     []string `json:"types,omitempty" jsonschema:"relation type ids, exact (injects, holds.one.internal…): stands for the relations of those types with both ends placed on this view right now, as if you had listed their ids; a one-time action — a relation of the type added to the registry later follows the view's defaults. Exactly one of relations or types"`
	Visible   bool     `json:"visible" jsonschema:"true: show, false: hide; a relation already so is left as it is"`
	List      bool     `json:"list,omitempty" jsonschema:"true: the answer also lists the relations acted on, in full (id, from -> to, type), split into changed and already so; default false"`
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
// getGraphIn: `Depth` is `any` because it is a number 1-5 OR the string "all",
// which has no plain Go type; parseDepthValue is the whole check.
type getGraphIn struct {
	Project string `json:"project,omitempty"`
	Level   string `json:"level,omitempty" jsonschema:"types (drop function/value/method nodes) or all; default types without around, all with around"`
	Lift    string `json:"lift,omitempty" jsonschema:"types (fold method-level edges up to their containing types, ADR_20260928-3 §6) or none; default types unless around is a method, or (without around) unless level=all"`
	// Kinds is only for a whole-graph request (no Around): a neighbourhood
	// names Follow instead (part 1 of the agent-answers rework).
	Kinds     string   `json:"kinds,omitempty" jsonschema:"whole-graph only (no around): comma list of edge kinds to keep; default: all"`
	Around    string   `json:"around,omitempty" jsonschema:"a node id or a name (part 2: resolved by id, then name, case-insensitively as a last resort) — only its neighbourhood"`
	Follow    []string `json:"follow,omitempty" jsonschema:"neighbourhood only: directed relation names to walk (call graph_formats for the vocabulary); default: every relation but containment"`
	Depth     any      `json:"depth,omitempty" jsonschema:"with around: a number 1-5 (default 1), or the string \"all\" to walk until no new node is reached — only when every relation in follow is an inheritance relation (extends, extended-by, implements, implemented-by)"`
	Fanout    int      `json:"fanout,omitempty" jsonschema:"with around: at most this many neighbours per node per relation; 0 (default) = unlimited"`
	Container string   `json:"container,omitempty" jsonschema:"a container node (id or name; its kind is a container kind of get_kinds): only it and the nodes inside it, at any depth"`
	// Fields is a list, not a comma string, so null/absent (the default,
	// via+position) can be told apart from an explicit empty list (nothing
	// extra): a plain string can't carry that distinction over JSON.
	Fields   []string `json:"fields,omitempty" jsonschema:"members, via, position, memberLines, dynamic; default (omitted/null) via,position,dynamic; [] for none"`
	Missing  bool     `json:"missing,omitempty" jsonschema:"include model-only nodes/edges whose entity/relation has status missing; default false"`
	Limit    int      `json:"limit,omitempty" jsonschema:"without around or container, cut to this many nodes; default: the .semaps mcp.limit setting (200 unless changed)"`
	ListCap  int      `json:"listCap,omitempty" jsonschema:"caps the names/lines printed for one relation ({fromMethods},{toMethods},{relationLines}) before '+N'; default: the .semaps mcp.list_cap setting (50 unless changed)"`
	Format   string   `json:"format,omitempty" jsonschema:"answer format name, or call graph_formats to see them; default: the .semaps mcp.format setting (facts unless changed)"`
	Template string   `json:"template,omitempty" jsonschema:"a template of your own (see graph_formats for the macro dictionary and rules), instead of a named format's"`
}

type findNodeIn struct {
	Project string `json:"project,omitempty"`
	Q       string `json:"q" jsonschema:"substring of a name or id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 50"`
}

type getViewIn struct {
	Project string `json:"project,omitempty"`
	View    string `json:"view" jsonschema:"a view id, or a container reference view#e_x to read only that subtree"`
	Detail  string `json:"detail,omitempty" jsonschema:"full: every placement nested in its container, and the visible lines; tree: containers only, each with its rectangle and blocks (the count of blocks lying directly in it), the blocks outside any container in full, and lines (a count) instead of the list. Default: full for a small view or subtree, tree for a big one — the answer says when it chose tree"`
	Hidden  bool   `json:"hidden,omitempty" jsonschema:"also list, as hiddenEdges, the relations with both ends placed that the rule hides; default false"`
}

type kindsIn struct {
	Project string `json:"project,omitempty" jsonschema:"the project whose entity kinds and relation types outside the dictionary are listed under unknown"`
	Lang    string `json:"lang,omitempty" jsonschema:"language of names and descriptions; default ru, else any present"`
}

type geomIn struct {
	Project          string   `json:"project,omitempty"`
	View             string   `json:"view,omitempty" jsonschema:"view id; may be left out when elements are references view#e_x"`
	Elements         []string `json:"elements" jsonschema:"entity ids of placements (blocks or containers), or references view#e_x"`
	DX               *float64 `json:"dx,omitempty"`
	DY               *float64 `json:"dy,omitempty"`
	X                *float64 `json:"x,omitempty" jsonschema:"move: the top-left corner of the common box of the elements goes here"`
	Y                *float64 `json:"y,omitempty"`
	Width            *float64 `json:"width,omitempty"`
	Height           *float64 `json:"height,omitempty"`
	Mode             string   `json:"mode,omitempty" jsonschema:"align: left, right, top, bottom, width or height, to the first element"`
	RequestedByHuman bool     `json:"requestedByHuman" jsonschema:"true only when a human asked for this layout in so many words"`
}

type setParentIn struct {
	Project          string   `json:"project,omitempty"`
	View             string   `json:"view,omitempty" jsonschema:"view id; may be left out when elements are references view#e_x"`
	Elements         []string `json:"elements" jsonschema:"entity ids of placements, or references view#e_x"`
	Parent           *string  `json:"parent" jsonschema:"entity id of a container placement of the view; null takes them out of any"`
	RequestedByHuman bool     `json:"requestedByHuman" jsonschema:"true only when a human asked for this layout in so many words"`
}

// setPlacementIn: only the fields present in the call change; null drops a field
// (which of them are present is read from the raw arguments, since a pointer
// cannot tell absent from null).
type setPlacementIn struct {
	Project          string          `json:"project,omitempty"`
	View             string          `json:"view,omitempty" jsonschema:"view id; may be left out when elements are references view#e_x"`
	Elements         []string        `json:"elements" jsonschema:"entity ids of placements (blocks or containers), or references view#e_x"`
	StyleID          *string         `json:"styleId,omitempty" jsonschema:"the placement's style; null drops it (the style of the entity's kind applies)"`
	Override         *map[string]any `json:"override,omitempty" jsonschema:"a partial style of these placements: fill, border {color, dash}, header {fill}, icon {glyph}; null drops it"`
	Template         *string         `json:"template,omitempty" jsonschema:"the content template of these placements; null drops it"`
	Collapsed        *bool           `json:"collapsed,omitempty" jsonschema:"containers only: collapsed to the header; null drops it"`
	RequestedByHuman bool            `json:"requestedByHuman" jsonschema:"true only when a human asked for this in so many words"`
}

type setRoutingIn struct {
	Project          string   `json:"project,omitempty"`
	View             string   `json:"view" jsonschema:"view id"`
	Routing          *string  `json:"routing" jsonschema:"bezier, orthogonal, tree-horizontal or tree-vertical; null removes the choice"`
	Relations        []string `json:"relations,omitempty" jsonschema:"relation ids: set the shape of only these lines on this view; without relations and types the whole view's own routing is set"`
	Types            []string `json:"types,omitempty" jsonschema:"relation type ids, exact (injects, extends…): stands for the lines of those types with both ends placed on this view right now, as if you had listed their ids; a one-time action; not together with relations"`
	List             bool     `json:"list,omitempty" jsonschema:"true: the answer also lists the relations whose entry was written, in full (id, from -> to, type); default false"`
	RequestedByHuman bool     `json:"requestedByHuman" jsonschema:"true only when a human asked for this in so many words"`
}

type fitContainerIn struct {
	Project          string `json:"project,omitempty"`
	View             string `json:"view,omitempty" jsonschema:"view id; may be left out when container is a reference view#e_x"`
	Container        string `json:"container" jsonschema:"entity id of a container placement, or a reference view#e_x"`
	RequestedByHuman bool   `json:"requestedByHuman" jsonschema:"true only when a human asked for this layout in so many words"`
}

type addContainerIn struct {
	Project          string  `json:"project,omitempty"`
	View             string  `json:"view"`
	Entity           string  `json:"entity" jsonschema:"the container's entity id: e_ then lowercase letters, digits, underscore. An existing entity of a container kind is placed as it is; one that does not exist is created with this id, from name and kind. The id is never made from a name"`
	Name             string  `json:"name,omitempty" jsonschema:"only for an entity that does not exist yet (then required): its name, the container's caption, written as a text in the project's first language. Refused for an existing entity: change a name with set_text"`
	Kind             string  `json:"kind,omitempty" jsonschema:"kind of a new entity: a container kind of get_kinds; default group. For an existing entity it may only repeat its own kind"`
	Parent           string  `json:"parent,omitempty" jsonschema:"entity id of the container placement it goes into; empty for none"`
	StyleID          string  `json:"styleId,omitempty"`
	X                float64 `json:"x"`
	Y                float64 `json:"y"`
	Width            float64 `json:"width,omitempty" jsonschema:"required unless contents is true; with contents, a minimum"`
	Height           float64 `json:"height,omitempty" jsonschema:"required unless contents is true; with contents, a minimum"`
	Contents         bool    `json:"contents,omitempty" jsonschema:"also place what the container directly contains (its contains relations), one level, in a grid; the container is sized to hold it. A container already on the view gets only its missing members"`
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

// read/write build a *mcp.Tool with its annotation: a reading tool says so
// (readOnlyHint), so a client may run it without asking, and the editor's
// sandbox asks before any other. Package-level (not closures inside
// server()) so registerGraphTools, called both from server() and from a
// settings-change rebuild, can build tools the same way.
func read(name, desc string) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
}

var notDestructive = false

func write(name, desc string) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive}}
}

func (s *mcpServer) server() *mcp.Server {
	// settings are read once here, before mcp.NewServer, since Instructions
	// is fixed for the life of THIS *mcp.Server (the Go SDK gives no way to
	// change it on a live one): a later PUT /api/setup change of
	// mcp.tools/mcp.description leaves ITS Instructions as they were.
	// registerMCPHTTP (host/mcp_http.go, mcpServerPool) is what makes the
	// change reach a new session at once regardless: it calls server() again
	// on a settings change and hands the fresh *mcp.Server, with the new
	// Instructions baked in, to every session that connects from then on,
	// while the servers still-open sessions are on get their tool lists
	// rebuilt in place (docs/API.md §6, PLAN_20260928-7 step 4).
	settings := s.mcpSettingsNow()
	srv := mcp.NewServer(&mcp.Implementation{Name: "semaps", Version: "1"}, &mcp.ServerOptions{
		Instructions: serverPreamble + serverInstructions(settings.Tools, settings.Description),
	})
	// A call with arguments that do not fit says which parameters the tool takes (mcp_errors.go).
	srv.AddReceivingMiddleware(explainArgumentErrors)
	// Reading tools say so (readOnlyHint): a client may run them without asking,
	// and the editor's sandbox asks before any other.
	mcp.AddTool(srv, read("list_projects", "Projects of the workspace and their views."), s.listProjects)
	mcp.AddTool(srv, read("list_views", "Views of one project."), s.listViews)
	mcp.AddTool(srv, read("get_entity", "One entity by id or by symbol."), s.getEntity)
	mcp.AddTool(srv, read("find_entities", "Entities by name/symbol/namespace substring, kind, status."), s.findEntities)
	mcp.AddTool(srv, read("get_relations", "Relations of an entity (or all), by direction, type, status."), s.getRelations)
	mcp.AddTool(srv, read("get_kinds", "The dictionary (kinds.json): groups of entity kinds with names, descriptions, whether a kind is a container, its base style; groups of relation types with names, descriptions, base style, default visibility; unknown lists the project's kinds and relation types outside it."), s.getKinds)
	mcp.AddTool(srv, read("get_text", "Text entry of a key in one language."), s.getText)
	mcp.AddTool(srv, read("get_view", getViewDescription), s.getView)
	mcp.AddTool(srv, read("doctor", "Extractors and runtimes found, and the model check of the workspace."), s.doctor)
	mcp.AddTool(srv, read("sync_preview", "What sync would change, writing nothing (= semaps sync --dry-run)."), s.syncPreview)
	s.registerGraphTools(srv, settings.Tools, settings.Description)
	// The view tools carry description levels like the graph tools do
	// (mcp_view.go); they are project-independent (layout_guide) or take the
	// project themselves.
	level := settings.Description
	mcp.AddTool(srv, read("layout_guide", viewToolDescriptions["layout_guide"].at(level)), s.layoutGuide)
	mcp.AddTool(srv, read("render_view", viewToolDescriptions["render_view"].at(level)), s.renderView)

	mcp.AddTool(srv, write("set_text", "Write one text field as authored, with at = now."), s.setText)
	mcp.AddTool(srv, write("add_entity", "Add an authored entity: a part of the system no extractor reports (a kind of get_kinds: app, external, database…); the id is minted unless given."), s.addEntity)
	mcp.AddTool(srv, write("add_relation", "Add an authored relation; the id is minted and returned."), s.addRelation)
	mcp.AddTool(srv, write("set_relation_visible", "Show or hide lines on one view (relations.except), one batch, all or nothing: exactly one of relations (ids) or types (relation type ids: stands for the relations of those types with both ends placed on the view right now; a one-time action, relations of the type added later follow the view's defaults). Call it once with everything, not once per relation. `list: true` also returns which relations were changed."), s.setRelationVisible)
	mcp.AddTool(srv, write("confirm_rename", "Answer a sync rename candidate: entity + symbol, or relation + member."), s.confirmRename)
	mcp.AddTool(srv, write("extract", "Run the extractors of the .semaps file; returns run ids."), s.extract)
	mcp.AddTool(srv, write("sync", "Reconcile the registry with the code (extracts first unless run is given)."), s.sync)
	mcp.AddTool(srv, write("place_entities", "Put entities on a view. Only when a human asked for it: requestedByHuman."), s.placeEntities)
	mcp.AddTool(srv, write("move_elements", "Move placements by dx/dy or to x/y; a container goes with everything inside it. Only when a human asked: requestedByHuman."), s.moveElements)
	mcp.AddTool(srv, write("resize_elements", "Set width/height of placements, not below the minimum nor, for a container, below its content. requestedByHuman."), s.resizeElements)
	mcp.AddTool(srv, write("set_parent", "Put placements into a container (parent: its entity id), or out of any (parent: null); coordinates untouched. requestedByHuman."), s.setParent)
	mcp.AddTool(srv, write("set_placement", "Change the look of placements already on a view: styleId, override (fill, border, header fill, icon), template, collapsed (containers only); only the fields you give change, null drops one; geometry and parent untouched. requestedByHuman."), s.setPlacement)
	mcp.AddTool(srv, write("set_routing", "Set the shape of lines on a view: routing bezier, orthogonal, tree-horizontal or tree-vertical, null to remove the choice. Without relations and types it is the view's own routing; with relations (ids) only those lines' own routing on this view; types (relation type ids) stands for the lines of those types on this view right now, one-time, as if you had listed their ids. `list: true` also returns which relations were written. The routed path is never stored. requestedByHuman."), s.setRouting)
	mcp.AddTool(srv, write("add_container", "Place a container on a view by its entity id (required, e_<name>): an existing entity of a container kind is placed as it is (no name, no other kind); an id that does not exist yet creates an authored entity with that id, name (required) and kind (a container kind, default group) — entity and placement in one step, with a rectangle, optional parent and style. The id is never made from a name. "+
		"With contents: true it also places what the container directly contains — its `contains` relations, one level, members that are missing left out — in a plain grid (default block size and gap, on the grid step, by name), sized to hold them; width and height are then only a minimum. "+
		"A member of a container kind comes as an empty frame (call again for it); a member already on the view is not moved and is named in the answer; a container already on the view gets only the members it lacks, under its lowest child. "+
		"Membership is a fact of the registry, the grid only a starting arrangement to be adjusted. One step: it applies whole or not at all. requestedByHuman."), s.addContainer)
	mcp.AddTool(srv, write("fit_container", "Fit a container to its content (caption strip and padding); ancestors grow if they no longer hold it. requestedByHuman."), s.fitContainer)
	mcp.AddTool(srv, write("align_elements", "Align elements to the first one: left, right, top, bottom, width, height. requestedByHuman."), s.alignElements)
	mcp.AddTool(srv, write("create_view", viewToolDescriptions["create_view"].at(level)), s.createView)
	mcp.AddTool(srv, write("create_project", viewToolDescriptions["create_project"].at(level)), s.createProject)
	mcp.AddTool(srv, write("save", "Save all unsaved project changes only when a human explicitly requested it."), s.save)
	mcp.AddTool(srv, write("discard", "Discard unsaved changes only when a human explicitly requested it."), s.discard)
	return srv
}

// narrowIn: input of every tool of the `narrow` set (PLAN_20260928-7 step
// 4): one required node name (or id), an optional depth, and the usual
// optional project.
type narrowIn struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
	Depth   int    `json:"depth,omitempty"`
}

// graphToolNames: every tool name either set can register, so
// rebuildGraphTools can remove them all before adding back the ones the new
// settings ask for — RemoveTools on a name the server does not have is a
// no-op.
func graphToolNames() []string {
	names := []string{"get_graph", "find_node", "graph_formats"}
	for name := range narrowTools {
		names = append(names, name)
	}
	return names
}

// registerGraphTools adds the graph tools of one set (`one` or `narrow`) at
// one description level to srv. Used both by server() (the set/level the
// running settings hold right now) and by getMCP's six-combination sizing
// (host/api_mcp.go), which builds a tool set for each combination without
// touching the live settings.
func (s *mcpServer) registerGraphTools(srv *mcp.Server, toolsSet, level string) {
	if toolsSet == "narrow" {
		for name, t := range narrowTools {
			name, t := name, t // capture for the handler closure
			d := narrowDescriptions(name, t)
			mcp.AddTool(srv, read(name, d.at(level)), func(ctx context.Context, req *mcp.CallToolRequest, in narrowIn) (*mcp.CallToolResult, any, error) {
				return s.narrowWalk(ctx, req, in, t.Follow)
			})
		}
		mcp.AddTool(srv, read("find_node", graphToolDescriptions["find_node"].at(level)), s.findNode)
		return
	}
	mcp.AddTool(srv, read("get_graph", graphToolDescriptions["get_graph"].at(level)), s.getGraph)
	mcp.AddTool(srv, read("graph_formats", graphToolDescriptions["graph_formats"].at(level)), s.graphFormats)
	mcp.AddTool(srv, read("find_node", graphToolDescriptions["find_node"].at(level)), s.findNode)
}

// rebuildGraphTools drops every graph tool srv may have and adds back the
// set/level the live settings hold now, notifying connected clients of the
// list change (mcp.Server.RemoveTools/AddTool both call changeAndNotify,
// which sends notifications/tools/list_changed to every session — the
// mechanism the Go SDK gives for changing a live server's tool list; PLAN
// step 4). Registered as settings.onChange by registerMCPHTTP, so PUT
// /api/setup reaches connected MCP clients as soon as it reaches the
// settings box.
func (s *mcpServer) rebuildGraphTools(srv *mcp.Server) func(mcpSettings) {
	return func(v mcpSettings) {
		srv.RemoveTools(graphToolNames()...)
		s.registerGraphTools(srv, v.Tools, v.Description)
	}
}

// narrowWalk is the one shared implementation behind every `narrow` tool
// (step 4: "thin wrappers over ONE shared function that get_graph uses
// too"): it is get_graph itself, called with `around` set to the node name
// and `follow` fixed to the tool's relations — the identical graph walk,
// same defaults, same answer shape.
// parseDepthValue reads get_graph's `depth`: absent or 0 is 1, a whole number
// 1-5 (a JSON number, or the same in a string), or "all".
func parseDepthValue(v any) (int, error) {
	switch d := v.(type) {
	case nil:
		return 1, nil
	case string:
		return parseDepth(strings.TrimSpace(d))
	case float64:
		if d != math.Trunc(d) {
			return 0, errors.New(`depth: not a whole number (1-5, or "all" along inheritance relations)`)
		}
		if d == 0 {
			return 1, nil
		}
		return parseDepth(strconv.Itoa(int(d)))
	case int:
		if d == 0 {
			return 1, nil
		}
		return parseDepth(strconv.Itoa(d))
	default:
		return 0, errors.New(`depth: a number 1-5, or "all" along inheritance relations`)
	}
}

func (s *mcpServer) narrowWalk(ctx context.Context, req *mcp.CallToolRequest, in narrowIn, follow []string) (*mcp.CallToolResult, any, error) {
	return s.getGraph(ctx, req, getGraphIn{Project: in.Project, Around: in.Name, Depth: in.Depth, Follow: follow})
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
	return nil, s.service().index(), nil
}

func (s *mcpServer) listViews(_ context.Context, _ *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, any, error) {
	dir, err := core.ProjectDir(s.workspace, s.pick(in.Project))
	if err != nil {
		return nil, nil, err
	}
	for _, p := range s.service().index().Projects {
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
		m, err := core.LoadModel(s.workspace, s.pick(project), defaultKinds())
		if err != nil {
			return nil, err
		}
		m.SetCanvas(loadCanvas(s.workspace))
		return m, nil
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
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	ents, err := s.records(in.Project, "entities.json")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range ents {
		if (in.ID != "" && e.str("id") == in.ID) || (in.Symbol != "" && slices.Contains(e.symbols(), in.Symbol)) {
			return nil, entityAnswer(e, m.EntityName(e.str("id"))), nil
		}
	}
	return nil, nil, fmt.Errorf("no entity %s%s", in.ID, in.Symbol)
}

// entityAnswer is an entity as an agent reads it: its record of entities.json
// and, for an authored entity — which has no name there, its name is a text
// (CONTRACT §7.1) — the name it is shown by: the text in the main language,
// else in another, else its id.
func entityAnswer(e record, name string) any {
	if e.str("origin") != "authored" {
		return e.raw
	}
	out := map[string]any{}
	for k, v := range e.fields {
		out[k] = v
	}
	out["name"] = name
	return out
}

// symbols are the symbols of the entity's realizations (code[]).
func (r record) symbols() []string {
	var out []string
	list, _ := r.fields["code"].([]any)
	for _, c := range list {
		if o, ok := c.(map[string]any); ok {
			if s, _ := o["symbol"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func (s *mcpServer) findEntities(_ context.Context, _ *mcp.CallToolRequest, in findIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	ents, err := s.records(in.Project, "entities.json")
	if err != nil {
		return nil, nil, err
	}
	names := m.EntityNames()
	q := strings.ToLower(in.Query)
	limit := orInt(in.Limit, 50)
	var out []any
	total := 0
	for _, e := range ents {
		if in.Kind != "" && e.str("kind") != in.Kind {
			continue
		}
		if in.Status != "" && orStr(e.str("status"), "present") != in.Status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(names[e.str("id")]+"\x00"+strings.Join(e.symbols(), "\x00")+"\x00"+e.str("namespace")+"\x00"+e.str("id")), q) {
			continue
		}
		total++
		if len(out) < limit {
			out = append(out, entityAnswer(e, names[e.str("id")]))
		}
	}
	if out == nil {
		out = []any{}
	}
	return nil, map[string]any{"total": total, "entities": out}, nil
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

// kindOut and friends are the dictionary of get_kinds, its texts in one language.
type kindOut struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Container   bool   `json:"container"`
	Style       string `json:"style,omitempty"`
}

type kindGroupOut struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Kinds       []kindOut `json:"kinds"`
}

type relationTypeOut struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Style       string `json:"style,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

type relationGroupOut struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Types       []relationTypeOut `json:"types"`
}

// dictionaryOut is the merged dictionary in one language: get_kinds and
// GET /api/kinds answer with it.
func dictionaryOut(catalog *core.KindCatalog, lang string) map[string]any {
	groups := []kindGroupOut{}
	for _, g := range catalog.Groups {
		out := kindGroupOut{ID: g.ID, Name: core.Localized(g.Name, lang), Description: core.Localized(g.Description, lang), Kinds: []kindOut{}}
		for _, k := range g.Kinds {
			out.Kinds = append(out.Kinds, kindOut{ID: k.ID, Name: core.Localized(k.Name, lang), Description: core.Localized(k.Description, lang), Container: k.Container, Style: k.Style})
		}
		groups = append(groups, out)
	}
	relations := []relationGroupOut{}
	for _, g := range catalog.RelationGroups {
		out := relationGroupOut{ID: g.ID, Name: core.Localized(g.Name, lang), Description: core.Localized(g.Description, lang), Types: []relationTypeOut{}}
		for _, t := range g.Types {
			out.Types = append(out.Types, relationTypeOut{ID: t.ID, Name: core.Localized(t.Name, lang), Description: core.Localized(t.Description, lang), Style: t.Style, Visibility: t.Visibility})
		}
		relations = append(relations, out)
	}
	return map[string]any{"groups": groups, "relationGroups": relations}
}

// getKinds answers the merged dictionary (the tool's default and the
// workspace's kinds.json) and what the project's entities and relations use
// that it lacks.
func (s *mcpServer) getKinds(_ context.Context, _ *mcp.CallToolRequest, in kindsIn) (*mcp.CallToolResult, any, error) {
	catalog, err := core.LoadKinds(s.workspace, defaultKinds())
	if err != nil {
		return nil, nil, err
	}
	out := dictionaryOut(catalog, orStr(in.Lang, "ru"))
	unknownKinds, unknownTypes := []string{}, []string{}
	ents, err := s.records(in.Project, "entities.json")
	if err != nil && in.Project != "" {
		return nil, nil, err
	}
	seen := map[string]bool{}
	for _, e := range ents {
		k := e.str("kind")
		if _, ok := catalog.Lookup(k); !ok && k != "" && !seen[k] {
			seen[k] = true
			unknownKinds = append(unknownKinds, k)
		}
	}
	rels, _ := s.records(in.Project, "relations.json")
	for _, r := range rels {
		id := r.str("type")
		if _, ok := catalog.LookupRelation(id); !ok && id != "" && !seen["rt:"+id] {
			seen["rt:"+id] = true
			unknownTypes = append(unknownTypes, id)
		}
	}
	slices.Sort(unknownKinds)
	slices.Sort(unknownTypes)
	out["unknown"] = map[string]any{"kinds": unknownKinds, "relationTypes": unknownTypes}
	return nil, out, nil
}

// getGraph is get_graph (docs/plans/PLAN_20260928_host_graph-provider.md step
// 6): the same filters as GET /api/graph/{project} (host/graph.go), reusing
// the same core filter functions, plus a `limit` on the node count so an
// agent asking for a whole project's graph without `around` or `container`
// gets a cut answer with `truncated: true` and the full counts, not
// everything at once.
func (s *mcpServer) getGraph(_ context.Context, _ *mcp.CallToolRequest, in getGraphIn) (*mcp.CallToolResult, any, error) {
	settings := s.mcpSettingsNow()
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
		formatName = settings.Format
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

	var containerID string
	if in.Container != "" {
		var problem *nodeProblem
		if containerID, problem = resolveContainer(graph, in.Container, in.Missing); problem != nil {
			if problem.Status == 409 {
				return nil, problem.Body, nil
			}
			return nil, nil, errors.New(problem.Message)
		}
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
	var liftHint string
	if around != "" {
		follow, err := core.ParseFollow(in.Follow)
		if err != nil {
			return nil, nil, err
		}
		depth, err := parseDepthValue(in.Depth)
		if err == nil {
			err = checkWalkAll(depth, follow, len(in.Follow) > 0)
		}
		if err != nil {
			return nil, nil, err
		}
		preWalk := graph
		if graph, fanoutNotes, err = core.Walk(graph, around, depth, follow, in.Fanout); err != nil {
			return nil, nil, err
		}
		bounded = true
		if lift == "none" {
			liftHint = core.LiftNoneHint(preWalk, graph, around, follow)
		}
	} else if in.Kinds != "" {
		graph = core.FilterEdgeKinds(graph, splitCSV(in.Kinds))
	}
	if containerID != "" {
		if graph, err = core.FilterContainer(graph, containerID); err != nil {
			return nil, nil, err
		}
		bounded = true
	}

	limit := in.Limit
	if limit <= 0 {
		limit = settings.Limit
	}
	var truncated bool
	var fullNodes, fullEdges int
	graph, truncated, fullNodes, fullEdges = truncateGraph(graph, bounded, limit)

	listCap := in.ListCap
	if listCap <= 0 {
		listCap = settings.ListCap
	}

	stats := graphStats(graph, hiddenNodes, hiddenEdges)
	opts := core.FormatOptions{
		Focus: around, Facts: facts, Stats: stats, Fields: fields,
		Truncated: truncated, FullNodes: fullNodes, FullEdges: fullEdges,
		FanoutNotes: fanoutNotes, Notice: notice, ListCap: listCap, LiftHint: liftHint,
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
		"findings":     core.Check(s.workspace, s.sourceRoot, defaultKinds()),
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

func (s *mcpServer) addEntity(_ context.Context, _ *mcp.CallToolRequest, in addEntityIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	lang := orStr(in.Lang, m.Languages()[0])
	id, err := m.AddEntity(in.ID, in.Name, in.Kind, lang, "agent")
	if err != nil {
		return nil, nil, err
	}
	if in.Description != "" {
		if err := m.SetText(lang, id, "description", in.Description, "agent"); err != nil {
			s.changed(in.Project, m)
			return nil, nil, fmt.Errorf("%s added, its description not: %w", id, err)
		}
	}
	s.changed(in.Project, m)
	return done("%s added, not saved; place it with place_entities; review and Save: %s", id, s.reviewLink(m, id))
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

func (s *mcpServer) setRelationVisible(_ context.Context, _ *mcp.CallToolRequest, in visibleIn) (*mcp.CallToolResult, any, error) {
	m, err := s.model(in.Project)
	if err != nil {
		return nil, nil, err
	}
	if (len(in.Relations) == 0) == (len(in.Types) == 0) {
		return nil, nil, errors.New("give exactly one of relations (ids) or types (relation type ids)")
	}
	ids, of := in.Relations, ""
	if len(in.Types) > 0 {
		// the selector is a one-time expansion: what is on the view now, then the same write
		if ids, err = m.RelationsOfTypes(in.View, in.Types); err != nil {
			return nil, nil, err
		}
		of = " of type " + strings.Join(in.Types, ", ")
		if len(ids) == 0 {
			return listedAnswer(in.List, fmt.Sprintf("no relation%s has both ends placed on %s: nothing changed (a type is matched exactly; get_relations shows the types in use)", of, in.View),
				map[string][]core.RelationRef{"changed": {}, "already": {}})
		}
	}
	res, err := m.SetRelationsVisible(in.View, ids, in.Visible, "agent")
	if err != nil {
		return nil, nil, err
	}
	lists := map[string][]core.RelationRef{"changed": m.RelationRefs(res.Changed), "already": m.RelationRefs(res.Already)}
	if len(res.Changed) == 0 {
		return listedAnswer(in.List, fmt.Sprintf("%d relation(s)%s on %s: nothing to change, all already visible=%v%s", len(res.Already), of, in.View, in.Visible, unplacedNote(res.Unplaced)), lists)
	}
	s.changed(in.Project, m)
	tail := ""
	if of != "" {
		tail = "; relations of these types added to the registry later follow the view's defaults"
	}
	return listedAnswer(in.List, fmt.Sprintf("%d relation(s)%s on %s now visible=%v, %d already were so%s%s; not saved; review and Save: %s",
		len(res.Changed), of, in.View, in.Visible, len(res.Already), unplacedNote(res.Unplaced), tail, s.reviewLink(m, in.View)), lists)
}

// listedAnswer is the text of a tool answer; with list it also carries the
// relations the call acted on, in full, one per line `<id>  <from> -> <to>  <type>`
// under a heading per group, and as structured content. Without list it is the
// text alone.
func listedAnswer(list bool, text string, groups map[string][]core.RelationRef) (*mcp.CallToolResult, any, error) {
	if !list {
		return done("%s", text)
	}
	var b strings.Builder
	b.WriteString(text)
	for _, name := range []string{"changed", "already", "written"} {
		refs, ok := groups[name]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n%s (%d):", name, len(refs))
		for _, r := range refs {
			fmt.Fprintf(&b, "\n%s  %s -> %s  %s", r.ID, r.From, r.To, r.Type)
		}
	}
	structured := map[string]any{}
	for k, v := range groups {
		structured[k] = v
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, structured, nil
}

// unplacedNote names the relations of a set_relation_visible call that the view
// does not draw anyway: an end is not placed on it. The setting is accepted.
func unplacedNote(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return "; not drawn on this view anyway because an end is not placed (setting accepted): " + nameIDs(ids, 5)
}

// nameIDs lists at most max ids and counts the rest, so a long list does not
// fill an answer.
func nameIDs(ids []string, max int) string {
	if len(ids) <= max {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ids[:max], ", "), len(ids)-max)
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
	info, err := m.GetViewWith(in.View, core.ViewOptions{Detail: in.Detail, FullMax: DefaultMcpViewFullMax, Hidden: in.Hidden})
	if err != nil {
		return nil, nil, err
	}
	// Every answer starts with the canvas — units, grid, sizes, gaps — so the
	// numbers below are read against it; the data follows as JSON. The answer is
	// given once, as text: no structured copy of the same data.
	cv, err := m.Canvas()
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(info)
	if err != nil {
		return nil, nil, err
	}
	head := cv.Describe()
	if info.FellBack {
		// the first line says the answer was cut and how to see more, as get_graph's does
		r, _ := core.ParseRef(in.View)
		what := "this view has"
		if info.View.Scope != "" {
			what = info.View.Scope + " holds"
		}
		head = fmt.Sprintf("Shown as a tree: %s %d placements (more than %d), so only containers are listed, each with the count of its blocks, and the lines only as a count; "+
			"read one container in full with a reference (`%s#e_x`), or everything with detail \"full\".\n", what, info.Scoped, DefaultMcpViewFullMax, r.View) + head
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: head}, &mcp.TextContent{Text: string(body)}}}, nil, nil
}

// getViewDescription: what get_view answers. The size that decides the default
// detail is the one constant, DefaultMcpViewFullMax.
var getViewDescription = fmt.Sprintf("A view with geometry: placements, containers with what lies in them (children), absolute rectangles, the visible lines (relations of the registry, with the view's own styleId, override, routing when it has them), what is unsaved; the view's own rule for lines is in view.relations, its shape of lines in view.routing. "+
	"`detail`: full (every placement nested in its container, each visible line) or tree (only containers, nested, each with its rectangle and `blocks`, the number of blocks lying directly in it; blocks outside any container in full; `lines`, a count, instead of the list). "+
	"Default: full when the view or subtree has at most %d placements, else tree — and then the first line of the answer says so and how to see more. "+
	"A container reference view#e_x reads only that subtree, in either detail. `hidden: true` adds hiddenEdges: the relations with both ends placed that the rule hides. "+
	"The answer starts with the canvas block: units, grid, default and minimum sizes, caption strip, padding, gaps; the data follows once, as JSON.", DefaultMcpViewFullMax)

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

func (s *mcpServer) setParent(_ context.Context, _ *mcp.CallToolRequest, in setParentIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	parent := ""
	if in.Parent != nil {
		parent = *in.Parent
	}
	rep, err := m.SetParent(view, in.Elements, parent, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) setPlacement(_ context.Context, req *mcp.CallToolRequest, in setPlacementIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, in.Elements)
	if err != nil {
		return nil, nil, err
	}
	// a field is set when its key is in the call, null included (which drops it)
	given := map[string]json.RawMessage{}
	if req != nil && req.Params != nil {
		_ = json.Unmarshal(req.Params.Arguments, &given)
	}
	fields := map[string]json.RawMessage{}
	for _, key := range core.PlacementLookFields {
		if raw, ok := given[key]; ok {
			fields[key] = raw
		}
	}
	rep, err := m.SetPlacement(view, in.Elements, fields, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	return s.geomDone(m, view, in.Project, rep)
}

func (s *mcpServer) setRouting(_ context.Context, _ *mcp.CallToolRequest, in setRoutingIn) (*mcp.CallToolResult, any, error) {
	if in.View == "" {
		return nil, nil, errors.New("give view")
	}
	m, view, err := s.geomTarget(in.Project, in.View, nil)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Relations) > 0 && len(in.Types) > 0 {
		return nil, nil, errors.New("give relations or types, not both: one target per call (neither: the view itself)")
	}
	relations, of := in.Relations, ""
	if len(in.Types) > 0 {
		// the selector is a one-time expansion: the lines on the view now, then the per-line write
		if in.RequestedByHuman {
			if relations, err = m.RelationsOfTypes(view, in.Types); err != nil {
				return nil, nil, err
			}
			of = " of type " + strings.Join(in.Types, ", ")
			if len(relations) == 0 {
				return listedAnswer(in.List, fmt.Sprintf("no relation%s has both ends placed on %s: nothing changed (a type is matched exactly; get_relations shows the types in use)", of, view),
					map[string][]core.RelationRef{"written": {}})
			}
		}
	}
	rep, err := m.SetRouting(view, in.Routing, relations, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	s.changed(in.Project, m)
	link := "/app/#" + view
	what := "the view's routing"
	if len(relations) > 0 {
		link += "?highlight=" + url.QueryEscape(strings.Join(rep.Relations, ","))
		what = fmt.Sprintf("the routing of %d line(s)%s", len(rep.Relations), of)
	}
	value := "removed"
	if in.Routing != nil {
		value = "set to " + *in.Routing
	}
	text := fmt.Sprintf("%s %s on %s, not saved; review and Save: %s", what, value, view, link)
	if of != "" {
		text += ". A one-time action: relations of these types added to the registry later follow the view's defaults"
	}
	if len(rep.NotDrawn) > 0 {
		text += fmt.Sprintf(". Accepted, but not drawn on this view now (an end is not placed, or the line is hidden): %s", nameIDs(rep.NotDrawn, 5))
	}
	structured := map[string]any{"relations": rep.Relations, "notDrawn": rep.NotDrawn, "saved": false, "link": link}
	if in.List {
		written := m.RelationRefs(rep.Relations)
		var b strings.Builder
		fmt.Fprintf(&b, "%s\nwritten (%d):", text, len(written))
		for _, r := range written {
			fmt.Fprintf(&b, "\n%s  %s -> %s  %s", r.ID, r.From, r.To, r.Type)
		}
		text = b.String()
		structured["written"] = written
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, structured, nil
}

func (s *mcpServer) fitContainer(_ context.Context, _ *mcp.CallToolRequest, in fitContainerIn) (*mcp.CallToolResult, any, error) {
	if in.Container == "" {
		return nil, nil, errors.New("give container")
	}
	m, view, err := s.geomTarget(in.Project, in.View, []string{in.Container})
	if err != nil {
		return nil, nil, err
	}
	rep, err := m.FitContainer(view, []string{in.Container}, in.RequestedByHuman, "agent")
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

func (s *mcpServer) addContainer(_ context.Context, _ *mcp.CallToolRequest, in addContainerIn) (*mcp.CallToolResult, any, error) {
	m, view, err := s.geomTarget(in.Project, in.View, nil)
	if err != nil {
		return nil, nil, err
	}
	if !in.Contents && (in.Width <= 0 || in.Height <= 0) {
		return nil, nil, errors.New("give width and height (or contents: true, then they are only a minimum)")
	}
	id, rep, err := m.AddContainer(view, core.ContainerSpec{Entity: in.Entity, Name: in.Name, Kind: in.Kind, Parent: in.Parent, StyleID: in.StyleID,
		Rect: core.Rect{X: in.X, Y: in.Y, Width: in.Width, Height: in.Height}, Contents: in.Contents}, in.RequestedByHuman, "agent")
	if err != nil {
		return nil, nil, err
	}
	res, out, err := s.geomDone(m, view, in.Project, rep)
	if o, ok := out.(map[string]any); ok {
		o["entity"] = id
		if rep.Contents != nil {
			o["contents"] = rep.Contents
		}
	}
	if res != nil && len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			if rep.Contents != nil {
				tc.Text = fmt.Sprintf("container %s: %s%s", id, contentsSentence(rep.Contents), tc.Text)
			} else {
				tc.Text = fmt.Sprintf("container %s placed. %s", id, tc.Text)
			}
		}
	}
	return res, out, err
}

// contentsSentence says what add_container with contents did: how many members
// were placed, which were left out and why, and where the grid went.
func contentsSentence(c *core.ContentsReport) string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d member(s) placed inside it", len(c.Placed))
	if len(c.Placed) > 0 {
		fmt.Fprintf(&b, " (%s; %s)", strings.Join(c.Placed, ", "), c.Note)
	}
	b.WriteString(". Membership is the registry's; the grid is only a starting arrangement.")
	if len(c.Skipped) > 0 {
		parts := make([]string, len(c.Skipped))
		for i, s := range c.Skipped {
			parts[i] = fmt.Sprintf("%s (%s)", s.Entity, s.Reason)
		}
		fmt.Fprintf(&b, " Skipped: %s.", strings.Join(parts, "; "))
	}
	return b.String() + " "
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
