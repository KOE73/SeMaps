// Package migrate rewrites a workspace to the current shape of contract v5 in
// place: from contract v3 (ADR_20260927-3: migrations live outside the loader;
// ADR_20260927-6: the v5 shapes) and, by content, from the earlier form of v5
// (ADR_20260930-4: code realizations; ADR_20260930-5: names of authored entities
// are texts). A step is a pure function over the JSON trees of a project: all new
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

// Extractor is an entry of the `extractors` list of the .semaps file: the
// project it writes into and its language. The language of the code realizations
// of a project is taken from here, never guessed.
type Extractor struct{ Project, Language string }

// Options of Workspace.
type Options struct {
	DryRun bool
	// DropUntypedStyles drops the styles of the workspace's styles.json that name
	// no type (`forKinds`) and are not a shipped default's id; the placements and
	// edges of the views that named them lose their styleId, and the report says
	// which.
	DropUntypedStyles bool
	// DefaultStyles is the tool's shipped styles.json. A workspace style of the same
	// id without `forKinds` is dropped: the default wins.
	DefaultStyles []byte
	// Extractors are the entries of the .semaps file, for the language of the
	// code realizations. Empty (no .semaps file): a project with a `symbol` in
	// its registry cannot be migrated — the language is not guessed.
	Extractors []Extractor
}

// Workspace migrates every project under <workspace>/projects and the
// workspace-level styles.json and canvas.json. What is already in the current
// shape is left alone: every step is decided by the content, so a second run
// changes nothing. With DryRun nothing is written but the report is full.
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
	defaults, err := defaultStyleIDs(opt.DefaultStyles)
	if err != nil {
		return nil, err
	}

	var projects []*projectMigration
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
		pm, err := prepareProject(workspace, e.Name(), styles, opt)
		if err != nil {
			return nil, err
		}
		projects = append(projects, pm)
	}
	// Workspace-level files after the projects: the projects above read the styles
	// as they were (a zone's colour style becomes an override from its old definition).
	wsOuts, dropped, err := migrateWorkspaceFiles(workspace, stylesFile, rep, defaults, opt)
	if err != nil {
		return nil, err
	}
	var outs []fileOut
	for _, pm := range projects {
		pm.dropStyleRefs(dropped)
		o, err := pm.collect()
		if err != nil {
			return nil, err
		}
		rep.Projects = append(rep.Projects, pm.rep)
		outs = append(outs, o...)
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
