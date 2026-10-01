# SeMaps

**English** · [Русский](README.ru.md)

![SeMaps — semantic architecture maps, shared ground for humans and agents](docs/images/promo.jpg)

Semantic architecture maps for humans and agents: one model, many views, reconciliation with the
code, and an MCP server through which an agent reads and edits the architecture the way you do.

![SeMaps at a glance: the human works through the editor, the agent through MCP, both on one map; below, the code the extractors read](docs/images/semaps-glance.png)

The human works on the map in the editor, the agent through MCP, and they share one map. Under it
lies the live code: extractors read it and build the code graph, which is reconciled with the map.

## Why SeMaps

- **Architecture the agent reads too.** Connect `semaps mcp`, and before touching code the agent
  learns from the map what lives where and how it fits together. About the code it asks the graph
  instead of searching text: faster, and many times fewer tokens. After the work it records on the
  map what it built.
- **The agent draws views with you.** On your request it lays out blocks, shows the relations that
  matter and looks at the result as a picture, not blindly.
- **The map does not lie about the code.** Extractors for C#, TypeScript and Go read the sources;
  `semaps check` in CI catches where the map drifted from the code.
- **A live code graph.** All the code as one graph that rebuilds itself while you write code.
  Pull the nodes you need onto a view.
- **Human and agent in one model at the same time.** Changes show in every window at once, and the
  «Changes» panel shows who changed what before it is saved.
- **One model, many views.** Each view has its own axis — the question it answers. A relation is
  made once and appears wherever both its ends are.
- **An editor for architects.** Drag to connect, lines route around blocks by themselves, and the
  layout stays yours.

### Architecture the agent reads too

The map is shared ground for humans and agents, and it works both ways.

- **Map → agent.** Before changing code, the agent reads the map and learns the intended
  architecture: which part owns what, which interactions are allowed, what a relation means. Its
  work follows the design instead of drifting away from it.
- **Agent → map.** The agent records what it built or learned: entities, relations, descriptions,
  membership in containers. It writes only through the MCP tools, and the host checks every edit by
  the same rules as a human's. The agent never edits the workspace files by hand; their format is in
  [`docs/CONTRACT.md`](docs/CONTRACT.md).
- **The graph instead of text search.** Who extends a type, who holds, calls or constructs it,
  what a namespace contains — the agent asks the code graph and gets the answer with files and lines
  in one call. No round of grep and reading whole files: faster, and many times fewer tokens. Only
  the lines the graph names need opening.

All of it goes through `semaps mcp` — an MCP server that plugs into Claude Code and other clients
with one entry in `.mcp.json`. The editor has an **MCP** mode: what the server offers, how to
connect it, and a sandbox to call any tool and see the request and the answer.

### The agent draws views with you

The agent gets a view as a tree of containers and blocks. It can move and align blocks, put them
into containers, show and hide relations by whole types. It changes the layout only when a human
asks. So as not to work blindly, it renders any view of the project as a picture (`render_view`)
and sees what you will see.

### The map does not lie about the code

Extractors read the sources and report facts: types, interfaces, inheritance, who holds whom and
through which member, with cardinality and the place in code. `semaps sync` turns the facts into
the model's registry, and `semaps check` verifies that the map's links into the code are alive,
texts are not stale and every view has an axis. Exit code 1 on a finding, so it fits into CI.

Only unambiguous facts enter the graph: SeMaps does not guess. An extractor for your language is a
small separate program, and a PR with one is welcome (see below).

### A live code graph

The graph page (`/app/#graph`) shows all the code at once. The layout is automatic; nodes are
grouped by code, by model or by structure and coloured by group. The host watches the sources: save
a file and the graph updates. For an agent the graph answers questions like "what derives from this
all the way down", "who holds this", "who calls this and where". Graph nodes can be copied and
pasted onto a view: that is a quick way from code to an architecture picture.

> The code graph has been proven in real work on C# only. Extractors for TypeScript and Go exist,
> but the graph on them has not been tried on real projects yet.

**A measurement: the graph against text search.** A C# project (NeuroModFlowNet.ONNX, 623 files),
four questions about the code: the subclasses of a base class two levels down, who holds a context
object and through which member, where it is constructed, who calls its methods. Two identical
agents (Claude Sonnet): one answered only with grep and file reads, the other through the SeMaps
code graph.

| | Text search | Code graph |
|---|---|---|
| agent tokens | 96.5k | 70.6k (−27%) |
| tool calls | 14 | 1 |
| time | 51 s | 33 s |
| completeness | everything found; some callers by inference | all but one of nine construction sites; callers from facts |

The agent tokens include its fixed part — the system prompt and tool descriptions — which is the
same for both, so the difference in the work itself is larger. It is one run on one project, not a
study. And the graph helps only while its facts are fresh: in a first run the latest extractor run
predated call extraction, and the graph could not answer two of the questions. Keep `watch: true` on
the extractor, or run extraction before the work.

