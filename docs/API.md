# SeMaps Host & Backend API Specification

This document defines the host/backend communication contract for the SeMaps editor (`@semaps/editor`). Any backend implementation (Go server, Node.js server, VSCode extension WebView, Electron, CLI embedded server, Cloud) must fulfill these requirements to ensure full compatibility for both reading and writing models.

**Scope.** This file covers transport and the host's working model. The disk format remains normative in [`CONTRACT.md`](CONTRACT.md) (contract **v3**). The host understands project objects and writes contract files only on project Save.

---

## 1. Architectural Overview

The editor operates over a project workspace containing:
1. **Workspace index**: `GET /api/workspace` — the projects and views the host found on disk (§2.4). There is no list file.
2. **Global Styles**: `styles.json` defining color schemes, strokes, typography, and badges.
3. **Content templates**: `templates.json`, the named registry of block content
   templates (`CONTRACT.md` §11.2) — sits next to `styles.json`, not inside it.
4. **Content assets**: `content/index.json`, the manifest of named images for the
   `@Asset` directive, plus the `content/*.svg` files it points at
   (`CONTRACT.md` §11.4).
5. **Projects**: `projects/<project_id>/` directories holding:
   - `project.json` — Project manifest
   - `entities.json` — Catalog of code entities / types
   - `relations.json` — Catalog of code relations & dependencies
   - `relation-types.json` — Authored vocabulary of relation types
   - `containers.json` — Container membership rules
   - `text.<lang>.json` — Localized texts with per-field provenance
   - `views/<view_id>.view.json` — Classification axis, geometry, zones, node placements

```
Workspace Root (--workspace, e.g. docs/diagrams/)
├── styles.json               Shared style library
├── templates.json            Shared content-template registry
├── content/
│   ├── index.json            Asset manifest for the @Asset directive
│   └── *.svg                 The asset files themselves
└── projects/
    └── <project_id>/
        ├── project.json
        ├── entities.json
        ├── relations.json
        ├── relation-types.json
        ├── containers.json
        ├── text.ru.json
        └── views/
            └── <view_id>.view.json
```

---

## 2. Read Requirements (Static / GET)

The backend serves application assets and global workspace JSON. Project model reads use §3.4; static project JSON remains available for compatibility and inspection, but is not the editor's working state.

### 2.1. Endpoints & Paths

| Path / URL | Method | Content-Type | Description |
|---|---|---|---|
| `/app/` | `GET` | `text/html` | Entry point for the editor application |
| `/app/assets/*` | `GET` | `application/javascript`, `text/css` | Bundled JavaScript, CSS, and media |
| `/api/workspace` | `GET` | `application/json` | Projects and their views, found on disk (§2.4) |
| `/styles.json` | `GET` | `application/json` | Shared style stylesheet (returns 404 if not created yet; client falls back to built-in styles) |
| `/templates.json` | `GET` | `application/json` | Content-template registry (404 tolerated: client falls back to the built-in `title-only` template) |
| `/content/index.json` | `GET` | `application/json` | Asset manifest for `@Asset` (404 tolerated: the picker is simply empty) |
| `/content/<file>.svg` | `GET` | `image/svg+xml` | An individual asset file named by the manifest; served as ordinary static content, no per-file endpoint |
| `/projects/<project_id>/project.json` | `GET` | `application/json` | Project manifest |
| `/projects/<project_id>/entities.json` | `GET` | `application/json` | Catalog of all entities |
| `/projects/<project_id>/relations.json` | `GET` | `application/json` | Catalog of all relations |
| `/projects/<project_id>/relation-types.json` | `GET` | `application/json` | Relation type vocabulary (404 tolerated: the client falls back to an empty vocabulary) |
| `/projects/<project_id>/text.<lang>.json` | `GET` | `application/json` | Localized strings (e.g. `text.ru.json`) |
| `/projects/<project_id>/views/<view_id>.view.json` | `GET` | `application/json` | Axis, placement & geometry layout |

`containers.json` is part of the project on disk but is **not requested by the current client** — a host still serves it like any other file.

### 2.1a. Tool defaults and workspace overrides

`styles.json`, `templates.json` and `content/` have defaults shipped with the tool (reference host: `host/defaults/`). A host serves the workspace copy when it exists and the default otherwise. Writes always go to the workspace, so the first save of a default creates the override; the defaults themselves are never written.

