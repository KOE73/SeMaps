# SeMaps Editor — Implemented Requirements

**Status:** baseline specification, reverse-engineered from the working implementation.
**Archive.** Baseline of the pre-TypeScript single-file editor (`docs/diagrams/index.html` @ `42197c9` in the original repository, not present here). Kept as history; the current contract is [`docs/CONTRACT.md`](../../docs/CONTRACT.md).
**Russian version:** [`REQUIREMENTS.ru.md`](REQUIREMENTS.ru.md) — same identifiers, kept in sync.

## 0. Purpose of this document

This is **not** a design proposal. It records what the current implementation
actually does, so that the TypeScript/ESM/Vite migration can be verified against
a written baseline rather than against memory.

Every statement below is either:

- **`R-*` — a requirement.** Implemented, intentional, must survive the migration.
- **`D-*` — a defect.** Implemented, but wrong. Must **not** be reproduced;
  each carries the intended behaviour.
- **`Q-*` — an open question.** Behaviour exists but its intent is unclear; needs
  a decision before it is either kept or dropped.

Requirement IDs are stable and are quoted by migration commits and tests.

## 1. Terminology

| Term | Meaning |
| --- | --- |
| **Model** | One diagram, serialized as a single JSON document. |
| **Node** | A leaf element on the canvas — a class, component, service, note. |
| **Zone** | A rectangular container carrying architectural meaning (subsystem, boundary, security zone). |
| **Edge** | A directed connection between two nodes. |
| **View** | A named highlight preset that dims everything outside its subject. |
| **Catalog** | The list of diagram models offered in the left sidebar. |
| **Canvas** | The SVG surface: rendering, viewport, direct manipulation. |
| **Editor** | The full application: canvas plus toolbar, inspector, catalog, history, persistence. |

## 2. Data contract (`R-MODEL`)

The JSON contract is **frozen** for the migration. It is described here as
observed, not as it ought to be.

### R-MODEL-01 — Model root

A model is a JSON object with the optional members `metadata`, `views`, `zones`,
`nodes`, `edges`. Every collection is optional; an absent collection is treated
as empty. No member is validated on load.

### R-MODEL-02 — `metadata`

`metadata.title` (string) is the display title. `metadata.layout` (string)
declares the layout variant (`semantic-atlas`, `message-flow`,
`subsystem-overview`). Only `title` is consumed by the editor; `layout` is
consumed by external tooling.

### R-MODEL-03 — Zone

```
id           string    required, unique
name         string    header caption
type         string    semantic type (boundary, subsystem, security-zone, storage-zone, component)
semanticId   string    stable semantic address, e.g. "zone.core.runtime"
tags         string[]  free tags
x, y         number    top-left corner, model coordinates
width        number
height       number
style        object    fill, stroke, strokeWidth, strokeDasharray, headerBg
metadata     object    description, responsibilities[]
```

### R-MODEL-04 — Node

```
id           string    required, unique
label        string    display caption
type         string    visual/semantic type, see R-REND-07
zone         string?   explicit parent zone id, or null
x, y         number    top-left corner, model coordinates
width        number
height       number
tags         string[]
metadata     object    type (subtitle text), description, codeRef, responsibilities[]
```

`metadata.type` is a free-text subtitle and is **not** the same field as `type`.

### R-MODEL-05 — Edge

```
id     string    required, unique
from   string    source node id
to     string    target node id
label  string    optional caption drawn at the midpoint
type   string    routing/visual family, see R-REND-11
```

Edges connect **nodes only**. An edge referring to a missing node is silently
skipped at render time.

### R-MODEL-06 — View

```
id              string
name            string    button caption
icon            string    emoji, defaults to 🔹
description      string    button tooltip
highlightZones  string[]  optional
highlightNodes  string[]  optional
```

### R-MODEL-07 — Coordinates are absolute

All `x`/`y` are absolute model coordinates. Nesting a node in a zone does **not**
make its coordinates relative to that zone. Only the Draw.io export converts to
relative coordinates (R-IO-07).

