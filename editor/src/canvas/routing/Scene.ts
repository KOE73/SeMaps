import { bottom, right } from "../../geometry/rect.js";
import type { Point, Rect } from "../../geometry/types.js";
import { routingTuning } from "./tuning.js";

/**
 * What the canvas looks like to a router: everything in the way, with a price
 * on it.
 *
 * One structure, two consumers — path finding and lane nudging. They used to
 * take two differently-shaped lists ("obstacles" and "boundaries") and each
 * decided for itself what those meant, which is how a rule ends up enforced in
 * one phase and forgotten in the other.
 *
 * The price is the whole trick. A router pays **per unit of length spent
 * inside a zone**, so "crossing a container's border is fine, running along it
 * is not" needs no rule at all: crossing a 20px band costs 20 units, hugging it
 * for 300px costs 300. Every preference we have — avoid this, prefer that —
 * becomes a weight instead of a branch. That is what keeps this from turning
 * into the thousand special cases it replaced.
 */

/** A region a route may not enter at all. */
export const SOLID = Number.POSITIVE_INFINITY;

export interface RouteZone {
  readonly rect: Rect;
  /**
   * Extra cost per unit length travelled inside. `SOLID` forbids entry.
   *
   * Costs are relative to plain distance, whose weight is 1: a zone of weight
   * 3 makes a detour worth taking when it is shorter than four times the
   * stretch it avoids.
   */
  readonly weight: number;
  /**
   * Element this zone belongs to, so an edge can ignore the two shapes it is
   * attached to — and the containers holding them — without anyone rebuilding
   * the list per edge.
   */
  readonly ownerId: string;
  /**
   * A line already drawn, not a shape. Its flanks are worth a grid line only
   * when the budget has room after the shapes' own — a detour around a block
   * matters more than a lane next to another line.
   */
  readonly lane?: boolean;
  /**
   * A soft preference, not an outline: it offers no grid lines of its own and
   * is no wall for nudging. A container's interior is one — crossing it is
   * fine, but a run that could have taken the free space between containers
   * should.
   */
  readonly area?: boolean;
  /** Priced, but no wall for nudging: a block's halo, which a lane may sit in. */
  readonly halo?: boolean;
  /** The band straddling a container's outline. */
  readonly frame?: boolean;
}

export interface RouteScene {
  readonly zones: readonly RouteZone[];
}

/** A scene with nothing in it: routing degrades to plain geometry. */
export const EMPTY_SCENE: RouteScene = { zones: [] };

/*
 * The fine constants — clearance, band widths and weights, lane gap — live in
 * `routingTuning` (./tuning.ts), read at call time so the panel can move them.
 */

/**
 * A finished route as a band around each of its segments, for the routes
 * found after it: following it until the two are one stroke is expensive,
 * crossing it is nearly free. The same pricing that keeps lines off a
 * container's frame keeps them off each other — which matters most at the
 * ends, where the separation pass may not move anything.
 */
export function laneZones(points: readonly Point[], ownerId: string): RouteZone[] {
  const laneWidth = routingTuning.laneWidth;
  const half = laneWidth / 2;
  const out: RouteZone[] = [];
  for (let i = 0; i < points.length - 1; i++) {
    const a = points[i]!;
    const b = points[i + 1]!;
    if (!Number.isFinite(a.x) || !Number.isFinite(a.y) || !Number.isFinite(b.x) || !Number.isFinite(b.y)) continue;
    const vertical = Math.abs(a.x - b.x) < 0.01;
    const horizontal = Math.abs(a.y - b.y) < 0.01;
    if (!vertical && !horizontal) continue;
    const rect = vertical
      ? { x: a.x - half, y: Math.min(a.y, b.y), width: laneWidth, height: Math.abs(a.y - b.y) }
      : { x: Math.min(a.x, b.x), y: a.y - half, width: Math.abs(a.x - b.x), height: laneWidth };
    if (rect.width < 0.5 || rect.height < 0.5) continue;
    out.push({ rect, weight: routingTuning.laneWeight, ownerId, lane: true });
  }
  return out;
}

/**
 * Blocks become forbidden rectangles, grown by the clearance so that a line
 * never grazes an outline it is merely passing.
 */
export function solidZone(rect: Rect, ownerId: string, clearance = routingTuning.clearance): RouteZone {
  return {
    rect: {
      x: rect.x - clearance,
      y: rect.y - clearance,
      width: rect.width + clearance * 2,
      height: rect.height + clearance * 2,
    },
    weight: SOLID,
    ownerId,
  };
}

/** The four bands around a solid block where hugging its outline is priced. */
export function haloZones(rect: Rect, ownerId: string, clearance = routingTuning.clearance): RouteZone[] {
  const haloWidth = routingTuning.haloWidth;
  const x0 = rect.x - clearance - haloWidth;
  const y0 = rect.y - clearance - haloWidth;
  const w = rect.width + (clearance + haloWidth) * 2;
  const make = (x: number, y: number, width: number, height: number): RouteZone => ({
    rect: { x, y, width, height },
    weight: routingTuning.haloWeight,
    ownerId,
    halo: true,
  });
  return [
    make(x0, y0, w, haloWidth),
    make(x0, bottom(rect) + clearance, w, haloWidth),
    make(x0, rect.y - clearance, haloWidth, rect.height + clearance * 2),
    make(right(rect) + clearance, rect.y - clearance, haloWidth, rect.height + clearance * 2),
  ];
}

