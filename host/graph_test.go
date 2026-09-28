package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

	// A whole-graph request (no around) now defaults to level=types
	// (ADR_20260928-3 §7), which drops the function node.
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (default level=types drops the function node), got %+v", body)
	}
	facts, _ := body["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("expected one facts entry, got %+v", body["facts"])
	}
	entry := facts[0].(map[string]any)
	if entry["extractor"] != "csharp" || entry["run"] != "20260101-000000-csharp" {
		t.Fatalf("unexpected facts entry: %+v", entry)
	}

	// level=all keeps the function node too
	res2, body2 := getGraphJSON(t, srv.URL+"/api/graph/p?level=all")
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res2.StatusCode)
	}
	if nodes2, _ := body2["nodes"].([]any); len(nodes2) != 2 {
		t.Fatalf("expected 2 nodes with level=all, got %+v", body2["nodes"])
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

// TestGraphEndpointAroundByName: part 2 — `around` accepts a name, not just
// an id, resolved against the graph (here, the short name "A" resolves to
// csharp:A, since it is the only node named that).
func TestGraphEndpointAroundByName(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/graph/p?around=A&depth=1&format=facts")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 resolving around by name, got %d: %s", res.StatusCode, b)
	}
	if !strings.Contains(string(b), "asked `A`, no such id; taken `csharp:A`") {
		t.Fatalf("expected the part 2 notice as the first line, got:\n%s", b)
	}
}

// TestGraphEndpointFanoutNote: `fanout` caps neighbours per (node, relation)
// and the answer says what was left out.
func TestGraphEndpointFanoutNote(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/graph/p?around=csharp:A&depth=1&fanout=0&format=facts")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}

// TestGraphFindEndpoint: part 2's plain substring search.
func TestGraphFindEndpoint(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p/find?q=run")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	candidates, _ := body["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatalf("expected at least one candidate for substring 'run', got %+v", body)
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

// TestGraphEndpointExactlyAsTheEditorSendsIt: editor/src/app/graph/types.ts
// fetchGraph() requests exactly `?fields=via,position` (no `around`, no
// `set`/`kinds`/`follow`, no `missing`) — this must keep working byte-shape
// compatible (nodes/edges/facts/stats) after the agent-answers rework, even
// though the whole-graph default level is now `types` (a no-op here: the
// fixture carries no function/value/method nodes besides the one the other
// test already covers under level=types explicitly).
func TestGraphEndpointExactlyAsTheEditorSendsIt(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?fields=via,position")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	for _, key := range []string{"nodes", "edges", "facts", "stats"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected key %q in the editor's graph response, got %+v", key, body)
		}
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

// TestGraphEndpointFieldsEmptyMeansNone: an absent `fields` is the default
// (via,position), but an explicit empty value means "nothing extra" — the
// query string cannot tell "absent" from "empty" on its own, so the handler
// must check presence, not just the value.
func TestGraphEndpointFieldsEmptyMeansNone(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()

	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?fields=")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	for _, n := range body["nodes"].([]any) {
		nm := n.(map[string]any)
		if nm["file"] != nil {
			t.Fatalf("fields= (empty) must strip position, got %+v", nm)
		}
	}
	for _, e := range body["edges"].([]any) {
		em := e.(map[string]any)
		if em["via"] != nil {
			t.Fatalf("fields= (empty) must strip via, got %+v", em)
		}
	}

	// no parameter at all keeps the default.
	_, body2 := getGraphJSON(t, srv.URL+"/api/graph/p")
	found := false
	for _, n := range body2["nodes"].([]any) {
		if n.(map[string]any)["file"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("no fields parameter must keep the default via,position: %+v", body2["nodes"])
	}
}

func TestGraphEndpointFieldsUnknownName(t *testing.T) {
	gs, _ := graphFixture(t, true)
	srv := graphServer(gs)
	defer srv.Close()
	if res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?fields=bogus"); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown field name, got %d", res.StatusCode)
	}
}

