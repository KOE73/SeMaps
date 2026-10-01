import { bottom, right } from "../../geometry/rect.js";
import type { Point, Rect, Side } from "../../geometry/types.js";
import { CLEARANCE, segmentPenalty, type RouteZone } from "./Scene.js";

/**
 * Finding an orthogonal route as a search, not as a pile of special cases.
 *
 * The grid is **sparse and meaningful**: lines are drawn only through
 * coordinates that matter — each zone's two flanks plus the ports themselves.
 * Sampling the canvas every few pixels buys nothing, because an optimal
 * orthogonal path only turns where something is in the way. Tens of blocks give
 * a couple of hundred candidate coordinates, and the search over them costs
 * less than the repaint that asked for it.
 *
 * Two properties matter more here than optimality:
 *
 * - **Perpendicular ends, by construction.** A port enters the grid on its own
 *   normal line, so leaving sideways is not rejected — it is unrepresentable.
 * - **Stability.** Bends cost real money and ties break deterministically, so
 *   nudging one block does not reshuffle the picture. On a hand-made layout a
 *   route that jumps on every drag is worse than a route that is merely long.
 */

/** What a turn costs, in units of length. Bends read as complexity. */
const BEND_COST = 40;

/** How far a route leaves a port before it is allowed to turn. */
const STUB = 14;

/** Grid lines further than this from the pair's bounding box do not help. */
const SEARCH_MARGIN = 240;

/**
 * Ceiling on grid lines per axis.
 *
 * Not an optimisation — a guarantee. A view with two hundred blocks would
 * otherwise build a grid whose size is quadratic in the model, and a canvas
 * that stutters while dragging is a worse failure than a route that takes a
 * slightly clumsier turn.
 */
const MAX_LINES_PER_AXIS = 64;

/**
 * How far an end may slide along its side, and what is already there.
 *
 * `lo`/`hi` bound the end's coordinate *along* the side (x on a top or bottom
 * side, y on a left or right one). The assigned port stays the cheapest spot;
 * moving away from it costs a little per pixel, so the end only moves when the
 * move buys something — a straight run instead of a staircase.
 */
export interface Slide {
  readonly lo: number;
  readonly hi: number;
  /** Where other lines already meet this side; landing next to one is priced, not forbidden. */
  readonly taken?: readonly number[];
}

/**
 * One side an end may use: where the side's anchor lies on the outline and how
 * far the end may slide along it. A shape without straight sides offers just
 * the anchor (no slide).
 */
export interface RouteEnd {
  readonly side: Side;
  readonly port: Point;
  readonly slide?: Slide;
}

export interface RouteQuery {
  readonly from: Point;
  readonly to: Point;
  readonly fromSide: Side;
  readonly toSide: Side;
  readonly zones: readonly RouteZone[];
  /** Absent: the end is the port and nothing else, as before. */
  readonly fromSlide?: Slide;
  readonly toSlide?: Slide;
  /**
   * Every side each end may use. When both are given the search chooses the
   * sides too (ADR_20261001-2) and `from`/`to`/`fromSide`/`toSide` are not read.
   */
  readonly fromEnds?: readonly RouteEnd[];
  readonly toEnds?: readonly RouteEnd[];
  /** The blocks the ends belong to: a stub shortens when the other block is close in front of it. */
  readonly fromRect?: Rect;
  readonly toRect?: Rect;
  /** The sides the ends had last time: any other side costs `SIDE_CHANGE_COST`. */
  readonly prevFromSide?: Side;
  readonly prevToSide?: Side;
  /** Debugging only: receives the grid the search ran on. */
  readonly onGrid?: (xs: readonly number[], ys: readonly number[]) => void;
}

/** The route and the sides its two ends ended up on. */
export interface FoundRoute {
  readonly points: Point[];
  readonly fromSide: Side;
  readonly toSide: Side;
}

/**
 * What using another side than last time costs. About a bend: enough that two
 * nearly equal routes do not swap sides on every pixel of a drag, little enough
 * that a clearly shorter route on another side still wins.
 */
const SIDE_CHANGE_COST = 30;

