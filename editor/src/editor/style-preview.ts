import { PaintRegistry } from "../canvas/render/PaintRegistry.js";
import { svg } from "../canvas/svg.js";
import { iconGlyphByKey } from "../canvas/render/iconGlyph.js";
import type { Paint, ResolvedBlockStyle, ResolvedEdgeStyle } from "../model/StyleLibrary.js";

/**
 * Thumbnails of a resolved style, drawn by the same machinery as the canvas.
 *
 * They go through `PaintRegistry` rather than approximating a gradient in CSS
 * because a preview that lies is worse than no preview: the whole reason to
 * show one in the picker is to answer "is this the style I mean" without
 * applying it and undoing.
 *
 * Every preview carries its own `<defs>`. One shared block would be smaller,
 * but a list row is created and thrown away as the filter is typed, and a
 * gradient whose definition outlived its user would leak ids into the document
 * for as long as the panel stayed open.
 */

type Attrs = NonNullable<Parameters<typeof svg>[1]>;

const BLOCK_W = 60;
const BLOCK_H = 34;
const EDGE_W = 64;
const EDGE_H = 18;

/**
 * A block style in miniature, in its real outline: an actor, a store, a use
 * case and a fork must not all read as the same rounded box. With
 * `container`, the header band is drawn too — a container style is mostly its
 * header.
 */
export function blockPreview(style: ResolvedBlockStyle, options: { container?: boolean } = {}): SVGSVGElement {
  const defs = svg("defs");
  const paints = new PaintRegistry(defs);
  const root = svg("svg", {
    class: "style-preview",
    viewBox: `0 0 ${BLOCK_W} ${BLOCK_H}`,
    width: BLOCK_W,
    height: BLOCK_H,
  }, [defs]);

  const paint: Attrs = {
    fill: paints.fill(style.fill),
    stroke: style.border.color,
    "stroke-width": Math.min(style.border.width, 2),
    "stroke-dasharray": style.border.dash === "none" ? null : scaleDash(style.border.dash),
    "stroke-opacity": style.border.opacity,
  };
  const box = { x: 1.5, y: 1.5, w: BLOCK_W - 3, h: BLOCK_H - 3 };

  if (options.container === true) {
    // The radius is a model-space value drawn here at roughly a third scale, so
    // it is clamped rather than scaled.
    const radius = Math.min(style.radius, 4);
    const header = Math.min(11, box.h * 0.36);
    root.appendChild(svg("rect", { x: box.x, y: box.y, width: box.w, height: box.h, rx: radius, ...paint }));
    root.appendChild(svg("path", {
      d: `M ${box.x + 0.75} ${box.y + header} V ${box.y + radius} Q ${box.x + 0.75} ${box.y + 0.75} ${box.x + radius} ${box.y + 0.75} ` +
        `H ${box.x + box.w - radius} Q ${box.x + box.w - 0.75} ${box.y + 0.75} ${box.x + box.w - 0.75} ${box.y + radius} V ${box.y + header} Z`,
      fill: paints.fill(style.header.fill),
      stroke: "none",
    }));
    root.appendChild(svg("line", {
      x1: box.x, y1: box.y + header, x2: box.x + box.w, y2: box.y + header,
      stroke: style.border.color, "stroke-width": 0.75, "stroke-opacity": style.border.opacity,
    }));
    root.appendChild(bar(box.x + 5, box.y + header / 2 - 1.5, 24, 3, style.header.text.color, style.header.text.opacity));
    return root;
  }

  root.appendChild(outline(style, box, paint));
  if (style.shape === "actor") return root;

  // Two bars standing in for title and subtitle: the colours are what separates
  // otherwise identical styles, and real text at this size is unreadable. On a
  // non-rectangular outline they sit centred, inside the silhouette.
  const rect = style.shape === "rect";
  const icon = rect && style.icon.show;
  if (icon) root.appendChild(iconGlyphByKey(style.icon.glyph, 5, BLOCK_H / 2 - 6, 12, "style-preview-glyph", style.title.color));
  const titleW = rect ? 26 : 22;
  const x = rect ? (icon ? 21 : 8) : (BLOCK_W - titleW) / 2;
  root.appendChild(bar(x, BLOCK_H / 2 - 4, titleW, 3, style.title.color, style.title.opacity));
  if (style.subtitle.show) {
    root.appendChild(bar(rect ? x : (BLOCK_W - 16) / 2, BLOCK_H / 2 + 2.5, 16, 2.5, style.subtitle.color, style.subtitle.opacity));
  }
  return root;
}

