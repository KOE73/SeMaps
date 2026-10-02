import type { Point, Rect, Side } from "../../geometry/types.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";
import { SOLID, type RouteZone } from "./Scene.js";
import type { Slide } from "./VisibilityGraph.js";

/**
 * One side an end of an orthogonal line may use (ADR_20261001-2): where the side
 * touches the outline, how the end may slide along it and where the outline
 * really is for it. The router is given every side and picks.
 */
export interface EndSide {
  readonly side: Side;
  /** The side's anchor on the outline: the shape's `pointAt` at the middle of the side. */
  readonly port: Point;
  /** Absent: the shape offers just the anchor on this side. */
  readonly slide?: Slide;
  /** As `RouteRequest.fromDepth`, for this side. */
  readonly depth?: (along: number) => number;
  /** Shape-aware corner inset on this side (px). */
  readonly inset: number;
}

export interface RouteRequest {
  readonly from: Point;
  readonly to: Point;
  readonly fromSide: Side;
  readonly toSide: Side;
  readonly fromRect: Rect;
  readonly toRect: Rect;
  /** Shape-aware corner inset for the starting element (px) */
  readonly fromInset?: number;
  /** Shape-aware corner inset for the target element (px) */
  readonly toInset?: number;
  /** Marker length at the start of the line (px) */
  readonly fromMarkerOffset?: number;
  /** Marker length at the end of the line (px) */
  readonly toMarkerOffset?: number;
  /**
   * Everything in the way, priced (ADR_20260903 §2.8).
   *
   * One list rather than the two differently-meaning ones this replaced:
   * a forbidden block and a discouraged border band differ by a weight, not by
   * a kind, and the phases that consume them — search and nudging — must not be
   * free to disagree about what either means.
   *
   * Optional: `BezierRouter` only looks at the solid zones, to draw a straight
   * line when one is free; a caller with nothing to say leaves it out.
   */
  readonly zones?: readonly RouteZone[];
  /**
   * How far each end may slide along its side (rectangular blocks only). A
   * router that searches may move the port to where the line runs straight;
   * one that does not simply ignores this and uses `from`/`to`.
   */
  readonly fromSlide?: Slide;
  readonly toSlide?: Slide;
  /**
   * Where the outline really is, for a shape whose side is not straight: how
   * much deeper the outline lies at a coordinate along the side than at the
   * port the end was given (`from`/`to` are already on the outline there). A
   * sliding end keeps the port's depth; this moves it onto the outline.
   */
  readonly fromDepth?: (along: number) => number;
  readonly toDepth?: (along: number) => number;
  /**
   * Every side each end may use. A router that searches picks the sides itself
   * (`from`/`to`/`fromSide`/`toSide` then only serve as the plain fallback); one
   * that does not ignores this and draws between the sides it was given.
   */
  readonly fromEnds?: readonly EndSide[];
  readonly toEnds?: readonly EndSide[];
  /** The sides this line's ends used last time: keeping them is cheaper, so a drag does not make the line jump. */
  readonly prevFromSide?: Side;
  readonly prevToSide?: Side;
  /** Debugging only: a searching router hands over the grid it searched. */
  readonly onGrid?: (xs: readonly number[], ys: readonly number[]) => void;
}

export interface Route {
  /** SVG path data. */
  readonly path: string;
  /** The sides the ends actually use, when the router chose them (see `RouteRequest.fromEnds`). */
  readonly fromSide?: Side;
  readonly toSide?: Side;
  /** Where the edge's centre label belongs. */
  readonly labelAt: Point;
  /**
   * Where the cardinality/role label at the `from` end belongs (ADR_20260903 §2.6).
   *
   * Optional because only a router that knows its own path shape can place it
   * correctly — the shape lives here, not in the caller. A router that has not
   * been taught end labels yet simply omits them and the canvas draws none,
   * rather than guessing at a shape it does not own.
   */
  readonly fromLabelAt?: Point;
  /** Where the cardinality/role label at the `to` end belongs. Same caveat as `fromLabelAt`. */
  readonly toLabelAt?: Point;
  /**
   * The route as a polyline, when it is one.
   *
   * Nudging needs this: separating lines that share a corridor is a decision
   * about *several* routes at once, and it cannot be taken from a `d` string.
   * A router that draws curves omits it and is simply left out of that phase.
   */
  readonly points?: readonly Point[];
  /**
   * How corners are drawn, so that a rebuilt path after nudging looks like the
   * one the router produced. Absent means sharp.
   */
  readonly corners?: "sharp" | "rounded";
}

