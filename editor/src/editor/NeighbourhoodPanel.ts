import { el, replaceChildren } from "../util/dom.js";
import { i18n } from "../workbench/i18n/I18nService.js";
import { entityOf } from "../model/types.js";
import type { EntityEntry, RelationEntry } from "../model/wire-types.js";
import { armsOf, nameOf, type Arm, type Dir } from "./relatives.js";
import type { DiagramEditor } from "./DiagramEditor.js";
import { placeEntities } from "./placeEntity.js";

type Group = Arm;

/**
 * The neighbourhood of one entity, as a tree you pull on like a chain: the
 * entity, its relations grouped by type and direction, and each neighbour
 * opening onto its own relations in turn, as deep as you care to go.
 *
 * Walks the project's registry (`relations.json`), not the view: the point is
 * to see what is related before it is on the canvas, and to drag it there.
 */
export class NeighbourhoodPanel {
  /** The entity at the top of the tree. */
  private root: string | null = null;
  /** Keep the root when the canvas selection changes. */
  private pinned = false;
  /** Relation types switched off; empty means every type. */
  private readonly hiddenTypes = new Set<string>();
  private readonly dirs = new Set<Dir>(["out", "in"]);
  /** Opened branches, by path from the root. */
  private readonly open = new Set<string>();
  /** Set while this panel selects on the canvas, so the root does not follow. */
  private selecting = false;

  constructor(
    private readonly head: HTMLElement,
    private readonly body: HTMLElement,
    private readonly editor: DiagramEditor,
  ) {}

  /** The canvas selection moved. Follow it unless pinned or we moved it. */
  followSelection(): void {
    // Several boxes selected is a batch being moved, not a new entity to look at.
    if (this.pinned || this.selecting || this.editor.canvas.selectedIds.size > 1) return;
    const id = this.selectedEntity();
    if (id !== null && id !== this.root) this.setRoot(id);
    // Opening a view replaces the model and announces only a selection change:
    // redraw anyway, or the panel keeps showing the model that is gone.
    else this.reset();
  }

  /** A new project or view: the old root may not exist any more. */
  reset(): void {
    const doc = this.editor.canvas.model;
    if (this.root !== null && !doc?.entities.some((e) => e.id === this.root)) {
      this.root = null;
      this.pinned = false;
      this.open.clear();
    }
    if (this.root === null) this.root = this.selectedEntity();
    this.render();
  }

  private setRoot(id: string): void {
    this.root = id;
    this.open.clear();
    // Show the first ring at once: nobody picks an entity to see a closed box.
    for (const g of this.groupsOf(id)) this.open.add(this.groupKey("", g));
    this.render();
  }

  private selectedEntity(): string | null {
    const doc = this.editor.canvas.model;
    const sel = this.editor.canvas.selected;
    if (!doc || !sel || sel.kind === "edge") return null;
    const element = doc.element(sel.id);
    if (!element) return null;
    const id = entityOf(element)?.id ?? element.id;
    return doc.entities.some((e) => e.id === id) ? id : null;
  }

  render(): void {
    const doc = this.editor.canvas.model;
    const t = i18n.d.panels.neighbourhood;
    if (!doc) {
      replaceChildren(this.head);
      replaceChildren(this.body, el("div", { class: "inspector-empty", text: t.emptyNoModel }));
      return;
    }
    const root = this.root === null ? undefined : this.entity(this.root);
    if (!root) {
      replaceChildren(this.head);
      replaceChildren(
        this.body,
        el("div", { class: "inspector-empty" }, [
          el("p", { class: "inspector-empty-icon", text: "🕸️" }),
          el("p", { text: t.empty }),
        ]),
      );
      return;
    }

    this.renderHead(root);
    const tree = el("div", { class: "nb-tree" }, [
      this.row(root, "", 0, true),
      ...this.children(root.id, "", 1, new Set([root.id])),
    ]);
    replaceChildren(this.body, tree);
  }

