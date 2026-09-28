package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"semaps/core"
)

// fakeExtractorBinary builds host/testdata/fakeextractor once per test run
// (testdata is never part of the semaps module build, docs/go tool
// convention) and returns its path: the smallest possible stand-in for a
// real extractor (EXTRACTOR.md §1) for tests that need runStore.start to
// actually run something, ignoring the --root/--include/--exclude/--edges
// runs.go always appends.
var fakeExtractorOnce = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "semaps-fakeextractor")
	if err != nil {
		return "", err
	}
	exe := filepath.Join(dir, "fakeextractor")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, "./testdata/fakeextractor")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build fakeextractor: %v: %s", err, out)
	}
	return exe, nil
})

// fakeExtractorCommand: an extractorConf.Command that, when run, prints the
// content of `factsFile` to stdout and exits 0 — or, when `fail` is true,
// writes to stderr and exits 1.
func fakeExtractorCommand(t *testing.T, factsFile string, fail bool) string {
	t.Helper()
	exe, err := fakeExtractorOnce()
	if err != nil {
		t.Skipf("cannot build the fake extractor: %v", err)
	}
	t.Setenv("SEMAPS_FAKE_FACTS", factsFile)
	if fail {
		t.Setenv("SEMAPS_FAKE_FAIL", "1")
	} else {
		t.Setenv("SEMAPS_FAKE_FAIL", "")
	}
	return `"` + exe + `"`
}

