import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { type KindGroup, kindIconEl } from "../../ui/kindIcons.js";
import type { GraphNode } from "./types.js";

/** Which icon group each filter group draws from (ui/kindIcons.ts). */
const KIND_GROUP: Record<FilterGroup, KindGroup> = {
  edgeKinds: "edge",
  symbolKinds: "symbol",
  visibility: "visibility",
  presence: "presence",
};

/**
 * Filter state, its single store and the filters panel. Filtering only hides
 * nodes/edges already drawn — it never refetches (PLAN_20260928-2 step 4).
 *
 * `filterStore` is the only way a filter changes: the panel's checkboxes, the
 * ribbon's commands and the live-update merge (`sync`) all go through it, and
 * whoever draws (the engine), lists (the panel) or shows state (the ribbon)
 * subscribes to `onChange` instead of being called by the control.
 */
export interface FilterState {
  edgeKinds: Set<string>;
  symbolKinds: Set<string>;
  visibility: Set<string>;
  presence: Set<string>;
  container: string; // "" = any
}

export type FilterGroup = "edgeKinds" | "symbolKinds" | "visibility" | "presence";

export const PRESENCE_VALUES = ["code", "model", "both"] as const;

export const DEFAULT_EDGE_KINDS: readonly string[] = ["implements"];
export const DEFAULT_SYMBOL_KINDS: readonly string[] = ["class", "enum"];
export const INHERITANCE_EDGE_KINDS: readonly string[] = ["implements", "extends"];
export const INHERITANCE_DEPENDENCY_EDGE_KINDS: readonly string[] = ["implements", "extends", "holds"];
export const CALLS_EDGE_KINDS: readonly string[] = ["calls", "constructs"];
export const PRESET_SYMBOL_KINDS: readonly string[] = ["class", "interface"];

/** What the data offers to filter by; the panel lists exactly these. */
export interface FilterAvailable {
  edgeKinds: string[];
  symbolKinds: string[];
  visibility: string[];
  containers: string[];
}

function distinct<T>(values: Iterable<T | undefined>): T[] {
  return [...new Set([...values].filter((v): v is T => v !== undefined))];
}

/** What the «Виды символов» filter lists: the language's own name for the symbol
 * without its modifiers (`class` for `abstract-class`; `enum`, `struct`, `interface`…),
 * falling back to the coarse kind. */
export function symbolKindOf(n: GraphNode): string | undefined {
  return n.symbolKind ?? n.nativeKind ?? n.kind;
}

export function nodeVisible(n: GraphNode, f: Readonly<FilterState>): boolean {
  if (!f.presence.has(n.presence)) return false;
  const symbol = symbolKindOf(n);
  if (symbol && !f.symbolKinds.has(symbol)) return false;
  if (n.visibility && !f.visibility.has(n.visibility)) return false;
  if (f.container && !(n.containers ?? []).includes(f.container)) return false;
  return true;
}

function emptyState(): FilterState {
  return { edgeKinds: new Set(), symbolKinds: new Set(), visibility: new Set(), presence: new Set(PRESENCE_VALUES), container: "" };
}

function emptyAvailable(): FilterAvailable {
  return { edgeKinds: [], symbolKinds: [], visibility: [], containers: [] };
}

export class FilterStore {
  private current: FilterState = emptyState();
  private offered: FilterAvailable = emptyAvailable();
  /** Values offered by an earlier `sync`: a value seen before keeps the
   * user's choice, a newly offered one starts included. */
  private known: FilterAvailable = emptyAvailable();
  private readonly listeners = new Set<() => void>();

  /** Read-only view; change it only through the methods below. */
  get state(): Readonly<FilterState> {
    return this.current;
  }

  get available(): Readonly<FilterAvailable> {
    return this.offered;
  }

