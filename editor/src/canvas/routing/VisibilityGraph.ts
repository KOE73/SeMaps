import { bottom, right } from "../../geometry/rect.js";
import type { Point, Side } from "../../geometry/types.js";
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

export interface RouteQuery {
  readonly from: Point;
  readonly to: Point;
  readonly fromSide: Side;
  readonly toSide: Side;
  readonly zones: readonly RouteZone[];
}

/**
 * The polyline through the ports, or null when nothing gets through — in which
 * case the caller draws something plain rather than nothing at all.
 */
export function findRoute(query: RouteQuery): Point[] | null {
  const { from, to, fromSide, toSide, zones } = query;
  if (!finite(from) || !finite(to)) return null;

  const enter = stubPoint(from, fromSide, STUB);
  const exit = stubPoint(to, toSide, STUB);

  const xs = axisLines("x", [from.x, to.x, enter.x, exit.x], zones, from, to);
  const ys = axisLines("y", [from.y, to.y, enter.y, exit.y], zones, from, to);
  // A single line on one axis is not a degenerate grid, it is a straight
  // corridor — which is exactly the case of two ports facing each other on the
  // same row. Refusing it sent the commonest route of all down the fallback.
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

  const optional = new Set<number>();
  for (const zone of zones) {
    const near = axis === "x" ? zone.rect.x : zone.rect.y;
    const far = axis === "x" ? right(zone.rect) : bottom(zone.rect);
    for (const v of [near - CLEARANCE / 2, far + CLEARANCE / 2]) {
      const r = round(v);
      if (r >= lo && r <= hi && !required.has(r)) optional.add(r);
    }
  }

  const budget = Math.max(MAX_LINES_PER_AXIS - required.size, 0);
  const kept = [...optional]
    .sort((a, b) => Math.abs(a - centre) - Math.abs(b - centre) || a - b)
    .slice(0, budget);

  return [...required, ...kept].sort((a, b) => a - b);
}

/** Half-pixel grid: keeps coordinate identity exact without float surprises. */
function round(v: number): number {
  return Math.round(v * 2) / 2;
}

function finite(p: Point): boolean {
  return Number.isFinite(p.x) && Number.isFinite(p.y);
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
