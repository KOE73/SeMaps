// Stage 1 of docs/plans/PLAN_20260928-4_host_watch-sources.md: the host
// watches an extractor's sources and starts a run on its own when they
// change. Stage 2 (a `--watch` mode inside the extractor process) is not
// this file's job; here the extractor stays the one-shot process
// EXTRACTOR.md §1 describes, just started more often.
package main

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// defaultQuietPeriod: a change opens a quiet period; each further change
// restarts it; a run starts only once it elapses undisturbed (step 1).
const defaultQuietPeriod = 1500 * time.Millisecond

// alwaysSkipDirs are never watched, whatever the language or the entry's own
// `exclude` says.
var alwaysSkipDirs = map[string]bool{
	".git": true, ".semaps": true, "bin": true, "obj": true,
	"node_modules": true, "dist": true, ".vs": true, ".idea": true,
}

// watchExtensions: which file extensions of a change can matter to a
// language's extractor. A language not listed here means "any file", so an
// out-of-tree `command:` extractor is still watched usefully.
var watchExtensions = map[string]map[string]bool{
	"csharp":     extSet(".cs", ".csproj", ".sln", ".props", ".targets"),
	"typescript": extSet(".ts", ".tsx", ".mts", ".cts", ".js", ".jsx"),
	// Entries may also be whole file names (go.mod, go.sum): relevantChange
	// tries the base name as well as the extension.
	"go": extSet(".go", "go.mod", "go.sum"),
}

func extSet(exts ...string) map[string]bool {
	m := map[string]bool{}
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// relevantChange decides whether a changed path can matter to `language`'s
// extractor. package.json and tsconfig*.json matter to typescript beyond its
// extension list because they change what gets compiled, not a symbol.
func relevantChange(language, path string) bool {
	exts, known := watchExtensions[language]
	if !known {
		return true
	}
	base := filepath.Base(path)
	if exts[strings.ToLower(filepath.Ext(base))] || exts[strings.ToLower(base)] {
		return true
	}
	if language == "typescript" {
		if strings.EqualFold(base, "package.json") {
			return true
		}
		if ok, _ := filepath.Match("tsconfig*.json", base); ok {
			return true
		}
	}
	return false
}

// excluded: `dir` (an absolute, cleaned path) is skipped from watching,
// either because a path component is one of alwaysSkipDirs, because it is
// inside the model workspace (the host owns that tree, ADR_20260925), or
// because it matches one of the entry's own `exclude` globs.
func excluded(dir, root, workspace string, globs []string) bool {
	if workspace != "" && within(workspace, dir) {
		return true
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		rel = dir
	}
	rel = filepath.ToSlash(rel)
	for _, part := range strings.Split(rel, "/") {
		if alwaysSkipDirs[part] {
			return true
		}
	}
	for _, g := range globs {
		if matchExcludeGlob(g, rel) {
			return true
		}
	}
	return false
}

// matchExcludeGlob matches an entry's `exclude` glob (EXTRACTOR.md §1) against
// a slash-separated path relative to the extractor's root: the whole glob
// against the whole relative path, or, when the glob has no `/`, against any
// one path segment — so `bin` in `exclude:` skips a `bin` folder wherever it
// sits, the common case, without needing `**/bin`. `**/` in front and `/**`
// behind (`**/*.Tests/**`) mean "at any depth" and "with everything inside":
// filepath.Match knows no `**`, so they are taken off and the rest is tried
// against every run of segments of the path.
func matchExcludeGlob(glob, relPath string) bool {
	if ok, _ := filepath.Match(glob, relPath); ok {
		return true
	}
	if inner := strings.TrimSuffix(strings.TrimPrefix(glob, "**/"), "/**"); inner != glob && inner != "" {
		parts := strings.Split(relPath, "/")
		n := strings.Count(inner, "/") + 1
		anywhere := strings.HasPrefix(glob, "**/")
		for i := 0; i+n <= len(parts); i++ {
			if ok, _ := filepath.Match(inner, strings.Join(parts[i:i+n], "/")); ok {
				return true
			}
			if !anywhere {
				break
			}
		}
		return false
	}
	if !strings.Contains(glob, "/") {
		for _, part := range strings.Split(relPath, "/") {
			if ok, _ := filepath.Match(glob, part); ok {
				return true
			}
		}
	}
	return false
}

func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// watchRoots: the directories an entry's watcher must add — its `root`
// joined with each `include`, or just `root` when there is none.
func watchRoots(proj project, e extractorConf) []string {
	root := filepath.Join(proj.Root, filepath.FromSlash(e.Root))
	if len(e.Include) == 0 {
		return []string{root}
	}
	roots := make([]string, 0, len(e.Include))
	for _, inc := range e.Include {
		roots = append(roots, filepath.Join(root, filepath.FromSlash(inc)))
	}
	return roots
}

// fileWatcher watches one extractor entry's sources and starts runs of it
// through runStore.start, coalescing changes into a quiet period so a burst
// of edits (a save-all, a branch switch) produces one run, not many.
type fileWatcher struct {
	id    string
	proj  project
	entry extractorConf
	runs  *runStore
	quiet time.Duration

	root  string
	globs []string

	fsw *fsnotify.Watcher

	mu      sync.Mutex
	timer   *time.Timer
	armed   bool // the quiet-period timer is set: a watch run is about to start
	running bool
	pending bool
	stopped bool
	wg      sync.WaitGroup
}

// newFileWatcher starts watching `entry`'s tree at once. The returned
// watcher must be stopped with stop() when it is no longer wanted.
func newFileWatcher(proj project, entry extractorConf, runs *runStore, quiet time.Duration) (*fileWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", entry.ID, err)
	}
	if quiet <= 0 {
		quiet = defaultQuietPeriod
	}
	w := &fileWatcher{
		id: entry.ID, proj: proj, entry: entry, runs: runs, quiet: quiet,
		root: filepath.Join(proj.Root, filepath.FromSlash(entry.Root)), globs: entry.Exclude,
		fsw: fsw,
	}
	for _, r := range watchRoots(proj, entry) {
		w.addTree(r)
	}
	w.wg.Add(1)
	go w.loop()
	return w, nil
}