/**
 * What sliding an end to the far end of its side costs; in between it grows with
 * the square of the distance from the middle, measured in half-sides.
 *
 * Measured per pixel, every spot between two blocks' middles cost the same, and
 * a straight run between a narrow block and a wide one settled at the narrow
 * block's corner. In half-sides the narrow block's end is the dearer one to
 * move, so the line stands in its middle; between two equal blocks the square
 * puts it in the middle of their overlap — the line a person would draw, and no
 * rule saying so.
 */
const SLIDE_COST = 20;

function slideCost(v: number, middle: number, span: { lo: number; hi: number }): number {
  const half = Math.max((span.hi - span.lo) / 2, 1);
  const t = (v - middle) / half;
  return SLIDE_COST * t * t;
}

/**
 * Where two ends sliding along one axis are cheapest together: the minimum of
 * their two slide costs, kept inside the overlap. A grid line goes there, or the
 * search could only reach the nearest spot some other reason put a line on.
 */
function sharedSpot(a: { port: Point; span: { lo: number; hi: number } }, b: { port: Point; span: { lo: number; hi: number } }, axis: "x" | "y"): number | null {
  const lo = Math.max(a.span.lo, b.span.lo);
  const hi = Math.min(a.span.hi, b.span.hi);
  if (lo > hi) return null;
  const wa = 1 / Math.max((a.span.hi - a.span.lo) / 2, 1) ** 2;
  const wb = 1 / Math.max((b.span.hi - b.span.lo) / 2, 1) ** 2;
  return clamp((a.port[axis] * wa + b.port[axis] * wb) / (wa + wb), { lo, hi });
}

/** Landing this close to a line already on the side counts as sharing its spot. */
const TAKEN_GAP = 20;

/** What sharing a spot costs: more than a bend, so a second line takes the next free spot. */
const TAKEN_COST = 60;

/**
 * The polyline through the ports, or null when nothing gets through — in which
 * case the caller draws something plain rather than nothing at all.
 */
export function findRoute(query: RouteQuery): Point[] | null {
  return searchRoute(query)?.points ?? null;
}

/** `findRoute`, and the sides the ends used — the ones given, unless the query offered several. */
export function searchRoute(query: RouteQuery): FoundRoute | null {
  const { from, to, fromSide, toSide } = query;
  if (query.fromEnds?.length && query.toEnds?.length) return findChoosingRoute(query, query.fromEnds, query.toEnds);
  if (!finite(from) || !finite(to)) return null;
  const points = query.fromSlide || query.toSlide ? findSlidingRoute(query) : findPlainRoute(query);
  return points === null ? null : { points, fromSide, toSide };
}

function findPlainRoute(query: RouteQuery): Point[] | null {
  const { from, to, fromSide, toSide, zones } = query;

  const stub = stubLength(from, to, fromSide, toSide);
  const enter = stubPoint(from, fromSide, stub);
  const exit = stubPoint(to, toSide, stub);

  const xs = axisLines("x", [from.x, to.x, enter.x, exit.x], zones, from, to);
  const ys = axisLines("y", [from.y, to.y, enter.y, exit.y], zones, from, to);
  // A single line on one axis is not a degenerate grid, it is a straight
  // corridor — which is exactly the case of two ports facing each other on the
  // same row. Refusing it sent the commonest route of all down the fallback.
  query.onGrid?.(xs, ys);
  if (xs.length === 0 || ys.length === 0) return null;

  // The stub's own coordinates are seeded above, so both ends land on their
  // normal line exactly — that, and not a later check, is what makes every
  // route leave and arrive square to the shape.
  const start = index(xs, ys, enter);
  const goal = index(xs, ys, exit);
  if (start === null || goal === null) return null;

  const middle = search(xs, ys, start, goal, zones);
  if (middle === null) return null;

  return simplify([from, ...middle, to]);
}

/**
 * The same search with ends that may slide along their sides: every grid point
 * on the start's stub line is a possible start, every one on the goal's a
 * possible goal. Each start and goal carries its own price — the slide, the
 * crowding, and a bend when the route does not leave or arrive straight — so
 * one search picks the ports and the path together. That is what turns
 * "exit the middle, dodge, jog back to the assigned entry" into a single
 * straight run whenever a straight run exists, at either end or both.
 */