### R-MODEL-08 — No auto-layout, ever

The editor never computes or adjusts a position that a human or a generator
authored. Coordinates change only as the direct result of a drag or resize
gesture. This is the founding constraint of the whole tool.

## 3. Catalog (`R-CAT`)

### R-CAT-01 — Source

The catalog is read from the global `window.SEMAPS_CATALOG`, populated by a
separate `catalog.js` loaded before the application script.

### R-CAT-02 — Entry shape

`{ id, file, title, subtitle, icon, theme }`. `theme` is one of `blue`,
`emerald`, `indigo`, `amber` and only tints the sidebar icon on hover.

### R-CAT-03 — Autoload

On startup the first catalog entry is selected and loaded automatically.

### R-CAT-04 — Model fetch

Selecting an entry issues `fetch(entry.file)` relative to the page and loads the
parsed JSON as the active model.

### R-CAT-05 — Sidebar rendering

Each entry renders as a button with icon, title and subtitle. The active entry
is visually distinguished. Sidebar buttons carry the DOM id `schema-nav-<id>`.

## 4. Rendering (`R-REND`)

### R-REND-01 — Layer order

The canvas contains exactly four SVG groups inside one transformed viewport
group, in this stacking order: **zones → edges → nodes → resizers**. Edges are
drawn beneath nodes so that connections disappear under the boxes they join.

### R-REND-02 — Full rebuild

Rendering clears all four layers and rebuilds every element from the model. No
partial or incremental update path exists.

### R-REND-03 — Zone body

A rounded rectangle (`rx=12`) at the zone's geometry, filled and stroked from
`zone.style`, defaulting to fill `#f8fafc`, stroke `#cbd5e1`, stroke width `2`.

### R-REND-04 — Zone header

A rounded rectangle of height **34** across the full zone width, filled from
`style.headerBg` (default `#e2e8f0`). The header is the zone's drag handle
(R-EDIT-02).

### R-REND-05 — Zone caption

The zone name is drawn at `x+36, y+22`, 13px bold, `#334155`. When collapsed the
caption is suffixed with the contained node count. The `semanticId` is drawn
right-aligned at `x+width-14, y+21`, 10px monospace, `#64748b`.

### R-REND-06 — Collapsed zone

A collapsed zone renders at height **34** (header only), its dash pattern is
suppressed, all nodes geometrically inside it are hidden, and its resize handle
is suppressed.

### R-REND-07 — Node styling by type

Node fill, stroke, stroke width and icon are chosen from a fixed table keyed on
`node.type`:

| type | fill | stroke | width | icon |
| --- | --- | --- | --- | --- |
| `concept` | `#fefce8` | `#fef08a` | 1.8 | 💡 |
| `note` | `#fef9c3` | `#fde047` | 1.5 | 📝 |
| `component` | `#ffffff` | `#cbd5e1` | 1.5 | 📦 |
| `service` | `#ffffff` | `#93c5fd` | 1.5 | ⚙️ |
| `security-component` | `#fff1f2` | `#fca5a5` | 2.0 | 🛡️ |
| `tool` | `#ffffff` | `#bfdbfe` | 1.5 | 🔧 |
| `database` | `#ffffff` | `#d8b4fe` | 1.5 | 💾 |
| `external-system` | `#fffbeb` | `#fde68a` | 1.5 | 🌐 |
| *(any other)* | `#ffffff` | `#cbd5e1` | 1.5 | 📄 |

### R-REND-08 — Node body

A rounded rectangle (`rx=8`) with a drop shadow filter, captioned
`<icon> <label>` at 12.5px semibold, plus a monospace subtitle showing
`metadata.type` (falling back to `type`). Caption baselines shift down when the
node is taller than 60 units.

### R-REND-09 — Node visibility

A node is not rendered when the zone geometrically containing it is collapsed.

### R-REND-10 — Edge routing

