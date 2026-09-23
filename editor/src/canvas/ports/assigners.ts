import { facingSide } from "../../geometry/rect.js";
import type { BoundarySlot, Side } from "../../geometry/types.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";
import {
  compareRequests,
  portKey,
  slotAlongSide,
  type PortAssigner,
  type PortKey,
  type PortRequest,
} from "./PortAssigner.js";

/**
 * Every end sits in the middle of the side that faces the other element.
 *
 * This reproduces the original renderer exactly (R-REND-10) and is the default,
 * so that porting the library did not change a single pixel. Its weakness is
 * the one worth knowing: several edges between the same pair overlap perfectly.
 */
export class CenterPortAssigner implements PortAssigner {
  readonly id = "center";

  assign(requests: readonly PortRequest[]): Map<PortKey, BoundarySlot> {
    const out = new Map<PortKey, BoundarySlot>();
    for (const req of requests) {
      out.set(portKey(req.edgeId, req.end), {
        side: facingSide(req.ownerRect, req.otherRect),
        t: 0.5,
      });
    }
    return out;
  }
}

/**
 * Two-Phase Smart Port Assigner.
 *
 * 1. Straight lines (where two boxes share an overlapping straight span) are
 *    anchored first at their exact aligned perpendicular position.
 * 2. Each anchored straight line forms an exclusion halo (haloRadius) around itself.
 * 3. Curved and distant lines are then distributed evenly inside the remaining
 *    unoccupied intervals along the side, preventing collision or overlapping arrows.
 */
export class UniformPortAssigner implements PortAssigner {
  readonly id = "uniform";

  assign(requests: readonly PortRequest[]): Map<PortKey, BoundarySlot> {
    return distributeTwoPhaseSmart(requests);
  }
}

/**
 * Place ends on a fixed grid of slots along the side, Visio-style.
 *
 * Ends keep their spacing regardless of how many there are, centered within
 * shape-aware safe corner gap margins.
 */
export class DiscretePortAssigner implements PortAssigner {
  readonly id = "discrete";

  constructor(private readonly step = DIAGRAM_CONFIG.ports.discreteStep) {}

  assign(requests: readonly PortRequest[]): Map<PortKey, BoundarySlot> {
    return distributeGrid(requests, this.step);
  }
}

