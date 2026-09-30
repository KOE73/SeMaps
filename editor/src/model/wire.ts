import { DiagramDocument } from "./document.js";
import type { StyleLibrary } from "./StyleLibrary.js";
import type {
  DiagramEdge,
  DiagramElement,
  DiagramMetadata,
  DiagramView,
} from "./types.js";
import type {
  WireDocument,
  WireEdge,
  WirePlacement,
  WireView,
} from "./wire-types.js";
import { EDGE_OVERRIDE_FIELDS, OVERRIDE_FIELDS, serializeOverride } from "./override.js";

/**
 * The only place that knows the editor's document shape.
 *
 * Everything above works with `DiagramDocument`. The loader (`ProjectStore`)
 * turns a view file into a `WireDocument` — one array of placements, each
 * knowing whether it is a container and which container it lies in — and this
 * file turns that into the element tree and back.
 */

const rawViews = new WeakMap<DiagramView, WireView>();

// ------------------------------------------------------------------ parsing

/**
 * @param _styles Kept for callers that pass the live library; nothing is
 *   migrated into it any more.
 */
export function parseDocument(wire: WireDocument, _styles?: StyleLibrary): DiagramDocument {
  // A document of an older contract had two arrays; it is named, not read (ADR_20260927-3).
  for (const key of ["zones", "nodes"] as const) {
    if (key in (wire as Record<string, unknown>)) {
      throw new Error(
        `Документ в старой форме: ключ «${key}». Вид контракта 5 — один массив «placements» ` +
          `(CONTRACT.md §8.2, ADR_20260927-6).`,
      );
    }
  }

  const elements = (wire.placements ?? []).map((p, i) => placementToElement(p, i));
  linkParents(elements, wire.placements ?? []);

  return new DiagramDocument({
    metadata: parseMetadata(wire),
    views: (wire.views ?? []).map(parseView),
    roots: elements.filter((el) => el.parent === null),
    edges: (wire.edges ?? []).map(parseEdge),
    raw: wire as unknown as Record<string, unknown>,
  });
}

function parseMetadata(wire: WireDocument): DiagramMetadata {
  const m = wire.metadata ?? {};
  return { ...m, title: m.title ?? "Схема без названия" };
}

function parseView(w: WireView): DiagramView {
  const view: DiagramView = {
    id: w.id,
    name: w.name ?? w.id,
    icon: w.icon ?? "",
    description: w.description ?? "",
    highlightZones: w.highlightZones ?? [],
    highlightNodes: w.highlightNodes ?? [],
  };
  rawViews.set(view, w);
  return view;
}

function parseEdge(w: WireEdge): DiagramEdge {
  return {
    id: w.id,
    from: w.from,
    to: w.to,
    label: w.label ?? "",
    // A code-origin edge carries no text at all (ADR_20260831 §2.13); even if
    // a stray value were present on the wire it is not read into the model,
    // so nothing downstream can offer to edit or render it.
    ...(w.origin === "code" || w.fromLabel === undefined ? {} : { fromLabel: w.fromLabel }),
    ...(w.origin === "code" || w.toLabel === undefined ? {} : { toLabel: w.toLabel }),
    type: w.type ?? "call",
    ...(w.styleId === undefined ? {} : { styleId: w.styleId }),
    ...(w.override === undefined ? {} : { override: w.override }),
    ...(w.origin === undefined ? {} : { origin: w.origin }),
    // The *choice* of line shape travels; the polyline it produces never does
    // (ADR_20260903 §2.7).
    ...(w.routing === undefined ? {} : { routing: w.routing }),
  };
}

function placementToElement(p: WirePlacement, order: number): DiagramElement {
  return {
    id: p.id,
    kind: p.container ? "zone" : "node",
    type: p.type ?? "",
    label: p.label ?? p.id,
    tags: p.tags ?? [],
    metadata: p.metadata ?? {},
    ...(p.styleId === undefined ? {} : { styleId: p.styleId }),
    ...(p.override === undefined ? {} : { override: p.override }),
    x: p.x,
    y: p.y,
    width: p.width,
    height: p.height,
    parent: null,
    children: [],
    wireOrder: order,
    raw: p as unknown as Record<string, unknown>,
  };
}