An edge is a cubic Bézier between the two node centres. The dominant axis is
chosen by comparing `|Δx|` and `|Δy|`; the curve leaves and enters through the
corresponding box edges, with control points at the midpoint of the span. There
is no obstacle avoidance and no orthogonal routing.

### R-REND-11 — Edge styling by type

Edge types fall into two visually distinct families:

**Structure** — how the code is assembled; grey, triangular heads, extracted
automatically by the generator:

| type | stroke | dash | head |
| --- | --- | --- | --- |
| `extends` | `#475569` | solid | hollow triangle |
| `implements` | `#64748b` | `8,4` | solid triangle |
| `realizes` | `#64748b` | `8,4` | solid triangle |
| `composes` | `#0f766e` | solid | solid diamond |

**Flow** — what happens at runtime; coloured, arrow heads, authored by hand:

| type | stroke | dash | head |
| --- | --- | --- | --- |
| `call` | `#94a3b8` | `4,4` | arrow |
| `data-flow` | `#3b82f6` | solid | arrow |
| `event` | `#ea580c` | `2,3` | open chevron |
| `security` | `#f43f5e` | `3,3` | arrow |
| `storage` | `#a855f7` | solid | arrow |
| *(any other)* | `#cbd5e1` | solid | arrow |

### R-REND-12 — Edges and collapsed zones

When an endpoint's zone is collapsed, that end of the edge is re-anchored to the
centre of the collapsed header instead of the hidden node. When **both**
endpoints live in the same collapsed zone, the edge is not drawn at all.

### R-REND-13 — Edge label

`edge.label`, when present, is drawn at the midpoint of the curve, 11px,
`#475569`, non-interactive.

### R-REND-14 — Selection affordance

The selected node or zone is stroked `#2563eb` at 2.5px. The selected edge is
stroked `#3b82f6` at 3px with a glow, and its label turns blue and bold.

### R-REND-15 — Hover affordance

Hovering a node raises a drop shadow. Hovering an edge turns it `#2563eb` and
thickens it to 2.5px.

## 5. Views and highlighting (`R-VIEW`)

### R-VIEW-01 — View bar

The header renders one button per model view, in model order. When the model
declares no views, a single implicit view `all` / "Все элементы" is shown.

### R-VIEW-02 — Initial view

On load the first declared view becomes active; with no views, `all` is active.

### R-VIEW-03 — Zone highlighting

When the active view declares a non-empty `highlightZones`, zones outside it
render at opacity **0.25**; otherwise all zones render fully opaque.

### R-VIEW-04 — Node highlighting

`highlightNodes` takes precedence: nodes outside it render at opacity **0.2**.
If the view declares no `highlightNodes` but does declare `highlightZones`, a
node is highlighted when its containing zone is highlighted.

### R-VIEW-05 — Edge highlighting

An edge is highlighted only when **both** its endpoints are in `highlightNodes`;
otherwise it renders at opacity **0.15**.

### R-VIEW-06 — Highlighting is presentation only

Switching views never mutates the model.

## 6. Containment (`R-CONT`)

### R-CONT-01 — Node in zone

A node belongs to a zone when `node.zone === zone.id`, **or**, failing that,
when the node's centre point lies inside the zone rectangle. Geometry is a
fallback for models that do not declare parentage.

### R-CONT-02 — Zone in zone

A zone is nested inside another when its rectangle is **fully** contained by the
outer rectangle. A zone is never nested in itself, and touching edges do not
count as containment.

### R-CONT-03 — Hit testing

The zone under a point is found by scanning zones in reverse model order, so the
last-declared zone wins. Collapsed zones are hit-tested at their collapsed
height.

## 7. Viewport (`R-NAV`)

### R-NAV-01 — Transform

The viewport applies `translate(panX, panY) scale(zoom)` to one group wrapping
all four layers. Initial state is `zoom=0.9`, `pan=(40, 20)`.

### R-NAV-02 — Pan

