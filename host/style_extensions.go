package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"semaps/core"
)

type extensionCreateIn struct {
	ID         string         `json:"id"`
	Definition map[string]any `json:"definition"`
	Expected   *uint64        `json:"expectedRevision,omitempty"`
}
type rulePreviewIn struct {
	Project string         `json:"project"`
	View    string         `json:"view"`
	Rule    map[string]any `json:"rule"`
}
type explainStyleIn struct {
	Project string `json:"project"`
	View    string `json:"view"`
	ID      string `json:"id"`
}
type ruleExclusionsIn struct {
	Project          string   `json:"project"`
	View             string   `json:"view"`
	IDs              []string `json:"ids"`
	RequestedByHuman bool     `json:"requestedByHuman"`
}
type assignModifiersIn struct {
	Project          string   `json:"project"`
	View             string   `json:"view"`
	Elements         []string `json:"elements,omitempty"`
	Relations        []string `json:"relations,omitempty"`
	Modifiers        []string `json:"modifiers,omitempty"`
	Disabled         []string `json:"disabledModifiers,omitempty"`
	Automatic        *bool    `json:"automaticModifiers,omitempty"`
	RequestedByHuman bool     `json:"requestedByHuman"`
}

func (s *modelService) registerStyleExtensions(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/model/{project}/views/{id}/style-rule-exclusions", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorize(w, r) {
			return
		}
		var body struct {
			IDs    []string `json:"ids"`
			Client string   `json:"client"`
		}
		if err := readJSON(r, &body); err != nil {
			modelError(w, err)
			return
		}
		model, err := s.get(r.PathValue("project"))
		if err != nil {
			modelError(w, err)
			return
		}
		refs, err := model.SetStyleRuleExclusions(r.PathValue("id"), body.IDs, true, "human")
		if err != nil {
			modelError(w, err)
			return
		}
		dirty := model.Dirty()
		s.publish(model.ProjectID(), modelEvent{Client: body.Client, Author: "human", Changed: refs, Dirty: dirty})
		writeJSON(w, map[string]any{"changed": refs, "dirty": dirty})
	})
	mux.HandleFunc("GET /api/style-selectors", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": 1, "fields": core.StyleSelectorFields(), "missing": "unknown propagates through not; exists explicitly tests presence", "order": "ascending order then id; later properties win"})
	})
	for path, collection := range map[string]string{"/api/style-modifiers": core.ModifiersCollection, "/api/style-rules": core.StyleRulesCollection} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
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
				modelError(w, err)
				return
			}
			detail, err := s.shared.EditStyleExtension(collection, body.StyleEdit, "human")
			if err != nil {
				modelError(w, err)
				return
			}
			s.publishStyleExtension("human", collection, body.StyleEdit, body.Client)
			writeJSON(w, map[string]any{"detail": detail, "state": s.shared.Styles()})
		})
	}
	mux.HandleFunc("POST /api/style-rules/preview", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorize(w, r) {
			return
		}
		var body rulePreviewIn
		if err := readJSON(r, &body); err != nil {
			modelError(w, err)
			return
		}
		raw, _ := json.Marshal(body.Rule)
		matches, err := s.shared.PreviewStyleRule(body.Project, body.View, raw)
		if err != nil {
			modelError(w, err)
			return
		}
		writeJSON(w, map[string]any{"matches": matches, "count": len(matches)})
	})
	mux.HandleFunc("POST /api/model/{project}/views/{id}/modifiers", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorize(w, r) {
			return
		}
		var body struct {
			Elements  []string                   `json:"elements"`
			Relations []string                   `json:"relations"`
			Fields    map[string]json.RawMessage `json:"fields"`
			Client    string                     `json:"client"`
		}
		if err := readJSON(r, &body); err != nil {
			modelError(w, err)
			return
		}
		model, err := s.get(r.PathValue("project"))
		if err != nil {
			modelError(w, err)
			return
		}
		refs, err := model.SetStyleModifiers(r.PathValue("id"), body.Elements, body.Relations, body.Fields, true, "human")
		if err != nil {
			modelError(w, err)
			return
		}
		dirty := model.Dirty()
		s.publish(model.ProjectID(), modelEvent{Client: body.Client, Author: "human", Changed: refs, Dirty: dirty})
		writeJSON(w, map[string]any{"changed": refs, "dirty": dirty})
	})
}