### 2.2. Workspace index: `GET /api/workspace`

The one listing a host gives. It walks `projects/*/` (a folder counts only with a `project.json`) and
`views/*.view.json` in each; nothing else is listed, and plain directory listing stays refused
([`ADR_20260923-7`](adr/ADR_20260923-7_contract_projects-and-views-found-not-listed.md)).

```json
{ "projects": [
  { "id": "shop", "title": "Магазин", "subtitle": "…", "icon": "📁", "theme": "blue", "order": 1,
    "languages": ["ru"],
    "views": [
      { "id": "v_main", "file": "projects/shop/views/v_main.view.json", "axis": "axis_subsystem",
        "icon": "🗺️", "theme": "blue", "order": 1, "names": { "ru": "Общая картина" } } ] } ] }
```

- `title` falls back to the folder name; `names` holds `name` under the view's id from each
  `text.<lang>.json` of `languages` (empty when there is none).
- Projects and views are sorted by `order`, entries without one last, then by `id`.
- A file that does not parse stays in the list with an `error` string instead of its fields.
- No `projects/` folder is an empty list, not an error. Served with `Cache-Control: no-cache`.
- Reference implementation: `core.Index` (`core/index.go`).

### 2.2a. Base URL Handling
- The editor bundle is **not** part of the workspace. The reference host serves it at `/app/` from its own installation (`host/app/`); the workspace is served at `/`.
- When served from `/app/`, relative model paths resolve relative to `../` (workspace root).
- In development mode (Vite dev server), model paths resolve relative to `./` (mounted public root).

### 2.3. Caching

Serve every `.json` with `Cache-Control: no-cache` (or an equivalent validator). Model API responses are working state and must not be cached. Application assets under `/app/assets/*` are content-hashed and may be cached normally.

---

## 3. Write Requirements (Save API)

The backend persists project model files through `POST /api/model/{project}/save` (§3.4). `/api/save` remains for global assets and creation of a new project/view file.

### 3.1. Save Endpoint: `POST /api/save`

- **URL Pattern**: `/api/save?file=<relative_path>`; existing files under `projects/` return `410 Gone`. The only project paths permitted are `create=1` on `projects/<id>/project.json` and `projects/<id>/views/<id>.view.json`.
- **Method**: `POST`
- **Headers**: `Content-Type: application/json`, `Authorization: Bearer <host key>`
- **Request Body**: Valid JSON payload formatted with 2-space indentation.

#### Query Parameter `create`
- `create=1`: write only if the file does not exist yet; otherwise `409 Conflict` and nothing is
  written. The editor creates `project.json` and new views this way, so a taken id never
  overwrites anything ([`ADR_20260923-8`](adr/ADR_20260923-8_contract_project-and-view-ids-can-change.md)).

