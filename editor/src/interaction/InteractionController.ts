import { center, snap, unionRect } from "../geometry/rect.js";
import type { Rect } from "../geometry/types.js";
import type { DiagramElement } from "../model/types.js";
import { elementRect, isContainer } from "../model/types.js";
import type { DiagramCanvas } from "../canvas/DiagramCanvas.js";
import type { ResizeDirection, ResizeGuide } from "../canvas/render/handles.js";
import { Role, hitTest, type RoleHit } from "./roles.js";
import { renderMarkdown } from "../editor/doc/MarkdownRenderer.js";
import { SourceCodeService } from "../editor/code/SourceCodeService.js";
import { DIAGRAM_CONFIG } from "../constants/diagram-constants.js";
import { i18n } from "../workbench/i18n/I18nService.js";

const MIN_SIZE = {
  zone: DIAGRAM_CONFIG.container.minSize,
  node: DIAGRAM_CONFIG.node.minSize,
} as const;

interface Origin {
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

interface PanGesture {
  readonly kind: "pan";
  readonly startX: number;
  readonly startY: number;
  readonly panX: number;
  readonly panY: number;
  readonly elementOnClick?: string | null;
  hasMoved?: boolean;
}

interface MoveGesture {
  readonly kind: "move";
  readonly lead: DiagramElement;
  readonly startX: number;
  readonly startY: number;
  readonly origins: ReadonlyMap<DiagramElement, Origin>;
  /** The elements the user actually selected, without their carried subtrees. */
  readonly moved: readonly DiagramElement[];
}

interface ResizeGesture {
  readonly kind: "resize";
  readonly dir: ResizeDirection;
  readonly startX: number;
  readonly startY: number;
  /** The box the grips were drawn around: one element, or the whole selection. */
  readonly bounds: Rect;
  readonly origins: ReadonlyMap<DiagramElement, Origin>;
  /** True when the box covers several elements and they scale proportionally. */
  readonly group: boolean;
  readonly single: DiagramElement | null;
}

interface MarqueeGesture {
  readonly kind: "marquee";
  readonly startX: number;
  readonly startY: number;
  readonly additive: boolean;
  readonly base: readonly string[];
}

type Gesture = PanGesture | MoveGesture | ResizeGesture | MarqueeGesture;

/**
 * Turns pointer input into model changes.
 *
 * Listens once on the canvas host and resolves what was hit through the
 * `data-role` marks renderers leave behind. No renderer is named here, and no
 * handler is attached to an individual element — which is what lets a renderer
 * added from outside behave like the built-in ones, and what stops thousands of
 * closures from being rebuilt on every frame.
 */
export class InteractionController {
  private gesture: Gesture | null = null;
  private readonly abort = new AbortController();
  private edgeControlsHideTimer: number | null = null;
  /** Pending show of the bubble, and the edge it is for. */
  private edgeControlsShowTimer: number | null = null;
  private pendingEdgeControlId: string | null = null;
  private lastPointer = { x: 0, y: 0 };
  private currentEdgeControlId: string | null = null;
  private isOverEdgeControls = false;

  constructor(
    private readonly canvas: DiagramCanvas,
    private readonly host: HTMLElement,
  ) {
    const signal = this.abort.signal;
    host.addEventListener("mousedown", this.onMouseDown, { signal });
    host.addEventListener("mouseup", this.onMouseUpTarget, { signal });
    host.addEventListener("dblclick", this.onDoubleClick, { signal });
    host.addEventListener("contextmenu", this.onContextMenu, { signal });
    host.addEventListener("wheel", this.onWheel, { passive: false, signal });
    host.addEventListener("mouseleave", () => {
      if (!this.isOverEdgeControls) {
        this.scheduleEdgeControlsHide();
        this.canvas.hideAllTooltips();
      }
    }, { signal });
    window.addEventListener("mousemove", this.onMouseMove, { signal });
    window.addEventListener("mouseup", this.onMouseUp, { signal });

    // Edge control bar events
    const edgeControls = this.canvas.edgeControlsEl;
    edgeControls.addEventListener("mouseenter", () => {
      this.isOverEdgeControls = true;
      this.clearEdgeControlsHideTimer();
    }, { signal });
    edgeControls.addEventListener("mouseleave", () => {
      this.isOverEdgeControls = false;
      this.scheduleEdgeControlsHide();
      this.canvas.hideRichTooltip();
    }, { signal });
    edgeControls.addEventListener("click", (e) => {
      const btn = (e.target as HTMLElement).closest<HTMLElement>(".semaps-edge-doc-btn");
      if (btn) {
        e.stopPropagation();
        const edgeId = btn.dataset.edgeId;
        if (edgeId) {
          this.currentEdgeControlId = null;
          this.isOverEdgeControls = false;
          this.canvas.hideAllTooltips();
          this.canvas.hideEdgeControls();
          this.canvas.events.emit("openDocEditor", { id: edgeId, kind: "edge" });
        }
      }
    }, { signal });
    edgeControls.addEventListener("mouseover", (e) => {
      const btn = (e.target as HTMLElement).closest<HTMLElement>(".semaps-edge-doc-btn");
      if (btn && btn.dataset.edgeId) {
        this.showEdgeDocTooltip(btn.dataset.edgeId, btn);
      }
    }, { signal });
    edgeControls.addEventListener("mouseout", (e) => {
      const btn = (e.target as HTMLElement).closest<HTMLElement>(".semaps-edge-doc-btn");
      if (btn) {
        this.canvas.hideRichTooltip();
      }
    }, { signal });
  }

