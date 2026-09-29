import { el } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { kindIcon, kindIconEl } from "../../ui/kindIcons.js";
import { openContextMenu, type MenuItem } from "../../workbench/menus/ContextMenu.js";
import type { GraphNode } from "./types.js";
import { filterStore, nodeVisible, symbolKindOf } from "./filters.js";
import { nodeSelection, workingSet } from "./workset.js";

/** What the panel needs from the engine that is currently alive. */
export interface NodesSource {
  nodes(): readonly GraphNode[];
  colorOf(n: GraphNode): string;
  focus(id: string): void;
  /** The node menu, the same one the canvas opens. */
  openMenu(id: string, clientX: number, clientY: number, scope?: readonly string[]): void;
}

const ROW_H = 24;
const HEAD_H = 24;
const OVERSCAN = 8;
const TYPE_BTN_W = 26;

type SortKey = "chosen" | "kind" | "name" | "ns";

const nameOf = (n: GraphNode): string => n.name ?? n.id;

/**
 * The «Узлы» panel. Two separate things live here:
 * - «Выбранные» (the working set): the checkbox of a row; what the graph shows.
 * - «Выделение» (the selection): highlighted rows; plain click = only this one,
 *   Ctrl+click = toggle, Shift+click = range. It is sticky — a search, a change
 *   of the set or a re-layout leave it alone; only a plain click replaces it
 *   and Esc / the button empty it.
 * The list is the universe (what the pre-filter lets through), narrowed by a
 * row of kind buttons (the LIST only) and a word search (all words must occur
 * in the name or id), sorted by its column headers and virtualised. It
 * outlives the engine: `setSource` connects the current one.
 */
export class NodesView {
  readonly root: HTMLElement;
  private source: NodesSource | undefined;
  private readonly search: HTMLInputElement;
  private readonly info = el("div", { class: "graph-nodes-info" });
  private readonly typeRow = el("div", { class: "graph-nodes-types" });
  private readonly head = el("div", { class: "graph-nodes-colhead graph-node-grid" });
  private readonly list = el("div", { class: "graph-nodes-list", attrs: { tabindex: "0" } });
  private readonly spacer = el("div", { class: "graph-nodes-spacer" });
  private readonly empty = el("p", { class: "tool-muted graph-nodes-empty", text: t.graphNodesEmpty });
  private readonly onlySetBtn: HTMLButtonElement;
  private onlySet = false;
  private readonly kinds = new Set<string>(); // list-only kind filter; empty = every kind
  private sortKey: SortKey = "name";
  private sortDir = 1;
  private shown: GraphNode[] = [];
  private anchor = -1;
  private raf = 0;

