# Adopting SeMaps in a consuming project

For an agent working in **another** repository that wants a SeMaps map there. It assumes only an
installed `semaps` binary (see [README](../README.md#getting-started)), not the SeMaps source.
It lists the steps and the traps. How each MCP tool works the server tells you itself (its
instructions and tool descriptions); the file format is in [CONTRACT.md](CONTRACT.md), for humans
and for SeMaps itself — an agent does not need it, since it never edits those files.

## 1. Set up

1. **Project file** in the consuming repo root, next to `.git`: `<name>.semaps`.
   ```yaml
   version: 1
   name: <Project name>
   workspace: docs/diagrams
   source_root: .
   port: <random free port>
   ```
   Do not use the default `8777`. Pick a random free port (for example 20000–60000, and confirm
   with `netstat` that nothing is listening on it), so it does not clash with other projects'
   servers (see the port trap below). First check that the target folder (`docs/diagrams`) is free.
   A `.semaps` file in the SeMaps repo itself is not the answer; the file goes into the consuming repo.
2. **MCP server** in the consuming repo root, `.mcp.json`:
   ```json
   {
     "mcpServers": {
       "semaps": { "command": "semaps", "args": ["mcp"], "env": {} }
     }
   }
   ```
   `semaps mcp` starts the host without a browser when needed and connects to its shared working
   model. Restart the agent session so that it picks the server up.
3. **A project and its views**: `create_project` and `create_view` — only when the human asked for
   them; the human can do the same in the editor (**Вставка → Проекты и схемы**). The workspace needs
   no list file: a project is a folder of `projects/`, a view a file of its `views/`.
4. **The first model**: let an extractor read the code (§ 3) rather than writing entities by hand.
5. **Check** with `semaps check` (judge it by the exit code) and hand over to the human (§ 5).

## 2. Work through MCP

- **Tools only.** The agent reads and writes the registry and views through the MCP tools, never by
  parsing or editing the workspace files. The tools enforce the rules: ids, provenance, what may be
  removed, what geometry may be written.
- **Unsaved until the human saves.** An accepted change shows in the editor at once, marked as the
  agent's, and stays unsaved until the human reviews it and presses «Сохранить». `save` and `discard`
  only when the human asked for exactly that. Ask the human to save before giving the agent a new
  task, so that what the agent changes is only what it was asked to change. Creating or renaming a
  project or view needs a clean working model: on «сначала сохраните», ask for a save first.
- **Geometry only on request.** Unasked, the agent never places, moves or sizes anything and never
  adds containers to a view. When the human asks ("spread these subclasses into frames by meaning"),
  read `layout_guide` first and do what was asked and nothing more. `remove` likewise only on request, and `delete_view` (a view's file, its captions; the registry
  stays) only when the human asked to delete that view.
- **References.** The human points at an object of a view with a reference copied in the editor
  («🔗» in Properties, or «Копировать ссылку» in the block menu): `v_ops#e_undistort`. Every view
  tool takes such references.
- **Report** each changed object and the link to the view from the last tool answer.

**Graph before grep.** A question about the structure of the code — what a type extends or
implements, who holds, calls or constructs it, what a namespace contains, where something is
declared — is one graph call, answered with `file:line`. Searching the text and reading whole
files for the same answer costs many calls and far more tokens, and still misses a caller under
another name. So ask the graph first and open only the lines it names; ask the registry and views
(`get_relations`, `get_view`) for what the code does not say — what a part is for and which
interactions are intended. The graph is what the code looks like now (the latest extractor run,
never stored); the registry is the authored model in git. On a 623-file C# project, four such
questions took an agent 14 tool calls and 96.5k tokens with grep and reads, and 2 calls and 69.4k
tokens with the graph (one run; see the README). The graph is only as fresh as the latest extractor
run, and every graph answer says so: a first line `facts: run <extractor> <time>, <age> old`, and a
`warning:` line when that run's facts lack an edge kind the `.semaps` entry asks for (an `edges:`
list that gained `calls` after the last run, say), so an empty `called-by` is not mistaken for
"nobody calls this". Then `extract` with `wait: true` refreshes it in one call, and `graph_status`
shows each extractor's runs and whether a watcher is on. Nothing runs by itself except a watcher on
a source change (`watch: true`): not at host start, not when the `.semaps` file changes. The
server's own instructions tell every
agent this; a consuming project that wants it in its own agent rules (`AGENTS.md`, `CLAUDE.md`) can
add:

```markdown
- SeMaps MCP (`semaps`) is the map of this repository. For code structure (extends, implements,
  holds, calls, constructs, contains, where declared) ask `get_graph` / `find_node` before grep or
  reading files, then open only the lines it names. For intended architecture read the registry
  and views (`get_relations`, `get_view`).
```

**Choosing `mcp.tools` / `mcp.description`** (the `mcp:` section of the `.semaps` file). A strong
model reads `get_graph`'s full parameter set on its own: `tools: one` with `description: standard`
(both the defaults) work well. A small or less steerable model does better with `tools: narrow` —
one tool per question shape (`who_calls`, `what_it_holds`, …), each taking only a name and a depth.
When context is tight, drop to `description: brief` with either set. The editor's MCP tab
(«Настройки для агента») shows the exact text and size of all six combinations and saves the choice.

## 3. Sync with code

An extractor reads the sources and prints facts; sync turns them into the project's registry
([EXTRACTOR.md](EXTRACTOR.md) §5). The extractors ship beside `semaps` and are listed in the
`.semaps` file — which code, into which model project:

```yaml
extractors:
  - id: backend
    language: csharp          # or typescript, go
    project: core             # a folder under <workspace>/projects/
    root: .
    include: [src]
    edges: [holds, injects]   # optional: which member-relation edge kinds to extract
    implements: [io.Writer]   # optional, Go only: external interfaces to report implementations of
    watch: true               # optional: rerun this extractor when its sources change
```

Through MCP: `doctor` (extractors and runtimes found), `sync_preview` (what would change, writing
nothing), `sync`, `confirm_rename`. From a console inside the project the same is:

```
semaps doctor                     # which extractors and runtimes are found; what is missing
semaps sync --dry-run             # extract and show the report, write nothing
semaps sync                       # extract and write the registry
semaps sync --extractor backend   # only one of them
```

The human does the same on the **Экстракторы** page of the editor. Flags go **before** the project
argument; `semaps extract` only runs the extractors and prints each run's id, `semaps sync --run <id>`
writes the facts of that run.

- **Something missing** (`doctor`): an extractor — install SeMaps again; or a runtime — .NET for C#,
  Node 24+ for TypeScript, Go for Go (it reads packages through `go list`). Do not work around it —
  tell the human. Reinstalling kills every running `semaps.exe`, this session's MCP proxy included:
  restart the session after it. A host that is merely restarted needs no restart of the agent: the
  proxy reconnects and re-lists the tools.
- Only symbols under `project.json → sources.include` enter the project (no include: the whole
  source root); external symbols the code refers to always do.
- The first run **adopts** entities you already have: same file and `name`, or same `namespace`,
  `name` and `kind`. They keep their ids, names and kinds and get the `symbol` of their realization —
  the key for every later run. An entity that fits several symbols (or the reverse) is reported as
  `неоднозначно`; tell the human.
- A run reconciles one realization per entity, the one in the facts' language; a second language is
  not bound to an existing entity by sync.
- Authored entities, texts and views are never written by sync. A class gone from the code gets
  `status: missing`, never deleted.
- `переименование?` in the report: an entity (or a member relation) vanished and a similar one
  appeared. Nothing is decided. If it is a rename, answer with `confirm_rename` and sync again; if
  not, sync with `--no-renames`. Decide with the human when the entity sits on views.
- `extends`, `implements`, `contains`, `depends`, `holds.*`, `uses`, `injects` become relations from
  code. Their names, styles and default visibility come from the dictionary (`get_kinds`): non-public
  members, `uses` and `injects` are hidden by default. Methods and calls never enter the registry;
  they live only in the graph.
- Exit code: `--dry-run` gives 1 when anything would change — use it in CI next to `semaps check`.
  Without it, 1 means something is left for a human (`неоднозначно`, `переименование?`) or the
  registry contradicts itself (`сломано`, nothing written); 2 is a usage mistake.
- `watch: true` only refreshes the live code graph, on a change of a source file made while the host
  runs (for `go`: `*.go`, `go.mod`, `go.sum`); it never touches the registry, sync stays a
  deliberate step. It does not catch up on changes made while the host was down or after a changed
  `edges:`/`include:` — the graph answer's `facts:` line and `warning:` show that, `extract` fixes it. The editor's graph page (`/app/#graph`) shows that graph laid out by algorithm and
  follows it live; it writes nothing.
- **A registry from before member relations.** Old `references` between types and modules become
  `missing` at the first sync, with `holds`/`uses`/`injects` and `depends` appearing next to them —
  sync does not migrate. Which views should show the new types, and whether the old relations are
  still wanted, is the human's call.

## 4. Two languages: link them or `check` fails

If the same field is `authored` in two languages, `check` reports `расхождение`. Decide which one
is the source and mark the other as translated from it
(`{ "origin": "translated", "from": "<src lang>", "fromHash": "<hash>" }`, CONTRACT §7.3). Pick the
direction from the project's own convention — for example, if its comment rule says "EN first, RU
the same meaning", RU is translated from EN. Texts you write yourself take whichever language you
wrote first as the source.

## 5. Hand-over: show the human where the model is

An empty canvas looks like a failure, so say where the registry went. In the editor the right
panel **«База сущностей»** sits next to «Свойства» and «Фильтры»; entities are dragged from it onto
the canvas, and relations appear as soon as both ends are on the view. **«Окрестность»** shows the
relations of the selected box as a tree, to expand and pull onto the canvas; right click on a box
adds its ancestors, descendants, interfaces, implementations or contents in one go. Placing nodes
is the human's job unless they ask you for it — no "starter" layout nobody requested.

To verify that the editor opened the model: `GET /api/workspace` lists what the host found. A 404 on
a view it listed means two servers share the port (see the port trap).

## 6. Moving a workspace to the current contract: `semaps migrate`

The loader reads only the current shape (contract 5, with the code shape of
[ADR_20260930-4](adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md) and the name shape
of [ADR_20260930-5](adr/ADR_20260930-5_contract_authored-entity-name-is-text.md)). A workspace written for
contract 3 — a `project.json` below 5, views with `zones`/`nodes`, a `containers.json`, `c_`/`z_` text
keys, `kinds` in `styles.json`, `zone` in `canvas.json` — or a contract-5 one in the earlier form — a
`codeRef`/`symbol` on an entity, a `via` on a relation, the `name` of an authored entity in
`entities.json`, an `edges` entry of a view with `from`/`to`/`type` — is refused with an error that names the file and the field, and `semaps check` says
the same. The way out is one command, run from the consuming repo:

```
semaps migrate --dry-run <name>.semaps   # what would be done; writes nothing, exit 1 when there is something
semaps migrate <name>.semaps             # rewrites the workspace files in place
semaps migrate --drop-untyped-styles <name>.semaps   # the same, and drops the styles that name no type
```

It rewrites **in place**, all or nothing (every new content is computed first), and is idempotent:
run again, it reports «Уже контракт v5». Commit the result as one change, apart from other work.
What it does:

- a **zone** of a view becomes an authored **entity of kind `group`**, and the view's `zones` and
  `nodes` become one `placements` array with `parent`; the text keys `z_`/`c_` are removed. The name of
  the zone (the container's, if the zone shows one) goes to the entity's `name` **text in every
  language** that has one, with its provenance — nothing is lost by language; a zone nobody names is
  named by the tail of its id, in the main language, and listed;
- `styleId` `zone.<colour>`/`container.<colour>[.dashed]` becomes an `override` with the colours of the
  old style definition (the workspace's, else the tool's old built-in one); other `styleId`s stay;
- a container of `containers.json` **that a zone shows** becomes the group entity of that zone, its
  `parent` a `contains` relation; **a container no view shows gets no entity**. The file is removed;
  its entries and their `match` rules, `axis` and `theme` are **not converted** — they are printed in the
  report;
- **realizations in code:** an entity's `codeRef` + `symbol` become `code: [{lang, ref, symbol}]`, a
  relation's `via` and `evidence[].codeRef` become `evidence: [{lang, ref, symbol, via}]` (a `line`
  becomes the anchor `:N` of the `ref`). The `lang` is the language of the project's extractor in the
  `.semaps` file when all extractors of that project are of one language; otherwise migrate **stops and
  asks** — it names the entries and writes nothing; add the extractor with its language to the file and
  run again. A `codeRef` alone (a file link) needs no language;
- **names:** the `name` of an `authored` entity leaves `entities.json` and becomes its `name` text in the
  main language of the project (`languages[0]`; provenance `authored`, `at` — the time of the run); a
  text catalogue that does not exist yet is made. A `name` text under the id of an entity from code
  that only repeats its name word for word is dead (the name of such an entity is the code's) and is
  removed; one that says something else stays, and `semaps check` lists it («лишнее имя»);
- `styles.json` of the workspace: `kinds` → `forKinds`, container styles get `appliesTo: container`,
  `default.zone` → `default.container`; a style **without `forKinds` whose id a shipped default has is
  dropped — the default wins** (the workspace `styles.json` replaces the library whole, so what stays in it
  hides the default of its id); the other styles that have no `forKinds` are kept and listed as «без
  типа» (a type is never invented — assign one, or delete the style). **`--drop-untyped-styles`** drops
  them too: the placements and edges of the views that named them lose the `styleId`, and the report
  lists which; a `styles.json` left without any style is removed, so that the shipped library applies;
  `canvas.json`: `zone` → `container`;
- **relation types** ([ADR_20260930-6](adr/ADR_20260930-6_contract_relation-types-live-in-the-dictionary.md)):
  a project's `relation-types.json` and its `rt_<type>` texts are removed; what the dictionary (the
  tool's, plus the workspace `kinds.json`) does not already say is carried into the **workspace**
  `kinds.json` and listed in the report: a `visibility` that differs from the dictionary's (also a type
  that had none where the dictionary has one), a name or description that differs from it, a type the
  dictionary does not have (its name is the `rt_` text, else the id). The entry goes into the group
  `relations.project` (a copy of the dictionary's entry, which replaces it whole) or, when the
  workspace already has the type, changes there in place. The default visibility is one per workspace:
  when projects disagree the first project wins and the report names the others. A `styleId` a type had
  is dropped with the file and named;
- **view lines** ([ADR_20260930-7](adr/ADR_20260930-7_contract_view-edges-are-an-overlay.md)): a view's
  `edges` was a full list — `id`, `from`, `to`, `type` of every line — that replaced the registry; it is
  an overlay on it now, `{id, styleId?, override?, routing?}` per line, and what is drawn is decided
  only by both ends being placed, the type's visibility in the dictionary (else the view's
  `relations.default`) and `relations.except`. For a view with the old list, a line the registry does
  not have becomes a relation `origin: authored`, `status: present` when both its ends are entities of
  the registry (otherwise it is dropped and named in the report); `relations.except` is recomputed so
  that exactly the lines of the list stay drawn; `edges` keeps only the entries that have a style, an
  override or a routing of their own, and the key is removed when none is left; an old `edges: []`
  («no lines») becomes exceptions. The relations added this way are hidden on the views that had no
  such list, so their picture does not change. The report gives per view the entries and the exceptions
  before and after. Do not write `edges: []` in the current shape: leave the key out;
- `contractVersion` goes to 5 in every file that has one.

The report ends with «Решает человек»: the `match` rules and unplaced containers of `containers.json`, styles
without a type (or the placements that lost a dropped style), zones nobody names. Then run `semaps
check`: entity kinds and relation types outside the dictionary are listed («не из словаря»); add the
project's own to `<workspace>/kinds.json`; two languages with the same name both `authored` are listed as
`расхождение` — mark one as translated from the other (§ 4). Do not edit `contractVersion` by
hand instead of migrating. An old `catalog.json` is no longer read and `check` reports it until it is
gone: titles become `name` texts under the view ids, `icon`/`theme` go into the `.view.json`.

## Traps

- **No `*.semaps`, no server.** The host does not look for a workspace by itself; the project file
  is required (or `--workspace`).
- **Judge `semaps check` by its exit code**: 0 is clean, 1 means findings (listed on stdout).
  Do not match the output text.
- **A stale `semaps` binary in `PATH`** (for example `~/go/bin/semaps` from an older `go install`)
  may not know `check` or `sync`. It treats the argument as a project and starts or reuses the server
  ("Already running: …"), still with exit 0. That looks like a pass, but nothing was checked.
  Run `where semaps` (Windows) / `which -a semaps` and `semaps --help`; if `check` is missing or
  another copy shadows the installed one, update from Releases and remove the stale copy.
- **A port shared between two servers gives a 404 on the view.** On Windows, one server can bind
  `0.0.0.0:P` and another `127.0.0.1:P` at the same time; "take the next free port" does not
  notice this. The browser then gets the project list from one workspace and a `HTTP 404` on the
  view from the other. Diagnose with `netstat -ano | grep :P` and the process command lines. The fix
  is a unique port in the `.semaps` file.
- Do not commit in the consuming repo unless asked; list the created files for the human.
