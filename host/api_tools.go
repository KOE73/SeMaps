package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

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
	models    *modelService
	runs      *runStore
	// watch is nil for --workspace (no .semaps file, so no extractors and no
	// watchers) and in tests that do not exercise step 5. When set, an edit
	// to `extractors:` reloads it so a `watch` toggle takes effect at once,
	// with no host restart (PLAN_20260928-4_host_watch-sources.md step 5).
	watch    *watchManager
	settings *mcpSettingsBox // the `mcp:` section, live (PLAN_20260928-7 step 2)
	syncMu   sync.Mutex      // one sync at a time: both write the same registry
}

// onFinish, when not nil, is attached to the run store so something outside
// the tool API (the graph service, PLAN_20260928_host_graph-provider.md step
// 3/5) hears about a finished run without the tool API knowing it exists.
// watch, when not nil, is reloaded after every change to `extractors:`.
func registerToolAPI(mux *http.ServeMux, file, workspace string, models *modelService, onFinish func(*runInfo), watch *watchManager, settings *mcpSettingsBox) {
	api := &toolAPI{file: file, workspace: workspace, models: models, watch: watch, settings: settings}
	if file != "" {
		api.runs = newRunStore(file)
		api.runs.onFinish = onFinish
	}
	mux.HandleFunc("GET /api/tools", api.guard(api.tools))
	mux.HandleFunc("GET /api/setup", api.guard(api.getSetup))
	mux.HandleFunc("PUT /api/setup", api.guard(api.putSetup))
	mux.HandleFunc("PUT /api/setup/extractors/{id}", api.guard(api.putExtractor))
	mux.HandleFunc("DELETE /api/setup/extractors/{id}", api.guard(api.deleteExtractor))
	mux.HandleFunc("POST /api/setup/extractors/{id}/libraries", api.guard(api.listLibraries))
	mux.HandleFunc("GET /api/runs", api.guard(api.listRuns))
	mux.HandleFunc("POST /api/runs", api.guard(api.startRun))
	mux.HandleFunc("GET /api/runs/{id}", api.guard(api.getRun))
	mux.HandleFunc("GET /api/runs/{id}/log", api.guard(api.getLog))
	mux.HandleFunc("POST /api/runs/{id}/sync", api.guard(api.syncRun))
	mux.HandleFunc("GET /api/mcp", api.guard(api.getMCP))
	mux.HandleFunc("POST /api/mcp/install", api.guard(api.installMCP))
	mux.HandleFunc("POST /api/mcp/call", api.guard(api.callMCP))
}