Dragging empty canvas pans the viewport 1:1 with the pointer. A drag that starts
on a node, zone header, resizer or edge does not pan.

### R-NAV-03 — Wheel zoom

The wheel zooms by ×1.1 / ×0.9 per notch. Page scrolling is suppressed.

### R-NAV-04 — Button zoom

Toolbar buttons zoom by ×1.2 and ×0.8.

### R-NAV-05 — Zoom clamp

Zoom is clamped to the range **0.2 … 3.0**.

### R-NAV-06 — Reset

Reset restores `zoom=1`, `pan=(40, 20)`.

### R-NAV-07 — Fit

Fit computes the bounding box of all zones and nodes, adds 60 units of padding
on each side, and centres it. The resulting zoom is capped at **1.2** so small
diagrams are not blown up. Fit runs automatically after every model load.

### R-NAV-08 — Zoom readout

The current zoom is displayed as a rounded percentage.

## 8. Direct manipulation (`R-EDIT`)

### R-EDIT-01 — Node drag

Pressing a node selects it and begins a drag. Movement is divided by the current
zoom so the node tracks the pointer at any scale.

### R-EDIT-02 — Zone drag

A zone is dragged by its header only; its body is inert, so the space inside a
zone remains available for panning and for grabbing the nodes within it.

### R-EDIT-03 — Container drag

While "Зона-Контейнер" is enabled (default **on**), dragging a zone moves both
the nodes it contains **and the zones nested inside it**. Without the second
list nested containers stay behind and the diagram visually falls apart.

### R-EDIT-04 — Grid snap

While "Привязка" is enabled (default **on**), drag and resize results snap to a
**10 unit** grid. Snapping is applied to the resulting coordinate, not to the
pointer delta, so a snapped element stays on the grid across successive drags.

### R-EDIT-05 — Drop target feedback

While a node is dragged, the zone under the node's centre is outlined in blue.

### R-EDIT-06 — Reparenting

Releasing a node over a zone assigns that zone's id to `node.zone`. Releasing it
over empty canvas leaves the previous parentage untouched.

### R-EDIT-07 — Zone resize

A selected, expanded zone shows a resize handle at its bottom-right corner.
Dragging it resizes the zone with minimums of **160 × 100**. The zone's contents
are not moved or scaled.

### R-EDIT-08 — Node resize

Every node carries an invisible resize handle at its bottom-right corner, with
minimums of **100 × 40**.

### R-EDIT-09 — Fit node width

Double-clicking a node's resize handle sets its width to
`max(120, label.length × 8 + 40)` — a rough fit to the caption. Height is
unchanged.

### R-EDIT-10 — Collapse toggle

The `−` / `+` button in a zone header toggles collapse. The gesture is bound to
mouse-up and stops propagation, so pressing the button never starts a zone drag.

### R-EDIT-11 — Collapse state is not persisted

Collapse is view state. It lives outside the model, is never serialized, and is
cleared whenever a model is loaded.

## 9. Creation and deletion (`R-CRUD`)

### R-CRUD-01 — New node

Creates a node at the centre of the current viewport, snapped to the 10 unit
grid, 190 × 60, type `concept`, auto-assigned to the zone under that point, then
selects it.

### R-CRUD-02 — New zone

Creates a zone at the centre of the current viewport, 420 × 300, type
`boundary`, `semanticId` derived from the generated id, with the default style,
then selects it.

### R-CRUD-03 — Generated ids

New ids are `node_`, `zone_` or `edge_` followed by a base-36 timestamp.

### R-CRUD-04 — Delete node

Deletes the node **and every edge referencing it**, then clears the selection.

### R-CRUD-05 — Delete zone

Deletes the zone and clears `node.zone` on every node that named it. Contained
nodes are **not** deleted — they survive, orphaned in place.

### R-CRUD-06 — Add edge

The inspector adds an outgoing edge from the selected node to a chosen target,
with a chosen type and an optional label.

