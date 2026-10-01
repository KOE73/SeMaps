import type { DiagramCanvas } from "../canvas/DiagramCanvas.js";
import { StyleLibrary, type StyleListEntry } from "../model/StyleLibrary.js";
import type { StyleTarget } from "../model/style-types.js";
import { KindCatalog } from "../model/KindCatalog.js";
import { el, replaceChildren } from "../util/dom.js";
import { iconEl } from "../ui/icons.js";
import { blockPreview, edgePreview } from "./style-preview.js";
import { variantLabel } from "./kindSelects.js";
import { i18n } from "../workbench/i18n/I18nService.js";

/**
 * What the two style panels need from the application.
 *
 * Deliberately the same shape as `InspectorHost`: a style edit is an edit like
 * any other, and routing it through the host's own history mechanics is what
 * keeps Ctrl+Z working across it. A panel that owned its own undo stack would
 * be a second history the user has to know about.
 */
export interface StylePanelHost {
  readonly canvas: DiagramCanvas;
  readonly styles: StyleLibrary;
  /**
   * A field edit inside one style. Coalesced with neighbouring edits into a
   * single history step, exactly as typing into the inspector is.
   */
  editStyle(apply: () => void): void;
  /**
   * A structural change — create, clone, rename, delete. Always its own step:
   * these are not a burst of typing and must not be swallowed by one.
   */
  commitStyle(apply: () => void): void;
  /** Show this style in the editor and highlight its row. */
  openStyle(id: string | null): void;
  /** The one way to set kind and style on blocks and containers (ADR_20260927-7). */
  applyKindAndStyle(ids: readonly string[], kind: string, styleId: string | null): void;
  /** The one way to set relation type and style on lines (ADR_20260930-2). */
  applyRelationTypeAndStyle(ids: readonly string[], type: string, styleId: string | null): void;
  notify(message: string): void;
}

/** Which tree is shown: entity kinds (blocks and containers) or relation types (lines). */
type Mode = "kinds" | "edge";

/** Sections outside the dictionary's groups: styles of no type, and the fallbacks. */
const NO_KIND = "@nokind";
const FALLBACK = "@fallback";
const UNKNOWN = "@unknown";

/** One type in the tree — an entity kind or a relation type — with the styles listed under it. */
interface TypeNode {
  readonly id: string;
  readonly name: string;
  readonly description: string;
  /** `block`, `container` or `edge`: the styles a placement of this type may wear. */
  readonly target: StyleTarget;
  /** The type's base style, marked as such (CONTRACT.md §11.5). */
  readonly base: string | null;
  readonly styles: readonly StyleListEntry[];
}

interface GroupNode {
  readonly id: string;
  readonly name: string;
  readonly types: readonly TypeNode[];
}

/**
 * The catalogue of named styles, by what they are for.
 *
 * A style belongs to a type (ADR_20260927-7, ADR_20260930-2), so the list is a
 * tree: group of the dictionary → type → the styles of that type (their own
 * `forKinds`; a style of several types is listed under each), the type's base
 * style first, marked. Blocks and containers are one tree of entity kinds;
 * lines are the same tree over relation types. A style of no type is not lost:
 * it waits under «Без типа» until it is given one; `default.node`,
 * `default.container` and `default.edge` sit under «Запасные».
 *
 * A style row under a type can be applied — that type and that style — to the
 * canvas selection, through the one mechanism, `applyKindAndStyle` or
 * `applyRelationTypeAndStyle`.
 *
 * Every row shows what the style actually looks like, and the usage count next
 * to it answers the question that decides whether an edit is safe — "how much
 * of the diagram am I about to repaint".
 */
export class StyleList {
  private mode: Mode = "kinds";
  private filter = "";
  private activeId: string | null = null;
  /** A type id, or `NO_KIND` / `FALLBACK`: what a new style is created for. */
  private selectedKind: string | null = null;
  private readonly openGroups = new Set<string>();
  private readonly openKinds = new Set<string>();
  /** Re-evaluate every «Применить» against the current selection. */
  private applyRefreshers: Array<() => void> = [];

  constructor(
    private readonly mount: HTMLElement,
    private readonly host: StylePanelHost,
  ) {
    host.canvas.events.on("select", () => {
      for (const refresh of this.applyRefreshers) refresh();
    });
  }

  get shownTarget(): StyleTarget {
    return this.mode === "edge" ? "edge" : "block";
  }

  /** Follow the editor's selection, and re-read the library. */
  setActive(id: string | null): void {
    this.activeId = id;
    if (id !== null) {
      const style = this.host.styles.get(id);
      if (style !== undefined) {
        this.switchMode(style.appliesTo === "edge" ? "edge" : "kinds");
        this.reveal(id);
      }
    }
    this.render();
  }

