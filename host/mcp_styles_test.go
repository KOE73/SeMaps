package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"semaps/core"
)

func TestMCPStylesTwoEditorsAndPersistence(t *testing.T) {
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
		events := make(chan modelEvent, 16)
		streams = append(streams, events)
		go func() {
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 4096), 2<<20)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					var event modelEvent
					_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event)
					events <- event
				}
			}
		}()
	}
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{key: models.key, next: http.DefaultTransport}}, DisableStandaloneSSE: true}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "style-test"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	invoke := func(name string, args map[string]any) {
		t.Helper()
		res, text := call(t, cs, name, args)
		if res.IsError {
			t.Fatalf("%s: %s", name, text)
		}
	}
	expectStyles := func(id, fill string, notification ...string) {
		t.Helper()
		for _, stream := range streams {
			for {
				select {
				case event := <-stream:
					if event.Styles == nil {
						continue
					}
					if !strings.Contains(string(event.Styles.Sheet), `"id":"`+id+`"`) || !strings.Contains(string(event.Styles.Sheet), fill) {
						t.Fatalf("event differs: %s", event.Styles.Sheet)
					}
					if event.StyleRenamed != nil && (event.StyleRenamed.OldID != "debug" || event.StyleRenamed.NewID != "debug.renamed") {
						t.Fatalf("invalid rename notification: %+v", event.StyleRenamed)
					}
					if len(notification) > 0 && notification[0] == "rename" && event.StyleRenamed == nil {
						t.Fatal("rename notification missing; selected style cannot follow its new id")
					}
					if len(notification) > 0 && notification[0] == "delete" && event.StyleDeleted != "debug.renamed" {
						t.Fatal("delete notification missing; pending selected style must be cleared")
					}
					goto next
				case <-time.After(3 * time.Second):
					t.Fatal("second editor did not receive shared style state")
				}
			}
		next:
		}
	}
	invoke("create_style", map[string]any{"id": "debug", "name": "Debug", "appliesTo": "block", "forKinds": []string{"class"}, "properties": map[string]any{"fill": "#ffe4e6", "border": map[string]any{"color": "#dc2626"}}})
	expectStyles("debug", "#ffe4e6")
	invoke("set_placement", map[string]any{"view": "v_main", "elements": []string{"e_a"}, "styleId": "debug", "override": nil, "requestedByHuman": true})
	invoke("update_style", map[string]any{"id": "debug", "patch": map[string]any{"fill": "#fed7aa"}})
	expectStyles("debug", "#fed7aa")
	invoke("rename_style", map[string]any{"id": "debug", "newId": "debug.renamed"})
	expectStyles("debug.renamed", "#fed7aa", "rename")
	for range 2 {
		res, err := srv.Client().Get(srv.URL + "/api/model/p/views/v_main/styled")
		if err != nil {
			t.Fatal(err)
		}
		var view core.StyledView
		_ = json.NewDecoder(res.Body).Decode(&view)
		res.Body.Close()
		if !strings.Contains(string(view.View), `"styleId":"debug.renamed"`) {
			t.Fatalf("view: %s", view.View)
		}
	}
	invoke("save", map[string]any{"scope": "styles", "requestedByHuman": true})
	expectStyles("debug.renamed", "#fed7aa")
	invoke("save", map[string]any{"project": "p", "requestedByHuman": true})
	invoke("update_style", map[string]any{"id": "debug.renamed", "patch": map[string]any{"fill": "#d1fae5"}})
	expectStyles("debug.renamed", "#d1fae5")
	invoke("discard", map[string]any{"scope": "styles", "requestedByHuman": true})
	expectStyles("debug.renamed", "#fed7aa")
	// An edit from a human editor uses the same core operation and broadcasts to both subscribers.
	body := `{"action":"update","id":"debug.renamed","patch":{"border":{"color":"#2563eb"}}}`
	res := modelRequest(t, srv.Client(), srv.URL+"/api/styles", models.key, "POST", body)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("human edit %d: %s", res.StatusCode, data)
	}
	expectStyles("debug.renamed", "#2563eb")
	invoke("discard", map[string]any{"scope": "styles", "requestedByHuman": true})
	expectStyles("debug.renamed", "#dc2626")
	restarted, err := newModelService(models.workspace)
	if err != nil {
		t.Fatal(err)
	}
	style, err := restarted.shared.GetStyle("debug.renamed")
	if err != nil || !strings.Contains(string(style.Resolved), "#fed7aa") || len(style.Dirty) > 0 {
		t.Fatalf("reopen style: %+v %v", style, err)
	}
	m, err := restarted.get("p")
	if err != nil {
		t.Fatal(err)
	}
	view, _ := m.View("v_main")
	if !strings.Contains(string(view), "debug.renamed") {
		t.Fatal(string(view))
	}
	if _, err := os.Stat(filepath.Join(models.workspace, "styles.json")); err != nil {
		t.Fatal(err)
	}
	resMCP, text := call(t, cs, "delete_style", map[string]any{"id": "debug.renamed"})
	if !resMCP.IsError || !strings.Contains(text, "e_a") {
		t.Fatal("used deletion did not enumerate references", text)
	}
	invoke("delete_style", map[string]any{"id": "debug.renamed", "replacement": "class"})
	expectStyles("class", "#ffffff", "delete")
	invoke("discard", map[string]any{"scope": "styles", "requestedByHuman": true})
	for _, stream := range streams {
		select {
		case event := <-stream:
			if event.Styles == nil || len(event.Changed) == 0 || len(event.Dirty.Views["v_main"]) != 0 {
				t.Fatalf("clean discard did not notify reverted view references: %+v", event)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("discard reference notification missing")
		}
	}
}
