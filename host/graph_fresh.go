// Freshness of the live graph's facts. The graph is joined from the latest
// successful run of each extractor, and such a run can be old, or made before
// the .semaps file asked for more (`edges: [..., calls]`). An answer that said
// nothing about it would let an agent conclude "nobody calls this" from facts
// that never had calls. So every graph answer carries which run(s) it comes
// from and how old they are, and, where a comparison of plain facts shows the
// facts lack something, a warning (docs/API.md §5). Nothing here guesses: a
// run's age is a time, its edge kinds a list, compared with the file's list.
package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// nowFunc is the clock of the age shown in an answer; tests replace it.
var nowFunc = time.Now

// edgeKindsOf: the edge kinds a run's facts cover — those the run asked the
// extractor for and those the facts declare in their own header (`declared`).
func edgeKindsOf(run *runInfo, declared []string) []string {
	out := slices.Clone(declared)
	for _, k := range run.Edges {
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// missingEdgeKinds: kinds of `asked` that `have` lacks.
func missingEdgeKinds(asked, have []string) []string {
	var out []string
	for _, k := range asked {
		if !slices.Contains(have, k) {
			out = append(out, k)
		}
	}
	return out
}

// runWarnings: what is known to be wrong with the facts of `run` for the
// entry `e` as the .semaps file has it now. `declared` is the edgeKinds the
// facts themselves declare; `newest` the newest run of the entry in any state.
func runWarnings(e extractorConf, run *runInfo, declared []string, newest *runInfo) []string {
	var out []string
	if newest != nil && newest.State == "failed" {
		out = append(out, "the newest run failed ("+newest.ID+"), these facts are from an earlier one; see `graph_status`")
	}
	if miss := missingEdgeKinds(e.Edges, edgeKindsOf(run, declared)); len(miss) > 0 {
		out = append(out, "facts lack edge kinds asked for in .semaps: "+strings.Join(miss, ", ")+" (no "+missingMeaning(miss)+" in this answer); run `extract`, see `graph_status`")
	}
	return out
}

// missingMeaning words what absent edge kinds mean for a reader, so that
// "nobody calls this" is not concluded from facts that never had calls.
func missingMeaning(kinds []string) string {
	names := map[string]string{"calls": "called-by/calls", "holds": "held-by/holds", "uses": "used-by/uses", "injects": "injected-into/injects"}
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = k
		if n, ok := names[k]; ok {
			parts[i] = n
		}
	}
	return strings.Join(parts, ", ")
}

// humanAge: "under 1 min", "12 min", "3 h", "2 d".
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under 1 min"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d d", int(d.Hours()/24))
}

// freshnessText is the top of a text graph answer: one line saying which
// runs the facts come from and how old they are, then one line per warning.
// Empty when no extractor feeds the project (a model-only graph says nothing
// about facts).
func freshnessText(facts []graphFactsInfo) string {
	if len(facts) == 0 {
		return ""
	}
	var runs, warns []string
	for _, f := range facts {
		if f.Run == "" {
			runs = append(runs, "no run of "+f.Extractor)
		} else {
			fin, _ := time.Parse(time.RFC3339, f.Finished)
			runs = append(runs, fmt.Sprintf("run %s %s, %s old", f.Extractor, fin.Local().Format("2006-01-02 15:04"), humanAge(time.Duration(f.AgeSeconds)*time.Second)))
		}
		for _, w := range f.Warnings {
			warns = append(warns, "warning: "+f.Extractor+": "+w)
		}
	}
	return strings.Join(append([]string{"facts: " + strings.Join(runs, "; ")}, warns...), "\n") + "\n"
}

// withFreshness puts the freshness lines before a text answer.
func withFreshness(body []byte, facts []graphFactsInfo) []byte {
	head := freshnessText(facts)
	if head == "" {
		return body
	}
	return append([]byte(head), body...)
}