### R-CRUD-07 — Delete edge

An outgoing edge is deleted from the selected node's connection list.

## 10. Inspector (`R-INSP`)

### R-INSP-01 — Selection scope

Nodes, zones and edges are selectable. Clicking a node or zone body selects it;
clicking an edge selects the edge.

### R-INSP-02 — Header badge

Shows the selection kind and type — `NODE: service`, `CONTAINER: boundary`,
`EDGE: call` — colour-coded per kind.

### R-INSP-03 — Live geometry readout

Shows `X, Y (W×H)` for nodes and zones, updated continuously **during** a drag
or resize. For edges it shows `FROM … TO …` instead.

### R-INSP-04 — Editable fields

Caption (`label` for nodes, `name` for zones), semantic type, description
(`metadata.description`) and code reference (`metadata.codeRef`). Caption and
type edits re-render the canvas immediately.

### R-INSP-05 — Type vocabulary

The type selector offers a fixed list: `concept`, `component`, `service`,
`security-component`, `tool`, `database`, `external-system`, `note`,
`boundary`, `subsystem`.

### R-INSP-06 — Parent zone readout

Shows the name of the containing zone, "Вне зон" when there is none, or
"Является зоной" when a zone is selected. Read-only.

### R-INSP-07 — Zone panel

For a zone, shows the contained node count and a collapse/expand button.

### R-INSP-08 — Node connections panel

For a node, lists outgoing edges with target name and type, each with a delete
control, plus a form to add a new edge (target, type, label).

### R-INSP-09 — Delete control

A delete button for the current selection, wording adapted to node vs zone.

### R-INSP-10 — Empty state

With nothing selected, the inspector shows a hint and the delete control is
disabled.

## 11. History (`R-HIST`)

### R-HIST-01 — Snapshot model

Undo history stores whole-model JSON snapshots. There is no command or diff
model.

### R-HIST-02 — Depth

At most **50** snapshots are retained; the oldest is discarded beyond that.

### R-HIST-03 — Redo truncation

Performing a new action after an undo discards the redo tail.

### R-HIST-04 — Initial snapshot

Loading a model seeds the history with exactly one snapshot and resets the
index.

### R-HIST-05 — Commit points

A snapshot is committed on: node create, zone create, item delete, edge add,
edge delete, fit-width, and on the release of a drag or resize.

### R-HIST-06 — No-op suppression

A drag or resize that ends where it started commits nothing. This is detected by
comparing the serialized model against the snapshot taken when the gesture
began.

### R-HIST-07 — Selection survives

After undo or redo the selection is re-resolved by id against the restored
model, so the inspector keeps pointing at the live object rather than a detached
copy.

### R-HIST-08 — Keyboard

`Ctrl+Z` undo, `Ctrl+Shift+Z` and `Ctrl+Y` redo, `Delete` / `Backspace` delete
selection. Shortcuts are suppressed while focus is in a text field.

### R-HIST-09 — Button state

Undo and redo controls are disabled at the ends of the stack.

## 12. Dirty state (`R-STATE`)

### R-STATE-01 — Marking

Any model mutation raises the dirty flag and reveals the "изменено" indicator.

### R-STATE-02 — Clearing

Loading a model, or a successful save, clears the flag.

### R-STATE-03 — No autosave

Nothing is written automatically. There is no `localStorage`, no session
recovery, no periodic flush. Unsaved edits are lost when the page closes.

## 13. Load, save, export (`R-IO`)

### R-IO-01 — Catalog load

See R-CAT-04.

### R-IO-02 — File picker

A local `.json` file can be opened through a file input.

### R-IO-03 — Drag and drop

Dropping a `.json` file anywhere on the window loads it; a full-window overlay
appears during the drag. A non-JSON file is rejected with a message.

### R-IO-04 — Custom entries

A manually loaded file is appended to a separate "Пользовательские" section of
the sidebar and can be re-selected from there for the rest of the session.

### R-IO-05 — Save

