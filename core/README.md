# core

Language-neutral part of the `semaps` binary, Go package `semaps/core`.

| File | What | State |
|---|---|---|
| `check.go` | `semaps check`: stale translations, divergences, missing texts, views without an axis, containment contradictions, broken `codeRef`, placeholder names, undeclared relation types, a leftover `catalog.json` | works |
| `index.go` | `GET /api/workspace`: the projects and views found on disk ([ADR_20260923-7](../docs/adr/ADR_20260923-7_contract_projects-and-views-found-not-listed.md)) | works |
| `facts.go` | read and validate extractor facts ([`EXTRACTOR.md`](../docs/EXTRACTOR.md) §2): required fields, `kind` vocabularies, unique ids, edges between printed symbols, sort order | works |
| `sync.go` | `semaps sync`: reconcile one project's `entities.json` / `relations.json` / `relation-types.json` with facts ([`EXTRACTOR.md`](../docs/EXTRACTOR.md) §5, [ADR_20260923-9](../docs/adr/ADR_20260923-9_core_sync-symbol-mapping-and-containment.md)); matches by the entity's `symbol`, adopts hand-made entities on the first run | works; running the extractor itself and `/api/sync` — [planned](../docs/plans/PLAN_20260923_core_sync-with-code.md) |

Nothing here knows about a particular workspace or source tree; both arrive as arguments.
Sync rewrites registry files through an ordered JSON object: keys it does not own survive, in
their order; a run that changes nothing writes nothing.
`textFields` in `check.go` mirrors `TEXT_FIELDS` in `editor/src/model/text-provenance.ts` by hand.
