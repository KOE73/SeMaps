import type { BoundarySlot, Rect, Side } from "../../geometry/types.js";

export type EdgeEnd = "from" | "to";

export interface PortRequest {
  readonly edgeId: string;
  readonly end: EdgeEnd;
  readonly ownerId: string;
  /** The visible rectangle of the element this end attaches to. */
  readonly ownerRect: Rect;
  /** The visible rectangle at the other end, used to choose a side. */
  readonly otherRect: Rect;
  readonly edgeType: string;
  /** Shape-aware corner inset / radius of the owner shape (px) */
  readonly ownerInset?: number;
  /** Shape-aware corner inset / radius of the opposite shape (px) */
  readonly otherInset?: number;
}

export type PortKey = string;

export function portKey(edgeId: string, end: EdgeEnd): PortKey {
  return `${edgeId}#${end}`;
}

/**
 * Decides where each edge end attaches, in shape-independent terms.
 *
 * Swappable on purpose. Ends are placed in the middle of a side today, which
 * is what the original renderer did; spreading several edges along a side is a
 * different implementation of this same interface, not a change to the canvas.
 *
 * Assignment is a pure function of the model: no anchor is ever stored. That
 * keeps the JSON contract free of presentation detail, and avoids a second
 * source of truth that node moves would silently invalidate.
 */
export interface PortAssigner {
  readonly id: string;
  assign(requests: readonly PortRequest[]): Map<PortKey, BoundarySlot>;
}

/**
 * Convert "the n-th of m ends along this side, counted in increasing screen
 * coordinate" into the side's own parameter.
 *
 * Sides are parameterised clockwise, so east and north already run in
 * increasing screen coordinate while west and south run backwards. Flipping
 * them here is what stops two edges between the same pair of boxes from
 * crossing: both ends then order their ends the same way in screen space.
 */
export function slotAlongSide(side: Side, fraction: number): number {
  return side === "west" || side === "south" ? 1 - fraction : fraction;
}

/**
 * Comparator shared by every distributing assigner.
 *
 * Ordering by where the opposite end sits is what keeps edges on one side from
 * crossing each other. Type comes next so that edges of a kind group together
 * and the diagram reads consistently. The id is the final tie-break, so the
 * result is deterministic — and, crucially, identical when computed from the
 * other end of the same edge.
 */
export function compareRequests(a: PortRequest, b: PortRequest, side: Side): number {
  const axis = side === "east" || side === "west" ? "y" : "x";
  const pa = axis === "y" ? a.otherRect.y + a.otherRect.height / 2 : a.otherRect.x + a.otherRect.width / 2;
  const pb = axis === "y" ? b.otherRect.y + b.otherRect.height / 2 : b.otherRect.x + b.otherRect.width / 2;
  if (pa !== pb) return pa - pb;
  if (a.edgeType !== b.edgeType) return a.edgeType < b.edgeType ? -1 : 1;
  return a.edgeId < b.edgeId ? -1 : a.edgeId > b.edgeId ? 1 : 0;
}

export type { BoundarySlot };
