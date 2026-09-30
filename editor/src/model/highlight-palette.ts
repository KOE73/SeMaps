/**
 * The ready-made colours of «Выделить» (ADR_20260927-7): a colour is a local
 * override of one placement, not a style. Each entry sets, in one click, the
 * override fields of the table in `override.ts` that carry a colour: fill,
 * border colour and (on a container) the header fill.
 */
export interface HighlightColour {
  readonly id: string;
  readonly fill: string;
  readonly border: string;
  readonly header: string;
}

export const HIGHLIGHT_PALETTE: readonly HighlightColour[] = [
  { id: "green", fill: "#f0fdf4", border: "#86efac", header: "#dcfce7" },
  { id: "blue", fill: "#eff6ff", border: "#93c5fd", header: "#dbeafe" },
  { id: "fuchsia", fill: "#fdf4ff", border: "#f0abfc", header: "#fae8ff" },
  { id: "red", fill: "#fff1f2", border: "#fca5a5", header: "#ffe4e6" },
  { id: "yellow", fill: "#fefce8", border: "#fde047", header: "#fef9c3" },
  { id: "slate", fill: "#f8fafc", border: "#cbd5e1", header: "#e2e8f0" },
  { id: "violet", fill: "#f5f3ff", border: "#c4b5fd", header: "#ede9fe" },
  { id: "amber", fill: "#fffbeb", border: "#fde68a", header: "#fef3c7" },
  { id: "sky", fill: "#f0f9ff", border: "#7dd3fc", header: "#e0f2fe" },
  { id: "cyan", fill: "#ecfeff", border: "#67e8f9", header: "#cffafe" },
  { id: "lime", fill: "#f7fee7", border: "#bef264", header: "#ecfccb" },
  { id: "pink", fill: "#fdf2f8", border: "#f9a8d4", header: "#fce7f3" },
  { id: "gray", fill: "#f1f5f9", border: "#94a3b8", header: "#e2e8f0" },
];

/**
 * The line colours offered for one edge: the border colours of the palette
 * are pale on purpose (they frame a box), so a line takes the stronger,
 * readable shade of each hue.
 */
export interface LineColour {
  readonly id: string;
  readonly color: string;
}

export const LINE_PALETTE: readonly LineColour[] = [
  { id: "green", color: "#16a34a" },
  { id: "blue", color: "#2563eb" },
  { id: "fuchsia", color: "#c026d3" },
  { id: "red", color: "#dc2626" },
  { id: "yellow", color: "#ca8a04" },
  { id: "slate", color: "#475569" },
  { id: "violet", color: "#7c3aed" },
  { id: "amber", color: "#d97706" },
  { id: "sky", color: "#0284c7" },
  { id: "cyan", color: "#0891b2" },
  { id: "lime", color: "#65a30d" },
  { id: "pink", color: "#db2777" },
  { id: "gray", color: "#94a3b8" },
];
