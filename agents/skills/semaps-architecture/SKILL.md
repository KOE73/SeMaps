---
name: semaps-architecture
description: Use SeMaps diagrams to understand or implement human-authored architecture, and use source code to create or update architecture, data-flow, and class views through SeMaps MCP. Apply when a task uses a SeMaps workspace or explicitly asks for these SeMaps workflows.
license: MIT
metadata:
  author: KOE73
  version: "0.1.1"
---

# SeMaps architecture

Work in both directions: human-authored diagrams inform architecture and code;
source code informs the map. A diagram may itself be the architectural design,
including components, data flows, responsibilities and classes, even before code
exists. Keep authored intent, extracted facts and your own proposals distinct.

## Requirements

**Requires an installed SeMaps binary and a SeMaps MCP server connected to the
agent for the intended repository.** Installing this skill supplies instructions
only; it does not install SeMaps, connect MCP, or include a hosted service.
Code extraction also requires the relevant extractor and language runtime.

Discover the connected server's instructions and tool schemas; use the tools
actually exposed rather than assuming a client-specific tool prefix. Check
`list_projects` and the requested view/reference against the intended workspace.
If prerequisites are missing, read [setup.md](references/setup.md). Do useful
read-only work that remains possible, report the missing dependency, and do not
pretend to have read or updated the map. Do not silently switch workspaces.

## Choose the direction

- **Diagram to architecture or code:** the user asks to explain a drawn design,
  derive responsibilities or data flows, plan an implementation, or implement a
  diagram. Read [diagram-to-code.md](references/diagram-to-code.md).
- **Code to diagram:** the user asks to document an implementation, create a
  class/architecture/data-flow view, or update a map after code changes. Read
  [code-to-diagram.md](references/code-to-diagram.md).
- **Both:** read the relevant existing diagrams first, compare them with fresh
  code facts, implement only the requested change, then reconcile the affected
  map if that is part of the task. Use both references without assuming a request
  to explain a diagram authorizes code changes or a request to inspect code
  authorizes map edits.

## What this skill adds to MCP

The connected MCP server supplies tool schemas, descriptions and operational
rules, including freshness, provenance, visibility, layout and Save/Discard.
Use those current instructions; this skill is not a replacement API manual.
Read and write the map through MCP, without parsing workspace files or learning
their storage format.

The skill chooses the workflow and the architectural evidence to use:

- Read human-authored views and texts for intended responsibilities and flows;
  use fresh graph facts and cited source for the actual implementation. Neither
  automatically overrides the other. Report consequential discrepancies and
  resolve the direction from the task; ask when the design choice is unresolved.
- For code structure, start with the graph and open the source locations it
  returns, following explicit repository tool preferences. Missing, stale,
  unsupported or truncated facts do not prove absence.
- Different views can describe the same system at different levels. Reuse
  registry entities instead of duplicating them; do not treat a visual grouping
  as proof of structural ownership or a dependency as proof of runtime data flow.
- Match mutations to the task. Explaining a diagram does not authorize code
  changes; examining code does not authorize diagram placement. Drawing a view
  does cover its necessary placements. Preserve unrelated authored decisions,
  geometry and pending work. Map edits remain unsaved; Save/Discard still require
  an explicit human request for that action and scope.

## Hand over

Name the view/object references inspected or changed, distinguish implemented
code facts from intended design and proposals, and include view links returned
by the tools. For edits, list what remains unsaved and any unresolved mismatch.
Use the server's available checks for the working model; a disk check does not
validate unsaved changes.
For diagram creation, read back the affected view and, when an editor is open,
use `render_view` to inspect it. State any rendering or runtime verification
limit. A populated registry alone does not mean a visible diagram was created.

## License and source

This skill is free and licensed under [MIT](LICENSE). Its workflows are adapted
from SeMaps [ADOPTING.md](https://github.com/KOE73/SeMaps/blob/main/docs/ADOPTING.md).
The connected server's schemas describe the installed version's capabilities.
