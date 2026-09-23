import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { BasePanel as SemapsBasePanel } from "../../editor/BasePanel.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class BasePanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly searchInput: HTMLInputElement;
  private readonly listSlot: HTMLElement;
  private readonly basePanel: SemapsBasePanel;

  constructor(editor: DiagramEditor) {
    this.searchInput = el("input", {
      type: "text",
      class: "input-strong",
      attrs: { style: "width: 100%;", placeholder: i18n.d.panels.base.searchPlaceholder },
    }) as HTMLInputElement;

    this.listSlot = el("div", {
      class: "base-panel-list",
      attrs: { style: "flex: 1; overflow-y: auto;" },
    });

    const head = el("div", {
      class: "base-panel-head",
      attrs: { style: "padding: 10px; border-bottom: 1px solid var(--border); flex-shrink: 0;" },
    }, [this.searchInput]);

    this.element = el(
      "div",
      {
        class: "inspector-body base-pane",
        attrs: { style: "display: flex; flex-direction: column; overflow: hidden; padding: 0; height: 100%; width: 100%; background: var(--panel);" },
      },
      [head, this.listSlot],
    );

    this.basePanel = new SemapsBasePanel(this.searchInput, this.listSlot, editor);

    editor.canvas.events.on("modelchange", () => {
      this.basePanel.render();
    });

    i18n.onLanguageChange(() => {
      this.searchInput.placeholder = i18n.d.panels.base.searchPlaceholder;
      this.basePanel.render();
    });
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.basePanel.render();
  }

  onShow(): void {
    this.basePanel.render();
  }
}