  render(): void {
    this.applyRefreshers = [];
    const usage = this.usageCounts();

    const rows = el("div", { class: "style-rows" });
    const foot = el("div", { class: "muted style-list-foot" });

    const filterInput = el("input", {
      class: "style-filter",
      type: "text",
      value: this.filter,
      placeholder: i18n.d.panels.styles.filterPlaceholder,
      on: {
        input: (e) => {
          this.filter = (e.target as HTMLInputElement).value;
          // Only the rows are rebuilt: a full render would recreate the field
          // being typed into and drop the caret after every character.
          this.renderRows(rows, foot, usage);
        },
      },
    });

    this.renderRows(rows, foot, usage);

    const shape = this.newStyleShape();
    replaceChildren(
      this.mount,
      el("div", { class: "style-list-head" }, [
        el("div", { class: "tab-row segmented" }, [
          this.modeButton("kinds", i18n.d.panels.styles.blocksTab),
          this.modeButton("edge", i18n.d.panels.styles.edgesTab),
        ]),
        el("button", {
          class: "btn btn-small btn-primary",
          title: this.createTitle(),
          disabled: shape === null,
          on: { click: () => this.create() },
        }, [iconEl("plus"), i18n.d.panels.styles.addStyle]),
      ]),
      filterInput,
      rows,
      foot,
    );
  }

  // ------------------------------------------------------------------ rows

  private renderRows(
    host: HTMLElement,
    foot: HTMLElement,
    usage: ReadonlyMap<string, number>,
  ): void {
    const lib = this.host.styles;
    const t = i18n.d.panels.styles;
    const filtering = this.filter.trim() !== "";
    const needle = this.filter.trim().toLowerCase();
    const shown = new Set<string>();
    const out: HTMLElement[] = [];

    for (const group of this.tree()) {
      // Filtering shows only types with a matching style, or whose own name matches.
      const types = group.types
        .map((type) => {
          if (!filtering) return type;
          const typeMatches = `${type.id} ${type.name}`.toLowerCase().includes(needle);
          return typeMatches ? type : { ...type, styles: type.styles.filter((s) => this.matches(s)) };
        })
        .filter((type) => !filtering || type.styles.length > 0 || `${type.id} ${type.name}`.toLowerCase().includes(needle));
      if (types.length === 0) continue;

      const groupOpen = filtering || this.openGroups.has(group.id);
      const count = new Set(types.flatMap((k) => k.styles.map((s) => s.id))).size;
      out.push(this.header("group", group.name, count, groupOpen, false, () => this.toggle(this.openGroups, group.id)));
      if (!groupOpen) continue;

      for (const type of types) {
        const typeOpen = filtering || this.openKinds.has(type.id);
        out.push(this.header("kind", type.name, type.styles.length, typeOpen, this.selectedKind === type.id, () => {
          this.selectedKind = type.id;
          this.toggle(this.openKinds, type.id);
        }, type.id, type.description));
        if (!typeOpen) continue;
        if (type.styles.length === 0) {
          out.push(el("div", { class: "muted italic style-tree-empty", text: t.kindNoStyles }));
        }
        for (const style of type.styles) {
          shown.add(style.id);
          out.push(this.styleRow(style, usage, style.id === type.base, type.id));
        }
      }
    }

    const sort = this.sortTargets();
    const ofSort = sort.flatMap((target) => lib.list(target, this.filter));
    const sections: Array<[string, string, string, StyleListEntry[]]> = [
      [NO_KIND, t.noKind, t.noKindTitle, ofSort.filter((s) => lib.isKindless(s.id))],
      [FALLBACK, t.fallbackStyles, t.fallbackStylesTitle, ofSort.filter((s) => StyleLibrary.isFallback(s.id))],
    ];
    for (const [key, name, title, styles] of sections) {
      // «Без типа» exists only while some style has no type.
      if (styles.length === 0 && (filtering || key === NO_KIND)) continue;
      const open = filtering || this.openKinds.has(key);
      out.push(this.header("group", name, styles.length, open, this.selectedKind === key, () => {
        this.selectedKind = key;
        this.toggle(this.openKinds, key);
      }, undefined, title));
      if (!open) continue;
      for (const style of styles) {
        shown.add(style.id);
        out.push(this.styleRow(style, usage, false, null));
      }
    }

    const total = sort.reduce((n, target) => n + lib.list(target).length, 0);
    foot.textContent = filtering
      ? i18n.format(t.shownCount, { shown: shown.size, total })
      : i18n.format(t.totalCount, { total });
    replaceChildren(host, ...(out.length > 0 ? out : [el("div", { class: "muted italic style-empty", text: t.empty })]));
  }