  private clearEdgeControlsHideTimer(): void {
    if (this.edgeControlsHideTimer !== null) {
      window.clearTimeout(this.edgeControlsHideTimer);
      this.edgeControlsHideTimer = null;
    }
  }

  private cancelEdgeControlsShow(): void {
    if (this.edgeControlsShowTimer !== null) {
      window.clearTimeout(this.edgeControlsShowTimer);
      this.edgeControlsShowTimer = null;
    }
    this.pendingEdgeControlId = null;
  }

  /** Show the bubble once the mouse has rested on the edge; crossing lines shows nothing. */
  private scheduleEdgeControlsShow(edgeId: string): void {
    if (this.pendingEdgeControlId === edgeId) return;
    this.cancelEdgeControlsShow();
    this.pendingEdgeControlId = edgeId;
    this.edgeControlsShowTimer = window.setTimeout(() => {
      this.edgeControlsShowTimer = null;
      this.pendingEdgeControlId = null;
      this.currentEdgeControlId = edgeId;
      this.canvas.showEdgeControls(edgeId, this.lastPointer.x, this.lastPointer.y);
    }, DIAGRAM_CONFIG.interaction.edgeControlsShowDelayMs);
  }

  private scheduleEdgeControlsHide(): void {
    this.cancelEdgeControlsShow();
    this.clearEdgeControlsHideTimer();
    this.edgeControlsHideTimer = window.setTimeout(() => {
      if (!this.isOverEdgeControls) {
        this.canvas.hideEdgeControls();
        this.currentEdgeControlId = null;
      }
      this.edgeControlsHideTimer = null;
    }, DIAGRAM_CONFIG.interaction.edgeControlsHideDelayMs);
  }

  destroy(): void {
    this.abort.abort();
    this.clearEdgeControlsHideTimer();
    this.cancelEdgeControlsShow();
    this.canvas.hideAllTooltips();
    this.canvas.hideEdgeControls();
    this.currentEdgeControlId = null;
    this.isOverEdgeControls = false;
  }

  // ------------------------------------------------------------- listeners

  private readonly onMouseDown = (e: MouseEvent): void => {
    const target = e.target as HTMLElement | null;
    if (target && this.canvas.edgeControlsEl.contains(target)) {
      return;
    }
    this.canvas.hideAllTooltips();
    this.canvas.hideEdgeControls();
    this.cancelEdgeControlsShow();
    this.currentEdgeControlId = null;
    this.isOverEdgeControls = false;
    if (e.button !== 0) return;
    const hit = hitTest(e.target);

    if (hit === null) {
      this.startOnEmptyCanvas(e);
      return;
    }

    if (hit.role === Role.ResizeHandle) {
      e.stopPropagation();
      this.startResize(e, hit);
      return;
    }

    if (hit.edgeId !== null && hit.elementId === null) {
      this.canvas.select(hit.edgeId);
      return;
    }

    const el = hit.elementId === null ? null : this.canvas.model?.element(hit.elementId) ?? null;
    if (el === null) return;

    switch (hit.role) {
      case Role.CollapseToggle:
      case Role.GhostToggle:
        // Handled on mouse-up so that pressing the button never starts a drag.
        e.stopPropagation();
        return;

      case Role.DragHandle:
        e.stopPropagation();
        this.applyClickSelection(el, e);
        if (!this.canvas.selectedIds.has(el.id)) return;
        this.canvas.emitGestureStart("move");
        this.gesture = this.buildMove(el, e);
        return;

      case Role.Body:
        if (isContainer(el)) {
          const additive = e.ctrlKey || e.metaKey || e.shiftKey;
          if (additive) {
            this.gesture = {
              kind: "marquee",
              startX: e.clientX,
              startY: e.clientY,
              additive: true,
              base: [...this.canvas.selectedIds],
            };
            return;
          }

          // Dragging container body pans the canvas view!
          this.gesture = {
            kind: "pan",
            startX: e.clientX,
            startY: e.clientY,
            panX: this.canvas.viewport.panX,
            panY: this.canvas.viewport.panY,
            elementOnClick: el.id,
            hasMoved: false,
          };
          this.host.classList.add("is-panning");
          return;
        }
        this.applyClickSelection(el, e);
        return;

      default:
        return;
    }
  };