function findSlidingRoute(query: RouteQuery): Point[] | null {
  const { from, to, fromSide, toSide, zones } = query;
  const stub = stubLength(from, to, fromSide, toSide);
  const enter = stubPoint(from, fromSide, stub);
  const exit = stubPoint(to, toSide, stub);
  const fromAxis = alongAxis(fromSide);
  const toAxis = alongAxis(toSide);

  // Extra lines worth having: where the other end sits, pulled onto this side,
  // and the middle of the overlap when both slide the same way — the spots a
  // straight run would use.
  const extra: Record<"x" | "y", number[]> = { x: [], y: [] };
  const fromSpan = spanOf(query.fromSlide, from, fromAxis);
  const toSpan = spanOf(query.toSlide, to, toAxis);
  extra[fromAxis].push(clamp(to[fromAxis], fromSpan), fromSpan.lo, fromSpan.hi);
  extra[toAxis].push(clamp(from[toAxis], toSpan), toSpan.lo, toSpan.hi);
  if (fromAxis === toAxis) {
    const spot = sharedSpot({ port: from, span: fromSpan }, { port: to, span: toSpan }, fromAxis);
    if (spot !== null) extra[fromAxis].push(spot);
  }

  const xs = axisLines("x", [from.x, to.x, enter.x, exit.x, ...extra.x], zones, from, to);
  const ys = axisLines("y", [from.y, to.y, enter.y, exit.y, ...extra.y], zones, from, to);
  query.onGrid?.(xs, ys);
  if (xs.length === 0 || ys.length === 0) return null;

  const starts = endChoices(xs, ys, from, fromSide, outward(fromSide), enter, fromSpan, query.fromSlide?.taken, 0, zones);
  const goals = endChoices(xs, ys, to, toSide, inward(toSide), exit, toSpan, query.toSlide?.taken, 0, zones);
  if (starts.length === 0 || goals.length === 0) return null;

  const found = searchMany(xs, ys, starts, goals, zones);
  if (found === null) return null;
  return simplify([found.startPort, ...found.middle, found.goalPort]);
}

/**
 * The search with every side of both ends on offer. Each side brings its own
 * stub, its own slide range and its own direction of travel; the one search
 * then picks the sides together with the path, by the price of the path —
 * length, bends, zones, slide and crowding — and nothing else. A side used last
 * time is the only thing that is cheaper than an equal one.
 */
