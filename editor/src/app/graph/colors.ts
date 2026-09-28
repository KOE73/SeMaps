import type { GraphEdge, GraphNode } from "./types.js";

/**
 * Colour and legend logic for the graph page. Presence (`code`/`model`/
 * `both`) is always visible, independent of the chosen colouring attribute
 * (PLAN_20260928-2 step 3): `code` is always grey, `model` is the
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
      return n.kind ?? "—";
  }
}

function lighten(hex: string, amount: number): string {
  const n = parseInt(hex.slice(1), 16);
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  const mix = (c: number) => Math.round(c + (255 - c) * amount);
  return `rgb(${mix(r)}, ${mix(g)}, ${mix(b)})`;
}

/** Final fill colour for a node: attribute colour, styled by presence. */
export function nodeColor(n: GraphNode, by: ColorBy): string {
  const base = colorFor(colorAttribute(n, by));
  if (n.presence === "code") return PRESENCE_CODE_COLOR;
  if (n.presence === "model") return lighten(base, 0.55);
  return base;
}

const EDGE_PALETTE: Record<string, string> = {
  extends: "#ff7a59",
  implements: "#a35bff",
  contains: "#8a8f98",
  depends: "#4f8cff",
  holds: "#33c37a",
  uses: "#f2b705",
};

export function edgeColor(e: GraphEdge): string {
  return EDGE_PALETTE[e.kind] ?? colorFor(e.kind);
}

/** Thicker for `many`/`keyed` cardinality; deferred edges get a lighter tint
 * since sigma's edge programs used here (line) draw solid strokes only —
 * dashing needs a custom edge program, so "lighter colour" stands in for a
 * dash (said in the report). */
export function edgeSize(e: GraphEdge): number {
  const card = e.via?.cardinality;
  return card === "many" || card === "keyed" ? 3 : 1.4;
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