#### Query Parameter `file`
- Contains a workspace-relative path (e.g. `projects/llm_pipeline/views/v_main.view.json`, `styles.json`, `templates.json`).
- Path separators may be `/` (standardized by client) or `\` (Windows).
- Model paths are always resolved relative to the **workspace root** (the directory named by `workspace` in the `.semaps` file), never relative to `/app/` — the editor application bundle is a separate, content-hashed tree served alongside the models, not a parent of them (§2.2a).

### 3.2. Response Status Codes

| HTTP Status | Condition | Body / Reason |
|---|---|---|
| `200 OK` | File successfully written to disk | `OK` or `{ "status": "ok" }` |
| `400 Bad Request` | Missing `file` param, invalid extension, or directory traversal attempt | Error message string |
| `405 Method Not Allowed` | Method is not `POST` | `Method not allowed` |
| `401 Unauthorized` | Missing or wrong host key on a write | Error text |
| `409 Conflict` | `create=1` and the file exists | `Already exists: <file>` |
| `410 Gone` | An existing project model file was submitted | Use project Save (§3.4) |
| `500 Internal Server Error` | File system I/O error or permission failure | Error message string |

### 3.3. Security & Path Traversal Rules
1. **Canonicalization**: The server must clean the requested path (e.g. via `filepath.Clean`).
2. **Directory Traversal Protection**: Paths starting with `..`, containing `../` or `..\`, or resolving outside the workspace root **MUST** be rejected with HTTP 400.
3. **Absolute Path Protection**: Absolute paths (e.g. `/etc/passwd`, `C:\Windows\...`) **MUST** be rejected with HTTP 400.
4. **Extension Whitelist**: Only `.json` files are permitted to be written via `/api/save`. `templates.json` is written through this same endpoint like any other model file — its multi-line template text is stored as a JSON array of strings (`lines`) precisely so it stays inside the `.json`-only whitelist without widening it (`CONTRACT.md` §11.2, `ADR_20260903` §2.9). Assets under `content/` are never written by the editor: they are static files dropped in by hand, read-only from the editor's point of view.
5. **Auto Directory Creation**: If parent subdirectories do not exist (e.g. `projects/<project_id>/views/`), the server **MUST** create them automatically before writing the file.

### 3.3a. Rename: `POST /api/move`

`/api/move?from=<rel>&to=<rel>` renames a project folder or a view file. Both paths are
workspace-relative and **must start with `projects/`**; a file keeps its `.json` extension.
`404` when `from` is missing, `409` when `to` exists or the project's working model is dirty
(save first), `400` for a path outside `projects/`, `403` cross-origin and `401` without the host
key like `/api/save`. Directories are moved whole; the host evicts a renamed project's loaded model.

### 3.4. Working model API

The running host owns one in-memory `core.Model` per project. Contract files stay unchanged until Save. Every accepted batch is appended to `<project root>/.semaps/work/<project>.jsonl`; startup replays it. A project-wide Save writes dirty files in contract v3 format (two spaces, no HTML escaping, existing key order), then clears the journal. Discard reloads the selected scope from files and rewrites the journal for remaining edits. There is no host Undo or field merge.

The host creates `<project root>/.semaps/host.json` containing `{pid, port, key}`. The key is also embedded as `<meta name="semaps-key" content="…">` in `/app/` HTML. Every mutating endpoint requires `Authorization: Bearer <key>` and the existing same-origin check; missing/wrong key returns `401`. This includes `/api/save`, `/api/move`, and the tool API writes. `.semaps/` is never served as workspace static content. The host listens on localhost.

| Path | Method | Response / effect |
|---|---|---|
| `/api/model/{project}` | `GET` | `{project, registry, texts, views, dirty}`. `project` is the manifest; `registry` maps registry filenames to full documents; `texts` maps languages to full text documents; `views` maps view ids to view properties without geometry; `dirty` is below. |
| `/api/model/{project}/views/{id}` | `GET` | Full working view document, including `zones`, `nodes` or `placements`, and `relations`. |
| `/api/model/{project}/ops` | `POST` | `{client, ops}` applies one atomic batch as `human` → `{changed, dirty}`. A broken rule returns `422` with text and applies nothing. |
| `/api/model/{project}/save` | `GET` | Current `dirty` summary for the Save confirmation. |
| `/api/model/{project}/save` | `POST` | Writes every dirty contract file and clears the journal → empty `dirty`. |
| `/api/model/{project}/discard` | `POST` | `{scope:"view",id}` or `{scope:"registry"}` or `{scope:"all"}`; reloads that scope from disk → new `dirty`. |
| `/api/events?project=<id>` | `GET` | SSE: one `data:` JSON object per accepted batch/save/discard: `{client,author,changed,dirty}`. Clients ignore their own `client` id. |

An operation is `{kind,id,view?,lang?,value,author?}`. `kind` is `entity`, `relation`, `relationType`, `text`, `view`, `zone`, or `node`. `value` is the whole object; `null` removes a zone or node placement only. Registry records cannot be removed. `text` requires `lang`; `view`, `zone`, and `node` require `view`. The host sets `author: human` on browser ops; the agent path sets `author: agent`. A changed reference is `{kind,id,view?,lang?,author}`. `dirty` is `{registry:[Ref],views:{<view-id>:[Ref]}}`, with the last author per touched object. The conflict unit is an object: later accepted replacement wins, with no field merge. The SSE event is emitted after the batch is in the journal. A view is loaded lazily when first read or changed.

Project and view creation still use `/api/save?create=1`; renames still use `/api/move`. Both writes require the host key. Renaming a dirty project is rejected with `409`; after a successful rename the host reloads the project on next access. `styles.json`, `templates.json`, and `content/` remain outside the working model.

---
## 4. Alternative Host Adapters (VSCode / Electron / Node)

When embedding `@semaps/editor` in non-HTTP hosts (e.g., VSCode Extension WebView, Electron IPC):

1. **Implement `ModelStore`**:
   ```typescript
   export interface ModelStore {
     load(file: string): Promise<WireDocument>;
     save(target: SaveTarget, wire: WireDocument): Promise<void>;
   }
   ```
2. **Implement `StyleStore`**:
   ```typescript
   export interface StyleStore {
     load(): Promise<WireStyleSheet>;
     save(sheet: WireStyleSheet): Promise<void>;
   }
   ```
3. **Implement `WorkspaceStore`**:
   ```typescript
   export interface WorkspaceStore {
     load(): Promise<WorkspaceIndex>;          // same shape as GET /api/workspace
     createProject(project: NewProject): Promise<void>;
     createView(view: NewView): Promise<string>; // returns the new view's file
     updateProject(oldId: string, project: NewProject): Promise<void>;
     updateView(oldId: string, view: NewView, languages: readonly string[]): Promise<string>;
   }
   ```

Any host fulfilling these three interfaces will provide complete visualizer and editor functionality without requiring a running Go HTTP server.

## Reference host (`host/`)

```
semaps [flags] [dir | file.semaps]
semaps check [flags] [dir | file.semaps]
semaps sync [--extractor <id>] [--run <id>] [--dry-run] [--no-renames] [flags] [dir | file.semaps]
semaps sync --facts <file.json | -> [--project <id>] [--dry-run] [--no-renames] [flags] [dir | file.semaps]
semaps extract [--extractor <id>] [dir | file.semaps]
semaps doctor [dir | file.semaps]
semaps install
```

`install` (Windows) copies the exe — and the `extractors\` folder beside it, replaced as a whole —
to `%LOCALAPPDATA%\Programs\SeMaps`, adds it to the user PATH and
registers the `*.semaps` association under HKCU; started with nothing to open and not installed, the exe
offers the same interactively.

`check` resolves the roots exactly as the server does, runs the model check from `core/` and
exits with `1` when anything is found. It does not serve.

`sync` resolves the roots the same way and reconciles one project's registry with extractor facts;
rules, report and exit codes — [`EXTRACTOR.md`](EXTRACTOR.md) §5. It does not serve. Without
`--facts` it runs the extractors of the `.semaps` file (all, or the one named by `--extractor`) and
syncs each into its `project`; `--run <id>` syncs the facts of an earlier run instead of extracting.

`extract` only runs the extractors; each run is kept (facts, log, statistics) in the temp directory
and printed with its id. `doctor` shows which extractor and runtime each entry resolves to; exit `1`
when one of them cannot run.

**Finding an extractor** (ADR_20260924-3 §1): the entry's `command` → `extractors/<language>/`
beside the running `semaps` (`csharp`: `semaps-extract-csharp[.exe]` or `semaps-extract-csharp.dll`
run by `dotnet`; `typescript`: `dist/cli.js` run by `node`) → `semaps-extract-<language>` on PATH.
It is started without a shell, in the project root, with `--root <root> --include … --exclude …`
([`EXTRACTOR.md`](EXTRACTOR.md) §1).

### Project file `*.semaps`

Lives in the project root; that folder is the project root, and every path in the file is relative
to it. YAML; a `#` at the start of a line or after a space is a comment (quote a value that needs
` #`). Unknown keys are an error, and so are two `*.semaps` files in one directory. The host edits
the file through the YAML tree: comments and key order survive.