Save issues `POST /api/save?file=<catalog file name>` with the serialized model
as the body, and confirms with a transient success state on the button.

### R-IO-06 — Save is catalog-only

A manually loaded file has no server-side origin and cannot be saved; the
attempt is refused with an explanation.

### R-IO-07 — Draw.io export

Exports a `.drawio` (mxGraph) document in which:

- zones become `swimlane` cells with `container=1`, `collapsible=1` and a
  start size of 34, preserving fill, stroke, stroke width and dash;
- nodes become child cells of their declared zone, with coordinates converted
  from absolute to **zone-relative**;
- edges become orthogonal `mxCell` edges;
- semantics (`type`, `semanticId`, `tags`, `codeRef`) are carried in an
  `<Object as="data">` child so the meaning survives the round trip;
- the file downloads with a name derived from the model title.

### R-IO-08 — JSON inspection

A modal shows the serialized model, allows copying it to the clipboard, and
allows pasting an edited document back — which replaces the active model and
re-renders.

## 14. Application shell (`R-SHELL`)

### R-SHELL-01 — Layout

Fixed header; three columns below it — catalog sidebar, canvas, inspector; the
canvas is the only element that grows.

### R-SHELL-02 — Collapsible catalog

The left sidebar collapses to an icon rail, hiding all text labels.

### R-SHELL-03 — Grid dots toggle

The canvas dot grid can be hidden. This is purely decorative and independent of
R-EDIT-04 snapping.

### R-SHELL-04 — Title and dirty indicator

The header shows the active model's title and, when applicable, the dirty badge.

### R-SHELL-05 — Zoom overlay

A floating control at the bottom-left of the canvas offers zoom in/out, 100%,
fit, and the zoom readout.

### R-SHELL-06 — Language

The user interface is in Russian. Semantic vocabulary — type names, edge types,
the JSON contract — is in English.

## 15. Non-functional characteristics

### R-NFR-01 — No runtime dependencies

The canvas depends on nothing but the browser's SVG and DOM APIs. No diagramming
library, no graph layout engine.

### R-NFR-02 — Scale

The largest model in use, the `full_core` project, holds ~246 nodes across 13
boundaries and 17 nested sub-roles. This is the working scale the renderer must
remain usable at.

### R-NFR-03 — Serving

The application is served over HTTP because it fetches models by relative URL
and saves through an HTTP endpoint. It does not work from `file://`.

## 16. Defects — not to be reproduced

### D-01 — Collapsed zone height disagrees between modules

Rendering and edge anchoring use **34**; fit-to-view and hit testing use **36**.
Intended: one constant, used everywhere.

### D-02 — Inspector text edits bypass history

Editing a caption, description or code reference mutates the model and marks it
dirty, but commits no snapshot — so undo jumps past the edit and silently
discards it. Intended: text edits are committed, debounced so that typing
produces one snapshot rather than one per keystroke.

### D-03 — Inspector HTML injection

The inspector builds its markup by string concatenation with unescaped model
values. A caption containing a quote or an angle bracket corrupts the panel, and
a crafted model can execute script. Intended: values are set as text/properties,
never interpolated into markup.

### D-04 — Ambiguous parent for nested zones

`R-CONT-01` resolves a node's parent with a first-match scan over the zone list,
so for nested zones the winner depends on model array order rather than on
nesting depth. Intended: the innermost containing zone wins.

### D-05 — Stale selection after edge delete

Deleting an edge re-selects the previous selection without checking that it
still exists, leaving the inspector bound to a deleted object. Intended: the
selection is cleared when its target is gone.

### D-06 — Save button located by its inline handler

The success animation finds the button with a
`button[onclick="downloadUpdatedJson()"]` selector, coupling behaviour to
markup. Removed by construction once inline handlers are gone.

### D-07 — Catalog duplicated in the application

`DEFAULT_CATALOG` re-states the entire contents of `catalog.js` as a fallback
that is dead whenever `catalog.js` loads, and stale whenever it does not.
Intended: one source, and a missing catalog is an explicit empty state.

