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
