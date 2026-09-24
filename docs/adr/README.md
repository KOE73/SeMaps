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
