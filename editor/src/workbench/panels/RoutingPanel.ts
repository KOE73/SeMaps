import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { canvas } from "../../constants/canvas.js";
import { svg, text } from "../../canvas/svg.js";
import {
  ROUTING_DEFAULTS, ROUTING_PARAMS, resetRoutingTuning, routingTuning, setRoutingTuning,
  type RoutingKey, type RoutingParam,
} from "../../canvas/routing/tuning.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

interface Row {
  readonly param: RoutingParam;
  readonly range: HTMLInputElement;
  readonly number: HTMLInputElement;
  readonly reset: HTMLButtonElement;
}

/** The colours of the zones, the same as the canvas's debug picture. */
const WALL = "220,38,38";
const HALO = "234,179,8";
const FRAME = "234,88,12";
const OVERLAP = "192,38,211";

/**
 * Line routing, tuned live: the router's fine constants as sliders, a picture of
 * the zones they make (drawn to scale from the canvas's default block, gap and
 * caption sizes) and the switch for the zone overlay on the canvas itself.
 * Every change reaches the next render at once and is kept per viewer.
 */
export class RoutingPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly body: HTMLElement;
  private readonly preview: HTMLElement;
  private rows: Row[] = [];
  private zones!: HTMLInputElement;

  constructor(private readonly editor: DiagramEditor) {
    this.body = el("div", { class: "routing-body" });
    this.preview = el("div", { class: "routing-preview" });
    this.element = el("aside", { class: "sidebar routing-panel" }, [this.body]);
    this.build();
    i18n.onLanguageChange(() => this.build());
  }

  init(_params: GroupPanelPartInitParameters): void { this.sync(); }
  onShow(): void { this.sync(); }

  private build(): void {
    const t = i18n.d.panels.routing;
    this.rows = [];

    this.zones = el("input", { type: "checkbox", on: { change: () => { this.editor.canvas.debugRouting = this.zones.checked; } } });
    this.zones.checked = this.editor.canvas.debugRouting;
    const head = el("div", { class: "routing-head" }, [
      el("label", { class: "routing-zones" }, [this.zones, el("span", { text: t.showZones })]),
      el("button", {
        class: "btn btn-small", text: t.resetAll,
        on: { click: () => { resetRoutingTuning(); this.changed(); } },
      }),
    ]);

    const groups = (["zones", "lanes", "search"] as const).map((group) =>
      el("section", { class: "routing-group" }, [
        el("div", { class: "section-label", text: t.groups[group] }),
        ...ROUTING_PARAMS.filter((p) => p.group === group).map((p) => this.row(p)),
      ]));

    replaceChildren(this.body,
      el("div", { class: "routing-hint", text: t.hint }),
      head,
      el("div", { class: "section-label", text: t.previewTitle }),
      this.preview,
      ...groups);
    this.sync();
  }

  private row(param: RoutingParam): HTMLElement {
    const t = i18n.d.panels.routing;
    const { key } = param;
    const set = (raw: string): void => {
      const v = Number(raw);
      if (raw.trim() === "" || !Number.isFinite(v)) return;
      setRoutingTuning({ [key]: v });
      this.changed(key);
    };
    const range = el("input", {
      class: "routing-range",
      attrs: { type: "range", min: String(param.min), max: String(param.max), step: String(param.step) },
      on: { input: () => set(range.value) },
    });
    const number = el("input", {
      class: "routing-number",
      attrs: { type: "number", min: String(param.min), max: String(param.max), step: String(param.step) },
      on: { input: () => set(number.value) },
    });
    const reset = el("button", {
      class: "btn btn-small routing-reset", text: "↺", title: t.reset,
      on: { click: () => { resetRoutingTuning([key]); this.changed(); } },
    });
    this.rows.push({ param, range, number, reset });
    return el("div", { class: "routing-row" }, [
      el("div", { class: "routing-label", text: t.params[param.label] ?? param.label }),
      el("div", { class: "routing-controls" }, [range, number, reset]),
      el("div", { class: "routing-default", text: i18n.format(t.defaultIs, { value: ROUTING_DEFAULTS[key] }) }),
    ]);
  }

  /**
   * A value moved: redraw the canvas from scratch (every route is found again
   * with the new constants) and the picture here. `source` keeps the control the
   * person is dragging from being rewritten under their hand.
   */
  private changed(source?: RoutingKey): void {
    this.editor.canvas.render();
    this.sync(source);
  }

  private sync(source?: RoutingKey): void {
    this.zones.checked = this.editor.canvas.debugRouting;
    for (const row of this.rows) {
      const key = row.param.key;
      const value = routingTuning[key];
      if (key !== source) {
        row.range.value = String(value);
        row.number.value = String(value);
      } else if (document.activeElement !== row.number) {
        row.number.value = String(value);
      }
      row.reset.disabled = value === ROUTING_DEFAULTS[key];
    }
    replaceChildren(this.preview, this.drawPreview());
  }

  /** Two default blocks side by side and a container below, with the bands the router prices. */
  private drawPreview(): HTMLElement {
    const t = i18n.d.panels.routing;
    const { clearance: c, haloWidth: hw } = routingTuning;
    const sizes = canvas();
    const w = sizes.node.width;
    const h = sizes.node.height;
    const gap = sizes.gap.node;
    const header = sizes.container.headerHeight;
    const below = sizes.gap.container;
    const margin = Math.max(c + hw, 20) + 14;
    const top = margin + 10;
    const ax = margin;
    const bx = ax + w + gap;
    const cy = top + h + below;
    const cw = w * 2 + gap;
    const ch = header + h;
    const width = margin * 2 + cw;
    const height = cy + ch + margin;

    const fill = (rgb: string, a: number) => ({ fill: `rgba(${rgb},${a})`, stroke: `rgba(${rgb},0.7)`, "stroke-width": 0.75 });
    const box = (x: number, y: number, bw: number, bh: number, attrs: Record<string, string | number>) =>
      svg("rect", { x, y, width: Math.max(bw, 0), height: Math.max(bh, 0), ...attrs });
    const grow = (x: number, y: number, bw: number, bh: number, by: number, attrs: Record<string, string | number>) =>
      box(x - by, y - by, bw + by * 2, bh + by * 2, attrs);
    /** A ring between a rectangle grown by `outer` and one grown by `inner` (negative: shrunk). */
    const ring = (x: number, y: number, bw: number, bh: number, outer: number, inner: number, rgb: string, a: number) => {
      const r = (g: number) => `M${x - g} ${y - g}h${bw + g * 2}v${bh + g * 2}h${-(bw + g * 2)}z`;
      return svg("path", { d: r(outer) + r(inner), "fill-rule": "evenodd", ...fill(rgb, a) });
    };
    const label = (x: number, y: number, s: string, anchor = "middle") =>
      text({ x, y, "text-anchor": anchor, "font-size": 9, fill: "currentColor", opacity: 0.8 }, s);

    const out: SVGElement[] = [];
    // Halos first (widest, underneath), then the walls, then the blocks.
    for (const x of [ax, bx]) out.push(grow(x, top, w, h, c + hw, fill(HALO, 0.14)));
    out.push(ring(ax, cy, cw, ch, c + hw, c, HALO, 0.14));
    // Where the neighbours' halos meet: a line running there pays twice.
    const overlap = 2 * (c + hw) - gap;
    if (overlap > 0) {
      out.push(box(ax + w + gap / 2 - overlap / 2, top - c - hw, overlap, h + (c + hw) * 2, fill(OVERLAP, 0.35)));
    }
    for (const x of [ax, bx]) out.push(grow(x, top, w, h, c, fill(WALL, 0.16)));
    // The container: its frame band, and the caption strip priced like it.
    out.push(ring(ax, cy, cw, ch, c, -c, FRAME, 0.22));
    if (header > c) out.push(box(ax, cy + c, cw, header - c, fill(FRAME, 0.18)));
    out.push(box(ax, cy, cw, ch, { fill: "none", stroke: "currentColor", "stroke-width": 1, opacity: 0.55 }));
    for (const x of [ax, bx]) {
      out.push(box(x, top, w, h, { fill: "rgba(148,163,184,0.35)", stroke: "currentColor", "stroke-width": 1, opacity: 0.9, rx: 4 }));
    }
    out.push(label(ax + w / 2, top + h / 2 + 3, t.previewBlock));
    out.push(label(bx + w / 2, top + h / 2 + 3, t.previewBlock));
    out.push(label(ax + 6, cy + header / 2 + 3, t.previewContainer, "start"));

    // Sizes: the corridor between the blocks, and the two bands.
    const mid = ax + w + gap / 2;
    const free = gap - 2 * c;
    out.push(label(mid, top - c - hw - 4, `${t.previewGap} ${gap} → ${Math.round(free * 10) / 10}`));
    out.push(label(ax + w / 2, top - c - hw - 4, `${c} + ${hw}`));
    out.push(label(ax - c - hw - 2, top + h / 2 + 3, `${c}+${hw}`, "end"));
    out.push(label(ax + cw - 6, cy + header / 2 + 3, t.legendCaption, "end"));

    const legend: [string, string][] = [
      [WALL, t.legendClearance], [HALO, t.legendHalo], [FRAME, t.legendBorder], [OVERLAP, t.legendOverlap],
    ];
    const key = el("div", { class: "routing-legend" }, legend.map(([rgb, name]) =>
      el("span", { class: "routing-legend-item" }, [
        el("i", { attrs: { style: `background: rgba(${rgb},0.45); border-color: rgba(${rgb},0.9);` } }),
        name,
      ])));
    const drawing = svg("svg", { viewBox: `0 0 ${width} ${height}`, class: "routing-svg", role: "img", "aria-label": t.previewTitle }, out);
    return el("div", {}, [drawing, key]);
  }
}
