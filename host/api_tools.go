package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"semaps/core"
)

// The tool API (docs/API.md §5, ADR_20260924-3 §3–§6): the .semaps settings,
// the extractors, their runs and the sync of a run. Everything is said in
// paths relative to the project root; no absolute path of this machine goes
// out or comes in. Every handler passes guard — the one place where "may
// this request do that" is decided, and where authentication would go.

type toolAPI struct {
	file      string // the .semaps file; empty when the host was started with --workspace
	workspace string
	runs      *runStore
	syncMu    sync.Mutex // one sync at a time: both write the same registry
}

func registerToolAPI(mux *http.ServeMux, file, workspace string) {
	api := &toolAPI{file: file, workspace: workspace}
	if file != "" {
		api.runs = newRunStore(file)
	}
	mux.HandleFunc("GET /api/tools", api.guard(api.tools))
	mux.HandleFunc("GET /api/setup", api.guard(api.getSetup))
	mux.HandleFunc("PUT /api/setup", api.guard(api.putSetup))
	mux.HandleFunc("PUT /api/setup/extractors/{id}", api.guard(api.putExtractor))
	mux.HandleFunc("DELETE /api/setup/extractors/{id}", api.guard(api.deleteExtractor))
	mux.HandleFunc("GET /api/runs", api.guard(api.listRuns))
	mux.HandleFunc("POST /api/runs", api.guard(api.startRun))
	mux.HandleFunc("GET /api/runs/{id}", api.guard(api.getRun))
	mux.HandleFunc("GET /api/runs/{id}/log", api.guard(api.getLog))
	mux.HandleFunc("POST /api/runs/{id}/sync", api.guard(api.syncRun))
	mux.HandleFunc("GET /api/mcp", api.guard(api.getMCP))
	mux.HandleFunc("POST /api/mcp/install", api.guard(api.installMCP))
}

