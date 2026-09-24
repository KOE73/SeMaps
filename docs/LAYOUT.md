# How a view is laid out

Reference for an agent the human has asked to do something with a view's geometry — typically
the rough work: "spread the subclasses into frames by meaning, stack each frame vertically". The
fine placement stays the human's. This file says how the picture works and what the defaults
are; *how* to arrange things is what the human asked for, not something this file decides.
Without such a request an agent does not write geometry at all
([ADR_20260924](adr/ADR_20260924_contract_agent-lays-out-views-on-request.md)). File format:
[CONTRACT.md](CONTRACT.md) §8.

## Two things that are not optional

- **The editor and you write the same file.** Saving from the editor rewrites the whole view and
  wipes what you wrote; your write wipes unsaved work in the editor. So the view has to be saved
  (or closed) before you write, and reloaded after.
- **Geometry you were not asked about stays as it is.** Existing zones and nodes keep their
  coordinates unless the request covers them.

## Coordinates

- Model units, absolute, **y grows downwards**. The origin is arbitrary; the editor fits the view
  on open.
- A node inside a zone still has **absolute** `x`/`y`, not relative to the zone.
- The editor's grid step is **10**; snapping is on by default, so the human's own moves land on
  multiples of 10.
- Membership is written, not inferred: a node is in a zone because its `zone` says so. A node drawn
  inside a frame with `zone: null` is legal, and the reverse is legal too (CONTRACT §8.2).

## Sizes (editor defaults)

| Thing | Default | Minimum |
|---|---|---|
| node | 180 × 60 | 100 × 40 |
| zone | — | 160 × 100 |
| zone caption strip | 28 high | |
| zone inner padding | 16 | |
| node corner radius | 8 | |

- Text is not wrapped. A caption longer than the box runs past its right edge; member rows of a
  template are cut with `…`. Caption font is 12.5 px sans; a character is roughly 7–8 units wide.
- A box's height does not follow its content. With a content template that lists members
  (`template: "class"`: fields, properties, methods, enum values, one section each), the rows are
  drawn from the top down and simply continue past the bottom edge if the box is short. Roughly:
  `55 + 15 × rows + 3 × sections`. The editor grows a box to fit when the human picks a template
  there; a height written into the file is used as is.

## Zones

- `container: null` — a frame that only groups visually and asserts nothing. `container: "c_…"`
  — a frame that stands for a container of `containers.json`, meaningful on the view's `axis`;
  two views on the same axis must not put one node into different containers (`semaps check`
  reports that).
- `parent` nests zones on this view; it may differ from the container hierarchy.
- Styles: the workspace's `styles.json`, `zone.*` for frames; a node's style is picked by its
  entity `kind` unless `styleId` names one.
- A collapsed zone (in the editor) shows as its caption strip; lines to its nodes then end at the
  zone.

## Lines

Lines are not stored in the view — every repaint computes them from the boxes.

- The shape comes from, most specific first: the edge's own `routing` → the view's `routing` →
  the relation type's style → a curve. Modes: `orthogonal` (right angles, goes around boxes),
  `bezier` (curve, ignores boxes), `tree-vertical`, `tree-horizontal` (buses, ignore boxes).
- A line leaves and enters a box on the side facing the other box. On a rectangle it may slide
  along that side to where it can run straight.
- Orthogonal lines keep **8** units off any box they pass, and a line costs more to run along
  another one than to cross it, so parallel lines settle roughly **16–24** units apart. A gap
  between two boxes narrower than about 16 is not a corridor.
- A box straight above another blocks the straight line from the lower one; the line then bends
  around it.
- Which relations show: a relation is drawn only when both of its entities are on the view, and
  `relations.default` / `relations.except` of the view decide the rest (CONTRACT §8.5).

## What the registry knows about grouping

Useful when a request says "by meaning" or "by assembly":

- `contains` relations: assembly → type, namespace → type, type → nested type (from `semaps sync`).
- `namespace` and `codeRef` on each entity.
- `extends` / `implements` relations; their arrows point at the base / the interface.

## Checking afterwards

`semaps check` does **not** verify the view's own references: that every node's `entity` exists
in `entities.json`, that every `zone` and `parent` names a zone of this view, that no entity is
placed twice. It does report a missing `axis` and containment contradictions between views.
