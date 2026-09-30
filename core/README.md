# core

Language-neutral part of the `semaps` binary, Go package `semaps/core`.

| File | What | State |
|---|---|---|
| `check.go` | `semaps check`: stale translations, divergences, missing texts, views without an axis, containment contradictions, broken `code[].ref`, malformed `code[]`/`evidence[]`, an authored entity with no name text, placeholder names, undeclared relation types, a leftover `catalog.json` | works |
| `index.go` | `GET /api/workspace`: the projects and views found on disk ([ADR_20260923-7](../docs/adr/ADR_20260923-7_contract_projects-and-views-found-not-listed.md)) | works |
| `facts.go` | read and validate extractor facts ([`EXTRACTOR.md`](../docs/EXTRACTOR.md) §2): required fields, `kind` vocabularies, unique ids, edges between printed symbols, sort order | works |
| `sync.go` | `semaps sync`: reconcile one project's `entities.json` / `relations.json` / `relation-types.json` with facts ([`EXTRACTOR.md`](../docs/EXTRACTOR.md) §5, [ADR_20260923-9](../docs/adr/ADR_20260923-9_core_sync-symbol-mapping-and-containment.md)); matches by the `symbol` of the entity's `code[]` entry of the facts' language (one realization per entity per run, [ADR_20260930-4](../docs/adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md)), adopts hand-made entities on the first run | works; running the extractor itself and `/api/sync` — [planned](../docs/plans/PLAN_20260923_core_sync-with-code.md) |
| `realize.go` | the shape of code realizations: read and write an entity's `code[]` and a relation's `evidence[]` (with the member signature `via` inside) as ordered objects, name the earlier shape, check an entry ([ADR_20260930-4](../docs/adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md)) | works |
| `names.go` | the name an entity is shown by: an authored entity's `name` text (main language, else another, else the id), any other entity's `name` in `entities.json` ([ADR_20260930-5](../docs/adr/ADR_20260930-5_contract_authored-entity-name-is-text.md)) | works |
| `migrate/` | `semaps migrate`: contract 3 → 5 and the earlier form of 5 → the current shape; the language of realizations from the `.semaps` extractors, `--drop-untyped-styles` ([ADR_20260927-3](../docs/adr/ADR_20260927-3_core_migrations-outside-the-loader.md)) | works |

Nothing here knows about a particular workspace or source tree; both arrive as arguments.
Sync rewrites registry files through an ordered JSON object: keys it does not own survive, in
their order; a run that changes nothing writes nothing.
`textFields` in `check.go` mirrors `TEXT_FIELDS` in `editor/src/model/text-provenance.ts` by hand.
