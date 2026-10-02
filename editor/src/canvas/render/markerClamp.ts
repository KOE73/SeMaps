import { ZOOM_MAX } from "../Viewport.js";

/**
 * A per-viewer preference: keep relation heads between `min` and `max` screen
 * pixels instead of letting them scale with the diagram. Never part of the
 * model, never part of an export.
 */
export interface MarkerClamp {
  readonly on: boolean;
  readonly min: number;
  readonly max: number;
}

export const DEFAULT_MARKER_CLAMP: MarkerClamp = { on: true, min: 7, max: 14 };

export function normalizeMarkerClamp(raw: Partial<MarkerClamp> | null | undefined): MarkerClamp {
  const num = (v: unknown, fallback: number): number =>
    typeof v === "number" && Number.isFinite(v) && v > 0 ? v : fallback;
  const min = num(raw?.min, DEFAULT_MARKER_CLAMP.min);
  const max = Math.max(min, num(raw?.max, DEFAULT_MARKER_CLAMP.max));
  return { on: raw?.on ?? DEFAULT_MARKER_CLAMP.on, min, max };
}

/**
 * The smallest factor a head of styled `size` is ever drawn at, over the whole
 * zoom range: the head shrinks only when `size × zoom` exceeds `max`, and the
 * most it can do so is at ZOOM_MAX. The line is cut for this smallest head, so
 * a larger one overlaps its end instead of leaving a gap. Keep in step with
 * `--kmin` in canvas.css.
 */
export function markerMinScale(size: number, clamp: MarkerClamp): number {
  if (!clamp.on || size <= 0) return 1;
  return Math.min(1, clamp.max / (size * ZOOM_MAX));
}
