/**
 * Parallel-segment separation for orthogonal routes ("nudging").
 *
 * ADR_20260903: routing computes each edge's path independently, purely from
 * its own two ports and the obstacle set. That is the right contract for a
 * single route, but it means nothing stops two unrelated edges from landing
 * in the same corridor and drawing on top of each other — the reader loses
 * the connection entirely, even though neither route is individually wrong.
 * Fixing that is not a property of one path (it can't be, by construction of
 * how routes are computed), so it lives here as a separate post-pass that
 * looks at *all* routes together and spreads out the ones that collide.
 *
 * This is a one-dimensional layout problem, not a collision-avoidance patch:
 * for each corridor (a cluster of parallel segments occupying overlapping
 * space), decide how many lanes it needs, find the free interval the lanes
 * have to live in (bounded by the nearest walls), and place the lanes evenly
 * spaced inside that interval. A segment that could not physically be placed
 * without leaving its wall-bounded interval simply keeps its original
 * coordinate — better a visible overlap than a line drawn through a block.
 */
import type { Point, Rect } from "../../geometry/types.js";

export interface NudgeRoute {
  readonly id: string;
  /** Orthogonal polyline: adjacent points differ on exactly one axis. */
  readonly points: readonly Point[];
}

export interface NudgeInput {
  readonly routes: readonly NudgeRoute[];
  /** Impassable rectangles (blocks). Behave as fixed walls. */
  readonly walls: readonly Rect[];
  /** Desired gap between neighbouring lanes, px. */
  readonly gap: number;
}

// ------------------------------------------------------------- primitives

type Axis = "x" | "y";

/** `Point` with its fields writable — used only for the working copies this
 * module mutates in place while resolving corridors; the public contract
 * still traffics exclusively in readonly `Point`s. */
interface MutablePoint {
  x: number;
  y: number;
}

/** A single interior segment of one route, identified by its index so the
 * result can be written back into a mutable copy of that route's points. */
interface SegmentRef {
  readonly routeIndex: number;
  /** Index of the segment's first point; the segment is [i, i+1]. */
  readonly pointIndex: number;
  readonly axis: Axis;
  /** The segment's fixed coordinate on its own axis (x for a vertical
   * segment, y for a horizontal one) before nudging. */
  readonly coord: number;
  /** Perpendicular span the segment covers, used to test corridor overlap. */
  readonly spanLo: number;
  readonly spanHi: number;
  /** Coordinate of the point just outside this segment on the "from" end
   * (the neighbour that anchors sort order) — see grouping/sort below. */
  readonly outerAnchor: number;
  readonly routeId: string;
}

function isFinitePoint(p: Point): boolean {
  return Number.isFinite(p.x) && Number.isFinite(p.y);
}

/** Collect every *interior* segment (never the first or last of a route —
 * those are the stubs leaving/entering a port and must never move). */