  /**
   * Ctrl or Shift adds to the selection; a plain click replaces it. Clicking an
   * element that is already part of a multi-selection keeps the group, so that
   * dragging it moves everything rather than collapsing to one.
   */
  private applyClickSelection(el: DiagramElement, e: MouseEvent): void {
    if (e.ctrlKey || e.metaKey) {
      this.canvas.toggleSelected(el.id);
      return;
    }
    if (e.shiftKey) {
      if (!this.canvas.selectedIds.has(el.id)) {
        this.canvas.toggleSelected(el.id);
      }
      return;
    }
    if (this.canvas.selectedIds.has(el.id) && this.canvas.selectedIds.size > 1) {
      return;
    }
    this.canvas.select(el.id);
  }

  private startOnEmptyCanvas(e: MouseEvent): void {
    const additive = e.ctrlKey || e.metaKey || e.shiftKey;
    if (additive) {
      this.gesture = {
        kind: "marquee",
        startX: e.clientX,
        startY: e.clientY,
        additive: true,
        base: [...this.canvas.selectedIds],
      };
      return;
    }

    this.canvas.select(null);
    this.canvas.clearGhostNode();
    this.gesture = {
      kind: "pan",
      startX: e.clientX,
      startY: e.clientY,
      panX: this.canvas.viewport.panX,
      panY: this.canvas.viewport.panY,
    };
    this.host.classList.add("is-panning");
  }

  private readonly onMouseUpTarget = (e: MouseEvent): void => {
    const hit = hitTest(e.target);
    if (hit?.role === Role.CollapseToggle && hit.elementId !== null) {
      e.stopPropagation();
      this.canvas.toggleCollapse(hit.elementId);
    } else if (hit?.role === Role.GhostToggle && hit.elementId !== null) {
      e.stopPropagation();
      this.canvas.toggleGhostNode(hit.elementId);
    } else if (hit?.role === Role.DocEdit && (hit.elementId !== null || hit.edgeId !== null)) {
      e.stopPropagation();
      this.canvas.hideAllTooltips();
      const targetId = hit.elementId || hit.edgeId!;
      const doc = this.canvas.model;
      const isEdge = Boolean(hit.edgeId);
      const isZone = Boolean(hit.elementId && doc && isContainer(doc.element(hit.elementId)!));
      const kind = isEdge ? "edge" : (isZone ? "zone" : "node");
      this.canvas.events.emit("openDocEditor", { id: targetId, kind });
    } else if (hit?.role === Role.CodeView && hit.elementId !== null) {
      e.stopPropagation();
      this.canvas.hideAllTooltips();
      const doc = this.canvas.model;
      const el = doc?.element(hit.elementId);
      const codeRef = typeof el?.metadata?.codeRef === "string" ? el.metadata.codeRef.trim() : "";
      if (codeRef) {
        this.canvas.events.emit("openCodeViewer", { id: hit.elementId, codeRef, label: el?.label });
      }
    }
  };

  private readonly onMouseMove = (e: MouseEvent): void => {
    const gesture = this.gesture;
    if (gesture === null) {
      const target = e.target as HTMLElement | null;
      if (target && (this.canvas.edgeControlsEl.contains(target) || this.canvas.richTooltipEl.contains(target))) {
        this.isOverEdgeControls = true;
        this.clearEdgeControlsHideTimer();
        return;
      }
      this.updateHoverTooltip(e);
      return;
    }

    this.canvas.hideAllTooltips();
    this.canvas.hideEdgeControls();
    this.cancelEdgeControlsShow();

    switch (gesture.kind) {
      case "pan": {
        const dx = Math.abs(e.clientX - gesture.startX);
        const dy = Math.abs(e.clientY - gesture.startY);
        if (dx > 3 || dy > 3) {
          gesture.hasMoved = true;
        }
        this.canvas.viewport.set({
          panX: gesture.panX + (e.clientX - gesture.startX),
          panY: gesture.panY + (e.clientY - gesture.startY),
        });
        return;
      }
      case "move":
        this.applyMove(gesture, e);
        return;
      case "resize":
        this.applyResize(gesture, e);
        return;
      case "marquee":
        this.applyMarquee(gesture, e);
        return;
    }
  };

