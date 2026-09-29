import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { COLOR_OPTIONS, FOCUS_OPTIONS, GROUP_OPTIONS, LAYOUT_OPTIONS, type ViewOption } from "./viewOptions.js";
import type { ViewSettings, ViewState } from "./viewSettings.js";

/** Axes up to this many are segmented buttons; more stay a list. */
const MAX_SEGMENTED_AXES = 5;

/**
 * The «Вид» panel: colour, layout, grouping and focus as plain lists, an icon
 * to an entry, the current one marked. A click applies it. It reads and writes
 * only the view settings store, exactly as the ribbon's selects do, so the two
 * always agree. Depth and axis controls appear inline, only under the grouping
 * they belong to, as segmented buttons («1 2 3»), never dropdowns.
 */
export class ViewPanel {
  readonly root = el("div", { class: "graph-view-panel" });

  constructor(private readonly store: ViewSettings) {
    store.onChange(() => this.render());
    this.render();
  }

  /**
   * One list. `also` is set together with the chosen value (choosing a grouping
   * switches to the «группами» layout), `dim` greys the whole list out.
   */
  private section<K extends keyof ViewState>(
    title: string,
    key: K,
    options: readonly ViewOption<ViewState[K] & string>[],
    opts: { after?: (value: string) => HTMLElement | null; also?: Partial<ViewState>; dim?: boolean } = {},
  ): HTMLElement {
    const current = this.store.state[key];
    const items: HTMLElement[] = [];
    for (const o of options) {
      const on = o.value === current;
      const icon = el("span", { class: "ui-icon" });
      icon.innerHTML = o.icon;
      const mark = el("span", { class: "graph-check-btn-mark" });
      if (on) mark.innerHTML = icons.check;
      const extra = on ? opts.after?.(o.value) : null;
      // Level buttons sit in the entry's own row; a long axis list goes below it.
      const inRow = extra?.classList.contains("graph-seg") ? extra : null;
      const btn = el(inRow ? "div" : "button", { class: `graph-check-btn${on ? " is-on" : ""}`, attrs: { "aria-pressed": String(on) } }, [
        icon,
        el("span", { class: "graph-check-btn-label", text: o.label }),
        inRow,
        on ? mark : null,
      ]);
      if (!inRow) btn.addEventListener("click", () => this.store.set({ [key]: o.value, ...opts.also } as Partial<ViewState>));
      items.push(btn);
      if (extra && !inRow) items.push(extra);
    }
    return el("section", { class: `graph-filter-group${opts.dim ? " is-dim" : ""}` }, [
      el("h3", { text: title }),
      ...(opts.dim ? [el("p", { class: "tool-muted graph-view-hint", text: t.graphGroupHint })] : []),
      el("div", { class: "graph-filter-list" }, items),
    ]);
  }

  /** A row of buttons, one per value, the current one highlighted. */
  private segmented(label: string, values: readonly { value: string; label: string }[], current: string, apply: (v: string) => void): HTMLElement {
    const buttons = values.map((v) => {
      const on = v.value === current;
      const b = el("button", { class: `graph-seg-btn${on ? " is-on" : ""}`, text: v.label, attrs: { "aria-pressed": String(on) } });
      b.addEventListener("click", () => apply(v.value));
      return b;
    });
    return el("div", { class: "graph-seg", attrs: { title: label, "aria-label": label } }, buttons);
  }

  /** «Глубина» / «ось» right under the grouping that uses them. */
  private inline(value: string): HTMLElement | null {
    const { state, available } = this.store;
    if (value === "namespace" || value === "folder" || value === "containers") {
      const max = value === "namespace" ? available.namespaceDepth : value === "folder" ? available.folderDepth : available.containerDepth;
      const depths = Array.from({ length: Math.max(1, max) }, (_, i) => ({ value: String(i + 1), label: String(i + 1) }));
      return this.segmented(t.graphGroupDepth, depths, String(Math.min(state.groupDepth, depths.length)), (v) => this.store.set({ groupDepth: Number(v) }));
    }
    if (value === "axis" && available.axes.length > 0) {
      const current = state.groupAxis || available.axes[0]!;
      if (available.axes.length <= MAX_SEGMENTED_AXES) {
        return this.segmented(t.graphGroupAxisLabel, available.axes.map((a) => ({ value: a, label: a.replace(/^axis_/, "") })), current, (v) => this.store.set({ groupAxis: v }));
      }
      // Many axes: a short list, one per line.
      const list = available.axes.map((a) => {
        const b = el("button", { class: `graph-check-btn${a === current ? " is-on" : ""}`, attrs: { "aria-pressed": String(a === current) } }, [el("span", { class: "graph-check-btn-label", text: a })]);
        b.addEventListener("click", () => this.store.set({ groupAxis: a }));
        return b;
      });
      return el("div", { class: "graph-filter-list graph-view-axes" }, list);
    }
    return null;
  }

  private render(): void {
    // Grouping only shapes the «группами» layout: dimmed under any other, and a click on it switches over.
    const dimGroup = this.store.state.layout !== "grouped";
    this.root.replaceChildren(
      this.section(t.graphColorBy, "colorBy", COLOR_OPTIONS),
      this.section(t.graphLayout, "layout", LAYOUT_OPTIONS),
      this.section(t.graphGroupBy, "groupBy", GROUP_OPTIONS, { after: (v) => this.inline(v), also: { layout: "grouped" }, dim: dimGroup }),
      this.section(t.graphFocus, "focus", FOCUS_OPTIONS),
    );
  }
}
