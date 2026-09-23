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

The map is the architecture as intended; `semaps check` shows where the model has rotted, and the
sync with code (in progress, see below) will show where the code has moved away from it.

## What exists today

| Part | State |
|---|---|
| Editor (`editor/`) | works: canvas, views, styles, content templates, texts with provenance |
| Host (`host/`) | works: one binary, `*.semaps` project files, save, source viewer |
| `semaps check` (`core/`) | works: stale texts, views without an axis, broken `codeRef`, … |
| Sync with code (`core/`) | **planned** — [`PLAN_20260923_core_sync-with-code`](docs/plans/PLAN_20260923_core_sync-with-code.md) |
| Extractor for C# (`extractors/csharp/`) | **planned** — [`PLAN_20260923_extractors_csharp`](docs/plans/PLAN_20260923_extractors_csharp.md) |
| JSON schemas (`schemas/`) | **planned**, with the extractor |

## Getting started

### 1. Install — once per machine

**Without Node or Go:** download `semaps.exe` from
[Releases](https://github.com/KOE73/SeMaps/releases) and double-click it. It asks whether to
install for the current user and then copies itself to `%LOCALAPPDATA%\Programs\SeMaps`, adds
that folder to your `PATH` and associates `*.semaps` files with itself. No admin rights. The same
from a console: `semaps.exe install`.

**From source** (needs Go 1.26+ and Node 24+, the editor is built and embedded):

```
git clone https://github.com/KOE73/SeMaps.git
SeMaps\host\install.cmd
```

This builds the editor and `semaps.exe`, then runs the same `install`.

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

### 4. Check — in CI or before a commit

```
semaps check
```

Same project lookup as above. Reports stale translations, texts that are missing, views without
an axis, one node in two containers on the same axis, broken `codeRef`. Exit code 1 when
anything is found, so a consuming project can run it in CI.

### Try it without a project

```
semaps SeMaps\examples\example.semaps
```

### Updating

A new exe from Releases, or `git pull` and `host\install.cmd` again — the editor is inside the
exe, so an old exe means an old editor.

## Layout

| Path | What |
|---|---|
| `editor/` | canvas editor, TypeScript (`@semaps/editor`) |
| `host/` | `semaps` binary, Go: serves the editor and a workspace |
| `core/` | language-neutral check and (planned) sync, Go, part of the `semaps` binary |
| `extractors/<lang>/` | one process per language, prints code facts as JSON (planned) |
| `schemas/` | JSON schemas: workspace contract, extractor facts (planned) |
| `docs/` | CONTRACT, API, EXTRACTOR, ADR, plans |
| `agents/` | rules for agents; documentation genres and naming |
| `examples/workspace/` | minimal workspace to open |

A consuming project keeps only its workspace (`docs/diagrams/`) — no Node, no Go.

The built editor (`host/app/`) is not in git: CI builds it and embeds it into the release
binaries ([`ADR_20260923-3`](docs/adr/ADR_20260923-3_build_bundle-from-ci-not-git.md)).
