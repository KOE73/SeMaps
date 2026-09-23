import type { Point, Rect } from "../geometry/types.js";
import { Emitter } from "../util/emitter.js";

export const ZOOM_MIN = 0.2;
export const ZOOM_MAX = 3.0;
export const FIT_PADDING = 60;
export const FIT_MAX_ZOOM = 1.2;

export interface ViewportState {
  readonly zoom: number;
  readonly panX: number;
  readonly panY: number;
}

const INITIAL: ViewportState = { zoom: 0.9, panX: 40, panY: 20 };
const RESET: ViewportState = { zoom: 1, panX: 40, panY: 20 };

/**
 * Pan and zoom over the model plane.
 *
 * Kept apart from rendering on purpose: moving the viewport is a transform on
 * one group and must never require rebuilding the diagram, so that incremental
 * rendering stays possible later.
 */
export class Viewport {
  readonly changed = new Emitter<{ change: ViewportState }>();

  private state: ViewportState = INITIAL;

  constructor(private readonly group: SVGGElement) {
    this.apply();
  }

  get zoom(): number {
    return this.state.zoom;
  }

  get panX(): number {
    return this.state.panX;
  }

  get panY(): number {
    return this.state.panY;
  }

  set(next: Partial<ViewportState>): void {
    const zoom = clampZoom(next.zoom ?? this.state.zoom);
    this.state = {
      zoom,
      panX: next.panX ?? this.state.panX,
      panY: next.panY ?? this.state.panY,
    };
    this.apply();
  }

  zoomBy(factor: number): void {
    this.set({ zoom: this.state.zoom * factor });
  }

  zoomTo(zoom: number): void {
    this.set({ zoom });
  }

  /**
   * Zoom while holding one screen point fixed under the cursor, instead of
   * the plane's origin. Reads pan/zoom together rather than going through
   * `set` so there is no frame where the new zoom is applied against the old
   * pan — that would flash the anchor out of place before the corrected pan
   * lands.
   */
  zoomAt(screen: Point, factor: number): void {
    const zoom = clampZoom(this.state.zoom * factor);
    if (zoom === this.state.zoom) return;

    const model = this.toModel(screen);
    this.state = {
      zoom,
      panX: screen.x - model.x * zoom,
      panY: screen.y - model.y * zoom,
    };
    this.apply();
  }

  reset(): void {
    this.state = RESET;
    this.apply();
  }

  panBy(dx: number, dy: number): void {
    this.set({ panX: this.state.panX + dx, panY: this.state.panY + dy });
  }

  /** Fit `bounds` into a viewport of `size`, never magnifying past FIT_MAX_ZOOM. */
  fit(bounds: Rect | null, size: { width: number; height: number }): void {
    if (bounds === null || size.width === 0 || size.height === 0) return;

    const contentW = bounds.width + FIT_PADDING * 2;
    const contentH = bounds.height + FIT_PADDING * 2;
    const zoom = clampZoom(
      Math.min(size.width / contentW, size.height / contentH, FIT_MAX_ZOOM),
    );

    this.state = {
      zoom,
      panX: (size.width - contentW * zoom) / 2 - bounds.x * zoom + FIT_PADDING * zoom,
      panY: (size.height - contentH * zoom) / 2 - bounds.y * zoom + FIT_PADDING * zoom,
    };
    this.apply();
  }

  /** Screen coordinates (relative to the host element) to model coordinates. */
  toModel(p: Point): Point {
    return {
      x: (p.x - this.state.panX) / this.state.zoom,
      y: (p.y - this.state.panY) / this.state.zoom,
    };
  }

  /** A screen-space delta expressed in model units. */
  scaleDelta(dx: number, dy: number): Point {
    return { x: dx / this.state.zoom, y: dy / this.state.zoom };
  }

  private apply(): void {
    const { panX, panY, zoom } = this.state;
    this.group.setAttribute("transform", `translate(${panX}, ${panY}) scale(${zoom})`);
    this.changed.emit("change", this.state);
  }
}

function clampZoom(zoom: number): number {
  return Math.min(Math.max(ZOOM_MIN, zoom), ZOOM_MAX);
}
