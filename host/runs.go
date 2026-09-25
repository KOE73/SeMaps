package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"semaps/core"
)

// A run is one call of one extractor: a folder in the temp directory with the
// facts it printed, its log and a run.json. Sync applies the facts of a run,
// so what a dry run showed is what gets written (ADR_20260924-3 §3). Facts
// never land in the workspace.

type runStats struct {
	Symbols     int            `json:"symbols"`
	Edges       int            `json:"edges"`
	SymbolKinds map[string]int `json:"symbolKinds"`
	EdgeKinds   map[string]int `json:"edgeKinds"`
	Language    string         `json:"language"`
}

type runInfo struct {
	ID        string    `json:"id"`
	Extractor string    `json:"extractor"` // entry id in .semaps
	Project   string    `json:"project"`   // model project the facts are for
	Language  string    `json:"language"`
	Command   []string  `json:"command,omitempty"`
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished,omitzero"`
	Seconds   float64   `json:"seconds,omitempty"`
	State     string    `json:"state"` // running | done | failed
	ExitCode  int       `json:"exitCode"`
	Error     string    `json:"error,omitempty"`
	Stats     *runStats `json:"stats,omitempty"`
}

// runStore keeps the runs of one project file.
type runStore struct {
	dir string
	mu  sync.Mutex
}

func newRunStore(projectFile string) *runStore {
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(projectFile))))
	return &runStore{dir: filepath.Join(os.TempDir(), "semaps-runs", hex.EncodeToString(sum[:6]))}
}

func (s *runStore) path(id string, name string) string { return filepath.Join(s.dir, id, name) }

// validRunID keeps a run id from walking out of the store.
func validRunID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r == '-') {
			return false
		}
	}
	return true
}

func (s *runStore) save(info *runInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(s.path(info.ID, "run.json"), data, 0o644)
}

func (s *runStore) get(id string) (*runInfo, error) {
	if !validRunID(id) {
		return nil, errors.New("bad run id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id, "run.json"))
	if err != nil {
		return nil, err
	}
	var info runInfo
	return &info, json.Unmarshal(data, &info)
}

// list: newest first; only runs of `extractor` when it is not empty.
func (s *runStore) list(extractor string) []*runInfo {
	entries, _ := os.ReadDir(s.dir)
	var out []*runInfo
	for _, e := range entries {
		if info, err := s.get(e.Name()); err == nil && (extractor == "" || info.Extractor == extractor) {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

// prune keeps the newest `keep` runs of each extractor.
func (s *runStore) prune(keep int) {
	count := map[string]int{}
	for _, r := range s.list("") {
		count[r.Extractor]++
		if count[r.Extractor] > keep && r.State != "running" {
			os.RemoveAll(filepath.Join(s.dir, r.ID))
		}
	}
}

// start creates a run and launches the extractor. The returned channel closes
// when it is finished; `echo`, when set, also receives the log.
func (s *runStore) start(proj project, e extractorConf, echo io.Writer) (*runInfo, <-chan struct{}, error) {
	tool := resolveExtractor(e)
	if !tool.Found {
		return nil, nil, errors.New(tool.Problem)
	}
	if !tool.ready() {
		return nil, nil, fmt.Errorf("%s for %s: not found; %s", tool.Runtime.Name, e.ID, tool.Runtime.Hint)
	}
	args := extractorArgs(tool, e, proj.Root)
	now := time.Now()
	info := &runInfo{
		ID:        now.Format("20060102-150405") + "-" + strings.ToLower(e.ID),
		Extractor: e.ID, Project: e.Project, Language: e.Language,
		Command: args, Started: now, State: "running",
	}
	if !validRunID(info.ID) {
		info.ID = now.Format("20060102-150405-000000000")
	}
	if err := os.MkdirAll(filepath.Join(s.dir, info.ID), 0o755); err != nil {
		return nil, nil, err
	}
	factsFile, err := os.Create(s.path(info.ID, "facts.json"))
	if err != nil {
		return nil, nil, err
	}
	logFile, err := os.Create(s.path(info.ID, "log.txt"))
	if err != nil {
		factsFile.Close()
		return nil, nil, err
	}
	var logOut io.Writer = logFile
	if echo != nil {
		logOut = io.MultiWriter(logFile, echo)
	}
	fmt.Fprintf(logOut, "> %s\n", strings.Join(args, " "))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = proj.Root
	cmd.Stdout = factsFile
	cmd.Stderr = logOut
	if err := s.save(info); err != nil {
		factsFile.Close()
		logFile.Close()
		return nil, nil, err
	}
	done := make(chan struct{})
	finish := func(runErr error) {
		factsFile.Close()
		info.Finished = time.Now()
		info.Seconds = info.Finished.Sub(info.Started).Round(10 * time.Millisecond).Seconds()
		info.State = "done"
		if runErr != nil {
			info.State, info.Error, info.ExitCode = "failed", runErr.Error(), -1
			var exit *exec.ExitError
			if errors.As(runErr, &exit) {
				info.ExitCode = exit.ExitCode()
			}
		} else if stats, err := readStats(s.path(info.ID, "facts.json")); err != nil {
			info.State, info.Error = "failed", "facts: "+err.Error()
		} else {
			info.Stats = stats
		}
		if info.Error != "" {
			fmt.Fprintf(logOut, "! %s\n", info.Error)
		} else {
			fmt.Fprintf(logOut, "= %d symbols, %d edges in %.1fs\n", info.Stats.Symbols, info.Stats.Edges, info.Seconds)
		}
		logFile.Close()
		s.save(info)
		close(done)
		s.prune(5)
	}
	if err := cmd.Start(); err != nil {
		finish(err)
		return info, done, nil
	}
	go func() { finish(cmd.Wait()) }()
	return info, done, nil
}

func readFacts(file string) (*core.Facts, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return core.ReadFacts(f)
}

func readStats(file string) (*runStats, error) {
	facts, err := readFacts(file)
	if err != nil {
		return nil, err
	}
	st := &runStats{Language: facts.Language, SymbolKinds: map[string]int{}, EdgeKinds: map[string]int{}}
	for _, s := range facts.Symbols {
		st.Symbols++
		st.SymbolKinds[s.NativeKind]++
	}
	for _, e := range facts.Edges {
		st.Edges++
		st.EdgeKinds[e.Kind]++
	}
	return st, nil
}

// syncRun reconciles the model project of a finished run with its facts.
func (s *runStore) syncRunModel(model *core.Model, info *runInfo, opt core.SyncOptions) (*core.SyncReport, error) {
	if info.State != "done" {
		return nil, fmt.Errorf("run %s is %s", info.ID, info.State)
	}
	facts, err := readFacts(s.path(info.ID, "facts.json"))
	if err != nil {
		return nil, err
	}
	opt.Project = info.Project
	return core.Sync(model, facts, opt)
}

func (s *runStore) syncRun(workspace string, info *runInfo, opt core.SyncOptions) (*core.SyncReport, error) {
	m, err := core.LoadModel(workspace, info.Project)
	if err != nil {
		return nil, err
	}
	rep, err := s.syncRunModel(m, info, opt)
	if err != nil {
		return rep, err
	}
	if !opt.DryRun && len(rep.Written) > 0 {
		err = m.Save()
	}
	return rep, err
}