/** A container's inside, lightly priced. */
export function areaZone(rect: Rect, ownerId: string): RouteZone {
  return { rect, weight: routingTuning.areaWeight, ownerId, area: true };
}

/**
 * A container contributes a band straddling its outline, not its interior.
 *
 * Its inside is legal territory — that is where the nodes it holds live, and a
 * relation entering the container has to get there. What reads badly is a line
 * that *follows* the frame until the two are one stroke, and that is precisely
 * what a per-length price on a narrow band discourages while leaving a
 * perpendicular crossing almost free.
 */
export function borderZones(rect: Rect, ownerId: string, header = 0, clearance = routingTuning.clearance): RouteZone[] {
  const band = clearance * 2;
  const r = right(rect);
  const b = bottom(rect);
  const make = (x: number, y: number, width: number, height: number): RouteZone => ({
    rect: { x, y, width, height },
    weight: routingTuning.borderWeight,
    ownerId,
    frame: true,
  });

  return [
    make(rect.x - clearance, rect.y - clearance, rect.width + band, band),
    make(rect.x - clearance, b - clearance, rect.width + band, band),
    make(rect.x - clearance, rect.y - clearance, band, rect.height + band),
    make(r - clearance, rect.y - clearance, band, rect.height + band),
    // The caption strip reads as part of the frame: a line running along it
    // crosses the title, so it is priced like the outline — crossing it is fine.
    ...(header > clearance ? [make(rect.x, rect.y + clearance, rect.width, header - clearance)] : []),
  ];
}

/**
 * The zones that apply to one edge.
 *
 * Derived by filtering, never rebuilt: the scene is assembled once per repaint,
 * and an edge's own shapes — plus every container that holds one of its ends —
 * simply drop out of it. Without that an edge could not leave its own block.
 */
export function zonesFor(
  scene: RouteScene,
  exclude: ReadonlySet<string>,
  holders: ReadonlySet<string> = new Set(),
): readonly RouteZone[] {
  if (exclude.size === 0 && holders.size === 0) return scene.zones;
  // A container holding an end is entered, so its inside is free — but its
  // frame still is not a path: crossing the band once costs the same for every
  // route, running along it is what the band is there to price.
  return scene.zones.filter((z) =>
    !exclude.has(z.ownerId) && (!holders.has(z.ownerId) || z.frame === true || z.halo === true));
}

/** Whether a point falls inside a rectangle, edges included. */
function inside(rect: Rect, p: Point): boolean {
  return p.x >= rect.x && p.x <= right(rect) && p.y >= rect.y && p.y <= bottom(rect);
}

/**
 * What one axis-aligned segment costs beyond its own length.
 *
 * Returns `null` when the segment is impossible — it enters something solid.
 * Overlap is measured, not sampled: a segment that clips a corner of a zone for
 * three pixels should pay for three pixels, or the cost function stops being a
 * distance and the search starts preferring nonsense.
 */
export function segmentPenalty(a: Point, b: Point, zones: readonly RouteZone[]): number | null {
  let penalty = 0;
  const horizontal = Math.abs(a.y - b.y) < 1e-6;

  for (const zone of zones) {
    const overlap = horizontal
      ? overlapAlongX(a, b, zone.rect)
      : overlapAlongY(a, b, zone.rect);
    if (overlap <= 0) continue;
    if (zone.weight === SOLID) return null;
    penalty += overlap * zone.weight;
  }
  return penalty;
}

/**
 * `segmentPenalty` over a uniform grid of buckets, built once for a list of zones.
 *
 * A segment only meets the zones in the buckets it passes through, so a query
 * stops scanning every zone of the picture. The answer is the same as the scan's,
 * to the last bit: the candidates are summed in the zones' own order, and a solid
 * overlap still gives `null`. Anything the grid cannot place (a non-finite
 * coordinate) falls back to the plain scan.
 */
export class ZoneIndex {
  private readonly cols: number;
  private readonly rows: number;
  private readonly minX: number;
  private readonly minY: number;
  private readonly cw: number;
  private readonly ch: number;
  private readonly cells: number[][];
  private readonly stamp: Int32Array;
  private readonly found: Int32Array;
  private visit = 0;
  private readonly usable: boolean;

