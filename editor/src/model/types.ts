import type { Rect } from "../geometry/types.js";
import type { RoutingMode } from "./style-types.js";
import type { EntityEntry, WireMetadata } from "./wire-types.js";
import type { PlacementOverride } from "./override.js";

/**
 * The in-memory model: one element shape, one containment tree.
 *
 * On the wire a view is one flat array of placements, each naming the container
 * it lies in (CONTRACT.md §8.2). In memory they form a real tree, because every
 * consumer above this layer — rendering, interaction, export — wants a parent
 * and children.
 */

/**
 * `zone` — a container placement (its entity's kind has `container: true` in
 * the kinds catalog), drawn as a frame; `node` — a block. Internal names: the
 * user-facing word is «контейнер».
 */
export type ElementKind = "zone" | "node";

export interface DiagramElement {
  readonly id: string;
  /**
   * Container or block, decided by the entity's kind when the view is loaded.
   * It selects the renderer and the style family (`appliesTo: container` /
   * `block`).
   */
  readonly kind: ElementKind;
  /** The entity's `kind` (CONTRACT.md §3). Selects a base style; never an enum. */
  type: string;
  /**
   * The entity's display name in the current text language (`entityDisplayName`):
   * derived state, kept current by `DiagramDocument.refreshNames()`.
   */
  label: string;
  semanticId?: string;
  tags: string[];
  metadata: WireMetadata;
  /**
   * Another style of the element's own kind, in place of the kind's base style.
   *
   * Absent is the normal case and the one to prefer: an element that says only
   * what it *is* keeps looking right when the look changes. The base style is
   * never written here — choosing it removes the field — and changing the kind
   * removes it too (ADR_20260927-7).
   */
  styleId?: string;
  /**
   * Partial style of this one placement, laid over its style when drawn
   * (CONTRACT.md §11.6). Only the fields of `OVERRIDE_FIELDS` exist here.
   */
  override?: PlacementOverride;

  /** Absolute model coordinates. Contract v1 stores these directly. */
  x: number;
  y: number;
  width: number;
  height: number;

  parent: DiagramElement | null;
  readonly children: DiagramElement[];

  /**
   * Position this element had in its wire array when loaded, so that saving
   * reproduces the file's original ordering instead of the tree's traversal
   * order. Without it the first save of an untouched model would produce a
   * whole-file diff. New elements get Infinity and are appended.
   */
  wireOrder: number;
  /**
   * The object this element was parsed from, kept so that fields this library
   * does not model survive a load/save round trip untouched.
   */
  readonly raw?: Record<string, unknown>;
}

export interface DiagramEdge {
  readonly id: string;
  from: string;
  to: string;
  label: string;
  /**
   * Cardinality/role captions at the two ends — "1", "0..*", "owner"
   * (ADR_20260903 §2.6). Plain strings in memory like every other text field;
   * their provenance travels beside them the same way `label`'s does.
   */
  fromLabel?: string;
  toLabel?: string;
  type: string;
  /**
   * Another style of the relation's own type, in place of the type's base
   * style. Never the base style itself; changing the type removes it
   * (ADR_20260930-2).
   */
  styleId?: string;
  /**
   * Colour, width and dash of this one edge on this view (CONTRACT.md §11.6):
   * only the fields of `EDGE_OVERRIDE_FIELDS`. Lives in the view's own `edges`
   * list, so setting one puts the view's edges there.
   */
  override?: PlacementOverride;
  /**
   * Where this edge came from. A `code` edge carries no text at all — see
   * `WireEdge.origin` — so the editor must not offer to edit `label`,
   * `fromLabel` or `toLabel` on one.
   */
  origin?: "code" | "authored";
  /**
   * Line shape for this one edge, overriding the view's and the type's choice.
   *
   * The exception, not the rule: when the shape of a line follows from the
   * relation's *type* it should be said once in the style, where it becomes a
   * reading cue — a bent line means structure, a curve means flow. Scattering
   * per-edge overrides is what destroys that cue (ADR_20260903 §2.7).
   */
  routing?: RoutingMode;
}

export interface DiagramView {
  readonly id: string;
  name: string;
  icon: string;
  description: string;
  highlightZones: string[];
  highlightNodes: string[];
}

export interface DiagramMetadata {
  title: string;
  layout?: string;
  description?: string;
  [key: string]: unknown;
}

export function elementRect(el: DiagramElement): Rect {
  return { x: el.x, y: el.y, width: el.width, height: el.height };
}

/**
 * The registry entry this element stands for, if it came from a project.
 *
 * It sits one level deeper than it looks: the loader stashes the entity on the
 * *wire node* as `raw._entity`, and the parser keeps that whole wire node as
 * the element's own `raw` — so the entity ends up at `raw.raw._entity`. Two
 * call sites had already guessed one level too shallow and silently got
 * nothing, which is exactly the kind of miss a helper with this comment above
 * it prevents from happening a third time.
 */
export function entityOf(el: DiagramElement): EntityEntry | undefined {
  const outer = el.raw as { raw?: { _entity?: EntityEntry }; _entity?: EntityEntry } | undefined;
  return outer?.raw?._entity ?? outer?._entity;
}

/** A container is an element that may hold children. Today: zones only. */
export function isContainer(el: DiagramElement): boolean {
  return el.kind === "zone";
}
