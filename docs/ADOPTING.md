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
2. **Workspace**: `<workspace>/catalog.json`, one entry per view: `id`, `file` (path to the
   view, relative to the workspace), `title`, `subtitle`, `icon`, `theme`.
3. **Model project** `<workspace>/projects/<id>/`:
   - `project.json`: `id` = folder name, `contractVersion: 3`, `defaultView`, `defaultAxis`,
     `languages`, `sources.include`.
   - `entities.json`: `e_` ids, `kind`, `origin: code` for anything read from the code, and `codeRef`
     relative to `source_root`.
   - `relations.json` + `relation-types.json`: every `type` used must be in the dictionary.
     For a derived relation, the id is `r_<from>_<to>_<type>` without the `e_` prefix.
   - `text.<lang>.json`: descriptions for `e_`, names for `rt_`/`v_`. Every value is
     `{ "v", "at", "origin" }`, and `at` is UTC ISO-8601 with `Z`.
   - `views/<id>.view.json`: `id`, `project`, `axis`, and **empty** `zones`/`nodes`.
4. **Check** (see the trap below) and hand over to the human to place nodes.

## A cheap first model for .NET

The solution's `src/*/*.csproj` give you entities of `kind: assembly`. The `ProjectReference`
entries give you relations of type `references`, with the `.csproj` as `evidence`. Generate both
with a short script instead of writing them by hand. This is enough to open the editor and drag
entities from the registry panel.

Deeper than assemblies (types, inheritance, calls) is the job of `extractors/csharp`
([plan](plans/PLAN_20260923_extractors_csharp.md)). Do not parse the code with regexes to fill
`entities.json` by hand: ids and relations produced that way are not reproducible, and nothing
will keep them in sync with the code.

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
view. Placing the first nodes is the human's job, not yours — including a "starter" layout
(CONTRACT §8.2 rule 3, §9.6): a view with nodes an agent wrote is a contract violation, not a
favour.

## Verifying that the editor opened the model

A 200 on `catalog.json` proves nothing. Open `/app/`, click the view in «Каталог схем» and check
that every file of the project (`views/…`, `project.json`, `entities.json`, `relations.json`,
`relation-types.json`, every `text.<lang>.json`) returned 200. A 404 on the view with a 200 on
`catalog.json` means two servers share the port (see the port trap).

## Traps

- **Geometry is not yours.** An agent never writes `x/y/width/height`, zones or nodes
  (CONTRACT §8.2, §9.6). Create the view empty; the human places the nodes.
- **Judge `semaps check` by its exit code**: 0 is clean, 1 means findings (listed on stdout).
  Do not match the output text.
- **A stale `semaps` binary in `PATH`** (for example `~/go/bin/semaps` from an older `go install`)
  may not know `check`. It treats the argument as a project and starts or reuses the server
  ("Already running: …"), still with exit 0. That looks like a pass, but nothing was checked.
  Run `where semaps` (Windows) / `which -a semaps` and `semaps --help`; if `check` is missing or
  another copy shadows the installed one, update from Releases and remove the stale copy.
- **A port shared between two servers gives a 404 on the view.** On Windows, one server can bind
  `0.0.0.0:P` and another `127.0.0.1:P` at the same time; "take the next free port" does not
  notice this. The browser then gets `catalog.json` from one workspace and a
  `HTTP 404` on the view from the other. Diagnose with `netstat -ano | grep :P` and the process
  command lines. The fix is a unique port in the `.semaps` file.
- Scripts with backslashes (`\t`, `\\`): write them as files, not through a bash heredoc or
  `python -c`; on Windows/Git Bash the escaping got mangled even with `<<'EOF'`.
- Do not commit in the consuming repo unless asked; list the created files for the user.
