import type { Point, Rect, Side } from "../../geometry/types.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";
import type { RouteZone } from "./Scene.js";

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
   * Optional: `BezierRouter` never reasons about the scene, and a caller with
   * nothing to say leaves it out.
   */
  readonly zones?: readonly RouteZone[];
}

export interface Route {
  /** SVG path data. */
  readonly path: string;
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

function straightLine(from: Point, to: Point): Route {
  return {
    path: `M ${from.x} ${from.y} L ${to.x} ${to.y}`,
    labelAt: { x: (from.x + to.x) / 2, y: (from.y + to.y) / 2 - 6 },
  };
}