export interface EdgeRouter {
  readonly id: string;
  route(req: RouteRequest): Route;
}

function offsetPoint(p: Point, side: Side, distance: number): Point {
  if (distance <= 0) return p;
  switch (side) {
    case "north": return { x: p.x, y: p.y - distance };
    case "south": return { x: p.x, y: p.y + distance };
    case "west":  return { x: p.x - distance, y: p.y };
    case "east":  return { x: p.x + distance, y: p.y };
  }
}

/** Unit vector a side's outward normal points along. */
function sideVector(side: Side): Point {
  switch (side) {
    case "north": return { x: 0, y: -1 };
    case "south": return { x: 0, y: 1 };
    case "west":  return { x: -1, y: 0 };
    case "east":  return { x: 1, y: 0 };
  }
}

/**
 * Where an end label sits: a small step from the port, outward along the
 * line (away from the box, toward the middle of the edge, so the label
 * doesn't crowd the shape it's attached to), then off to the side so it
 * doesn't sit on top of the stroke itself.
 *
 * Driven by `side` rather than the actual path geometry because at both ends
 * of a bezier segment the tangent equals the port's own side — the curve
 * leaves and arrives along the straight line implied by its control points
 * (see `BezierRouter.route`). A router with a different path shape (e.g. an
 * orthogonal ladder) would compute this from its own first/last segment
 * instead; this helper is `BezierRouter`-local for that reason.
 */
function endLabelPoint(anchor: Point, side: Side, along: number, perp: number): Point {
  const dir = sideVector(side);
  // Rotate the outward direction 90° to get the offset that clears the line.
  const normal = { x: -dir.y, y: dir.x };
  return {
    x: anchor.x + dir.x * along + normal.x * perp,
    y: anchor.y + dir.y * along + normal.y * perp,
  };
}

export class BezierRouter implements EdgeRouter {
  readonly id = "bezier";

  /** Below this perpendicular offset, curving would add nothing but noise. */
  private static readonly STRAIGHT_THRESHOLD = DIAGRAM_CONFIG.routing.bezierStraightThreshold;
  /** How far a control point is pushed outward, capped so short hops stay gentle. */
  private static readonly MAX_HANDLE = DIAGRAM_CONFIG.routing.bezierMaxHandle;

  /** How far an end label sits from its port; see `endLabelPoint`. */
  private static readonly END_LABEL_ALONG = DIAGRAM_CONFIG.routing.endLabelAlong;
  private static readonly END_LABEL_PERP = DIAGRAM_CONFIG.routing.endLabelPerp;