function findChoosingRoute(query: RouteQuery, fromEnds: readonly RouteEnd[], toEnds: readonly RouteEnd[]): FoundRoute | null {
  const { zones, fromRect, toRect } = query;
  const plan = (ends: readonly RouteEnd[], own: Rect | undefined, other: Rect | undefined) =>
    ends
      .filter((e) => finite(e.port))
      .map((end) => {
        const stub = sideStub(end.port, end.side, own, other);
        return { end, enter: stubPoint(end.port, end.side, stub), span: spanOf(end.slide, end.port, alongAxis(end.side)) };
      });
  const fromPlan = plan(fromEnds, fromRect, toRect);
  const toPlan = plan(toEnds, toRect, fromRect);
  if (fromPlan.length === 0 || toPlan.length === 0) return null;

  // Lines worth having: each side's stub line, its port and slide bounds, where
  // the other end's ports sit pulled onto this side, and the middle of an
  // overlap when two sides slide along the same axis — the spots a straight run
  // would use. Rounding merges the many that coincide (a top and a bottom side
  // share their x), which is what keeps the grid within the ceiling.
  const extra: Record<"x" | "y", number[]> = { x: [], y: [] };
  const pull = (mine: typeof fromPlan, theirs: typeof toPlan) => {
    for (const a of mine) {
      const axis = alongAxis(a.end.side);
      extra[axis].push(a.span.lo, a.span.hi);
      for (const b of theirs) extra[axis].push(clamp(b.end.port[axis], a.span));
    }
  };
  pull(fromPlan, toPlan);
  pull(toPlan, fromPlan);
  for (const a of fromPlan) {
    for (const b of toPlan) {
      const axis = alongAxis(a.end.side);
      if (axis !== alongAxis(b.end.side)) continue;
      const spot = sharedSpot({ port: a.end.port, span: a.span }, { port: b.end.port, span: b.span }, axis);
      if (spot !== null) extra[axis].push(spot);
    }
  }
  const stubs = [...fromPlan, ...toPlan].flatMap((p) => [p.end.port, p.enter]);
  // The grid's bounding box covers both blocks and every stub, not just two ports.
  const all =[...stubs, ...[fromRect, toRect].flatMap((r) => (r ? [{ x: r.x, y: r.y }, { x: right(r), y: bottom(r) }] : []))];
  const low = { x: Math.min(...all.map((p) => p.x)), y: Math.min(...all.map((p) => p.y)) };
  const high = { x: Math.max(...all.map((p) => p.x)), y: Math.max(...all.map((p) => p.y)) };

  const xs = axisLines("x", [...stubs.map((p) => p.x), ...extra.x], zones, low, high);
  const ys = axisLines("y", [...stubs.map((p) => p.y), ...extra.y], zones, low, high);
  query.onGrid?.(xs, ys);
  if (xs.length === 0 || ys.length === 0) return null;

  const starts = fromPlan.flatMap((p) =>
    endChoices(xs, ys, p.end.port, p.end.side, outward(p.end.side), p.enter, p.span, p.end.slide?.taken,
      p.end.side === query.prevFromSide || !query.prevFromSide ? 0 : SIDE_CHANGE_COST, zones));
  const goals = toPlan.flatMap((p) =>
    endChoices(xs, ys, p.end.port, p.end.side, inward(p.end.side), p.enter, p.span, p.end.slide?.taken,
      p.end.side === query.prevToSide || !query.prevToSide ? 0 : SIDE_CHANGE_COST, zones));
  if (starts.length === 0 || goals.length === 0) return null;

  // The blocks themselves are walls for the middle of the route: the end's own
  // shapes are not in `zones` (a line must be able to leave them), and with every
  // side on offer a route could otherwise cut straight through its own block.
  // Not for a container that holds the other end — that line has to go inside.
  const walls = (own: Rect | undefined, other: Rect | undefined): RouteZone[] =>
    own && !(other && contains(own, other))
      ? [{ rect: { x: own.x + 1, y: own.y + 1, width: Math.max(own.width - 2, 0), height: Math.max(own.height - 2, 0) }, weight: Number.POSITIVE_INFINITY, ownerId: "end" }]
      : [];
  const found = searchMany(xs, ys, starts, goals, [...zones, ...walls(fromRect, toRect), ...walls(toRect, fromRect)]);
  if (found === null) return null;
  return {
    points: simplify([found.startPort, ...found.middle, found.goalPort]),
    fromSide: found.startSide,
    toSide: found.goalSide,
  };
}

function contains(outer: Rect, inner: Rect): boolean {
  return inner.x >= outer.x && inner.y >= outer.y && right(inner) <= right(outer) && bottom(inner) <= bottom(outer);
}

interface EndChoice {
  /** Grid node of the stub point. */
  readonly node: number;
  /** The point on the block's side the line touches. */
  readonly port: Point;
  /** Slide and crowding, before any bend. */
  readonly cost: number;
  /** The side this choice is on, and the direction of travel through its stub (out of a start, into a goal). */
  readonly side: Side;
  readonly dir: Dir;
}

/** The slide range along the side, or just the port itself when the end may not slide. */
function spanOf(slide: Slide | undefined, port: Point, axis: "x" | "y"): { lo: number; hi: number } {
  if (!slide || !(slide.lo <= slide.hi)) return { lo: port[axis], hi: port[axis] };
  return { lo: Math.min(slide.lo, port[axis]), hi: Math.max(slide.hi, port[axis]) };
}

function endChoices(
  xs: readonly number[],
  ys: readonly number[],
  port: Point,
  side: Side,
  dir: Dir,
  stub: Point,
  span: { lo: number; hi: number },
  taken: readonly number[] | undefined,
  /** A price every choice on this side pays: leaving the side the end had before. */
  bias: number,
  zones: readonly RouteZone[],
): EndChoice[] {
  const axis = alongAxis(side);
  const lines = axis === "x" ? xs : ys;
  const out: EndChoice[] = [];
  for (const v of lines) {
    if (v < round(span.lo) || v > round(span.hi)) continue;
    const p = axis === "x" ? { x: v, y: port.y } : { x: port.x, y: v };
    const s = axis === "x" ? { x: v, y: stub.y } : { x: stub.x, y: v };
    // The short run from the side to the stub must itself be clear.
    const penalty = segmentPenalty(p, s, zones);
    if (penalty === null) continue;
    const node = index(xs, ys, s);
    if (node === null) continue;
    const crowd = (taken ?? []).some((t) => Math.abs(t - v) < TAKEN_GAP) ? TAKEN_COST : 0;
    out.push({ node, port: p, side, dir, cost: slideCost(v, port[axis], span) + crowd + penalty + bias });
  }
  return out;
}

