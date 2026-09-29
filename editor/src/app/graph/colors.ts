import type { GraphEdge, GraphNode } from "./types.js";

/**
 * Colour and legend logic for the graph page. Presence (`code`/`model`/
 * `both`) is always visible, independent of the chosen colouring attribute
 * (PLAN_20260928-2 step 3): `code` is the attribute's colour dulled towards
 * grey (plain grey when colouring by presence), `model` is the
 * attribute's colour lightened — sigma's built-in node programs
 * (`node-circle`, `node-point`) have no outline-only mode, so an outline is
 * not possible without a custom WebGL program; a lighter tint plus a "◇"
 * label marker stands in for it (said in the report).
 */

export type ColorBy = "kind" | "container" | "namespace" | "presence";

export const PRESENCE_CODE_COLOR = "#8a8f98";
export const MODEL_MARKER = "◇ ";

const PALETTE = [
  "#4f8cff", "#ff7a59", "#33c37a", "#f2b705", "#a35bff", "#ff5da2",
  "#20c4c4", "#e0563b", "#7d9a3b", "#5c6bc0", "#c2185b", "#00897b",
];

const cache = new Map<string, string>();

/** A stable colour per distinct attribute value, in first-seen order. */
export function colorFor(value: string): string {
  let c = cache.get(value);
  if (c) return c;
  c = PALETTE[cache.size % PALETTE.length] ?? "#999999";
  cache.set(value, c);
  return c;
}

export function resetPalette(): void {
  cache.clear();
}

/** The attribute value a node is coloured by, before presence is applied. */
export function colorAttribute(n: GraphNode, by: ColorBy): string {
  switch (by) {
    case "container":
      return n.containers?.[0] ?? "—";
    case "namespace":
      return n.namespace ?? "—";
    case "presence":
      return n.presence as string;
    case "kind":
    default:
      // The base kind (`class` for `abstract-class`): a modifier is a look, not a colour.
      return n.symbolKind ?? n.nativeKind ?? n.kind ?? "—";
  }
}

/** The attribute's own colour (hex), before presence or modifiers style it. */
export function baseColor(n: GraphNode, by: ColorBy): string {
  return colorFor(colorAttribute(n, by));
}

export function lighten(hex: string, amount: number): string {
  const n = parseInt(hex.slice(1), 16);
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  const mix = (c: number) => Math.round(c + (255 - c) * amount);
  return `rgb(${mix(r)}, ${mix(g)}, ${mix(b)})`;
}

function towards(hex: string, target: string, amount: number): string {
  const a = parseInt(hex.slice(1), 16), b = parseInt(target.slice(1), 16);
  const mix = (shift: number) => Math.round(((a >> shift) & 255) * (1 - amount) + ((b >> shift) & 255) * amount);
  return `rgb(${mix(16)}, ${mix(8)}, ${mix(0)})`;
}

/** Final fill colour for a node: attribute colour, styled by presence. */
export function nodeColor(n: GraphNode, by: ColorBy): string {
  const base = colorFor(colorAttribute(n, by));
  if (by === "presence") return n.presence === "code" ? PRESENCE_CODE_COLOR : base;
  // A project that was never synced has code-only nodes alone: plain grey
  // would leave it without any colouring, so the attribute's colour stays,
  // dulled towards the grey.
  if (n.presence === "code") return towards(base, PRESENCE_CODE_COLOR, 0.45);
  if (n.presence === "model") return lighten(base, 0.55);
  return base;
}

// `contains` is the majority of edges (most nodes sit inside something) and
// the original grey (#8a8f98) all but disappeared against the dark canvas
// background (#1c1e22, graph.css) once ~800 of them overlapped — checked by
// looking at the demo graph before and after. Lightened towards the label
// colour so it reads as "structure" without competing with the brighter,
// rarer relation kinds below.
const EDGE_PALETTE: Record<string, string> = {
  extends: "#ff7a59",
  implements: "#a35bff",
  contains: "#aeb4bf",
  depends: "#4f8cff",
  holds: "#33c37a",
  uses: "#f2b705",
  calls: "#ff5da2",
  constructs: "#20c4c4",
  overrides: "#c2185b",
};

export function edgeColor(e: GraphEdge): string {
  return EDGE_PALETTE[e.kind] ?? colorFor(e.kind);
}

/** Thicker for `many`/`keyed` cardinality; deferred edges get a lighter tint
 * since sigma's edge programs used here (line) draw solid strokes only —
 * dashing needs a custom edge program, so "lighter colour" stands in for a
 * dash (said in the report). Base size raised from 1.4 to 2: at 1.4 the line
 * program (sigma's default `edge` renderer draws screen-pixel-sized lines,
 * not zoom-scaled) was a near-invisible hairline against the node clutter at
 * overview zoom on the demo graph; 2 was the smallest that stayed clearly
 * visible without the graph looking like it was drawn in marker. */
export function edgeSize(e: Pick<GraphEdge, "via" | "count">): number {
  // A lifted calls/constructs edge grows with how many method calls it folds.
  if (e.count && e.count > 1) return 2 + Math.min(6, Math.log2(e.count) * 1.6);
  const card = e.via?.cardinality;
  return card === "many" || card === "keyed" ? 3.6 : 2;
}

export function edgeRenderColor(e: GraphEdge): string {
  const base = edgeColor(e);
  return e.via?.deferred ? lighten(base, 0.5) : base;
}

export interface LegendEntry {
  swatch: string;
  label: string;
}

export function legendFor(nodes: readonly GraphNode[], by: ColorBy): LegendEntry[] {
  const seen = new Map<string, string>();
  for (const n of nodes) {
    const v = colorAttribute(n, by);
    if (!seen.has(v)) seen.set(v, colorFor(v));
  }
  return [...seen.entries()].map(([label, swatch]) => ({ label, swatch }));
}
