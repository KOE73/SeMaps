/**
 * Manhattan-style edge routers: the searched orthogonal route, and the two
 * "tree" shapes used by use-case flows (horizontal) and inheritance or org
 * hierarchies (vertical).
 *
 * ADR_20260903 §2.7: a router is a pure function of ports and scene. Nothing
 * here is cached or stored — the caller recomputes on every repaint and throws
 * the result away — which is what lets the algorithm improve without ever
 * migrating a file.
 *
 * The division of labour is deliberate. **Orthogonal** is the mode that tries
 * to be right: it searches a sparse grid, prices bends and zones, and comes out
 * square to both shapes. The **tree** modes are a *style* — one shared spine,
 * every child hanging off it — and a style that quietly rerouted itself around
 * obstacles would stop being the shape someone asked for. So they draw their
 * canonical figure and leave avoidance to the mode whose job it is.
 */
import type { Point, Side } from "../../geometry/types.js";
import type { EdgeRouter, Route, RouteRequest } from "./EdgeRouter.js";
import { findRoute, simplify } from "./VisibilityGraph.js";

/** Corner rounding for the searched route. Tree shapes stay deliberately sharp. */
const FILLET_RADIUS = 10;

function offsetPoint(p: Point, side: Side, distance: number): Point {
  if (distance <= 0) return p;
  switch (side) {
    case "north": return { x: p.x, y: p.y - distance };
    case "south": return { x: p.x, y: p.y + distance };
    case "west": return { x: p.x - distance, y: p.y };
    case "east": return { x: p.x + distance, y: p.y };
  }
}

function isHorizontal(side: Side): boolean {
  return side === "east" || side === "west";
}

function segLength(a: Point, b: Point): number {
  return Math.abs(a.x - b.x) + Math.abs(a.y - b.y);
}

/**
 * The label rides the middle of the longest straight run, not the middle of
 * the path.
 *
 * On a Manhattan route the geometric midpoint lands on a corner or a short jog
 * about as often as not, and a caption crowding an elbow is unreadable. The
 * longest run is where a person would have written it.
 */
function labelOnLongestSegment(points: readonly Point[]): Point {
  if (points.length === 0) return { x: 0, y: 0 };
  if (points.length === 1) return points[0]!;

  let best = { a: points[0]!, b: points[1]!, len: -1 };
  for (let i = 0; i < points.length - 1; i++) {
    const a = points[i]!;
    const b = points[i + 1]!;
    const len = segLength(a, b);
    if (len > best.len) best = { a, b, len };
  }
  const horizontal = Math.abs(best.a.y - best.b.y) < 0.01;
  return {
    x: (best.a.x + best.b.x) / 2 + (horizontal ? 0 : 8),
    y: (best.a.y + best.b.y) / 2 - (horizontal ? 6 : 0),
  };
}

/**
 * Polyline as SVG, with non-finite coordinates collapsed onto their neighbour.
 *
 * A `NaN` in the `d` attribute does not fail loudly: the line simply vanishes,
 * which reads as "the relation is gone" rather than "the router is broken".
 */
export function polylinePath(points: readonly Point[]): string {
  const cleaned: Point[] = [];
  for (const p of points) {
    // Fall back to the last point known good, not to the previous *input*:
    // a run of bad coordinates would otherwise copy one of its own kind
    // forward and leak a NaN into the attribute anyway.
    cleaned.push(
      Number.isFinite(p.x) && Number.isFinite(p.y) ? p : cleaned[cleaned.length - 1] ?? { x: 0, y: 0 },
    );
  }
  const first = cleaned[0];
  if (first === undefined) return "";
  return `M ${first.x} ${first.y}` + cleaned.slice(1).map((p) => ` L ${p.x} ${p.y}`).join("");
}

/**
 * Same polyline with rounded corners.
 *
 * Each fillet is capped at half of the shorter adjoining segment, so a tight
 * jog degrades to a sharp corner instead of two arcs overrunning each other.
 */
