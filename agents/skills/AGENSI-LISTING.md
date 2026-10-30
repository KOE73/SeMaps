# Agensi listing draft — not published

## Listing fields

- Title: SeMaps Architecture — Diagrams to Code and Code to Diagrams
- Skill name: semaps-architecture
- Version: 0.1.1
- Price: Free ($0)
- License: MIT; the source and redistribution rights remain available under MIT.
- Suggested category: Technical Documentation
- Suggested tags: architecture, diagrams, data-flow, code-understanding, mcp, semaps
- Source: https://github.com/KOE73/SeMaps
- Prerequisites: Installed SeMaps binary and connected SeMaps MCP for the target repository; language extractor/runtime for code extraction.

## Short description

Use human-authored SeMaps diagrams to guide architecture and implementation,
and turn code into architecture, data-flow or class views. Requires installed
SeMaps and connected SeMaps MCP. Free, open source, MIT licensed.

## Full description

SeMaps keeps human-authored architecture views and extracted code facts in a
shared model. This skill teaches your coding agent how to work in both directions.

**From diagrams to code:** read the architect's existing components, boundaries,
texts and relations; explain the intended architecture or implement the requested
design. A diagram can define a future system before its code exists.

**From code to diagrams:** inspect fresh extraction facts and source evidence,
reconcile code-backed records, and create or update the requested class,
architecture or data-flow view while preserving authored intent and layout.

The skill distinguishes design intent from implementation facts and proposals.
It reports discrepancies instead of silently making the design match the code.
It uses SeMaps MCP rather than editing workspace files directly and leaves map
changes unsaved until the human explicitly requests Save. Read-only inspection
does not trigger layout or code changes.

**Requirements:** install SeMaps separately and connect its MCP server to your
agent in the target repository. Installing this skill does not install the
binary, configure MCP, or include a hosted service. Code extraction additionally
requires the appropriate SeMaps extractor and language runtime.

Includes one SKILL.md, three workflow/setup references, optional Codex UI
metadata, and the MIT LICENSE. Free to use, modify and redistribute under MIT.

## Compatibility and limits

- Intended for agents that load SKILL.md and can use the connected SeMaps MCP;
  code implementation also needs repository read/write and execution tools.
- Agent-specific installation and MCP setup are separate. Do not label this
  release as verified on all supported Agensi clients.
- SeMaps documents real Windows editor and C# code-graph use. Linux/macOS builds
  and TypeScript/Go extractors exist; equivalent validation is not claimed.
- Extracted calls/dependencies alone do not prove runtime data flow. Methods and
  calls live in the live code graph, not the authored registry.
- Local integration QA used a separate source/workspace copy of SeMaps: an
  existing authored diagram guided executable Model lifecycle tests, and Go
  extraction guided a rendered six-type view with seven structural relations.
  The original source/map files remained unchanged. This is one exercised
  workflow, not a controlled comparison with MCP alone or a client support matrix.
- Marketplace submission, security review and automatic skill discovery in
  installed agent clients have not been tested.

## Before a future submission

Confirm Agensi accepts this free, public-source MIT skill without restricting
its MIT redistribution rights. Use the Agensi ZIP from the local packager;
preserve its LICENSE and `license: MIT` frontmatter. Publication and any account
or submission actions require a separate user request.
