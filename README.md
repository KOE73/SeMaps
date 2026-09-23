# SeMaps

![SeMaps — semantic architecture maps, shared ground for humans and agents](docs/images/promo.jpg)

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

## Getting started

You need Go (1.26+) once, to install. Node is **not** needed — the editor is prebuilt and embedded.

### 1. Install — once per machine

```
git clone https://github.com/KOE73/SeMaps.git
SeMaps\host\install.cmd
```

This does two things:

- builds `semaps.exe` and puts it into `%USERPROFILE%\go\bin` (Go puts that folder on `PATH`);
- associates `*.semaps` files with it for the current user (no admin rights needed).

Check: open a new console and type `semaps --help`.

### 2. Add a project file — once per project

In the **root of your project** (next to `.git`) create `<name>.semaps`, e.g. `myproject.semaps`:

```yaml
version: 1
name: My project
workspace: docs/diagrams   # where the maps live (catalog.json, projects/)
source_root: .             # where the code lives, for codeRef
port: 8777
```

All paths are relative to the folder of this file. All keys are optional — the values above are
the defaults. Commit the file: everyone who clones the project gets the same environment.

The workspace folder needs at least `catalog.json`; copy
[`examples/workspace`](examples/workspace) as a start.

### 3. Open — every day

Any of these:

- **Enter** on `myproject.semaps` in Far / Total Commander, or a **double click** in Explorer;
- type **`semaps`** in a console anywhere inside the project — it walks up to the `*.semaps` file;
- `semaps path\to\myproject.semaps` from anywhere.

The server starts in **its own console window** and your prompt / file manager is free at once;
the browser opens on the editor. To stop, close that window (or Ctrl+C in it). Opening the same
project again does not start a second server — it just brings up the browser.
Edits are saved straight into the workspace folder — review and commit them like code.

### Try it without a project

```
semaps SeMaps\examples\example.semaps
```

### Updating

`git pull`, then run `host\install.cmd` again — the editor is inside the exe, so an old exe means
an old editor.

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

