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

// TestMCPHTTPSettingsChangeReachesNewSessionsInstructionsToo: a settings
// change (mcp.tools/mcp.description) must reach a brand new session's
// Instructions, not only its tool list — the Go SDK gives no way to change
// Instructions on a live *mcp.Server, so registerMCPHTTP keeps a small pool
// of them (mcpServerPool, host/mcp_http.go) and swaps in a fresh one on a
// change. A session opened before the change keeps its own server (old
// Instructions), but still sees the new tool list once it asks again — the
// same promise the tool list already had.
func TestMCPHTTPSettingsChangeReachesNewSessionsInstructionsToo(t *testing.T) {
	models, fixture := hostModelFixture(t)
	defer fixture.Close()
	mux := http.NewServeMux()
	box := newMcpSettingsBox(mcpSettings{Tools: "one", Description: "standard"})
	registerMCPHTTP(mux, project{Root: models.workspace}, models.workspace, models.workspace, models, nil, box)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	connect := func() *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
		transport := &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{key: models.key, next: http.DefaultTransport}}, DisableStandaloneSSE: true}
		session, err := client.Connect(context.Background(), transport, nil)
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	toolNames := func(session *mcp.ClientSession) map[string]bool {
		t.Helper()
		res, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, tl := range res.Tools {
			out[tl.Name] = true
		}
		return out
	}

	first := connect()
	defer first.Close()
	firstInstructions := first.InitializeResult().Instructions
	wantOne := serverInstructions("one", "standard")
	if firstInstructions != "SeMaps registry of this repository. Read with list_*/get_*/find_*; write only through these tools. "+
		"Nothing can be deleted; view geometry only with requestedByHuman when a human asked. "+wantOne {
		t.Fatalf("first session instructions not the `one`/`standard` text:\n%s", firstInstructions)
	}
	before := toolNames(first)
	if !before["get_graph"] || before["who_extends"] {
		t.Fatalf("expected the `one` set before the change, got %+v", before)
	}

	box.Set(mcpSettings{Tools: "narrow", Description: "brief"})

	second := connect()
	defer second.Close()
	wantNarrow := serverInstructions("narrow", "brief")
	secondInstructions := second.InitializeResult().Instructions
	if secondInstructions == firstInstructions {
		t.Fatal("second session got the same (stale) instructions as the first")
	}
	if !strings.Contains(secondInstructions, wantNarrow) {
		t.Fatalf("second session instructions not the `narrow`/`brief` text:\n%s", secondInstructions)
	}
	after := toolNames(second)
	if after["get_graph"] || !after["who_extends"] {
		t.Fatalf("expected the `narrow` set on the second session, got %+v", after)
	}

	// The FIRST session, asked again, sees the new tool list too — its own
	// Instructions (already delivered at initialize) do not change, but its
	// server's tools were rebuilt in place (mcpServerPool.onSettingsChange).
	firstAfter := toolNames(first)
	if firstAfter["get_graph"] || !firstAfter["who_extends"] {
		t.Fatalf("expected the first session's tool list to have moved to `narrow` too, got %+v", firstAfter)
	}
	if first.InitializeResult().Instructions != firstInstructions {
		t.Fatal("first session's already-delivered instructions changed")
	}
}

func TestMCPHTTPUsesLiveModelAndRequiresHumanSave(t *testing.T) {
	models, fixture := hostModelFixture(t)
	defer fixture.Close()
	mux := http.NewServeMux()
	registerMCPHTTP(mux, project{Root: models.workspace}, models.workspace, models.workspace, models, nil, nil)
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
