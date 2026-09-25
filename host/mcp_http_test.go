package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPHTTPUsesLiveModelAndRequiresHumanSave(t *testing.T) {
	models, fixture := hostModelFixture(t)
	defer fixture.Close()
	mux := http.NewServeMux()
	registerMCPHTTP(mux, project{Root: models.workspace}, models.workspace, models.workspace, models)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := srv.Client().Post(srv.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without key: %d", res.StatusCode)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{key: models.key, next: http.DefaultTransport}}, DisableStandaloneSSE: true}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	events := make(chan modelEvent, 1)
	models.mu.Lock()
	models.clients["p"] = map[chan modelEvent]struct{}{events: {}}
	models.mu.Unlock()
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := call("set_text", map[string]any{"lang": "ru", "key": "e_a", "field": "description", "value": "changed"}); r.IsError {
		t.Fatal(r)
	}
	select {
	case ev := <-events:
		if ev.Author != "agent" || len(ev.Changed) != 1 || ev.Changed[0].ID != "e_a" {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("agent change did not publish event")
	}
	m, err := models.get("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Dirty().Registry) != 1 {
		t.Fatalf("no model dirt: %+v", m.Dirty())
	}
	if _, err := os.Stat(filepath.Join(models.workspace, "projects", "p", "text.ru.json")); !os.IsNotExist(err) {
		t.Fatalf("agent wrote file before Save: %v", err)
	}
	if r := call("save", map[string]any{}); !r.IsError {
		t.Fatal("save without human accepted")
	}
	if r := call("save", map[string]any{"requestedByHuman": true}); r.IsError {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(models.workspace, "projects", "p", "text.ru.json")); err != nil {
		t.Fatal(err)
	}
}
