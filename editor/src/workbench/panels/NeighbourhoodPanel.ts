import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { NeighbourhoodPanel as SemapsNeighbourhoodPanel } from "../../editor/NeighbourhoodPanel.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class NeighbourhoodPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly panel: SemapsNeighbourhoodPanel;

  constructor(editor: DiagramEditor) {
    const head = el("div", { class: "nb-head" });
    const body = el("div", { class: "nb-body" });
    this.element = el("div", { class: "inspector-body nb-pane" }, [head, body]);

    this.panel = new SemapsNeighbourhoodPanel(head, body, editor);

    editor.canvas.events.on("select", () => this.panel.followSelection());
    editor.canvas.events.on("modelchange", () => this.panel.reset());
    i18n.onLanguageChange(() => this.panel.render());
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.panel.reset();
  }

  onShow(): void {
    this.panel.reset();
  }
}
