import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditorFacade } from "../commands/types.js";
import type { CatalogEntry } from "../../editor/io/types.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class CatalogPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly catalogSlot: HTMLElement;
  private readonly customCatalogSlot: HTMLElement;
  private readonly customSection: HTMLElement;
  private readonly customLabel: HTMLElement;
  private readonly projectLabel: HTMLElement;
  private readonly hintDiv: HTMLElement;
  private readonly openJsonBtn: HTMLElement;

  constructor(
    private readonly editor: DiagramEditorFacade,
    private readonly getCatalog: () => readonly CatalogEntry[],
  ) {
    this.catalogSlot = el("div");
    this.customCatalogSlot = el("div");
    this.customLabel = el("div", { class: "section-label sidebar-label", text: i18n.d.panels.catalog.customSection });
    this.customSection = el("div", { attrs: { hidden: "true" } }, [
      this.customLabel,
      this.customCatalogSlot,
    ]);

    this.projectLabel = el("div", { class: "section-label sidebar-label", text: i18n.d.panels.catalog.projectCatalog });
    const scroll = el("div", { class: "sidebar-scroll", attrs: { style: "flex: 1; overflow-y: auto; padding: 8px;" } }, [
      this.projectLabel,
      this.catalogSlot,
      this.customSection,
    ]);

    this.hintDiv = el("div", { class: "hint sidebar-label", attrs: { style: "padding: 8px; border-top: 1px solid var(--border);" } }, [
      el("div", { text: i18n.d.panels.catalog.hintDragZone }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseZone }),
    ]);

    this.openJsonBtn = el("button", {
      class: "btn full",
      text: i18n.d.panels.catalog.openJsonBtn,
      on: { click: () => editor.openFile() },
    });

    const foot = el("div", { class: "sidebar-foot", attrs: { style: "padding: 8px; border-top: 1px solid var(--border);" } }, [
      this.openJsonBtn,
    ]);

    this.element = el(
      "aside",
      {
        class: "sidebar",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [scroll, this.hintDiv, foot],
    );

    i18n.onLanguageChange(() => {
      this.updateLabels();
      this.render();
    });
  }

  private updateLabels(): void {
    this.customLabel.textContent = i18n.d.panels.catalog.customSection;
    this.projectLabel.textContent = i18n.d.panels.catalog.projectCatalog;
    this.openJsonBtn.textContent = i18n.d.panels.catalog.openJsonBtn;
    replaceChildren(this.hintDiv,
      el("div", { text: i18n.d.panels.catalog.hintDragZone }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseZone }),
    );
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.render();
  }

  onShow(): void {
    this.render();
  }

  render(): void {
    const catalog = this.getCatalog();
    if (catalog.length === 0) {
      replaceChildren(
        this.catalogSlot,
        el("div", { class: "catalog-empty", text: i18n.d.panels.catalog.empty }),
      );
      return;
    }

    replaceChildren(
      this.catalogSlot,
      ...catalog.map((entry) =>
        el(
          "button",
          {
            class: "catalog-item",
            dataset: { catalogId: entry.id },
            on: {
              click: () => {
                void this.editor.openCatalogEntry(entry);
              },
            },
          },
          [
            el("span", { class: `catalog-icon theme-${entry.theme ?? "blue"}`, text: entry.icon ?? "📄" }),
            el("span", { class: "catalog-text sidebar-label" }, [
              el("span", { class: "catalog-title", text: entry.title }),
              el("span", { class: "catalog-subtitle", text: entry.subtitle ?? "" }),
            ]),
          ],
        ),
      ),
    );
  }
}
