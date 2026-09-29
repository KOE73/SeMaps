import type { DiagramCanvas } from "../../canvas/DiagramCanvas.js";
import { ELEMENT_ATTR } from "../../interaction/roles.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";
import type { Point, Rect } from "../../geometry/types.js";
import type { DiagramElement } from "../../model/types.js";
import { isContainer } from "../../model/types.js";

export type ProblemKind =
  | "overlap"
  | "clipped-caption"
  | "clipped-rows"
  | "line-through-box"
  | "line-crossing"
  | "outside-zone";

export interface Problem {
  kind: ProblemKind;
  ids: string[];
  text: string;
}

/** At most this many entries per kind; the rest become one "+N more" entry. */
const CAP = 20;
/** Slack, in model units, so a box touching its neighbour or a zone edge is not a finding. */
const EPS = 0.5;

/**
 * What is wrong with the picture the open view draws now.
 *
 * Nothing here is estimated a second time: the boxes are the rectangles the
 * renderers draw, the lines are the routes the canvas drew (its real router,
 * separated and re-laid), and text is measured on the drawn `<text>` elements,
 * which are already cut with "…" by the box renderer's own eliding.
 */
export function findProblems(canvas: DiagramCanvas): Problem[] {
  const boxes = canvas.shownBoxes();
  const found = new Map<ProblemKind, Problem[]>();
  const add = (p: Problem): void => {
    const list = found.get(p.kind) ?? [];
    list.push(p);
    found.set(p.kind, list);
  };

  overlaps(boxes, add);
  outsideZone(boxes, add);
  text(canvas, boxes, add);
  lines(canvas, boxes, add);

  const out: Problem[] = [];
  for (const [kind, list] of found) {
    out.push(...list.slice(0, CAP));
    if (list.length > CAP) {
      out.push({ kind, ids: [], text: `+${list.length - CAP} more (${list.length} in all)` });
    }
  }
  return out;
}

type Box = { el: DiagramElement; rect: Rect };

const inside = (outer: Rect, inner: Rect): boolean =>
  inner.x >= outer.x - EPS && inner.y >= outer.y - EPS &&
  inner.x + inner.width <= outer.x + outer.width + EPS &&
  inner.y + inner.height <= outer.y + outer.height + EPS;

function isAncestor(a: DiagramElement, b: DiagramElement): boolean {
  for (let p = b.parent; p !== null; p = p.parent) if (p === a) return true;
  return false;
}

/** Two shown elements whose interiors meet, neither holding the other. */
function overlaps(boxes: readonly Box[], add: (p: Problem) => void): void {
  const sorted = [...boxes].sort((a, b) => a.rect.x - b.rect.x);
  for (let i = 0; i < sorted.length; i++) {
    const a = sorted[i]!;
    for (let j = i + 1; j < sorted.length; j++) {
      const b = sorted[j]!;
      if (b.rect.x >= a.rect.x + a.rect.width - EPS) break;
      if (isAncestor(a.el, b.el) || isAncestor(b.el, a.el)) continue;
      const w = Math.min(a.rect.x + a.rect.width, b.rect.x + b.rect.width) - Math.max(a.rect.x, b.rect.x);
      const h = Math.min(a.rect.y + a.rect.height, b.rect.y + b.rect.height) - Math.max(a.rect.y, b.rect.y);
      if (w > EPS && h > EPS) {
        add({ kind: "overlap", ids: [a.el.id, b.el.id], text: `${a.el.id} and ${b.el.id} overlap by ${Math.round(w)}x${Math.round(h)}` });
      }
    }
  }
}

/** A node that names a zone (or sits in one) but is not wholly inside that zone's rectangle. */
function outsideZone(boxes: readonly Box[], add: (p: Problem) => void): void {
  const rects = new Map(boxes.map((b) => [b.el.id, b.rect]));
  for (const { el, rect } of boxes) {
    if (isContainer(el) || el.parent === null) continue;
    if (el.origin !== undefined && !el.origin.zoneDeclared) continue;
    const zone = rects.get(el.parent.id);
    if (zone !== undefined && !inside(zone, rect)) {
      add({ kind: "outside-zone", ids: [el.id, el.parent.id], text: `${el.id} is not inside its zone ${el.parent.id}` });
    }
  }
}

/** Captions and member rows, from the drawn text: measured, cut with "…", or below the box. */
function text(canvas: DiagramCanvas, boxes: readonly Box[], add: (p: Problem) => void): void {
  const root = canvas.svgElement;
  for (const { el, rect } of boxes) {
    const g = root.querySelector(`[${ELEMENT_ATTR}="${CSS.escape(el.id)}"]`);
    if (g === null) continue;
    const zone = isContainer(el);
    const available = zone
      ? rect.width - DIAGRAM_CONFIG.container.titlePad - DIAGRAM_CONFIG.node.padX
      : rect.width - DIAGRAM_CONFIG.node.padX * 2;

    let cutCaption: string | null = null;
    let cutRows = 0;
    let lowRows = 0;
    for (const t of g.querySelectorAll<SVGTextElement>("text")) {
      const cls = t.getAttribute("class") ?? "";
      const isCaption = cls.includes("semaps-node-label") || cls.includes("semaps-zone-title");
      const isRow = cls.includes("semaps-node-member") || cls.includes("semaps-node-subtitle") || cls.includes("semaps-node-members-collapsed");
      if (!isCaption && !isRow) continue;
      const box = t.getBBox();
      if (box.width === 0 && box.height === 0) continue; // not laid out: nothing to measure
      const shown = t.textContent ?? "";
      const elided = shown.endsWith("…") && shown !== el.label;

      if (isCaption) {
        if (elided || t.getComputedTextLength() > available + EPS) cutCaption = el.label;
      } else if (elided) {
        cutRows++;
      }
      if (!zone && box.y + box.height > rect.y + rect.height + EPS) lowRows++;
    }
    if (cutCaption !== null) {
      add({ kind: "clipped-caption", ids: [el.id], text: `caption of ${el.id} does not fit its ${Math.round(available)}-wide box` });
    }
    if (cutRows > 0 || lowRows > 0) {
      const parts: string[] = [];
      if (cutRows > 0) parts.push(`${cutRows} cut with …`);
      if (lowRows > 0) parts.push(`${lowRows} extend past the bottom`);
      add({ kind: "clipped-rows", ids: [el.id], text: `rows of ${el.id}: ${parts.join(", ")}` });
    }
  }
}

