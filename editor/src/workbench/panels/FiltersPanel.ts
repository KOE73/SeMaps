import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { FiltersPanel as SemapsFiltersPanel } from "../../editor/FiltersPanel.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class FiltersPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly body: HTMLElement;
  private readonly filters: SemapsFiltersPanel;

  constructor(editor: DiagramEditor) {
    this.body = el("div", {
      class: "inspector-body filters-pane",
      attrs: { style: "display: flex; flex-direction: column; overflow-y: auto; height: 100%; padding: 0;" },
    });

    this.element = el(
      "div",
      {
        class: "inspector",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [this.body],
    );

    this.filters = new SemapsFiltersPanel(this.body, editor);

    editor.canvas.events.on("modelchange", () => {
      this.filters.render();
    });

    i18n.onLanguageChange(() => {
      this.filters.render();
    });
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.filters.render();
  }

  onShow(): void {
    this.filters.render();
  }
}
