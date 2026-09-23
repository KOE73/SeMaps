# SeMaps

Semantic architecture maps: one model, several views, each view declares an **axis** —
the question it answers. Layout is manual only; there is no auto-layout.

- The machine checks mechanical sync with code: does the entity exist, does `codeRef` resolve,
  does a relation point anywhere.
- Meaning (containers, axes, texts, views) stays with the human and the agent.

## Layout

| Path | What |
|---|---|
| `editor/` | canvas editor, TypeScript (`@semaps/editor`) |
| `host/` | `semaps` binary, Go: serves the editor and a workspace |
| `core/` | language-neutral sync/verify, Go, part of the `semaps` binary |
| `extractors/<lang>/` | one process per language, prints code facts as JSON |
| `schemas/` | JSON schemas: workspace contract, extractor facts |
| `docs/` | CONTRACT, API, EXTRACTOR, ADR |
| `examples/workspace/` | minimal workspace to open |

A consuming project keeps only its workspace (`docs/diagrams/`) — no Node, no Go.

Origin: `tools/spla-diagram` and `tools/spla-atlas` from SPLA.