/** Lines through boxes they do not join, and lines crossing lines. */
function lines(canvas: DiagramCanvas, boxes: readonly Box[], add: (p: Problem) => void): void {
  const routed = canvas.routedLines().filter((l) => l.points.length >= 2);
  const nodes = boxes.filter((b) => !isContainer(b.el));

  for (const line of routed) {
    for (const { el, rect } of nodes) {
      if (el.id === line.from || el.id === line.to) continue;
      if (crossesInterior(line.points, rect)) {
        add({ kind: "line-through-box", ids: [line.id, el.id], text: `line ${line.id} runs through ${el.id}` });
      }
    }
  }

  const spans = routed.map((l) => bounds(l.points));
  for (let i = 0; i < routed.length; i++) {
    for (let j = i + 1; j < routed.length; j++) {
      const a = routed[i]!;
      const b = routed[j]!;
      if (!meet(spans[i]!, spans[j]!)) continue;
      if (linesCross(a.points, b.points)) {
        add({ kind: "line-crossing", ids: [a.id, b.id], text: `lines ${a.id} and ${b.id} cross` });
      }
    }
  }
}

function bounds(points: readonly Point[]): Rect {
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const p of points) {
    x0 = Math.min(x0, p.x); y0 = Math.min(y0, p.y);
    x1 = Math.max(x1, p.x); y1 = Math.max(y1, p.y);
  }
  return { x: x0, y: y0, width: x1 - x0, height: y1 - y0 };
}

const meet = (a: Rect, b: Rect): boolean =>
  a.x <= b.x + b.width && b.x <= a.x + a.width && a.y <= b.y + b.height && b.y <= a.y + a.height;

/** Does any segment pass through the inside of `rect` (shrunk a little, so grazing an edge is fine)? */
function crossesInterior(points: readonly Point[], rect: Rect): boolean {
  const m = 1;
  const x0 = rect.x + m, y0 = rect.y + m, x1 = rect.x + rect.width - m, y1 = rect.y + rect.height - m;
  if (x1 <= x0 || y1 <= y0) return false;
  for (let i = 0; i < points.length - 1; i++) {
    const p = points[i]!, q = points[i + 1]!;
    // Liang-Barsky clip of p-q against the rectangle.
    let t0 = 0, t1 = 1;
    const dx = q.x - p.x, dy = q.y - p.y;
    const edges: [number, number][] = [[-dx, p.x - x0], [dx, x1 - p.x], [-dy, p.y - y0], [dy, y1 - p.y]];
    let hit = true;
    for (const [d, dist] of edges) {
      if (d === 0) {
        if (dist < 0) { hit = false; break; }
      } else {
        const r = dist / d;
        if (d < 0) { if (r > t1) { hit = false; break; } t0 = Math.max(t0, r); }
        else { if (r < t0) { hit = false; break; } t1 = Math.min(t1, r); }
      }
    }
    if (hit && t1 - t0 > 0) {
      const len = Math.hypot(dx, dy) * (t1 - t0);
      if (len > 2) return true;
    }
  }
  return false;
}

/** Do two polylines cross, away from the places where they start and end? */
function linesCross(a: readonly Point[], b: readonly Point[]): boolean {
  const ends = [a[0]!, a[a.length - 1]!, b[0]!, b[b.length - 1]!];
  const nearEnd = (x: number, y: number): boolean => ends.some((e) => Math.hypot(e.x - x, e.y - y) < 1.5);
  for (let i = 0; i < a.length - 1; i++) {
    const p = a[i]!, q = a[i + 1]!;
    for (let j = 0; j < b.length - 1; j++) {
      const r = b[j]!, s = b[j + 1]!;
      const d = (q.x - p.x) * (s.y - r.y) - (q.y - p.y) * (s.x - r.x);
      if (Math.abs(d) < 1e-9) continue; // parallel: running side by side is not a crossing
      const t = ((r.x - p.x) * (s.y - r.y) - (r.y - p.y) * (s.x - r.x)) / d;
      const u = ((r.x - p.x) * (q.y - p.y) - (r.y - p.y) * (q.x - p.x)) / d;
      // Half-open, so a crossing at a shared vertex of a sampled curve is counted once.
      if (t >= 0 && t < 1 && u >= 0 && u < 1 && !nearEnd(p.x + t * (q.x - p.x), p.y + t * (q.y - p.y))) return true;
    }
  }
  return false;
}