  private updateHoverTooltip(e: MouseEvent): void {
    const hit = hitTest(e.target);
    if (hit === null || (hit.elementId === null && hit.edgeId === null)) {
      this.scheduleEdgeControlsHide();
      this.canvas.hideAllTooltips();
      return;
    }

    const doc = this.canvas.model;
    if (!doc) {
      this.scheduleEdgeControlsHide();
      this.canvas.hideAllTooltips();
      return;
    }

    const lang = this.canvas.dataLang || "ru";

    // 1. Hover on DocEdit Button -> Show Rich Doc Tooltip immediately
    if (hit.role === Role.DocEdit) {
      const targetId = hit.elementId || hit.edgeId;
      if (!targetId) {
        this.canvas.hideAllTooltips();
        return;
      }

      let name = targetId;
      let kind = "NODE";
      if (hit.edgeId) {
        const edge = doc.edge(targetId);
        if (edge) {
          const fromEl = doc.element(edge.from);
          const toEl = doc.element(edge.to);
          const fromName = doc.getText(edge.from, lang)?.name || fromEl?.label || edge.from;
          const toName = doc.getText(edge.to, lang)?.name || toEl?.label || edge.to;
          name = `${fromName} ➔ ${toName}`;
          kind = edge.type || "RELATION";
        }
      } else if (hit.elementId) {
        const el = doc.element(targetId);
        if (el) {
          name = doc.getText(el.id, lang)?.name || el.label || el.id;
          kind = el.type || (isContainer(el) ? "ZONE" : "NODE");
        }
      }

      const text = doc.getText(targetId, lang);
      const desc = text?.description || "";
      const docText = text?.doc || "";
      const docMd = docText ? renderMarkdown(docText) : "";

      let content = `<div class="semaps-rich-doc-tooltip-head">
        <span class="semaps-rich-doc-tooltip-title">${escapeHtml(name)}</span>
        <span class="semaps-doc-kind-badge">${escapeHtml(kind)}</span>
      </div>`;
      if (desc) {
        content += `<div class="semaps-rich-doc-tooltip-desc"><b>${escapeHtml(i18n.d.dialogs.docEditor.descriptionLabel)}:</b> ${escapeHtml(desc)}</div>`;
      }
      if (docMd) {
        content += `<div class="semaps-rich-doc-tooltip-doc semaps-markdown-body">${docMd}</div>`;
      } else {
        content += `<div class="semaps-rich-doc-tooltip-doc" style="color: var(--muted); font-style: italic; font-size: 11px; padding: 4px 0;">${escapeHtml(i18n.d.dialogs.docEditor.previewEmpty)}</div>`;
      }
      content += `<div class="semaps-rich-doc-tooltip-foot">
        <span>${escapeHtml(i18n.d.dialogs.docEditor.viewDocTooltip)}</span>
        <span class="chip chip-lang">${lang.toUpperCase()}</span>
      </div>`;

      this.canvas.showRichTooltip(content, e.clientX, e.clientY);
      return;
    }

    // 1b. Hover on CodeView Button -> Show Rich Code Tooltip immediately
    if (hit.role === Role.CodeView && hit.elementId !== null) {
      const el = doc.element(hit.elementId);
      const codeRef = typeof el?.metadata?.codeRef === "string" ? el.metadata.codeRef.trim() : "";
      if (codeRef) {
        this.showCodePreviewTooltip(codeRef, el?.label || hit.elementId, e.clientX, e.clientY);
        return;
      }
    }

    // 2. Normal Element Hover
    if (hit.elementId !== null) {
      const el = doc.element(hit.elementId);
      if (el) {
        const text = doc.getText(el.id, lang);
        const name = text?.name || text?.title || el.label || el.id;
        const desc = text?.description || (typeof el.metadata?.description === "string" ? el.metadata.description : "");
        const codeRef = typeof el.metadata?.codeRef === "string" ? el.metadata.codeRef : "";
        const kind = el.type || (isContainer(el) ? "Zone" : "Component");

        let content = `<div class="semaps-tooltip-header">
          <span>${escapeHtml(name)}</span>
          <span class="semaps-tooltip-kind">${escapeHtml(kind)}</span>
        </div>`;

        if (desc) {
          content += `<div class="semaps-tooltip-body">${escapeHtml(desc)}</div>`;
        }
        if (codeRef) {
          content += `<div class="semaps-tooltip-coderef">${escapeHtml(codeRef)}</div>`;
        }

        this.canvas.showTooltip(content, e.clientX, e.clientY);
        return;
      }
    }

    // 3. Edge Hover -> Show standard tooltip & show dynamic edge control bar
    if (hit.edgeId !== null) {
      const edgeId = hit.edgeId;
      const edge = doc.edge(edgeId);
      if (edge) {
        const fromEl = doc.element(edge.from);
        const toEl = doc.element(edge.to);
        const fromName = doc.getText(edge.from, lang)?.name || fromEl?.label || edge.from;
        const toName = doc.getText(edge.to, lang)?.name || toEl?.label || edge.to;
        const text = doc.getText(edge.id, lang);
        const desc = text?.description || edge.label || "";

        let content = `<div class="semaps-tooltip-header">
          <span>${escapeHtml(fromName)} ➔ ${escapeHtml(toName)}</span>
          <span class="semaps-tooltip-kind">${escapeHtml(edge.type)}</span>
        </div>`;
        if (desc) {
          content += `<div class="semaps-tooltip-body">${escapeHtml(desc)}</div>`;
        }
        this.canvas.showTooltip(content, e.clientX, e.clientY);

        // Show floating edge control bar with Doc button (stable position once per edge)
        this.clearEdgeControlsHideTimer();
        this.lastPointer = { x: e.clientX, y: e.clientY };
        if (this.currentEdgeControlId !== edgeId) this.scheduleEdgeControlsShow(edgeId);
        return;
      }
    }

    if (!this.isOverEdgeControls) {
      this.scheduleEdgeControlsHide();
    }
    this.canvas.hideAllTooltips();
  }

