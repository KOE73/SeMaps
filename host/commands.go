package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"semaps/core"
)

// selectExtractors: the one named, or all of them.
func selectExtractors(proj project, id string) ([]extractorConf, error) {
	if len(proj.Extractors) == 0 {
		return nil, fmt.Errorf("%s lists no extractors (see README: `extractors:`)", proj.File)
	}
	if id == "" {
		return proj.Extractors, nil
	}
	for _, e := range proj.Extractors {
		if e.ID == id {
			return []extractorConf{e}, nil
		}
	}
	return nil, fmt.Errorf("no extractor %q in %s", id, proj.File)
}

// runDoctor prints what semaps.exe finds for the project: its extractors and
// their runtimes. Exit code 1 when an extractor of the project cannot run.
func runDoctor(w io.Writer, proj project) int {
	fmt.Fprintf(w, "semaps:     %s\nextractors: %s\n\n", selfPath(), toolsDir())
	if len(proj.Extractors) == 0 {
		fmt.Fprintf(w, "%s lists no extractors.\n", proj.File)
		return 0
	}
	code := 0
	for _, e := range proj.Extractors {
		t := resolveExtractor(e)
		fmt.Fprintf(w, "%s (%s → project %s)\n", e.ID, e.Language, e.Project)
		if t.Found {
			fmt.Fprintf(w, "  extractor: %s [%s]\n", t.Where, t.Source)
		} else {
			fmt.Fprintf(w, "  extractor: НЕТ — %s\n", t.Problem)
			code = 1
		}
		if r := t.Runtime; r != nil {
			if r.OK {
				fmt.Fprintf(w, "  runtime:   %s %s\n", r.Name, r.Version)
			} else {
				fmt.Fprintf(w, "  runtime:   НЕТ %s %s — %s\n", r.Name, r.Version, r.Hint)
				code = 1
			}
		}
	}
	return code
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "?"
	}
	return p
}

// runExtract runs the chosen extractors one after another, the log on the
// console. The runs stay for `semaps sync --run`.
func runExtract(proj project, id string) ([]*runInfo, int) {
	list, err := selectExtractors(proj, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semaps extract: %v\n", err)
		return nil, 2
	}
	store := newRunStore(proj.File)
	var runs []*runInfo
	code := 0
	for _, e := range list {
		fmt.Printf("== %s (%s)\n", e.ID, e.Language)
		info, done, err := store.start(proj, e, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "semaps extract: %s: %v\n", e.ID, err)
			code = 1
			continue
		}
		<-done
		info, _ = store.get(info.ID)
		if info.State != "done" {
			code = 1
			continue
		}
		fmt.Printf("   run %s\n", info.ID)
		runs = append(runs, info)
	}
	return runs, code
}

// runSyncProject: sync without --facts. With --run, the facts of that run;
// otherwise the extractors run now and their facts are used at once.
func runSyncProject(proj project, workspace, id, runID string, opt core.SyncOptions) int {
	store := newRunStore(proj.File)
	var runs []*runInfo
	if runID != "" {
		info, err := store.get(runID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "semaps sync: run %s: %v\n", runID, err)
			return 2
		}
		runs = []*runInfo{info}
	} else {
		var code int
		runs, code = runExtract(proj, id)
		if code == 2 {
			return 2
		}
		if code != 0 {
			fmt.Fprintln(os.Stderr, "semaps sync: an extractor failed; nothing reconciled")
			return 1
		}
	}
	exit := 0
	for _, info := range runs {
		fmt.Printf("\n== sync %s → project %s\n", info.Extractor, info.Project)
		report, err := store.syncRun(workspace, info, opt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "semaps sync: %v\n", err)
			var usage *core.UsageError
			if errors.As(err, &usage) {
				return 2
			}
			exit = 1
			continue
		}
		report.Print(os.Stdout)
		exit = max(exit, report.ExitCode())
	}
	return exit
}