function distributeTwoPhaseSmart(requests: readonly PortRequest[]): Map<PortKey, BoundarySlot> {
  const groups = new Map<string, { side: Side; items: PortRequest[] }>();

  for (const req of requests) {
    const side = facingSide(req.ownerRect, req.otherRect);
    const key = `${req.ownerId}#${side}`;
    const group = groups.get(key);
    if (group === undefined) groups.set(key, { side, items: [req] });
    else group.items.push(req);
  }

  const out = new Map<PortKey, BoundarySlot>();
  const minGap = DIAGRAM_CONFIG.ports.minPortGap;
  const halo = DIAGRAM_CONFIG.ports.haloRadius;

  for (const { side, items } of groups.values()) {
    if (items.length === 0) continue;
    const first = items[0];
    if (first === undefined) continue;

    const horizontal = side === "north" || side === "south";
    const sideMin = horizontal ? first.ownerRect.x : first.ownerRect.y;
    const sideLength = horizontal ? first.ownerRect.width : first.ownerRect.height;
    const sideMax = sideMin + sideLength;
    const ownerInset = first.ownerInset ?? DIAGRAM_CONFIG.ports.defaultCornerInset;
    const safeMin = sideMin + ownerInset;
    const safeMax = sideMax - ownerInset;
    const safeSpan = Math.max(0, safeMax - safeMin);

    // Degenerate case or single edge
    if (items.length === 1 || safeSpan <= 0) {
      for (const req of items) {
        out.set(portKey(req.edgeId, req.end), { side, t: 0.5 });
      }
      continue;
    }

    // Phase 1: Separate into Straight Candidates vs Curved/Distant Candidates
    interface StraightCandidate {
      readonly req: PortRequest;
      readonly desiredPos: number;
      readonly lo: number;
      readonly hi: number;
    }
    interface CurvedCandidate {
      readonly req: PortRequest;
      readonly otherCenter: number;
    }

    const straightCandidates: StraightCandidate[] = [];
    const curvedCandidates: CurvedCandidate[] = [];

    for (const req of items) {
      const otherSide = facingSide(req.otherRect, req.ownerRect);
      const isOpposite =
        (side === "east" && otherSide === "west") ||
        (side === "west" && otherSide === "east") ||
        (side === "north" && otherSide === "south") ||
        (side === "south" && otherSide === "north");

      const otherInset = req.otherInset ?? DIAGRAM_CONFIG.ports.defaultCornerInset;
      const otherMin = (horizontal ? req.otherRect.x : req.otherRect.y) + otherInset;
      const otherMax = (horizontal ? req.otherRect.x + req.otherRect.width : req.otherRect.y + req.otherRect.height) - otherInset;
      const lo = Math.max(safeMin, otherMin);
      const hi = Math.min(safeMax, otherMax);
      const otherCenter = horizontal
        ? req.otherRect.x + req.otherRect.width / 2
        : req.otherRect.y + req.otherRect.height / 2;

      if (isOpposite && lo <= hi) {
        const desiredPos = Math.min(Math.max(otherCenter, lo), hi);
        straightCandidates.push({ req, desiredPos, lo, hi });
      } else {
        curvedCandidates.push({ req, otherCenter });
      }
    }

    // Sort straight candidates by target position
    straightCandidates.sort((a, b) => {
      if (Math.abs(a.desiredPos - b.desiredPos) > 0.01) return a.desiredPos - b.desiredPos;
      return compareRequests(a.req, b.req, side);
    });

    // Anchor straight ports, keeping min separation
    interface AnchoredPort {
      readonly req: PortRequest;
      pos: number;
    }
    const anchored: AnchoredPort[] = [];
    for (const cand of straightCandidates) {
      let pos = cand.desiredPos;
      const prev = anchored[anchored.length - 1];
      if (prev !== undefined && pos < prev.pos + minGap) {
        pos = Math.min(cand.hi, prev.pos + minGap);
      }
      anchored.push({ req: cand.req, pos });
    }

    // If there are no straight ports, distribute all curved ports evenly along safeSpan
    if (anchored.length === 0) {
      curvedCandidates.sort((a, b) => compareRequests(a.req, b.req, side));
      curvedCandidates.forEach((item, i) => {
        const pos = safeMin + ((i + 1) / (curvedCandidates.length + 1)) * safeSpan;
        const fraction = sideLength <= 0 ? 0.5 : (pos - sideMin) / sideLength;
        out.set(portKey(item.req.edgeId, item.req.end), { side, t: slotAlongSide(side, fraction) });
      });
      continue;
    }

    // Record straight ports into result
    for (const p of anchored) {
      const fraction = sideLength <= 0 ? 0.5 : (p.pos - sideMin) / sideLength;
      out.set(portKey(p.req.edgeId, p.req.end), { side, t: slotAlongSide(side, fraction) });
    }

    // If no curved candidates, we are done
    if (curvedCandidates.length === 0) {
      continue;
    }

    // Phase 2: Partition available free space into intervals with halo exclusion
    interface Interval {
      min: number;
      max: number;
      items: CurvedCandidate[];
    }
    const intervals: Interval[] = [];
    const m = anchored.length;
    const firstAnchored = anchored[0];
    const lastAnchored = anchored[m - 1];

    if (firstAnchored && lastAnchored) {
      // Interval 0 (before first straight port)
      intervals.push({
        min: safeMin,
        max: Math.max(safeMin, firstAnchored.pos - halo),
        items: [],
      });

      // Intermediate intervals (between straight ports)
      for (let j = 1; j < m; j++) {
        const prev = anchored[j - 1];
        const curr = anchored[j];
        if (prev && curr) {
          const iMin = prev.pos + halo;
          const iMax = curr.pos - halo;
          intervals.push({
            min: iMin,
            max: Math.max(iMin, iMax),
            items: [],
          });
        }
      }

      // Final interval (after last straight port)
      intervals.push({
        min: Math.min(safeMax, lastAnchored.pos + halo),
        max: safeMax,
        items: [],
      });
    }

    // Distribute each curved candidate into its closest/natural interval
    curvedCandidates.sort((a, b) => compareRequests(a.req, b.req, side));
    for (const cItem of curvedCandidates) {
      let bestIdx = 0;
      let bestDist = Infinity;
      for (let j = 0; j < intervals.length; j++) {
        const iv = intervals[j];
        if (iv === undefined) continue;
        const span = iv.max - iv.min;
        const iCenter = (iv.min + iv.max) / 2;
        // Penalize collapsed or tiny intervals
        const penalty = span < minGap ? 10000 : 0;
        const dist = Math.abs(cItem.otherCenter - iCenter) + penalty;
        if (dist < bestDist) {
          bestDist = dist;
          bestIdx = j;
        }
      }
      const targetInterval = intervals[bestIdx];
      if (targetInterval !== undefined) {
        targetInterval.items.push(cItem);
      }
    }

    // Distribute items evenly inside each interval
    for (const iv of intervals) {
      if (iv.items.length === 0) continue;
      const iSpan = iv.max - iv.min;
      if (iSpan <= 0) {
        const mid = (iv.min + iv.max) / 2;
        const fraction = sideLength <= 0 ? 0.5 : (mid - sideMin) / sideLength;
        for (const cItem of iv.items) {
          out.set(portKey(cItem.req.edgeId, cItem.req.end), { side, t: slotAlongSide(side, fraction) });
        }
      } else {
        iv.items.sort((a, b) => compareRequests(a.req, b.req, side));
        iv.items.forEach((cItem, idx) => {
          const pos = iv.min + ((idx + 1) / (iv.items.length + 1)) * iSpan;
          const fraction = sideLength <= 0 ? 0.5 : (pos - sideMin) / sideLength;
          out.set(portKey(cItem.req.edgeId, cItem.req.end), { side, t: slotAlongSide(side, fraction) });
        });
      }
    }
  }

  return out;
}