// guard: a request that changes or starts something must come from a page
// this host served (sameOrigin); everything needs a .semaps file.
func (api *toolAPI) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !sameOrigin(r) {
			http.Error(w, "Cross-origin request refused", http.StatusForbidden)
			return
		}
		if api.file == "" {
			http.Error(w, "Started with --workspace: no .semaps file, no extractors", http.StatusConflict)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

type extractorView struct {
	ID       string        `json:"id"`
	Language string        `json:"language"`
	Project  string        `json:"project"`
	Root     string        `json:"root"`
	Include  []string      `json:"include"`
	Exclude  []string      `json:"exclude"`
	Edges    []string      `json:"edges"`
	Command  string        `json:"command,omitempty"` // shown, never written through the API
	Tool     extractorTool `json:"tool"`
	LastRun  *runInfo      `json:"lastRun,omitempty"`
}

func (api *toolAPI) extractorViews(list []extractorConf) []extractorView {
	out := []extractorView{}
	for _, e := range list {
		v := extractorView{
			ID: e.ID, Language: e.Language, Project: e.Project, Root: e.Root,
			Include: nonNil(e.Include), Exclude: nonNil(e.Exclude), Edges: nonNil(e.Edges), Command: e.Command,
			Tool: resolveExtractor(e),
		}
		// Absolute paths of this machine stay here (ADR_20260924-3 §6).
		if v.Tool.Source != "command" {
			v.Tool.Where = filepath.Base(v.Tool.Where)
		}
		if runs := api.runs.list(e.ID); len(runs) > 0 {
			v.LastRun = publicRun(runs[0])
		}
		out = append(out, v)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// publicRun: a run without the command line, which carries absolute paths.
func publicRun(r *runInfo) *runInfo {
	c := *r
	c.Command = nil
	return &c
}

func (api *toolAPI) tools(w http.ResponseWriter, r *http.Request) {
	f, err := readProjectFile(api.file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var runtimes []*runtimeInfo
	for _, l := range knownLanguages {
		runtimes = append(runtimes, runtimeFor(l))
	}
	writeJSON(w, map[string]any{
		"projectFile": filepath.Base(api.file),
		"shipped":     shippedLanguages(),
		"runtimes":    runtimes,
		"extractors":  api.extractorViews(f.Extractors),
	})
}

// shippedLanguages: the extractors found beside semaps.exe.
func shippedLanguages() []string {
	out := []string{}
	for _, l := range knownLanguages {
		if _, _, ok := shippedExtractor(l); ok {
			out = append(out, l)
		}
	}
	return out
}

func (api *toolAPI) getSetup(w http.ResponseWriter, r *http.Request) {
	f, err := readProjectFile(api.file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"projectFile": filepath.Base(api.file),
		"name":        f.Name,
		"workspace":   f.Workspace,
		"sourceRoot":  f.SourceRoot,
		"port":        f.Port,
		"extractors":  api.extractorViews(f.Extractors),
		"languages":   knownLanguages,
	})
}

func (api *toolAPI) putSetup(w http.ResponseWriter, r *http.Request) {
	var patch settingsPatch
	if err := readJSON(r, &patch); err != nil {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := patchSettings(api.file, patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	api.getSetup(w, r)
}

func (api *toolAPI) putExtractor(w http.ResponseWriter, r *http.Request) {
	// `command` is not a field here on purpose: through HTTP only data changes,
	// never what gets executed (ADR_20260924-3 §5). An unknown field is refused.
	var body struct {
		Language *string   `json:"language"`
		Project  *string   `json:"project"`
		Root     *string   `json:"root"`
		Include  *[]string `json:"include"`
		Exclude  *[]string `json:"exclude"`
		Edges    *[]string `json:"edges"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if !validRunID(id) {
		http.Error(w, "id: lowercase letters, digits and `-`", http.StatusBadRequest)
		return
	}
	patch := extractorPatch{Language: body.Language, Project: body.Project, Root: body.Root, Include: body.Include, Exclude: body.Exclude, Edges: body.Edges}
	if err := patchExtractor(api.file, id, patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	api.getSetup(w, r)
}

func (api *toolAPI) deleteExtractor(w http.ResponseWriter, r *http.Request) {
	if err := removeExtractor(api.file, r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	api.getSetup(w, r)
}

func (api *toolAPI) listRuns(w http.ResponseWriter, r *http.Request) {
	out := []*runInfo{}
	for _, run := range api.runs.list(r.URL.Query().Get("extractor")) {
		out = append(out, publicRun(run))
	}
	writeJSON(w, out)
}

func (api *toolAPI) startRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Extractor string `json:"extractor"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	proj, err := loadProject(api.file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	list, err := selectExtractors(proj, body.Extractor)
	if err != nil || body.Extractor == "" {
		http.Error(w, fmt.Sprintf("extractor %q: not in the .semaps file", body.Extractor), http.StatusBadRequest)
		return
	}
	info, _, err := api.runs.start(proj, list[0], nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, publicRun(info))
}

func (api *toolAPI) run(w http.ResponseWriter, r *http.Request) (*runInfo, bool) {
	info, err := api.runs.get(r.PathValue("id"))
	if err != nil {
		http.Error(w, "No such run", http.StatusNotFound)
		return nil, false
	}
	return info, true
}

func (api *toolAPI) getRun(w http.ResponseWriter, r *http.Request) {
	if info, ok := api.run(w, r); ok {
		writeJSON(w, publicRun(info))
	}
}

// getLog returns the log from `offset` on, and the run's state: the page asks
// again until the state is not `running`. Plain polling, so any proxy passes it.
func (api *toolAPI) getLog(w http.ResponseWriter, r *http.Request) {
	info, ok := api.run(w, r)
	if !ok {
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	f, err := os.Open(api.runs.path(info.ID, "log.txt"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	if _, err := f.Seek(max(offset, 0), io.SeekStart); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	chunk, _ := io.ReadAll(io.LimitReader(f, 256<<10))
	writeJSON(w, map[string]any{
		"text":   string(chunk),
		"offset": max(offset, 0) + int64(len(chunk)),
		"state":  info.State,
	})
}

func (api *toolAPI) syncRun(w http.ResponseWriter, r *http.Request) {
	info, ok := api.run(w, r)
	if !ok {
		return
	}
	var body struct {
		DryRun    bool `json:"dryRun"`
		NoRenames bool `json:"noRenames"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	api.syncMu.Lock()
	defer api.syncMu.Unlock()
	report, err := api.runs.syncRun(api.workspace, info, core.SyncOptions{DryRun: body.DryRun, NoRenames: body.NoRenames})
	if err != nil {
		status := http.StatusConflict
		var usage *core.UsageError
		if errors.As(err, &usage) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, map[string]any{"report": report, "exitCode": report.ExitCode(), "empty": report.Empty()})
}
