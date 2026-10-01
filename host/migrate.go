package main

import (
	"fmt"
	"io"
	"os"

	"semaps/core/migrate"
)

// runMigrate is `semaps migrate`: the workspace of a project of an older
// contract — or of the earlier form of the current one — is rewritten to the
// current shape, in place (ADR_20260927-3, docs/ADOPTING.md). The report says
// what was done and what only a human can decide. Exit code 0 when done, 1 on an
// error; with --dry-run, 1 when anything would change (as `semaps sync --dry-run`).
//
// The language of code realizations comes from the extractors of the .semaps
// file (extractors), the ids of the shipped styles from the tool's own
// styles.json, the relation types the dictionary already describes from its
// kinds.json; dropUntyped is --drop-untyped-styles.
func runMigrate(w io.Writer, workspace string, dryRun, dropUntyped bool, extractors []extractorConf) int {
	opt := migrate.Options{DryRun: dryRun, DropUntypedStyles: dropUntyped, DefaultKinds: defaultKinds()}
	if b, err := bundled.ReadFile("defaults/styles.json"); err == nil {
		opt.DefaultStyles = b
	}
	for _, e := range extractors {
		opt.Extractors = append(opt.Extractors, migrate.Extractor{Project: e.Project, Language: e.Language})
	}
	rep, err := migrate.Workspace(workspace, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semaps migrate: %v\n", err)
		return 1
	}
	rep.Print(w)
	if dryRun {
		fmt.Fprintln(w, "\n--dry-run: ничего не записано.")
		if rep.Changed() {
			return 1
		}
	}
	return 0
}
