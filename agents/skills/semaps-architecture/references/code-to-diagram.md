# From code to SeMaps diagrams

Use this workflow to describe an implementation, draw a requested view, or
reconcile an existing map. Read the existing authored views before changing
them: their abstractions and data flows may carry intent unavailable in code.

## Establish the facts

Use `doctor` and `graph_status` to establish extractor/runtime availability and
the latest run state. Query `get_graph` / `find_node` for relevant structures
and open the source locations returned. If the facts are stale or missing edge
kinds required by the question, run `extract` with `wait: true` and inspect its
results. A failed run or an unsupported edge kind is a verification limit, not
an empty implementation.

Choose the perspective the user requested:

- **Class view:** types and structural relationships the extractor actually
  reports, such as inheritance, implementation and member relations.
- **Architecture view:** meaningful components and responsibilities, with code
  references as evidence. Use authored entities for abstractions the extractor
  does not report; find existing entities before creating new ones.
- **Data-flow view:** inspect producers, consumers, payloads and transitions in
  source. Calls or dependencies alone do not establish a data-flow contract or
  runtime ordering. Describe supported flows and identify inferred or unresolved
  ones separately, using the workspace dictionary for authored relations.

## Reconcile into the working model

For a request that includes updating the map, use `sync_preview` before `sync`
and inspect the affected records. `extract` refreshes only the code graph;
`sync` reconciles code-backed registry records. `watch: true` refreshes the graph
on source changes while the host is running; it does not reconcile the registry
or lay out a view. Sync is a deliberate operation.

The first sync may adopt existing entities by unambiguous file/name or
namespace/name/kind matches, preserving their IDs. Ambiguities require a human
decision. Rename candidates are proposals: use `confirm_rename` only after the
rename is established, consulting the human when it affects authored views;
otherwise use the supported `noRenames` option. A removed code class becomes
`status: missing`, not a silently deleted design component.

Sync preserves authored entities, texts and views. It reconciles one language's
realization per entity; it does not automatically bind another language to the
same entity. Types can carry member lists, while live graph details need not be
standalone registry entities. Do not promise that sync produces a method/call
diagram on the authored canvas; check the installed server's capabilities.

Use `add_entity`, `add_relation` and text tools only for the requested authored
abstractions or explanations that code extraction cannot supply. Preserve IDs,
text provenance and human decisions. Do not update the design to match an
implementation discrepancy unless that direction is part of the task.

## Make a visible view when requested

Reuse the requested view or create the project/view when the task calls for it.
Keep one purpose/axis per view, and reuse registry entities across perspectives.
Read `layout_guide` before placing anything. Use `get_view` to inspect existing
placements, then make the requested placements/grouping through MCP; retain
unrelated positions, manual styling and view visibility decisions.

Relations normally appear when both endpoints are placed and the view's
visibility rules allow them. `get_kinds` supplies defaults; non-public member
relations, `uses` and `injects` are hidden by default. Inspect the actual view
and use `set_relation_visible` for requested relationships rather than inventing
duplicate relations or rewriting edge overlays by hand.

Read back the view and use `render_view` if an editor is available. If rendering
is unavailable, report that limitation. Without a request to draw/place, leave
the geometry alone and explain that the updated entities are available in the
entity database for the human to place. Registry reconciliation alone is not
a finished visible diagram.

Return the affected view/entity references, evidence for authored explanations,
unresolved discrepancies, and unsaved changes. Save only on an explicit request;
for saved files run `semaps check` and assess its exit code.