### Human and agent in one model

The editor and the agents work on one working model in the host. What the model accepted shows in
every open window at once and survives a restart. The **«Changes»** panel shows what is unsaved and
who made it, the human or an agent. **Save** writes the files into the workspace; from there they
are reviewed and committed like code.

A mistaken hand-made record — an entity or a relation — can be removed: by a human from the editor,
by an agent through MCP when a human asks. A removal waits for Save too, so changing your mind is
still possible. Records that came from sync with the code are not removed this way: the code
decides their fate.

### One model, many views

Entities and relations live in the project's registry, and a view is a look at them along its own
axis: "what it consists of", "who calls whom", "where data is kept". A relation is made once. Each
view decides whether to show it, and it appears wherever both its ends are.

For example, the picture at the top is one of SeMaps' own views ([`docs/diagrams/`](docs/diagrams/)).
Two more sit on the same registry. The overall architecture: clients, host, core, storage,
extractors and code.

![SeMaps architecture overview](docs/images/semaps-overview.png)

The inside: what is in git, what is on disk outside git, what lives only in the host's memory, and
where MCP takes its data from.

![Inside SeMaps: storage and data](docs/images/semaps-internals.png)

### An editor for architects

- Hover a block: an arrow appears under it. Drag it onto another block and pick the relation type.
  Or select several blocks and choose **Create relation** from the context menu.
- Lines route around blocks by themselves, keep off their edges and run through the free space
  between containers. The route search itself picks the side of a block a line comes to, and
  straight lines sit in the middle of the shared span.
- Routing can be tuned to taste: the **«Line routing»** panel has sliders for clearances, halos and
  bend costs. The view redraws at once, and the zones show both on the canvas and on a small
  to-scale preview.
- While dragging, only the lines of the moved blocks are routed again, so even a big view moves
  smoothly.
- Layout is manual only; nothing is rearranged automatically: the view stays the way you designed
  it. Algorithmic layout exists only on the code graph page, on purpose
  ([`ADR_20260928-2`](docs/adr/ADR_20260928-2_editor_graph-is-a-separate-form.md)).

What changed in each version is in [CHANGELOG.md](CHANGELOG.md) (in Russian).

## What exists today

| Part | Status |
|---|---|
| Editor (`editor/`) | works: canvas, views, styles, content templates, texts with provenance, hand-made relations |
| Host (`host/`) | works: one binary, `*.semaps` project files, a shared working model with a journal, saving, source viewer |
| MCP server (`semaps mcp`) | works: reading and writing the model, view tools, questions to the code graph |
| `semaps check` (`core/`) | works: stale texts, views without an axis, broken `code[].ref`, … |
| Sync with code (`core/`) | works: `semaps sync` runs the extractors of `.semaps`; `/extract` and `/setup` pages in the editor |
| TypeScript extractor (`extractors/typescript/`) | works: types, member relations, positions in code |
| C# extractor (`extractors/csharp/`) | works: types, member relations, positions, methods and calls |
| Go extractor (`extractors/go/`) | works: types, implicit interface implementations, member relations, positions; mapping table — [`docs/extractors/go.md`](docs/extractors/go.md) |
| Extractor facts JSON schema (`schemas/`) | works: [`extractor-facts.schema.json`](schemas/extractor-facts.schema.json) |
| Code graph page (`editor/`, `/app/#graph`) | works: sigma + graphology, algorithmic layout, live updates |
| Source watching (`host/`) | works: `watch` on an extractor reruns it when sources change and updates only the graph |

## Getting started

### 1. Install — once per machine