/** The style's outline at thumbnail size — the same silhouettes as `canvas/render/shapes.ts`. */
function outline(
  style: ResolvedBlockStyle,
  b: { x: number; y: number; w: number; h: number },
  paint: Attrs,
): SVGElement {
  const r = b.x + b.w;
  const bottom = b.y + b.h;
  const cx = b.x + b.w / 2;
  const cy = b.y + b.h / 2;
  switch (style.shape) {
    case "ellipse":
      return svg("ellipse", { cx, cy, rx: b.w / 2, ry: b.h / 2, ...paint });
    case "diamond":
      return svg("path", { d: `M ${cx} ${b.y} L ${r} ${cy} L ${cx} ${bottom} L ${b.x} ${cy} Z`, ...paint });
    case "hexagon": {
      const notch = b.w * 0.15;
      return svg("path", {
        d: `M ${b.x + notch} ${b.y} L ${r - notch} ${b.y} L ${r} ${cy} L ${r - notch} ${bottom} L ${b.x + notch} ${bottom} L ${b.x} ${cy} Z`,
        ...paint,
      });
    }
    case "cylinder": {
      const cap = Math.min(b.h * 0.18, 5);
      return svg("g", {}, [
        svg("path", {
          d: `M ${b.x} ${b.y + cap} A ${b.w / 2} ${cap} 0 0 1 ${r} ${b.y + cap} L ${r} ${bottom - cap} ` +
            `A ${b.w / 2} ${cap} 0 0 1 ${b.x} ${bottom - cap} Z`,
          ...paint,
        }),
        svg("path", { d: `M ${b.x} ${b.y + cap} A ${b.w / 2} ${cap} 0 0 0 ${r} ${b.y + cap}`, ...paint, fill: "none" }),
      ]);
    }
    case "actor": {
      const headR = b.h * 0.14;
      const headY = b.y + headR + 1;
      const shoulder = headY + headR + 2;
      const hip = b.y + b.h * 0.62;
      const feet = bottom - 1;
      return svg("g", {}, [
        svg("circle", { cx, cy: headY, r: headR, ...paint }),
        svg("path", {
          d: `M ${cx} ${shoulder} L ${cx} ${hip} M ${cx - 8} ${shoulder + 3} L ${cx + 8} ${shoulder + 3} ` +
            `M ${cx} ${hip} L ${cx - 6} ${feet} M ${cx} ${hip} L ${cx + 6} ${feet}`,
          ...paint,
          fill: "none",
          "stroke-linecap": "round",
        }),
      ]);
    }
    case "rect":
    default:
      // Clamped rather than scaled: a 14px radius on a 34px-tall thumbnail
      // would turn every rounded style into the same pill.
      return svg("rect", { x: b.x, y: b.y, width: b.w, height: b.h, rx: Math.min(style.radius, BLOCK_H / 3), ...paint });
  }
}

/** A dash pattern drawn at thumbnail scale, so a dashed border still reads as dashed. */
function scaleDash(dash: string): string {
  const parts = dash.split(/[\s,]+/).map(Number).filter((n) => Number.isFinite(n) && n > 0);
  return parts.length === 0 ? dash : parts.map((n) => Math.max(1, n * 0.5)).join(" ");
}

export function edgePreview(style: ResolvedEdgeStyle): SVGSVGElement {
  const defs = svg("defs");
  const paints = new PaintRegistry(defs);
  const root = svg("svg", {
    class: "style-preview",
    viewBox: `0 0 ${EDGE_W} ${EDGE_H}`,
    width: EDGE_W,
    height: EDGE_H,
  }, [defs]);

  const y = EDGE_H / 2;
  root.appendChild(
    svg("line", {
      x1: 10, y1: y, x2: EDGE_W - 10, y2: y,
      stroke: style.line.color,
      "stroke-width": style.line.width,
      "stroke-dasharray": style.line.dash === "none" ? null : style.line.dash,
      "stroke-opacity": style.line.opacity,
      "marker-start": paints.marker(style.source, style.line.color),
      "marker-end": paints.marker(style.target, style.line.color),
    }),
  );
  return root;
}

/** A swatch of one paint, for the fill editor's live band. */
export function paintPreview(paint: Paint, width = 200, height = 18): SVGSVGElement {
  const defs = svg("defs");
  const paints = new PaintRegistry(defs);
  const root = svg("svg", {
    class: "paint-preview",
    viewBox: `0 0 ${width} ${height}`,
    preserveAspectRatio: "none",
  }, [defs]);
  root.appendChild(
    svg("rect", { x: 0, y: 0, width, height, rx: 3, fill: paints.fill(paint) }),
  );
  return root;
}

function bar(x: number, y: number, w: number, h: number, color: string, opacity: number): SVGElement {
  return svg("rect", { x, y, width: w, height: h, rx: h / 2, fill: color, opacity });
}