function collectInteriorSegments(routes: readonly NudgeRoute[]): SegmentRef[] {
  const out: SegmentRef[] = [];
  routes.forEach((route, routeIndex) => {
    const pts = route.points;
    // Need at least 4 points (2 stubs + >=1 interior segment) for anything
    // interior to exist; a 2- or 3-point route is all stub, nothing to nudge.
    if (pts.length < 4) return;
    for (let i = 1; i + 1 < pts.length - 1; i++) {
      const a = pts[i];
      const b = pts[i + 1];
      if (!a || !b || !isFinitePoint(a) || !isFinitePoint(b)) continue;
      const dx = Math.abs(a.x - b.x);
      const dy = Math.abs(a.y - b.y);
      // Not axis-aligned (shouldn't happen for a valid orthogonal route):
      // leave it untouched rather than guess which axis it belongs to.
      if (dx > 1e-6 && dy > 1e-6) continue;
      // A horizontal segment (constant y) occupies a "y corridor"; a
      // vertical segment (constant x) occupies an "x corridor". Name the
      // axis by what stays constant, i.e. the axis you'd nudge it along.
      const nudgeAxis: Axis = dy <= 1e-6 ? "y" : "x";
      const coord = nudgeAxis === "y" ? a.y : a.x;
      const spanLo = nudgeAxis === "y" ? Math.min(a.x, b.x) : Math.min(a.y, b.y);
      const spanHi = nudgeAxis === "y" ? Math.max(a.x, b.x) : Math.max(a.y, b.y);
      // Anchor sort order on the point just before this segment (its
      // predecessor in the polyline). That point is fixed relative to this
      // corridor's own routing decision and does not depend on iteration
      // order over any Map/Set, so two runs over the same input always
      // produce the same anchor.
      const prev = pts[i - 1];
      const outerAnchor = prev ? (nudgeAxis === "y" ? prev.x : prev.y) : coord;
      out.push({
        routeIndex,
        pointIndex: i,
        axis: nudgeAxis,
        coord,
        spanLo,
        spanHi,
        outerAnchor,
        routeId: route.id,
      });
    }
  });
  return out;
}

/** Two segments belong to the same corridor when they run on the same axis,
 * sit within `tolerance` of each other's coordinate, and their perpendicular
 * spans overlap (otherwise they occupy different parts of the canvas and
 * have nothing to do with each other). */
function corridorsOverlap(a: SegmentRef, b: SegmentRef, tolerance: number): boolean {
  if (a.axis !== b.axis) return false;
  if (Math.abs(a.coord - b.coord) > tolerance) return false;
  return a.spanLo <= b.spanHi && b.spanLo <= a.spanHi;
}

/** Union-find over segment indices, used to cluster all pairwise-overlapping
 * segments (not just ones close to a single reference coordinate) into
 * connected corridors — a chain of overlapping segments can drift further
 * apart than `tolerance` end to end. */
function groupIntoCorridors(segments: readonly SegmentRef[], tolerance: number): number[][] {
  const parent = segments.map((_, i) => i);
  function find(i: number): number {
    while (parent[i] !== i) {
      const gp = parent[parent[i] as number] as number;
      parent[i] = gp;
      i = gp;
    }
    return i;
  }
  function union(a: number, b: number): void {
    const ra = find(a);
    const rb = find(b);
    if (ra !== rb) parent[ra] = rb;
  }
  for (let i = 0; i < segments.length; i++) {
    for (let j = i + 1; j < segments.length; j++) {
      const si = segments[i];
      const sj = segments[j];
      if (si && sj && corridorsOverlap(si, sj, tolerance)) union(i, j);
    }
  }
  const groups = new Map<number, number[]>();
  for (let i = 0; i < segments.length; i++) {
    const root = find(i);
    const list = groups.get(root);
    if (list) list.push(i);
    else groups.set(root, [i]);
  }
  // Sort corridors themselves by (min coord, then first route id) so the
  // overall processing order is also deterministic — not that it affects
  // the result (corridors are independent), but it keeps output/log order
  // stable for debugging.
  return Array.from(groups.values()).sort((a, b) => {
    const ca = Math.min(...a.map((i) => segments[i]?.coord ?? 0));
    const cb = Math.min(...b.map((i) => segments[i]?.coord ?? 0));
    return ca - cb;
  });
}

/** Deterministic lane order within one corridor: by the outer-anchor
 * coordinate (where the segment's own neighbour sits), then by route id as
 * an absolute tie-breaker. Never depends on array/object iteration order,
 * so nudging the same logical input twice — even with routes supplied in a
 * different order — assigns lanes identically. */
function laneOrder(segments: readonly SegmentRef[], indices: readonly number[]): number[] {
  return [...indices].sort((i, j) => {
    const a = segments[i];
    const b = segments[j];
    if (!a || !b) return 0;
    if (a.outerAnchor !== b.outerAnchor) return a.outerAnchor - b.outerAnchor;
    if (a.coord !== b.coord) return a.coord - b.coord;
    return a.routeId < b.routeId ? -1 : a.routeId > b.routeId ? 1 : 0;
  });
}

