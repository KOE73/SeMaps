# ADR

Decisions — never edited. Rules: [agents/documentation.md](../../agents/documentation.md).

## Diagrams

Current form of what they decided: [`CONTRACT.md`](../CONTRACT.md), [`API.md`](../API.md).

| ADR | About |
|---|---|
| [ADR_20260828_diagrams_model-contract-v2](ADR_20260828_diagrams_model-contract-v2.md) | four registries, a view is a selection, sync instead of generation |
| [ADR_20260829_diagrams_ui-theme-and-canvas-isolation](ADR_20260829_diagrams_ui-theme-and-canvas-isolation.md) | UI theme variables never reach the canvas |
| [ADR_20260831_diagrams_text-provenance-and-view-axes](ADR_20260831_diagrams_text-provenance-and-view-axes.md) | contract v3: per-field text provenance, view axes |
| [ADR_20260901_diagrams_shape-aware-geometry-and-insets](ADR_20260901_diagrams_shape-aware-geometry-and-insets.md) | the shape owns its corner insets |
| [ADR_20260903_diagrams_content-templates-and-edge-routing](ADR_20260903_diagrams_content-templates-and-edge-routing.md) | block content templates, end labels, routing choice |

## SeMaps

| ADR | About |
|---|---|
| [ADR_20260923_host_workspace-tool-source-roots](ADR_20260923_host_workspace-tool-source-roots.md) | host takes three separate roots; defaults with workspace override |
| [ADR_20260923-2_host_one-binary-and-project-file](ADR_20260923-2_host_one-binary-and-project-file.md) | one embedded binary; project file `*.semaps` with the environment |
| [ADR_20260923-3_build_bundle-from-ci-not-git](ADR_20260923-3_build_bundle-from-ci-not-git.md) | editor bundle built by CI, not committed; binaries in Releases |
| [ADR_20260923-4_core_check-lives-in-the-binary](ADR_20260923-4_core_check-lives-in-the-binary.md) | `semaps check` in `core/`, the Node script is gone |
| [ADR_20260923-7_contract_projects-and-views-found-not-listed](ADR_20260923-7_contract_projects-and-views-found-not-listed.md) | no `catalog.json`: projects and views are found on disk; workspace only from `*.semaps` |
| [ADR_20260923-8_contract_project-and-view-ids-can-change](ADR_20260923-8_contract_project-and-view-ids-can-change.md) | project/view ids are renamable, entity ids are not; `/api/move`, `save?create=1` |
| [ADR_20260923-9_core_sync-symbol-mapping-and-containment](ADR_20260923-9_core_sync-symbol-mapping-and-containment.md) | `semaps sync`: entity `symbol` as the match key, first-run adoption, held renames, `contains` written as a relation |
| [ADR_20260923-10_extractors_csharp-assembly-symbol](ADR_20260923-10_extractors_csharp-assembly-symbol.md) | C#: each project is a `module`/`assembly` symbol `[Name]`, `contains` its top-level types, `references` its project references |
| [ADR_20260924_contract_agent-lays-out-views-on-request](ADR_20260924_contract_agent-lays-out-views-on-request.md) | an agent may write view geometry on the human's explicit request, within it; tools never; reference — `docs/LAYOUT.md` |
| [ADR_20260924-2_core_sync-writes-references](ADR_20260924-2_core_sync-writes-references.md) | superseded by ADR_20260924-4. sync writes `references` as relations like the other edge kinds; the type is created `visibility: hidden`, a view switches them on; supersedes ADR_20260923-9 §5 on `references` and the rejected `byType` of ADR_20260828 |
| [ADR_20260924-3_host_tools-beside-the-binary-and-setup](ADR_20260924-3_host_tools-beside-the-binary-and-setup.md) | extractors ship in `extractors/<lang>/` beside `semaps.exe`; `.semaps` (now YAML) lists them; `semaps doctor/extract/sync` and `/setup` drive one engine; `command` only from the file; a way back to a server kept open |
| [ADR_20260924-4_contract_member-relations-from-code](ADR_20260924-4_contract_member-relations-from-code.md) | one relation per member occurrence with `via` (cardinality, mutability, slot path — features, not wrapper names); sync composes `holds.*`/`uses`/`injects`, public visible, internal/uses hidden; `depends` between modules; `--edges` filter; migration described, not coded; supersedes ADR_20260924-2 |
| [ADR_20260924-5_host_mcp-server](ADR_20260924-5_host_mcp-server.md) | `semaps mcp`: an MCP server in the binary over `core/`, so an agent reads and writes the registry through checked tools instead of whole files; files stay the contract |
| [ADR_20260925_host_model-owned-by-host](ADR_20260925_host_model-owned-by-host.md) | the host owns the working model: journal, Save, events, MCP over HTTP |
| [ADR_20260925-2_host_project-manifest-and-structure](ADR_20260925-2_host_project-manifest-and-structure.md) | the project manifest is part of the working model; create and rename stay separate structural operations of the host |
| [ADR_20260928_host_live-code-graph](ADR_20260928_host_live-code-graph.md) | live code graph: a derived layer beside the model, outside git; one graph for the editor and MCP, scoped by query; calls are not stored; own extractors, not LSP |
| [ADR_20260928-2_editor_graph-is-a-separate-form](ADR_20260928-2_editor_graph-is-a-separate-form.md) | the graph is a separate form with computed layout; "no auto-layout" covers diagrams only; one WebGL renderer |
| [ADR_20260928-3_host_calls-live-in-the-graph](ADR_20260928-3_host_calls-live-in-the-graph.md) | methods and calls live in the live graph only, never in git or the registry; methods are graph nodes; supersedes §6 of ADR_20260928 |
| [ADR_20260928-4_extractors_methods-and-calls](ADR_20260928-4_extractors_methods-and-calls.md) | symbol kind `method`, method ids, edges `calls`/`constructs`/`overrides`; sync skips all of it; switched on by `--edges calls` |
| [ADR_20260928-5_extractors_only-unambiguous-facts](ADR_20260928-5_extractors_only-unambiguous-facts.md) | only facts that follow from the language; no heuristics, no knowledge of libraries except the platform's base library; a blind spot is marked, its target is not guessed |
| [ADR_20260929_editor_icons-are-registry-keys](ADR_20260929_editor_icons-are-registry-keys.md) | every UI icon comes from one registry by key; `icon` (project, view) and `icon.glyph` (style) hold registry keys, no emoji; unknown values show the fallback icon |
| [ADR_20260929-2_host_no-arrange-like](ADR_20260929-2_host_no-arrange-like.md) | no `arrange_like`: pairing nodes by inheritance role and name words fits one family shape only; the agent decides, the tools give it `get_view` and precise steps; removed from core, MCP and the live docs |