  /** The `appliesTo` values this tree lists. */
  private sortTargets(): StyleTarget[] {
    return this.mode === "edge" ? ["edge"] : ["block", "container"];
  }

  /**
   * Groups and types of the dictionary with their styles, plus a group for
   * types that styles name in `forKinds` but the dictionary does not know.
   */
  private tree(): GroupNode[] {
    const lib = this.host.styles;
    const catalog = KindCatalog.active;
    const lang = i18n.currentLanguage;
    const all = this.sortTargets().flatMap((target) => lib.list(target));

    const node = (id: string, target: StyleTarget, name: string, description: string): TypeNode => {
      const found = lib.baseStyleOf(id, target);
      const base = found !== null && !StyleLibrary.isFallback(found) ? found : null;
      const own = all.filter((s) => s.appliesTo === target && lib.forKindsOf(s.id).includes(id) && s.id !== base);
      const baseEntry = base === null ? undefined : all.find((s) => s.id === base);
      return {
        id, name, description, target,
        base: baseEntry === undefined ? null : base,
        styles: baseEntry === undefined ? own : [baseEntry, ...own],
      };
    };

    const groups: GroupNode[] =
      this.mode === "edge"
        ? catalog.relationGroups().map((group) => ({
            id: group.id,
            name: catalog.groupName(group, lang),
            types: group.types.map((t) => node(t.id, "edge", catalog.relationName(t.id, lang), catalog.relationDescription(t.id, lang))),
          }))
        : catalog.groups().map((group) => ({
            id: group.id,
            name: catalog.groupName(group, lang),
            types: group.kinds.map((k) =>
              node(k.id, k.container === true ? "container" : "block", catalog.name(k.id, lang), catalog.description(k.id, lang))),
          }));

    const known = (id: string): boolean => (this.mode === "edge" ? catalog.lookupRelation(id) : catalog.lookup(id)) !== undefined;
    const unknown = new Map<string, StyleTarget>();
    for (const s of all) {
      for (const type of lib.forKindsOf(s.id)) {
        if (!known(type) && !unknown.has(type)) unknown.set(type, s.appliesTo);
      }
    }
    if (unknown.size > 0) {
      groups.push({
        id: UNKNOWN,
        name: i18n.d.panels.styles.kindsOutsideCatalog,
        types: [...unknown].sort(([a], [b]) => a.localeCompare(b)).map(([id, target]) => node(id, target, id, "")),
      });
    }
    return groups;
  }

  private matches(entry: StyleListEntry): boolean {
    const needle = this.filter.trim().toLowerCase();
    const s = entry.style;
    return [s.id, s.name ?? "", s.description ?? "", ...(s.tags ?? []), ...(s.forKinds ?? [])].join(" ").toLowerCase().includes(needle);
  }

  /** A group or type header: chevron, name, count. */
  private header(
    level: "group" | "kind",
    name: string,
    count: number,
    open: boolean,
    selected: boolean,
    onClick: () => void,
    id?: string,
    description?: string,
  ): HTMLElement {
    return el("div", {
      class: `style-tree-${level}${selected ? " is-selected" : ""}`,
      title: description ?? "",
      on: { click: onClick },
    }, [
      el("span", { class: "style-tree-chevron" }, [iconEl(open ? "chevronDown" : "chevronRight")]),
      el("span", { class: "style-tree-name", text: name }),
      id !== undefined && id !== name ? el("span", { class: "mono muted style-tree-id", text: id }) : null,
      el("span", { class: `style-row-count${count === 0 ? " is-zero" : ""}`, text: String(count) }),
    ]);
  }

