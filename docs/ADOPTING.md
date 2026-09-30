# Adopting SeMaps in a consuming project

For an agent working in **another** repository that wants a SeMaps map there. It assumes only an
installed `semaps` binary (see [README](../README.md#getting-started)), not the SeMaps source.
Format details live in [CONTRACT.md](CONTRACT.md); this file only lists the steps and the traps.

## Primary path: through MCP

An agent reads and writes the registry through MCP tools (API.md §6), never parsing files directly. Set up once, use everywhere:

1. **Project file** (same as below)
2. **MCP server configuration** in the consuming project root: `.mcp.json`
   ```json
   {
     "mcpServers": {
       "semaps": {
         "command": "semaps",
         "args": ["mcp"],
         "env": {}
       }
     }
   }
   ```
3. **Use the tools**: `list_projects`, `get_entity`, `set_text`, `add_relation`, `sync_preview`, `sync`, etc.
   The agent does not read `entities.json` or write files by hand — all changes go through tools,
   which enforce rules (id generation, provenance, deletion prevention, view restrictions).
   `semaps mcp` starts the host without a browser when needed and connects to its shared working
   model. Tool edits appear in the editor immediately but remain unsaved until the human reviews
   them and presses «Сохранить». Save the current project before assigning a new task to an agent.

**Choosing `mcp.tools`/`mcp.description`.** The `.semaps` file's `mcp:` section (API.md §2.1a, §6)
picks which graph tools the MCP server offers and how much each says about itself: a strong model
reads `get_graph`'s full parameter set on its own, so `mcp.tools: one` (the default) with
`mcp.description: standard` (also the default) works well; a small or less steerable model does
better with `mcp.tools: narrow` — one tool per question shape (`who_calls`, `what_it_holds`, ...),
each taking only a node name and depth, so there is nothing to get wrong; when context is tight
(a long task, or a model with a small window), drop to `mcp.description: brief` regardless of the
tool set — one or two sentences and an example per tool instead of the worked examples and trap
list `standard` carries. In the editor it is the section «Настройки для агента» of the MCP tab:
it shows the exact text and size (bytes and an estimated token count) of all six combinations
before you choose, and saves the choice into the `.semaps` file.

**`get_graph` vs `get_relations`.** Both read structure, but from different sources
(ADR_20260928_host_live-code-graph.md): `get_relations` answers "what the registry says" — the
authored model, entities and relations as the human and past syncs left them, exactly what is in
git. `get_graph` answers "what the code looks like now" — symbols and edges from the latest
extractor run, joined with the model, including code that was never synced and positions
(`file`, `line`) that the registry never stores. Call `get_graph` when the task needs to read
actual code — "where is this declared", "what does this file currently define", "is this call
still there" — or needs a symbol's file and line to open it. Call `get_relations` when the task is
about the authored architecture — what the model asserts should be connected, independent of
whether the latest extraction agrees. Bound `get_graph` with `around` (a node id) or `container`;
without either it cuts to `limit` nodes (default from the `.semaps` `mcp.limit` setting, `200`
unless changed) and reports `truncated: true`; a text answer says so on its own first line too,
with how to ask again for the rest (`limit`, `fanout` and `list_cap` cuts all show up there; the
trailing counts line then carries only the counts, split into types and methods).

**Asking the graph: `around`, `follow`, formats.** `around` takes an id *or a name* — `get_graph`
resolves it (exact id, then progressively looser names, case-insensitive as a last resort,
docs/API.md §5 part 2); types/interfaces/modules are tried before methods, so a common name like
`Runner` resolves to the type, never made ambiguous by a same-named constructor or method — a
method is only tried when nothing else matched, or the query itself is method-shaped (`(`, or
`Type.Member`). Several matches come back as a list to choose from, none come back with the
nearest short names (with their full ids) by edit distance, so do not spend calls guessing the
exact spelling; a resolved answer that isn't an exact id match says what kind of match it was
("a short name", "no `X` without type parameters", "case differs"), not a blanket "no such id".
A `method`-kind node prints its short signature (`Type.Method(ShortParamTypes)`), not its
(possibly 250-character) full id — pasting that printed name back into `around` resolves to the
same method, or lists every overload sharing it. Start with
`get_graph`'s default answer shape — it is deliberately provisional (`core.DefaultToolFormat`,
docs/API.md §6) — so do not assume it is JSON. Call `graph_formats` once to see the other formats,
the relation vocabulary `follow` accepts (two names per relation, one per direction — `extends` /
`extended-by`, `holds` / `held-by`, `calls` / `called-by`, …), the template macro dictionary and
rules, and the defaults. A neighbourhood walk (`around`) names exactly what to follow: leave
`follow` out for the default (everything but containment), or name `contains`/`inside` explicitly
to see what is inside a namespace or assembly. `fanout=N` caps neighbours per node per relation
when a hub node would otherwise flood the answer; the answer says what was left out.

**Two worked examples.**
- Descendants two levels deep, without climbing to an interface's other implementers:
  `get_graph(around: "OpBase", follow: ["extended-by"], depth: 2)`.
- Who holds `OnnxExecutionContext`, through which member:
  `get_graph(around: "OnnxExecutionContext", follow: ["held-by", "injected-into"], depth: 1)` — the
  default `facts` format prints one line per holder with the member name and line.

**Finding a node by name.** Unsure of the exact spelling? Call `find_node(q: "...")` (or `GET
/api/graph/{project}/find?q=`): a substring search over names and ids, returning candidates to pass
as `around`.

**Writing your own template.** `format` names a stored format, or pass `template` with your own
(docs/API.md §6 part 3): a small language of `{macro}` placeholders, `[...]` optional groups that
drop the literal text around a macro that turned out empty (`\[`/`\]` for a literal bracket), and a
nested `{relations: TEMPLATE | SEPARATOR}` block for the relations reaching a node from the node it
was reached from. `relation` is always named from that reached-from node's point of view — the
same rule in `facts`, `lines` and `tree`, and the same word `follow=` would take to keep walking in
that direction. A lifted `calls`/`constructs` relation (`lift=types`) names its methods
(`{fromMethods}`/`{toMethods}`) and, when the call sites are in one file, that file's lines
(`{relationLines}`, plus `{relationLinesFile}` when that file isn't the one already printed on the
line). `graph_formats` documents every macro with worked examples — read that before writing one,
rather than guessing the grammar.

To point an agent at an object of a view, copy its reference in the editor («🔗» in Properties, or
«Копировать ссылку» in the block/container menu): `v_ops#e_undistort`. A request that names the
reference — «сделай остальные контейнеры в `v_ops#e_transform` как `v_ops#e_undistort`» — is what
the view tools work from (the MCP tool `layout_guide` explains how; the tools are in API.md §6).
Save before such a task, so that what the agent changes is only what it was asked to change.

The agent reports each changed object and a link to the relevant view. The editor marks unsaved
registry and view changes by author; «Сохранить» shows a summary when the agent contributed.
The MCP tools `save` and `discard` require `requestedByHuman: true` and may be called only after
the person explicitly requests that action. Creating or renaming a project or view requires a
clean working model; if the host returns `409` «сначала сохраните», save first and retry.

For the first model, the steps below still apply (project file, workspace folder, entities.json),
but the agent uses tools instead of hand-editing JSON.

## Fallback path: by files

When working with the registry files directly (not recommended for agents; useful for hand edits):

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
   - `project.json`: `id` = folder name, `title`, `contractVersion: 5`, `defaultAxis`,
     `languages`, `sources.include`; optional `subtitle`, `icon`, `theme`, `order` for the catalogue.
   - `entities.json`: `e_` ids, `kind` (a kind of the dictionary — MCP `get_kinds`; the project's
     own kinds go into `<workspace>/kinds.json`, which adds to the tool's dictionary), `origin: code`
     and the `name` of the code for anything read from the code, and `code: [{lang, ref, symbol}]`
     with `ref` relative to `source_root` (a hand link to a file is `code: [{ref}]`). With an
     extractor for the language, let `semaps sync` write it (see below) instead. A container (a
     subsystem, a layer, a group) is an entity of a container kind, not a separate file. An entity
     you draw is `origin: authored` and has **no** `name` here: its name is a `name` text under its
     id in `text.<lang>.json` (through MCP `add_entity`, or `set_text` with `field: name`; the id
     never changes, the name may).
   - `relations.json` + `relation-types.json`: every `type` used must be declared in
     `relation-types.json`; its name, description and style belong to the dictionary
     (`relationGroups` of `kinds.json`).
     For a derived relation, the id is `r_<from>_<to>_<type>` without the `e_` prefix.
   - `text.<lang>.json`: descriptions for `e_`, names for `rt_`/`v_`. Every value is
     `{ "v", "at", "origin" }` — never a bare string — and `at` is UTC ISO-8601 with `Z`.
   - `views/<id>.view.json`: `id` (starts with `v_`), `project`, `axis`, and **empty**
     `placements`; optional `icon`, `theme`, `order`. The view's catalogue title is `name` under
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
    language: csharp          # or typescript, go
    project: core             # a folder under <workspace>/projects/
    root: .
    include: [src]
    edges: [holds, injects]   # optional: which member-relation edge kinds to extract
    implements: [io.Writer]   # optional, Go only: external interfaces to report implementations of
    watch: true                # optional, default false: rerun this extractor when its sources change
```

`watch: true` only refreshes the host's live code graph — the extra layer next to the model,
[`ADR_20260928`](adr/ADR_20260928_host_live-code-graph.md): a change under `root`/`include` starts
this extractor again after a short quiet period, and `GET /api/graph/{project}` (and the editor's
graph view) picks it up on its own. It never touches the registry: sync is still a separate,
deliberate step (`semaps sync`, above, or the page's "Sync" button), because otherwise unreviewed
edits would land in the model without anyone asking for them
(`docs/plans/PLAN_20260928-4_host_watch-sources.md` step 4). Switch it off the same way, by editing
the `.semaps` file or its checkbox on the editor's «Экстракторы» page; either takes effect at once,
no restart of the host needed. With `--workspace` (no `.semaps` file) nothing is watched.

## The code graph page

`/app/#graph` shows the live code graph of a project as nodes and edges — the same source
`get_graph` reads, rendered and laid out by algorithm rather than authored by hand
(`docs/adr/ADR_20260928-2_editor_graph-is-a-separate-form.md`). It colours by container,
namespace, symbol kind or code/model presence, filters and searches, and — for an extractor with
`watch: true` above — follows the graph live as sources change, without a page reload. It writes
nothing to the workspace; nothing on this page changes the registry or the `.semaps` file.

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
  the archive from Releases; `install.cmd` kills every running `semaps.exe`, this session's MCP
  server included — restart the session after it) or a runtime (.NET SDK for C#, Node 24+ for TypeScript, the Go toolchain for Go). Do not work
  around it — tell the human.
- Flags go **before** the project argument. `--facts <file>` still takes facts made by hand
  (`--facts -` reads stdin); keep such files out of the workspace and out of git.
- Only symbols under `project.json → sources.include` enter the project.
- The first run **adopts** entities you already have (for example the assemblies from the
  section above): same file (`ref` of its `code[]`) and `name`, or same `namespace`, `name` and
  `kind`. Generic parameter lists in names (`IRunner<in TIn, out TOut>`) and leading modifiers in
  kinds (`class` vs `abstract-class`) do not get in the way; compound kinds stay whole
  (`struct` does not adopt `record-struct`). They keep their ids, names and kinds, and their
  `code[]` entry gets the run's `lang` and the `symbol` — that is the key for every later run; do
  not remove it. An entity that fits several symbols (or the reverse) is reported as `неоднозначно`:
  set the `symbol` of its `code[]` entry by hand.
- A run reconciles **one** realization per entity: the `code[]` entry whose `lang` is the facts'
  language ([ADR_20260930-4](adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md)). An
  entity realized only in another language is not touched by it, and a second language is not bound
  to an existing entity by sync — put the entry into its `code[]` yourself.
- `authored` entities, texts and views are never written. A class gone from the code gets
  `status: missing`, never deleted.
- `переименование?` in the report: an entity vanished and a new symbol of the same kind
  appeared in the same file. Nothing is decided. If it is a rename, set the `symbol` of the old
  entity's `code[]` entry to the new symbol id (and `name`, if you want) and run again; if not, run with
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

## Переход на членские связи

При первом извлечении реестр содержит только органические связи (`extends`, `implements`,
`contains`, `references` для модулей). Членские связи (`holds`, `uses`, `injects`)
печатаются по явному выбору. Переход со старой модели выглядит так:

1. **Выбрать виды членских связей** в записи экстрактора файла `.semaps`:
   ```yaml
   extractors:
     - id: backend
       language: csharp
       project: core
       edges: [holds, injects]      # какие виды членских связей печатать
   ```
   На странице «Экстракторы» в редакторе это же выбирается чекбоксами.

2. **Запустить извлечение и синхронизацию:**
   ```
   semaps sync --dry-run   # посмотреть отчёт
   semaps sync             # применить факты
   ```
   Сверка **не мигрирует**: старые `references` между типами при первой сверке
   становятся `missing`, рядом появляются новые `holds` и `uses`. Старые `references`
   между модулями — `missing`, рядом `depends`.

3. **Перенести видимость типов на видах.** На каждом виде в `relations.except`
   переместить записи старых типов связей на новые. Например, если вид скрывал все
   `references` (кроме списка в `except`), проверить, нужны ли новые типы — они могут быть
   скрыты по умолчанию (см. пункт 5).

4. **Удалить или оставить старые связи.** Членские связи возникли параллельно старым —
   они не подменили их, а добавились рядом. Какие старые связи удалить, решает человек:
   `references` между типами обычно становятся ненужны, а `references` между модулями
   иногда оставляют как авторский вид. Удаление: `relations.json` вручную или
   `semaps sync` их не трогает (так и остаются `missing`).

5. **Добавить имена новых типов связей в текстовые каталоги.** Каждый новый тип (`holds.one`,
   `holds.many`, `holds.internal` и т. д.) требует названия в `text.<lang>.json` под ключом
   `rt_<тип>`, иначе `semaps check` сообщит `не хватает`. Рекомендуемые:
   - `holds.one`, `holds.optional`, `holds.many`, `holds.keyed` — содержит (публичный член)
   - `holds.one.internal`, `holds.many.internal` и т. д. — внутреннее устройство
   - `uses` — использует (параметр, возвращаемое значение)
   - `injects` — внедряет (конструктор)
   - `depends` — зависит (модуль от модуля)

   Пример (для русскоязычного проекта):
   ```json
   "rt_holds.one": { "v": "содержит (одно)", … },
   "rt_holds.many": { "v": "содержит (много)", … },
   "rt_depends": { "v": "зависит от", … }
   ```

**Видимость по умолчанию.** На виде без явной настройки видны:
- органические: `extends`, `implements`, `contains`, `depends`
- членские через публичные члены: `holds.one`, `holds.optional`, `holds.many`,
  `holds.many.ro`, `holds.keyed`, `holds.keyed.ro`

Скрыты по умолчанию:
- членские через приватные члены: `holds.*.internal`
- прочие: `uses`, `injects`

Нужна другая видимость — задать `relations.except` на виде или `visibility` типа в
`relation-types.json` (при создании типа).

**Переименование члена.** При синхронизации может появиться кандидат-переименование:
«пропала связь `r_…_memberOld`, появилась `r_…_memberNew` с теми же `from`, `to` и
`path`». Это не решение, а вопрос. Если это действительно переименование члена в коде:
1. найти в `relations.json` старую связь
2. в её поле `via.member` написать новое имя
3. запустить `semaps sync` снова

Синхронизация найдёт совпадение и обновит связь. Если переименования не было —
`semaps sync --no-renames` пропустит кандидата.

**Что не мигрирует.** Инструмент только сообщает, что изменилось: старые связи
становятся `missing`, новые появляются рядом. Переносить видимость типов на виды,
удалять ненужные, добавлять названия — делает агент или человек.

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

## Moving a workspace to the current contract: `semaps migrate`

The loader reads only the current shape (contract 5, with the code shape of
[ADR_20260930-4](adr/ADR_20260930-4_contract_code-shape-now-multi-language-later.md) and the name shape
of [ADR_20260930-5](adr/ADR_20260930-5_contract_authored-entity-name-is-text.md)). A workspace written for
contract 3 — a `project.json` below 5, views with `zones`/`nodes`, a `containers.json`, `c_`/`z_` text
keys, `kinds` in `styles.json`, `zone` in `canvas.json` — or a contract-5 one in the earlier form — a
`codeRef`/`symbol` on an entity, a `via` on a relation, the `name` of an authored entity in
`entities.json` — is refused with an error that names the file and the field, and `semaps check` says
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
  `canvas.json`: `zone` → `container`; a `styleId` on a relation type is dropped and named;
- `contractVersion` goes to 5 in every file that has one.

The report ends with «Решает человек»: the `match` rules and unplaced containers of `containers.json`, styles
without a type (or the placements that lost a dropped style), zones nobody names. Then run `semaps
check`: entity kinds and relation types outside the dictionary are listed («не из словаря»); add the
project's own to `<workspace>/kinds.json`; two languages with the same name both `authored` are listed as
`расхождение` — mark one as translated from the other (§ «Two languages»). Do not edit `contractVersion` by
hand instead of migrating.

## Traps

- **Old workspace with `catalog.json`.** Move each entry: `title` → `name` under the view's id in
  `text.<lang>.json`, `icon`/`theme` → into the `.view.json`; drop `views` from `project.json`;
  then delete `catalog.json`. `semaps check` reports it until it is gone.
- **No `*.semaps`, no server.** The host no longer looks for a workspace by itself; the project file
  is required (or `--workspace`).

- **Geometry only on request.** Unasked, an agent never writes `x/y/width/height`, containers or
  placements (CONTRACT §8.2, §9.6): create the view empty and let the human place the blocks. When the
  human explicitly asks for help with a view ("spread these subclasses into frames by meaning"),
  do what was asked and nothing more. An agent works on the canvas **only through MCP**, never by
  editing view files: the MCP tool `layout_guide` explains how a view works — canvas numbers, each
  tool, the order of work — and the two things that are not optional: the shared working model must
  be reviewed and saved by the human, and geometry outside the request stays put. A new view or
  project (`create_view`, `create_project`) only when a human explicitly asks for one.
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