func (s *mcpServer) registerStyleExtensionTools(srv *mcp.Server) {
	mcp.AddTool(srv, write("set_style_rule_exclusions", "Replace the list of automatic rule IDs disabled on a view. Empty ids enables all rules. Does not change rule definitions, placements or geometry. requestedByHuman required."), func(_ context.Context, _ *mcp.CallToolRequest, in ruleExclusionsIn) (*mcp.CallToolResult, any, error) {
		model, err := s.service().get(in.Project)
		if err != nil {
			return nil, nil, err
		}
		refs, err := model.SetStyleRuleExclusions(in.View, in.IDs, in.RequestedByHuman, "agent")
		if err != nil {
			return nil, nil, err
		}
		dirty := model.Dirty()
		s.models.publish(model.ProjectID(), modelEvent{Author: "agent", Changed: refs, Dirty: dirty})
		return result(map[string]any{"changed": refs, "dirty": dirty})
	})
	mcp.AddTool(srv, read("style_selector_fields", "Discover versioned selector fields and allowed operators. Conditions are all/any/not trees or {field,op,value}. Missing facts propagate as unknown through not; exists explicitly tests presence. Structural assembly IDs come from contains, never placement parents."), func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return result(map[string]any{"version": 1, "fields": core.StyleSelectorFields()})
	})
	for _, entry := range []struct{ collection, singular, plural string }{{core.ModifiersCollection, "modifier", "modifiers"}, {core.StyleRulesCollection, "style_rule", "style_rules"}} {
		mcp.AddTool(srv, read("list_"+entry.plural, "Workspace definitions including unsaved changes, authors and revision. Modifiers and application rules are independent collections; save/discard scope styles covers both."), func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			items, err := s.models.shared.ListStyleExtensions(entry.collection)
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{entry.plural: items, "state": s.models.shared.Styles()})
		})
		mcp.AddTool(srv, read("get_"+entry.singular, "Definition, unsaved changes, revision and all explicit references, including closed projects and saved references hidden by pending edits."), func(_ context.Context, _ *mcp.CallToolRequest, in styleIDIn) (*mcp.CallToolResult, any, error) {
			detail, err := s.models.shared.GetStyleExtension(entry.collection, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return result(detail)
		})
		mcp.AddTool(srv, write("create_"+entry.singular, "Create an unsaved workspace definition. Modifier: name, patches keyed block/container/edge with sparse formatting. Rule: name, selectorVersion:1, selector, ordered modifierIds, optional enabled/order/scope.projects/scope.views. Changes broadcast to every editor."), func(_ context.Context, _ *mcp.CallToolRequest, in extensionCreateIn) (*mcp.CallToolResult, any, error) {
			return s.editStyleExtension(entry.collection, core.StyleEdit{Action: "create", ID: in.ID, Definition: in.Definition, Expected: in.Expected})
		})
		mcp.AddTool(srv, write("update_"+entry.singular, "Recursive partial patch: omitted fields survive, null removes own values. expectedRevision detects conflicting changes. Rules are evaluated, never baked into placements."), func(_ context.Context, _ *mcp.CallToolRequest, in updateStyleIn) (*mcp.CallToolResult, any, error) {
			return s.editStyleExtension(entry.collection, core.StyleEdit{Action: "update", ID: in.ID, Patch: in.Patch, Expected: in.Expected})
		})
		mcp.AddTool(srv, write("rename_"+entry.singular, "Atomically rename definition and all explicit references across workspace and project views. Geometry, parents, templates and local overrides remain unchanged. Save workspace styles before project references."), func(_ context.Context, _ *mcp.CallToolRequest, in renameStyleIn) (*mcp.CallToolResult, any, error) {
			return s.editStyleExtension(entry.collection, core.StyleEdit{Action: "rename", ID: in.ID, NewID: in.NewID, Expected: in.Expected})
		})
		mcp.AddTool(srv, write("delete_"+entry.singular, "Delete an unused definition. While used, report references and require an explicit compatible replacement. Unsaved until save scope styles."), func(_ context.Context, _ *mcp.CallToolRequest, in deleteStyleIn) (*mcp.CallToolResult, any, error) {
			return s.editStyleExtension(entry.collection, core.StyleEdit{Action: "delete", ID: in.ID, Replacement: in.Replacement, Expected: in.Expected})
		})
	}
	mcp.AddTool(srv, read("preview_style_rule", "Evaluate a draft rule against visible placements and relations on a view without changing the model. Return matches, facts, effective formatting and property provenance; namespace within respects dot boundaries."), func(_ context.Context, _ *mcp.CallToolRequest, in rulePreviewIn) (*mcp.CallToolResult, any, error) {
		raw, _ := json.Marshal(in.Rule)
		matches, err := s.models.shared.PreviewStyleRule(in.Project, in.View, raw)
		if err != nil {
			return nil, nil, err
		}
		return result(map[string]any{"matches": matches, "count": len(matches)})
	})
	mcp.AddTool(srv, read("explain_style", "Effective style of a visible placement or relation: base style, automatic rules, manual modifiers, exclusions, local override, property origins and matching facts."), func(_ context.Context, _ *mcp.CallToolRequest, in explainStyleIn) (*mcp.CallToolResult, any, error) {
		view, err := s.models.shared.View(in.Project, in.View)
		if err != nil {
			return nil, nil, err
		}
		appearance, ok := view.Appearances[in.ID]
		if !ok {
			return nil, nil, core.ErrNotFound
		}
		return result(appearance)
	})
	mcp.AddTool(srv, write("set_modifiers", "Set ordered manual modifiers or automatic exclusions on existing elements/relations of a view. Only given fields change; null removes a field. modifiers, disabledModifiers, automaticModifiers:false disables all automatic modifiers for these placements. Geometry untouched. requestedByHuman."), func(_ context.Context, req *mcp.CallToolRequest, in assignModifiersIn) (*mcp.CallToolResult, any, error) {
		model, err := s.service().get(in.Project)
		if err != nil {
			return nil, nil, err
		}
		given := map[string]json.RawMessage{}
		if req != nil && req.Params != nil {
			_ = json.Unmarshal(req.Params.Arguments, &given)
		}
		fields := map[string]json.RawMessage{}
		for _, key := range []string{"modifiers", "disabledModifiers", "automaticModifiers"} {
			if raw, ok := given[key]; ok {
				fields[key] = raw
			}
		}
		refs, err := model.SetStyleModifiers(in.View, in.Elements, in.Relations, fields, in.RequestedByHuman, "agent")
		if err != nil {
			return nil, nil, err
		}
		s.models.publish(model.ProjectID(), modelEvent{Author: "agent", Changed: refs, Dirty: model.Dirty()})
		return result(map[string]any{"changed": refs, "saved": false})
	})
}

func (s *mcpServer) editStyleExtension(collection string, edit core.StyleEdit) (*mcp.CallToolResult, any, error) {
	s.models.structureMu.Lock()
	defer s.models.structureMu.Unlock()
	detail, err := s.models.shared.EditStyleExtension(collection, edit, "agent")
	if err != nil {
		return nil, nil, err
	}
	s.models.publishStyleExtension("agent", collection, edit, "")
	return result(map[string]any{"detail": detail, "state": s.models.shared.Styles(), "saved": false, "scope": "workspace styles"})
}

func (s *modelService) publishStyleExtension(author, collection string, edit core.StyleEdit, client string) {
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
		dirty := m.Dirty()
		refs := []core.Ref{}
		for _, changes := range dirty.Views {
			refs = append(refs, changes...)
		}
		s.publish(id, modelEvent{Author: author, Client: client, Styles: &snapshot, Changed: refs, Dirty: dirty, StyleExtension: &styleExtensionChange{collection, edit.Action, edit.ID, edit.NewID}})
	}
}
