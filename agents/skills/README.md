# SeMaps architecture skill

**Experimental and optional.** The connected MCP server supplies the architecture workflow;
this package is not a prerequisite. Comparative trials did not establish an advantage over
MCP alone. See [the decision and measurements](../../docs/adr/ADR_20261012_host_mcp-without-required-skill.md).

Free MIT skill for two directions of work: use human-authored diagrams to
understand or implement architecture, and use code to create or update SeMaps
architecture, data-flow and class views. The two modes share workspace discovery,
provenance, reconciliation and human review rules.

**Requires an installed SeMaps binary and a SeMaps MCP server connected to the
intended repository.** This package supplies instructions, not the binary, an
MCP connection or a hosted service. Code extraction needs its extractor/runtime.

## Contents and installation

The distributable folder is [semaps-architecture](semaps-architecture/). Copy
that entire folder into your agent's skills directory, retaining its LICENSE
and references. For example, Claude Code uses `~/.claude/skills/`; Codex uses
`~/.codex/skills/`. Other clients have their own paths and MCP configuration.
Reload the agent and separately verify that SeMaps MCP targets your repository.

Example requests:

- "Explain the architecture and data flow in my existing SeMaps diagram."
- "Implement the ingestion pipeline shown in this view, preserving its design."
- "Create a SeMaps class view for this subsystem from its current code."
- "Compare the design with the implementation and report the differences."

The skill leaves map changes unsaved until a human explicitly requests Save.
Read-only analysis does not authorize changing code, creating views or layout.
An explicit request to draw a view does cover its necessary placements.

## Build local distribution archives

From the SeMaps repository root:

```powershell
powershell -NoProfile -File agents/skills/Pack-SeMapsSkill.ps1
```

The script creates two archives with the same files in ignored `dist/skills/`:

- `semaps-architecture-github.zip`: a `semaps-architecture/` folder to unpack
  into a client's skills directory.
- `semaps-architecture-agensi.zip`: `SKILL.md` at the archive root for upload.
- `semaps-architecture-manifest.json`: per-file size/hash and archive SHA-256.

The packaging script includes only the explicit distributable file list. It
checks that the skill's LICENSE is byte-identical to the repository MIT LICENSE
and verifies each ZIP entry against its source after creation. No upload,
account registration, installation, commit or publication is performed.
Existing outputs are not overwritten unless `-Force` is supplied.

## GitHub and Agensi preparation

Publish the source folder in GitHub only when requested. The separate
[Agensi listing draft](AGENSI-LISTING.md) is submission copy, not agent context;
it is intentionally excluded from the skill archives. Price: free ($0).
Confirm that Agensi accepts the included MIT license for a public-source free
skill before publishing; do not replace MIT with a restrictive buyer license.

The connected MCP supplies tool instructions and schemas; the skill adds
workflow selection and the distinction between design intent and code facts,
without documenting workspace storage formats.

Version 0.1.1 was exercised through MCP on an isolated copy of SeMaps' source
and saved workspace: an existing authored diagram guided executable Model
lifecycle tests, and Go extraction guided a rendered six-type, seven-relation
view. This manual integration exercise does not prove that automatic skill
selection works or that the skill improves results over MCP alone. Client
installation and marketplace review remain untested; no 20+ agent claim is made.

## Maintenance

[ADOPTING.md](../../docs/ADOPTING.md), [API.md](../../docs/API.md) and the connected
server's current instructions are the source for SeMaps workflows. Update the
skill's relevant reference when those workflows change; avoid copying full
manuals or freezing tool schemas in the skill. Bump `metadata.version`, retain
MIT, run the skill validator and rebuild the distribution archives. Check both
directions, especially design/implementation discrepancies, stale facts,
unsupported extraction, requested layout and unsaved changes.
