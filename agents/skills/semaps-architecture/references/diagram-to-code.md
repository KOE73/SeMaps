# From human-authored diagrams to architecture and code

Use this workflow when the map is the starting point, including a design with
no implementation yet. The output may be an architectural explanation, a plan,
or code, depending on the user's request.

## Read the design before interpreting the implementation

1. Use `list_projects` / `list_views` to locate the requested perspective. A
   copied object reference such as `v_ops#e_decoder` scopes a view subtree; it is
   not a source-code symbol. Use the real IDs returned by this workspace.
2. Read `get_view` and the view's title, description and axis via `get_text`.
   Inspect relevant entities with `get_entity`, their texts with `get_text`, and
   the registry relations with `get_relations`. Get relation meanings from
   `get_kinds` rather than guessing from arrow direction or colour.
3. For a large view, the default `get_view` response may be a container tree
   with counts. Use `detail: full` on the relevant view/subtree when individual
   blocks or lines matter. `hidden: true` exposes hidden relations whose ends
   are placed; registry queries cover relationships beyond this perspective.
   A missing line on the canvas does not mean no registry relationship exists.
4. Describe the actual view: responsibilities, boundaries, components, data
   flow, and assumptions that its texts and relations support. Inspect other
   views when they clarify the requested scope. Avoid treating visual placement
   or a class dependency as proof of runtime order, ownership or data transfer.

## Relate intent to code without erasing the design

If code exists, follow entity code references and query a fresh graph for the
parts involved. Separate:

- **Authored design:** what the human's map says should exist or happen.
- **Implementation facts:** what current extraction and source inspection show.
- **Proposal or uncertainty:** your interpretation where the map is incomplete.

An authored component without a code realization can be planned architecture;
it is not automatically obsolete. Conversely, an extractor relationship may
show an implementation detail the design intentionally abstracts away.

Report consequential differences explicitly, for example: the design shows
ingestion through a queue, while the current source calls storage directly.
If asked to implement the diagram, use the diagram as the requested design and
identify any remaining architectural decisions before depending on them. If
asked to explain or review it, return findings without changing code or the map.

## Implement when requested

Map the requested responsibilities, data contracts and interactions to code
changes and meaningful acceptance checks. Respect repository rules and existing
code; SeMaps provides design context rather than generating a program by itself.
Validate the changed code with the repository's appropriate checks.

If the task includes updating the map after implementation, use the
[code-to-diagram workflow](code-to-diagram.md) for the affected scope. Preserve
the authored view and its layout; do not replace it with the extracted class
graph. Do not add a new view, move blocks or save solely because code changed.

Return the design references used, resulting explanation/plan or implemented
changes, validation results, and any remaining design-to-code discrepancy.
