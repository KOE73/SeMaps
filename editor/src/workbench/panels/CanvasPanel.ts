import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditorFacade } from "../commands/types.js";
import { el } from "../../util/dom.js";

export class CanvasPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly zoomReadout: HTMLElement;
  private readonly dropHint: HTMLElement;

  constructor(private readonly editor: DiagramEditorFacade) {
    this.zoomReadout = el("span", { class: "zoom-readout", text: "100%" });
    this.dropHint = el("div", { class: "drop-hint", attrs: { hidden: "true" } }, [
      el("div", { class: "drop-hint-card" }, [
        el("p", { attrs: { style: "font-size: 28px; margin: 0 0 4px" }, text: "📥" }),
        el("p", { attrs: { style: "margin: 0; font-weight: 600" }, text: "Перетащите сюда JSON-файл схемы" }),
      ]),
    ]);

    const zoomBar = el("div", { class: "zoom-bar" }, [
      el("button", { text: "+", title: "Приблизить", on: { click: () => editor.canvas.zoomBy(1.2) } }),
      el("button", { text: "−", title: "Отдалить", on: { click: () => editor.canvas.zoomBy(0.8) } }),
      el("button", { text: "100%", title: "100%", on: { click: () => editor.canvas.resetZoom() } }),
      el("button", { text: "Вписать", title: "Вписать в экран", on: { click: () => editor.canvas.fit() } }),
      this.zoomReadout,
    ]);

    this.element = el("div", { class: "canvas-area", attrs: { style: "width: 100%; height: 100%; position: relative; overflow: hidden;" } }, [
      editor.canvas.hostElement,
      zoomBar,
      this.dropHint,
    ]);

    editor.canvas.events.on("viewport", (state) => {
      this.zoomReadout.textContent = `${Math.round(state.zoom * 100)}%`;
    });

    this.bindDrop();

    const ro = new ResizeObserver(() => {
      this.editor.canvas.render();
    });
    ro.observe(this.element);
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.editor.canvas.render();
  }

  onShow(): void {
    this.editor.canvas.render();
  }

  layout(_width: number, _height: number): void {
    this.editor.canvas.render();
  }

  private bindDrop(): void {
    window.addEventListener("dragover", (e) => {
      e.preventDefault();
      if (!e.dataTransfer?.types.includes("application/semaps-entity")) {
        this.dropHint.hidden = false;
      }
    });

    window.addEventListener("dragleave", (e) => {
      if (e.relatedTarget === null) this.dropHint.hidden = true;
    });

    window.addEventListener("drop", () => {
      this.dropHint.hidden = true;
    });
  }
}
