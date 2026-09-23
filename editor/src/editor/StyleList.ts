import type { DiagramCanvas } from "../canvas/DiagramCanvas.js";
import type { StyleLibrary } from "../model/StyleLibrary.js";
import type { StyleTarget } from "../model/style-types.js";
import { el, replaceChildren } from "../util/dom.js";
import { blockPreview, edgePreview } from "./style-preview.js";
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
  notify(message: string): void;
}

/**
 * The catalogue of named styles.
 *
 * A style's identity is the thing being chosen here, so every row shows what it
 * actually looks like. The usage count next to it answers the question that
 * decides whether an edit is safe — "how much of the diagram am I about to
 * repaint" — which is precisely the question the old per-element colours made
 * unanswerable.
 */
export class StyleList {
  private target: StyleTarget = "block";
  private filter = "";
  private activeId: string | null = null;

  constructor(
    private readonly mount: HTMLElement,
    private readonly host: StylePanelHost,
  ) {}

  get shownTarget(): StyleTarget {
    return this.target;
  }

  /** Follow the editor's selection, and re-read the library. */
  setActive(id: string | null): void {
    this.activeId = id;
    if (id !== null) {
      const style = this.host.styles.get(id);
      if (style?.appliesTo !== undefined) this.target = style.appliesTo;
    }
    this.render();
  }

  render(): void {
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

    replaceChildren(
      this.mount,
      el("div", { class: "style-list-head" }, [
        el("div", { class: "tab-row segmented" }, [
          this.targetButton("block", i18n.d.panels.styles.blocksTab),
          this.targetButton("edge", i18n.d.panels.styles.edgesTab),
        ]),
        el("button", {
          class: "btn btn-small btn-primary",
          text: i18n.d.panels.styles.addStyle,
          title: i18n.d.panels.styles.addStyleTitle,
          on: { click: () => this.create() },
        }),
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
    const entries = lib.list(this.target, this.filter);
    foot.textContent = i18n.format(i18n.d.panels.styles.shownCount, {
      shown: entries.length,
      total: lib.list(this.target).length,
    });

    if (entries.length === 0) {
      replaceChildren(
        host,
        el("div", { class: "muted italic style-empty", text: i18n.d.panels.styles.empty }),
      );
      return;
    }

    replaceChildren(
      host,
      ...entries.map((entry) => {
        const count = usage.get(entry.id) ?? 0;
        const preview =
          this.target === "block"
            ? blockPreview(lib.resolveBlock(entry.id))
            : edgePreview(lib.resolveEdge(entry.id));

        const row = el(
          "div",
          {
            class: `style-row${entry.id === this.activeId ? " is-active" : ""}`,
            title: entry.style.description ?? "",
            on: { click: () => this.host.openStyle(entry.id) },
          },
          [
            el("div", { class: "style-row-preview" }, [preview]),
            el("div", { class: "style-row-text" }, [
              el("span", { class: "style-row-name", text: entry.name }),
              el("span", { class: "mono muted style-row-id", text: entry.id }),
            ]),
            el("span", {
              class: `style-row-count${count === 0 ? " is-zero" : ""}`,
              text: String(count),
              title: i18n.format(i18n.d.panels.styles.elementsCountTooltip, { count }),
            }),
            el("button", {
              class: "btn-icon",
              text: "⧉",
              title: i18n.d.panels.styles.cloneTooltip,
              on: {
                click: (e) => {
                  e.stopPropagation();
                  this.cloneStyle(entry.id);
                },
              },
            }),
            el("button", {
              class: "btn-icon danger",
              text: "✕",
              title: i18n.d.panels.styles.deleteTooltip,
              on: {
                click: (e) => {
                  e.stopPropagation();
                  this.removeStyle(entry.id, count);
                },
              },
            }),
          ],
        );
        return row;
      }),
    );
  }

  private targetButton(target: StyleTarget, label: string): HTMLElement {
    return el("button", {
      class: `tab${this.target === target ? " is-active" : ""}`,
      text: label,
      on: {
        click: () => {
          this.target = target;
          this.render();
        },
      },
    });
  }

  // -------------------------------------------------------------- commands

  private create(): void {
    const lib = this.host.styles;
    const id = lib.freeId(this.target === "block" ? "block.new" : "edge.new");
    this.host.commitStyle(() => {
      lib.put({
        id,
        name: i18n.d.panels.styles.newStyleDefaultName,
        appliesTo: this.target,
        // Nothing else: a brand-new style inherits everything, so it looks like
        // the default until the user says otherwise, and the fields they leave
        // alone keep tracking it.
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
   * at `styleId`, because most elements never name a style at all — they match
   * one by `type`, and those are exactly the ones an edit will repaint.
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

    if (this.target === "block") {
      for (const element of doc.elements()) bump(lib.blockStyleIdFor(element));
    } else {
      for (const edge of doc.edges) bump(lib.edgeStyleIdFor(edge));
    }
    return counts;
  }
}
