// Package migrate rewrites a contract-v3 workspace to contract v5 in place
// (ADR_20260927-3: migrations live outside the loader; ADR_20260927-6: the v5
// shapes). A step is a pure function over the JSON trees of a project: all new
// file contents are computed in memory first and written only when the whole
// workspace converted without an error. The package is self-contained and
// imports only the standard library.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// timeNow is replaceable in tests.
var timeNow = time.Now

// Options of Workspace.
type Options struct{ DryRun bool }

// Workspace migrates every project under <workspace>/projects and the
// workspace-level styles.json and canvas.json. Projects already at contract v5
// are skipped; the workspace-level files are converted by content, so a second
// run changes nothing. With DryRun nothing is written but the report is full.
func Workspace(workspace string, opt Options) (*Report, error) {
	st, err := os.Stat(workspace)
	if err != nil {
		return nil, fmt.Errorf("рабочее пространство %s: %w", workspace, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("рабочее пространство %s: не каталог", workspace)
	}
	rep := &Report{DryRun: opt.DryRun}

	stylesFile, err := loadFile(workspace, "styles.json", false)
	if err != nil {
		return nil, err
	}
	var styles *styleLib
	if stylesFile != nil {
		arr, err := stylesFile.array("styles")
		if err != nil {
			return nil, err
		}
		styles = newStyleLib(arr)
	}

	var outs []fileOut
	entries, err := os.ReadDir(filepath.Join(workspace, "projects"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("projects: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(workspace, "projects", e.Name(), "project.json")); err != nil {
			continue
		}
		pr, o, err := migrateProject(workspace, e.Name(), styles)
		if err != nil {
			return nil, err
		}
		rep.Projects = append(rep.Projects, pr)
		outs = append(outs, o...)
	}
	// Workspace-level files last: the projects above read the styles as they were.
	wsOuts, err := migrateWorkspaceFiles(workspace, stylesFile, rep)
	if err != nil {
		return nil, err
	}
	outs = append(outs, wsOuts...)
	rep.changed = len(outs) > 0

	if opt.DryRun {
		return rep, nil
	}
	for _, o := range outs {
		if o.remove {
			if err := os.Remove(o.path); err != nil {
				return rep, fmt.Errorf("%s: %w", o.rel, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(o.path), 0o755); err != nil {
			return rep, fmt.Errorf("%s: %w", o.rel, err)
		}
		if err := os.WriteFile(o.path, o.data, 0o644); err != nil {
			return rep, fmt.Errorf("%s: %w", o.rel, err)
		}
	}
	return rep, nil
}
