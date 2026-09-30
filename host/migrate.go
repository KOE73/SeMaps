package main

import (
	"fmt"
	"io"
	"os"

	"semaps/core/migrate"
)

// runMigrate is `semaps migrate`: the workspace of a project of an older
// contract is rewritten to the current one, in place (ADR_20260927-3,
// docs/ADOPTING.md). The report says what was done and what only a human can
// decide. Exit code 0 when done, 1 on an error; with --dry-run, 1 when
// anything would change (as `semaps sync --dry-run`).
func runMigrate(w io.Writer, workspace string, dryRun bool) int {
	rep, err := migrate.Workspace(workspace, migrate.Options{DryRun: dryRun})
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
