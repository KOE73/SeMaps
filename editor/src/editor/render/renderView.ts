import type { Rect } from "../../geometry/types.js";
import type { DiagramCanvas } from "../../canvas/DiagramCanvas.js";
import type { DiagramEditor } from "../DiagramEditor.js";
import type { RenderRequest } from "../io/HostModelStore.js";
import { MARKER_BODY_CLASS } from "../../canvas/render/PaintRegistry.js";
import { markerMinScale } from "../../canvas/render/markerClamp.js";
import { findProblems, type Problem } from "./problems.js";

export type RenderAnswer =
  | { png: string; width: number; height: number; rect: Rect; problems: Problem[] }
  | { error: string };

const NS = "http://www.w3.org/2000/svg";
const MARGIN = 24;
/** Same as the host's default; the host always sends its own. */
const DEFAULT_MAX = 1600;
/** Styles the stylesheet would otherwise supply, read from the browser and written inline. */
const INLINE = [
  "fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity", "stroke-dasharray",
  "stroke-dashoffset", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit", "opacity", "font-family",
  "font-size", "font-weight", "font-style", "letter-spacing", "text-anchor", "dominant-baseline", "paint-order",
  "filter", "display", "visibility", "marker-start", "marker-mid", "marker-end", "stop-color", "stop-opacity",
];

/**
 * Answer one render request from the editor's current working state, unsaved
 * included. The view on screen is drawn from its own canvas; any other view of
 * the project is drawn from a hidden canvas that lives only for this answer.
 */
export async function renderView(editor: DiagramEditor, req: RenderRequest): Promise<RenderAnswer> {
  try {
    if (editor.currentViewId === req.view && editor.canvas.model !== null) {
      return await draw(editor.canvas, req);
    }
    const side = await editor.offscreenCanvas(req.view);
    try {
      return await draw(side.canvas, req);
    } finally {
      side.dispose();
    }
  } catch (e) {
    return { error: (e as Error).message };
  }
}

async function draw(canvas: DiagramCanvas, req: RenderRequest): Promise<RenderAnswer> {
  const region = regionOf(canvas, req);
  if ("error" in region) return region;

  const wanted = req.scale && req.scale > 0 ? req.scale : 1;
  const max = req.maxSize && req.maxSize > 0 ? req.maxSize : DEFAULT_MAX;
  const scale = Math.min(wanted, max / Math.max(region.width, region.height));
  const width = Math.max(1, Math.round(region.width * scale));
  const height = Math.max(1, Math.round(region.height * scale));

  const png = await paint(canvas, region, width, height);
  return { png, width, height, rect: region, problems: findProblems(canvas) };
}

function regionOf(canvas: DiagramCanvas, req: RenderRequest): Rect | { error: string } {
  const grow = (r: Rect): Rect => ({ x: r.x - MARGIN, y: r.y - MARGIN, width: r.width + MARGIN * 2, height: r.height + MARGIN * 2 });

  if (req.rect && req.rect.width > 0 && req.rect.height > 0) return req.rect;

  const container = req.ref?.includes("#") ? req.ref.slice(req.ref.indexOf("#") + 1) : "";
  if (container !== "") {
    const box = canvas.shownBoxes().find((b) => b.el.id === container);
    if (box === undefined) return { error: `container ${container} is not shown in view ${req.view}` };
    return grow(box.rect);
  }

  const all = canvas.contentBounds();
  if (all === null) return { error: `view ${req.view} is empty` };
  return grow(all);
}

/** The region as a PNG, base64 without the data: prefix. */
async function paint(canvas: DiagramCanvas, region: Rect, width: number, height: number): Promise<string> {
  const live = canvas.svgElement;
  const host = canvas.hostElement;

  const copy = live.cloneNode(true) as SVGSVGElement;
  // The picture is the drawing alone: no grips, guides, debug layers, or invisible hit areas.
  for (const sel of [".semaps-layer-overlay", ".semaps-layer-debug", ".semaps-edge-hit", ".semaps-edge-glow"]) {
    for (const node of copy.querySelectorAll(sel)) {
      if (sel === ".semaps-layer-overlay" || sel === ".semaps-layer-debug") node.replaceChildren();
      else node.remove();
    }
  }
  for (const node of copy.querySelectorAll(".is-selected, .is-drop-target, .is-ghost-focus")) {
    node.classList.remove("is-selected", "is-drop-target", "is-ghost-focus");
  }
  // Model coordinates 1:1: the live pan and zoom are not part of the picture.
  const group = copy.querySelector<SVGGElement>(".semaps-viewport");
  if (group) {
    group.setAttribute("transform", "translate(0, 0) scale(1)");
    group.style.setProperty("--semaps-stroke-k", "1");
  }
  // Heads at their styled size, not the on-screen clamp. The line was cut for the
  // smallest clamped head, so the body keeps only the shift that puts its tip back
  // on the box edge; the stylesheet rule is not part of the exported picture.
  const clampMax = Number.parseFloat(live.style.getPropertyValue("--semaps-marker-max"));
  for (const body of copy.querySelectorAll<SVGGElement>(`.${MARKER_BODY_CLASS}`)) {
    const size = Number.parseFloat(body.style.getPropertyValue("--ms"));
    const tail = Number.parseFloat(body.style.getPropertyValue("--mt"));
    const kmin = Number.isFinite(clampMax) ? markerMinScale(size, { on: true, min: 0, max: clampMax }) : 1;
    body.removeAttribute("style");
    body.removeAttribute("class");
    if (kmin < 1) body.setAttribute("transform", `translate(${-(1 - kmin) * tail} 0)`);
  }
  copy.setAttribute("xmlns", NS);
  copy.setAttribute("viewBox", `${region.x} ${region.y} ${region.width} ${region.height}`);
  copy.setAttribute("width", String(width));
  copy.setAttribute("height", String(height));

  // Off-screen, inside a canvas host, so the same stylesheet rules apply; then read them.
  const stage = document.createElement("div");
  stage.className = host.className.replace(/\b(is-panning|with-grid)\b/g, "").trim();
  stage.style.cssText = `position:fixed;left:-100000px;top:0;width:${width}px;height:${height}px;overflow:hidden;pointer-events:none;`;
  stage.appendChild(copy);
  document.body.appendChild(stage);
  let background = "#ffffff";
  try {
    const bg = getComputedStyle(host).backgroundColor;
    if (bg && bg !== "rgba(0, 0, 0, 0)" && bg !== "transparent") background = bg;
    for (const node of copy.querySelectorAll("*")) {
      const style = getComputedStyle(node);
      const target = (node as SVGElement).style;
      for (const prop of INLINE) {
        const value = style.getPropertyValue(prop);
        if (value !== "") target.setProperty(prop, value);
      }
    }
  } finally {
    stage.remove();
  }

  const markup = new XMLSerializer().serializeToString(copy);
  const url = URL.createObjectURL(new Blob([markup], { type: "image/svg+xml;charset=utf-8" }));
  try {
    const image = new Image();
    image.decoding = "sync";
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = () => reject(new Error("the browser could not draw the view as an image"));
      image.src = url;
    });
    const surface = document.createElement("canvas");
    surface.width = width;
    surface.height = height;
    const ctx = surface.getContext("2d");
    if (ctx === null) throw new Error("no 2d canvas");
    ctx.fillStyle = background;
    ctx.fillRect(0, 0, width, height);
    ctx.drawImage(image, 0, 0, width, height);
    const data = surface.toDataURL("image/png");
    return data.slice(data.indexOf(",") + 1);
  } finally {
    URL.revokeObjectURL(url);
  }
}
