import { GRAPH_TUNING } from "../app/graph/tuning.js";
import type { GraphClipboard,GraphClipboardNode } from "../model/graphClipboard.js";
import { isContainer, type DiagramElement } from "../model/types.js";
import { canvas } from "../constants/canvas.js";
import type { DiagramEditor } from "./DiagramEditor.js";
import { blockFor, boxMetrics, drawRelations } from "./placeEntity.js";

const MAX_EXTENT = GRAPH_TUNING.pasteMaxExtent;
/** Read at use time: canvas.json is loaded after this module is evaluated. */
const containerPad = (): number => canvas().zone.padding;
const containerHeader = (): number => canvas().zone.headerHeight;

export interface PasteResult {
  /** Element ids put on the view. */
  placed: string[];
  /** Nodes the registry has no entity for (code-only, or an entity it does not know): a reconcile is needed. */
  notInModel: number;
  /** Entities that are on the view already (one box per entity: not placed twice). */
  alreadyOnView: number;
}

interface Point {
  x: number;
  y: number;
}

const snap = (v: number): number => Math.round(v / canvas().grid) * canvas().grid;

/**
 * The graph's arrangement of these nodes, scaled up until no two blocks touch
 * and snapped to the grid, as offsets from the top-left corner. The graph's y
 * points up, the editor's down, so the picture is kept, not mirrored. When the
 * arrangement cannot be kept (points on top of each other, or it would sprawl
 * across thousands of pixels) the nodes go in a tidy grid instead.
 */
function arrange(nodes: readonly GraphClipboardNode[]): Point[] {
  const { WIDTH, HEIGHT, GAP_X, GAP_Y } = boxMetrics();
  const pts = nodes.map((n) => ({ x: n.x, y: -n.y }));
  const grid = (): Point[] => {
    const perRow = Math.max(1, Math.ceil(Math.sqrt(nodes.length * GRAPH_TUNING.pasteGridAspect)));
    return [...nodes.keys()].map((i) => ({ x: (i % perRow) * (WIDTH + GAP_X), y: Math.floor(i / perRow) * (HEIGHT + GAP_Y) }));
  };
  if (pts.length === 1) return [{ x: 0, y: 0 }];
  if (pts.length > GRAPH_TUNING.pasteMaxArranged) return grid();

  // The smallest scale at which every pair is apart by a block plus a gap, sideways or vertically.
  let scale = 0;
  for (let i = 0; i < pts.length; i++) {
    for (let j = i + 1; j < pts.length; j++) {
      const dx = Math.abs(pts[i]!.x - pts[j]!.x);
      const dy = Math.abs(pts[i]!.y - pts[j]!.y);
      if (dx < 1e-9 && dy < 1e-9) return grid();
      const need = Math.min(dx < 1e-9 ? Infinity : (WIDTH + GAP_X) / dx, dy < 1e-9 ? Infinity : (HEIGHT + GAP_Y) / dy);
      scale = Math.max(scale, need);
    }
  }
  const minX = Math.min(...pts.map((p) => p.x));
  const minY = Math.min(...pts.map((p) => p.y));
  const out = pts.map((p) => ({ x: snap((p.x - minX) * scale), y: snap((p.y - minY) * scale) }));
  const width = Math.max(...out.map((p) => p.x)) + WIDTH;
  const height = Math.max(...out.map((p) => p.y)) + HEIGHT;
  return width > MAX_EXTENT || height > MAX_EXTENT ? grid() : out;
}

/** Widen `container` (and, above it, whatever holds it) until it holds `rect` with some room. */
function growToFit(container: DiagramElement, rect: { x: number; y: number; width: number; height: number }): void {
  const pad = containerPad();
  let target = { x: rect.x - pad, y: rect.y - pad, right: rect.x + rect.width + pad, bottom: rect.y + rect.height + pad };
  for (let el: DiagramElement | null = container; el !== null; el = el.parent) {
    const x = Math.min(el.x, target.x);
    const y = Math.min(el.y, target.y);
    const right = Math.max(el.x + el.width, target.right);
    const bottom = Math.max(el.y + el.height, target.bottom);
    el.x = x;
    el.y = y;
    el.width = right - x;
    el.height = bottom - y;
    target = { x: el.x - pad, y: el.y - pad, right: el.x + el.width + pad, bottom: el.y + el.height + pad };
  }
}