  private renderHead(root: EntityEntry): void {
    const t = i18n.d.panels.neighbourhood;
    const types = this.presentTypes();

    const chip = (label: string, active: boolean, title: string, onClick: () => void): HTMLElement =>
      el("button", {
        type: "button",
        class: `kind-chip${active ? " is-active" : ""}`,
        title,
        on: { click: onClick },
      }, [el("span", { text: label })]);

    const pin = el("button", {
      type: "button",
      class: `nb-pin${this.pinned ? " is-active" : ""}`,
      text: "📌",
      title: this.pinned ? t.unpin : t.pin,
      on: { click: () => { this.pinned = !this.pinned; this.render(); } },
    });

    const dirChip = (dir: Dir, label: string, title: string) =>
      chip(label, this.dirs.has(dir), title, () => {
        // Never both off: that would be an empty tree with no way to tell why.
        if (this.dirs.has(dir) && this.dirs.size > 1) this.dirs.delete(dir);
        else this.dirs.add(dir);
        this.render();
      });

    replaceChildren(
      this.head,
      el("div", { class: "nb-root" }, [
        pin,
        el("span", { class: "nb-root-name", text: this.name(root) }),
        el("span", { class: "mono muted nb-kind", text: root.kind }),
      ]),
      el("div", { class: "kind-chips" }, [
        ...types.map((type) =>
          chip(type, !this.hiddenTypes.has(type), t.typeHint, () => {
            if (this.hiddenTypes.has(type)) this.hiddenTypes.delete(type);
            else this.hiddenTypes.add(type);
            this.render();
          }),
        ),
      ]),
      el("div", { class: "kind-chips" }, [
        dirChip("out", t.outgoing, t.outgoingHint),
        dirChip("in", t.incoming, t.incomingHint),
      ]),
    );
  }

  /** The rows under one entity: its groups, and under each open group its neighbours. */
  private children(id: string, path: string, depth: number, onPath: Set<string>): HTMLElement[] {
    const rows: HTMLElement[] = [];
    for (const g of this.groupsOf(id)) {
      const key = this.groupKey(path, g);
      const isOpen = this.open.has(key);
      rows.push(this.groupRow(g, key, depth, isOpen));
      if (!isOpen) continue;
      for (const n of g.entities) {
        const nPath = `${key}/${n.id}`;
        const cycle = onPath.has(n.id);
        rows.push(this.row(n, nPath, depth + 1, false, cycle));
        if (!cycle && this.open.has(nPath)) {
          rows.push(...this.children(n.id, nPath, depth + 2, new Set([...onPath, n.id])));
        }
      }
    }
    return rows;
  }

  private groupRow(g: Group, key: string, depth: number, isOpen: boolean): HTMLElement {
    const t = i18n.d.panels.neighbourhood;
    const unplaced = g.entities.filter((e) => !this.isPlaced(e.id));
    return el("div", { class: "nb-row nb-group", attrs: { style: `padding-left: ${8 + depth * 14}px` } }, [
      el("button", {
        type: "button",
        class: "nb-twisty",
        text: isOpen ? "▾" : "▸",
        on: { click: () => this.toggle(key) },
      }),
      el("span", {
        class: "nb-group-label",
        text: this.groupLabel(g),
        title: `${g.type} ${g.dir === "out" ? "→" : "←"}`,
        on: { click: () => this.toggle(key) },
      }),
      el("span", { class: "kind-chip-count", text: String(g.entities.length) }),
      unplaced.length > 0
        ? el("button", {
            type: "button",
            class: "nb-add",
            text: `+${unplaced.length}`,
            title: t.addGroup,
            on: { click: () => this.place(unplaced) },
          })
        : el("span", { class: "nb-placed", text: "✓", title: t.allPlaced }),
    ]);
  }