  private showEdgeDocTooltip(edgeId: string, anchorEl: HTMLElement): void {
    const doc = this.canvas.model;
    if (!doc) return;
    const edge = doc.edge(edgeId);
    if (!edge) return;
    const lang = this.canvas.dataLang || "ru";
    const fromEl = doc.element(edge.from);
    const toEl = doc.element(edge.to);
    const fromName = doc.getText(edge.from, lang)?.name || fromEl?.label || edge.from;
    const toName = doc.getText(edge.to, lang)?.name || toEl?.label || edge.to;
    const text = doc.getText(edge.id, lang);
    const desc = text?.description || edge.label || "";
    const docText = text?.doc || "";
    const docMd = docText ? renderMarkdown(docText) : "";

    let content = `<div class="semaps-rich-doc-tooltip-head">
      <span class="semaps-rich-doc-tooltip-title">${escapeHtml(fromName)} ➔ ${escapeHtml(toName)}</span>
      <span class="semaps-doc-kind-badge">${escapeHtml(edge.type || "RELATION")}</span>
    </div>`;
    if (desc) {
      content += `<div class="semaps-rich-doc-tooltip-desc"><b>${escapeHtml(i18n.d.dialogs.docEditor.descriptionLabel)}:</b> ${escapeHtml(desc)}</div>`;
    }
    if (docMd) {
      content += `<div class="semaps-rich-doc-tooltip-doc semaps-markdown-body">${docMd}</div>`;
    } else {
      content += `<div class="semaps-rich-doc-tooltip-doc" style="color: var(--muted); font-style: italic; font-size: 11px; padding: 4px 0;">${escapeHtml(i18n.d.dialogs.docEditor.previewEmpty)}</div>`;
    }
    content += `<div class="semaps-rich-doc-tooltip-foot">
      <span>${escapeHtml(i18n.d.dialogs.docEditor.viewDocTooltip)}</span>
      <span class="chip chip-lang">${lang.toUpperCase()}</span>
    </div>`;

    const rect = anchorEl.getBoundingClientRect();
    this.canvas.showRichTooltip(content, rect.left + rect.width / 2, rect.top);
  }

  private async showCodePreviewTooltip(
    codeRef: string,
    label: string,
    x: number,
    y: number,
  ): Promise<void> {
    const langLabel = SourceCodeService.getLanguageLabel(codeRef);
    const initialHtml = `
      <div class="semaps-rich-code-tooltip-head">
        <span style="font-weight: 700;">💻 ${escapeHtml(label)}</span>
        <span class="semaps-rich-code-tooltip-path" title="${escapeHtml(codeRef)}">${escapeHtml(codeRef)}</span>
      </div>
      <div class="semaps-rich-code-tooltip-body">
        <div style="color: #888; padding: 8px;">⏳ Загрузка фрагмента кода...</div>
      </div>
      <div class="semaps-rich-code-tooltip-foot">
        <span>Клик — открыть полный просмотрщик</span>
        <span style="color: #60a5fa; font-weight: 700;">${escapeHtml(langLabel)}</span>
      </div>
    `;

    this.canvas.showRichTooltip(initialHtml, x, y);

    try {
      const preview = await SourceCodeService.getPreview(codeRef, 12);
      const content = `
        <div class="semaps-rich-code-tooltip-head">
          <span style="font-weight: 700;">💻 ${escapeHtml(label)}</span>
          <span class="semaps-rich-code-tooltip-path" title="${escapeHtml(codeRef)}">${escapeHtml(codeRef)}</span>
        </div>
        <div class="semaps-rich-code-tooltip-body">
          ${preview.snippetHtml}
        </div>
        <div class="semaps-rich-code-tooltip-foot">
          <span>Клик — открыть полный просмотрщик (${preview.totalLines} строк)</span>
          <span style="color: #60a5fa; font-weight: 700;">${escapeHtml(preview.language)}</span>
        </div>
      `;
      this.canvas.showRichTooltip(content, x, y);
    } catch (err: any) {
      const content = `
        <div class="semaps-rich-code-tooltip-head">
          <span style="font-weight: 700;">💻 ${escapeHtml(label)}</span>
          <span class="semaps-rich-code-tooltip-path" title="${escapeHtml(codeRef)}">${escapeHtml(codeRef)}</span>
        </div>
        <div class="semaps-rich-code-tooltip-body">
          <div style="color: #f87171; padding: 8px;">⚠️ ${escapeHtml(err?.message || "Файл недоступен")}</div>
        </div>
        <div class="semaps-rich-code-tooltip-foot">
          <span>Клик — открыть просмотрщик</span>
          <span style="color: #ef4444; font-weight: 700;">Ошибка</span>
        </div>
      `;
      this.canvas.showRichTooltip(content, x, y);
    }
  }

