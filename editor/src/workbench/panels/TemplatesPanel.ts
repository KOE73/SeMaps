import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { TemplateList, type TemplatePanelHost } from "../../editor/TemplateList.js";
import { TemplateEditor } from "../../editor/TemplateEditor.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

const TEMPLATE_LIST_WIDTH_KEY = "semaps:template-list-width";
const MIN_LIST_WIDTH = 180;
const MIN_EDITOR_WIDTH = 260;

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/**
 * Tab for editing content templates (ADR_20260903 §2.2/§2.3), docked next to
 * "Стили" — a template is the same kind of shared, named thing a style is,
 * and the panel is laid out the same way on purpose: a list on the left, the
 * live editor on the right.
 */
export class TemplatesPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly listMount: HTMLElement;
  private readonly editorMount: HTMLElement;
  private readonly resizer: HTMLElement;
  private readonly list: TemplateList;
  private readonly editor: TemplateEditor;

  constructor(editor: DiagramEditor) {
    this.listMount = el("div", {
      class: "style-list",
      attrs: { style: "width: 220px; flex-shrink: 0; display: flex; flex-direction: column; overflow: hidden; border-right: 1px solid var(--border);" },
    });

    this.resizer = el("div", {
      class: "style-pane-resizer",
      title: i18n.d.panels.templates.resizerTitle,
    });

    this.editorMount = el("div", {
      class: "style-editor",
      attrs: { style: "flex: 1; min-width: 0; overflow-y: auto; overflow-x: hidden; height: 100%;" },
    });

    const body = el(
      "div",
      {
        class: "inspector-body styles-pane",
        attrs: { style: "display: flex; flex-direction: row; height: 100%; overflow: hidden; padding: 0;" },
      },
      [this.listMount, this.resizer, this.editorMount],
    );

    this.element = el(
      "div",
      {
        class: "inspector",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [body],
    );

    const host: TemplatePanelHost = {
      get canvas() { return editor.canvas; },
      notify: (msg) => editor.notify(msg),
      openTemplate: (id) => this.openTemplate(id),
    };

    this.list = new TemplateList(this.listMount, host);
    this.editor = new TemplateEditor(this.editorMount, host);

    this.bindResizer();

    i18n.onLanguageChange(() => this.render());
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.render();
  }

  onShow(): void {
    this.render();
  }

  render(): void {
    this.list.render();
    this.editor.render();
  }

  openTemplate(id: string | null): void {
    this.list.setActive(id);
    this.editor.open(id);
  }

  private bindResizer(): void {
    const list = this.listMount;
    const stored = Number(window.localStorage.getItem(TEMPLATE_LIST_WIDTH_KEY));
    if (Number.isFinite(stored) && stored > 0) {
      list.style.width = `${clamp(stored, MIN_LIST_WIDTH, 400)}px`;
    }

    this.resizer.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      const startX = e.clientX;
      const startWidth = list.getBoundingClientRect().width;
      this.resizer.setPointerCapture(e.pointerId);
      this.resizer.classList.add("is-dragging");

      const onMove = (move: PointerEvent): void => {
        const maxWidth = this.element.getBoundingClientRect().width - MIN_EDITOR_WIDTH;
        const width = clamp(startWidth + (move.clientX - startX), MIN_LIST_WIDTH, Math.max(MIN_LIST_WIDTH, maxWidth));
        list.style.width = `${width}px`;
      };

      const onUp = (): void => {
        this.resizer.classList.remove("is-dragging");
        this.resizer.releasePointerCapture(e.pointerId);
        this.resizer.removeEventListener("pointermove", onMove);
        this.resizer.removeEventListener("pointerup", onUp);
        window.localStorage.setItem(TEMPLATE_LIST_WIDTH_KEY, list.getBoundingClientRect().width.toFixed(0));
      };

      this.resizer.addEventListener("pointermove", onMove);
      this.resizer.addEventListener("pointerup", onUp);
    });
  }
}