function alongAxis(side: Side): "x" | "y" {
  return side === "north" || side === "south" ? "x" : "y";
}

function clamp(v: number, span: { lo: number; hi: number }): number {
  return Math.min(Math.max(v, span.lo), span.hi);
}

/** The direction of travel leaving a side. */
function outward(side: Side): Dir {
  return side === "north" ? Dir.North : side === "south" ? Dir.South : side === "west" ? Dir.West : Dir.East;
}

/** The direction of travel arriving at a side. */
function inward(side: Side): Dir {
  return side === "north" ? Dir.South : side === "south" ? Dir.North : side === "west" ? Dir.East : Dir.West;
}

function index(xs: readonly number[], ys: readonly number[], p: Point): number | null {
  const xi = xs.indexOf(round(p.x));
  const yi = ys.indexOf(round(p.y));
  if (xi < 0 || yi < 0) return null;
  return xi * ys.length + yi;
}

/**
 * Where a route may turn on one axis.
 *
 * Each zone offers the two lines just outside itself — a route getting past
 * something wants to travel exactly there — plus the ports' own coordinates so
 * that a straight shot is always representable. When that yields more lines
 * than the ceiling allows, the ones nearest the pair survive: detours far from
 * both ends are the first thing nobody misses.
 */
function axisLines(
  axis: "x" | "y",
  seeds: readonly number[],
  zones: readonly RouteZone[],
  from: Point,
  to: Point,
): number[] {
  const lo = Math.min(from[axis], to[axis]) - SEARCH_MARGIN;
  const hi = Math.max(from[axis], to[axis]) + SEARCH_MARGIN;
  const centre = (from[axis] + to[axis]) / 2;

  const required = new Set<number>();
  for (const seed of seeds) if (Number.isFinite(seed)) required.add(round(seed));

  // Shapes first, lanes beside other lines only with what budget is left.
  const optional = new Set<number>();
  const lanes = new Set<number>();
  for (const zone of zones) {
    if (zone.area) continue;
    const near = axis === "x" ? zone.rect.x : zone.rect.y;
    const far = axis === "x" ? right(zone.rect) : bottom(zone.rect);
    for (const v of [near - CLEARANCE / 2, far + CLEARANCE / 2]) {
      const r = round(v);
      if (r >= lo && r <= hi && !required.has(r)) (zone.lane ? lanes : optional).add(r);
    }
  }

  const byCentre = (a: number, b: number) => Math.abs(a - centre) - Math.abs(b - centre) || a - b;
  const budget = Math.max(MAX_LINES_PER_AXIS - required.size, 0);
  const kept = [...optional].sort(byCentre).slice(0, budget);
  const keptSet = new Set(kept);
  const laneKept = [...lanes].filter((v) => !keptSet.has(v)).sort(byCentre).slice(0, Math.max(budget - kept.length, 0));

  return [...required, ...kept, ...laneKept].sort((a, b) => a - b);
}

/** Half-pixel grid: keeps coordinate identity exact without float surprises. */
function round(v: number): number {
  return Math.round(v * 2) / 2;
}

function finite(p: Point): boolean {
  return Number.isFinite(p.x) && Number.isFinite(p.y);
}

/**
 * How far both ends step out before they may turn: the full stub, unless the
 * two sides face each other closer than two stubs. Then each takes half the gap,
 * so the stubs meet instead of passing each other — passed stubs leave a goal
 * behind the start, and a route that may not double back can only reach it by a
 * loop. A gap of 20 between two blocks is one straight line, not four bends.
 */