  private readonly onMouseUp = (): void => {
    const gesture = this.gesture;
    this.gesture = null;
    this.host.classList.remove("is-panning");
    if (gesture === null) return;

    if (gesture.kind === "pan") {
      if (!gesture.hasMoved && gesture.elementOnClick) {
        this.canvas.select(gesture.elementOnClick);
      }
      return;
    }

    if (gesture.kind === "marquee") {
      this.canvas.marqueeRect = null;
      this.canvas.render();
      return;
    }

    if (gesture.kind === "move") {
      this.finishMove(gesture);
    }
    if (this.canvas.resizeGuides.length > 0) {
      this.canvas.resizeGuides = [];
      this.canvas.render();
    }
    this.canvas.emitGestureEnd(gesture.kind);
  };

  private readonly onDoubleClick = (e: MouseEvent): void => {
    const hit = hitTest(e.target);
    if (hit?.edgeId && hit.role !== Role.ResizeHandle) {
      e.stopPropagation();
      this.canvas.events.emit("edgeToggle", { id: hit.edgeId });
      return;
    }
    if (hit === null || hit.role !== Role.ResizeHandle) return;

    const elements = this.canvas.selectedElements();
    const el = elements.length === 1 ? elements[0] : undefined;
    if (el === undefined || isContainer(el)) return;

    e.stopPropagation();
    const fitted = Math.max(120, el.label.length * 8 + 40);
    if (fitted === el.width) return;
    this.canvas.emitGestureStart("fit-width");
    el.width = fitted;
    this.canvas.notifyModelChanged("fit-width");
    this.canvas.emitGestureEnd("fit-width");
  };

  /**
   * Right click asks for a menu: on a box (selecting it, unless it is already
   * part of the selection), on a line, or on the empty canvas.
   */
  private readonly onContextMenu = (e: MouseEvent): void => {
    const target = e.target as HTMLElement | null;
    if (target && this.canvas.edgeControlsEl.contains(target)) return;
    e.preventDefault();
    this.canvas.hideAllTooltips();
    this.canvas.hideEdgeControls();
    this.cancelEdgeControlsShow();
    const hit = hitTest(e.target);
    const at = { clientX: e.clientX, clientY: e.clientY };
    if (hit?.edgeId) {
      if (!this.canvas.selectedIds.has(hit.edgeId)) this.canvas.select(hit.edgeId);
      this.canvas.events.emit("contextmenu", { target: "edge", id: hit.edgeId, ...at });
    } else if (hit?.elementId) {
      if (!this.canvas.selectedIds.has(hit.elementId)) this.canvas.select(hit.elementId);
      this.canvas.events.emit("contextmenu", { target: "element", id: hit.elementId, ...at });
    } else {
      this.canvas.events.emit("contextmenu", { target: "canvas", id: null, ...at });
    }
  };

  private readonly onWheel = (e: WheelEvent): void => {
    e.preventDefault();
    this.canvas.zoomAtClient(e.deltaY < 0 ? 1.1 : 0.9, e.clientX, e.clientY);
  };

  // --------------------------------------------------------------- gestures

  /**
   * Remember where everything sat before a move.
   *
   * Everything selected moves together, and a container carries its whole
   * subtree — nested containers included. Without the descendants the diagram
   * visually falls apart the moment a container holding another container is
   * moved (R-EDIT-03).
   */
  private buildMove(lead: DiagramElement, e: MouseEvent): MoveGesture {
    const doc = this.canvas.model;
    const origins = new Map<DiagramElement, Origin>();
    const moved = this.canvas.selectedElements();
    const targets = moved.length > 0 ? moved : [lead];

    const remember = (el: DiagramElement): void => {
      if (!origins.has(el)) origins.set(el, snapshotOf(el));
    };

    for (const el of targets) {
      remember(el);
      if (isContainer(el) && doc !== null) {
        for (const child of doc.descendants(el)) remember(child);
      }
    }

    return {
      kind: "move",
      lead,
      startX: e.clientX,
      startY: e.clientY,
      origins,
      moved: targets,
    };
  }