export function filletedPath(points: readonly Point[], radius = FILLET_RADIUS): string {
  const cleaned = points.filter((p) => Number.isFinite(p.x) && Number.isFinite(p.y));
  if (cleaned.length < 3 || radius <= 0) return polylinePath(cleaned);

  let d = `M ${cleaned[0]!.x} ${cleaned[0]!.y}`;
  for (let i = 1; i < cleaned.length - 1; i++) {
    const prev = cleaned[i - 1]!;
    const curr = cleaned[i]!;
    const next = cleaned[i + 1]!;
    const inLen = segLength(prev, curr);
    const outLen = segLength(curr, next);
    const r = Math.min(radius, inLen / 2, outLen / 2);

    if (r <= 0.01) {
      d += ` L ${curr.x} ${curr.y}`;
      continue;
    }
    const enter = {
      x: prev.x + (curr.x - prev.x) * ((inLen - r) / inLen),
      y: prev.y + (curr.y - prev.y) * ((inLen - r) / inLen),
    };
    const exit = {
      x: curr.x + (next.x - curr.x) * (r / outLen),
      y: curr.y + (next.y - curr.y) * (r / outLen),
    };
    d += ` L ${enter.x} ${enter.y} Q ${curr.x} ${curr.y}, ${exit.x} ${exit.y}`;
  }
  const last = cleaned[cleaned.length - 1]!;
  return `${d} L ${last.x} ${last.y}`;
}

function sideVector(side: Side): Point {
  switch (side) {
    case "north": return { x: 0, y: -1 };
    case "south": return { x: 0, y: 1 };
    case "west": return { x: -1, y: 0 };
    case "east": return { x: 1, y: 0 };
  }
}

const END_LABEL_ALONG = 14;
const END_LABEL_PERP = 9;

/** A step out along the port's own normal, then aside so the text clears the stroke. */
function endLabelPoint(anchor: Point, side: Side, along: number, perp: number): Point {
  const dir = sideVector(side);
  const normal = { x: -dir.y, y: dir.x };
  return {
    x: anchor.x + dir.x * along + normal.x * perp,
    y: anchor.y + dir.y * along + normal.y * perp,
  };
}

function endLabels(pFrom: Point, fromSide: Side, pTo: Point, toSide: Side) {
  return {
    fromLabelAt: endLabelPoint(pFrom, fromSide, END_LABEL_ALONG, END_LABEL_PERP),
    toLabelAt: endLabelPoint(pTo, toSide, END_LABEL_ALONG, END_LABEL_PERP),
  };
}

/**
 * The plain ladder, used when the search finds nothing at all.
 *
 * A crowded diagram should still show its relations: a line that cuts a corner
 * is a cosmetic failure, a missing line is a factual one.
 */
function ladder(pFrom: Point, fromSide: Side, pTo: Point, toSide: Side): Point[] {
  if (pFrom.x === pTo.x && pFrom.y === pTo.y) return [pFrom, { ...pTo }];

  const fromHoriz = isHorizontal(fromSide);
  const toHoriz = isHorizontal(toSide);

  if (fromHoriz && !toHoriz) return [pFrom, { x: pTo.x, y: pFrom.y }, pTo];
  if (!fromHoriz && toHoriz) return [pFrom, { x: pFrom.x, y: pTo.y }, pTo];
  if (fromHoriz) {
    const midX = (pFrom.x + pTo.x) / 2;
    return [pFrom, { x: midX, y: pFrom.y }, { x: midX, y: pTo.y }, pTo];
  }
  const midY = (pFrom.y + pTo.y) / 2;
  return [pFrom, { x: pFrom.x, y: midY }, { x: pTo.x, y: midY }, pTo];
}

/**
 * The shared spine of both tree modes: leave along the primary axis, run one
 * collector at the halfway point, arrive along the primary axis.
 *
 * The spine is drawn between the *stub* points, not the ports, so that the
 * line still leaves and arrives square to the shape even when the port's side
 * disagrees with the mode's axis — a vertical bus hanging off an east-facing
 * port used to strike the outline at an angle, which is the same defect the
 * searched router fixes by construction.
 */
