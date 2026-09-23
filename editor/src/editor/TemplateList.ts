import type { DiagramCanvas } from "../canvas/DiagramCanvas.js";
import type { WireTemplate } from "../content/TemplateLibrary.js";
import { el, replaceChildren } from "../util/dom.js";
import { i18n } from "../workbench/i18n/I18nService.js";

/**
 * What the templates panel needs from the application.
 *
 * Mirrors `StylePanelHost` on purpose: a template is a named, shared thing an
 * element can wear, exactly like a style, and the two panels should feel like
 * the same kind of surface.
 */
export interface TemplatePanelHost {
  readonly canvas: DiagramCanvas;
  notify(message: string): void;
  /** Show this template in the editor and highlight its row. */
  openTemplate(id: string | null): void;
}

/**
 * The catalogue of named content templates (ADR_20260903 §2.2/§2.5).
 *
 * Read straight from `TemplateLibrary.list()` on every render rather than
 * cached here: the library is the live, in-memory draft the editor pane keeps
 * mutating on every keystroke, and this list has to reflect that immediately,
 * not just after a save.
 */
export class TemplateList {
  private filter = "";
  private activeId: string | null = null;

  constructor(
    private readonly mount: HTMLElement,
    private readonly host: TemplatePanelHost,
  ) {}

  setActive(id: string | null): void {
    this.activeId = id;
    this.render();
  }

  render(): void {
    const rows = el("div", { class: "style-rows" });
    const foot = el("div", { class: "muted style-list-foot" });

    const filterInput = el("input", {
      class: "style-filter",
      type: "text",
      value: this.filter,
      placeholder: i18n.d.panels.templates.filterPlaceholder,
      on: {
        input: (e) => {
          this.filter = (e.target as HTMLInputElement).value;
          this.renderRows(rows, foot);
        },
      },
    });

    this.renderRows(rows, foot);

    replaceChildren(
      this.mount,
      el("div", { class: "style-list-head" }, [
        el("div", { class: "field-label accent", text: i18n.d.panels.templates.listTitle }),
      ]),
      filterInput,
      rows,
      foot,
    );
  }

  private renderRows(host: HTMLElement, foot: HTMLElement): void {
    const all = this.host.canvas.templates.list();
    const needle = this.filter.trim().toLowerCase();
    const entries = needle === "" ? all : all.filter((t) => matches(t, needle));

    foot.textContent = i18n.format(i18n.d.panels.templates.shownCount, {
      shown: entries.length,
      total: all.length,
    });

    if (entries.length === 0) {
      replaceChildren(
        host,
        el("div", { class: "muted italic style-empty", text: i18n.d.panels.templates.empty }),
      );
      return;
    }

    replaceChildren(
      host,
      ...entries.map((entry) =>
        el(
          "div",
          {
            class: `style-row${entry.id === this.activeId ? " is-active" : ""}`,
            title: entry.description ?? "",
            on: { click: () => this.host.openTemplate(entry.id) },
          },
          [
            el("div", { class: "style-row-text" }, [
              el("span", { class: "style-row-name", text: entry.name ?? entry.id }),
              el("span", { class: "mono muted style-row-id", text: entry.id }),
              entry.description
                ? el("span", { class: "muted template-row-description", text: entry.description })
                : null,
            ]),
          ],
        ),
      ),
    );
  }
}

function matches(entry: WireTemplate, needle: string): boolean {
  return (
    entry.id.toLowerCase().includes(needle) ||
    (entry.name ?? "").toLowerCase().includes(needle) ||
    (entry.description ?? "").toLowerCase().includes(needle)
  );
}
