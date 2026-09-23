import type { Rect } from "../../geometry/types.js";
import type { TextStyle } from "../../model/StyleLibrary.js";

type AttrValue = string | number | null;

/**
 * A resolved text style as SVG attributes.
 *
 * Every caption on the canvas — node label, subtitle, zone header, edge label —
 * used to spell its font out inline, so "make subtitles smaller" was four edits
 * in three files and they drifted. One conversion, used by every renderer.
 *
 * Attributes that mean "the default" are emitted as null rather than their
 * value: `setAttrs` removes those, which keeps the markup readable and lets the
 * stylesheet still have a say for anything a style did not override.
 */
export function textAttrs(style: TextStyle): Record<string, AttrValue> {
  return {
    "font-family": style.family,
    "font-size": style.size,
    "font-weight": style.weight,
    "font-style": style.italic ? "italic" : null,
    fill: style.color,
    "fill-opacity": style.opacity === 1 ? null : style.opacity,
  };
}

/**
 * Where a caption sits inside a box, given its alignment.
 *
 * The anchor and the x coordinate have to be decided together — a "middle"
 * anchor at the box's left edge puts half the text outside the box — so they
 * are one answer rather than two attributes a caller can mismatch.
 */
export function alignX(
  style: TextStyle,
  rect: Rect,
  pad = 12,
): { x: number; "text-anchor": string } {
  if (style.align === "middle") {
    return { x: rect.x + rect.width / 2, "text-anchor": "middle" };
  }
  if (style.align === "end") {
    return { x: rect.x + rect.width - pad, "text-anchor": "end" };
  }
  return { x: rect.x + pad, "text-anchor": "start" };
}

/** A dash pattern, or null when the stroke is solid and the attribute is noise. */
export function dashArray(dash: string): string | null {
  return dash === "none" || dash === "" ? null : dash;
}