function stubLength(from: Point, to: Point, fromSide: Side, toSide: Side): number {
  const out = sideVector(fromSide);
  const back = sideVector(toSide);
  if (out.x !== -back.x || out.y !== -back.y) return STUB;
  const gap = (to.x - from.x) * out.x + (to.y - from.y) * out.y;
  if (gap <= 0) return STUB;
  return Math.min(STUB, gap / 2);
}

/**
 * One side's step out, judged against the other block alone: the full stub,
 * unless that block lies right in front of the side and closer than two stubs.
 * Then the step is half the gap — the same reason as `stubLength`, but a side
 * that may not be the one the other end uses cannot wait for the other end's
 * port, so it looks at the block, and every side is judged by itself.
 */
function sideStub(port: Point, side: Side, own: Rect | undefined, other: Rect | undefined): number {
  if (!other) return STUB;
  const horizontal = side === "west" || side === "east";
  // In front means the other block overlaps this block's extent across the normal.
  const across = horizontal ? "y" : "x";
  const lo = own ? (horizontal ? own.y : own.x) : port[across];
  const hi = own ? (horizontal ? bottom(own) : right(own)) : port[across];
  const otherLo = horizontal ? other.y : other.x;
  const otherHi = horizontal ? bottom(other) : right(other);
  if (otherLo > hi || otherHi < lo) return STUB;
  const gap =
    side === "east" ? other.x - port.x :
    side === "west" ? port.x - right(other) :
    side === "south" ? other.y - port.y :
    port.y - bottom(other);
  if (gap <= 0 || gap >= STUB * 2) return STUB;
  return gap / 2;
}

function sideVector(side: Side): Point {
  switch (side) {
    case "north": return { x: 0, y: -1 };
    case "south": return { x: 0, y: 1 };
    case "west": return { x: -1, y: 0 };
    case "east": return { x: 1, y: 0 };
  }
}

function stubPoint(p: Point, side: Side, distance: number): Point {
  switch (side) {
    case "north": return { x: p.x, y: p.y - distance };
    case "south": return { x: p.x, y: p.y + distance };
    case "west": return { x: p.x - distance, y: p.y };
    case "east": return { x: p.x + distance, y: p.y };
  }
}

/** Direction of travel into a node, so that turning can be priced. */
const enum Dir { North = 0, East = 1, South = 2, West = 3, None = 4 }

/**
 * The opposite direction. A route never turns back on itself: on a grid a
 * U-turn only retraces a segment, and allowing it at the price of one bend let
 * a route slip into the gap between a stub and its block and hook into the port.
 */
function reverse(dir: Dir): Dir {
  return dir === Dir.None ? Dir.None : (((dir + 2) % 4) as Dir);
}

/**
 * A* over the sparse grid, with the arrival direction part of the state.
 *
 * Direction has to be in the state: without it the search cannot tell a
 * straight continuation from a turn, every equal-length path scores the same,
 * and the winner is whichever staircase the tie-break happened to reach first.
 */
function search(
  xs: readonly number[],
  ys: readonly number[],
  start: number,
  goal: number,
  zones: readonly RouteZone[],
): Point[] | null {
  const height = ys.length;
  const at = (node: number): Point => ({ x: xs[Math.floor(node / height)]!, y: ys[node % height]! });
  if (start === goal) return [at(start)];

  const goalPoint = at(goal);
  const best = new Map<number, number>();
  const cameFrom = new Map<number, number>();
  const open = new Heap();
  open.push(start * 5 + Dir.None, 0, 0);
  best.set(start * 5 + Dir.None, 0);

  while (!open.empty) {
    const { key, cost } = open.pop();
    const node = Math.floor(key / 5);
    const dir = (key % 5) as Dir;
    if ((best.get(key) ?? Infinity) < cost) continue;
    if (node === goal) return rebuild(cameFrom, key, at);

    const here = at(node);
    const gx = Math.floor(node / height);
    const gy = node % height;

    for (const step of [Dir.North, Dir.East, Dir.South, Dir.West]) {
      if (dir !== Dir.None && step === reverse(dir)) continue;
      const nx = gx + (step === Dir.East ? 1 : step === Dir.West ? -1 : 0);
      const ny = gy + (step === Dir.South ? 1 : step === Dir.North ? -1 : 0);
      if (nx < 0 || ny < 0 || nx >= xs.length || ny >= height) continue;

      const next = nx * height + ny;
      const there = at(next);
      const penalty = segmentPenalty(here, there, zones);
      if (penalty === null) continue;

      const length = Math.abs(there.x - here.x) + Math.abs(there.y - here.y);
      const turn = dir === Dir.None || dir === step ? 0 : BEND_COST;
      const nextCost = cost + length + penalty + turn;
      const nextKey = next * 5 + step;
      if (nextCost >= (best.get(nextKey) ?? Infinity)) continue;

      best.set(nextKey, nextCost);
      cameFrom.set(nextKey, key);
      const heuristic = Math.abs(goalPoint.x - there.x) + Math.abs(goalPoint.y - there.y);
      open.push(nextKey, nextCost, nextCost + heuristic);
    }
  }
  return null;
}

