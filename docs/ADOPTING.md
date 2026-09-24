# Adopting SeMaps in a consuming project

For an agent working in **another** repository that wants a SeMaps map there. It assumes only an
installed `semaps` binary (see [README](../README.md#getting-started)), not the SeMaps source.
Format details live in [CONTRACT.md](CONTRACT.md); this file only lists the steps and the traps.

## Steps

1. **Project file** in the consuming repo root, next to `.git`: `<name>.semaps`.
   ```yaml
   version: 1
   name: <Project name>
   workspace: docs/diagrams
   source_root: .
   port: <random free port>
   ```
   Do not use the default `8777`. Pick a random free port (for example 20000–60000, and confirm
   with `netstat` that nothing is listening on it), so it doesn't clash with other projects'
   servers. See the port trap below.
   First check that the target folder (`docs/diagrams`) is free. A `.semaps` file in the SeMaps
   repo itself is not the answer; the file goes into the consuming repo.
2. **Workspace**: just the folder. There is no list file: every `projects/<id>/` with a
   `project.json` is a project, every `views/*.view.json` in it is a view. Do **not** create
   `catalog.json` — it is no longer read, and `semaps check` reports it as `устарело`.
3. **Model project** `<workspace>/projects/<id>/`:
   - `project.json`: `id` = folder name, `title`, `contractVersion: 3`, `defaultAxis`,
     `languages`, `sources.include`; optional `subtitle`, `icon`, `theme`, `order` for the catalogue.
   - `entities.json`: `e_` ids, `kind`, `origin: code` for anything read from the code, and `codeRef`
     relative to `source_root`. With an extractor for the language, let `semaps sync` write it
     (see below) instead.
   - `relations.json` + `relation-types.json`: every `type` used must be in the dictionary.
     For a derived relation, the id is `r_<from>_<to>_<type>` without the `e_` prefix.
   - `text.<lang>.json`: descriptions for `e_`, names for `rt_`/`v_`. Every value is
     `{ "v", "at", "origin" }`, and `at` is UTC ISO-8601 with `Z`.
   - `views/<id>.view.json`: `id` (starts with `v_`), `project`, `axis`, and **empty**
     `zones`/`nodes`; optional `icon`, `theme`, `order`. The view's catalogue title is `name` under
     its id in `text.<lang>.json` — without it the catalogue shows the bare id.

   The human can also create and rename projects and views in the editor (**Вставка → Проекты и
   схемы**, or ＋ / ✎ in «Каталог схем»); both write exactly these files.
4. **Check** (see the trap below) and hand over to the human to place nodes.

## A cheap first model for .NET

The solution's `src/*/*.csproj` give you entities of `kind: assembly`. The `ProjectReference`
entries give you relations of type `references`, with the `.csproj` as `evidence`. Generate both
with a short script instead of writing them by hand. This is enough to open the editor and drag
entities from the registry panel.

Deeper than assemblies (types, inheritance, containment) is the job of an extractor and
`semaps sync` (next section). Do not parse the code with regexes to fill `entities.json` by
hand: ids and relations produced that way are not reproducible, and nothing will keep them in
sync with the code.

## Sync with code: `semaps sync`

An extractor prints facts; `semaps sync` turns them into `entities.json`, `relations.json` and
`relation-types.json` of a project ([EXTRACTOR.md](EXTRACTOR.md) §5 has the rules). The
extractors ship beside `semaps` and are listed in the `.semaps` file — which code, into which
model project:

```yaml
extractors:
  - id: backend
    language: csharp          # or typescript
    project: core             # a folder under <workspace>/projects/
    root: .
    include: [src]
    edges: [holds, injects]   # optional: which member-relation edge kinds to extract
```

Then, from anywhere inside the project:

```
semaps doctor                 # which extractors and runtimes are found; what is missing
semaps sync --dry-run         # extract and show the report, write nothing
semaps sync                   # extract and write the registry
semaps sync --extractor backend   # only one of them
```

The human does the same on the **Экстракторы** page of the editor (`/extract`): extract, read the
report, write. `semaps extract` only runs the extractors and prints each run's id;
`semaps sync --run <id>` writes the facts of that run.

- `doctor` says what is missing: an extractor (install SeMaps again with `install.cmd` in the SeMaps root or take
  the archive from Releases) or a runtime (.NET SDK for C#, Node 24+ for TypeScript). Do not work
  around it — tell the human.
- Flags go **before** the project argument. `--facts <file>` still takes facts made by hand
  (`--facts -` reads stdin); keep such files out of the workspace and out of git.
- Only symbols under `project.json → sources.include` enter the project.
- The first run **adopts** entities you already have (for example the assemblies from the
  section above): same `codeRef` and `name`, or same `namespace`, `name` and `kind`. Generic
  parameter lists in names (`IRunner<in TIn, out TOut>`) and modifiers in kinds (`class` vs
  `abstract-class`) do not get in the way. They keep their ids, names and kinds and get a
  `symbol` field — that is the key for every later run; do not remove it.
  An entity that fits several symbols (or the reverse) is reported as `неоднозначно`: set its
  `symbol` by hand.
- `authored` entities, texts and views are never written. A class gone from the code gets
  `status: missing`, never deleted.
- `переименование?` in the report: an entity vanished and a new symbol of the same kind
  appeared in the same file. Nothing is decided. If it is a rename, set the old entity's
  `symbol` to the new symbol id (and `name`, if you want) and run again; if not, run with
  `--no-renames`. Decide with the human when the entity sits on views.
- `extends`, `implements`, `contains`, `references` become relations (`origin: code`) and their
  types are added to `relation-types.json`. Give each new type a name in `text.<lang>.json`
  (`rt_contains`, …), or `semaps check` reports it. The `references` type is created with
  `"visibility": "hidden"`: its relations are known but off on every view until switched on in
  the editor (the relations panel checkbox).
- Exit code: `--dry-run` gives 1 when anything would change — use it in CI next to
  `semaps check`. Without it, 1 means something is left for a human (`неоднозначно`,
  `переименование?`) or the registry contradicts itself (`сломано`, nothing written); 2 is a
  usage mistake.

## Two languages: link them or `check` fails

If the same field is `authored` in two catalogues, `check` reports `расхождение`. Decide which
one is the source and mark the other as
`{ "origin": "translated", "from": "<src lang>", "fromHash": "<hash>" }`. Compute `fromHash`
as CONTRACT §7.3 describes.

Pick the direction from the project's own convention. For example, if its comment rule says "EN
first, RU the same meaning", then RU is `translated from en`. Texts you write yourself take
whichever language you wrote first as the source.

## Hand-over: show the user where the model is

An empty canvas looks like a failure, so say where the registry went. In the editor the right
panel **«База сущностей»** (entity base) sits collapsed next to «Свойства» and «Фильтры»;
entities are dragged from it onto the canvas. Relations appear as soon as both ends are on the
view. Next to it, **«Окрестность»** (neighbourhood) shows the relations of the selected box as a
tree, by type and direction, to expand and pull onto the canvas. Right click on a box adds its
ancestors, descendants, interfaces, implementations or contents in one go, and lists every other
relation type with who is at the other end. Placing nodes is the human's job unless they ask you
for it — no "starter" layout nobody requested (CONTRACT §8.2 rule 3, §9.6).

## Verifying that the editor opened the model

`GET /api/workspace` lists what the host found; check your project and views are in it. That still
proves nothing about loading: open `/app/`, click the view in «Каталог схем» and check that every
file of the project (`views/…`, `project.json`, `entities.json`, `relations.json`,
`relation-types.json`, every `text.<lang>.json`) returned 200. A 404 on a view that
`/api/workspace` listed means two servers share the port (see the port trap).

## Traps

- **Old workspace with `catalog.json`.** Move each entry: `title` → `name` under the view's id in
  `text.<lang>.json`, `icon`/`theme` → into the `.view.json`; drop `views` from `project.json`;
  then delete `catalog.json`. `semaps check` reports it until it is gone.
- **No `*.semaps`, no server.** The host no longer looks for a workspace by itself; the project file
  is required (or `--workspace`).

- **Geometry only on request.** Unasked, an agent never writes `x/y/width/height`, zones or
  nodes (CONTRACT §8.2, §9.6): create the view empty and let the human place the nodes. When the
  human explicitly asks for help with a view ("spread these subclasses into frames by meaning"),
  do what was asked and nothing more. [LAYOUT.md](LAYOUT.md) explains how a view works — sizes,
  frames, lines — and the two things that are not optional: the view must not be open unsaved in
  the editor, and geometry outside the request stays put.
- **Judge `semaps check` by its exit code**: 0 is clean, 1 means findings (listed on stdout).
  Do not match the output text.
- **A stale `semaps` binary in `PATH`** (for example `~/go/bin/semaps` from an older `go install`)
  may not know `check` or `sync`. It treats the argument as a project and starts or reuses the server
  ("Already running: …"), still with exit 0. That looks like a pass, but nothing was checked.
  Run `where semaps` (Windows) / `which -a semaps` and `semaps --help`; if `check` is missing or
  another copy shadows the installed one, update from Releases and remove the stale copy.
- **A port shared between two servers gives a 404 on the view.** On Windows, one server can bind
  `0.0.0.0:P` and another `127.0.0.1:P` at the same time; "take the next free port" does not
  notice this. The browser then gets the project list from one workspace and a
  `HTTP 404` on the view from the other. Diagnose with `netstat -ano | grep :P` and the process
  command lines. The fix is a unique port in the `.semaps` file.
- Scripts with backslashes (`\t`, `\\`): write them as files, not through a bash heredoc or
  `python -c`; on Windows/Git Bash the escaping got mangled even with `<<'EOF'`.
- Do not commit in the consuming repo unless asked; list the created files for the user.
