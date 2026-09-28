package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"semaps/core"
)

// graphFixture builds a workspace with one project "p", one entity joined to
// a symbol, and (unless proj.File == "") a run store with a single "done"
// run of the "csharp" extractor whose facts add a code-only symbol on top of
// the entity's own. Returns the graphService and its models, wired the same
// way server.go wires them.
func graphFixture(t *testing.T, withRun bool) (*graphService, *modelService) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for file, body := range map[string]string{
		"project.json":   `{"id":"p"}`,
		"entities.json":  `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","status":"present","codeRef":"A.cs","symbol":"A"}]}`,
		"relations.json": `{"relations":[]}`,
	} {
		p := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	models, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}

	semapsFile := ""
	var extractors []extractorConf
	if withRun {
		semapsFile = filepath.Join(ws, "test.semaps")
		extractors = []extractorConf{{ID: "csharp", Language: "csharp", Project: "p"}}
	}
	proj := project{File: semapsFile, Extractors: extractors}
	gs := newGraphService(proj, models)

	if withRun {
		store := newRunStore(semapsFile)
		info := &runInfo{ID: "20260101-000000-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "done", Finished: time.Now()}
		if err := os.MkdirAll(filepath.Join(store.dir, info.ID), 0o755); err != nil {
			t.Fatal(err)
		}
		facts := core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{
			{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"},
			{ID: "A.Run", Kind: "function", NativeKind: "method", Name: "Run", File: "A.cs"},
		}, Edges: []core.Edge{{From: "A", To: "A.Run", Kind: "holds"}}}
		b, _ := json.Marshal(facts)
		if err := os.WriteFile(store.path(info.ID, "facts.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := store.save(info); err != nil {
			t.Fatal(err)
		}
	}
	return gs, models
}

func graphServer(gs *graphService) *httptest.Server {
	mux := http.NewServeMux()
	gs.register(mux)
	return httptest.NewServer(mux)
}

func getGraphJSON(t *testing.T, url string) (*http.Response, map[string]any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	if res.StatusCode == http.StatusOK {
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("bad JSON: %v: %s", err, b)
		}
	}
	return res, out
}

func TestGraphEndpointNoRuns(t *testing.T) {
	gs, _ := graphFixture(t, false)
	srv := graphServer(gs)
	defer srv.Close()
	// no .semaps file at all: --workspace mode, 409 like the other tool endpoints.
	res, _ := getGraphJSON(t, srv.URL+"/api/graph/p")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 with no .semaps file, got %d", res.StatusCode)
	}
}

func TestGraphEndpointWithRun(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()

	res, body := getGraphJSON(t, srv.URL+"/api/graph/p")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes (A joined, A.Run code-only), got %+v", body)
	}
	facts, _ := body["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("expected one facts entry, got %+v", body["facts"])
	}
	entry := facts[0].(map[string]any)
	if entry["extractor"] != "csharp" || entry["run"] != "20260101-000000-csharp" {
		t.Fatalf("unexpected facts entry: %+v", entry)
	}

	// level=types drops the function node
	res2, body2 := getGraphJSON(t, srv.URL+"/api/graph/p?level=types")
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res2.StatusCode)
	}
	if nodes2, _ := body2["nodes"].([]any); len(nodes2) != 1 {
		t.Fatalf("expected 1 node with level=types, got %+v", body2["nodes"])
	}
}

func TestGraphEndpointAround(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()

	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:A&depth=1")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	if nodes, _ := body["nodes"].([]any); len(nodes) != 2 {
		t.Fatalf("expected both nodes in the neighbourhood, got %+v", nodes)
	}

	res2, _ := getGraphJSON(t, srv.URL+"/api/graph/p?around=nope")
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown node, got %d", res2.StatusCode)
	}
}

func TestGraphEndpointBadLevelAndDepth(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()

	if res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?level=bogus"); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a bad level, got %d", res.StatusCode)
	}
	if res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:A&depth=99"); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for depth out of range, got %d", res.StatusCode)
	}
}

func TestGraphEndpointUnknownContainer(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	if res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?container=c_nope"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown container, got %d", res.StatusCode)
	}
}

func TestGraphEndpointFieldsDefaultStripsPositionAndVia(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()

	// default fields (via,position) keep File/Line and Via.
	_, body := getGraphJSON(t, srv.URL+"/api/graph/p")
	nodes := body["nodes"].([]any)
	found := false
	for _, n := range nodes {
		nm := n.(map[string]any)
		if nm["file"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected at least one node with a file by default: %+v", nodes)
	}

	// fields=members only: no via, no position.
	_, body2 := getGraphJSON(t, srv.URL+"/api/graph/p?fields=members")
	for _, n := range body2["nodes"].([]any) {
		nm := n.(map[string]any)
		if nm["file"] != nil {
			t.Fatalf("fields=members must strip position: %+v", nm)
		}
	}
}

// TestGraphServiceNotifyPublishesDiff exercises step 5's wiring end to end:
// a run finishing calls the callback registered on the run store, which
// diffs against the previous graph and publishes to /api/events.
func TestGraphServiceNotifyPublishesDiff(t *testing.T) {
	gs, models := graphFixture(t, true)
	// seed gs.prev with the graph before the run, as if it had been read once already.
	old := &core.Graph{Nodes: []core.GraphNode{{ID: "csharp:A", Presence: "both"}}}
	gs.prev["p"] = old

	mux := http.NewServeMux()
	models.register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	events := make(chan string, 1)
	req, _ := http.NewRequest("GET", srv.URL+"/api/events?project=p", nil)
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := res.Body.Read(buf)
			if n > 0 {
				s := string(buf[:n])
				if len(s) > 6 && s[:6] == "data: " {
					events <- s
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	gs.notifyRunFinished(&runInfo{ID: "run2", Extractor: "csharp", Project: "p", State: "done"})

	select {
	case ev := <-events:
		if !contains(ev, `"graph"`) {
			t.Fatalf("expected a graph diff in the SSE event, got %s", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE event received after a run finished")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