// addTree adds `dir` and every non-excluded subdirectory under it.
// fsnotify does not watch recursively by itself on any of its backends
// (github.com/fsnotify/fsnotify README, "Do I need to add each new
// directory..."), including ReadDirectoryChangesW on Windows: every
// directory is added one by one here, and a directory created later is
// picked up from the Create event in loop().
func (w *fileWatcher) addTree(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip it, do not fail the whole watch
		}
		if !d.IsDir() {
			return nil
		}
		clean := filepath.Clean(path)
		if clean != filepath.Clean(w.root) && excluded(clean, w.root, w.proj.Workspace, w.globs) {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(clean); err != nil {
			log.Printf("watch %s: %v", w.id, err)
		}
		return nil
	})
}

func (w *fileWatcher) loop() {
	defer w.wg.Done()
	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watch %s: %v", w.id, err)
		}
	}
}

func (w *fileWatcher) handle(ev fsnotify.Event) {
	clean := filepath.Clean(ev.Name)
	if excluded(filepath.Dir(clean), w.root, w.proj.Workspace, w.globs) || excluded(clean, w.root, w.proj.Workspace, w.globs) {
		return
	}
	if ev.Has(fsnotify.Create) {
		// A new directory needs its own watch before its own files can be
		// seen; harmless (and cheap) to try on a file too.
		w.addTree(clean)
	}
	if !relevantChange(w.entry.Language, clean) {
		return
	}
	w.scheduleRun()
}

// scheduleRun (re)starts the quiet-period timer. Each call while it is
// pending pushes the run further out — a burst of edits fires it once.
func (w *fileWatcher) scheduleRun() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.armed = true
	if w.timer == nil {
		w.timer = time.AfterFunc(w.quiet, w.fire)
	} else {
		w.timer.Reset(w.quiet)
	}
}

// fire runs once the quiet period elapses. While a run of this extractor is
// already going, the change is remembered instead: exactly one more run
// follows the one in flight, never a run per change.
func (w *fileWatcher) fire() {
	w.mu.Lock()
	w.armed = false
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if w.running {
		w.pending = true
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()
	w.launch()
}

// launch starts one run of the entry. A watch run only refreshes the live
// graph (runStore.onFinish -> graphService.notifyRunFinished): it never
// syncs and never touches the model — sync is a person's or an agent's own
// action (plan step 4, ADR_20260928 §4 "не автоматически").
func (w *fileWatcher) launch() {
	_, done, err := w.runs.start(w.proj, w.entry, nil, "watch")
	if err != nil {
		log.Printf("watch %s: %v", w.id, err)
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		return
	}
	go func() {
		<-done
		w.mu.Lock()
		w.running = false
		again := w.pending
		w.pending = false
		stopped := w.stopped
		w.mu.Unlock()
		if again && !stopped {
			w.fire()
		}
	}()
}

// stop closes the underlying watcher and waits for its goroutine to exit. A
// run already started is left to finish; it just will not chain into another.
func (w *fileWatcher) stop() {
	w.mu.Lock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	_ = w.fsw.Close()
	w.wg.Wait()
}

// watchManager owns the watchers of every extractor entry with `watch: true`
// (step 5). It exists once per host process; server.go starts it with the
// host and stops it on shutdown, and the tool API calls Reload after any
// change to the .semaps file so a toggle takes effect at once, no restart.
type watchManager struct {
	runs  *runStore
	quiet time.Duration

	mu       sync.Mutex
	watchers map[string]*fileWatcher
}

func newWatchManager(runs *runStore) *watchManager {
	return &watchManager{runs: runs, quiet: defaultQuietPeriod, watchers: map[string]*fileWatcher{}}
}

// Reload rebuilds the set of watchers from `proj`'s current extractors: one
// per entry with `watch: true`, none for --workspace (proj.File == "", the
// caller simply never constructs a manager then) or a project with no such
// entry. Cheap enough to call on every settings change instead of diffing.
// It starts no run: a watcher sees only changes made while it runs, and what
// happened before is for graph_status and the `extract` tool to show and fix.
func (m *watchManager) Reload(proj project) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.watchers {
		w.stop()
		delete(m.watchers, id)
	}
	for _, e := range proj.Extractors {
		if !e.Watch {
			continue
		}
		w, err := newFileWatcher(proj, e, m.runs, m.quiet)
		if err != nil {
			log.Printf("watch: %s not started: %v", e.ID, err)
			continue
		}
		m.watchers[e.ID] = w
	}
}

// watchState is what graph_status says of one entry's watcher.
type watchState struct {
	// Watching: a watcher is active for the entry (`watch: true` and started).
	Watching bool `json:"watching"`
	// RunPending: a change was seen and a watch run has not started yet (the
	// quiet period is running, or one more run waits for the run in flight).
	RunPending bool `json:"runPending"`
}

// state reports the watcher of entry `id`; the zero value when there is none.
func (m *watchManager) state(id string) watchState {
	if m == nil {
		return watchState{}
	}
	m.mu.Lock()
	w := m.watchers[id]
	m.mu.Unlock()
	if w == nil {
		return watchState{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return watchState{Watching: true, RunPending: w.armed || w.pending}
}

// Stop tears down every watcher; called once, on host shutdown.
func (m *watchManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.watchers {
		w.stop()
		delete(m.watchers, id)
	}
}
