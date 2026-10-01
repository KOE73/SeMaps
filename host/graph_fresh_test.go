package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"semaps/core"
)

// freshWorkspace: a workspace with one model project `p` of one entity.
func freshWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	for file, body := range map[string]string{
		"project.json":   `{"id":"p","contractVersion":5}`,
		"entities.json":  `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","status":"present","code":[{"lang":"csharp","ref":"A.cs","symbol":"A"}]}]}`,
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
	return ws
}

// addRun saves a run of extractor `ex` that finished at `finished`, which
// asked for edge kinds `asked`, its facts declaring `declared`; returns its id.
func addRun(t *testing.T, store *runStore, ex, state string, finished time.Time, asked, declared []string) string {
	t.Helper()
	id := finished.Format("20060102-150405") + "-" + ex
	if err := os.MkdirAll(filepath.Join(store.dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	info := &runInfo{ID: id, Extractor: ex, Project: "p", Language: "csharp", State: state, Edges: asked, Started: finished, Finished: finished}
	if state == "failed" {
		info.Error = "boom"
	} else {
		writeFacts(t, store.path(id, "facts.json"), core.Facts{Language: "csharp", Root: ".", EdgeKinds: declared, Symbols: []core.Symbol{{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"}}})
	}
	if err := store.save(info); err != nil {
		t.Fatal(err)
	}
	return id
}

func fixedNow(t *testing.T, now time.Time) {
	t.Helper()
	old := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = old })
}

// The header names the run and its age; a run whose facts lack an edge kind
// the entry asks for now gets a warning naming it; one that asked for it does not.
func TestFreshnessHeaderAndMissingEdgeKinds(t *testing.T) {
	now := time.Date(2026, 10, 1, 17, 26, 0, 0, time.Local)
	fixedNow(t, now)
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	store := newRunStore(semapsFile)
	finished := now.Add(-3 * time.Hour)
	addRun(t, store, "cs", "done", finished, nil, []string{"extends", "holds"})

	entry := extractorConf{ID: "cs", Language: "csharp", Project: "p", Edges: []string{"holds", "calls"}}
	proj := project{File: semapsFile, Extractors: []extractorConf{entry}}
	_, facts := sourcesFor(proj, "p")
	if len(facts) != 1 || facts[0].AgeSeconds != 3*3600 {
		t.Fatalf("facts = %+v", facts)
	}
	if len(facts[0].Warnings) != 1 || !strings.Contains(facts[0].Warnings[0], "calls") || !strings.Contains(facts[0].Warnings[0], "`extract`") || !strings.Contains(facts[0].Warnings[0], "`graph_status`") {
		t.Fatalf("warnings = %q", facts[0].Warnings)
	}
	text := freshnessText(facts)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if want := "facts: run cs " + finished.Local().Format("2006-01-02 15:04") + ", 3 h old"; lines[0] != want {
		t.Errorf("header = %q, want %q", lines[0], want)
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "warning: cs: facts lack edge kinds asked for in .semaps: calls") {
		t.Errorf("text = %q", text)
	}

	// A run that asked for calls (older runs: declared by the facts) is fine.
	if w := runWarnings(entry, &runInfo{Edges: []string{"holds", "calls"}}, nil, nil); len(w) != 0 {
		t.Errorf("run that asked for calls: warnings %q", w)
	}
	if w := runWarnings(entry, &runInfo{}, []string{"holds", "calls"}, nil); len(w) != 0 {
		t.Errorf("facts that declare calls: warnings %q", w)
	}
}

func TestFreshnessNeverRunAndFailedRun(t *testing.T) {
	now := time.Date(2026, 10, 1, 17, 26, 0, 0, time.UTC)
	fixedNow(t, now)
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	proj := project{File: semapsFile, Extractors: []extractorConf{{ID: "cs", Language: "csharp", Project: "p"}}}

	_, facts := sourcesFor(proj, "p")
	if len(facts) != 1 || facts[0].Run != "" || len(facts[0].Warnings) != 1 {
		t.Fatalf("never run: %+v", facts)
	}
	if text := freshnessText(facts); !strings.HasPrefix(text, "facts: no run of cs\nwarning: cs: no run yet") {
		t.Errorf("text = %q", text)
	}

	store := newRunStore(semapsFile)
	addRun(t, store, "cs", "done", now.Add(-time.Hour), nil, nil)
	failed := addRun(t, store, "cs", "failed", now.Add(-time.Minute), nil, nil)
	_, facts = sourcesFor(proj, "p")
	if len(facts) != 1 || facts[0].Run == "" || !facts[0].LastRunFailed {
		t.Fatalf("failed newest: %+v", facts)
	}
	if len(facts[0].Warnings) != 1 || !strings.Contains(facts[0].Warnings[0], failed) {
		t.Errorf("warnings = %q", facts[0].Warnings)
	}
}

// graphSession is an MCP session over workspace `ws`, served as the host
// serves it for the project file `proj`.
func graphSession(t *testing.T, ws string, proj project, watch *watchManager) *mcp.ClientSession {
	t.Helper()
	s := &mcpServer{proj: proj, workspace: ws, sourceRoot: ws, watch: watch}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, map[string]any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	if res.IsError {
		t.Fatalf("%s returned an error: %s", name, text.String())
	}
	out, _ := res.StructuredContent.(map[string]any)
	return text.String(), out
}

// get_graph carries the header and the warning in a text answer, and as
// fields in a json one; find_node carries the fields.
func TestGetGraphCarriesFreshness(t *testing.T) {
	ws := freshWorkspace(t)
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	store := newRunStore(semapsFile)
	addRun(t, store, "cs", "done", time.Now().Add(-2*time.Hour), nil, []string{"extends"})
	proj := project{File: semapsFile, Root: t.TempDir(), Extractors: []extractorConf{{ID: "cs", Language: "csharp", Project: "p", Edges: []string{"calls"}}}}
	cs := graphSession(t, ws, proj, nil)

	text, _ := callTool(t, cs, "get_graph", map[string]any{"around": "A"})
	lines := strings.Split(text, "\n")
	if !strings.HasPrefix(lines[0], "facts: run cs ") || !strings.HasSuffix(lines[0], ", 2 h old") {
		t.Fatalf("first line = %q in:\n%s", lines[0], text)
	}
	if !strings.HasPrefix(lines[1], "warning: cs: facts lack edge kinds asked for in .semaps: calls") {
		t.Fatalf("second line = %q in:\n%s", lines[1], text)
	}

	_, out := callTool(t, cs, "get_graph", map[string]any{"format": "json"})
	facts, _ := out["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("facts = %+v", out["facts"])
	}
	f := facts[0].(map[string]any)
	warns, _ := f["warnings"].([]any)
	if f["ageSeconds"].(float64) < 7000 || len(warns) != 1 {
		t.Errorf("json facts = %+v", f)
	}

	_, found := callTool(t, cs, "find_node", map[string]any{"q": "A"})
	if _, ok := found["facts"]; !ok {
		t.Errorf("find_node answer has no facts: %+v", found)
	}
}

func TestGraphEndpointsCarryFreshnessAndStatus(t *testing.T) {
	ws := freshWorkspace(t)
	models, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	store := newRunStore(semapsFile)
	addRun(t, store, "cs", "done", time.Now().Add(-time.Hour), nil, []string{"extends"})
	proj := project{File: semapsFile, Root: t.TempDir(), Extractors: []extractorConf{{ID: "cs", Language: "csharp", Project: "p", Edges: []string{"calls"}}}}
	g := newGraphService(proj, models, nil)
	mux := http.NewServeMux()
	g.register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(path string) string {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, res.StatusCode, b)
		}
		return string(b)
	}

	if text := get("/api/graph/p?format=facts"); !strings.HasPrefix(text, "facts: run cs ") || !strings.Contains(text, "\nwarning: cs: facts lack") {
		t.Errorf("text answer:\n%s", text)
	}
	var j struct {
		Facts []graphFactsInfo `json:"facts"`
	}
	if err := json.Unmarshal([]byte(get("/api/graph/p")), &j); err != nil || len(j.Facts) != 1 || len(j.Facts[0].Warnings) != 1 || j.Facts[0].AgeSeconds < 3500 {
		t.Errorf("json answer: %+v %v", j, err)
	}

	var st struct {
		Extractors []extractorStatus `json:"extractors"`
	}
	if err := json.Unmarshal([]byte(get("/api/graph/p/status")), &st); err != nil || len(st.Extractors) != 1 {
		t.Fatalf("status: %+v %v", st, err)
	}
	e := st.Extractors[0]
	if e.LastRun == nil || e.LastRun.State != "done" || e.FactsRun == "" || !slices.Equal(e.EdgesMissing, []string{"calls"}) || !slices.Equal(e.EdgesInFacts, []string{"extends"}) || e.Watching {
		t.Errorf("status = %+v", e)
	}
}

// graph_status shows a watcher that is on, and that a change is waiting for
// its quiet period to pass.
func TestGraphStatusShowsWatcher(t *testing.T) {
	ws := freshWorkspace(t)
	srcRoot := t.TempDir()
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	entry := extractorConf{ID: "cs", Language: "csharp", Project: "p", Watch: true}
	proj := project{File: semapsFile, Root: srcRoot, Extractors: []extractorConf{entry}}
	wm := newWatchManager(newRunStore(semapsFile))
	wm.quiet = time.Hour // the run never starts here: the change stays pending
	defer wm.Stop()
	wm.Reload(proj)
	time.Sleep(50 * time.Millisecond)

	if got := graphStatus(proj, "p", wm); len(got) != 1 || !got[0].Watching || got[0].RunPending || got[0].LastRun != nil {
		t.Fatalf("before a change: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "a.cs"), []byte("class A {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !graphStatus(proj, "p", wm)[0].RunPending {
		if time.Now().After(deadline) {
			t.Fatal("runPending never became true after a source change")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cs := graphSession(t, ws, proj, wm)
	_, out := callTool(t, cs, "graph_status", nil)
	ex, _ := out["extractors"].([]any)
	if len(ex) != 1 || ex[0].(map[string]any)["watching"] != true || ex[0].(map[string]any)["runPending"] != true {
		t.Errorf("graph_status = %+v", out)
	}
	// No watcher (a stdio server): the entry is configured but not watched.
	off := graphStatus(proj, "p", nil)
	if len(off) != 1 || off[0].Watching || !off[0].Watch {
		t.Errorf("without a manager: %+v", off)
	}
}

// Reload starts no run by itself: a host that starts, or a file that changes
// its extractor entries, leaves the runs as they are.
func TestReloadStartsNoRun(t *testing.T) {
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	runs := newRunStore(semapsFile)
	entry := extractorConf{ID: "cs", Language: "csharp", Project: "p", Watch: true}
	wm := newWatchManager(runs)
	wm.quiet = 10 * time.Millisecond
	defer wm.Stop()
	proj := project{File: semapsFile, Root: t.TempDir(), Extractors: []extractorConf{entry}}
	wm.Reload(proj)
	proj.Extractors[0].Edges = []string{"calls"}
	wm.Reload(proj)
	time.Sleep(200 * time.Millisecond)
	if n := len(runs.list("cs")); n != 0 {
		t.Fatalf("Reload started %d run(s)", n)
	}
}

// extract starts the runs and returns at once; with wait it returns when they
// have finished and says how each ended, a failure being data, not an error.
func TestExtractWait(t *testing.T) {
	ws := freshWorkspace(t)
	factsFile := filepath.Join(t.TempDir(), "facts.json")
	writeFacts(t, factsFile, core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"}}})
	cmd := fakeExtractorCommand(t, factsFile, false)
	semapsFile := filepath.Join(t.TempDir(), "t.semaps")
	proj := project{File: semapsFile, Root: t.TempDir(), Extractors: []extractorConf{{ID: "cs", Language: "csharp", Project: "p", Command: cmd, Edges: []string{"calls"}}}}
	cs := graphSession(t, ws, proj, nil)

	_, out := callTool(t, cs, "extract", map[string]any{"wait": true})
	runs, _ := out["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("extract = %+v", out)
	}
	r := runs[0].(map[string]any)
	if r["state"] != "done" || r["symbols"] != float64(1) || r["extractor"] != "cs" {
		t.Errorf("waited run = %+v", r)
	}
	info, err := newRunStore(semapsFile).get(r["run"].(string))
	if err != nil || !slices.Equal(info.Edges, []string{"calls"}) {
		t.Errorf("the run records the edges asked for: %+v %v", info, err)
	}

	// Without wait: the run ids at once; the run ends on its own.
	time.Sleep(1100 * time.Millisecond) // run ids carry the second: keep the two apart
	_, out = callTool(t, cs, "extract", nil)
	runs, _ = out["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["run"] == "" {
		t.Fatalf("extract without wait = %+v", out)
	}
	if st := runs[0].(map[string]any)["state"]; st != "running" && st != "done" {
		t.Errorf("state without wait = %v", st)
	}
	id := runs[0].(map[string]any)["run"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if i, err := newRunStore(semapsFile).get(id); err == nil && i.State != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A failing extractor: wait returns, saying failed.
	proj.Extractors[0].Command = fakeExtractorCommand(t, factsFile, true)
	cs = graphSession(t, ws, proj, nil)
	time.Sleep(1100 * time.Millisecond)
	_, out = callTool(t, cs, "extract", map[string]any{"wait": true})
	runs, _ = out["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["state"] != "failed" || runs[0].(map[string]any)["error"] == "" {
		t.Errorf("failed run = %+v", out)
	}
}

func TestRelevantChangeGo(t *testing.T) {
	for path, want := range map[string]bool{
		"a/b.go": true, "go.mod": true, "x/go.sum": true, "x/GO.MOD": true,
		"a/b.cs": false, "README.md": false, "x/go.mod.bak": false,
	} {
		if got := relevantChange("go", path); got != want {
			t.Errorf("relevantChange(go, %q) = %v, want %v", path, got, want)
		}
	}
}
