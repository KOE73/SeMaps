# SeMaps Host & Backend API Specification

This document defines the host/backend communication contract for the SeMaps editor (`@semaps/editor`). Any backend implementation (Go server, Node.js server, VSCode extension WebView, Electron, CLI embedded server, Cloud) must fulfill these requirements to ensure full compatibility for both reading and writing models.

**Scope.** This file covers transport and the host's working model. The disk format remains normative in [`CONTRACT.md`](CONTRACT.md) (contract **v5**). The host understands project objects and writes contract files only on project Save. A project of an older contract is not served but named: the error carries the file, the field and «`semaps migrate`» ([`ADR_20260927-3`](adr/ADR_20260927-3_core_migrations-outside-the-loader.md)).

---

## 1. Architectural Overview

The editor operates over a project workspace containing:
1. **Workspace index**: `GET /api/workspace` — the projects and views the host found on disk, with titles and names from the working model (§2.2). There is no list file.
2. **Global Styles**: `styles.json` defining color schemes, strokes, typography, and badges of blocks, containers and edges; each style belongs to the entity kinds or relation types named in its `forKinds` (`CONTRACT.md` §11.5).
2a. **Dictionary**: `kinds.json` — groups of entity kinds and of relation types with names, descriptions and a base style (`CONTRACT.md` §6). The tool's default is embedded; the workspace file adds to it.
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
   - `text.<lang>.json` — Localized texts with per-field provenance
   - `views/<view_id>.view.json` — Classification axis, geometry, one `placements` array (blocks and containers alike)