  constructor() {
    this.search = el("input", { class: "graph-search", placeholder: t.graphSearchPlaceholder }) as HTMLInputElement;
    this.search.addEventListener("input", () => this.recompute());

    const button = (icon: string, title: string, run: () => void): HTMLButtonElement => {
      const b = el("button", { class: "tool-btn graph-icon-btn", title }) as HTMLButtonElement;
      b.innerHTML = icon;
      b.setAttribute("aria-label", title);
      b.addEventListener("click", run);
      return b;
    };
    this.onlySetBtn = button(icons.filter, t.graphNodesOnlySet, () => {
      this.onlySet = !this.onlySet;
      this.onlySetBtn.classList.toggle("is-on", this.onlySet);
      this.recompute();
    });
    const top = el("div", { class: "graph-nodes-head" }, [
      this.search,
      button(icons.listCheck, t.graphNodesSelectFound, () => workingSet.add(this.shown.map((n) => n.id))),
      button(icons.playlistX, t.graphNodesClear, () => workingSet.clear()),
      button(icons.squareOff, t.graphNodesClearSelection, () => nodeSelection.clear()),
      this.onlySetBtn,
    ]);

    this.list.append(this.head, this.spacer);
    this.list.addEventListener("scroll", () => this.schedulePaint());
    this.list.addEventListener("click", (e) => this.onClick(e));
    this.list.addEventListener("dblclick", (e) => this.onDoubleClick(e));
    this.list.addEventListener("contextmenu", (e) => this.onContextMenu(e));
    // In the list, Ctrl+A / Ctrl+I mean the rows listed now (the graph's own handler stands down).
    this.list.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        nodeSelection.clear();
      } else if ((e.ctrlKey || e.metaKey) && e.code === "KeyA") {
        e.preventDefault();
        nodeSelection.selectAll(this.shown.map((n) => n.id));
      } else if ((e.ctrlKey || e.metaKey) && e.code === "KeyI") {
        e.preventDefault();
        nodeSelection.invert(this.shown.map((n) => n.id));
      }
    });
    new ResizeObserver(() => this.schedulePaint()).observe(this.list);
    new ResizeObserver(() => this.renderTypes()).observe(this.typeRow);
    this.root = el("div", { class: "graph-nodes" }, [top, this.typeRow, this.info, this.list, this.empty]);

    filterStore.onChange(() => this.recompute());
    workingSet.onChange(() => {
      if (this.onlySet || this.sortKey === "chosen") this.recompute();
      else this.paint();
    });
    nodeSelection.onChange(() => {
      this.paint();
      this.scrollToPrimary();
    });
    this.renderHead();
    this.recompute();
  }

  setSource(source: NodesSource | undefined): void {
    this.source = source;
    this.recompute();
  }

  /** The engine's data changed (a live update) or its colouring did. */
  refresh(): void {
    this.recompute();
  }

  // ---------------------------------------------------------------- header

  /** Column headers on the rows' own grid; a click sorts, another click turns it round. */
  private renderHead(): void {
    const cell = (key: SortKey, label: string, iconOnly?: string): HTMLElement => {
      const active = this.sortKey === key;
      const b = el("button", { class: `graph-col${active ? " is-active" : ""}`, attrs: { "aria-sort": active ? (this.sortDir > 0 ? "ascending" : "descending") : "none", title: label } }, iconOnly ? [] : [
        el("span", { class: "graph-col-label", text: label }),
      ]);
      if (iconOnly && !active) {
        const glyph = el("span", { class: "graph-col-arrow" });
        glyph.innerHTML = iconOnly;
        b.append(glyph);
      }
      if (active) {
        const arrow = el("span", { class: "graph-col-arrow" });
        arrow.innerHTML = this.sortDir > 0 ? icons.arrowUp : icons.arrowDown;
        b.append(arrow);
      }
      b.addEventListener("click", () => {
        if (this.sortKey === key) this.sortDir = -this.sortDir;
        else {
          this.sortKey = key;
          this.sortDir = 1;
        }
        this.renderHead();
        this.recompute();
      });
      return b;
    };
    this.head.replaceChildren(cell("chosen", t.graphColChosen, icons.check), cell("kind", t.graphColKind, icons.category), cell("name", t.graphColName), cell("ns", t.graphColNs));
  }

  // ------------------------------------------------------------ kind row

  /** One line of icon-only kind buttons; those that do not fit go behind «…». */
  private renderTypes(): void {
    const counts = new Map<string, number>();
    for (const n of this.source?.nodes() ?? []) {
      const k = symbolKindOf(n);
      if (k) counts.set(k, (counts.get(k) ?? 0) + 1);
    }
    const kinds = [...counts.keys()].sort((a, b) => a.localeCompare(b));
    const width = this.typeRow.clientWidth || 300;
    const fit = kinds.length * TYPE_BTN_W <= width ? kinds.length : Math.max(1, Math.floor((width - TYPE_BTN_W) / TYPE_BTN_W));
    const toggle = (kind: string): void => {
      if (this.kinds.has(kind)) this.kinds.delete(kind);
      else this.kinds.add(kind);
      this.renderTypes();
      this.recompute();
    };
    const title = (kind: string) => `${kind}: ${counts.get(kind) ?? 0} — ${t.graphNodesTypeHint}`;

    const buttons = kinds.slice(0, fit).map((kind) => {
      const b = el("button", { class: `graph-type-btn${this.kinds.has(kind) ? " is-on" : ""}`, title: title(kind), attrs: { "aria-pressed": String(this.kinds.has(kind)), "aria-label": kind } });
      b.innerHTML = kindIcon("symbol", kind);
      b.addEventListener("click", () => toggle(kind));
      return b;
    });
    const rest = kinds.slice(fit);
    if (rest.length > 0) {
      const more = el("button", { class: "graph-type-btn", title: t.graphNodesMoreKinds });
      more.innerHTML = icons.dots;
      more.addEventListener("click", () => {
        const r = more.getBoundingClientRect();
        const items: MenuItem[] = rest.map((kind) => ({ label: kind, icon: kindIcon("symbol", kind), note: String(counts.get(kind) ?? 0), checked: this.kinds.has(kind), onSelect: () => toggle(kind) }));
        openContextMenu(items, r.left, r.bottom);
      });
      buttons.push(more);
    }
    this.typeRow.replaceChildren(...buttons);
  }

  // ------------------------------------------------------------------ list

  private recompute(): void {
    const words = this.search.value.toLowerCase().split(/\s+/).filter(Boolean);
    const all = this.source?.nodes() ?? [];
    const dir = this.sortDir;
    const byName = (a: GraphNode, b: GraphNode) => nameOf(a).localeCompare(nameOf(b));
    this.shown = all
      .filter((n) => {
        if (!nodeVisible(n, filterStore.state)) return false;
        if (this.onlySet && !workingSet.has(n.id)) return false;
        if (this.kinds.size > 0 && !this.kinds.has(symbolKindOf(n) ?? "")) return false;
        if (words.length === 0) return true;
        const hay = `${nameOf(n)}\n${n.id}`.toLowerCase();
        return words.every((w) => hay.includes(w));
      })
      .sort((a, b) => {
        if (this.sortKey === "chosen") {
          const d = Number(workingSet.has(b.id)) - Number(workingSet.has(a.id)); // chosen first when ascending
          return d !== 0 ? d * dir : byName(a, b);
        }
        if (this.sortKey === "kind") {
          const d = (symbolKindOf(a) ?? "").localeCompare(symbolKindOf(b) ?? "");
          return d !== 0 ? d * dir : byName(a, b);
        }
        if (this.sortKey === "ns") {
          const d = (a.namespace ?? "").localeCompare(b.namespace ?? "");
          return d !== 0 ? d * dir : byName(a, b);
        }
        return byName(a, b) * dir;
      });
    this.anchor = -1;
    this.spacer.style.height = `${this.shown.length * ROW_H}px`;
    this.empty.hidden = this.shown.length > 0;
    this.renderTypes();
    this.paint();
  }

  private schedulePaint(): void {
    if (this.raf) return;
    this.raf = requestAnimationFrame(() => {
      this.raf = 0;
      this.paint();
    });
  }

  /** A click on the canvas selects a node: bring its row into view. */
  private scrollToPrimary(): void {
    const id = nodeSelection.primary;
    const index = id ? this.shown.findIndex((n) => n.id === id) : -1;
    if (index < 0) return;
    const top = index * ROW_H;
    const height = (this.list.clientHeight || 400) - HEAD_H;
    if (top < this.list.scrollTop || top + ROW_H > this.list.scrollTop + height) {
      this.list.scrollTop = Math.max(0, top - height / 2);
    }
  }

  private paint(): void {
    const all = this.source?.nodes() ?? [];
    const universeIds = new Set(all.filter((n) => nodeVisible(n, filterStore.state)).map((n) => n.id));
    let hidden = 0;
    for (const id of workingSet.members) if (!universeIds.has(id)) hidden++;
    const chosen = fmt(t.graphSetHeader, { inSet: String(workingSet.size), hidden: String(hidden) });
    this.info.textContent = nodeSelection.size > 0 ? `${chosen} · ${fmt(t.graphSelectionCount, { n: String(nodeSelection.size) })}` : chosen;

    const height = this.list.clientHeight || 400;
    const first = Math.max(0, Math.floor(this.list.scrollTop / ROW_H) - OVERSCAN);
    const last = Math.min(this.shown.length, Math.ceil((this.list.scrollTop + height) / ROW_H) + OVERSCAN);
    const rows: HTMLElement[] = [];
    for (let i = first; i < last; i++) rows.push(this.row(this.shown[i]!, i));
    this.list.replaceChildren(this.head, this.spacer, ...rows);
  }

  /** checkbox | kind icon (in the node's colour) | name | dim namespace — on the header's grid. */
  private row(n: GraphNode, index: number): HTMLElement {
    const check = el("input", { class: "graph-node-check", type: "checkbox", title: t.graphNodesCheck }) as HTMLInputElement;
    check.checked = workingSet.has(n.id);
    const icon = kindIconEl("symbol", symbolKindOf(n), "graph-node-kind");
    icon.style.color = this.source?.colorOf(n) ?? "var(--muted)";
    const row = el("div", { class: `graph-node-row graph-node-grid${nodeSelection.has(n.id) ? " is-selected" : ""}`, title: n.id }, [
      check,
      icon,
      // abstract: italic name (UML), static: dotted underline — the same looks as on the canvas
      el("span", { class: `graph-node-name${n.modifiers?.includes("abstract") ? " is-abstract" : ""}${n.modifiers?.includes("static") ? " is-static" : ""}`, text: nameOf(n) }),
      el("span", { class: "graph-node-ns", text: n.namespace ?? "" }),
    ]);
    row.style.top = `${HEAD_H + index * ROW_H}px`;
    row.dataset.index = String(index);
    return row;
  }

  /** Rows are rebuilt on every change, so events are handled on the list. */
  private nodeAt(e: Event): { n: GraphNode; index: number } | undefined {
    const row = (e.target as HTMLElement).closest<HTMLElement>(".graph-node-row");
    const index = row ? Number(row.dataset.index) : -1;
    const n = this.shown[index];
    return n ? { n, index } : undefined;
  }

  private onClick(e: MouseEvent): void {
    const hit = this.nodeAt(e);
    if (!hit) return;
    const { n, index } = hit;
    if ((e.target as HTMLElement).classList.contains("graph-node-check")) {
      // The checkbox chooses; on a highlighted row it chooses every highlighted
      // row together, on any other only this one — the highlight stays as it is.
      const on = (e.target as HTMLInputElement).checked;
      const ids = nodeSelection.has(n.id) ? [...nodeSelection.members] : [n.id];
      if (on) workingSet.add(ids);
      else workingSet.remove(ids);
      return;
    }
    if (e.ctrlKey || e.metaKey) {
      nodeSelection.toggle(n.id);
    } else if (e.shiftKey && this.anchor >= 0) {
      const [a, b] = [Math.min(this.anchor, index), Math.max(this.anchor, index)];
      nodeSelection.add(this.shown.slice(a, b + 1).map((x) => x.id));
    } else {
      nodeSelection.setOnly(n.id);
    }
    this.anchor = index;
  }

  private onDoubleClick(e: MouseEvent): void {
    if ((e.target as HTMLElement).classList.contains("graph-node-check")) return;
    const hit = this.nodeAt(e);
    if (hit) this.source?.focus(hit.n.id);
  }

  private onContextMenu(e: MouseEvent): void {
    const hit = this.nodeAt(e);
    if (!hit) return;
    e.preventDefault();
    // Select all / invert in this menu mean the rows listed now.
    this.source?.openMenu(hit.n.id, e.clientX, e.clientY, this.shown.map((n) => n.id));
  }
}
