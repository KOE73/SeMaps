import { StyleExtensionsEditor } from "./StyleExtensionsPanel.js";
import { styleListFraction } from "./styleSplit.js";
import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { Selection } from "../../canvas/DiagramCanvas.js";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { StyleList, type StylePanelHost } from "../../editor/StyleList.js";
import { StyleEditor } from "../../editor/StyleEditor.js";
import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

const STYLE_LIST_WIDTH_KEY = "semaps:style-list-ratio";

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export class StylesPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly styleListMount: HTMLElement;
  private readonly styleEditorMount: HTMLElement;
  private readonly resizer: HTMLElement;
  private readonly styleList: StyleList;
  private readonly styleEditor: StyleEditor;
  private mode: "kinds" | "edge" | "modifiers" = "kinds";
  private modifiers: StyleExtensionsEditor | null = null;
  private readonly modifierList = el("div", { attrs: { style: "height:100%;overflow:auto" } });
  private readonly modifierForm = el("div", { attrs: { style: "height:100%;overflow:auto" } });
  private readonly baseList = el("div", { attrs: { style: "height:100%;overflow:auto;display:flex;flex-direction:column" } });
  private readonly baseForm = el("div");
  private readonly modeButtons = new Map<string, HTMLButtonElement>();
  private readonly listContent = el("div", { class: "style-list-content" });
  private resizeObserver: ResizeObserver | null = null;
  private panelApi: GroupPanelPartInitParameters["api"] | undefined;

  constructor(editor: DiagramEditor & StylePanelHost) {
    this.styleListMount = el("div", {
      class: "style-list",
      attrs: { style: "width: 40%; min-width: 0; padding-right: 0; flex-shrink: 0; display: flex; flex-direction: column; overflow: hidden; border-right: 1px solid var(--border);" },
    });

    this.resizer = el("div", {
      class: "style-pane-resizer",
      title: "Потяните, чтобы изменить ширину списка",
    });

    this.styleEditorMount = el("div", {
      class: "style-editor",
      attrs: { style: "flex: 1; min-width: 0; overflow-y: auto; overflow-x: hidden; height: 100%;" },
    });

    const body = el(
      "div",
      {
        class: "inspector-body styles-pane",
        attrs: { style: "display: flex; flex-direction: row; height: 100%; overflow: hidden; padding: 0;" },
      },
      [this.styleListMount, this.resizer, this.styleEditorMount],
    );

    const modes = el("div", { class: "style-mode-switch" });
    for (const mode of ["kinds", "edge", "modifiers"] as const) {
      const control = el("button", { class: "btn style-mode-button", on: { click: () => this.switchMode(mode, editor) } });
      this.modeButtons.set(mode, control); modes.append(control);
    }
    this.updateModes();
    this.styleListMount.append(modes, this.listContent);
    this.element = el(
      "div",
      {
        class: "inspector",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [body],
    );

    const host: StylePanelHost = {
      get canvas() { return editor.canvas; },
      get styles() { return editor.styles; },
      get sharedStyles() { return editor.sharedStyles; },
      renameStyle: (id,next)=>editor.renameStyle(id,next),
      deleteStyle: (id,replacement)=>editor.deleteStyle(id,replacement),
      editStyle: (apply) => editor.editStyle(apply),
      commitStyle: (apply) => editor.commitStyle(apply),
      openStyle: (id) => {
        this.openStyle(id);
      },
      notify: (msg) => editor.notify(msg),
      applyKindAndStyle: (ids, kind, styleId) => editor.applyKindAndStyle(ids, kind, styleId),
      applyRelationTypeAndStyle: (ids, type, styleId) => editor.applyRelationTypeAndStyle(ids, type, styleId),
    };

    this.baseList.classList.add("style-base-list");
    this.listContent.append(this.baseList); this.styleEditorMount.append(this.baseForm);
    this.styleList = new StyleList(this.baseList, host, true);
    this.styleEditor = new StyleEditor(this.baseForm, host);

    editor.onOpenStyle = (id: string | null) => {
      this.openStyle(id);
    };

    this.bindResizer();
    editor.libraryEvents.on("openExtension", ({collection,id}) => {
      if(collection !== "modifiers") return;
      this.switchMode("modifiers", editor); this.modifiers?.open(id);
    });

    // Selecting something on the canvas shows its style here — only while this panel is on screen;
    // a closed or hidden panel is left alone (selecting must never open it).
    editor.canvas.events.on("select", (selection) => this.followSelection(selection, editor));

    // The panel can be built (and shown) before the host's styles and dictionary have arrived.
    editor.libraryEvents.on("loaded", () => this.render());
    editor.libraryEvents.on("changed",()=>{if(this.mode!=="modifiers")this.styleList.render()});
    editor.libraryEvents.on("renamed",({oldId,newId})=>{
      if(this.styleEditor.followRename(oldId,newId) && this.mode!=="modifiers")this.styleList.setActive(this.styleEditor.openId);
    });
    editor.libraryEvents.on("removed",id=>{
      this.styleEditor.removed(id);
      if(this.mode!=="modifiers")this.styleList.setActive(this.styleEditor.openId);
    });

    i18n.onLanguageChange(() => {
      this.updateModes();
      this.render();
    });
  }

  init(params: GroupPanelPartInitParameters): void {
    this.panelApi = params.api;
    this.render();
  }

  /**
   * Open the style that dresses the primary selected element: sub-tab, group,
   * the style's row (scrolled into view) and the style editor. Multi-selection
   * follows the primary (first) element.
   */
  private followSelection(selection: Selection | null, editor: DiagramEditor): void {
    if (this.mode === "modifiers") return;
    if (selection === null || this.panelApi?.isVisible !== true) return;
    const doc = editor.canvas.model;
    if (doc === null) return;
    const lib = editor.styles;
    let styleId: string | null = null;
    if (selection.kind === "edge") {
      const edge = doc.edge(selection.id);
      const relation = edge === undefined ? doc.relations.find((r) => r.id === selection.id) : undefined;
      if (edge !== undefined) styleId = lib.edgeStyleIdFor(edge);
      else if (relation !== undefined) styleId = lib.edgeStyleIdFor({ type: relation.type });
    } else {
      const element = doc.element(selection.id);
      if (element !== undefined) styleId = lib.blockStyleIdFor(element);
    }
    if (styleId !== null) this.openStyle(styleId);
  }

  onShow(): void {
    this.render();
  }

  render(): void {
    if (this.mode === "modifiers") return;
    this.styleList.render();
    this.styleEditor.render();
  }

  openStyle(id: string | null): void {
    this.styleList.setActive(id);
    this.styleEditor.open(id);
  }

  private switchMode(mode: "kinds" | "edge" | "modifiers", editor: DiagramEditor): void {
    this.mode = mode;
    this.updateModes();
    if (mode === "modifiers") {
      this.listContent.replaceChildren(this.modifierList);
      this.styleEditorMount.replaceChildren(this.modifierForm);
      this.modifiers ??= new StyleExtensionsEditor(this.modifierList, this.modifierForm, editor, "modifiers");
    } else {
      this.listContent.replaceChildren(this.baseList); this.styleEditorMount.replaceChildren(this.baseForm);
      this.styleList.showMode(mode); this.styleEditor.render();
    }
  }
  private updateModes(): void {
    for (const [mode, control] of this.modeButtons) {
      control.textContent = mode === "kinds" ? i18n.d.panels.styles.blocksTab : mode === "edge" ? i18n.d.panels.styles.edgesTab : i18n.currentLanguage === "ru" ? "Модификаторы" : "Modifiers";
      control.setAttribute("aria-pressed", String(mode === this.mode));
      control.classList.toggle("active", mode === this.mode);
    }
  }
  dispose(): void { this.resizeObserver?.disconnect(); }
  private bindResizer(): void {
    const list = this.styleListMount;
    const stored = Number(window.localStorage.getItem(STYLE_LIST_WIDTH_KEY));
    let preferred = Number.isFinite(stored) && stored > 0 ? stored : 0.4;
    const available = (): number => Math.max(1, this.element.offsetWidth - this.resizer.offsetWidth);
    const update = (): void => { list.style.width = `${styleListFraction(preferred, available()) * available()}px`; };
    this.resizeObserver = new ResizeObserver(update); this.resizeObserver.observe(this.element); update();

    this.resizer.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      const startX = e.clientX;
      const startWidth = list.offsetWidth;
      this.resizer.setPointerCapture(e.pointerId);
      this.resizer.classList.add("is-dragging");

      const onMove = (move: PointerEvent): void => {
        preferred = clamp((startWidth + move.clientX - startX) / available(), 0.15, 0.85);
        update();
      };

      const onUp = (): void => {
        this.resizer.classList.remove("is-dragging");
        this.resizer.releasePointerCapture(e.pointerId);
        this.resizer.removeEventListener("pointermove", onMove);
        this.resizer.removeEventListener("pointerup", onUp);
        window.localStorage.setItem(STYLE_LIST_WIDTH_KEY, String(preferred));
      };

      this.resizer.addEventListener("pointermove", onMove);
      this.resizer.addEventListener("pointerup", onUp);
    });
  }
}