/**
 * A* from several starts to several goals. Starts are entered already
 * travelling outward, so a route that turns at once pays for the bend like any
 * other; a goal is only reached through a virtual last step that charges the
 * goal's own cost plus a bend when the route does not arrive head-on.
 */
function searchMany(
  xs: readonly number[],
  ys: readonly number[],
  starts: readonly EndChoice[],
  goals: readonly EndChoice[],
  zones: readonly RouteZone[],
): { startPort: Point; goalPort: Point; startSide: Side; goalSide: Side; middle: Point[] } | null {
  const height = ys.length;
  const at = (node: number): Point => ({ x: xs[Math.floor(node / height)]!, y: ys[node % height]! });
  const FINISH = -1;

  // Several goals may share a node — different sides whose stubs meet — each
  // with its own direction of arrival.
  const goalsAt = new Map<number, EndChoice[]>();
  for (const g of goals) {
    const list = goalsAt.get(g.node);
    if (list) list.push(g);
    else goalsAt.set(g.node, [g]);
  }
  const goalPoints = [...goalsAt.keys()].map(at);
  // Looked up for every pushed node, and with goals on four sides there are many
  // of them: the distance to the nearest goal is worked out once per node.
  const nearest = new Float64Array(xs.length * height).fill(-1);
  const heuristic = (node: number, p: Point): number => {
    const known = nearest[node]!;
    if (known >= 0) return known;
    let h = Infinity;
    for (const g of goalPoints) h = Math.min(h, Math.abs(g.x - p.x) + Math.abs(g.y - p.y));
    nearest[node] = h;
    return h;
  };

  const best = new Map<number, number>();
  const cameFrom = new Map<number, number>();
  const startOf = new Map<number, EndChoice>();
  let finishFrom: number | null = null;
  let finishGoal: EndChoice | null = null;
  const open = new Heap();
  for (const s of starts) {
    const key = s.node * 5 + s.dir;
    if (s.cost >= (best.get(key) ?? Infinity)) continue;
    best.set(key, s.cost);
    startOf.set(key, s);
    open.push(key, s.cost, s.cost + heuristic(s.node, at(s.node)));
  }

  while (!open.empty) {
    const { key, cost } = open.pop();
    if (key === FINISH) {
      if (finishFrom === null || finishGoal === null) return null;
      const middle = rebuild(cameFrom, finishFrom, at);
      let first = finishFrom;
      while (cameFrom.has(first)) first = cameFrom.get(first)!;
      const start = startOf.get(first);
      if (!start) return null;
      return { startPort: start.port, goalPort: finishGoal.port, startSide: start.side, goalSide: finishGoal.side, middle };
    }
    if ((best.get(key) ?? Infinity) < cost) continue;
    const node = Math.floor(key / 5);
    const dir = (key % 5) as Dir;

    // Arriving at the stub travelling away from the block would mean doubling
    // back over the stub — a hook at the arrowhead, never a route.
    for (const goal of goalsAt.get(node) ?? []) {
      if (dir === reverse(goal.dir)) continue;
      const total = cost + goal.cost + (dir === goal.dir ? 0 : BEND_COST);
      if (total < (best.get(FINISH) ?? Infinity)) {
        best.set(FINISH, total);
        finishFrom = key;
        finishGoal = goal;
        open.push(FINISH, total, total);
      }
    }

    const here = at(node);
    const gx = Math.floor(node / height);
    const gy = node % height;
    for (const step of [Dir.North, Dir.East, Dir.South, Dir.West]) {
      if (step === reverse(dir)) continue;
      const nx = gx + (step === Dir.East ? 1 : step === Dir.West ? -1 : 0);
      const ny = gy + (step === Dir.South ? 1 : step === Dir.North ? -1 : 0);
      if (nx < 0 || ny < 0 || nx >= xs.length || ny >= height) continue;

      const next = nx * height + ny;
      const there = at(next);
      const penalty = segmentPenalty(here, there, zones);
      if (penalty === null) continue;

      const length = Math.abs(there.x - here.x) + Math.abs(there.y - here.y);
      const turn = dir === step ? 0 : BEND_COST;
      const nextCost = cost + length + penalty + turn;
      const nextKey = next * 5 + step;
      if (nextCost >= (best.get(nextKey) ?? Infinity)) continue;

      best.set(nextKey, nextCost);
      cameFrom.set(nextKey, key);
      open.push(nextKey, nextCost, nextCost + heuristic(next, there));
    }
  }
  return null;
}

