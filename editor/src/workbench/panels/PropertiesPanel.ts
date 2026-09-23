import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditorFacade } from "../commands/types.js";
import { Inspector, type InspectorHost } from "../../editor/Inspector.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export class PropertiesPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly badge: HTMLElement;
  private readonly body: HTMLElement;
  private readonly titleEl: HTMLElement;
  private readonly inspector: Inspector;

  constructor(private readonly editor: DiagramEditorFacade & InspectorHost) {
    this.badge = el("span", { class: "badge", text: i18n.d.common.notSelected });
    this.body = el("div", { class: "inspector-body", attrs: { style: "padding: 12px; overflow-y: auto; height: 100%;" } });
    this.titleEl = el("h2", { text: i18n.d.panels.properties.title, attrs: { style: "font-size: 13px; margin: 0;" } });

    const head = el("div", { class: "inspector-head" }, [
      this.titleEl,
      this.badge,
    ]);

    this.element = el("div", { class: "inspector", attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" } }, [
      head,
      this.body,
    ]);

    this.inspector = new Inspector(this.badge, this.body, editor);

    editor.canvas.events.on("select", (selection) => {
      this.inspector.render(selection);
    });

    editor.canvas.events.on("modelchange", () => {
      const element = editor.canvas.selectedElement();
      if (element !== null) this.inspector.updateGeometry(element);
    });

    editor.canvas.events.on("collapse", () => {
      this.inspector.render(editor.canvas.selected);
    });

    i18n.onLanguageChange(() => {
      this.titleEl.textContent = i18n.d.panels.properties.title;
      this.inspector.render(this.editor.canvas.selected);
    });
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.inspector.render(this.editor.canvas.selected);
  }

  onShow(): void {
    this.inspector.render(this.editor.canvas.selected);
  }
}
