/**
 * Every visual tuning number of the graph mode and of pasting from it, in one place.
 * They are taste, not data: change them here, nothing else has to follow.
 */
export const GRAPH_TUNING = {
  /** Groups the legend lists when colouring by group; the rest is «ещё N». */
  legendGroupCap: 12,
  /** Share of a colour that stays when dimmed, focus «мягко». */
  dimKeepSoft: 0.55,
  /** Share of a colour that stays when dimmed, focus «строго». */
  dimKeepStrong: 0.16,
  /** The canvas background (graph.css `--canvas-bg`) as rgb: dimming mixes toward it. */
  canvasBg: [0x1c, 0x1e, 0x22] as readonly [number, number, number],
  /** Node radius = base + min(max, sqrt(degree) * perSqrtDegree). */
  nodeSize: { base: 3, max: 18, perSqrtDegree: 2.4 },
  /** Edge width: plain, with a "many"/"keyed" cardinality, and the growth of a lifted edge per doubling of its count (capped). */
  edgeSize: { plain: 2, many: 3.6, liftedPerLog2: 1.6, liftedMaxExtra: 6 },
  /** Width of one kind button in the «Узлы» panel header, px. */
  typeButtonWidth: 26,
  /** Hierarchy layout: gap between siblings and between ranks inside a tree, and between trees. */
  hierarchy: { nodeSep: 14, rankSep: 40, gap: 40, cellPadding: 16, gridStep: 34 },
  /** Paste from the graph: an arrangement wider or taller than this (px) falls back to a grid. */
  pasteMaxExtent: 8000,
  /** Paste from the graph: more nodes than this go in a grid instead of the graph's arrangement. */
  pasteMaxArranged: 300,
  /** Paste from the graph: width-to-height of the fallback grid. */
  pasteGridAspect: 1.6,
} as const;
