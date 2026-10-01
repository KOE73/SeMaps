import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditorFacade } from "../commands/types.js";
import { el } from "../../util/dom.js";

export class CanvasPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly zoomReadout: HTMLElement;

  constructor(private readonly editor: DiagramEditorFacade) {
    this.zoomReadout = el("span", { class: "zoom-readout", text: "100%" });

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
    ]);

    editor.canvas.events.on("viewport", (state) => {
      this.zoomReadout.textContent = `${Math.round(state.zoom * 100)}%`;
    });

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
}