function distributeGrid(
  requests: readonly PortRequest[],
  step: number,
): Map<PortKey, BoundarySlot> {
  const groups = new Map<string, { side: Side; items: PortRequest[] }>();

  for (const req of requests) {
    const side = facingSide(req.ownerRect, req.otherRect);
    const key = `${req.ownerId}#${side}`;
    const group = groups.get(key);
    if (group === undefined) groups.set(key, { side, items: [req] });
    else group.items.push(req);
  }

  const out = new Map<PortKey, BoundarySlot>();
  for (const { side, items } of groups.values()) {
    items.sort((a, b) => compareRequests(a, b, side));
    const first = items[0];
    if (first === undefined) continue;

    const horizontal = side === "north" || side === "south";
    const sideLength = horizontal ? first.ownerRect.width : first.ownerRect.height;
    const cornerGap = first.ownerInset ?? DIAGRAM_CONFIG.ports.defaultCornerInset;

    const usableSpan = Math.max(0, sideLength - 2 * cornerGap);
    const span = Math.min(step * (items.length - 1), usableSpan);
    const gap = items.length > 1 ? span / (items.length - 1) : 0;
    const start = cornerGap + (usableSpan - span) / 2;

    items.forEach((req, i) => {
      const pos = start + gap * i;
      const fraction = sideLength <= 0 ? 0.5 : pos / sideLength;
      out.set(portKey(req.edgeId, req.end), { side, t: slotAlongSide(side, fraction) });
    });
  }
  return out;
}
