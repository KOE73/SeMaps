/**
 * Pure geometry. No DOM, no model, no rendering.
 */

export interface Point {
  readonly x: number;
  readonly y: number;
}

export interface Size {
  readonly width: number;
  readonly height: number;
}

export interface Rect {
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

/** The four sides of an axis-aligned box, named by compass direction. */
export type Side = "north" | "east" | "south" | "west";

/**
 * A place on an element's boundary, expressed independently of the element's
 * shape: which side, and how far along it (0 = start, 0.5 = middle, 1 = end).
 *
 * This indirection is what lets port assignment work on shapes that are not
 * rectangles. The assigner decides "east side, 30% along"; the shape decides
 * where that lands — a straight edge for a box, an arc for an ellipse.
 */
export interface BoundarySlot {
  readonly side: Side;
  /** Position along the side, 0..1, measured clockwise. */
  readonly t: number;
}