  private row(
    e: EntityEntry,
    path: string,
    depth: number,
    isRoot: boolean,
    cycle = false,
  ): HTMLElement {
    const t = i18n.d.panels.neighbourhood;
    const placed = this.isPlaced(e.id);
    const expandable = !isRoot && !cycle && this.groupsOf(e.id).length > 0;
    const isOpen = this.open.has(path);

    const row = el("div", {
      class: `nb-row nb-entity${isRoot ? " is-root" : ""}${placed ? " is-placed" : ""}`,
      title: placed ? t.selectHint : t.dragHint,
      attrs: { style: `padding-left: ${8 + depth * 14}px` },
      on: {
        click: () => this.reveal(e.id),
        dblclick: () => { if (!isRoot) this.setRoot(e.id); },
      },
    }, [
      expandable
        ? el("button", {
            type: "button",
            class: "nb-twisty",
            text: isOpen ? "▾" : "▸",
            on: { click: (ev) => { ev.stopPropagation(); this.toggle(path, e.id); } },
          })
        : el("span", { class: "nb-twisty" }),
      el("span", { class: "nb-name", text: this.name(e) }),
      el("span", { class: "mono muted nb-kind", text: e.kind }),
      cycle ? el("span", { class: "nb-cycle", text: "↺", title: t.cycle }) : null,
      !isRoot
        ? el("button", {
            type: "button",
            class: "nb-reroot",
            text: "⤴",
            title: t.reroot,
            on: { click: (ev) => { ev.stopPropagation(); this.setRoot(e.id); } },
          })
        : null,
      placed
        ? el("span", { class: "nb-placed", text: "✓", title: t.placed })
        : el("button", {
            type: "button",
            class: "nb-add",
            text: "+",
            title: t.add,
            on: { click: (ev) => { ev.stopPropagation(); this.place([e]); } },
          }),
    ]);

    row.draggable = !placed;
    row.addEventListener("dragstart", (ev) => {
      const texts = this.editor.canvas.model?.bundle?.text?.entries || {};
      ev.dataTransfer?.setData("application/semaps-entity", JSON.stringify({
        id: e.id,
        entity: e,
        textEntry: texts[e.id],
      }));
    });
    return row;
  }

  /** Open or close a branch. Opening an entity opens its groups too: one click, one ring. */
  private toggle(key: string, entity?: string): void {
    if (this.open.has(key)) {
      for (const k of [...this.open]) if (k === key || k.startsWith(key + "/")) this.open.delete(k);
    } else {
      this.open.add(key);
      if (entity !== undefined) for (const g of this.groupsOf(entity)) this.open.add(this.groupKey(key, g));
    }
    this.render();
  }

  /** Select the entity's box on the canvas, if it has one, without moving the root. */
  private reveal(id: string): void {
    if (!this.isPlaced(id)) return;
    this.selecting = true;
    try {
      this.editor.canvas.select(id);
    } finally {
      this.selecting = false;
    }
  }

  private place(entities: EntityEntry[]): void {
    const placed = placeEntities(this.editor, entities);
    if (placed.length > 0) {
      // Selected together so the batch can be dragged aside at once; the root stays.
      this.selecting = true;
      try {
        this.editor.canvas.selectMany(placed);
      } finally {
        this.selecting = false;
      }
    }
    this.render();
  }

  // ------------------------------------------------------------------ data

  private groupsOf(id: string): Group[] {
    const doc = this.editor.canvas.model;
    return doc ? armsOf(doc, id, { hiddenTypes: this.hiddenTypes, dirs: this.dirs }) : [];
  }

  private presentTypes(): string[] {
    const doc = this.editor.canvas.model;
    const types = new Set<string>();
    for (const r of (doc?.relations ?? []) as RelationEntry[]) if (r?.type) types.add(r.type);
    return [...types].sort();
  }

  private groupKey(path: string, g: Group): string {
    return `${path}/${g.type}|${g.dir}`;
  }

  /** Words for the arms we know; any other type reads as itself with an arrow. */
  private groupLabel(g: Group): string {
    const names = i18n.d.panels.neighbourhood.arms as Record<string, { out: string; in: string } | undefined>;
    const known = names[g.type];
    if (known) return known[g.dir];
    return g.dir === "out" ? `${g.type} →` : `← ${g.type}`;
  }

  private entity(id: string): EntityEntry | undefined {
    return this.editor.canvas.model?.entities.find((e) => e.id === id);
  }

  private name(e: EntityEntry): string {
    const doc = this.editor.canvas.model;
    return doc ? nameOf(doc, e) : e.name;
  }

  private isPlaced(id: string): boolean {
    return this.editor.canvas.model?.element(id) !== undefined;
  }
}
