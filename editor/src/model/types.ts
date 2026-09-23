import type { Rect } from "../geometry/types.js";
import type { RoutingMode } from "./style-types.js";
import type { EntityEntry, WireMetadata } from "./wire-types.js";

/**
 * The in-memory model: one element shape, one containment tree.
 *
 * On the wire, zones and nodes are two flat arrays whose shapes are nearly
 * identical and whose nesting is implied by geometry. In memory they are one
 * kind of thing in a real tree, because every consumer above this layer —
 * rendering, interaction, export — wants a parent and children, not a guess.
 */

export type ElementKind = "zone" | "node";

export interface DiagramElement {
  readonly id: string;
  /**
   * Whether this element came from `zones` or `nodes` on the wire, and which
   * array it goes back to on save. It also selects a default renderer, and it
   * is what `views` still discriminates on (highlightZones vs highlightNodes).
   */
  readonly kind: ElementKind;
  /** Free-form type string. Selects a renderer and a style; never an enum. */
  type: string;
  /** Unified caption: `zone.name` or `node.label` on the wire. */
  label: string;
  semanticId?: string;
  tags: string[];
  metadata: WireMetadata;
  /**
   * Pin this element to one named style, overriding the match by `type`.
   *
   * Absent is the normal case and the one to prefer: an element that says only
   * what it *is* keeps looking right when the look changes. This field is for
   * the exception — one box that must stand out, or a zone whose colour carries
   * meaning that no type expresses.
   *
   * There is deliberately no per-element colour: the old inline `style` on
   * zones is migrated into named styles on load (see `wire.ts`), because a
   * hundred one-off palettes is exactly the state this replaced.
   */
  styleId?: string;

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

  /**
   * How this element's parentage arrived, so that saving an untouched model
   * reproduces the file byte for byte. A node whose containment was inferred
   * from geometry must not silently gain a `zone` field just because it was
   * opened — that would rewrite files nobody edited.
   */
  origin?: {
    /** Whether the wire object carried a `zone` key at all. */
    readonly zoneDeclared: boolean;
    /** Parent resolved at load time; a change from it means a real edit. */
    readonly parentId: string | null;
  };
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
  /** Pin to one named style; otherwise the style named after `type` wins. */
  styleId?: string;
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