func writeFacts(t *testing.T, path string, f core.Facts) {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPruneKeepsPersonAndWatchRunsSeparately: step 2 — a watcher running on
// its own must never push out the runs a person or an agent started, so the
// two are counted apart even though they share one extractor.
func TestPruneKeepsPersonAndWatchRunsSeparately(t *testing.T) {
	store := &runStore{dir: t.TempDir()}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(i int, trigger string) {
		id := base.Add(time.Duration(i)*time.Minute).Format("20060102-150405") + "-e"
		info := &runInfo{ID: id, Extractor: "e", State: "done", Trigger: trigger, Started: base.Add(time.Duration(i) * time.Minute)}
		if err := os.MkdirAll(filepath.Join(store.dir, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.save(info); err != nil {
			t.Fatal(err)
		}
	}
	// 6 person runs and 4 watch runs, interleaved, oldest first.
	for i := 0; i < 6; i++ {
		add(i*2, "")
	}
	for i := 0; i < 4; i++ {
		add(i*2+1, "watch")
	}
	store.prune(5, 2)

	left := store.list("")
	if len(left) != 7 {
		t.Fatalf("expected 5 person + 2 watch = 7 runs left, got %d: %+v", len(left), left)
	}
	var person, watch int
	for _, r := range left {
		if r.Trigger == "watch" {
			watch++
		} else {
			person++
		}
	}
	if person != 5 || watch != 2 {
		t.Fatalf("expected 5 person and 2 watch runs, got %d person, %d watch", person, watch)
	}
}

// TestGraphFactsReportsLastRunAndFailure: step 2 — a failed run leaves the
// graph untouched, but GET /api/graph's `facts` still says the newest run of
// that extractor failed, apart from which run the graph's facts came from.
func TestGraphFactsReportsLastRunAndFailure(t *testing.T) {
	semapsFile := filepath.Join(t.TempDir(), "test.semaps")
	store := newRunStore(semapsFile)

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := &runInfo{ID: "20260101-000000-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "done", Started: baseTime, Finished: baseTime}
	if err := os.MkdirAll(filepath.Join(store.dir, older.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFacts(t, store.path(older.ID, "facts.json"), core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"}}})
	if err := store.save(older); err != nil {
		t.Fatal(err)
	}

	newer := &runInfo{ID: "20260101-000100-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "failed", Started: baseTime.Add(time.Minute), Finished: baseTime.Add(time.Minute), Error: "boom"}
	if err := os.MkdirAll(filepath.Join(store.dir, newer.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.save(newer); err != nil {
		t.Fatal(err)
	}

	proj := project{File: semapsFile, Extractors: []extractorConf{{ID: "csharp", Language: "csharp", Project: "p"}}}
	sources, facts := sourcesFor(proj, "p")

	if len(sources) != 1 {
		t.Fatalf("expected the graph to still use the older done run's facts, got %d sources", len(sources))
	}
	if len(facts) != 1 {
		t.Fatalf("expected one facts entry, got %+v", facts)
	}
	f := facts[0]
	if f.Run != older.ID {
		t.Errorf("expected facts to come from the older done run %s, got %s", older.ID, f.Run)
	}
	if f.LastRun != newer.ID {
		t.Errorf("expected lastRun to be the newest run %s, got %s", newer.ID, f.LastRun)
	}
	if !f.LastRunFailed {
		t.Error("expected lastRunFailed to be true: the newest run failed")
	}
}

// TestWatchRunEndsInGraphEvent covers steps 1, 3 and 4 together: a real
// change under a watched entry's root starts a run of the fake extractor
// (trigger "watch"); when it finishes, the diff against the previous graph
// reaches /api/events, and the model on disk is untouched — a watch run
// never syncs (host/watch.go's launch(), ADR_20260928 §4).
func TestWatchRunEndsInGraphEvent(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	entitiesFile := filepath.Join(dir, "entities.json")
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
	before, err := os.ReadFile(entitiesFile)
	if err != nil {
		t.Fatal(err)
	}
	models, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}

	srcRoot := t.TempDir()
	factsFile := filepath.Join(t.TempDir(), "facts.json")
	writeFacts(t, factsFile, core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{
		{ID: "A", Kind: "type", NativeKind: "class", Name: "A", File: "A.cs"},
		{ID: "B", Kind: "type", NativeKind: "class", Name: "B", File: "B.cs"},
	}})
	cmd := fakeExtractorCommand(t, factsFile, false)

	entry := extractorConf{ID: "csharp", Language: "csharp", Project: "p", Watch: true, Command: cmd}
	semapsFile := filepath.Join(t.TempDir(), "test.semaps")
	proj := project{File: semapsFile, Root: srcRoot, Extractors: []extractorConf{entry}}

	gs := newGraphService(proj, models)
	gs.prev["p"] = &core.Graph{Nodes: []core.GraphNode{{ID: "csharp:A", Presence: "both"}}}

	runs := newRunStore(semapsFile)
	runs.onFinish = gs.notifyRunFinished

	wm := newWatchManager(runs)
	wm.quiet = 20 * time.Millisecond
	defer wm.Stop()
	wm.Reload(proj)
	time.Sleep(50 * time.Millisecond) // let the watch of srcRoot start before the write below

	mux := http.NewServeMux()
	models.register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	events := make(chan string, 1)
	req, _ := http.NewRequest("GET", srv.URL+"/api/events?project=p", nil)
	res, err := http.DefaultClient.Do(req)
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

	if err := os.WriteFile(filepath.Join(srcRoot, "a.cs"), []byte("class A {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		if !contains(ev, `"graph"`) {
			t.Fatalf("expected a graph diff in the SSE event, got %s", ev)
		}
	case <-time.After(5 * time.Second):
		var got []runInfo
		for _, r := range runs.list("") {
			got = append(got, *r)
		}
		t.Fatalf("no SSE event received after the watched file changed; runs: %+v", got)
	}

	after, err := os.ReadFile(entitiesFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("a watch run must never sync: entities.json changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestWatchCoalescesChangesDuringARun: while a run is going, further changes
// collect into exactly one following run — never one run per change.
func TestWatchCoalescesChangesDuringARun(t *testing.T) {
	srcRoot := t.TempDir()
	factsFile := filepath.Join(t.TempDir(), "facts.json")
	writeFacts(t, factsFile, core.Facts{Language: "csharp", Root: "."})
	cmd := fakeExtractorCommand(t, factsFile, false)

	semapsFile := filepath.Join(t.TempDir(), "test.semaps")
	entry := extractorConf{ID: "csharp", Language: "csharp", Project: "p", Watch: true, Command: cmd}
	proj := project{File: semapsFile, Root: srcRoot, Extractors: []extractorConf{entry}}

	runs := newRunStore(semapsFile)
	wm := newWatchManager(runs)
	wm.quiet = 10 * time.Millisecond
	defer wm.Stop()
	wm.Reload(proj)
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(srcRoot, "a.cs"), []byte("class A {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Give the quiet period, one run, and a possible chained run time to
	// finish, then check exactly one or two runs happened — never five.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(runs.list("csharp")) > 0 {
			allDone := true
			for _, r := range runs.list("csharp") {
				if r.State == "running" {
					allDone = false
				}
			}
			if allDone {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // settle any chained run
	if n := len(runs.list("csharp")); n == 0 || n > 2 {
		t.Fatalf("expected 1-2 runs from a burst of 5 changes, got %d", n)
	}
}
