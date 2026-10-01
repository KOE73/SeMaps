import { icons, type IconName } from "../../ui/icons.js";
import { iconByKey } from "../../ui/kindIcons.js";
import { svg } from "../svg.js";

/** The drawing part of a Tabler icon: what sits inside its outer `<svg>`, without the reset path. */
function innerMarkup(source: string): string {
  return source
    .replace(/^[\s\S]*?<svg[^>]*>/, "")
    .replace(/<\/svg>\s*$/, "")
    .replace(/<path[^>]*stroke="none"[^>]*\/>/, "");
}

function draw(source: string, x: number, y: number, size: number, cls: string, stroke?: string): SVGSVGElement {
  const node = svg("svg", {
    x,
    y,
    width: size,
    height: size,
    viewBox: "0 0 24 24",
    fill: "none",
    "stroke-width": 2,
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
    class: cls,
    "pointer-events": "none",
    // An explicit colour keeps the icon right wherever the SVG is taken to (no stylesheet needed).
    ...(stroke ? { stroke } : {}),
  });
  node.innerHTML = innerMarkup(source);
  return node;
}

/**
 * A Tabler outline icon drawn inside a diagram's SVG at (x, y), `size` px square.
 * It is stroked, not filled: colour it with `stroke` from the CSS class, e.g.
 * `.cls { stroke: #334155; }` (the icon's own `fill` stays none), or pass `stroke`.
 */
export function iconGlyph(name: IconName, x: number, y: number, size: number, cls: string, stroke?: string): SVGSVGElement {
  return draw(icons[name], x, y, size, cls, stroke);
}

/** The same for a stored icon (a style's `icon.glyph`): a registry key, else the fallback icon. */
export function iconGlyphByKey(value: string | undefined, x: number, y: number, size: number, cls: string, stroke?: string): SVGSVGElement {
  return draw(iconByKey(value), x, y, size, cls, stroke);
}