| Key | Default | Meaning |
|---|---|---|
| `version` | — | format version, `1` |
| `name` | — | shown in the console |
| `workspace` | `docs/diagrams` | served at `/`; the only place `/api/save` writes to |
| `source_root` | `.` | what `codeRef` and `/api/source*` resolve against |
| `port` | `8777` | if busy, the next free port is taken |
| `extractors` | — | list; each: `id` (lowercase, digits, `-`), `language`, `project` (model project under `projects/`), `root` (default `.`), `include`, `exclude`, `edges` (optional list: `holds`, `uses`, `injects`), `command` (replaces the found extractor; written by hand only) |

### Resolution order

1. `--workspace` / `--source-root` flags, if given.
2. The `.semaps` file passed as the argument, or the first `*.semaps` found walking up from `dir`
   (default: the current directory).
3. Nothing else: without a `*.semaps` file (or `--workspace`) the host exits with an error. With
   `--workspace` and no `--source-root`, the source root is the nearest directory with `.git` above
   the workspace, else the workspace.

`--port` beats the file's `port`. `--no-browser` does not open a browser.

On Windows the server moves to a new console window and the command returns at once; `--here`
keeps it in the current console. Before starting, the host asks `GET /api/info` on the port range;
if a host already serves the same workspace, it only opens the browser there.