/**
 * Nest placements by their declared `parent` (CONTRACT.md §8.2): nesting is
 * written down, never read from geometry. A parent that is not a container
 * placement on this view, or that would close a cycle, is not followed — the
 * placement stays at the top, where it is at least visible.
 */
function linkParents(elements: DiagramElement[], wire: readonly WirePlacement[]): void {
  const byId = new Map(elements.map((el) => [el.id, el]));
  elements.forEach((el, i) => {
    const declared = wire[i]?.parent;
    if (declared === undefined || declared === null) return;
    const parent = byId.get(declared);
    if (parent === undefined || parent.kind !== "zone" || parent === el) return;
    for (let p: DiagramElement | null = parent; p !== null; p = p.parent) if (p === el) return;
    el.parent = parent;
    parent.children.push(el);
  });
}

// --------------------------------------------------------------- serializing

export function serializeDocument(doc: DiagramDocument): WireDocument {
  const placements = [...doc.elements()]
    .map((el) => ({ order: el.wireOrder, value: elementToPlacement(el) }))
    .sort((a, b) => a.order - b.order)
    .map((p) => p.value);

  const { placements: _drop, ...rest } = doc.raw as WireDocument;
  return {
    // Unknown top-level keys ($schema, version, the loader's bundle) ride along.
    ...rest,
    metadata: { ...doc.metadata },
    views: doc.views.map(serializeView),
    placements,
    edges: doc.edges.map(serializeEdge),
  };
}

function serializeView(view: DiagramView): WireView {
  const raw = rawViews.get(view);
  const out: WireView = { ...(raw ?? {}), id: view.id };
  if (raw?.name !== undefined || view.name !== view.id) out.name = view.name;
  if (raw?.icon !== undefined) out.icon = view.icon;
  if (raw?.description !== undefined) out.description = view.description;
  if (view.highlightZones.length > 0) out.highlightZones = view.highlightZones;
  if (view.highlightNodes.length > 0) out.highlightNodes = view.highlightNodes;
  return out;
}

function serializeEdge(edge: DiagramEdge): WireEdge {
  const out: WireEdge = { id: edge.id, from: edge.from, to: edge.to };
  if (edge.label !== "") out.label = edge.label;
  // Mirrors parseEdge's rule: a code-origin edge never re-acquires text on
  // save even if something upstream slipped a value onto the in-memory edge.
  if (edge.origin !== "code") {
    if (edge.fromLabel !== undefined && edge.fromLabel !== "") out.fromLabel = edge.fromLabel;
    if (edge.toLabel !== undefined && edge.toLabel !== "") out.toLabel = edge.toLabel;
  }
  out.type = edge.type;
  if (edge.styleId !== undefined) out.styleId = edge.styleId;
  const override = serializeOverride(EDGE_OVERRIDE_FIELDS, edge.override);
  if (override !== undefined) out.override = override;
  if (edge.origin !== undefined) out.origin = edge.origin;
  if (edge.routing !== undefined) out.routing = edge.routing;
  return out;
}

function elementToPlacement(el: DiagramElement): WirePlacement {
  const raw = (el.raw ?? {}) as Partial<WirePlacement>;
  const out: WirePlacement = {
    ...raw,
    id: el.id,
    container: el.kind === "zone",
    label: el.label,
    type: el.type,
    parent: el.parent !== null && el.parent.kind === "zone" ? el.parent.id : null,
    x: round(el.x),
    y: round(el.y),
    width: round(el.width),
    height: round(el.height),
  };
  if (el.tags.length > 0) out.tags = el.tags;
  else delete out.tags;
  if (el.styleId !== undefined) out.styleId = el.styleId;
  else delete out.styleId;
  const override = serializeOverride(OVERRIDE_FIELDS, el.override);
  if (override !== undefined) out.override = override;
  else delete out.override;
  // An element that never carried metadata does not acquire an empty object
  // just by being opened.
  if (Object.keys(el.metadata).length > 0 || "metadata" in raw) out.metadata = el.metadata;
  else delete out.metadata;
  return out;
}

function round(n: number): number {
  return Math.round(n * 1000) / 1000;
}