  /** Takes over what the data now offers, keeping earlier choices (a graph
   * re-created for the same project, a live diff). */
  sync(nodes: readonly GraphNode[], edgeKinds: readonly string[]): void {
    const next: FilterAvailable = {
      edgeKinds: distinct(edgeKinds),
      symbolKinds: distinct(nodes.map(symbolKindOf)),
      visibility: distinct(nodes.map((n) => n.visibility)),
      containers: distinct(nodes.flatMap((n) => n.containers ?? [])),
    };
    const merge = (selected: Set<string>, was: readonly string[], now: readonly string[], defaults?: readonly string[]) => {
      const before = new Set(was);
      return new Set(now.filter((v) => (before.has(v) ? selected.has(v) : !defaults || defaults.includes(v))));
    };
    this.current.edgeKinds = merge(this.current.edgeKinds, this.known.edgeKinds, next.edgeKinds, DEFAULT_EDGE_KINDS);
    this.current.symbolKinds = merge(this.current.symbolKinds, this.known.symbolKinds, next.symbolKinds, DEFAULT_SYMBOL_KINDS);
    this.current.visibility = merge(this.current.visibility, this.known.visibility, next.visibility);
    if (this.current.container && !next.containers.includes(this.current.container)) this.current.container = "";
    this.known = next;
    this.offered = next;
    this.emit();
  }

  /** Forgets every choice (another project); the next `sync` includes all. */
  clear(): void {
    this.current = emptyState();
    this.known = emptyAvailable();
    this.offered = emptyAvailable();
    this.emit();
  }

  set(group: FilterGroup, value: string, on: boolean): void {
    const set = this.current[group];
    if (set.has(value) === on) return;
    if (on) set.add(value);
    else set.delete(value);
    this.emit();
  }

  toggle(group: FilterGroup, value: string): void {
    this.set(group, value, !this.current[group].has(value));
  }

  setContainer(container: string): void {
    if (this.current.container === container) return;
    this.current.container = container;
    this.emit();
  }

  /** The defaults: `implements`, `class`/`enum`, all presence and visibility, any container. */
  reset(): void {
    const only = (values: readonly string[], keep: readonly string[]) => new Set(values.filter((v) => keep.includes(v)));
    this.current = {
      edgeKinds: only(this.offered.edgeKinds, DEFAULT_EDGE_KINDS),
      symbolKinds: only(this.offered.symbolKinds, DEFAULT_SYMBOL_KINDS),
      visibility: new Set(this.offered.visibility),
      presence: new Set(PRESENCE_VALUES),
      container: "",
    };
    this.emit();
  }

  /** Everything checked, container any. */
  enableAll(): void {
    this.current = {
      edgeKinds: new Set(this.offered.edgeKinds),
      symbolKinds: new Set(this.offered.symbolKinds),
      visibility: new Set(this.offered.visibility),
      presence: new Set(PRESENCE_VALUES),
      container: "",
    };
    this.emit();
  }

  /** All edge kinds and symbol kinds unchecked; the rest untouched. */
  disableAll(): void {
    this.current.edgeKinds = new Set();
    this.current.symbolKinds = new Set();
    this.emit();
  }

  /** Every value of one group checked (`on`) or unchecked. */
  setAll(group: FilterGroup, on: boolean): void {
    const all = group === "presence" ? [...PRESENCE_VALUES] : this.offered[group];
    this.current[group] = new Set(on ? all : []);
    this.emit();
  }

  /** Only these edge kinds and symbol kinds; presence, visibility, container untouched. */
  onlyKinds(edgeKinds: readonly string[], symbolKinds: readonly string[]): void {
    this.current.edgeKinds = new Set(this.offered.edgeKinds.filter((k) => edgeKinds.includes(k)));
    this.current.symbolKinds = new Set(this.offered.symbolKinds.filter((k) => symbolKinds.includes(k)));
    this.emit();
  }

  /** Whether the data has any of these edge kinds (a preset without one is disabled). */
  hasEdgeKind(kinds: readonly string[]): boolean {
    return this.offered.edgeKinds.some((k) => kinds.includes(k));
  }

  onChange(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const l of [...this.listeners]) l();
  }
}

export const filterStore = new FilterStore();

function link(text: string, run: () => void): HTMLElement {
  const a = el("a", { class: "graph-filter-link", text });
  a.href = "#";
  a.addEventListener("click", (e) => {
    e.preventDefault();
    run();
  });
  return a;
}