**No Node, no Go:** download `semaps-windows-amd64.zip` from
[Releases](https://github.com/KOE73/SeMaps/releases), unpack it and double-click `semaps.exe`. It
asks whether to install for the current user, then copies itself and the `extractors\` folder to
`%LOCALAPPDATA%\Programs\SeMaps`, adds that folder to `PATH` and associates `*.semaps` files with
itself. No administrator rights needed. The same from a console: `semaps.exe install`. The bare
`semaps.exe` from the same page works too, without the extractors.

**From source** (needs Go 1.26+ and Node 24+; the editor is built and embedded):

```
git clone https://github.com/KOE73/SeMaps.git
SeMaps\install.cmd
```

It builds the editor, `semaps.exe` and the extractors (Go always, C# when `dotnet` is present,
TypeScript when `npm` is), then runs the same `install`.

Check: open a new console and type `semaps --help`.

**Extractors** sit in the same folder (`extractors\csharp`, `extractors\typescript`,
`extractors\go`). Each needs its language's runtime: .NET for C#, Node 24+ for TypeScript, Go for Go
(it reads packages through `go list`). `semaps doctor` in a project says what is found and what is
missing.

### 2. Project file — once per project

In **your project's root** (next to `.git`) create `<name>.semaps`, for example `myproject.semaps`:

```yaml
version: 1
name: My project
workspace: docs/diagrams   # where the maps live (projects/)
source_root: .             # where the code lives, for code[].ref paths
port: 8777
```

All paths are relative to this file's folder. Every key is optional; the values above are the
defaults. The file is YAML: comments survive when SeMaps edits it.

`extractors:` lists what to read from the code and into which model project; a repository may have
several (C# and TypeScript side by side). `semaps sync` runs them and updates the registry; in the
editor the same is on the **Extractors** page, and this file's settings on the **Settings** page.

```yaml
extractors:
  - id: backend
    language: csharp
    project: core            # a model project in the workspace
    root: .
    include: [src]
    exclude: ["**/*.Tests/**"]
```

Commit the file: everyone who clones the project gets the same setup.

The workspace folder may start empty: projects and views are created in the editor
(**Insert → Projects & diagrams**), or copy [`examples/workspace`](examples/workspace). There is no
list of views to keep: a project is a folder in `projects/`, a view a file in its `views/`.
**Help → How a project works** in the editor draws the layout.

### 3. Open — every day

Any of:

- **Enter** on `myproject.semaps` in Far / Total Commander, or a **double click** in Explorer;
- **`semaps`** in a console anywhere inside the project — it walks up to the `*.semaps` file;
- `semaps path\to\myproject.semaps` from anywhere.

The server starts **in its own console window**, and your console or file manager is free at once;
the browser opens on the editor. To stop it, close that window (or Ctrl+C in it). Opening the same
project again starts no second server; it just opens the browser. The editor and MCP agents work on
the host's shared working model. Accepted changes show in open windows at once and survive a
restart in a local journal. **Save** writes the changed contract files into the workspace; the
**«Changes»** panel shows what is unsaved and what an agent did. Review the changes and commit them
like code. Creating or renaming a project or view needs its current changes saved first.

### Setting up a map with an agent

Your agent does not have the SeMaps sources, so give it a link. Paste something like this to the
agent in your project:

```
Set up a SeMaps architecture map in this repository. Follow
https://github.com/KOE73/SeMaps/blob/main/docs/ADOPTING.md:
connect the semaps MCP server and do everything through its tools.
Do not place nodes on views — leave that to me. Do not commit.
```

[`docs/ADOPTING.md`](docs/ADOPTING.md) lists the steps and the traps. From then on the agent works
through MCP and never edits the workspace files by hand.

### Try it without a project

```
semaps SeMaps\examples\example.semaps
```

### Updating

A new exe from Releases, or `git pull` and `install.cmd` again: the editor is inside the exe, so an
old exe is an old editor. `install.cmd` first stops every running `semaps.exe` (open editors, the
MCP servers of agent sessions) and every exe started from the install folder: reopen the editor and
restart agent sessions afterwards.

## Extractors: one per language, yours is welcome

An extractor is a separate program for one language. It reads the sources and prints facts about
the code (types, interfaces, functions, who extends and refers to whom) as one JSON document;
`semaps sync` in the host turns those facts into the registry. An extractor never writes files and
knows nothing about views, texts or containers, so writing one for a new language is a small,
well-bounded job:

- the contract: [`docs/EXTRACTOR.md`](docs/EXTRACTOR.md), the output schema:
  [`schemas/extractor-facts.schema.json`](schemas/extractor-facts.schema.json), the shape of symbol
  ids: [`ADR_20260923-5`](docs/adr/ADR_20260923-5_extractors_symbol-ids.md);
- the mapping table for each language: [`docs/extractors/`](docs/extractors/);
- three examples: `extractors/typescript/` (runs on SeMaps' own editor), `extractors/csharp/`
  (Roslyn) and `extractors/go/`.

If you want an extractor for your language, take one of them as a template and open a PR.

## Layout

| Path | What |
|---|---|
| `editor/` | the canvas editor, TypeScript (`@semaps/editor`) |
| `host/` | the `semaps` binary, Go: serves the editor and the workspace, the MCP server |
| `core/` | language-neutral check and sync, Go, part of the `semaps` binary |
| `extractors/<lang>/` | one process per language, prints code facts as JSON |
| `schemas/` | the extractor facts JSON schema |
| `docs/` | CONTRACT, API, EXTRACTOR, ADOPTING, ADR, plans |
| `agents/` | rules for agents; documentation genres and naming |
| `examples/workspace/` | a minimal workspace to open |

A consuming project keeps only its workspace (`docs/diagrams/`) — no Node, no Go.

The built editor (`host/app/`) is not in git: CI builds it and embeds it into the release binaries
([`ADR_20260923-3`](docs/adr/ADR_20260923-3_build_bundle-from-ci-not-git.md)).

## License

[MIT](LICENSE).
