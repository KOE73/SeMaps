import { PaintRegistry } from "../canvas/render/PaintRegistry.js";
import { svg, text } from "../canvas/svg.js";
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

const BLOCK_W = 60;
const BLOCK_H = 34;
const EDGE_W = 64;
const EDGE_H = 18;

export function blockPreview(style: ResolvedBlockStyle): SVGSVGElement {
  const defs = svg("defs");
  const paints = new PaintRegistry(defs);
  const root = svg("svg", {
    class: "style-preview",
    viewBox: `0 0 ${BLOCK_W} ${BLOCK_H}`,
    width: BLOCK_W,
    height: BLOCK_H,
  }, [defs]);

  // The radius is a model-space value drawn here at roughly a third scale, so
  // it is clamped rather than scaled: a 14px radius on a 34px-tall thumbnail
  // would turn every rounded style into the same pill.
  const radius = Math.min(style.radius, BLOCK_H / 3);

  root.appendChild(
    svg("rect", {
      x: 1.5, y: 1.5, width: BLOCK_W - 3, height: BLOCK_H - 3, rx: radius,
      fill: paints.fill(style.fill),
      stroke: style.border.color,
      "stroke-width": Math.min(style.border.width, 2.5),
      "stroke-dasharray": style.border.dash === "none" ? null : style.border.dash,
      "stroke-opacity": style.border.opacity,
    }),
  );

  if (style.icon.show && style.icon.glyph !== "") {
    root.appendChild(
      text({ x: 8, y: BLOCK_H / 2 + 4, "font-size": 11 }, style.icon.glyph),
    );
  }

  // Two bars standing in for title and subtitle: the colours are what separates
  // otherwise identical styles, and real text at this size is unreadable.
  root.appendChild(bar(style.icon.show ? 22 : 8, BLOCK_H / 2 - 5, 26, 3.5, style.title.color, style.title.opacity));
  if (style.subtitle.show) {
    root.appendChild(bar(style.icon.show ? 22 : 8, BLOCK_H / 2 + 3, 18, 2.5, style.subtitle.color, style.subtitle.opacity));
  }

  return root;
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