function presenceLabel(v: string): string {
  return v === "code" ? t.graphPresenceCode : v === "model" ? t.graphPresenceModel : t.graphPresenceBoth;
}

/**
 * The filters panel: the pre-filter (symbol kinds, presence, visibility,
 * container) and the edge kinds — every filter in one column — a header per section, one checkbox per line.
 * It redraws from the store whenever the store changes.
 */
export class FiltersView {
  readonly root: HTMLElement;
  private readonly body = el("div", { class: "graph-filters" });

  constructor(private readonly store: FilterStore) {
    this.root = el("div", { class: "graph-filters-root" }, [this.body]);
    store.onChange(() => this.render());
    this.render();
  }

  private render(): void {
    const { available: a, state: s } = this.store;
    const scroll = this.root.scrollTop;
    const section = (title: string, group: FilterGroup, values: readonly string[], labelOf: (v: string) => string = (v) => v) =>
      el("section", { class: "graph-filter-group" }, [
        el("div", { class: "graph-filter-head" }, [
          el("h3", { text: title }),
          link(t.graphAll, () => this.store.setAll(group, true)),
          link(t.graphNone, () => this.store.setAll(group, false)),
        ]),
        el(
          "div",
          { class: "graph-filter-list" },
          values.map((v) => {
            // A check-button: the kind's icon and label, clearly on (filled, ticked) or off (outline).
            const on = s[group].has(v);
            const mark = el("span", { class: "graph-check-btn-mark" });
            mark.innerHTML = icons.check;
            const btn = el("button", { class: `graph-check-btn${on ? " is-on" : ""}`, attrs: { "aria-pressed": String(on) } }, [
              kindIconEl(KIND_GROUP[group], v),
              el("span", { class: "graph-check-btn-label", text: labelOf(v) }),
              on ? mark : null,
            ]);
            btn.addEventListener("click", () => this.store.set(group, v, !on));
            return btn;
          }),
        ),
      ]);

    const containerSelect = el("select", {}) as HTMLSelectElement;
    containerSelect.appendChild(el("option", { value: "", text: t.graphAnyContainer }));
    for (const c of a.containers) containerSelect.appendChild(el("option", { value: c, text: c }));
    containerSelect.value = s.container;
    containerSelect.addEventListener("change", () => this.store.setContainer(containerSelect.value));

    const preset = (text: string, run: () => void, enabled = true) => {
      const b = el("button", { class: "tool-btn", text, disabled: !enabled });
      b.addEventListener("click", run);
      return b;
    };
    const presets = el("div", { class: "graph-filter-presets" }, [
      preset(t.graphResetFilters, () => this.store.reset()),
      preset(t.graphEnableAll, () => this.store.enableAll()),
      preset(t.graphDisableAll, () => this.store.disableAll()),
      preset(t.graphOnlyInheritance, () => this.store.onlyKinds(INHERITANCE_EDGE_KINDS, PRESET_SYMBOL_KINDS), this.store.hasEdgeKind(INHERITANCE_EDGE_KINDS)),
      preset(t.graphInheritanceDependencies, () => this.store.onlyKinds(INHERITANCE_DEPENDENCY_EDGE_KINDS, PRESET_SYMBOL_KINDS), this.store.hasEdgeKind(INHERITANCE_DEPENDENCY_EDGE_KINDS)),
      preset(t.graphPresetCalls, () => this.store.onlyKinds(CALLS_EDGE_KINDS, PRESET_SYMBOL_KINDS), this.store.hasEdgeKind(CALLS_EDGE_KINDS)),
    ]);

    this.body.replaceChildren(
      presets,
      section(t.graphFilterEdgeKinds, "edgeKinds", a.edgeKinds),
      section(t.graphFilterSymbolKinds, "symbolKinds", a.symbolKinds),
      section(t.graphFilterVisibility, "visibility", a.visibility),
      section(t.graphFilterPresence, "presence", PRESENCE_VALUES, presenceLabel),
      el("section", { class: "graph-filter-group" }, [el("h3", { text: t.graphFilterContainer }), containerSelect]),
    );
    this.root.scrollTop = scroll;
  }
}
