import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { EdgesPanel } from "../../editor/EdgesPanel.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class RelationsPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly body: HTMLElement;
  private readonly edgesPanel: EdgesPanel;

  constructor(editor: DiagramEditor) {
    this.body = el("div", {
      class: "inspector-body edges-pane",
      attrs: { style: "padding: 12px; overflow-y: auto; height: 100%;" },
    });

    this.element = el(
      "div",
      {
        class: "inspector",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [this.body],
    );

    this.edgesPanel = new EdgesPanel(this.body, editor);

    editor.canvas.events.on("select", () => {
      this.edgesPanel.render();
    });

    editor.canvas.events.on("collapse", () => {
      this.edgesPanel.render();
    });

    i18n.onLanguageChange(() => {
      this.edgesPanel.render();
    });
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.edgesPanel.render();
  }

  onShow(): void {
    this.edgesPanel.render();
  }
}