### D-08 — Escaped newline in an error message

The save failure message contains a literal `\n` because the escape is
double-escaped in the source string.

### D-09 — No-op tooltip assignment

`resizer.title = "…"` is assigned on an SVG element, where `title` is not a
property and no tooltip appears. Intended: a `<title>` child element, or drop it.

### D-10 — Dead marker definition

The `#triangle` marker in `<defs>` uses `stroke="currentColor"` with no fill and
is referenced by nothing. Intended: removed.

### D-11 — Embedded fallback model

A complete copy of an old `model.json` is embedded in the source and loaded
whenever a fetch fails, so a broken path silently displays stale architecture
instead of reporting an error. Intended: a fetch failure is surfaced.

## 16a. Deviations introduced by the TypeScript port (pass 1)

The port was verified against `model-features.json` (now the `features` project —
file names here are historical, see [`CONTRACT.md`](CONTRACT.md)): all six models
load, render and round-trip byte-identically, and all twelve edge paths in the
fixture are character-for-character what the original produced. Three things
changed, all of them forced by the target structure rather than chosen.

### Δ1 — Containment resolves to the innermost container (fixes D-04)

The in-memory model is a real tree, so first-match-over-an-array no longer
exists to reproduce. On the fixture, `n_concept` now resolves to `zone_nested`
where it previously resolved to `zone_outer`.

### Δ2 — One collapsed-height constant (fixes D-01)

`HEADER_HEIGHT = 34` is used by rendering, hit testing and fit-to-view alike.
Hit testing and fit previously used 36, so both are two pixels tighter now.

### Δ3 — Structural defects removed by construction