/** Nearest wall boundaries that bound the free interval a corridor's lanes
 * may occupy: the highest wall edge below `coord` on the "low" side, the
 * lowest wall edge above `coord` on the "high" side, considering only walls
 * whose extent along the corridor's own span actually overlaps it. */
function boundingInterval(
  axis: Axis,
  coord: number,
  spanLo: number,
  spanHi: number,
  walls: readonly Rect[],
): { lo: number; hi: number } {
  let lo = -Infinity;
  let hi = Infinity;
  for (const w of walls) {
    if (!Number.isFinite(w.x) || !Number.isFinite(w.y) || !Number.isFinite(w.width) || !Number.isFinite(w.height)) continue;
    if (axis === "y") {
      // Horizontal corridor: wall must overlap the corridor's x-span to matter.
      const wLo = w.x;
      const wHi = w.x + w.width;
      if (wHi < spanLo || wLo > spanHi) continue;
      const wTop = w.y;
      const wBottom = w.y + w.height;
      if (wBottom <= coord && wBottom > lo) lo = wBottom;
      if (wTop >= coord && wTop < hi) hi = wTop;
    } else {
      // Vertical corridor: wall must overlap the corridor's y-span.
      const wLo = w.y;
      const wHi = w.y + w.height;
      if (wHi < spanLo || wLo > spanHi) continue;
      const wLeft = w.x;
      const wRight = w.x + w.width;
      if (wRight <= coord && wRight > lo) lo = wRight;
      if (wLeft >= coord && wLeft < hi) hi = wLeft;
    }
  }
  return { lo, hi };
}

/**
 * Places `count` lanes, evenly spaced by `gap` and centred on `center`,
 * inside `[lo, hi]`. If the ideal spread does not fit, shrinks the spacing
 * uniformly until it does; if even zero spacing does not fit (the interval
 * is narrower than needed to hold `count` lanes at all, which only happens
 * when a wall is closer than the segment itself), returns null — the caller
 * then leaves those segments at their original coordinate rather than push
 * them past a wall.
 */
function layoutLanes(count: number, center: number, gap: number, lo: number, hi: number): number[] | null {
  if (count <= 1) {
    // Single occupant: nothing to spread, but still respect the interval by
    // clamping (keeps the "never past a wall" guarantee uniform).
    if (!Number.isFinite(lo) && !Number.isFinite(hi)) return [center];
    const clamped = Math.min(Number.isFinite(hi) ? hi : center, Math.max(Number.isFinite(lo) ? lo : center, center));
    return [clamped];
  }

  const idealSpan = gap * (count - 1);
  const available = Number.isFinite(hi) && Number.isFinite(lo) ? hi - lo : Infinity;
  if (Number.isFinite(available) && available < 0) return null; // walls overlap: no room at all

  const span = Number.isFinite(available) ? Math.min(idealSpan, available) : idealSpan;
  if (span < 0) return null;

  let start = center - span / 2;
  const end = start + span;
  // Slide the whole block inside [lo, hi] if the symmetric placement would
  // overhang either wall.
  if (Number.isFinite(hi) && end > hi) start -= end - hi;
  if (Number.isFinite(lo) && start < lo) start = lo;
  if (Number.isFinite(hi) && start + span > hi) {
    // Still doesn't fit (interval narrower than span, e.g. both walls close
    // in) — no valid placement; caller falls back to "leave as is".
    return null;
  }

  const step = count > 1 ? span / (count - 1) : 0;
  const lanes: number[] = [];
  for (let k = 0; k < count; k++) lanes.push(start + step * k);
  return lanes;
}

// ------------------------------------------------------------------ main