  private applyMove(gesture: MoveGesture, e: MouseEvent): void {
    const raw = this.canvas.viewport.scaleDelta(
      e.clientX - gesture.startX,
      e.clientY - gesture.startY,
    );
    const origin = gesture.origins.get(gesture.lead);
    if (origin === undefined) return;

    const step = this.canvas.gridStep;
    // Snap the resulting coordinate, not the pointer delta, so an element that
    // is on the grid stays on it across successive drags (R-EDIT-04). The whole
    // group shifts by the lead element's snapped delta, keeping relative
    // positions exact.
    let dx = snap(origin.x + raw.x, step) - origin.x;
    let dy = snap(origin.y + raw.y, step) - origin.y;

    // Shift key constrains movement to a single axis (dominant X or Y)
    if (e.shiftKey) {
      if (Math.abs(raw.x) >= Math.abs(raw.y)) {
        dy = 0;
      } else {
        dx = 0;
      }
    }

    for (const [el, from] of gesture.origins) {
      el.x = from.x + dx;
      el.y = from.y + dy;
    }

    // Reparenting targets a single dragged element, leaf or container alike;
    // moving a multi-selection into a container is a different operation and
    // is left alone. `containerAt` excludes the dragged element's own subtree,
    // so a container can never be dropped into itself or a descendant.
    if (gesture.moved.length === 1) {
      const doc = this.canvas.model;
      const target = doc?.containerAt(center(elementRect(gesture.lead)), gesture.lead) ?? null;
      this.canvas.setDropTarget(target?.id ?? null);
    }

    let box: Rect | null = null;
    for (const el of gesture.moved) {
      const r = elementRect(el);
      box = box === null ? r : unionRect(box, r);
    }
    this.canvas.resizeGuides = box === null ? [] : this.guidesFor(gesture.origins, box, "nsew");

    this.canvas.notifyModelChanged("move");
  }

  private finishMove(gesture: MoveGesture): void {
    const doc = this.canvas.model;
    const dropTargetId = this.canvas.dropTargetId;
    this.canvas.setDropTarget(null);

    if (doc === null) return;
    if (gesture.moved.length !== 1) return;
    // A click without a drag never ran applyMove, so there is no drop target
    // to speak of — and a null one would mean "the root" and lift the element
    // out of its container just for being selected.
    const origin = gesture.origins.get(gesture.lead);
    if (origin !== undefined && origin.x === gesture.lead.x && origin.y === gesture.lead.y) return;

    const target = dropTargetId === null ? null : doc.element(dropTargetId) ?? null;
    if (target !== gesture.lead.parent) {
      doc.reparent(gesture.lead, target);
      this.canvas.notifyModelChanged("reparent");
    }
  }

  private startResize(e: MouseEvent, hit: RoleHit): void {
    const dir = (hit.handle ?? "se") as ResizeDirection;
    const bounds = this.canvas.selectionBounds();
    const elements = this.canvas.selectedElements();
    if (bounds === null || elements.length === 0) return;

    const origins = new Map<DiagramElement, Origin>();
    const doc = this.canvas.model;
    for (const el of elements) {
      origins.set(el, snapshotOf(el));
      // Scaling a group moves whatever those containers hold, so their contents
      // must travel too or the diagram comes apart.
      if (elements.length > 1 && isContainer(el) && doc !== null) {
        for (const child of doc.descendants(el)) {
          if (!origins.has(child)) origins.set(child, snapshotOf(child));
        }
      }
    }

    this.canvas.emitGestureStart("resize");
    this.gesture = {
      kind: "resize",
      dir,
      startX: e.clientX,
      startY: e.clientY,
      bounds,
      origins,
      group: elements.length > 1,
      single: elements.length === 1 ? elements[0]! : null,
    };
  }

  private applyResize(gesture: ResizeGesture, e: MouseEvent): void {
    const raw = this.canvas.viewport.scaleDelta(
      e.clientX - gesture.startX,
      e.clientY - gesture.startY,
    );
    const step = this.canvas.gridStep;
    const min = gesture.single !== null && isContainer(gesture.single)
      ? MIN_SIZE.zone
      : MIN_SIZE.node;

    const next = resizeRect(gesture.bounds, gesture.dir, raw.x, raw.y, min, step);

    if (gesture.single !== null) {
      const el = gesture.single;
      el.x = next.x;
      el.y = next.y;
      el.width = next.width;
      el.height = next.height;
    } else {
      this.scaleGroup(gesture, next);
    }

    this.canvas.resizeGuides = this.guidesFor(gesture.origins, next, gesture.dir);
    this.canvas.notifyModelChanged("resize");
  }