  /**
   * One style: preview, name, and a dense 2×2 of actions — apply, copy,
   * delete, usage count. `type` is the tree's type the row sits under; only
   * such a row can be applied, since applying sets the type too.
   */
  private styleRow(
    entry: StyleListEntry,
    usage: ReadonlyMap<string, number>,
    isBase: boolean,
    type: string | null,
  ): HTMLElement {
    const lib = this.host.styles;
    const t = i18n.d.panels.styles;
    const count = usage.get(entry.id) ?? 0;
    const preview = entry.appliesTo === "edge"
      ? edgePreview(lib.resolveEdge(entry.id))
      : blockPreview(lib.resolveBlock(entry.id), { container: entry.appliesTo === "container" });

    return el(
      "div",
      {
        class: `style-row style-tree-leaf${entry.id === this.activeId ? " is-active" : ""}`,
        title: entry.style.description ?? "",
        on: { click: () => this.host.openStyle(entry.id) },
      },
      [
        el("div", { class: "style-row-preview" }, [preview]),
        el("div", { class: "style-row-text" }, [
          el("span", { class: "style-row-name", text: variantLabel(entry.name, isBase), title: isBase ? t.baseStyleTitle : "" }),
          el("span", { class: "mono muted style-row-id", text: entry.id }),
        ]),
        el("div", { class: "style-row-actions" }, [
          this.applyButton(entry, type),
          el("button", {
            class: "btn-icon",
            title: t.cloneTooltip,
            on: {
              click: (e) => {
                e.stopPropagation();
                this.cloneStyle(entry.id);
              },
            },
          }, [iconEl("copy")]),
          el("button", {
            class: "btn-icon danger",
            title: t.deleteTooltip,
            on: {
              click: (e) => {
                e.stopPropagation();
                this.removeStyle(entry.id, count);
              },
            },
          }, [iconEl("trash")]),
          el("span", {
            class: `style-row-count${count === 0 ? " is-zero" : ""}`,
            text: String(count),
            title: i18n.format(t.elementsCountTooltip, { count }),
          }),
        ]),
      ],
    );
  }

  /**
   * «Применить»: the tree's type and this style, on the selected objects of the
   * style's sort (block, container or line). Kept in step with the selection
   * without re-rendering the list.
   */
  private applyButton(entry: StyleListEntry, type: string | null): HTMLButtonElement {
    const t = i18n.d.panels.styles;
    const button = el("button", { class: "btn-icon" }, [iconEl("circleCheck")]) as HTMLButtonElement;
    const targets = (): string[] => {
      const doc = this.host.canvas.model;
      if (doc === null) return [];
      return [...this.host.canvas.selectedIds].filter((id) => {
        if (entry.appliesTo === "edge") return doc.edge(id) !== undefined || doc.relations.some((r) => r.id === id);
        const element = doc.element(id);
        return element !== undefined && StyleLibrary.targetOfElement(element) === entry.appliesTo;
      });
    };
    const name = (): string =>
      type === null ? "" : entry.appliesTo === "edge"
        ? KindCatalog.active.relationName(type, i18n.currentLanguage)
        : KindCatalog.active.name(type, i18n.currentLanguage);
    const refresh = (): void => {
      const selected = this.host.canvas.selectedIds.size;
      const ids = type === null ? [] : targets();
      button.disabled = ids.length === 0;
      button.title = type === null
        ? t.applyNoKind
        : selected === 0
          ? t.applyNoSelection
          : ids.length === 0
            ? t.applyWrongTarget
            : i18n.format(t.applyTooltip, { kind: name() });
    };
    button.addEventListener("click", (e) => {
      e.stopPropagation();
      const ids = targets();
      if (type === null || ids.length === 0) return;
      if (entry.appliesTo === "edge") this.host.applyRelationTypeAndStyle(ids, type, entry.id);
      else this.host.applyKindAndStyle(ids, type, entry.id);
      this.render();
    });
    refresh();
    this.applyRefreshers.push(refresh);
    return button;
  }

  private toggle(set: Set<string>, id: string): void {
    if (set.has(id)) set.delete(id);
    else set.add(id);
    this.render();
  }

  /** Open the group and type where a style is listed, so the active row is visible. */
  private reveal(id: string): void {
    const catalog = KindCatalog.active;
    const lib = this.host.styles;
    const target = lib.targetOf(id);
    const forKinds = lib.forKindsOf(id);
    const edge = target === "edge";
    const listed = edge
      ? catalog.relationGroups().flatMap((g) => g.types.map((t) => t.id))
      : catalog.groups().flatMap((g) => g.kinds.map((k) => k.id));
    // A style is listed under each type it names; the first is enough to show it.
    const type = forKinds[0]
      ?? (StyleLibrary.isFallback(id) ? undefined : listed.find((key) => lib.baseStyleOf(key, target) === id));
    if (type === undefined) {
      this.openKinds.add(StyleLibrary.isFallback(id) ? FALLBACK : NO_KIND);
      return;
    }
    const group = edge ? catalog.relationGroupOf(type) : catalog.groupOf(type);
    this.openGroups.add(group?.id ?? UNKNOWN);
    this.openKinds.add(type);
  }

  private switchMode(mode: Mode): void {
    if (this.mode === mode) return;
    this.mode = mode;
    // What was selected in the other tree means nothing in this one.
    this.selectedKind = null;
  }

