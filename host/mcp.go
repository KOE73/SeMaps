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
}

func runMCP(proj project, workspace, sourceRoot, projectID string) int {
	s := &mcpServer{proj: proj, workspace: workspace, sourceRoot: sourceRoot, project: projectID}
	if err := s.server().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "semaps mcp: %v\n", err)
		return 1
	}
	return 0
}

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

func (s *mcpServer) server() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "semaps", Version: "1"}, &mcp.ServerOptions{
		Instructions: "SeMaps registry of this repository. Read with list_*/get_*/find_*; write only through these tools. " +
			"Nothing can be deleted; view geometry only with requestedByHuman when a human asked. See docs/ADOPTING.md.",
	})
	tool := func(name, desc string) *mcp.Tool { return &mcp.Tool{Name: name, Description: desc} }

	mcp.AddTool(srv, tool("list_projects", "Projects of the workspace and their views."), s.listProjects)
	mcp.AddTool(srv, tool("list_views", "Views of one project."), s.listViews)
	mcp.AddTool(srv, tool("get_entity", "One entity by id or by symbol."), s.getEntity)
	mcp.AddTool(srv, tool("find_entities", "Entities by name/symbol/namespace substring, kind, status."), s.findEntities)
	mcp.AddTool(srv, tool("get_relations", "Relations of an entity (or all), by direction, type, status."), s.getRelations)
	mcp.AddTool(srv, tool("get_relation_types", "The relation-type vocabulary with default visibility."), s.getRelationTypes)
	mcp.AddTool(srv, tool("get_text", "Text entry of a key in one language."), s.getText)
	mcp.AddTool(srv, tool("doctor", "Extractors and runtimes found, and the model check of the workspace."), s.doctor)
	mcp.AddTool(srv, tool("sync_preview", "What sync would change, writing nothing (= semaps sync --dry-run)."), s.syncPreview)

	mcp.AddTool(srv, tool("set_text", "Write one text field as authored, with at = now."), s.setText)
	mcp.AddTool(srv, tool("add_relation", "Add an authored relation; the id is minted and returned."), s.addRelation)
	mcp.AddTool(srv, tool("add_relation_type", "Add an authored relation type."), s.addRelationType)
	mcp.AddTool(srv, tool("set_relation_visible", "Show or hide one relation on one view (relations.except)."), s.setRelationVisible)
	mcp.AddTool(srv, tool("confirm_rename", "Answer a sync rename candidate: entity + symbol, or relation + member."), s.confirmRename)
	mcp.AddTool(srv, tool("extract", "Run the extractors of the .semaps file; returns run ids."), s.extract)
	mcp.AddTool(srv, tool("sync", "Reconcile the registry with the code (extracts first unless run is given)."), s.sync)
	mcp.AddTool(srv, tool("place_entities", "Put entities on a view. Only when a human asked for it: requestedByHuman."), s.placeEntities)
	return srv
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
			return nil, p.Views, nil
		}
	}
	return nil, []any{}, nil
}

// record is one registry item, read for filtering.
type record struct {
	raw    json.RawMessage
	fields map[string]any
}

func (r record) str(k string) string { s, _ := r.fields[k].(string); return s }

func (s *mcpServer) records(project, file string) ([]record, error) {
	raws, err := core.Records(s.workspace, s.pick(project), file)
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
	return nil, raws(types), nil
}

func (s *mcpServer) getText(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, any, error) {
	entry, err := core.Text(s.workspace, s.pick(in.Project), in.Lang, in.Key)
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
	if err := core.SetText(s.workspace, s.pick(in.Project), in.Lang, in.Key, in.Field, in.Value); err != nil {
		return nil, nil, err
	}
	return done("%s.%s (%s) written", in.Key, in.Field, in.Lang)
}

func (s *mcpServer) addRelation(_ context.Context, _ *mcp.CallToolRequest, in addRelationIn) (*mcp.CallToolResult, any, error) {
	id, err := core.AddRelation(s.workspace, s.pick(in.Project), in.From, in.To, in.Type)
	if err != nil {
		return nil, nil, err
	}
	return done("%s added", id)
}

func (s *mcpServer) addRelationType(_ context.Context, _ *mcp.CallToolRequest, in addTypeIn) (*mcp.CallToolResult, any, error) {
	if err := core.AddRelationType(s.workspace, s.pick(in.Project), in.ID, in.Visibility, in.StyleID); err != nil {
		return nil, nil, err
	}
	return done("type %s added; give it a name: set_text key rt_%s", in.ID, in.ID)
}

func (s *mcpServer) setRelationVisible(_ context.Context, _ *mcp.CallToolRequest, in visibleIn) (*mcp.CallToolResult, any, error) {
	if err := core.SetRelationVisible(s.workspace, s.pick(in.Project), in.View, in.Relation, in.Visible); err != nil {
		return nil, nil, err
	}
	return done("%s on %s: visible=%v", in.Relation, in.View, in.Visible)
}

func (s *mcpServer) confirmRename(_ context.Context, _ *mcp.CallToolRequest, in renameIn) (*mcp.CallToolResult, any, error) {
	p := s.pick(in.Project)
	switch {
	case in.Entity != "" && in.Relation == "":
		if err := core.ConfirmEntityRename(s.workspace, p, in.Entity, in.Symbol); err != nil {
			return nil, nil, err
		}
		return done("%s takes symbol %s; run sync again", in.Entity, in.Symbol)
	case in.Relation != "" && in.Entity == "":
		if err := core.ConfirmRelationRename(s.workspace, p, in.Relation, in.Member); err != nil {
			return nil, nil, err
		}
		return done("%s takes member %s; run sync again", in.Relation, in.Member)
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
	var out []*runInfo
	for _, e := range list {
		info, wait, err := store.start(s.proj, e, os.Stderr)
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
	return nil, out, nil
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
		rep, err := store.syncRun(s.workspace, info, core.SyncOptions{DryRun: dryRun, NoRenames: in.NoRenames})
		if err != nil {
			return nil, nil, err
		}
		fmt.Fprintf(&b, "== %s → project %s (run %s)\n", info.Extractor, info.Project, info.ID)
		rep.Print(&b)
		reports = append(reports, rep)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, reports, nil
}

func (s *mcpServer) syncPreview(_ context.Context, _ *mcp.CallToolRequest, in syncIn) (*mcp.CallToolResult, any, error) {
	return s.reconcile(in, true)
}

func (s *mcpServer) sync(_ context.Context, _ *mcp.CallToolRequest, in syncIn) (*mcp.CallToolResult, any, error) {
	return s.reconcile(in, false)
}

func (s *mcpServer) placeEntities(_ context.Context, _ *mcp.CallToolRequest, in placeIn) (*mcp.CallToolResult, any, error) {
	if err := core.PlaceEntities(s.workspace, s.pick(in.Project), in.View, in.Entities, in.RequestedByHuman); err != nil {
		return nil, nil, err
	}
	return done("%d placed on %s", len(in.Entities), in.View)
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