```
Workspace Root (--workspace, e.g. docs/diagrams/)
├── styles.json               Shared style library
├── kinds.json                Dictionary supplement: entity kinds, relation types
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
| `/api/workspace` | `GET` | `application/json` | Projects and their views as the working model sees them (§2.2) |
| `/styles.json` | `GET` | `application/json` | Shared style stylesheet (returns 404 if not created yet; client falls back to built-in styles) |
| `/canvas.json` | `GET` | `application/json` | Numbers of the canvas: grid, block and container sizes, caption strip, padding, gaps (§2.1a). Always answers: the workspace copy, else the tool's default |
| `/kinds.json` | `GET` | `application/json` | The **workspace** dictionary supplement, as written (404 when the workspace has none). It does not replace the default: the merged dictionary is `/api/kinds` (§2.1b) |
| `/api/kinds` | `GET` | `application/json` | The dictionary — the tool's default with the workspace `kinds.json` added — §2.1b |
| `/templates.json` | `GET` | `application/json` | Content-template registry (404 tolerated: client falls back to the built-in `title-only` template) |
| `/content/index.json` | `GET` | `application/json` | Asset manifest for `@Asset` (404 tolerated: the picker is simply empty) |
| `/content/<file>.svg` | `GET` | `image/svg+xml` | An individual asset file named by the manifest; served as ordinary static content, no per-file endpoint |
| `/projects/<project_id>/project.json` | `GET` | `application/json` | Project manifest |
| `/projects/<project_id>/entities.json` | `GET` | `application/json` | Catalog of all entities |
| `/projects/<project_id>/relations.json` | `GET` | `application/json` | Catalog of all relations |
| `/projects/<project_id>/relation-types.json` | `GET` | `application/json` | Relation type vocabulary (404 tolerated: the client falls back to an empty vocabulary) |
| `/projects/<project_id>/text.<lang>.json` | `GET` | `application/json` | Localized strings (e.g. `text.ru.json`) |
| `/projects/<project_id>/views/<view_id>.view.json` | `GET` | `application/json` | Axis, placement & geometry layout |

There is no `containers.json`: a container is an entity of a container kind (`CONTRACT.md` §6, §8.2). A workspace that still has one is refused by `semaps check` and rewritten by `semaps migrate`.

### 2.1a. Tool defaults and workspace overrides

`styles.json`, `templates.json`, `canvas.json`, `graph-filters.json` and `content/` have defaults shipped with the tool (reference host: `host/defaults/`). A host serves the workspace copy when it exists and the default otherwise. Writes always go to the workspace, so the first save of a default creates the override; the defaults themselves are never written. `kinds.json` is the exception: the workspace file **adds to** the shipped one instead of replacing it (§2.1b).

`canvas.json` is the one definition of the canvas numbers, in model units (1 unit = 1 screen pixel at zoom 1): `{"grid":10,"node":{"width":180,"height":60,"minWidth":100,"minHeight":40,"radius":8},"container":{"minWidth":160,"minHeight":100,"headerHeight":28,"padding":16,"radius":10},"gap":{"node":40,"container":40}}`. `grid` is the step the **editor** snaps to; the **host does not snap** — geometry an agent sends is written as sent, so an agent puts things on multiples of `grid` itself. `node.width/height` is the size of a block written without one; the `min*` values are floors for `resize_elements`, `add_container` and friends; `container.headerHeight` (caption strip) and `container.padding` are what `fit_container` and the growing of parent containers add around content; `gap.*` are the defaults an agent leaves between what it places. (Before contract 5 the key was `zone`; `semaps check` names it, `semaps migrate` renames it.) The host reads the workspace copy on every use (a broken one is reported and the shipped default used) and `core` receives it as a `Canvas` value — `core/` holds no copy of the numbers; the editor reads the same file over `/canvas.json`. The host renders these numbers into the head of every `get_view` answer and into `layout_guide` (§6).

### 2.1b. Dictionary: `GET /api/kinds`

The merged dictionary of `CONTRACT.md` §6: the default the tool ships (`host/defaults/kinds.json`)
with `<workspace>/kinds.json` added by the rule of §6 (a new group or entry is appended, an entry
with a known `id` replaces the earlier one). Both the editor and MCP `get_kinds` read this one
answer — nobody merges files themselves. A broken `kinds.json` is `500` with the file named.

- Without `lang`: every text in all its languages, as stored —
  `{groups:[{id, name:{ru,en}, description?:{…}, kinds:[{id, name:{…}, description?:{…}, container?, style?}]}],
  relationGroups:[{id, name:{…}, description?:{…}, types:[{id, name:{…}, description?:{…}, style?}]}]}`.
- With `lang=<code>`: the same tree with `name` and `description` picked in that language (else any
  present, the first by code) and `container` always present — the shape of `get_kinds` (§6).

The dictionary is not an enum: an entity `kind` or a relation type outside it is legal and only
listed by `semaps check` («не из словаря»).

### 2.2. Workspace index: `GET /api/workspace`

The one listing a host gives. It walks `projects/*/` (a folder counts only with a `project.json`) and
`views/*.view.json` in each; nothing else is listed, and plain directory listing stays refused
([`ADR_20260923-7`](adr/ADR_20260923-7_contract_projects-and-views-found-not-listed.md)).
Which projects and views exist comes from disk: creating one is structural and written at once
(§3.4a). What they are called and how they look — `title`, `subtitle`, `icon`, `theme`, `order`,
`languages`, a view's `axis` and `names` — comes from the working model, unsaved edits included.
MCP `list_projects` gives the same answer (§6); clients do not merge anything themselves.

```json
{ "projects": [
  { "id": "shop", "title": "Магазин", "subtitle": "…", "icon": "folder", "theme": "blue", "order": 1,
    "languages": ["ru"],
    "views": [
      { "id": "v_main", "file": "projects/shop/views/v_main.view.json", "axis": "axis_subsystem",
        "icon": "map", "theme": "blue", "order": 1, "names": { "ru": "Общая картина" } } ] } ] }
```

- `title` falls back to the folder name; `names` holds `name` under the view's id from each
  `text.<lang>.json` of `languages` (empty when there is none).
- Projects and views are sorted by `order`, entries without one last, then by `id`.
- A file that does not parse stays in the list with an `error` string instead of its fields.
- No `projects/` folder is an empty list, not an error. Served with `Cache-Control: no-cache`.
- Reference implementation: `core.LiveIndex` over `core.Index` (`core/index.go`).

### 2.2a. Base URL Handling
- The editor bundle is **not** part of the workspace. The reference host serves it at `/app/` from its own installation (`host/app/`); the workspace is served at `/`.
- When served from `/app/`, relative model paths resolve relative to `../` (workspace root).
- In development mode (Vite dev server), model paths resolve relative to `./` (mounted public root).

### 2.3. Caching

Serve every `.json` with `Cache-Control: no-cache` (or an equivalent validator). Model API responses are working state and must not be cached. Application assets under `/app/assets/*` are content-hashed and may be cached normally.

---

## 3. Write Requirements (Save API)

The backend persists project model files through `POST /api/model/{project}/save` (§3.4) and
creates projects and views through §3.4a. `/api/save` remains for global assets only.

### 3.1. Save Endpoint: `POST /api/save`

- **URL Pattern**: `/api/save?file=<relative_path>`; any path under `projects/` returns `410 Gone`.
- **Method**: `POST`
- **Headers**: `Content-Type: application/json`, `Authorization: Bearer <host key>`
- **Request Body**: Valid JSON payload formatted with 2-space indentation.

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
| `410 Gone` | A path under `projects/` | Use §3.4 / §3.4a |
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
key like `/api/save`. The host changes the project or view `id`, the `project` field in view files,
the default view reference, and text keys as needed. It then reloads the model and emits
`projectReloaded` to subscribers.

### 3.4. Working model API

The running host owns one in-memory `core.Model` per project. Contract files stay unchanged until Save. Every accepted batch is appended to `<project root>/.semaps/work/<project>.jsonl`; startup replays it. A project-wide Save writes dirty files in contract v5 format (two spaces, no HTML escaping, existing key order), then clears the journal. Discard reloads the selected scope from files and rewrites the journal for remaining edits. There is no host Undo or field merge.

The host creates `<project root>/.semaps/host.json` containing `{pid, port, key}`. The key is also embedded as `<meta name="semaps-key" content="…">` in `/app/` HTML. Every mutating endpoint requires `Authorization: Bearer <key>` and the existing same-origin check; missing/wrong key returns `401`. This includes `/api/save`, `/api/move`, and the tool API writes. `.semaps/` is never served as workspace static content. The host listens on localhost.

| Path | Method | Response / effect |
|---|---|---|
| `/api/model/{project}` | `GET` | `{project, registry, texts, views, viewFiles, dirty}`. `project` is the manifest; `registry` maps registry filenames to full documents; `texts` maps languages to full text documents; `views` maps view ids to view properties without geometry; `viewFiles` maps view ids to workspace-relative paths; `dirty` is below. |
| `/api/model/{project}/views/{id}` | `GET` | Full working view document, including `placements` (one array, blocks and containers alike, `CONTRACT.md` §8.2), `edges` when the view has its own, and `relations`. A view of an older shape is not served: `422` with the file and the field. |
| `/api/model/{project}/ops` | `POST` | `{client, ops}` applies one atomic batch as `human` → `{changed, dirty}`. A broken rule returns `422` with text and applies nothing. |
| `/api/model/{project}/save` | `GET` | Current `dirty` summary: what is unsaved, for the editor's «Изменения» panel (Save itself asks nothing). |
| `/api/model/{project}/save` | `POST` | Writes every dirty contract file and clears the journal → empty `dirty`. |
| `/api/model/{project}/discard` | `POST` | `{scope:"view",id}` or `{scope:"registry"}` or `{scope:"all"}`; reloads that scope from disk → new `dirty`. |
| `/api/render/{id}` | `POST` | An editor's answer to a `render` request (below); authorised like `/api/model/*` (host key + same origin). Body `{png, width, height, problems:[{kind,ids,text}], error?}`: `png` is base64 without a prefix, `kind` is `overlap`, `clipped-caption`, `clipped-rows`, `line-through-box`, `line-crossing` or `outside-container`; on failure `{error}` only. `204` when taken, `404` when nothing waits for `id` (answered already, or timed out). |
| `/api/events?project=<id>&render=1` | `GET` | The same stream; `render=1` marks an editor that can draw a view. Only such subscribers receive events carrying `render` (below); graph-mode subscribers omit the parameter. |
| `/api/events?project=<id>` | `GET` | SSE: one `data:` JSON object per accepted batch/save/discard: `{client,author,changed,dirty}`. Clients ignore their own `client` id. A `render_view` call (§6) adds to render subscribers only an event `{author:"host",changed:[],dirty,render:{id,view,ref?,rect?,scale,maxSize}}`: draw `ref` (a `view#entity` reference: a container draws that container) or the view, or just `rect` (`{x,y,width,height}`, model units) when given, at `scale` (0.25–4), the longer side at most `maxSize` px, from the editor's current unsaved state, and answer `POST /api/render/{id}`. The host waits 20 s for the first successful answer (an error from one editor is kept while the others are awaited; all failed → the first error); no render subscriber for the project is an immediate tool error. Structural changes add `projectReloaded:{oldProject,newProject,oldView?,newView?}`. A run finishing with new facts adds `graph:{addedNodes,removedNodes,changedNodes,addedEdges,removedEdges}` (ids only, node ids for the first three, `{from,to,kind}` for the edges) instead of the whole graph — a subscriber that wants the new content re-reads `GET /api/graph/{project}`. The current editor's `ModelEvent` interface does not declare `graph`; its handler does `JSON.parse(...) as ModelEvent` with no runtime validation, so the field is present in the payload but simply unread — safe today, and there for a future graph page. |

An operation is `{kind,id,view?,lang?,value,author?}`. `kind` is `project`, `entity`, `relation`, `relationType`, `text`, `view`, or `placement`. `project` replaces the `project.json` manifest and is counted in registry dirt; its `contractVersion` may not be changed by an op. `value` is the whole object; `null` removes a placement only (its `id` is the placed entity). Registry records and the manifest cannot be removed. `text` requires `lang`; `view` and `placement` require `view`. `view` replaces the view's properties without `placements` (they change one by one). The one op `placement` covers blocks and containers alike: a placement is `{entity, parent, x, y, width, height, styleId?, override?, template?, collapsed?}` (`CONTRACT.md` §8.2). The host sets `author: human` on browser ops; the agent path sets `author: agent`. A changed reference is `{kind,id,view?,lang?,author}`. `dirty` is `{registry:[Ref],views:{<view-id>:[Ref]}}`, with the last author per touched object. The conflict unit is an object: later accepted replacement wins, with no field merge. The SSE event is emitted after the batch is in the journal. A view is loaded lazily when first read or changed.

Every batch is checked against the contract, whoever sends it — editor, agent, sync
([`ADR_20260926`](adr/ADR_20260926_core_one-rule-set-for-every-writer.md), `core/rules.go`). The
check runs on the whole batch after all its operations, so order inside a batch does not matter,
and only on what the batch creates or changes: a new object entirely, a changed one by its changed
fields. A new entity or relation needs its prefix (`e_`, `r_`); an entity a `kind` (a kind
outside the dictionary is allowed, §2.1b) and — by its `origin` — its name: an entity that comes
from code (or has no `origin`) a non-empty `name` in `entities.json`, an `authored` entity **no**
`name` there but a `name` text under its id in at least one language, in the same batch
([`ADR_20260930-5`](adr/ADR_20260930-5_contract_authored-entity-name-is-text.md)); a `name` text under
the key of an entity from code is refused, and a `title` under any `e_` key; the `code[]` of an
entity and the `evidence[]` of a relation the shape of `CONTRACT.md` §3–§4 (`lang` with `symbol`,
a `ref`, `symbol` or `via`, one entry per language), the top-level `codeRef`, `symbol` and `via` of
the earlier form are refused ([`ADR_20260930-4`](adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md));
a relation existing ends and
a type from `relation-types.json`; a text field a value `{v, origin, at}` with a non-empty `v`,
under a key with the prefix `e_`, `r_`, `rt_` or `v_` (`c_` and `z_` are gone); a placement an
existing entity, a `parent` (`null` for none) that is a container placement of the same view — an
entity whose kind is a container kind — not itself or its own content, and an `override` with only
the fields of the table in `CONTRACT.md` §11.6; a removed container placement must not leave
placements pointing at it; the `override` of an edge entry of the view's own `edges` only the
fields of the edge table. Enumerations
(`origin`, `status`, `visibility`) take only their CONTRACT values. A broken rule refuses the whole
batch with `422`. Journal replay at startup is not checked again. What binds only the agent
(`requestedByHuman`, not moving someone's placement, minted ids) stays in the MCP tools.

Renames use `/api/move`; creation uses §3.4a. These structural operations require the host key; a rename and a new view also require a clean project model — when dirty, the host returns `409` with «сначала сохраните». A rename onto a taken id is `409`, an id outside the pattern `422`. After success the host reloads the project from files and emits `projectReloaded`. Ordinary manifest edits use a `project` op and the common Save. `styles.json`, `templates.json`, and `content/` remain outside the working model.

### 3.4a. Creating projects and views

One create path for every client: these endpoints and the MCP tools `create_project` /
`create_view` (§6) call the same host service, which calls `core.CreateProject` /
`core.CreateView`. No client writes `project.json` or a view file itself.

| Path | Method | Body → answer |
|---|---|---|
| `/api/projects` | `POST` | `{id, title, subtitle?, defaultAxis?, language?, icon?, theme?}` → `{id}`. Writes `project.json` only: no views, no entities, no extractor |
| `/api/model/{project}/views` | `POST` | `{id, axis?, icon?, theme?, setDefault?, name?, language?}` → `{id, file}`. Writes an empty view (`placements: []`, no `edges` key: CONTRACT §8.5); `setDefault` rewrites `defaultView` in `project.json`; `name` goes into the working model as a text under the view's id in `language` (default: the project's first language), unsaved |

- Ids: project `^[a-z][a-z0-9_]*$`, view `^v_[a-z0-9_]+$` (`core.ProjectIDPattern`,
  `core.ViewIDPattern`); otherwise `422`. A project needs a non-empty `title`. A view needs
  an `axis` unless the project has a `defaultAxis` (CONTRACT §8.1); otherwise `422`.
- A taken id → `409`, nothing written: a create never overwrites
  ([`ADR_20260923-8`](adr/ADR_20260923-8_contract_project-and-view-ids-can-change.md)).
- A new view on a project with unsaved changes → `409` «сначала сохраните»; a view in a missing
  project → `404`.
- Both need the host key and same origin, like `/api/move`; on success the host reloads the
  project and emits `projectReloaded` (with `newView` for a view).

The editor sends operations after a completed action. Drag movement in progress remains local;
the final placement is one `placement` operation. Incoming SSE changes are read from the
working snapshot and appear in other windows without a file reload. The editor keeps Undo history
locally for its own actions; the host has no Undo. Save and discard affect the whole working model
or the requested scope and notify subscribers.

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
semaps migrate [--dry-run] [--drop-untyped-styles] [flags] [dir | file.semaps]
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
exits with `1` when anything is found. It does not serve. Besides the texts, axes and `code[].ref`
(a file that is gone; a malformed `code[]` or `evidence[]` entry; an authored entity with a `name`
text in no language) it reports the old shape (a project below contract 5, a view with `zones`/`nodes`, a
`containers.json`, `c_`/`z_` text keys, a bare string instead of a text value, `kinds` or `zone`
in a workspace file, `styleId` on a relation type, a top-level `codeRef`/`symbol`/`via`, the `name`
of an authored entity in `entities.json`), entity kinds and relation types outside the
dictionary («не из словаря»), placements that break §8.2, and styles without `forKinds`
(«без типа»).

`migrate` rewrites the workspace of a contract-3 project to contract 5 — and one already at 5 in
the earlier form of the code and name shapes to the current one — **in place**: the projects
under `projects/` and the workspace-level `styles.json` and `canvas.json`; it prints what it
did and what only a human can decide ([`ADOPTING.md`](ADOPTING.md), `core/migrate/`,
[`ADR_20260927-3`](adr/ADR_20260927-3_core_migrations-outside-the-loader.md)). It is idempotent:
what already has the current shape is left alone. All new contents are computed first and written only when the
whole workspace converted; `--dry-run` writes nothing and exits `1` when anything would change.
`--drop-untyped-styles` also drops the workspace styles that name no type (`forKinds`); the
placements and edges of the views that named them lose the `styleId`, and the report says which.
The language of the code realizations comes from the `extractors` of the `.semaps` file — when all of
a project's extractors have one language; otherwise `migrate` stops and asks (nothing is written).
It does not serve and needs no running host.

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
| `source_root` | `.` | what the `ref` of `code[]` and `/api/source*` resolve against |
| `port` | `8777` | if busy, the next free port is taken |
| `extractors` | — | list; each: `id` (lowercase, digits, `-`), `language`, `project` (model project under `projects/`), `root` (default `.`), `include`, `exclude`, `edges` (optional list: `holds`, `uses`, `injects`, `calls` — `calls` prints methods and the `calls`/`constructs`/`overrides` edges of `ADR_20260928-4`, dynamic data of the live graph only, never synced), `implements` (optional list of full names of external interfaces, Go only: passed as `--implements`, `ADR_20260927-4`; written by hand, not settable through the setup API), `watch` (default `false`: the host watches this entry's sources and reruns it on a change, `docs/plans/PLAN_20260928-4_host_watch-sources.md`), `command` (replaces the found extractor; written by hand only) |
| `mcp` | — | see below |

`mcp` (`PLAN_20260928-7` step 1) is the settings that shape the MCP tools; every key is optional:

| Key | Default | Meaning |
|---|---|---|
| `tools` | `one` | `one` or `narrow` — which tool set is served (`PLAN_20260928-7` step 4) |
| `description` | `standard` | `brief`, `standard` or `full` — how much a tool's own description says (step 4) |
| `format` | `facts` | the name of a registered graph format (`graph_formats`): the MCP tool `get_graph`'s own default answer format. The HTTP endpoint `GET /api/graph/{project}` keeps its own default, `json`, regardless of this key |
| `list_cap` | `50` | how many names/lines one relation prints ({fromMethods}, {toMethods}, {relationLines}) before `+N` — a request parameter too (below), which wins over this |
| `limit` | `200` | how many nodes an unbounded (no `around`, no `container`) answer is cut to — a request parameter too (below), which wins over this |

An unknown key, or a value outside the ranges above, is refused in the same error style as a bad
`extractors:` entry. The file is edited through the YAML tree, exactly like `extractors:`:
comments and key order survive.

`GET /api/setup` (§5) returns `mcp` with every key filled in with its default; `PUT /api/setup`
accepts `mcp: {…}`, any subset of the keys (camelCase in the request body: `tools`, `description`,
`format`, `listCap`, `limit`). The change is written to the file and, in the same request, reaches
the running host's in-memory settings — every `GET /api/graph/{project}` and every MCP `get_graph`
call from the next one on uses the new `format`/`list_cap`/`limit`, with no restart (step 2). A
change to `tools`/`description` rebuilds the MCP server's graph tool list in place (the Go SDK's
`Server.RemoveTools`/`AddTool`, which both send `notifications/tools/list_changed` to every
connected session) — an already-open session sees the new list once its client re-lists tools
(the SDK gives no way to change *which tools a session is mid-call with* without that
notification round-trip); a brand new session always gets the current set and level from its
first `tools/list`. `semaps mcp`'s stdio proxy copies the remote tool list once, at startup, and
does not watch for a later change — an agent using it needs its session restarted (`semaps mcp`
started again) to pick up a `tools`/`description` change (`PLAN_20260928-7` step 4).

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
| `/api/setup` | `GET` | → `{projectFile, name, workspace, sourceRoot, port, extractors: [Extractor], languages, mcp}` — keys as written, `mcp` with its defaults filled in |
| `/api/setup` | `PUT` | `{name?, workspace?, sourceRoot?, port?, mcp?: {tools?, description?, format?, listCap?, limit?}}` → setup. Workspace, source root and port apply after a restart; `mcp` applies at once (step 2 above) |
| `/api/setup/extractors/{id}` | `PUT` | `{language?, project?, root?, include?, exclude?, edges?, watch?}` → setup; adds the entry when missing. Any other field (`command`) → `400`. Toggling `watch` starts or stops that extractor's watcher at once, no host restart |
| `/api/setup/extractors/{id}` | `DELETE` | → setup |
| `/api/runs?extractor=<id>` | `GET` | → `[Run]`, newest first; five person/agent runs and two watch runs (`trigger: "watch"`) kept per extractor, counted apart |
| `/api/runs` | `POST` | `{extractor}` → `202` `Run` (`state: running`). Only runs the extractor and keeps its facts under the runs directory; it never writes the registry — only `POST /api/runs/{id}/sync` does. The live graph reads these facts, so this is also what the graph mode's «Обновить граф» calls (the same start the watch mode makes, apart from `trigger`) |
| `/api/runs/{id}` | `GET` | → `Run` |
| `/api/runs/{id}/log?offset=<n>` | `GET` | → `{text, offset, state}`: the log from byte `n`; ask again with the new `offset` while `state` is `running` |
| `/api/runs/{id}/sync` | `POST` | `{dryRun, noRenames}` → `{report, exitCode, empty}`; the report is `core.SyncReport`. Applies the facts of that run to the working model (unsaved), never extracts again |
| `/api/mcp` | `GET` | → `{file, exists, configured, entry, onPath, snippet, tools: [{name, description, readOnly, inputSchema}], mcpTools, mcpDescription, combos: [{tools, description, graphTools: [{name, description, readOnly, inputSchema}], instructions, bytes, estimateTokens}], error?}`: does `.mcp.json` at the project root start `semaps mcp` (§6), and the tools it offers now, at the live `mcp.tools`/`mcp.description` settings (`mcpTools`/`mcpDescription`). `combos` (`PLAN_20260928-7` step 5) has all six `tools`×`description` combinations, whichever is live or not — each with the graph tools that set would register, at that level's full description text, that combination's server `instructions` text (the text the session would actually receive, `serverInstructions`), and the whole combination's size: `bytes` sums the tool names, descriptions, parameter (JSON-schema) descriptions, and `instructions`; `estimateTokens` is `bytes / 4`, an estimate, not a real tokenizer count |
| `/api/mcp/install` | `POST` | → the same, after adding the `semaps` entry to `.mcp.json` (created when missing; other servers and keys stay; a file that does not parse → `409`) |
| `/api/mcp/call` | `POST` | `{name, arguments}` → `{messages: [{dir: out \| in, message}], ms, isError, error?}`: the editor's sandbox. Runs one tool through the host's `/mcp` endpoint; `messages` are the JSON-RPC call and answer, with the handshake left out. A writing tool changes the shared working model and remains unsaved. |
| `/api/graph/{project}` | `GET` | → `{nodes, edges, facts: [{extractor, run?, finished?, language, lastRun?, lastRunFailed?}], stats: {nodes, edges, byPresence, hiddenMissing: {nodes, edges}}}`: the live code graph (`docs/adr/ADR_20260928_host_live-code-graph.md`, `docs/adr/ADR_20260928-3_host_calls-live-in-the-graph.md`), joining `project`'s live working model with the latest successful run of each of its `.semaps` extractors (an entity joins a symbol by the pair — the run's `language`, the `symbol` of that language's entry in its `code[]`; a model-only node is named by `EntityName`, an authored entity by its `name` text). Each `facts` item's `lastRun` is the id of that extractor's newest run in any state, and `lastRunFailed` says whether it failed — apart from `run`/`finished`, which stay whichever done run's facts the graph actually used (a failed run never changes the graph). No run at all for an extractor: it is left out of `facts`. No successful run yet for one that has run: `run`/`finished` are absent from its item, and the graph carries only model nodes for it. A node also carries `namespace` and `assembly` — the full name of its namespace and the name of its assembly, computed once when the graph is built, never by a per-request walk — and, on a neighbourhood answer (`around` given), `step`, its distance from the focus node. A node also carries `container: true` when its kind is a container kind of the dictionary (§2.1b), and `containers`: at most one id, the innermost container that `contains` it (a `contains` edge from a container node — whether the extractor reported it or a person wrote it as a relation); none when it lies in no container or in two unrelated ones, nothing is guessed ([`ADR_20260930_contract_graph-containers-from-contains`](adr/ADR_20260930_contract_graph-containers-from-contains.md)). A `GraphEdge` also carries `status` (the relation's `present`/`missing`, CONTRACT.md §4) for a `both`- or `model`-presence edge; a `code`-only edge has none, and — only on an edge `lift=types` produced (below) — `count` (how many method-level edges were merged into it), `fromMethods`/`toMethods` (the short names of the methods lifted from each end, sorted, de-duplicated). Query parameters, all optional: `missing` (`1`/`true` to include; absent or `0`/`false`, the default, leaves out model-only nodes whose entity has status `missing` and model-only edges whose relation has status `missing`, plus every edge touching a node left out this way — a node/edge with presence `both` or `code` is never dropped by this; `stats.hiddenMissing` counts what was left out, so a reader knows it exists even at the default); `level` (`types` drops `function`/`value`/`method` nodes and the edges touching them; `all` keeps everything — default `types` for a whole-graph request (no `around`/`container`, ADR_20260928-3 §7), default `all` for a neighbourhood); `lift` (`types` or `none`, ADR_20260928-3 §6: `types` moves every edge with a `method`-kind node at either end to the type — or interface or module — that `contains` it, before `level`/`follow` run, so `follow=calls` from a type walks type to type; an edge that becomes a type to itself (recursion, or two methods of one type calling each other) is dropped; edges that become equal (same from, to, kind, type) after lifting are merged into one, keeping `count`/`fromMethods`/`toMethods`, above. Default: with `around`, `types` unless the resolved node is itself a `method`; without `around`, `types` exactly when `level` is also `types` (the whole-graph default) — so the whole-graph page shows calls/constructs between types by default, and `level=all` shows the raw method edges); `around` (a node id **or a name**, resolved as part 2 below: only its neighbourhood) with `follow` (comma list of directed relation names, below — a neighbourhood request only, `kinds` is refused alongside it) and `depth` (1–5, default 1, or `all`: walk until no new node is reached — only when **every** relation in `follow` is an inheritance relation, `extends`/`extended-by`/`implements`/`implemented-by`; with any other relation in `follow`, or with `follow` absent, `all` is a `400` that names the allowed relations) and `fanout` (a non-negative integer, 0/absent = unlimited: at most this many fresh neighbours per node per relation are taken into the walk, the rest reported, never silently dropped from the count); `kinds` (comma list of edge kinds to keep — **whole-graph request only**, no `around`; default: every kind); `container` (a container node — an id **or a name**, resolved like `around`; its kind is a container kind of the dictionary, §2.1b: only it and the nodes it contains at any depth, and the edges between two kept nodes. A node that is no container → `400`, an ambiguous name → `409` with the candidates, no such node → `404` with the nearest names. A node's own `containers` is unchanged, see below); `fields` (comma list from `members`, `via`, `position`, `memberLines`, `dynamic`. The parameter *absent* means the default, `via,position,dynamic`; the parameter *present but empty* (`fields=`) means none — `members` adds a joined entity's own `members`; `position` is `file`, `line`, `endLine`, `spans` of a node and `line`, `file` of an edge; `memberLines` is its own name, off by default, [`EXTRACTOR.md`](EXTRACTOR.md) §2; `dynamic` is a node's marks of blind spots, on by default (`PLAN_20260928-7` step 6, below) — applied by the `json`/`json-compact` formats only, see below); `limit` (a positive integer: an *unbounded* answer — no `around`, no `container` — is cut to this many nodes, edges between two kept nodes kept, the rest dropped; default the `.semaps` `mcp.limit` setting, `200` unless changed, `PLAN_20260928-7` step 2 — a bounded request, `around` or `container`, is never cut by this); `list_cap` (a positive integer: caps the names/lines a text format prints for one relation — `{fromMethods}`, `{toMethods}`, `{relationLines}` — before `+N`; default the `.semaps` `mcp.list_cap` setting, `50` unless changed; has no effect on `json`/`json-compact`); `format` (the answer's shape, below — the HTTP endpoint's own default stays `json`, `mcp.format` only changes the MCP tool's default) or `template` (a template of your own, part 3 below — conflicts with an unknown `format`, but a `template` always wins when both are given). Filters apply in this order: name resolution of `around` → `missing` → `lift` → `level` → `follow`+`fanout` (a neighbourhood) or `kinds` (a whole graph) → `container` → `limit` → `fields` → `format`/`template`. An unknown `around` name → `404` with the nearest names by edit distance; several candidates → a `409` listing them (id, kind, file:line); a node hidden by `missing` → `404` saying so and how to include it. An unknown `follow`/`fields`/`format` name, a bad `level`/`depth`/`fanout`, `around`+`kinds` both given, or a malformed `template` → `400`. Unlike the rest of this table, this is a plain read with no side effect, so it needs no host key — matching the `GET`s of §3.4, which likewise skip `authorize`. A text format's first line (after the name-resolution notice, if any) names anything the answer was cut by — `limit`, `fanout`, `list_cap`, or (asked with `lift=none`) a type/module whose own relations are empty while its methods' are not — and how to ask again for the rest; the trailing counts line (`N nodes (T types, M methods), E edges` — a module counts as a type) never repeats it. |
| `/api/graph/{project}/find?q=<text>&limit=` | `GET` | → `{candidates: [{id, kind, file?, line?}]}`: part 2's plain substring search (case-insensitive) over node names and ids, for a caller unsure of the exact spelling to give `around`. `limit` defaults to 50. |
| `/api/graph/{project}/groups` | `GET` | → `{containers: [{id, name?, parent?}], axes: [{axis, containers: [{id, name?, parent?}], of: {<entity id>: <container entity id>}}]}`: what the graph mode groups nodes by beyond the facts (read-only, from the working model, unsaved edits included). `containers` are the container nodes of the graph (ids are graph node ids) with their nesting: `parent` is the container that contains them (`contains` between container nodes, [`ADR_20260930_contract_graph-containers-from-contains`](adr/ADR_20260930_contract_graph-containers-from-contains.md)); none when there is none or it is ambiguous. `axes` has one item per axis the project's views declare — a view without `axis` takes `project.defaultAxis`, and one with neither is left out (CONTRACT.md §8.1) — with the container entities placed on those views (ids are entity ids) and, per entity, the container its placement lies in (`placements[].parent`); an entity placed outside any container (`parent: null`), or on no view of the axis, is absent from `of` (the client calls that «вне рамок»). |
| `/api/graph-formats` | `GET` | → `{formats:[{name,description,mediaType,template?}], relations:[{name,inverse,kind,typeMatch,description}], terms:[{name,description}], defaultFollow:[name], template:{rules,examples}, defaults:{format,level}}`: the formats (with a text format's own stored template), the relation vocabulary `follow` accepts, `terms` — one line each for the words of the registry's relation types and statuses that vocabulary does not explain (`holds.one`, `holds.optional`, `holds.many`, `holds.many.ro`, `holds.keyed`, `holds.*.internal`, `injects`, `uses`, `references`, `present`, `missing`, `planned`, and a node's presence `code`/`model`/`both`) — the template macro dictionary and rules with worked examples, and the defaults — so a caller never has to guess a name or the template grammar. |

**The relation vocabulary (`follow=`).** Every relation has two names, one per direction, and the same words are used in a request (`follow=`) and in an answer (the `{relation}` macro, part 3). `injects`/`injected-into` is the `uses` relation narrowed to a constructor parameter; plain `uses`/`used-by` excludes it. `holds`/`held-by` covers every cardinality (`holds.one`, `holds.many`, …); `holds.many`/`held-by.many` narrows to one. The full table (`core/graph_relations.go`, also returned by `graph_formats`):

| Name | Inverse | Fact kind |
|---|---|---|
| `extends` | `extended-by` | extends |
| `implements` | `implemented-by` | implements (a type, or a method implementing an interface method) |
| `overrides` | `overridden-by` | overrides (a method) |
| `holds` | `held-by` | holds, any cardinality |
| `holds.many` | `held-by.many` | holds, narrowed to cardinality many |
| `uses` | `used-by` | uses (excludes `injects`) |
| `injects` | `injected-into` | uses, narrowed to a constructor parameter |
| `calls` | `called-by` | calls (a method) |
| `constructs` | `constructed-by` | constructs (`new T(...)`, a method to a type) |
| `depends` | `depended-on-by` | depends |
| `contains` | `inside` | contains |

**`follow` (neighbourhood requests only).** The walk goes along exactly the named relations, in exactly the directions named — `follow=extended-by` at depth 2 gives descendants and never climbs to an interface's other implementers. An unknown name is a `400` listing the vocabulary. Left out (absent `follow`): `DefaultFollow`, a constant naming every relation but `contains`/`inside` — `extends, extended-by, implements, implemented-by, holds, held-by, uses, used-by, injects, injected-into, calls, called-by, constructs, constructed-by, overrides, overridden-by, depends, depended-on-by`. There is no more named `set` and no undirected `kinds` for a neighbourhood — a whole-graph request keeps `kinds` for filtering by fact kind (below), since the graph page needs it and has no "direction" to speak of.

**Finding a node by name (`around=`, part 2, then defects 8/9/11).** `around` accepts a full id, or a name. Types/interfaces/modules (and function/value symbols) are tried before methods — a method is tried only when nothing else matched, or the query is method-shaped (contains `(`, or has the `Type.Member` form): asking `Runner` resolves to the type `Runner`, never made ambiguous by a constructor or method of a similar name, even in a graph with ~1900 methods; asking `YoloObbFactory.CreateRunner` resolves to that method (every overload sharing that name, when there is more than one). Within each of those two tiers, resolution is tried in order, case-sensitively then (only if nothing matched) case-insensitively: exact id; id without the `<extractor>:` prefix; full name without arity (a generic type's `` `N ``); short name with or without arity; for a method, its printed short signature (below) or, for the coarser `Type.Member` form, its container's short name plus its own (arity stripped). A `method`-kind node prints, in every text format, its short signature — `Container.Method\`arity(ShortParamTypes)`, not its full id (up to 250 characters; `{id}` still gives that) — and that printed name always resolves back to the same method via `around` (or lists its overloads).

Exactly one candidate → the answer is for it, and begins with a notice line saying what happened, unless the id matched exactly (no notice then): a short name (`` asked `OnnxExecutionContext` (a short name); taken `nmfn:NeuroModFlowNet.ONNX.OnnxExecutionContext` ``); a missing generic arity (`` asked `IRunner`; no `IRunner` without type parameters; taken `nmfn:…IRunner`2` ``); a case difference (`` asked `x`; case differs; taken `X` ``); a method match (says so too). Several candidates → no graph: a `409` listing them (id, kind, file:line) for the caller to ask again — several method overloads included. None → a `404` with up to 5 suggestions, each `shortName (fullID)`: compared against SHORT names (a method's `Type.Member` form), types before methods, kept only within a third of the asked name's length (at least 1, at most 3) — `IRaner` suggests `IRunner\`2 (…)` first, not an unrelated name past that distance. A node that exists but is hidden by `missing=0` (the default) → a `404` saying exactly that and how to include it (`missing=1`).

**Answer formats (`format=`).** Several formats exist side by side (`core/graph_format*.go`); a format receives the already-filtered graph, the focus node (if `around` was given), and the name-resolution notice, and never filters on its own. A text format's shape is nothing but a stored template (part 3) — pass your own with `template=` instead of `format=` to get a different shape without a code change.

| Format | Media type | Shape |
|---|---|---|
| `json` (default of the HTTP endpoint) | `application/json` | `{nodes,edges,facts,stats}`. `GraphNode`/`GraphEdge` as always, plus `namespace`/`assembly` and, on a neighbourhood, `step`. E.g. one node: `{"id":"cs:App.Guards.RepetitionGuard","name":"RepetitionGuard","kind":"type","file":"src/Guards/RepetitionGuard.cs","line":10,"endLine":40,"namespace":"App.Guards","assembly":"App"}` |
| `json-compact` | `application/json` | short keys, nothing repeated: `language`/`extractor` once at the head; a `files` table, nodes/edges refer to a file by index (`fi`); **edges refer to nodes by id**, not by index (an index is easy to misread across a large answer). E.g. `{"language":["csharp"],"files":["src/A.cs"],"nodes":[{"i":"cs:A","n":"A","k":"type","fi":1,"l":10}],"edges":[{"f":"cs:A","t":"cs:B","k":"extends"}]}` |
| `facts` (default of `get_graph`, `core.DefaultToolFormat`, still provisional) | `text/plain` | one line per neighbour: `{step} {fullName}  {file}:{lines}  [\[{relations:{relation}[ {member}[:{memberLine}]][ {memberKind}][ {modifiers}][ [×{count} ]from {fromMethods}[ to {toMethods}]][ @{relationLines}[ in {relationLinesFile}]][ {injected}]\|; }\]][ via {via}]`. The focus node first, with where it lies; relations are in square brackets (escaped `\[`/`\]` in the template, part 3), separated by `; `. Several relations to the same neighbour are one line; a `holds`/`held-by` and the `injects`/`injected-into` of the same member of the same pair (compared case-insensitively, one leading underscore ignored) print as one relation, ending in `(injected)`; a lifted relation (`lift=types`) names its methods — `from {fromMethods}` (`to {toMethods}` when the far end was also lifted) — with a `×{count}` prefix only when more than one edge merged, and, when the merged call/construct sites share one file, `@{relationLines}` (at most 8, then `+N`) — with `in {relationLinesFile}` appended only when that file is not the one already printed on this line (the "places of calls" trap: the sites are always in the file of the node the relation starts FROM, which on a `calls`/`constructs` line is the parent, not the neighbour named on the line). At step >= 2, a trailing `via {via}` names the node(s) this one was reached from. E.g.: <br>`0 App.Guards.RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40`<br>`1 App.Middleware  src/Middleware.cs:1-60  [extends]`<br>`1 App.Guards.RepetitionGuard.Log  src/Guards/RepetitionGuard.cs:12  [held-by Log:15]`<br>`1 App.Widgets.Widget  src/Widgets/Widget.cs:3-20  [constructed-by ×3 from Create,List to Widget]`<br>`2 App.Guards.SubGuard  src/Guards/SubGuard.cs:1-5  [extended-by] via RepetitionGuard`<br>`1 App.OnnxModel  src/OnnxModel.cs:14-164  [calls ×3 from CreateRunner to PrimaryInputName,PrimaryOutputName,Session @38,41,44 in YoloObbFactory.cs]` (the focus, YoloObbFactory, is the caller — not App.OnnxModel, whose own file this line already gives) |
| `lines` | `text/plain` | one block per node: a header line (full name, position, namespace), then its relations to the node it was reached from, each with the member name and line — the same relation words, held+injects merge and lifted naming as `facts` (defect 7: the same shared functions, not a copy). |
| `locations` | `text/plain` | the degenerate one: `fullName<TAB>file<TAB>line<TAB>endLine`, one line per node, no relations at all; a namespace/assembly node prints no file. |
| `tree` | `text/plain` | indented by walk depth from the focus node, `relation [member] → name  file:line`; a node met again is printed as a reference **once per parent**, never a flood of "see above" for containment. Refused (`400`) without `around`. Same relation words/merge/lifted naming as `facts` (defect 7). |

Every text format's `relation` (and a method-kind node's printed name) is worded identically for the same edge/node: `facts`, `lines` and `tree` all name a relation from the point of view of the node the walk came FROM (defect 7) — the exact word `follow=` would take to keep walking that way — and a `method`-kind node prints its short signature (`Type.Method\`arity(ShortParamTypes)`, not its up-to-250-character full id; {id} still gives that) in all three, resolvable back via `around`.

Every text format ends with a short counts line (`N nodes, M edges`), plus a note when the answer was cut by a limit or by `fanout`, and begins with the part 2 notice when there is one. Paths are printed exactly as the facts give them (forward slashes, relative to the source root). `fields` applies to `json`/`json-compact` only; a text format always shows position (it is the format's point) and never shows `via`/`members` as separate data.

**Templates (part 3, then defects 2-6 of the agent-answers-graph task).** A text format's line is produced by a small template language, documented in full by `graph_formats` (rules and macro dictionary, with worked examples) so an agent can write its own after reading only that. In short: literal text is copied as is, subject to the whitespace rule below; `{macro}` is replaced by its value or nothing; `[...]` is an optional group, printed as is when every macro (or nested `{relations: ...}` block) directly inside it has a value, dropped whole (with its literal text) when any one is empty — this is the one rule that keeps `holds Context:7` and a bare `extends` both clean; `\[`/`\]` are literal brackets, needed since `[`/`]` are the optional-group syntax and `facts` wraps its relations in real ones; `{relations: TEMPLATE | SEPARATOR}` renders TEMPLATE once per relation reaching the node from the node it was reached from, joined by SEPARATOR.

**Whitespace rule.** After rendering a template's own top-level pieces (and, the same way, one `lines`/`tree` relation line), a run made only of the space character `' '` is dropped when it falls in the *leading* run (everything from the start up to and including it renders empty) or the *trailing* run (everything from it to the end renders empty); a space run with real content on both sides — even when the piece immediately touching it on one side is an empty macro or a dropped group — is kept exactly as written. A tab or newline is never touched (so `locations`' tab-separated columns stay put even when a value is empty). This is applied once, to a template's own top level, not separately inside every nested `[...]` group — a group that carries its own leading/trailing separator (`[×{count} ]from ...`) keeps it even when a neighbouring group renders empty.

Node macros: `step name fullName id kind nativeKind visibility file line endLine lines namespace assembly containers presence status entity via viaFullName dynamic`. Relation macros (inside a `relations` block only): `relation member memberKind memberLine modifiers cardinality type text relationLine relationLines relationLinesFile count fromMethods toMethods injected`.

A node macro `{dynamic}` prints a node's marks of blind spots as `kind @line; …`, cut at `list_cap`; on a type after lifting each mark names its method (`in Method`) and, for a partial type, the file. `\{` and `\}` are literal braces, as `\[` and `\]` are literal brackets.
- `relation` is always named from the point of view of the node the walk came FROM — the same rule in `facts`, `lines` and `tree` alike (defect 7), and the same word `follow=` would take to keep walking that way.
- `{memberLine}` is where the member is declared — for a `holds`/`uses`/`injects` edge, simply that edge's own line (EXTRACTOR.md §2.2). `{relationLine}`/`{relationLines}` is the call/construct site(s) — a `calls`/`constructs` edge only. The two are never both set on the same relation.
- `{relationLinesFile}` (defect B, "places of calls"): set only when those call/construct sites are NOT already in the file printed on this line — the file of the node the relation starts FROM (its e.From), which on a `calls`/`constructs` line is the parent (not shown elsewhere on that line) and on a `called-by`/`constructed-by` line is the node the line names (already shown, so empty).
- `{via}`/`{viaFullName}` (short/full name of the node(s) this node was reached from, comma-separated, several parents all named once) are non-empty only at step >= 2.
- `{count}`/`{fromMethods}`/`{toMethods}` are set on a relation `lift=types` produced (above) — `{count}` is empty for an unmerged (single-edge) lift, so a plain `constructs`/`calls` line does not gain a stray `×1`, but `{fromMethods}`/`{toMethods}` still name the method(s) even then.
- `{injected}` is the literal `(injected)` on a `holds`/`held-by` relation that combineHoldsAndInjects folded the matching `injects`/`injected-into` of the *same member* (compared case-insensitively, one leading underscore ignored) *of the same pair* into; empty on every other relation, including a lone `injects`/`injected-into`.
- A `method`-kind node's `{fullName}` is its short printed signature (`Container.Method\`arity(ShortParamTypes)`), never its full id — `{id}` still gives that (up to 250 characters). The printed short name always resolves back to the same method (or its overloads) via `around`.

A malformed template (unmatched `{`/`[`/`]`, or an unknown macro) is a `400` naming the position and the macro. In `facts`, `relation` is the directed name from the point of view of the node the walk came FROM (e.g. `holds`, not `held-by`, read from the holder) — the exact word a `follow=` request would use to continue in that direction; `lines` and `tree` still name a relation from the node's own point of view (unchanged, not part of this pass).

`Extractor`: `{id, language, project, root, include, exclude, edges, watch, command?, tool: {language, found,
source?: command|bundled|path, where?, runtime?, problem?}, lastRun?: Run}`.
`Run`: `{id, extractor, project, language, trigger?: "watch", started, finished?, seconds?, state: running|done|failed,
exitCode, error?, stats?: {symbols, edges, symbolKinds, edgeKinds, language}}`. `trigger` is absent for a run a
person or an agent started, `"watch"` for one the host started on its own after a source change
(`docs/plans/PLAN_20260928-4_host_watch-sources.md`).

A run's facts never enter the workspace; they stay in the temp directory beside its log.

The tool pages are modes of the one editor page: `/app/#extract`, `/app/#project`, `/app/#mcp`. Short addresses `/extract` and `/setup` redirect there.

## 6. MCP: `semaps mcp`

`semaps mcp [--project <id>] [dir | file.semaps]` provides MCP over stdio as a proxy to the
running host's Streamable HTTP endpoint `/mcp` (official Go SDK,
[`ADR_20260924-5`](adr/ADR_20260924-5_host_mcp-server.md)). It reads the host port and key from
`<project root>/.semaps/host.json`. When the host is absent, it starts the same binary with
`--here --no-browser`, waits for the endpoint, then connects. Roots are found from a `.semaps`
file upward from the current directory. stdout is only the protocol; diagnostics go to stderr.
`--project` supplies a default project when the workspace has several. The host also serves
`/mcp` directly, requiring the same bearer key as other writes; the editor sandbox uses it too.
All MCP reads and writes use the host's live `core.Model`. Writes are journaled and visible to
subscribers; contract files change only on `save`.

The tools are thin: every rule of writing is a function of `core/` (`core/edit.go`), and a broken
rule comes back as a tool error (`isError: true`) whose text names the rule. Nothing was written
then.

**Promises.**
- `id`s are minted by `core`, never passed in: `r_<from>_<to>_<type>` for an authored relation,
  `_2` on collision.
- Texts are written `origin: authored`, `at` = now in UTC; `from`/`fromHash` of a translation go.
  A relation with `origin: code` takes no text (CONTRACT §4); a `name` text is taken only by an
  authored entity (CONTRACT §7.1) — an entity from code has its name in `entities.json`.
- **Nothing is deleted**: there is no tool for it.
- Geometry of a view is written only by `place_entities` and the geometry tools below with
  `requestedByHuman: true`, and an entity already on the view is refused, not moved (CONTRACT §8.2
  p. 3). An agent works on the canvas **only through these tools**, never by editing files;
  `layout_guide` says how. The host does not snap to the grid (§2.1a).
- A new view or project only with `create_view` / `create_project` and `requestedByHuman: true`, when
  a human explicitly asked.
- Files keep every key they had, in the order they had it. Tool edits are unsaved until `save`; a new project or view file is the exception (§3.4a).

**Object references.** A tool that takes an object of a view takes it as `<view>#<id>`
(`v_ops#e_undistort` for a container, `v_ops#e_op_crop` for a block: a placement is named by its entity), or
`<project>/<view>#<id>` when the workspace has several projects. A bare `<view>` names the view. The
editor's link `/app/#<view>?highlight=<id>` is accepted as well. An unknown view or an object that
is not on the view is a tool error naming it (`core.ParseRef`, `Model.ResolveRef`). The editor
copies the same string: «Копировать ссылку» in the block/container menu and in the Properties panel;
several selected objects give references joined by commas.

Every structured answer (`structuredContent`) is a JSON object, lists included: clients
reject anything else.

**Two independent settings shape the graph tools** (`mcp.tools`/`mcp.description` of the
`.semaps` file, §2.1a; `PLAN_20260928-7` step 4). Every other tool (`list_projects`, `get_entity`,
`set_text`, ...) is unaffected by either.

`mcp.tools`:
- `one` (default) — `get_graph`, `find_node`, `graph_formats`, as described below.
- `narrow` — ten single-purpose tools instead of `get_graph`, each with one required `name`
  (a node name, or an id from `find_node`), an optional `depth` (default `1`) and the usual
  optional `project`; `get_graph` and `graph_formats` are not offered in this set (`find_node`
  is, in both sets). Each is a thin wrapper over the exact same walk `get_graph` uses, with
  `follow` fixed and every other default (level, lift, `limit`, `listCap`, `format`) untouched —
  calling one gives the identical answer as `get_graph` with `around: name` and that `follow`.

| Tool | `follow` |
|---|---|
| `who_extends` | `extended-by`, `implemented-by` |
| `what_it_extends` | `extends`, `implements` |
| `who_holds` | `held-by`, `injected-into` |
| `what_it_holds` | `holds`, `injects` |
| `who_calls` | `called-by` |
| `what_it_calls` | `calls`, `constructs` |
| `where_created` | `constructed-by` |
| `what_is_inside` | `contains` |
| `where_it_lies` | `inside` |

`mcp.description`: `brief` (what a tool answers, one sentence, one example call for `get_graph`
and the narrow tools), `standard` (default: what it answers, its main parameters, and several
question → call examples for `get_graph`; a narrow tool's own parameters and one example call
with the first two lines of its answer), `full` (`standard` plus, for `get_graph`, the whole
relation vocabulary, `lift`, `fanout`, the template language and its macros, name resolution and
its notices; a narrow tool's own `depth` and what a deeper answer looks like). How to read a
line of the default `facts` answer, and the traps found in practice — a relation is named from
the node the walk came from; asking about a type lifts its methods' calls up to it; reading or
writing a property counts as a `calls` edge; a cut answer says so in its first line; a plain name
is enough for `around`/`name` — are said ONCE, in the server's own `Instructions`
(`serverInstructions`, `PLAN_20260928-7` step 4: the `narrow` set exists for small models, and
repeating that explanation in each of its ten tools defeated the point), not in every tool's own
description. `Instructions` is given to a client once, at initialisation, and the Go SDK gives no
way to change it on a live `*mcp.Server` — so the host keeps a small pool of them instead of one
(`mcpServerPool`, `host/mcp_http.go`). A change of `mcp.tools`/`mcp.description` (PUT
`/api/setup`) does two things at once: every `*mcp.Server` that may still have an open session has
its graph tools rebuilt in place, exactly as before (`RemoveTools`/`AddTool`, `list_changed`) —
so an already-connected session sees the new tool list as soon as it next asks for it, still with
the `Instructions` it was given at `initialize`; and a brand new `*mcp.Server` is built, with the
new `Instructions` baked in, and becomes the one handed to every session that connects from this
point on — the stdio proxy included, the next time it starts `semaps mcp` (it copies the tool
list once at startup and does not watch for a later change, as before). The pool keeps only the
most recent 8 servers (the SDK gives no way to learn a server's last session has closed, so there
is no way to prune it sooner); a session open on a server older than that keeps stale
`Instructions` and stops getting tool-list rebuilds. The texts are written out in full in
`host/mcp_descriptions.go`, not assembled from pieces, and describe only parameters that exist
now.

**Mental model.** The `Instructions` open with what the system is, before anything about reading an
answer (`mentalModel`): at `brief` two sentences, at `standard` and `full` a paragraph. It names
the three layers — the registry (entities `e_*` and relations, in git, changed only through the
tools and `sync`), the live graph (facts the extractor reads from the code, never stored in git or
in the registry) and views (a human's projection of part of the model: geometry, containers on an
`axis`) — and the cycle between them: the graph follows the code by itself, the registry only
through `sync` (`sync_preview` first, `confirm_rename`, `discard` on a human's request). It says
what `present`, `missing` (kept in the registry, ids never deleted, no longer found in code) and
`planned` mean, and which tool answers which question: structure of the code, who calls whom, all
descendants → `get_graph` with `around`/`follow`/`depth` (all descendants = `follow`
`extended-by`/`implemented-by` with `depth` `all`; on the `narrow` set the `who_*`/`what_*` tools with `depth` up to 5),
the drawn architecture → `get_view` (a container reference reads its subtree), one entity's relations
with evidence → `get_relations`, the vocabulary of relations → `graph_formats`, the kinds of entities
and the types of relations with their meaning → `get_kinds`. The `narrow` wording names only
tools that set has. The vocabulary itself is not repeated there: `graph_formats` answers it
(`terms`, §5).

**A call that does not fit.** When a tool call fails input validation (`unexpected additional
properties ["items"]`, a missing required field), the error text keeps the SDK's message and adds a
line naming the tool's own parameters, required ones first — `get_view accepts: view (required),
project` (`explainArgumentErrors`, `host/mcp_errors.go`, read from the server's own
`tools/list`). An error of the tool itself gets no such line.

**Log.** Every tool call — the tool, its arguments, time, the error if any — is a JSON line in
`<project root>/.semaps/logs/mcp-<date>.jsonl` (kept 14 days; the folder carries its own
  `.gitignore`), and a short line on stderr, which clients keep in their server logs. Calls from the
editor's sandbox land in the same file.

**Reading**

| Tool | Input | Answer |
|---|---|---|
| `list_projects` | — | the workspace index, the same as `GET /api/workspace` (§2.2): unsaved titles and names included |
| `list_views` | `project?` | `{views}` |
| `get_entity` | `id` \| `symbol` (of any entry of its `code[]`) | the entity as in `entities.json`; an authored entity — which has no `name` there — comes with its `name` taken from the text (main language, else another, else the id) |
| `find_entities` | `query?` (substring of name, symbol, namespace, id), `kind?`, `status?`, `limit?` = 50 | `{total, entities}`, each as `get_entity` gives it |
| `get_relations` | `entity?`, `direction?` = `both` \| `out` \| `in`, `type?` (a prefix ending with `.` matches `holds.`), `status?`, `limit?` = 200 | `{total, relations}` |
| `get_relation_types` | — | `{relationTypes}`: the vocabulary with `visibility` |
| `get_kinds` | `lang?` = `ru`, `project?` | The dictionary (§2.1b) in one language: `{groups:[{id,name,description?,kinds:[{id,name,description?,container,style?}]}], relationGroups:[{id,name,description?,types:[{id,name,description?,style?}]}], unknown:{kinds, relationTypes}}`. `container: true` marks a kind whose entity is a frame that holds other placements; `style` is the base style of a kind or relation type (empty: the style whose id is the type, `CONTRACT.md` §11.5). `unknown` lists what the project's entities and `relation-types.json` use that the dictionary lacks. The agent reads it before `add_entity`, `add_container` and `add_relation_type` to choose a kind or type that exists |
| `get_view` | `view` (a view id, or a container reference `v_ops#e_a` for that subtree only), `project?` | The text answer has two parts: first the canvas block (`core.Canvas.Describe`, the same lines as in `layout_guide`: units, axes, grid and that the host does not snap, block/container default and minimum sizes, caption strip, padding, gaps), then the data as JSON, also the structured content: `{view:{id,project,axis,axisInherited?,relations,routing,scope?}, placements, edges, unsaved}`. `axis` is the view's own, else `project.defaultAxis` with `axisInherited: true` (CONTRACT §8.1). `placements` is a tree: each has `entity`, `name`, `kind` (of its entity), `container` (the kind is a container kind), `parent` (`null` at the top), absolute rectangle `x,y,width,height` (default size when the file has none), `styleId`, `override`, `template`, `collapsed`; a container also has `children` (its placements, nested the same way; `[]` when it holds nothing). `edges` are the lines the view shows, each `{id,from,to,type,styleId?,override?,routing?}`: its own `edges` list when it has the key, else relations by CONTRACT §8.5. `unsaved` lists the view's unsaved objects with their authors |
| `layout_guide` | — | Markdown for working on a view: how to look (`get_view`, `render_view`), each view tool, the workflow (sample → decide → apply in steps → check → say what is unsaved, give the link), `requestedByHuman`, no grid snapping, and the canvas block. No JSON file format in it. Project-independent; the canvas numbers come from `canvas.json` (§2.1a) |
| `render_view` | `project?`, `view?` \| `ref?` (`view#entity`), `rect?` (`{x,y,width,height}`), `scale?` = 1 (0.25–4), `maxSize?` = 1600 (cap 4096) | a picture from the editor open on the project, over the bridge of §3.4 (`render=1`, `POST /api/render/{id}`): an MCP `ImageContent` (`image/png`) plus a text with the problems as lines `kind: text (ids)` or `no problems found`, the model rectangle drawn (the content bounds of the view/container, or `rect`) and a note that it is the editor's unsaved state. No editor subscribed: error «no editor is open on this project: open the view in the editor (`/app/#<view>`) and repeat»; no answer in 20 s: an error saying the editor did not answer. Read-only, no `requestedByHuman` |
| `get_text` | `lang`, `key` | the entry of `key` in `text.<lang>.json`, `{}` when none |
| `doctor` | — | `{extractors, extractorsOK, findings}`: `semaps doctor` and `semaps check` |
| `get_graph` | `project?`, `missing?`, `level?`, `kinds?`, `around?`, `follow?`, `depth?` (a number 1–5, or the string `all` along inheritance relations only — `depth` is `any` in the schema and described there), `fanout?`, `container?`, `fields?`, `limit?`, `listCap?`, `format?`, `template?` | Same parameters and order of filters as `GET /api/graph/{project}` (§5), with differences forced by the tool's typed JSON schema rather than a query string: `missing` is a plain boolean (default `false`); `follow` is a list of strings, not a comma string, defaulting to `DefaultFollow` when omitted/`null`; `fields` is a list of strings, from `members`, `via`, `position`, `memberLines`, `dynamic` — omitted/`null` is the default (`via,position,dynamic`), an explicit empty list (`[]`) is none, the same "absent vs empty" distinction §5 makes with `fields=`. `dynamic` is a node's marks of blind spots, below. `around` accepts a name as well as an id (part 2): an ambiguous name is a tool error listing the candidates, an unresolved one names the nearest matches, a hidden-as-missing one says so. A `json`/`json-compact` `format` is returned both as the tool's text content (the same bytes) and as `structuredContent`: `{nodes, edges, facts, stats, truncated, fullNodes?, fullEdges?}`. Any other format, or a `template`, is text content only, no `structuredContent`. Without `around` or `container`, the node list is cut to `limit` and `truncated: true` is reported alongside `fullNodes`/`fullEdges`, the untruncated counts — an agent asking for a whole project's graph never gets it all by accident. `around`/`container` bound the answer themselves and are never truncated. `limit` and `listCap`, when omitted/`0`, default to the `.semaps` `mcp.limit`/`mcp.list_cap` settings (`200`/`50` unless changed, `PLAN_20260928-7` step 2); given, they win over the setting. `format`, when omitted, defaults to the `.semaps` `mcp.format` setting (`core.DefaultToolFormat`, `"facts"` unless changed) — call `graph_formats` to see the alternatives, or pass `template` for a shape of your own. |
| `graph_formats` | — | `{formats, relations, terms, defaultFollow, template, defaults}`, the same shape as `GET /api/graph-formats` (§5) |
| `find_node` | `project?`, `q`, `limit?` = 50 | `{candidates: [{id, kind, file?, line?}]}`: substring search (part 2) over node names/ids, the same as `GET /api/graph/{project}/find` |
| `who_extends`, `what_it_extends`, `who_holds`, `what_it_holds`, `who_calls`, `what_it_calls`, `where_created`, `what_is_inside`, `where_it_lies` (`mcp.tools: narrow` only) | `name` (a node name or id), `depth?` = 1, `project?` | the identical text answer `get_graph` would give with `around: name` and this tool's fixed `follow` (see the table above) — same defaults, same format |
| `sync_preview` | as `sync` | as `sync`, writing nothing |

**Writing**

| Tool | Input | Effect |
|---|---|---|
| `set_text` | `lang`, `key`, `field` (`name`, `title`, `description`, `doc`, `fromLabel`, `toLabel`), `value` | one field, authored |
| `add_entity` | `name`, `kind`, `id?`, `description?`, `lang?` (default: the project's first language) | authored entity (`origin: authored`, `status: present`) **with its name as a `name` text in `lang`** in the same batch — there is no name in `entities.json` (CONTRACT §7.1); a rename later is `set_text` with `field: name`, the id stays; the id is minted from the name unless given; an existing id is refused; `kind` is a kind of `get_kinds`, a kind outside it is allowed and listed by `check`; `description` goes as a text; what an entity may be is checked by `Apply` (§3.4) as for every writer. Place it with `place_entities` (a container: `add_container`) |
| `add_relation` | `from`, `to`, `type` | authored relation; both entities and the type must exist; the id comes back |
| `add_relation_type` | `id`, `visibility?` | authored type of the project (`origin`, default visibility); its name and description are the dictionary's (`kinds.json`, `relationGroups`) and its style is a style with the type in `forKinds`, not a field of the project's file (`CONTRACT.md` §5, §11.5) |
| `set_relation_visible` | `view`, `relation`, `visible` | the relation into or out of `relations.except` against the default (CONTRACT §8.5) |
| `confirm_rename` | `entity` + `symbol` \| `relation` + `member` | answers «переименование?» of sync: the old entity's realization (the entry of its `code[]` with a symbol; an entity realized in several languages is refused) takes the new `symbol`, the old member relation the new `via.member` in its `evidence[]`; then `sync` again |
| `extract` | `extractor?` | runs the extractors of the `.semaps` file one by one → `{runs: [{run, extractor, project}]}` |
| `sync` | `extractor?`, `run?`, `noRenames?` | extracts (or takes the facts of `run`) and reconciles; the text answer is the sync report, the structured one `{reports: [core.SyncReport]}` |
| `place_entities` | `view`, `entities: [{entity, parent?, x, y, width?, height?}]`, `requestedByHuman` | puts entities on a view; `parent` is a container placement of the view (or one placed earlier in the same call); none — outside any container |
| `move_elements` | `view?`, `elements`, `dx`/`dy` or `x`/`y`, `requestedByHuman` | shifts placements, or puts the top-left corner of their common box at `x`,`y`; a container goes with everything inside it; a container that stops holding its content grows (ancestors too); written exactly as given, not snapped to the grid |
| `resize_elements` | `view?`, `elements`, `width?`, `height?`, `requestedByHuman` | a block not below `node.minWidth` × `node.minHeight`, a container not below `container.minWidth` × `container.minHeight` and not below its content (`canvas.json`, §2.1a) |
| `set_parent` | `view?`, `elements`, `parent` (an entity id of a container placement; `null` takes them out of any), `requestedByHuman` | the `parent` of the placements only, coordinates untouched; a container cannot go into itself or its own content; a parent whose kind is no container kind is refused |
| `add_container` | `view`, `entity?` \| `name` (+ `kind?` = `group`), `parent?`, `styleId?`, `x`, `y`, `width`, `height`, `requestedByHuman` | places a container: an existing entity of a container kind (`entity`), or — the entity and its placement in one batch — a new authored entity named `name` of a container kind `kind` (`get_kinds`: `container: true`); the id is minted from the name and returned as `entity` |
| `fit_container` | `view?`, `container` (an entity id or a reference), `requestedByHuman` | the container = its content plus caption strip `container.headerHeight` and padding `container.padding`, exactly, no grid rounding; ancestors grow if they stop holding it |
| `align_elements` | `view?`, `elements`, `mode` (`left` \| `right` \| `top` \| `bottom` \| `width` \| `height`), `requestedByHuman` | to the first element, as the editor's align buttons |

`elements` are entity ids of placements, or references `view#e_x`; `view` may be left out when they are references. A placement nests only by its `parent`, written explicitly (CONTRACT §8.2): what merely lies inside a rectangle is not inside it. Containers that hold each other are refused. A container is named by the `name` of its entity; there is no separate caption. There is no tool that lays containers out "like another": what should match what is the agent's judgement, made with `get_view` (eyes) and these steps (hands). `add_zone`, `set_zone` and `fit_zone` no longer exist ([`ADR_20260927-6`](adr/ADR_20260927-6_contract_v5-kinds-and-containers.md)).

Every geometry step is one batch — a bad element applies nothing — authored `agent`, and answers `{touched:[view#id…], saved:false, link}` with the link that opens the view and highlights the touched objects.
| `create_view` | `project?`, `id` (`v_…`), `name?`, `lang?`, `names?` (language → text), `axis?` (else the project's `defaultAxis`; neither is refused), `icon?`, `theme?`, `setDefault?`, `requestedByHuman` | a new empty view, as `POST /api/model/{project}/views` (§3.4a): the file is written at once and every open editor lists it; `name`/`names` are texts of the working model and stay unsaved. Refused while the project has unsaved changes. Answers `{view, project, saved, link}` (`saved: false` only when a name is left unsaved). Only when a human explicitly asked for a new view |
| `create_project` | `id`, `title?` (default: the id), `subtitle?`, `defaultAxis?`, `language?` = `ru`, `icon?`, `theme?`, `requestedByHuman` | a new hand-authored project, as `POST /api/projects` (§3.4a): writes `projects/<id>/project.json` at once (a project is a folder, not a change of another project's model, so it cannot be unsaved; the answer says so) and returns `{project, saved:true}`; an existing id is refused. Only when a human explicitly asked for a new project |
| `save` | `project?`, `requestedByHuman: true` | saves all dirty project files and clears the journal; refused without explicit human request |
| `discard` | `project?`, `scope: registry \| view \| all`, `view?`, `requestedByHuman: true` | drops the requested unsaved changes; refused without explicit human request |

**Entry** in the consuming project, `.mcp.json` at its root:
```json
{ "mcpServers": { "semaps": { "command": "semaps", "args": ["mcp"] } } }
```