  private modeButton(mode: Mode, label: string): HTMLElement {
    return el("button", {
      class: `tab${this.mode === mode ? " is-active" : ""}`,
      text: label,
      on: {
        click: () => {
          this.switchMode(mode);
          this.render();
        },
      },
    });
  }

  // -------------------------------------------------------------- commands

  /**
   * What a new style is for: the selected type. A style belongs to a type and
   * has none without one (ADR_20260927-7), so with no type open there is
   * nothing to create it for — `null`.
   */
  private newStyleShape(): { target: StyleTarget; kind: string } | null {
    const selected = this.selectedKind;
    if (selected === null || selected === NO_KIND || selected === FALLBACK) return null;
    const catalog = KindCatalog.active;
    const target: StyleTarget = this.mode === "edge"
      ? "edge"
      : catalog.lookup(selected) !== undefined
        ? (catalog.isContainer(selected) ? "container" : "block")
        : this.tree().flatMap((g) => g.types).find((k) => k.id === selected)?.target ?? "block";
    return { target, kind: selected };
  }

  private createTitle(): string {
    const shape = this.newStyleShape();
    if (shape === null) return i18n.d.panels.styles.addStyleNeedsKind;
    const catalog = KindCatalog.active;
    const name = shape.target === "edge"
      ? catalog.relationName(shape.kind, i18n.currentLanguage)
      : catalog.name(shape.kind, i18n.currentLanguage);
    return i18n.format(i18n.d.panels.styles.addStyleForKind, { kind: name });
  }

  private create(): void {
    const lib = this.host.styles;
    const shape = this.newStyleShape();
    if (shape === null) {
      this.host.notify(i18n.d.panels.styles.addStyleNeedsKind);
      return;
    }
    const id = lib.freeId(`${shape.kind}.new`);
    this.host.commitStyle(() => {
      lib.put({
        id,
        name: i18n.d.panels.styles.newStyleDefaultName,
        appliesTo: shape.target,
        // Created while a type is selected: meant for that type. Nothing else —
        // a brand-new style inherits everything, so it looks like the default
        // until the user says otherwise, and the fields they leave alone keep
        // tracking it.
        forKinds: [shape.kind],
      });
    });
    this.host.openStyle(id);
  }

  private cloneStyle(id: string): void {
    const lib = this.host.styles;
    if (!lib.has(id)) return;
    // The id is reserved before the commit rather than read back from `clone`,
    // so the caller knows which style to open without depending on what the
    // library chose inside a callback.
    const newId = lib.freeId(id);
    this.host.commitStyle(() => {
      lib.clone(id, newId);
    });
    this.host.openStyle(newId);
  }

  /**
   * Delete, after saying what breaks.
   *
   * Both consequences are real and different: elements wearing the style fall
   * back to their type or the default, while styles based on it keep their own
   * fields and lose the inherited half. Neither is recoverable by looking at
   * the result, so both are counted before the fact.
   */
  private removeStyle(id: string, count: number): void {
    const lib = this.host.styles;
    const dependents = lib.dependents(id);

    if (count > 0 || dependents.length > 0) {
      const lines = [i18n.format(i18n.d.panels.styles.deleteConfirm, { name: lib.get(id)?.name ?? id }), ""];
      if (count > 0) lines.push(i18n.format(i18n.d.panels.styles.deleteElementsWarning, { count }));
      if (dependents.length > 0) {
        lines.push(i18n.format(i18n.d.panels.styles.deleteDependentsWarning, { count: dependents.length, names: dependents.join(", ") }));
      }
      if (!window.confirm(lines.join("\n"))) return;
    }

    this.host.commitStyle(() => {
      lib.remove(id);
    });
    if (this.activeId === id) this.host.openStyle(null);
    else this.render();
  }

  // ----------------------------------------------------------------- usage

  /**
   * How many elements each style actually dresses.
   *
   * Counted through `blockStyleIdFor` / `edgeStyleIdFor` rather than by looking
   * at `styleId`, because most elements never name a style at all — they wear
   * their type's, and those are exactly the ones an edit will repaint.
   */
  private usageCounts(): Map<string, number> {
    const counts = new Map<string, number>();
    const doc = this.host.canvas.model;
    if (doc === null) return counts;
    const lib = this.host.styles;

    const bump = (id: string | null): void => {
      if (id === null) return;
      counts.set(id, (counts.get(id) ?? 0) + 1);
    };

    if (this.mode === "kinds") {
      for (const element of doc.elements()) bump(lib.blockStyleIdFor(element));
    } else {
      for (const edge of doc.edges) bump(lib.edgeStyleIdFor(edge));
    }
    return counts;
  }
}
