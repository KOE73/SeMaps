import type { Rect } from "../../geometry/types.js";
import { HANDLE_ATTR, ROLE_ATTR, Role } from "../../interaction/roles.js";
import { svg } from "../svg.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";

/** Which part of a rectangle a resize handle drags. */
export type ResizeDirection = "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw";

export const RESIZE_DIRECTIONS: readonly ResizeDirection[] = [
  "nw", "n", "ne", "e", "se", "s", "sw", "w",
];

const CURSORS: Readonly<Record<ResizeDirection, string>> = {
  n: "ns-resize",
  s: "ns-resize",
  e: "ew-resize",
  w: "ew-resize",
  ne: "nesw-resize",
  sw: "nesw-resize",
  nw: "nwse-resize",
  se: "nwse-resize",
};

const SIZE: number = DIAGRAM_CONFIG.handles.size;

/** Top-left corner of the handle box for one direction. */
export function handlePosition(rect: Rect, dir: ResizeDirection, size: number = SIZE): Rect {
  const half = size / 2;
  const left = rect.x - half;
  const midX = rect.x + rect.width / 2 - half;
  const rightX = rect.x + rect.width - half;
  const top = rect.y - half;
  const midY = rect.y + rect.height / 2 - half;
  const bottomY = rect.y + rect.height - half;

  const x = dir === "w" || dir === "nw" || dir === "sw"
    ? left
    : dir === "e" || dir === "ne" || dir === "se"
      ? rightX
      : midX;
  const y = dir === "n" || dir === "ne" || dir === "nw"
    ? top
    : dir === "s" || dir === "se" || dir === "sw"
      ? bottomY
      : midY;

  return { x, y, width: size, height: size };
}

/**
 * The eight grips around a rectangle.
 *
 * Drawn by the canvas rather than by a renderer, because with more than one
 * element selected the handles belong to the selection's bounding box and not
 * to any single element.
 */
export function resizeHandles(rect: Rect, scale: number): SVGGElement {
  // Handles are screen-sized: dividing by the zoom keeps them grabbable when
  // zoomed out and unobtrusive when zoomed in.
  const size = SIZE / scale;

  return svg(
    "g",
    { class: "semaps-handles" },
    RESIZE_DIRECTIONS.map((dir) => {
      const box = handlePosition(rect, dir, size);
      return svg("rect", {
        [ROLE_ATTR]: Role.ResizeHandle,
        [HANDLE_ATTR]: dir,
        class: "semaps-handle",
        x: box.x,
        y: box.y,
        width: box.width,
        height: box.height,
        rx: 2 / scale,
        style: `cursor: ${CURSORS[dir]}`,
      });
    }),
  );
}

/** Dashed outline drawn around a multi-element selection. */
export function selectionOutline(rect: Rect, scale: number): SVGRectElement {
  return svg("rect", {
    class: "semaps-selection-outline",
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    "stroke-width": 1.5 / scale,
    "stroke-dasharray": `${6 / scale},${4 / scale}`,
  });
}

/** One edge being dragged by a resize grip, and whether it lines up with another element. */
export interface ResizeGuide {
  readonly axis: "x" | "y";
  readonly at: number;
  readonly aligned: boolean;
}

/** How far a guide runs each way: well past anything anyone draws. */
const GUIDE_REACH = 100_000;

/**
 * A line through the dragged edge across the whole plane, so it can be read
 * against every other element. It turns solid when it meets another edge.
 */
export function resizeGuide(guide: ResizeGuide, scale: number): SVGLineElement {
  const vertical = guide.axis === "x";
  return svg("line", {
    class: `semaps-resize-guide${guide.aligned ? " is-aligned" : ""}`,
    x1: vertical ? guide.at : -GUIDE_REACH,
    x2: vertical ? guide.at : GUIDE_REACH,
    y1: vertical ? -GUIDE_REACH : guide.at,
    y2: vertical ? GUIDE_REACH : guide.at,
    "stroke-dasharray": guide.aligned ? null : `${5 / scale},${4 / scale}`,
  });
}

/** The rubber band drawn while sweeping a selection. */
export function marquee(rect: Rect, scale: number): SVGRectElement {
  return svg("rect", {
    class: "semaps-marquee",
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    "stroke-width": 1 / scale,
    "stroke-dasharray": `${4 / scale},${3 / scale}`,
  });
}