  /**
   * Lines through the named sides of `rect` ("n", "se", "nsew"…): a side grip
   * moves one edge, a corner grip two, a drag all four. Each is marked aligned
   * when another element's edges on the same axis sit on it, so the author can
   * line boxes up by eye instead of by the inspector.
   */
  private guidesFor(skip: ReadonlyMap<DiagramElement, unknown>, next: Rect, sides: string): ResizeGuide[] {
    const doc = this.canvas.model;
    const xs: number[] = [];
    const ys: number[] = [];
    if (doc !== null) {
      for (const el of doc.elements()) {
        if (skip.has(el)) continue;
        xs.push(el.x, el.x + el.width);
        ys.push(el.y, el.y + el.height);
      }
    }
    const meets = (values: number[], at: number) => values.some((v) => Math.abs(v - at) < 0.5);

    const out: ResizeGuide[] = [];
    const dir = sides;
    if (dir.includes("w")) out.push({ axis: "x", at: next.x, aligned: meets(xs, next.x) });
    if (dir.includes("e")) {
      const at = next.x + next.width;
      out.push({ axis: "x", at, aligned: meets(xs, at) });
    }
    if (dir.includes("n")) out.push({ axis: "y", at: next.y, aligned: meets(ys, next.y) });
    if (dir.includes("s")) {
      const at = next.y + next.height;
      out.push({ axis: "y", at, aligned: meets(ys, at) });
    }
    return out;
  }

  /**
   * Map every element from the original selection box into the new one.
   *
   * Positions and sizes scale proportionally, so the arrangement the author
   * built is preserved — the group is stretched, not re-laid-out.
   */
  private scaleGroup(gesture: ResizeGesture, next: Rect): void {
    const from = gesture.bounds;
    const sx = from.width === 0 ? 1 : next.width / from.width;
    const sy = from.height === 0 ? 1 : next.height / from.height;

    for (const [el, origin] of gesture.origins) {
      el.x = next.x + (origin.x - from.x) * sx;
      el.y = next.y + (origin.y - from.y) * sy;
      el.width = Math.max(20, origin.width * sx);
      el.height = Math.max(16, origin.height * sy);
    }
  }

  private applyMarquee(gesture: MarqueeGesture, e: MouseEvent): void {
    const a = this.canvas.toModel(gesture.startX, gesture.startY);
    const b = this.canvas.toModel(e.clientX, e.clientY);
    const rect: Rect = {
      x: Math.min(a.x, b.x),
      y: Math.min(a.y, b.y),
      width: Math.abs(b.x - a.x),
      height: Math.abs(b.y - a.y),
    };
    this.canvas.marqueeRect = rect;

    const doc = this.canvas.model;
    if (doc === null) return;

    const caught: string[] = [];
    for (const el of doc.elements()) {
      const r = elementRect(el);
      // Fully enclosed only: brushing past an element must not grab it.
      const inside =
        r.x >= rect.x &&
        r.y >= rect.y &&
        r.x + r.width <= rect.x + rect.width &&
        r.y + r.height <= rect.y + rect.height;
      if (inside) caught.push(el.id);
    }

    const ids = gesture.additive ? [...new Set([...gesture.base, ...caught])] : caught;
    this.canvas.selectMany(ids);
  }
}

function snapshotOf(el: DiagramElement): Origin {
  return { x: el.x, y: el.y, width: el.width, height: el.height };
}

/**
 * Apply a drag on one grip to a rectangle.
 *
 * North and west grips move the origin as well as the size, which is what makes
 * resizing from any side behave the way people expect: the opposite edge stays
 * put.
 */
function resizeRect(
  rect: Rect,
  dir: ResizeDirection,
  dx: number,
  dy: number,
  min: { width: number; height: number },
  step: number,
): Rect {
  let { x, y, width, height } = rect;

  if (dir.includes("e")) {
    width = Math.max(min.width, snap(rect.width + dx, step));
  }
  if (dir.includes("s")) {
    height = Math.max(min.height, snap(rect.height + dy, step));
  }
  if (dir.includes("w")) {
    const right = rect.x + rect.width;
    x = Math.min(snap(rect.x + dx, step), right - min.width);
    width = right - x;
  }
  if (dir.includes("n")) {
    const bottom = rect.y + rect.height;
    y = Math.min(snap(rect.y + dy, step), bottom - min.height);
    height = bottom - y;
  }

  return { x, y, width, height };
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

export type { RoleHit };