function treePath(from: Point, fromSide: Side, to: Point, toSide: Side, axis: "x" | "y"): Point[] {
  const enter = stub(from, fromSide);
  const exit = stub(to, toSide);
  const spine = axis === "x" ? spineAlongX(enter, exit) : spineAlongY(enter, exit);
  return simplify([from, ...spine, to]);
}

function spineAlongX(a: Point, b: Point): Point[] {
  if (Math.abs(a.y - b.y) < 0.01) return [a, b];
  const midX = (a.x + b.x) / 2;
  return [a, { x: midX, y: a.y }, { x: midX, y: b.y }, b];
}

function spineAlongY(a: Point, b: Point): Point[] {
  if (Math.abs(a.x - b.x) < 0.01) return [a, b];
  const midY = (a.y + b.y) / 2;
  return [a, { x: a.x, y: midY }, { x: b.x, y: midY }, b];
}

/** One step out along the port's own normal — the promise of a square end. */
function stub(p: Point, side: Side): Point {
  return offsetPoint(p, side, STUB);
}

/** How far a line leaves a port before it may turn. Matches the searched router. */
const STUB = 14;

/**
 * Ports, with any non-finite coordinate replaced before it can travel.
 *
 * Cleaning at the boundary rather than at the end: a NaN that reaches the path
 * builder is caught there, but one that reaches `points` is not, and nudging
 * would then lay other routes out against a corridor at coordinate NaN.
 * Degenerate input should produce a drawable line, not spread.
 */
function ends(req: RouteRequest): { pFrom: Point; pTo: Point } {
  const from = usable(req.from, req.to);
  const to = usable(req.to, from);
  return {
    pFrom: offsetPoint(from, req.fromSide, req.fromMarkerOffset ?? 0),
    pTo: offsetPoint(to, req.toSide, req.toMarkerOffset ?? 0),
  };
}

function usable(p: Point, fallback: Point): Point {
  return {
    x: Number.isFinite(p.x) ? p.x : Number.isFinite(fallback.x) ? fallback.x : 0,
    y: Number.isFinite(p.y) ? p.y : Number.isFinite(fallback.y) ? fallback.y : 0,
  };
}

/**
 * The mode that routes properly: a search over the scene, priced by length,
 * bends and the zones it passes through.
 */
export class OrthogonalRouter implements EdgeRouter {
  readonly id = "orthogonal";

  route(req: RouteRequest): Route {
    const { pFrom, pTo } = ends(req);
    const searched = findRoute({
      from: pFrom,
      to: pTo,
      fromSide: req.fromSide,
      toSide: req.toSide,
      zones: req.zones ?? [],
    });
    const points = searched ?? simplify(ladder(pFrom, req.fromSide, pTo, req.toSide));

    return {
      path: filletedPath(points),
      points,
      corners: "rounded",
      labelAt: labelOnLongestSegment(points),
      ...endLabels(pFrom, req.fromSide, pTo, req.toSide),
    };
  }
}

/** Left-to-right bus: use-case flows, pipelines. */
export class TreeHorizontalRouter implements EdgeRouter {
  readonly id = "tree-horizontal";

  route(req: RouteRequest): Route {
    const { pFrom, pTo } = ends(req);
    const points = treePath(pFrom, req.fromSide, pTo, req.toSide, "x");
    return {
      path: polylinePath(points),
      points,
      labelAt: labelOnLongestSegment(points),
      ...endLabels(pFrom, req.fromSide, pTo, req.toSide),
    };
  }
}

/** Top-to-bottom bus: inheritance, org charts. */
export class TreeVerticalRouter implements EdgeRouter {
  readonly id = "tree-vertical";

  route(req: RouteRequest): Route {
    const { pFrom, pTo } = ends(req);
    const points = treePath(pFrom, req.fromSide, pTo, req.toSide, "y");
    return {
      path: polylinePath(points),
      points,
      labelAt: labelOnLongestSegment(points),
      ...endLabels(pFrom, req.fromSide, pTo, req.toSide),
    };
  }
}