// guard: a request that changes or starts something must come from a page
// this host served (sameOrigin); everything needs a .semaps file.
func (api *toolAPI) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !sameOrigin(r) {
			http.Error(w, "Cross-origin request refused", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("Authorization") != "Bearer "+api.models.key {
			http.Error(w, "Missing or invalid host key", http.StatusUnauthorized)
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
	ID            string        `json:"id"`
	Language      string        `json:"language"`
	Project       string        `json:"project"`
	Root          string        `json:"root"`
	Include       []string      `json:"include"`
	Exclude       []string      `json:"exclude"`
	Edges         []string      `json:"edges"`
	Watch         bool          `json:"watch"`
	External      string        `json:"external,omitempty"`
	Libraries     []string      `json:"libraries,omitempty"`
	ExternalTypes []string      `json:"externalTypes,omitempty"`
	ExternalDepth *int          `json:"externalDepth,omitempty"`
	Command       string        `json:"command,omitempty"` // shown, never written through the API
	Tool          extractorTool `json:"tool"`
	LastRun       *runInfo      `json:"lastRun,omitempty"`
}

func (api *toolAPI) extractorViews(list []extractorConf) []extractorView {
	out := []extractorView{}
	for _, e := range list {
		v := extractorView{
			External: e.External, Libraries: e.Libraries, ExternalTypes: e.ExternalTypes, ExternalDepth: e.ExternalDepth,
			ID: e.ID, Language: e.Language, Project: e.Project, Root: e.Root,
			Include: nonNil(e.Include), Exclude: nonNil(e.Exclude), Edges: nonNil(e.Edges), Watch: e.Watch, Command: e.Command,
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
		"mcp":         f.Mcp.withDefaults(),
	})
}

// putSetupBody is settingsPatch plus the optional `mcp` patch (step 1/2):
// kept as its own body type since mcp's keys are not settingsPatch's.
type putSetupBody struct {
	settingsPatch
	Mcp *mcpPatch `json:"mcp"`
}

func (api *toolAPI) putSetup(w http.ResponseWriter, r *http.Request) {
	var body putSetupBody
	if err := readJSON(r, &body); err != nil {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := patchSettings(api.file, body.settingsPatch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Mcp != nil {
		if err := patchMcp(api.file, *body.Mcp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Take effect at once (step 2): re-read the file and update the
		// shared box, so the HTTP graph endpoint and the MCP server both use
		// the new format/list_cap/limit from the next call on, no restart.
		if api.settings != nil {
			if f, err := readProjectFile(api.file); err == nil {
				api.settings.Set(f.Mcp)
			}
		}
	}
	api.getSetup(w, r)
}

func (api *toolAPI) putExtractor(w http.ResponseWriter, r *http.Request) {
	// `command` is not a field here on purpose: through HTTP only data changes,
	// never what gets executed (ADR_20260924-3 §5). An unknown field is refused.
	var body struct {
		Language      *string   `json:"language"`
		Project       *string   `json:"project"`
		Root          *string   `json:"root"`
		Include       *[]string `json:"include"`
		Exclude       *[]string `json:"exclude"`
		Edges         *[]string `json:"edges"`
		Watch         *bool     `json:"watch"`
		External      *string   `json:"external"`
		Libraries     *[]string `json:"libraries"`
		ExternalTypes *[]string `json:"externalTypes"`
		ExternalDepth *int      `json:"externalDepth"`
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
	patch := extractorPatch{Language: body.Language, Project: body.Project, Root: body.Root, Include: body.Include, Exclude: body.Exclude, Edges: body.Edges, Watch: body.Watch}
	patch.External, patch.Libraries, patch.ExternalTypes, patch.ExternalDepth = body.External, body.Libraries, body.ExternalTypes, body.ExternalDepth
	if err := patchExtractor(api.file, id, patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	api.reloadWatch()
	api.getSetup(w, r)
}

// Analysis uses the current form's paths without changing configuration or facts runs.
func (api *toolAPI) listLibraries(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Root    *string   `json:"root"`
		Include *[]string `json:"include"`
		Exclude *[]string `json:"exclude"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "Bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	proj, err := loadProject(api.file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entries, err := selectExtractors(proj, r.PathValue("id"))
	if err != nil || len(entries) != 1 {
		http.Error(w, "No such extractor", http.StatusNotFound)
		return
	}
	entry := entries[0]
	if entry.Language != "csharp" {
		http.Error(w, "Reference analysis requires a C# extractor", http.StatusBadRequest)
		return
	}
	entry.External, entry.Libraries, entry.ExternalTypes, entry.ExternalDepth, entry.Edges = "", nil, nil, nil, nil
	if body.Root != nil {
		entry.Root = *body.Root
	}
	if body.Include != nil {
		entry.Include = *body.Include
	}
	if body.Exclude != nil {
		entry.Exclude = *body.Exclude
	}
	if err := validateExtractors([]extractorConf{entry}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tool := resolveExtractor(entry)
	if !tool.ready() {
		http.Error(w, "C# extractor or runtime unavailable", http.StatusConflict)
		return
	}
	args := append(extractorArgs(tool, entry, proj.Root), "--list-libraries")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = proj.Root
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, "Could not analyze references: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	var result struct {
		Libraries *[]struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(output, &result); err != nil || result.Libraries == nil {
		http.Error(w, "Invalid reference catalog; update the C# extractor", http.StatusBadGateway)
		return
	}
	for _, library := range *result.Libraries {
		if library.ID == "" || library.Name == "" || library.Version == "" {
			http.Error(w, "Invalid reference catalog", http.StatusBadGateway)
			return
		}
	}
	writeJSON(w, result)
}

func (api *toolAPI) deleteExtractor(w http.ResponseWriter, r *http.Request) {
	if err := removeExtractor(api.file, r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	api.reloadWatch()
	api.getSetup(w, r)
}

// reloadWatch re-reads the .semaps file and rebuilds the watchers, so a
// `watch` toggle (or an added/removed/edited entry) takes effect at once.
func (api *toolAPI) reloadWatch() {
	if api.watch == nil {
		return
	}
	if proj, err := loadProject(api.file); err == nil {
		api.watch.Reload(proj)
	}
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
	info, _, err := api.runs.start(proj, list[0], nil, "")
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
	model, err := api.models.get(info.Project)
	if err != nil {
		modelError(w, err)
		return
	}
	before := model.Dirty()
	report, err := api.runs.syncRunModel(model, info, core.SyncOptions{DryRun: body.DryRun, NoRenames: body.NoRenames})
	if err != nil {
		status := http.StatusConflict
		var usage *core.UsageError
		if errors.As(err, &usage) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !body.DryRun {
		after := model.Dirty()
		changed := []core.Ref{}
		for _, ref := range after.Registry {
			found := false
			for _, old := range before.Registry {
				if old.Kind == ref.Kind && old.ID == ref.ID && old.Lang == ref.Lang && old.Author == ref.Author {
					found = true
					break
				}
			}
			if !found {
				changed = append(changed, ref)
			}
		}
		if len(changed) > 0 {
			api.models.publish(info.Project, modelEvent{Author: "sync", Changed: changed, Dirty: after})
		}
	}
	writeJSON(w, map[string]any{"report": report, "exitCode": report.ExitCode(), "empty": report.Empty()})
}
