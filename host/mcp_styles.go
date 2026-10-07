package main

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"semaps/core"
)

func result(value any) (*mcp.CallToolResult, any, error) { return nil, value, nil }

type styleIDIn struct {
	ID string `json:"id"`
}
type createStyleIn struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	AppliesTo  string         `json:"appliesTo" jsonschema:"block, container or edge"`
	ForKinds   []string       `json:"forKinds,omitempty"`
	BasedOn    string         `json:"basedOn,omitempty"`
	Properties map[string]any `json:"properties,omitempty" jsonschema:"sparse formatting fields of CONTRACT 11.5: fill, border, header, title, subtitle, icon, shape, template or edge line, source, target, label, routing"`
	Expected   *uint64        `json:"expectedRevision,omitempty"`
}
type updateStyleIn struct {
	ID       string         `json:"id"`
	Patch    map[string]any `json:"patch" jsonschema:"recursive partial definition: omitted fields survive; null removes own values to resume inheritance; arrays replace; use rename_style for id"`
	Expected *uint64        `json:"expectedRevision,omitempty"`
}
type renameStyleIn struct {
	ID       string  `json:"id"`
	NewID    string  `json:"newId"`
	Expected *uint64 `json:"expectedRevision,omitempty"`
}
type deleteStyleIn struct {
	ID          string  `json:"id"`
	Replacement string  `json:"replacement,omitempty" jsonschema:"explicit compatible style id, required while used; errors list all workspace references"`
	Expected    *uint64 `json:"expectedRevision,omitempty"`
}

func (s *mcpServer) registerStyleTools(srv *mcp.Server) {
	mcp.AddTool(srv, read("list_styles", "Shared workspace named styles, including unsaved edits. Filter by exact id/appliesTo, any forKinds and name substring. Styles belong to kinds, independently of the entity's kind."), s.listStyles)
	mcp.AddTool(srv, read("get_style", "Definition, basedOn, fully inherited formatting including terminal defaults, unsaved authors, workspace usage references and revision."), s.getStyle)
	mcp.AddTool(srv, write("create_style", "Create a named workspace style with explicit id/name/appliesTo and own forKinds (except three fallbacks). Block, container and edge styles stay separate. Unsaved; visible in every editor immediately."), s.createStyle)
	mcp.AddTool(srv, write("update_style", "Recursively patch a named style: omitted fields survive, explicit null removes own values and restores inheritance. Updates every open editor and descendant style. expectedRevision detects conflicts. Unsaved."), s.updateStyle)
	mcp.AddTool(srv, write("rename_style", "Atomically rename a workspace style and all references: every project's placements/edges, basedOn and dictionary base styles. Geometry and overrides untouched. Unsaved until save scope styles."), s.renameStyle)
	mcp.AddTool(srv, write("delete_style", "Delete a style; if used, list references and require explicit compatible replacement. Atomically repoint references across the workspace. Unsaved until save scope styles."), s.deleteStyle)
}

func (s *mcpServer) listStyles(_ context.Context, _ *mcp.CallToolRequest, in core.StyleFilter) (*mcp.CallToolResult, any, error) {
	styles, err := s.models.shared.ListStyles(in)
	if err != nil {
		return nil, nil, err
	}
	return result(map[string]any{"styles": styles, "state": s.models.shared.Styles()})
}
func (s *mcpServer) getStyle(_ context.Context, _ *mcp.CallToolRequest, in styleIDIn) (*mcp.CallToolResult, any, error) {
	detail, err := s.models.shared.GetStyle(in.ID)
	if err != nil {
		return nil, nil, err
	}
	return result(detail)
}
func (s *mcpServer) styleEdit(edit core.StyleEdit) (*mcp.CallToolResult, any, error) {
	s.models.structureMu.Lock()
	defer s.models.structureMu.Unlock()
	detail, err := s.models.shared.EditStyle(edit, "agent")
	if err != nil {
		return nil, nil, err
	}
	s.models.publishStyleEdit("agent", edit)
	return result(map[string]any{"style": detail, "saved": false, "scope": "workspace styles", "link": "/app/"})
}
func (s *mcpServer) createStyle(_ context.Context, _ *mcp.CallToolRequest, in createStyleIn) (*mcp.CallToolResult, any, error) {
	definition := map[string]any{}
	for k, v := range in.Properties {
		definition[k] = v
	}
	definition["name"], definition["appliesTo"] = in.Name, in.AppliesTo
	if in.ForKinds != nil {
		definition["forKinds"] = in.ForKinds
	}
	if in.BasedOn != "" {
		definition["basedOn"] = in.BasedOn
	}
	return s.styleEdit(core.StyleEdit{Action: "create", ID: in.ID, Definition: definition, Expected: in.Expected})
}
func (s *mcpServer) updateStyle(_ context.Context, _ *mcp.CallToolRequest, in updateStyleIn) (*mcp.CallToolResult, any, error) {
	if len(in.Patch) == 0 {
		return nil, nil, errors.New("patch must contain at least one field")
	}
	return s.styleEdit(core.StyleEdit{Action: "update", ID: in.ID, Patch: in.Patch, Expected: in.Expected})
}
func (s *mcpServer) renameStyle(_ context.Context, _ *mcp.CallToolRequest, in renameStyleIn) (*mcp.CallToolResult, any, error) {
	return s.styleEdit(core.StyleEdit{Action: "rename", ID: in.ID, NewID: in.NewID, Expected: in.Expected})
}
func (s *mcpServer) deleteStyle(_ context.Context, _ *mcp.CallToolRequest, in deleteStyleIn) (*mcp.CallToolResult, any, error) {
	return s.styleEdit(core.StyleEdit{Action: "delete", ID: in.ID, Replacement: in.Replacement, Expected: in.Expected})
}