  constructor(private readonly zones: readonly RouteZone[]) {
    const n = zones.length;
    this.stamp = new Int32Array(n);
    this.found = new Int32Array(n);
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    let finiteAll = true;
    for (const z of zones) {
      const r = z.rect;
      if (!Number.isFinite(r.x) || !Number.isFinite(r.y) || !Number.isFinite(r.width) || !Number.isFinite(r.height)) {
        finiteAll = false;
        break;
      }
      if (r.x < minX) minX = r.x;
      if (r.y < minY) minY = r.y;
      if (r.x + r.width > maxX) maxX = r.x + r.width;
      if (r.y + r.height > maxY) maxY = r.y + r.height;
    }
    this.usable = finiteAll && n > 8;
    const side = this.usable ? Math.max(1, Math.min(32, Math.ceil(Math.sqrt(n / 2)))) : 1;
    this.cols = side;
    this.rows = side;
    this.minX = minX;
    this.minY = minY;
    this.cw = Math.max((maxX - minX) / side, 1e-9);
    this.ch = Math.max((maxY - minY) / side, 1e-9);
    this.cells = [];
    if (!this.usable) return;
    for (let i = 0; i < side * side; i++) this.cells.push([]);
    for (let i = 0; i < n; i++) {
      const r = zones[i]!.rect;
      const x0 = this.col(r.x), x1 = this.col(r.x + r.width);
      const y0 = this.row(r.y), y1 = this.row(r.y + r.height);
      for (let cx = x0; cx <= x1; cx++) for (let cy = y0; cy <= y1; cy++) this.cells[cx * side + cy]!.push(i);
    }
  }

  private col(x: number): number {
    const c = Math.floor((x - this.minX) / this.cw);
    return c < 0 ? 0 : c >= this.cols ? this.cols - 1 : c;
  }

  private row(y: number): number {
    const c = Math.floor((y - this.minY) / this.ch);
    return c < 0 ? 0 : c >= this.rows ? this.rows - 1 : c;
  }

  penalty(a: Point, b: Point): number | null {
    if (!this.usable || !Number.isFinite(a.x) || !Number.isFinite(a.y) || !Number.isFinite(b.x) || !Number.isFinite(b.y)) {
      return segmentPenalty(a, b, this.zones);
    }
    const horizontal = Math.abs(a.y - b.y) < 1e-6;
    const x0 = this.col(horizontal ? Math.min(a.x, b.x) : a.x);
    const x1 = this.col(horizontal ? Math.max(a.x, b.x) : a.x);
    const y0 = this.row(horizontal ? a.y : Math.min(a.y, b.y));
    const y1 = this.row(horizontal ? a.y : Math.max(a.y, b.y));
    const stamp = ++this.visit;
    let count = 0;
    for (let cx = x0; cx <= x1; cx++) {
      for (let cy = y0; cy <= y1; cy++) {
        for (const i of this.cells[cx * this.rows + cy]!) {
          if (this.stamp[i] === stamp) continue;
          this.stamp[i] = stamp;
          this.found[count++] = i;
        }
      }
    }
    // Zones' own order: the sum must add up in the same sequence as the plain scan.
    const hits = this.found.subarray(0, count).sort();
    let penalty = 0;
    for (let k = 0; k < count; k++) {
      const zone = this.zones[hits[k]!]!;
      const overlap = horizontal ? overlapAlongX(a, b, zone.rect) : overlapAlongY(a, b, zone.rect);
      if (overlap <= 0) continue;
      if (zone.weight === SOLID) return null;
      penalty += overlap * zone.weight;
    }
    return penalty;
  }
}

function overlapAlongX(a: Point, b: Point, rect: Rect): number {
  if (a.y < rect.y || a.y > bottom(rect)) return 0;
  const lo = Math.max(Math.min(a.x, b.x), rect.x);
  const hi = Math.min(Math.max(a.x, b.x), right(rect));
  return hi - lo;
}

function overlapAlongY(a: Point, b: Point, rect: Rect): number {
  if (a.x < rect.x || a.x > right(rect)) return 0;
  const lo = Math.max(Math.min(a.y, b.y), rect.y);
  const hi = Math.min(Math.max(a.y, b.y), bottom(rect));
  return hi - lo;
}

/** Solid zones only, for phases that need a hard "may not enter" test. */
export function walls(zones: readonly RouteZone[]): Rect[] {
  return zones.filter((z) => z.weight === SOLID).map((z) => z.rect);
}

/**
 * Every priced zone as a hard wall — solid blocks and border bands alike.
 *
 * Nudging is a cosmetic pass, not the one guaranteeing a route exists: the
 * search already found a path and already paid to keep it off a container's
 * border band. If nudging is only told about solid blocks, it is free to slide
 * a lane straight into that band and undo exactly the clearance the search
 * bought — which is the routing sin this file exists to price, reappearing one
 * phase later. Treating the band as a wall here costs nothing when a lane has
 * room to spare, and nudging already falls back to leaving a lane in place
 * when no interval fits, so this can only pull a lane away from a border, not
 * break a route that legitimately crosses one.
 */
export function nudgeWalls(zones: readonly RouteZone[]): Rect[] {
  return zones.filter((z) => !z.area && !z.halo).map((z) => z.rect);
}

/** Whether a point sits inside any forbidden zone. */
export function blocked(p: Point, zones: readonly RouteZone[]): boolean {
  return zones.some((z) => z.weight === SOLID && inside(z.rect, p));
}