  route(req: RouteRequest): Route {
    const { from, to, fromSide, toSide } = req;
    const fromOffset = req.fromMarkerOffset ?? 0;
    const toOffset = req.toMarkerOffset ?? 0;

    // With a scene: a straight line between the boxes, if one clears every block.
    const free = req.zones ? freeStraightLine(req) : null;
    if (free !== null) {
      const dx = free.b.x - free.a.x;
      const dy = free.b.y - free.a.y;
      const len = Math.hypot(dx, dy);
      if (len > fromOffset + toOffset + 1) {
        const ux = dx / len;
        const uy = dy / len;
        const a = { x: free.a.x + ux * fromOffset, y: free.a.y + uy * fromOffset };
        const b = { x: free.b.x - ux * toOffset, y: free.b.y - uy * toOffset };
        const sideOf = (x: number, y: number): Side =>
          Math.abs(x) >= Math.abs(y) ? (x >= 0 ? "east" : "west") : (y >= 0 ? "south" : "north");
        return {
          ...straightLine(a, b),
          fromLabelAt: endLabelPoint(a, sideOf(dx, dy), BezierRouter.END_LABEL_ALONG, BezierRouter.END_LABEL_PERP),
          toLabelAt: endLabelPoint(b, sideOf(-dx, -dy), BezierRouter.END_LABEL_ALONG, BezierRouter.END_LABEL_PERP),
        };
      }
    }

    const pFrom = offsetPoint(from, fromSide, fromOffset);
    const pTo = offsetPoint(to, toSide, toOffset);

    // Computed once and reused by every path shape below (straight or curved):
    // both ends leave along their own port's side regardless of how the
    // middle of the curve bends, so the anchor does not depend on that choice.
    const endLabels = {
      fromLabelAt: endLabelPoint(pFrom, fromSide, BezierRouter.END_LABEL_ALONG, BezierRouter.END_LABEL_PERP),
      toLabelAt: endLabelPoint(pTo, toSide, BezierRouter.END_LABEL_ALONG, BezierRouter.END_LABEL_PERP),
    };

    const horizontal = fromSide === "east" || fromSide === "west";

    if (horizontal) {
      if (Math.abs(pTo.y - pFrom.y) <= BezierRouter.STRAIGHT_THRESHOLD) {
        return { ...straightLine(pFrom, pTo), ...endLabels };
      }
      const handle = Math.min(Math.abs(pTo.x - pFrom.x) / 2, BezierRouter.MAX_HANDLE);
      const c1x = pFrom.x + (fromSide === "east" ? handle : -handle);
      const c2x = pTo.x + (toSide === "east" ? handle : -handle);
      return {
        path: `M ${pFrom.x} ${pFrom.y} C ${c1x} ${pFrom.y}, ${c2x} ${pTo.y}, ${pTo.x} ${pTo.y}`,
        labelAt: { x: (pFrom.x + pTo.x) / 2, y: (pFrom.y + pTo.y) / 2 - 6 },
        ...endLabels,
      };
    }

    // Vertical (north/south)
    if (Math.abs(pTo.x - pFrom.x) <= BezierRouter.STRAIGHT_THRESHOLD) {
      return { ...straightLine(pFrom, pTo), ...endLabels };
    }
    const handle = Math.min(Math.abs(pTo.y - pFrom.y) / 2, BezierRouter.MAX_HANDLE);
    const c1y = pFrom.y + (fromSide === "south" ? handle : -handle);
    const c2y = pTo.y + (toSide === "south" ? handle : -handle);
    return {
      path: `M ${pFrom.x} ${pFrom.y} C ${pFrom.x} ${c1y}, ${pTo.x} ${c2y}, ${pTo.x} ${pTo.y}`,
      labelAt: { x: (pFrom.x + pTo.x) / 2 + 8, y: (pFrom.y + pTo.y) / 2 },
      ...endLabels,
    };
  }
}

/** Whether the segment a-b passes through `r` grown by `pad` (Liang-Barsky). */
function segmentHitsRect(a: Point, b: Point, r: Rect, pad: number): boolean {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const p = [-dx, dx, -dy, dy];
  const q = [a.x - (r.x - pad), r.x + r.width + pad - a.x, a.y - (r.y - pad), r.y + r.height + pad - a.y];
  let t0 = 0;
  let t1 = 1;
  for (let i = 0; i < 4; i++) {
    if (p[i] === 0) {
      if (q[i]! < 0) return false;
    } else {
      const t = q[i]! / p[i]!;
      if (p[i]! < 0) { if (t > t1) return false; if (t > t0) t0 = t; }
      else { if (t < t0) return false; if (t < t1) t1 = t; }
    }
  }
  return true;
}