function rebuild(
  cameFrom: ReadonlyMap<number, number>,
  key: number,
  at: (node: number) => Point,
): Point[] {
  const points: Point[] = [];
  let cursor: number | undefined = key;
  const seen = new Set<number>();
  while (cursor !== undefined && !seen.has(cursor)) {
    seen.add(cursor);
    points.push(at(Math.floor(cursor / 5)));
    cursor = cameFrom.get(cursor);
  }
  return points.reverse();
}

/**
 * Binary heap keyed on the A* estimate.
 *
 * Written out rather than "scan the open list": the scan is quadratic, and the
 * point at which it stops being free is the point at which a real diagram
 * arrives.
 */
class Heap {
  private readonly keys: number[] = [];
  private readonly costs: number[] = [];
  private readonly scores: number[] = [];

  get empty(): boolean {
    return this.keys.length === 0;
  }

  push(key: number, cost: number, score: number): void {
    this.keys.push(key);
    this.costs.push(cost);
    this.scores.push(score);
    let i = this.keys.length - 1;
    while (i > 0) {
      const parent = (i - 1) >> 1;
      if (this.scores[parent]! <= this.scores[i]!) break;
      this.swap(i, parent);
      i = parent;
    }
  }

  pop(): { key: number; cost: number } {
    const key = this.keys[0]!;
    const cost = this.costs[0]!;
    const lastKey = this.keys.pop()!;
    const lastCost = this.costs.pop()!;
    const lastScore = this.scores.pop()!;
    if (this.keys.length > 0) {
      this.keys[0] = lastKey;
      this.costs[0] = lastCost;
      this.scores[0] = lastScore;
      let i = 0;
      for (;;) {
        const left = i * 2 + 1;
        const rightChild = left + 1;
        let smallest = i;
        if (left < this.keys.length && this.scores[left]! < this.scores[smallest]!) smallest = left;
        if (rightChild < this.keys.length && this.scores[rightChild]! < this.scores[smallest]!) smallest = rightChild;
        if (smallest === i) break;
        this.swap(i, smallest);
        i = smallest;
      }
    }
    return { key, cost };
  }

  private swap(a: number, b: number): void {
    [this.keys[a], this.keys[b]] = [this.keys[b]!, this.keys[a]!];
    [this.costs[a], this.costs[b]] = [this.costs[b]!, this.costs[a]!];
    [this.scores[a], this.scores[b]] = [this.scores[b]!, this.scores[a]!];
  }
}

/** Drop repeated points and points that merely continue a straight run. */
export function simplify(points: readonly Point[]): Point[] {
  const out: Point[] = [];
  for (const p of points) {
    const last = out[out.length - 1];
    if (last !== undefined && Math.abs(last.x - p.x) < 0.01 && Math.abs(last.y - p.y) < 0.01) continue;
    out.push(p);
  }
  for (let i = out.length - 2; i > 0; i--) {
    const a = out[i - 1]!;
    const b = out[i]!;
    const c = out[i + 1]!;
    const straight =
      (Math.abs(a.x - b.x) < 0.01 && Math.abs(b.x - c.x) < 0.01) ||
      (Math.abs(a.y - b.y) < 0.01 && Math.abs(b.y - c.y) < 0.01);
    if (straight) out.splice(i, 1);
  }
  return out;
}
