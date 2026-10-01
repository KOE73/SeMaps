import { bottom, right } from "../../geometry/rect.js";
import type { Point, Rect } from "../../geometry/types.js";

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

/**
 * How far a line keeps off a shape it is not attached to. Also the width of
 * the discouraged band along a container's outline.
 */
export const CLEARANCE = 8;

/** Cost of a container's border band, per unit length. */
const BORDER_WEIGHT = 6;

/**
 * A block's halo: a band beyond its clearance where running alongside costs
 * extra, so a line that only passes a block keeps away from its outline
 * instead of tracing it. Crossing the band costs its width — next to nothing.
 */
const HALO_WIDTH = 24;
const HALO_WEIGHT = 1.5;

/**
 * Travel inside a container that does not hold either end, per unit length.
 * Light on purpose: it decides between a run through someone else's container
 * and a run of similar length through the space between containers.
 */
const AREA_WEIGHT = 0.5;

/** Spacing between routes that end up sharing a corridor. */
export const LANE_GAP = 10;

/** Cost of running along a line already drawn, per unit length. Crossing it costs almost nothing. */
const LANE_WEIGHT = 3;

/**
 * Width of that band. Wider than the separation pass's gap on purpose: two
 * lines 10 units apart are one stroke at the zoom a whole view is read at,
 * and "separate but indistinguishable" is the same failure as merged.
 */
const LANE_WIDTH = 24;

/**
 * A finished route as a band around each of its segments, for the routes
 * found after it: following it until the two are one stroke is expensive,
 * crossing it is nearly free. The same pricing that keeps lines off a
 * container's frame keeps them off each other — which matters most at the
 * ends, where the separation pass may not move anything.
 */
export function laneZones(points: readonly Point[], ownerId: string): RouteZone[] {
  const half = LANE_WIDTH / 2;
  const out: RouteZone[] = [];
  for (let i = 0; i < points.length - 1; i++) {
    const a = points[i]!;
    const b = points[i + 1]!;
    if (!Number.isFinite(a.x) || !Number.isFinite(a.y) || !Number.isFinite(b.x) || !Number.isFinite(b.y)) continue;
    const vertical = Math.abs(a.x - b.x) < 0.01;
    const horizontal = Math.abs(a.y - b.y) < 0.01;
    if (!vertical && !horizontal) continue;
    const rect = vertical
      ? { x: a.x - half, y: Math.min(a.y, b.y), width: LANE_WIDTH, height: Math.abs(a.y - b.y) }
      : { x: Math.min(a.x, b.x), y: a.y - half, width: Math.abs(a.x - b.x), height: LANE_WIDTH };
    if (rect.width < 0.5 || rect.height < 0.5) continue;
    out.push({ rect, weight: LANE_WEIGHT, ownerId, lane: true });
  }
  return out;
}

/**
 * Blocks become forbidden rectangles, grown by the clearance so that a line
 * never grazes an outline it is merely passing.
 */
export function solidZone(rect: Rect, ownerId: string, clearance = CLEARANCE): RouteZone {
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
export function haloZones(rect: Rect, ownerId: string, clearance = CLEARANCE): RouteZone[] {
  const x0 = rect.x - clearance - HALO_WIDTH;
  const y0 = rect.y - clearance - HALO_WIDTH;
  const w = rect.width + (clearance + HALO_WIDTH) * 2;
  const make = (x: number, y: number, width: number, height: number): RouteZone => ({
    rect: { x, y, width, height },
    weight: HALO_WEIGHT,
    ownerId,
    halo: true,
  });
  return [
    make(x0, y0, w, HALO_WIDTH),
    make(x0, bottom(rect) + clearance, w, HALO_WIDTH),
    make(x0, rect.y - clearance, HALO_WIDTH, rect.height + clearance * 2),
    make(right(rect) + clearance, rect.y - clearance, HALO_WIDTH, rect.height + clearance * 2),
  ];
}

/** A container's inside, lightly priced. */
export function areaZone(rect: Rect, ownerId: string): RouteZone {
  return { rect, weight: AREA_WEIGHT, ownerId, area: true };
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
export function borderZones(rect: Rect, ownerId: string, header = 0, clearance = CLEARANCE): RouteZone[] {
  const band = clearance * 2;
  const r = right(rect);
  const b = bottom(rect);
  const make = (x: number, y: number, width: number, height: number): RouteZone => ({
    rect: { x, y, width, height },
    weight: BORDER_WEIGHT,
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