D-03 (inspector HTML injection), D-06 (button located by its inline handler),
D-07 (catalog duplicated in the source), D-10 (dead marker) and D-11 (embedded
fallback model) have no expression in the new structure: the inspector builds
DOM nodes, inline handlers are gone, the catalog is fetched from
`catalog.json` (since ADR_20260923-7: the host's `GET /api/workspace`), and a failed fetch is reported instead of silently replaced.

XML attribute escaping was added to the Draw.io export at the same time, since
it was the same defect class as D-03 in a different output.

D-05 (stale selection after edge delete) was fixed because the new deletion path
had to make an explicit choice either way. D-08 and D-09 had no expression in
the new code either — the message is a template literal and no tooltip is
assigned to an SVG element.

**Every defect in section 16 is now closed.** D-02 was the only one that needed
a deliberate second pass; see R-HIST-10.

### Structure

`R-REND-01` now names four layers as **zones → edges → nodes → overlay**. The
overlay layer holds container resize grips, which previously lived in a layer
called `resizers`; the stacking order is unchanged.

## 17. Open questions

### Q-01 — Semantic vs visual containment

The concept document treats logical membership and physical placement as
distinct, and `node.zone` is written on reparenting — but nothing ever reads it
except as a hint in `R-CONT-01` and the Draw.io export. Either the field becomes
authoritative, or the geometric rule does, and the field becomes derived.

### Q-02 — Collapse state lifetime

Collapse is deliberately not persisted (R-EDIT-11), so a review session's
expand/collapse work is lost on reload. Whether that is right depends on whether
collapse is a reading aid or part of the diagram's meaning.

### Q-03 — Read-only viewing mode

Everything is editable at all times, including generated models where hand edits
will be overwritten by the next generator run. A viewer mode may belong here, or
may belong to a separate consumer of the same canvas.

## 18. Added after the port

New capabilities, not present in the original. Recorded here with the same id
scheme so later work has something to check against.

### R-EDIT-12 — Resize from any side or corner

A selected element shows eight grips: four corners and four edge midpoints.
Dragging a north or west grip moves the origin as well as the size, so the
opposite edge stays where it was. Minimums (160 × 100 for containers, 100 × 40
for leaves) and grid snapping apply to every direction.

Grips are drawn at a constant screen size, dividing by the current zoom, so they
stay grabbable when zoomed out and unobtrusive when zoomed in.

### R-EDIT-13 — Multi-selection

`Ctrl`/`Cmd`/`Shift` + click adds an element to the selection or removes it.
`Shift` + drag on empty canvas sweeps a rubber band, which selects every element
**fully enclosed** by it — brushing past an element does not catch it. A plain
click on empty canvas clears the selection and pans as before.

Clicking an element that is already part of a multi-selection keeps the group,
so that dragging it moves everything rather than collapsing the selection to one.

### R-EDIT-14 — Group move

Dragging any selected element moves the whole selection by the same delta, with
containers still carrying their subtrees. Snapping is computed once from the
element under the pointer and applied to all, so relative positions are exact.

Reparenting stays a single-element operation: dropping a group into a container
does not re-parent anything.

### R-EDIT-15 — Group resize

With more than one element selected, the grips sit on the union of their boxes,
outlined with a dashed border. Dragging one maps every element from the old box
into the new one, scaling positions and sizes proportionally — the arrangement
is stretched, not re-laid-out. Containers carry their contents through the
scale.

### R-HIST-10 — Typing is one history step (closes D-02)

An inspector text edit holds the state from before the first keystroke and
commits it once the field has been quiet for 600 ms, or sooner if anything else
needs the history — a selection change, a gesture, a save, an undo, or loading
another model. Five keystrokes produce one undo step, not five and not none.

### R-REND-16 — Edge attachment is swappable

Three placements ship, chosen from the toolbar and never stored in the model:

| mode | behaviour |
| --- | --- |
| `center` (default) | every end at the middle of the facing side — the original behaviour, and several edges between one pair overlap exactly |
| `uniform` | ends sharing a side spread evenly along it |
| `discrete` | ends sit on a fixed grid along the side, group centred |

The distributing modes order both ends of an edge by the same key — position of
the opposite endpoint, then type, then id — so lines stay parallel rather than
crossing. Placement is a pure function of the model, which is why no anchor
field exists in the JSON and why switching modes is free.

### R-REND-17 — Shape-Aware Geometry and Insets (`cornerInset`)

Safe attachment spans and corner insets are determined by each shape renderer via `ElementRenderer.cornerInset(side, style)`, never by global canvas-wide constants.
- Rounded boxes (`BoxRenderer`) report their resolved corner radius (`style.radius ?? 8`), container zones (`ContainerRenderer`) report `style.radius ?? 10`, sharp UML shapes report `0`.
- The router (`EdgeRouter.sharedRun`) constructs straight horizontal/vertical runs only across the true overlapping straight spans of both shapes, avoiding curved corner attachments and preventing premature Bezier curve breaks (see [`ADR_20260901_diagrams_shape-aware-geometry-and-insets.md`](../../../docs/adr/ADR_20260901_diagrams_shape-aware-geometry-and-insets.md)).

### R-CRUD-08 — Delete removes the whole selection

Deleting with several elements selected removes all of them, each with the edges
that referenced it.

### R-INSP-11 — Multi-selection readout

With several elements selected the badge shows the count, and the panel states
that geometry operations apply to the group while the fields below edit the
primary element only.

### Deviation — grips require selection

The original gave every node an invisible resize grip that worked without
selecting it first. Grips are now drawn only around the current selection,
because with several elements selected they belong to the selection's box rather
than to any one element. Selecting an element is one click, and it is what every
other editor requires.

## 19. Out of scope

Deliberately absent, and to remain absent through the migration:

- automatic layout of any kind, global or local;
- orthogonal or obstacle-avoiding edge routing;
- multi-select and group operations;
- copy, paste, duplicate;
- alignment and distribution tools;
- level-of-detail switching by zoom;
- collaboration, comments, presence;
- model validation and schema enforcement;
- generation of models — that belongs to the layout generator.
