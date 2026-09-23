# core

Language-neutral part of the `semaps` binary, Go package `semaps/core`.

| File | What | State |
|---|---|---|
| `check.go` | `semaps check`: stale translations, divergences, missing texts, views without an axis, containment contradictions, broken `codeRef`, placeholder names, undeclared relation types, a leftover `catalog.json` | works |
| `index.go` | `GET /api/workspace`: the projects and views found on disk ([ADR_20260923-7](../docs/adr/ADR_20260923-7_contract_projects-and-views-found-not-listed.md)) | works |
| sync | reconcile the registry with extractor facts ([`EXTRACTOR.md`](../docs/EXTRACTOR.md) §5) | [planned](../docs/plans/PLAN_20260923_core_sync-with-code.md) |

Nothing here knows about a particular workspace or source tree; both arrive as arguments.
`textFields` in `check.go` mirrors `TEXT_FIELDS` in `editor/src/model/text-provenance.ts` by hand.