`GET /api/info` → `{"workspace": "<abs path>", "sourceRoot": "<abs path>"}`.

`POST /api/save` is refused with `403` when the request carries an `Origin` that is not the host
itself: the host listens on localhost, but any page open in the same browser could otherwise POST
to it. Requests without `Origin` (curl, agents) pass. Directories are never listed.

The editor bundle (`app/`) and the defaults (`defaults/`) are embedded into the binary, never taken from the workspace.

## 5. Tool API: settings, extractors, runs

Served only when the host was started from a `.semaps` file; with `--workspace` every endpoint
answers `409`. Every request passes one check (`guard`): anything but `GET` is refused with `403`
when its `Origin` is not the host itself, as for `/api/save`. Paths in requests and answers are
relative to the project root; absolute paths of the machine are not handed out (ADR_20260924-3 §6).
The command line of an extractor cannot be set through the API — only in the file.

| Path | Method | Body → answer |
|---|---|---|
| `/api/tools` | `GET` | → `{projectFile, shipped: [language], runtimes: [{name, ok, version?, hint?}], extractors: [Extractor]}` |
| `/api/setup` | `GET` | → `{projectFile, name, workspace, sourceRoot, port, extractors: [Extractor], languages}` — keys as written |
| `/api/setup` | `PUT` | `{name?, workspace?, sourceRoot?, port?}` → setup. Workspace, source root and port apply after a restart |
| `/api/setup/extractors/{id}` | `PUT` | `{language?, project?, root?, include?, exclude?, edges?}` → setup; adds the entry when missing. Any other field (`command`) → `400` |
| `/api/setup/extractors/{id}` | `DELETE` | → setup |
| `/api/runs?extractor=<id>` | `GET` | → `[Run]`, newest first; five kept per extractor |
| `/api/runs` | `POST` | `{extractor}` → `202` `Run` (`state: running`) |
| `/api/runs/{id}` | `GET` | → `Run` |
| `/api/runs/{id}/log?offset=<n>` | `GET` | → `{text, offset, state}`: the log from byte `n`; ask again with the new `offset` while `state` is `running` |
| `/api/runs/{id}/sync` | `POST` | `{dryRun, noRenames}` → `{report, exitCode, empty}`; the report is `core.SyncReport`. Applies the facts of that run to the working model (unsaved), never extracts again |
| `/api/mcp` | `GET` | → `{file, exists, configured, entry, onPath, snippet, tools: [{name, description, readOnly, inputSchema}], error?}`: does `.mcp.json` at the project root start `semaps mcp` (§6), and the tools it offers |
| `/api/mcp/install` | `POST` | → the same, after adding the `semaps` entry to `.mcp.json` (created when missing; other servers and keys stay; a file that does not parse → `409`) |
| `/api/mcp/call` | `POST` | `{name, arguments}` → `{messages: [{dir: out \| in, message}], ms, isError, error?}`: the editor's sandbox. Runs one tool of `semaps mcp` on this project in a fresh in-memory session, as an agent would; `messages` are the JSON-RPC call and answer as they went, the handshake left out. The call is real: a writing tool writes |

`Extractor`: `{id, language, project, root, include, exclude, edges, command?, tool: {language, found,
source?: command|bundled|path, where?, runtime?, problem?}, lastRun?: Run}`.
`Run`: `{id, extractor, project, language, started, finished?, seconds?, state: running|done|failed,
exitCode, error?, stats?: {symbols, edges, symbolKinds, edgeKinds, language}}`.

A run's facts never enter the workspace; they stay in the temp directory beside its log.