/**
 * Paste nodes copied from the graph onto the open view, as one undo step.
 *
 * Only nodes with a registry entity are placed — the registry is never
 * written; the rest are counted for the caller to say so. With a zone (or a
 * box inside one) selected the boxes go inside it and it grows to hold them;
 * otherwise they go onto free space beside the existing content. Relations are
 * only ever the model's own: the ones between the pasted entities of the edge
 * kinds that were drawn on the graph are put on the view (which is what the
 * view's edge list is for, CONTRACT.md §8.5), whatever their default said.
 */
export function pasteGraphNodes(editor: DiagramEditor, payload: GraphClipboard): PasteResult {
  const doc = editor.canvas.model;
  const result: PasteResult = { placed: [], notInModel: 0, alreadyOnView: 0 };
  if (!doc) return result;
  const { WIDTH, HEIGHT, GAP_X, GAP_Y } = boxMetrics();

  const entities = new Map(doc.entities.map((e) => [e.id, e]));
  const fresh: { node: GraphClipboardNode; entityId: string }[] = [];
  const seen = new Set<string>();
  for (const node of payload.nodes) {
    if (!node.entity || !entities.has(node.entity)) {
      result.notInModel++;
      continue;
    }
    if (doc.element(node.entity) !== undefined || seen.has(node.entity)) {
      result.alreadyOnView++;
      continue;
    }
    seen.add(node.entity);
    fresh.push({ node, entityId: node.entity });
  }
  if (fresh.length === 0) return result;

  const spots = arrange(fresh.map((f) => f.node));
  const extent = {
    width: Math.max(...spots.map((p) => p.x)) + WIDTH,
    height: Math.max(...spots.map((p) => p.y)) + HEIGHT,
  };

  // Where: inside the selected zone, else beside everything already there.
  const selected = editor.canvas.selected;
  const picked = selected && selected.kind !== "edge" ? doc.element(selected.id) : undefined;
  const container = picked === undefined ? null : isContainer(picked) ? picked : picked.parent;
  let origin: Point;
  if (container !== null) {
    const below = container.children.length > 0 ? Math.max(...container.children.map((c) => c.y + c.height)) + GAP_Y : container.y + containerHeader();
    origin = { x: container.x + containerPad(), y: below };
  } else {
    const bounds = doc.bounds();
    const centre = editor.canvas.viewCenter();
    origin = bounds ? { x: snap(bounds.x + bounds.width + 2 * GAP_X), y: snap(bounds.y) } : { x: snap(centre.x - extent.width / 2), y: snap(centre.y - extent.height / 2) };
  }

  fresh.forEach(({ entityId }, i) => {
    const spot = spots[i]!;
    const x = origin.x + spot.x;
    const y = origin.y + spot.y;
    doc.add(blockFor(doc, entities.get(entityId)!, x, y), container ?? doc.containerAt({ x: x + WIDTH / 2, y: y + HEIGHT / 2 }));
    result.placed.push(entityId);
  });
  if (container !== null) growToFit(container, { x: origin.x, y: origin.y, width: extent.width, height: extent.height });

  // The model's relations: the usual ones to what is on the view, and every one between the
  // pasted entities of a kind that was drawn on the graph.
  drawRelations(editor, result.placed);
  const drawn = new Set(payload.edgeKinds);
  const placedSet = new Set(result.placed);
  const have = new Set(doc.edges.map((e) => e.id));
  for (const r of doc.relations) {
    if (!r || have.has(r.id) || r.from === r.to) continue;
    if (!placedSet.has(r.from) || !placedSet.has(r.to)) continue;
    if (!drawn.has((r.type ?? "").split(".")[0] ?? "")) continue;
    doc.addEdge({
      id: r.id,
      from: r.from,
      to: r.to,
      type: r.type,
      label: "",
      ...(r.origin === undefined ? {} : { origin: r.origin }),
    } as any);
    have.add(r.id);
  }

  (editor as any).commit("paste-graph");
  editor.canvas.selectMany(result.placed);
  return result;
}