/**
 * Spreads out orthogonal routes whose interior segments overlap in the same
 * corridor, so parallel edges read as distinct lines instead of merging.
 *
 * Pure function: same input always produces the same output (see laneOrder
 * for the determinism argument), no shared state, no DOM/model access.
 */
export function nudgeRoutes(input: NudgeInput): Map<string, Point[]> {
  const { routes, walls, gap } = input;
  const safeGap = Number.isFinite(gap) && gap > 0 ? gap : 0;

  // Mutable working copies of every route's points — segments get rewritten
  // in place as corridors are resolved, and adjacent perpendicular segments
  // get their endpoints dragged along to keep each polyline connected.
  const working: MutablePoint[][] = routes.map((r) => r.points.map((p) => ({ ...p })));

  const segments = collectInteriorSegments(routes);
  if (segments.length > 0) {
    const tolerance = Math.max(safeGap, 1);
    const corridors = groupIntoCorridors(segments, tolerance);

    for (const group of corridors) {
      if (group.length === 0) continue;
      const ordered = laneOrder(segments, group);
      const first = segments[ordered[0] as number];
      if (!first) continue;
      const { axis, spanLo: groupSpanLoInit } = first;
      let spanLo = groupSpanLoInit;
      let spanHi = first.spanHi;
      let coordSum = 0;
      for (const idx of ordered) {
        const s = segments[idx];
        if (!s) continue;
        spanLo = Math.min(spanLo, s.spanLo);
        spanHi = Math.max(spanHi, s.spanHi);
        coordSum += s.coord;
      }
      const center = coordSum / ordered.length;

      const { lo, hi } = boundingInterval(axis, center, spanLo, spanHi, walls);
      const lanes = layoutLanes(ordered.length, center, safeGap, lo, hi);
      if (lanes === null) continue; // no room: leave this corridor's segments untouched

      ordered.forEach((segIdx, lane) => {
        const seg = segments[segIdx];
        const newCoord = lanes[lane];
        if (!seg || newCoord === undefined || !Number.isFinite(newCoord)) return;
        applySegmentCoord(working, seg, newCoord);
      });
    }
  }

  const result = new Map<string, Point[]>();
  routes.forEach((route, i) => {
    const pts = working[i] ?? [];
    // Final NaN guard: any non-finite point (should not occur given the
    // guards above, but the contract promises it categorically) collapses
    // to its neighbour so a broken corridor never turns into a broken path.
    const cleaned = pts.map((p, idx) => {
      if (Number.isFinite(p.x) && Number.isFinite(p.y)) return p;
      const prev = pts[idx - 1];
      return prev ? { ...prev } : { x: 0, y: 0 };
    });
    result.set(route.id, cleaned);
  });
  return result;
}

/**
 * Writes a corridor's new coordinate into one segment's two endpoints.
 *
 * No propagation to neighbouring segments is needed: in a valid orthogonal
 * polyline, consecutive segments always alternate axis (a horizontal run is
 * followed by a vertical one and vice versa), so the segment on either side
 * of the one being moved is perpendicular to it and shares the *other*
 * coordinate with its shared endpoint — the one this function does not
 * touch. Moving a horizontal segment's y, for instance, only changes the
 * length of the vertical segments joining it to its neighbours; their shared
 * x is untouched, so the polyline stays connected and orthogonal for free.
 * This is also why two segments of the same route are never assigned to the
 * same corridor in the first place (same-axis segments can't be adjacent).
 */
function applySegmentCoord(working: MutablePoint[][], seg: SegmentRef, newCoord: number): void {
  const pts = working[seg.routeIndex];
  if (!pts) return;
  const a = pts[seg.pointIndex];
  const b = pts[seg.pointIndex + 1];
  if (!a || !b) return;

  if (seg.axis === "y") {
    a.y = newCoord;
    b.y = newCoord;
  } else {
    a.x = newCoord;
    b.x = newCoord;
  }
}