/**
 * A straight segment from the source box to the target box that crosses no
 * block, or null. Candidates, in order of preference: a line along the overlap
 * of the boxes' extents (square to the facing sides), the side midpoints (and
 * the ports the assigner gave), a few points spread along the facing sides.
 * The shortest free one of the first group that has any wins. Few candidates
 * on purpose: this runs for every edge on every drag.
 */
function freeStraightLine(req: RouteRequest): { a: Point; b: Point } | null {
  const fr = req.fromRect;
  const tr = req.toRect;
  const walls: Rect[] = [];
  for (const z of req.zones ?? []) if (z.weight === SOLID) walls.push(z.rect);
  const free = (a: Point, b: Point) => {
    for (const w of walls) if (segmentHitsRect(a, b, w, 2)) return false;
    return true;
  };
  const MARGIN = 6;

  const below = tr.y >= fr.y + fr.height;
  const above = tr.y + tr.height <= fr.y;
  const right = tr.x >= fr.x + fr.width;
  const left = tr.x + tr.width <= fr.x;
  const vertical = below || above;
  const horizontal = right || left;
  if (!vertical && !horizontal) return null;
  const fy = below ? fr.y + fr.height : fr.y;
  const ty = below ? tr.y : tr.y + tr.height;
  const fx = right ? fr.x + fr.width : fr.x;
  const tx = right ? tr.x : tr.x + tr.width;

  const along = (lo: number, size: number, f: number) => lo + MARGIN + (size - 2 * MARGIN) * f;
  const best = (cands: Array<[Point, Point]>): { a: Point; b: Point } | null => {
    let found: { a: Point; b: Point } | null = null;
    let bestLen = Infinity;
    for (const [a, b] of cands) {
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      if (len < bestLen && free(a, b)) { bestLen = len; found = { a, b }; }
    }
    return found;
  };

  // 1. Along the overlap of the extents.
  const overlap: Array<[Point, Point]> = [];
  const xLo = Math.max(fr.x, tr.x);
  const xHi = Math.min(fr.x + fr.width, tr.x + tr.width);
  const yLo = Math.max(fr.y, tr.y);
  const yHi = Math.min(fr.y + fr.height, tr.y + tr.height);
  for (const f of [0.5, 0.25, 0.75, 0.1, 0.9]) {
    if (vertical && xHi - xLo > 2 * MARGIN) {
      const x = along(xLo, xHi - xLo, f);
      overlap.push([{ x, y: fy }, { x, y: ty }]);
    }
    if (horizontal && yHi - yLo > 2 * MARGIN) {
      const y = along(yLo, yHi - yLo, f);
      overlap.push([{ x: fx, y }, { x: tx, y }]);
    }
  }
  // The first free one in preference order (the middle first); they differ little in length.
  for (const [a, b] of overlap) if (free(a, b)) return { a, b };

  // 2. Side midpoints, and the ports the assigner chose.
  const mids: Array<[Point, Point]> = [[req.from, req.to]];
  if (vertical) mids.push([{ x: fr.x + fr.width / 2, y: fy }, { x: tr.x + tr.width / 2, y: ty }]);
  if (horizontal) mids.push([{ x: fx, y: fr.y + fr.height / 2 }, { x: tx, y: tr.y + tr.height / 2 }]);
  const m = best(mids);
  if (m !== null) return m;

  // 3. A few points along the facing sides.
  const spread: Array<[Point, Point]> = [];
  const fs = [0.15, 0.5, 0.85];
  for (const f of fs) for (const g of fs) {
    if (vertical) spread.push([{ x: along(fr.x, fr.width, f), y: fy }, { x: along(tr.x, tr.width, g), y: ty }]);
    if (horizontal) spread.push([{ x: fx, y: along(fr.y, fr.height, f) }, { x: tx, y: along(tr.y, tr.height, g) }]);
  }
  return best(spread);
}

function straightLine(from: Point, to: Point): Route {
  return {
    path: `M ${from.x} ${from.y} L ${to.x} ${to.y}`,
    labelAt: { x: (from.x + to.x) / 2, y: (from.y + to.y) / 2 - 6 },
  };
}