// TestGraphEndpointContainerGrandchild: container=c_onnx must include a node
// resolved into a more specific descendant container (fix 2).
func TestGraphEndpointContainerGrandchild(t *testing.T) {
	gs, models := graphFixture(t, true)
	m, err := models.get("p")
	if err != nil {
		t.Fatal(err)
	}
	containersJSON := `{"containers":[
		{"id":"c_onnx"},
		{"id":"c_onnx_core","parent":"c_onnx","match":{"path":["A.cs"]}}
	]}`
	if err := os.WriteFile(filepath.Join(m.ProjectDir(), "containers.json"), []byte(containersJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := graphServer(gs)
	defer srv.Close()

	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?container=c_onnx")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) == 0 {
		t.Fatalf("expected nodes resolved into the descendant c_onnx_core to show up under c_onnx, got %+v", body)
	}
}

func TestGraphEndpointMissingDefaultVsInclude(t *testing.T) {
	// A dedicated workspace, with a model-only entity with status "missing"
	// and a model-only relation (also "missing") from the already-joined
	// entity e_a to it, present from the start — the model loads registry
	// files once, so they must be right before newModelService, not patched
	// in afterwards.
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json": `{"id":"p"}`,
		"entities.json": `{"entities":[
			{"id":"e_a","name":"A","kind":"class","origin":"code","status":"present","codeRef":"A.cs","symbol":"A"},
			{"id":"e_gone","name":"Gone","kind":"class","origin":"code","status":"missing"}
		]}`,
		"relations.json": `{"relations":[
			{"id":"r_gone","from":"e_a","to":"e_gone","type":"uses","origin":"code","status":"missing"}
		]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
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
	semapsFile := filepath.Join(ws, "test.semaps")
	extractors := []extractorConf{{ID: "csharp", Language: "csharp", Project: "p"}}
	proj := project{File: semapsFile, Extractors: extractors}
	gs := newGraphService(proj, models)
	store := newRunStore(semapsFile)
	info := &runInfo{ID: "20260101-000000-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "done", Finished: time.Now()}
	if err := os.MkdirAll(filepath.Join(store.dir, info.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	facts := core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{
		{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"},
	}}
	b, _ := json.Marshal(facts)
	if err := os.WriteFile(store.path(info.ID, "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.save(info); err != nil {
		t.Fatal(err)
	}

	srv := graphServer(gs)
	defer srv.Close()

	// default: e_gone and r_gone are left out.
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (e_gone hidden), got %d: %+v", len(nodes), nodes)
	}
	stats := body["stats"].(map[string]any)
	hidden := stats["hiddenMissing"].(map[string]any)
	if hidden["nodes"].(float64) != 1 || hidden["edges"].(float64) != 1 {
		t.Fatalf("expected hiddenMissing {nodes:1,edges:1}, got %+v", hidden)
	}

	// missing=1: e_gone and r_gone come back.
	res2, body2 := getGraphJSON(t, srv.URL+"/api/graph/p?missing=1")
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res2.StatusCode, body2)
	}
	nodes2, _ := body2["nodes"].([]any)
	if len(nodes2) != 2 {
		t.Fatalf("expected 2 nodes with missing=1, got %d: %+v", len(nodes2), nodes2)
	}
	stats2 := body2["stats"].(map[string]any)
	hidden2 := stats2["hiddenMissing"].(map[string]any)
	if hidden2["nodes"].(float64) != 0 || hidden2["edges"].(float64) != 0 {
		t.Fatalf("expected hiddenMissing {nodes:0,edges:0} with missing=1, got %+v", hidden2)
	}
}

// TestGraphServiceNotifyPublishesDiff exercises step 5's wiring end to end:
// a run finishing calls the callback registered on the run store, which
// diffs against the previous graph and publishes to /api/events.
// liftFixture is a workspace with two types, A and B, A having two methods
// that each call one of B's methods, plus their `contains` edges — enough to
// exercise the default `lift=types` (ADR_20260928-3 §6) at the whole-graph
// level (which asking about a type also inherits).
func liftFixture(t *testing.T) *graphService {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for file, body := range map[string]string{
		"project.json":   `{"id":"p"}`,
		"entities.json":  `{"entities":[]}`,
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
	semapsFile := filepath.Join(ws, "test.semaps")
	proj := project{File: semapsFile, Extractors: []extractorConf{{ID: "csharp", Language: "csharp", Project: "p"}}}
	gs := newGraphService(proj, models)

	store := newRunStore(semapsFile)
	info := &runInfo{ID: "20260101-000000-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "done", Finished: time.Now()}
	if err := os.MkdirAll(filepath.Join(store.dir, info.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	facts := core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{
		{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"},
		{ID: "A.M1", Kind: "method", NativeKind: "method", Name: "M1", File: "A.cs"},
		{ID: "A.M2", Kind: "method", NativeKind: "method", Name: "M2", File: "A.cs"},
		{ID: "B", Kind: "type", NativeKind: "class", Name: "B", File: "B.cs"},
		{ID: "B.N1", Kind: "method", NativeKind: "method", Name: "N1", File: "B.cs"},
	}, Edges: []core.Edge{
		{From: "A", To: "A.M1", Kind: "contains"},
		{From: "A", To: "A.M2", Kind: "contains"},
		{From: "A.M1", To: "B.N1", Kind: "calls"},
		{From: "A.M2", To: "B.N1", Kind: "calls"},
		{From: "B", To: "B.N1", Kind: "contains"},
	}}
	b, _ := json.Marshal(facts)
	if err := os.WriteFile(store.path(info.ID, "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.save(info); err != nil {
		t.Fatal(err)
	}
	return gs
}

// TestGraphEndpointLiftDefaultAndNone: asking about a type shows the calls
// its methods make, lifted to the type it and the target belong to
// (ADR_20260928-3 §6); `lift=none` shows none of them (the raw graph has no
// type-to-type `calls` edge, only method-to-method ones, which `around` a
// type does not walk since `follow` matches Kind "calls" regardless of
// endpoint but the neighbourhood only includes nodes reached — the type node
// itself has no `calls` edge before lifting).
func TestGraphEndpointLiftDefaultAndNone(t *testing.T) {
	gs := liftFixture(t)
	srv := graphServer(gs)
	defer srv.Close()

	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:A&depth=1&format=json")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	edges, _ := body["edges"].([]any)
	found := false
	for _, e := range edges {
		em := e.(map[string]any)
		if em["kind"] == "calls" && em["from"] == "csharp:A" && em["to"] == "csharp:B" {
			found = true
			if em["count"].(float64) != 2 {
				t.Fatalf("expected count 2 on the lifted calls edge, got %+v", em)
			}
		}
	}
	if !found {
		t.Fatalf("expected a lifted calls edge csharp:A -> csharp:B by default, got %+v", edges)
	}

	res2, body2 := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:A&depth=1&format=json&lift=none")
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res2.StatusCode, body2)
	}
	edges2, _ := body2["edges"].([]any)
	for _, e := range edges2 {
		em := e.(map[string]any)
		if em["kind"] == "calls" && em["from"] == "csharp:A" {
			t.Fatalf("expected no calls edge from the type itself with lift=none, got %+v", em)
		}
	}
}

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
