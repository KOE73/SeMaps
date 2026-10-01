import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { COLOR_OPTIONS, FOCUS_OPTIONS, GROUP_OPTIONS, LAYOUT_OPTIONS, type ViewOption } from "./viewOptions.js";
import { LEVEL_GROUPS, type GroupBy, type LevelGroup, type ViewSettings, type ViewState } from "./viewSettings.js";

/** Axes up to this many are segmented buttons; more stay a list. */
const MAX_SEGMENTED_AXES = 5;

/**
 * The «Вид» panel: colour, layout, grouping and focus as plain lists, an icon
 * to an entry, the current one marked. A click applies it. It reads and writes
 * only the view settings store, exactly as the ribbon's selects do, so the two
 * always agree. Every grouping that has levels shows its level buttons («1 2 3»)
 * in its own row, never a dropdown: the selected grouping's current level is
 * filled, the others show the level they remember, subtly; a click on a number
 * selects that grouping at that level.
 */
export class ViewPanel {
  readonly root = el("div", { class: "graph-view-panel" });

  constructor(private readonly store: ViewSettings) {
    store.onChange(() => this.render());
    this.render();
  }

  /**
   * One list. `also` is set together with the chosen value, `dim` greys the whole
   * list out, `seg` puts level buttons in an entry's own row and `after` adds a
   * block under the selected entry.
   */
  private section<K extends keyof ViewState>(
    title: string,
    key: K,
    options: readonly ViewOption<ViewState[K] & string>[],
    opts: { seg?: (value: string, on: boolean) => HTMLElement | null; after?: (value: string) => HTMLElement | null; also?: Partial<ViewState>; dim?: boolean } = {},
  ): HTMLElement {
    const current = this.store.state[key];
    const items: HTMLElement[] = [];
    for (const o of options) {
      const on = o.value === current;
      const icon = el("span", { class: "ui-icon" });
      icon.innerHTML = o.icon;
      const mark = el("span", { class: "graph-check-btn-mark" });
      if (on) mark.innerHTML = icons.check;
      const seg = opts.seg?.(o.value, on) ?? null;
      // An entry with level buttons is a row that also selects; its buttons select at their level.
      const row = el(seg ? "div" : "button", { class: `graph-check-btn${on ? " is-on" : ""}`, attrs: { "aria-pressed": String(on) } }, [
        icon,
        el("span", { class: "graph-check-btn-label", text: o.label }),
        seg,
        on ? mark : null,
      ]);
      row.addEventListener("click", () => this.store.set({ [key]: o.value, ...opts.also } as Partial<ViewState>));
      items.push(row);
      const extra = on ? opts.after?.(o.value) : null;
      if (extra) items.push(extra);
    }
    return el("section", { class: `graph-filter-group${opts.dim ? " is-dim" : ""}` }, [
      el("h3", { text: title }),
      ...(opts.dim ? [el("p", { class: "tool-muted graph-view-hint", text: t.graphGroupHint })] : []),
      el("div", { class: "graph-filter-list" }, items),
    ]);
  }

  /** A row of buttons, one per value: `current` filled when `filled`, else marked subtly (the level an entry remembers). */
  private segmented(
    label: string,
    values: readonly { value: string; label: string }[],
    current: string,
    filled: boolean,
    apply: (v: string) => void,
  ): HTMLElement {
    const buttons = values.map((v) => {
      const same = v.value === current;
      const b = el("button", { class: `graph-seg-btn${same ? (filled ? " is-on" : " is-mem") : ""}`, text: v.label, attrs: { "aria-pressed": String(same && filled) } });
      b.addEventListener("click", (e) => {
        e.stopPropagation(); // the row's own click would select the grouping without the level
        apply(v.value);
      });
      return b;
    });
    return el("div", { class: "graph-seg", attrs: { title: label, "aria-label": label } }, buttons);
  }

  /** Level (or axis) buttons of a grouping, in its row; null for a grouping without levels. */
  private levels(value: GroupBy, on: boolean, dim: boolean): HTMLElement | null {
    const { state, available } = this.store;
    // Picking a level of an entry selects that grouping (and, while dimmed, the «группами» layout).
    const pick = (patch: Partial<ViewState>) => this.store.set({ groupBy: value, ...patch, ...(dim ? { layout: "grouped" as const } : {}) });
    if ((LEVEL_GROUPS as readonly string[]).includes(value)) {
      const g = value as LevelGroup;
      const max = g === "namespace" ? available.namespaceDepth : g === "folder" ? available.folderDepth : available.containerDepth;
      const depths = Array.from({ length: Math.max(1, max) }, (_, i) => ({ value: String(i + 1), label: String(i + 1) }));
      return this.segmented(t.graphGroupDepth, depths, String(Math.min(state.groupDepths[g], depths.length)), on, (v) =>
        pick({ groupDepths: { ...state.groupDepths, [g]: Number(v) } }),
      );
    }
    if (value === "axis" && available.axes.length > 0 && available.axes.length <= MAX_SEGMENTED_AXES) {
      const axes = available.axes.map((a) => ({ value: a, label: a.replace(/^axis_/, "") }));
      return this.segmented(t.graphGroupAxisLabel, axes, state.groupAxis || available.axes[0]!, on, (v) => pick({ groupAxis: v }));
    }
    return null;
  }

  /** Many axes: a short list under the axis entry, one per line. */
  private axisList(value: string): HTMLElement | null {
    const { state, available } = this.store;
    if (value !== "axis" || available.axes.length <= MAX_SEGMENTED_AXES) return null;
    const current = state.groupAxis || available.axes[0]!;
    const list = available.axes.map((a) => {
      const b = el("button", { class: `graph-check-btn${a === current ? " is-on" : ""}`, attrs: { "aria-pressed": String(a === current) } }, [el("span", { class: "graph-check-btn-label", text: a })]);
      b.addEventListener("click", () => this.store.set({ groupAxis: a }));
      return b;
    });
    return el("div", { class: "graph-filter-list graph-view-axes" }, list);
  }

  private render(): void {
    // Grouping shapes the «группами» layout and the colour «по группе»; under neither it is dimmed,
    // and a click on it switches the layout over.
    const dimGroup = this.store.state.layout !== "grouped" && this.store.state.colorBy !== "group";
    this.root.replaceChildren(
      this.section(t.graphColorBy, "colorBy", COLOR_OPTIONS),
      this.section(t.graphLayout, "layout", LAYOUT_OPTIONS),
      this.section(t.graphGroupBy, "groupBy", GROUP_OPTIONS, {
        seg: (v, on) => this.levels(v as GroupBy, on, dimGroup),
        after: (v) => this.axisList(v),
        also: dimGroup ? { layout: "grouped" } : {},
        dim: dimGroup,
      }),
      this.section(t.graphFocus, "focus", FOCUS_OPTIONS),
    );
  }
}