The tool pages are modes of the one editor page: `/app/#extract`, `/app/#project`, `/app/#mcp`. Short addresses `/extract` and `/setup` redirect there.

## 6. MCP: `semaps mcp`

`semaps mcp [--project <id>] [dir | file.semaps]` serves the registry as MCP tools over stdio
(official Go SDK, [`ADR_20260924-5`](adr/ADR_20260924-5_host_mcp-server.md)). Roots are found as
for `semaps` without arguments: a `.semaps` file upward from the current directory. stdout is the
protocol; the project banner and extractor logs go to stderr. `--project` picks the model project
when the workspace has several; every tool also takes `project`.

The tools are thin: every rule of writing is a function of `core/` (`core/edit.go`), and a broken
rule comes back as a tool error (`isError: true`) whose text names the rule. Nothing was written
then.

**Promises.**
- `id`s are minted by `core`, never passed in: `r_<from>_<to>_<type>` for an authored relation,
  `_2` on collision.
- Texts are written `origin: authored`, `at` = now in UTC; `from`/`fromHash` of a translation go.
  A relation with `origin: code` takes no text (CONTRACT §4); an entity takes no `name`.
- **Nothing is deleted**: there is no tool for it.
- Geometry of a view is written only by `place_entities` with `requestedByHuman: true`, and an
  entity already on the view is refused, not moved (CONTRACT §8.2 p. 3).
- Files keep every key they had, in the order they had it.

Every structured answer (`structuredContent`) is a JSON object, lists included: clients
reject anything else.

**Log.** Every tool call — the tool, its arguments, time, the error if any — is a JSON line in
`<project root>/.semaps/logs/mcp-<date>.jsonl` (kept 14 days; the folder carries its own
`.gitignore`), and a short line on stderr, which clients keep in their server logs. Calls from the
editor's sandbox land in the same file.

**Reading**

| Tool | Input | Answer |
|---|---|---|
| `list_projects` | — | `core.Index` of the workspace: projects and their views |
| `list_views` | `project?` | `{views}` |
| `get_entity` | `id` \| `symbol` | the entity as in `entities.json` |
| `find_entities` | `query?` (substring of name, symbol, namespace, id), `kind?`, `status?`, `limit?` = 50 | `{total, entities}` |
| `get_relations` | `entity?`, `direction?` = `both` \| `out` \| `in`, `type?` (a prefix ending with `.` matches `holds.`), `status?`, `limit?` = 200 | `{total, relations}` |
| `get_relation_types` | — | `{relationTypes}`: the vocabulary with `visibility` |
| `get_text` | `lang`, `key` | the entry of `key` in `text.<lang>.json`, `{}` when none |
| `doctor` | — | `{extractors, extractorsOK, findings}`: `semaps doctor` and `semaps check` |
| `sync_preview` | as `sync` | as `sync`, writing nothing |

**Writing**

| Tool | Input | Effect |
|---|---|---|
| `set_text` | `lang`, `key`, `field` (`name`, `title`, `description`, `doc`, `fromLabel`, `toLabel`), `value` | one field, authored |
| `add_relation` | `from`, `to`, `type` | authored relation; both entities and the type must exist; the id comes back |
| `add_relation_type` | `id`, `visibility?`, `styleId?` | authored type; its name goes by `set_text` under `rt_<id>` |
| `set_relation_visible` | `view`, `relation`, `visible` | the relation into or out of `relations.except` against the default (CONTRACT §8.5) |
| `confirm_rename` | `entity` + `symbol` \| `relation` + `member` | answers «переименование?» of sync: the old entity takes the new `symbol`, the old member relation the new `via.member`; then `sync` again |
| `extract` | `extractor?` | runs the extractors of the `.semaps` file one by one → `{runs: [{run, extractor, project}]}` |
| `sync` | `extractor?`, `run?`, `noRenames?` | extracts (or takes the facts of `run`) and reconciles; the text answer is the sync report, the structured one `{reports: [core.SyncReport]}` |
| `place_entities` | `view`, `entities: [{entity, zone?, x, y, width?, height?}]`, `requestedByHuman` | puts entities on a view |

**Entry** in the consuming project, `.mcp.json` at its root:
```json
{ "mcpServers": { "semaps": { "command": "semaps", "args": ["mcp"] } } }
```
