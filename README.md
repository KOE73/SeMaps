# SeMaps

Semantic architecture maps: one model, several views, each view declares an **axis** —
the question it answers. Layout is manual only; there is no auto-layout.

- The machine checks mechanical sync with code: does the entity exist, does `codeRef` resolve,
  does a relation point anywhere.
- Meaning (containers, axes, texts, views) stays with the human and the agent.

## Two-way work with agents

The map is shared ground between people and coding agents, and it works in both directions:

- **Agent → map.** Following the rules in [`docs/CONTRACT.md`](docs/CONTRACT.md), an agent records
  what it built or learned: entities, relations, descriptions, which container a thing belongs to.
  Views and layout stay human; the agent writes registries and texts, never geometry.
- **Map → agent.** Before changing code, an agent reads the map to learn the intended
  architecture: which part owns what, which interactions are allowed, what a relation means. Its
  work then conforms to the design instead of drifting from it.

The map is the architecture as intended; the sync check shows where the code has moved away from it.

## Layout

| Path | What |
|---|---|
| `editor/` | canvas editor, TypeScript (`@semaps/editor`) |
| `host/` | `semaps` binary, Go: serves the editor and a workspace |
| `core/` | language-neutral sync/verify, Go, part of the `semaps` binary |
| `extractors/<lang>/` | one process per language, prints code facts as JSON |
| `schemas/` | JSON schemas: workspace contract, extractor facts |
| `docs/` | CONTRACT, API, EXTRACTOR, ADR |
| `agents/` | rules for agents; documentation genres and naming |
| `examples/workspace/` | minimal workspace to open |

A consuming project keeps only its workspace (`docs/diagrams/`) — no Node, no Go.

