package main

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"semaps/core"
	"strings"
	"testing"
	"time"
)

func TestMCPStyleExtensionsTwoEditorsAndRestart(t *testing.T) {
	models, old := hostModelFixture(t)
	old.Close()
	mux := http.NewServeMux()
	models.register(mux)
	registerMCPHTTP(mux, project{Root: models.workspace}, models.workspace, models.workspace, models, nil, nil, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	streams := []chan modelEvent{}
	for range 2 {
		response, err := srv.Client().Get(srv.URL + "/api/events?project=p")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		events := make(chan modelEvent, 64)
		streams = append(streams, events)
		go func() {
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 4096), 2<<20)
			for scanner.Scan() {
				if raw, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
					var e modelEvent
					_ = json.Unmarshal([]byte(raw), &e)
					events <- e
				}
			}
		}()
	}
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{key: models.key, next: http.DefaultTransport}}, DisableStandaloneSSE: true}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "modifier-test"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	invoke := func(name string, args map[string]any) string {
		t.Helper()
		r, body := call(t, cs, name, args)
		if r.IsError {
			t.Fatal(name, body)
		}
		return body
	}
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"style_selector_fields", "list_modifiers", "get_modifier", "create_modifier", "update_modifier", "rename_modifier", "delete_modifier", "list_style_rules", "get_style_rule", "create_style_rule", "update_style_rule", "rename_style_rule", "delete_style_rule", "preview_style_rule", "explain_style", "set_modifiers", "set_style_rule_exclusions"} {
		if !names[name] {
			t.Fatal("missing tool", name)
		}
	}
	expect := func(collection, action, id string) {
		t.Helper()
		for _, stream := range streams {
			for {
				select {
				case e := <-stream:
					if e.StyleExtension == nil {
						continue
					}
					if e.StyleExtension.Collection != collection || e.StyleExtension.Action != action || e.StyleExtension.ID != id || e.Styles == nil {
						t.Fatal(e)
					}
					goto next
				case <-time.After(3 * time.Second):
					t.Fatal("shared extension notification missing")
				}
			}
		next:
		}
	}
	invoke("create_modifier", map[string]any{"id": "palette", "definition": map[string]any{"name": "Palette", "patches": map[string]any{"block": map[string]any{"fill": "#aabbcc80", "title": map[string]any{"opacity": 0.6}}}}})
	expect("modifiers", "create", "palette")
	rule := map[string]any{"id": "types", "name": "Types", "selectorVersion": 1, "selector": map[string]any{"field": "kind", "op": "eq", "value": "class"}, "modifierIds": []string{"palette"}}
	invoke("create_style_rule", map[string]any{"id": "types", "definition": rule})
	expect("styleRules", "create", "types")
	preview := invoke("preview_style_rule", map[string]any{"project": "p", "view": "v_main", "rule": rule})
	if !strings.Contains(preview, "e_a") {
		t.Fatal(preview)
	}
	for range 2 {
		response, err := srv.Client().Get(srv.URL + "/api/model/p/views/v_main/styled")
		if err != nil {
			t.Fatal(err)
		}
		var styled core.StyledView
		_ = json.NewDecoder(response.Body).Decode(&styled)
		response.Body.Close()
		if !strings.Contains(string(styled.Appearances["e_a"].Patch), "#aabbcc80") {
			t.Fatal(styled)
		}
	}
	invoke("set_modifiers", map[string]any{"project": "p", "view": "v_main", "elements": []string{"e_a"}, "modifiers": []string{"palette"}, "automaticModifiers": false, "requestedByHuman": true})
	explained := invoke("explain_style", map[string]any{"project": "p", "view": "v_main", "id": "e_a"})
	if !strings.Contains(explained, `manual:palette`) {
		t.Fatal(explained)
	}
	invoke("set_style_rule_exclusions", map[string]any{"project": "p", "view": "v_main", "ids": []string{"types"}, "requestedByHuman": true})
	r, body := call(t, cs, "delete_modifier", map[string]any{"id": "palette"})
	if !r.IsError || !strings.Contains(body, "replacement") {
		t.Fatal(body)
	}
	invoke("rename_modifier", map[string]any{"id": "palette", "newId": "palette.new"})
	expect("modifiers", "rename", "palette")
	invoke("rename_style_rule", map[string]any{"id": "types", "newId": "types.new"})
	expect("styleRules", "rename", "types")
	invoke("set_modifiers", map[string]any{"project": "p", "view": "v_main", "elements": []string{"e_a"}, "automaticModifiers": nil, "requestedByHuman": true})
	invoke("save", map[string]any{"scope": "styles", "requestedByHuman": true})
	invoke("save", map[string]any{"project": "p", "requestedByHuman": true})
	restart, err := newModelService(models.workspace)
	if err != nil {
		t.Fatal(err)
	}
	view, err := restart.shared.View("p", "v_main")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Appearances["e_a"].Applied) != 1 || len(view.Appearances["e_a"].Skipped) != 1 || !strings.Contains(string(view.View), "palette.new") || !strings.Contains(string(view.View), "types.new") {
		t.Fatal(view)
	}
	var geometry struct{ Placements []struct{ X, Y int } }
	_ = json.Unmarshal(view.View, &geometry)
	if len(geometry.Placements) != 1 || geometry.Placements[0].X != 1 || geometry.Placements[0].Y != 2 {
		t.Fatal("geometry changed", string(view.View))
	}
	denied := modelRequest(t, srv.Client(), srv.URL+"/api/style-modifiers", "", http.MethodPost, `{"action":"delete","id":"palette.new"}`)
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthorized write", denied.StatusCode)
	}
}
