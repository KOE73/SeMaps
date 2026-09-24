import { elementRect, entityOf, isContainer } from "../model/types.js";
import type { DiagramEdge, DiagramElement } from "../model/types.js";
import type { DiagramDocument } from "../model/document.js";
import { StyleLibrary } from "../model/StyleLibrary.js";
import { builtinStyleSheet } from "../model/style-defaults.js";
import { PaintRegistry } from "./render/PaintRegistry.js";
import type { Point, Rect, Side } from "../geometry/types.js";
import { Emitter } from "../util/emitter.js";
import { createDefs } from "./defs.js";
import { clear, setAttrs, svg, text } from "./svg.js";
import { Viewport, type ViewportState } from "./Viewport.js";
import { BoxRenderer } from "./render/BoxRenderer.js";
import { ContainerRenderer } from "./render/ContainerRenderer.js";
import type { ElementRenderer, RenderContext, ResolvedContent } from "./render/ElementRenderer.js";
import { ActorRenderer, CylinderRenderer, DiamondRenderer, EllipseRenderer, HexagonRenderer } from "./render/shapes.js";
import { TemplateLibrary } from "../content/TemplateLibrary.js";
import { AssetRegistry } from "../assets/AssetRegistry.js";
import { renderContent, type MemberView } from "../content/ContentRenderer.js";
import { TypeRegistry } from "./render/TypeRegistry.js";
import { DIM } from "./render/styles.js";
import { dashArray, textAttrs } from "./render/textAttrs.js";
import { marquee, resizeHandles, selectionOutline } from "./render/handles.js";
import { UniformPortAssigner } from "./ports/assigners.js";
import { portKey, type PortAssigner, type PortRequest } from "./ports/PortAssigner.js";
import { BezierRouter, type EdgeRouter, type Route } from "./routing/EdgeRouter.js";
import type { Slide } from "./routing/VisibilityGraph.js";
import {
  OrthogonalRouter,
  TreeHorizontalRouter,
  TreeVerticalRouter,
  filletedPath,
  polylinePath,
} from "./routing/routers.js";
import { borderZones, laneZones, solidZone, nudgeWalls, zonesFor, LANE_GAP, SOLID, type RouteScene, type RouteZone } from "./routing/Scene.js";
import { nudgeRoutes } from "./routing/nudge.js";
import type { ResolvedEdgeStyle } from "../model/StyleLibrary.js";
import type { RoutingMode } from "../model/style-types.js";
import { getMarkerOffset } from "./render/PaintRegistry.js";
import { EDGE_ATTR } from "../interaction/roles.js";
import { InteractionController } from "../interaction/InteractionController.js";
import { SourceCodeService } from "../editor/code/SourceCodeService.js";
import { DIAGRAM_CONFIG } from "../constants/diagram-constants.js";

export type SelectionKind = "zone" | "node" | "edge";

/** Named here so a host can build a toggle without importing the style module. */
export type EdgeFamily = "structure" | "flow";

export interface Selection {
  readonly id: string;
  readonly kind: SelectionKind;
}

export interface CanvasEvents {
  /** Selection changed, including to nothing. */
  select: Selection | null;
  /** The model was mutated by a canvas interaction. */
  modelchange: { reason: string };
  /** A direct-manipulation gesture began; a good moment to snapshot. */
  gesturestart: { reason: string };
  /** A gesture finished. */
  gestureend: { reason: string };
  viewport: ViewportState;
  collapse: { id: string; collapsed: boolean };
  openDocEditor: { id: string; kind?: "node" | "zone" | "edge" };
  openCodeViewer: { id: string; codeRef: string; label?: string };
  /** Right click on a box, a line or the empty canvas: whoever owns menus decides what to offer. */
  contextmenu: { target: "element" | "edge" | "canvas"; id: string | null; clientX: number; clientY: number };
}

export interface DiagramCanvasOptions {
  registry?: TypeRegistry;
  portAssigner?: PortAssigner;
  router?: EdgeRouter;
  /** Grid step for snapping, 0 disables. */
  gridStep?: number;
  /** The look of everything. Defaults to the built-in library. */
  styles?: StyleLibrary;
  /**
   * Base for the files the canvas fetches itself — `templates.json` and the
   * content directory. Same value the stores get; see `DiagramEditorOptions`.
   */
  modelsBase?: string;
}

/**
 * The reusable half of the library: everything needed to show a diagram and
 * manipulate it directly, and nothing about where the model came from or where
 * it is saved.
 *
 * Knows nothing about toolbars, inspectors, catalogs, `/api/save` or undo. It
 * reports what happened through events and lets a host decide what that means.
 */
export class DiagramCanvas {
  readonly events = new Emitter<CanvasEvents>();
  readonly viewport: Viewport;
  readonly registry: TypeRegistry;

  /** Snapping step used by interactions, in model units. */
  gridStep: number;

  /**
   * Families of edge currently *not* drawn. Empty means everything shows.
   *
   * In `model-core-full.json` 98 of 119 edges are `implements`/`extends`. That
   * thicket makes the canvas unreadable no matter how the lines are coloured —
   * the only thing that helps is being able to switch the structural family off
   * and look at what happens at runtime.
   */
  hiddenEdgeFamilies = new Set<EdgeFamily>();

  private readonly host: HTMLElement;
  private readonly svgEl: SVGSVGElement;
  private readonly viewportGroup: SVGGElement;
  private readonly zonesLayer: SVGGElement;
  private readonly edgesLayer: SVGGElement;
  private readonly nodesLayer: SVGGElement;
  private readonly overlayLayer: SVGGElement;
  /** Routing debug picture: what the search sees. Empty unless `debugRouting`. */
  private readonly debugLayer: SVGGElement;

  /**
   * Draw what the line search sees: forbidden block zones, priced bands along
   * containers and along lines already drawn, and — for the selected line —
   * its search grid, how far its ends may slide and the spots already taken.
   * A developer's view; nothing in it is saved.
   */
  get debugRouting(): boolean {
    return this._debugRouting;
  }
  set debugRouting(on: boolean) {
    this._debugRouting = on;
    this.render();
  }
  private _debugRouting = false;

  private readonly interaction: InteractionController;

  private doc: DiagramDocument | null = null;
  private styleLibrary: StyleLibrary;
  /**
   * Named content templates, and the pictures they can pull in.
   *
   * Both are read lazily and both survive being empty: a workspace with no
   * `templates.json` and no `content/` draws exactly what it drew before they
   * existed, which is what makes them safe to add to live models.
   */
  private _templates: TemplateLibrary;
  private _assets: AssetRegistry;
  /**
   * Every line shape the canvas can draw, by name.
   *
   * A map rather than a chain of ifs so that the set can grow — an
   * obstacle-avoiding router is a new entry here and nothing else, precisely
   * because no file records the paths any router produced.
   */
  private readonly routers = new Map<RoutingMode, EdgeRouter>([
    ["orthogonal", new OrthogonalRouter()],
    ["tree-horizontal", new TreeHorizontalRouter()],
    ["tree-vertical", new TreeVerticalRouter()],
  ]);
  private paintRegistry!: PaintRegistry;
  private selection: Selection | null = null;
  /**
   * Everything currently selected, including the primary. Group move and group
   * resize operate on this set; the inspector still follows `selection`, since
   * editing many elements at once is a different feature.
   */
  private selectionIds = new Set<string>();
  private collapsed = new Set<string>();
  private activeViewId: string | null = null;
  /**
   * Style tags currently isolated. Empty means no isolation.
   *
   * Not a second highlighting system: a view's `highlightZones`/`highlightNodes`
   * name elements one by one, by hand. A tag names a *domain* — it lives on the
   * style the elements already wear (`WireStyle.tags`), so isolating "llm" dims
   * everything except what a style already colours as belonging to it. No
   * separate per-element tag field exists or is needed; the style a thing wears
   * already says what it is.
   *
   * A set, not one tag: a style can carry several tags at once (`WireStyle.tags`
   * always was an array), because the same subdomain often belongs to more than
   * one classification worth isolating separately — a security-relevant zone
   * inside the LLM domain is both "llm" and "security". Selecting several tags
   * shows the union: anything wearing *any* of them.
   */
  private activeTags = new Set<string>();
  private ghostNodeId: string | null = null;
  private showOverviewShadows = false;

  /** Rubber band in model coordinates while a selection sweep is running. */
  marqueeRect: Rect | null = null;

  private portAssigner: PortAssigner;
  private router: EdgeRouter;

  /** Set while a node is dragged over a container. */
  dropTargetId: string | null = null;

  constructor(host: HTMLElement, options: DiagramCanvasOptions = {}) {
    this.host = host;
    this.gridStep = options.gridStep ?? DIAGRAM_CONFIG.handles.defaultGridStep;
    this.portAssigner = options.portAssigner ?? new UniformPortAssigner();
    this.router = options.router ?? new BezierRouter();
    this.registry = options.registry ?? defaultRegistry();
    this._templates = new TemplateLibrary(options.modelsBase);
    this._assets = new AssetRegistry(options.modelsBase);

    this.zonesLayer = svg("g", { class: "semaps-layer-zones" });
    this.edgesLayer = svg("g", { class: "semaps-layer-edges" });
    this.nodesLayer = svg("g", { class: "semaps-layer-nodes" });
    this.overlayLayer = svg("g", { class: "semaps-layer-overlay" });
    this.debugLayer = svg("g", { class: "semaps-layer-debug", "pointer-events": "none" });

    this.viewportGroup = svg("g", { class: "semaps-viewport" }, [
      this.zonesLayer,
      this.debugLayer,
      this.edgesLayer,
      this.nodesLayer,
      this.overlayLayer,
    ]);

    // Held rather than inlined: gradients and arrow heads are created on demand
    // as styles ask for them, so something has to own the block they land in.
    const defs = createDefs();
    this.paintRegistry = new PaintRegistry(defs);
    this.styleLibrary =
      options.styles ?? StyleLibrary.parse(builtinStyleSheet());

    this.svgEl = svg("svg", { class: "semaps-canvas", xmlns: "http://www.w3.org/2000/svg" }, [
      defs,
      this.viewportGroup,
    ]);

    host.classList.add("semaps-canvas-host");
    host.appendChild(this.svgEl);

    this.viewport = new Viewport(this.viewportGroup);
    this.viewport.changed.on("change", (state) => this.events.emit("viewport", state));

    // Templates are read once; a picture arrives whenever it arrives, and the
    // frame that needed it has long been drawn. Repainting on arrival is why
    // `AssetRegistry.peek` may answer "not yet" without anything going wrong.
    void this._templates.load().then(() => this.render());
    this._assets.onLoaded(() => this.render());

    this.tooltipEl = document.createElement("div");
    this.tooltipEl.className = "semaps-tooltip";
    this.tooltipEl.style.display = "none";
    this.tooltipEl.style.opacity = "0";
    document.body.appendChild(this.tooltipEl);

    this.richTooltipEl = document.createElement("div");
    this.richTooltipEl.className = "semaps-rich-doc-tooltip";
    this.richTooltipEl.style.display = "none";
    this.richTooltipEl.style.opacity = "0";
    document.body.appendChild(this.richTooltipEl);

    this.edgeControlsEl = document.createElement("div");
    this.edgeControlsEl.className = "semaps-edge-controls";
    this.edgeControlsEl.style.display = "none";
    this.edgeControlsEl.style.opacity = "0";
    document.body.appendChild(this.edgeControlsEl);

    this.interaction = new InteractionController(this, host);
  }

  readonly tooltipEl: HTMLElement;
  readonly richTooltipEl: HTMLElement;
  readonly edgeControlsEl: HTMLElement;
  dataLang: string = "ru";

  showTooltip(content: string, x: number, y: number): void {
    this.hideRichTooltip();
    this.tooltipEl.innerHTML = content;
    this.tooltipEl.style.display = "flex";
    this.tooltipEl.style.opacity = "1";

    const pad = DIAGRAM_CONFIG.interaction.tooltipOffset;
    let left = x + pad;
    let top = y + pad;

    const rect = this.tooltipEl.getBoundingClientRect();
    if (left + rect.width > window.innerWidth - 10) {
      left = x - rect.width - pad;
    }
    if (top + rect.height > window.innerHeight - 10) {
      top = y - rect.height - pad;
    }
    left = Math.max(10, left);
    top = Math.max(10, top);

    this.tooltipEl.style.left = `${left}px`;
    this.tooltipEl.style.top = `${top}px`;
  }

  hideTooltip(): void {
    this.tooltipEl.style.display = "none";
    this.tooltipEl.style.opacity = "0";
  }

  showRichTooltip(content: string, x: number, y: number): void {
    this.hideTooltip();
    this.richTooltipEl.innerHTML = content;
    this.richTooltipEl.style.display = "flex";
    this.richTooltipEl.style.opacity = "1";

    const pad = DIAGRAM_CONFIG.interaction.tooltipOffset;
    let left = x + pad;
    let top = y + pad;

    const rect = this.richTooltipEl.getBoundingClientRect();
    if (left + rect.width > window.innerWidth - 10) {
      left = x - rect.width - pad;
    }
    if (top + rect.height > window.innerHeight - 10) {
      top = y - rect.height - pad;
    }
    left = Math.max(10, left);
    top = Math.max(10, top);

    this.richTooltipEl.style.left = `${left}px`;
    this.richTooltipEl.style.top = `${top}px`;
  }

  hideRichTooltip(): void {
    this.richTooltipEl.style.display = "none";
    this.richTooltipEl.style.opacity = "0";
  }

  getEdgeCenter(edgeId: string): { x: number; y: number } | null {
    const edgeGroup = this.svgEl.querySelector<SVGGElement>(`g[data-edge="${edgeId}"]`);
    if (!edgeGroup) return null;
    const label = edgeGroup.querySelector<SVGTextElement>(".semaps-edge-label");
    if (label) {
      const rect = label.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        return { x: rect.left + rect.width / 2, y: rect.top };
      }
    }
    const path = edgeGroup.querySelector<SVGPathElement>(".semaps-edge-line");
    if (path) {
      try {
        const len = path.getTotalLength();
        const pt = path.getPointAtLength(len / 2);
        const ctm = path.getScreenCTM();
        if (ctm) {
          const domPt = new DOMPoint(pt.x, pt.y).matrixTransform(ctm);
          return { x: domPt.x, y: domPt.y };
        }
      } catch {
        // Fallback to coordinates
      }
    }
    return null;
  }

  showEdgeControls(edgeId: string, fallbackX: number, fallbackY: number): void {
    const doc = this.doc;
    if (!doc) return;
    const edge = doc.edge(edgeId);
    if (!edge) return;

    const lang = this.dataLang || "ru";
    const textEntry = doc.getText(edgeId, lang);
    const hasDoc = Boolean(textEntry?.doc?.trim());
    const type = edge.type || "relates";

    this.edgeControlsEl.innerHTML = `
      <span class="semaps-edge-ctrl-type">${escapeCanvasHtml(type)}</span>
      <button type="button" class="semaps-edge-ctrl-btn semaps-edge-doc-btn${hasDoc ? " has-doc" : ""}" data-edge-id="${escapeCanvasHtml(edgeId)}" title="Документация (клик — редактор, наведение — просмотр)">
        ${hasDoc ? "📝" : "📄"}
      </button>
    `;

    this.edgeControlsEl.style.display = "flex";
    this.edgeControlsEl.style.opacity = "1";

    const center = this.getEdgeCenter(edgeId);
    const targetX = center ? center.x : fallbackX;
    const targetY = center ? center.y : fallbackY;

    let left = targetX;
    let top = targetY - 14;
    if (left < 60) left = 60;
    if (left > window.innerWidth - 60) left = window.innerWidth - 60;
    if (top < 35) top = targetY + 28;

    this.edgeControlsEl.style.left = `${left}px`;
    this.edgeControlsEl.style.top = `${top}px`;
  }

  hideEdgeControls(): void {
    this.edgeControlsEl.style.display = "none";
    this.edgeControlsEl.style.opacity = "0";
  }

  hideAllTooltips(): void {
    this.hideTooltip();
    this.hideRichTooltip();
  }

  // ------------------------------------------------------------ public API

  get hostElement(): HTMLElement {
    return this.host;
  }

  setModel(doc: DiagramDocument): void {
    this.doc = doc;
    this.selection = null;
    this.selectionIds.clear();
    this.collapsed = new Set();
    this.activeViewId = doc.views[0]?.id ?? null;
    this.render();
    this.fit();
    this.events.emit("select", null);
    this.validateModelCodeRefs(doc);
  }

  /**
   * Swap in a rebuilt model while keeping the presentation state: viewport,
   * collapse, active view, and the selection re-resolved by id.
   *
   * This is what undo needs — the model is rebuilt from a snapshot, but the
   * user must not be thrown back to a different zoom or lose their place
   * (R-HIST-07).
   */
  replaceModel(doc: DiagramDocument): void {
    this.doc = doc;
    const alive = (id: string): boolean =>
      doc.element(id) !== undefined || doc.edge(id) !== undefined;

    for (const id of [...this.selectionIds]) {
      if (!alive(id)) this.selectionIds.delete(id);
    }
    if (this.selection !== null && !alive(this.selection.id)) {
      this.selection = this.resolveSelection([...this.selectionIds].at(-1) ?? null);
    }
    this.render();
    this.events.emit("select", this.selection);
    // Undo and redo land here: every list built from the model must redraw.
    this.events.emit("modelchange", { reason: "replace" });
    this.validateModelCodeRefs(doc);
  }

  private validateModelCodeRefs(doc: DiagramDocument): void {
    const codeRefs: string[] = [];
    for (const el of doc.elements()) {
      if (typeof el.metadata?.codeRef === "string") {
        const ref = el.metadata.codeRef.trim();
        if (ref) codeRefs.push(ref);
      }
    }
    if (codeRefs.length === 0) return;

    void SourceCodeService.validateCodeRefs(codeRefs).then((hasUpdates) => {
      if (hasUpdates && this.doc === doc) {
        this.render();
      }
    });
  }

  /**
   * The named content-template registry, exposed so a settings panel can list,
   * edit and save templates without the canvas mediating every call — the
   * panel needs `list`/`setText`/`save` directly, and re-renders the canvas
   * itself after a change (ADR_20260903 §2.3).
   */
  get templates(): TemplateLibrary {
    return this._templates;
  }

  /** The named picture registry behind `@Asset`, exposed for the same reason. */
  get assets(): AssetRegistry {
    return this._assets;
  }

  get model(): DiagramDocument | null {
    return this.doc;
  }

  /** The library every element's look is resolved through. */
  get styles(): StyleLibrary {
    return this.styleLibrary;
  }

  /**
   * Swap the library, or redraw after editing a style inside it.
   *
   * Call this after any style edit: a style is shared by everything wearing it,
   * so there is no such thing as repainting one element — which is exactly the
   * property that made styles worth building.
   */
  setStyles(library?: StyleLibrary): void {
    if (library !== undefined) this.styleLibrary = library;
    this.render();
  }

  /** Show or hide a whole family of connections, and redraw. */
  setEdgeFamilyHidden(family: EdgeFamily, hidden: boolean): void {
    if (hidden) this.hiddenEdgeFamilies.add(family);
    else this.hiddenEdgeFamilies.delete(family);
    this.render();
  }

  isEdgeFamilyHidden(family: EdgeFamily): boolean {
    return this.hiddenEdgeFamilies.has(family);
  }

  get activeView(): string | null {
    return this.activeViewId;
  }

  setView(viewId: string | null): void {
    this.activeViewId = viewId;
    this.render();
  }

  /** Tags currently isolated. Empty means no isolation. */
  get highlightTags(): ReadonlySet<string> {
    return this.activeTags;
  }

  /** Add or remove one tag from the isolated set, keeping the rest. */
  toggleHighlightTag(tag: string): void {
    if (this.activeTags.has(tag)) this.activeTags.delete(tag);
    else this.activeTags.add(tag);
    this.render();
  }

  clearHighlightTags(): void {
    if (this.activeTags.size === 0) return;
    this.activeTags.clear();
    this.render();
  }

  get currentGhostNodeId(): string | null {
    return this.ghostNodeId;
  }

  toggleGhostNode(id: string): void {
    this.ghostNodeId = this.ghostNodeId === id ? null : id;
    this.render();
  }

  clearGhostNode(): void {
    if (this.ghostNodeId !== null) {
      this.ghostNodeId = null;
      this.render();
    }
  }

  get isOverviewShadowsEnabled(): boolean {
    return this.showOverviewShadows;
  }

  toggleOverviewShadows(enabled?: boolean): boolean {
    this.showOverviewShadows = enabled !== undefined ? enabled : !this.showOverviewShadows;
    this.render();
    return this.showOverviewShadows;
  }

  /**
   * Every tag worn by a style currently in use, sorted.
   *
   * "In use" — not every tag in the library — because a hundred styles will
   * carry tags for domains this particular diagram never touches, and a picker
   * offering those is a picker offering dead ends.
   */
  tagsInUse(): string[] {
    if (this.doc === null) return [];
    const tags = new Set<string>();
    for (const el of this.doc.elements()) {
      for (const t of this.styleLibrary.tagsOf(this.styleLibrary.blockStyleIdFor(el))) tags.add(t);
    }
    for (const edge of this.doc.edges) {
      for (const t of this.styleLibrary.tagsOf(this.styleLibrary.edgeStyleIdFor(edge))) tags.add(t);
    }
    return [...tags].sort((a, b) => a.localeCompare(b, "ru"));
  }

  select(id: string | null): void {
    this.selection = this.resolveSelection(id);
    this.selectionIds = this.selection === null ? new Set() : new Set([this.selection.id]);
    this.render();
    this.events.emit("select", this.selection);
  }

  /**
   * Add or remove one element from the selection, keeping the rest.
   *
   * The last element added becomes primary, so the inspector follows what the
   * user just touched.
   */
  toggleSelected(id: string): void {
    if (this.selectionIds.has(id)) {
      this.selectionIds.delete(id);
      if (this.selection?.id === id) {
        const next = [...this.selectionIds].at(-1) ?? null;
        this.selection = this.resolveSelection(next);
      }
    } else {
      this.selectionIds.add(id);
      this.selection = this.resolveSelection(id);
    }
    this.render();
    this.events.emit("select", this.selection);
  }

  /** Replace the whole selection at once, as a marquee sweep does. */
  selectMany(ids: readonly string[]): void {
    this.selectionIds = new Set(ids);
    this.selection = this.resolveSelection(ids.at(-1) ?? null);
    this.render();
    this.events.emit("select", this.selection);
  }

  private resolveSelection(id: string | null): Selection | null {
    if (id === null || this.doc === null) return null;
    const el = this.doc.element(id);
    if (el !== undefined) return { id, kind: el.kind };
    if (this.doc.edge(id) !== undefined) return { id, kind: "edge" };

    const relations = this.doc.relations;
    if (Array.isArray(relations)) {
      const rel = relations.find((r) => r.id === id);
      if (rel) return { id, kind: "edge" };
    }
    if (id.startsWith("ghost_")) {
      return { id, kind: "edge" };
    }
    return null;
  }

  get selected(): Selection | null {
    return this.selection;
  }

  /** Ids of every selected element, primary included. */
  get selectedIds(): ReadonlySet<string> {
    return this.selectionIds;
  }

  /** The selected elements, skipping edges and anything already gone. */
  selectedElements(): DiagramElement[] {
    const doc = this.doc;
    if (doc === null) return [];
    const out: DiagramElement[] = [];
    for (const id of this.selectionIds) {
      const el = doc.element(id);
      if (el !== undefined) out.push(el);
    }
    return out;
  }

  /** Union of the visible rectangles of the selected elements. */
  selectionBounds(): Rect | null {
    const elements = this.selectedElements();
    if (elements.length === 0 || this.doc === null) return null;
    const ctx = this.context();
    let out: Rect | null = null;
    for (const el of elements) {
      if (ctx.isHidden(el)) continue;
      const r = this.rendererFor(el).visibleRect(el, ctx);
      out = out === null ? r : unionRect(out, r);
    }
    return out;
  }

  selectedElement(): DiagramElement | null {
    if (this.selection === null || this.selection.kind === "edge") return null;
    return this.doc?.element(this.selection.id) ?? null;
  }

  selectedEdge(): DiagramEdge | null {
    if (this.selection === null || this.selection.kind !== "edge") return null;
    return this.doc?.edge(this.selection.id) ?? null;
  }

  isCollapsed(el: DiagramElement): boolean {
    return this.collapsed.has(el.id);
  }

  toggleCollapse(id: string): void {
    if (this.collapsed.has(id)) this.collapsed.delete(id);
    else this.collapsed.add(id);
    this.render();
    this.events.emit("collapse", { id, collapsed: this.collapsed.has(id) });
  }

  fit(): void {
    this.viewport.fit(this.visibleBounds(), {
      width: this.host.clientWidth,
      height: this.host.clientHeight,
    });
  }

  zoomTo(zoom: number): void {
    this.viewport.zoomTo(zoom);
  }

  zoomBy(factor: number): void {
    this.viewport.zoomBy(factor);
  }

  /** Zoom by `factor`, keeping the model point under the given client coordinates fixed. */
  zoomAtClient(factor: number, clientX: number, clientY: number): void {
    const box = this.host.getBoundingClientRect();
    this.viewport.zoomAt({ x: clientX - box.left, y: clientY - box.top }, factor);
  }

  resetZoom(): void {
    this.viewport.reset();
  }

  setPortAssigner(assigner: PortAssigner): void {
    this.portAssigner = assigner;
    this.render();
  }

  setRouter(router: EdgeRouter): void {
    this.router = router;
    this.render();
  }

  /** Screen point (client coordinates) to model coordinates. */
  toModel(clientX: number, clientY: number): Point {
    const box = this.host.getBoundingClientRect();
    return this.viewport.toModel({ x: clientX - box.left, y: clientY - box.top });
  }

  /** The centre of the visible area, in model coordinates. */
  viewCenter(): Point {
    return this.viewport.toModel({
      x: this.host.clientWidth / 2,
      y: this.host.clientHeight / 2,
    });
  }

  notifyModelChanged(reason: string): void {
    this.render();
    this.events.emit("modelchange", { reason });
  }

  destroy(): void {
    this.interaction.destroy();
    this.svgEl.remove();
    this.events.clear();
  }

  // -------------------------------------------------------------- rendering

  render(): void {
    clear(this.zonesLayer);
    clear(this.edgesLayer);
    clear(this.debugLayer);
    clear(this.nodesLayer);
    clear(this.overlayLayer);
    if (this.doc === null) return;

    const ctx = this.context();

    // Parents before children, so nested containers stack above their parent.
    for (const el of this.doc.elements()) {
      if (ctx.isHidden(el)) continue;
      const renderer = this.rendererFor(el);
      const layer = isContainer(el) ? this.zonesLayer : this.nodesLayer;
      layer.appendChild(renderer.create(el, ctx));

      const overlay = renderer.overlay?.(el, ctx) ?? null;
      if (overlay !== null) this.overlayLayer.appendChild(overlay);
    }

    this.renderEdges(ctx);
    this.renderSelectionOverlay();
  }

  /**
   * Grips and outlines, drawn above everything.
   *
   * One element selected: grips sit on the element. Several: they sit on the
   * union of their boxes, with a dashed outline, and dragging one scales the
   * whole group.
   */
  private renderSelectionOverlay(): void {
    const scale = this.viewport.zoom;

    if (this.marqueeRect !== null) {
      this.overlayLayer.appendChild(marquee(this.marqueeRect, scale));
    }

    const elements = this.selectedElements();
    if (elements.length === 0) return;

    const bounds = this.selectionBounds();
    if (bounds === null) return;

    if (elements.length > 1) {
      this.overlayLayer.appendChild(selectionOutline(bounds, scale));
    } else if (this.collapsed.has(elements[0]!.id)) {
      // A collapsed container shows no grips: resizing it would change a height
      // that is not currently visible.
      return;
    }

    this.overlayLayer.appendChild(resizeHandles(bounds, scale));
  }

  private context(): RenderContext {
    const doc = this.doc;
    if (doc === null) throw new Error("render context requested with no model");
    const view = this.activeViewId === null
      ? undefined
      : doc.views.find((v) => v.id === this.activeViewId);

    const ghostConnectedIds = new Set<string>();
    if (this.ghostNodeId !== null && doc !== null) {
      ghostConnectedIds.add(this.ghostNodeId);
      for (const e of doc.edges) {
        if (e.from === this.ghostNodeId || e.to === this.ghostNodeId) {
          ghostConnectedIds.add(e.from);
          ghostConnectedIds.add(e.to);
        }
      }
      const ghostEl = doc.element(this.ghostNodeId);
      const rawGhostEntityId = ghostEl ? (entityOf(ghostEl)?.id ?? null) : null;
      const relations = doc.relations;
      if (Array.isArray(relations)) {
        for (const rel of relations) {
          const fromMatch = rel.from === this.ghostNodeId || (rawGhostEntityId && rel.from === rawGhostEntityId);
          const toMatch = rel.to === this.ghostNodeId || (rawGhostEntityId && rel.to === rawGhostEntityId);
          if (fromMatch || toMatch) {
            const canvasFrom = fromMatch ? this.ghostNodeId : rel.from;
            const canvasTo = toMatch ? this.ghostNodeId : rel.to;
            if (doc.element(canvasFrom)) ghostConnectedIds.add(canvasFrom);
            if (doc.element(canvasTo)) ghostConnectedIds.add(canvasTo);
          }
        }
      }
    }

    return {
      doc,
      selectedId: this.selection?.id ?? null,
      dropTargetId: this.dropTargetId,
      ghostNodeId: this.ghostNodeId,
      isSelected: (el) => this.selectionIds.has(el.id),
      isCollapsed: (el) => this.collapsed.has(el.id),
      isHidden: (el) => this.collapsedAncestor(el) !== null,
      opacity: (el) => {
        if (this.ghostNodeId !== null) {
          if (isContainer(el)) {
            const desc = doc.descendants(el);
            const containsConnected = ghostConnectedIds.has(el.id) || desc.some((d) => ghostConnectedIds.has(d.id));
            if (!containsConnected) return DIM.zone;
          } else {
            if (!ghostConnectedIds.has(el.id)) return DIM.node;
          }
        }

        const viewOpacity = ((): number => {
          if (view === undefined) return 1;
          if (isContainer(el)) {
            if (view.highlightZones.length === 0) return 1;
            return view.highlightZones.includes(el.id) ? 1 : DIM.zone;
          }
          if (view.highlightNodes.length > 0) {
            return view.highlightNodes.includes(el.id) ? 1 : DIM.node;
          }
          if (view.highlightZones.length > 0) {
            const inHighlighted = doc
              .ancestors(el)
              .some((a) => view.highlightZones.includes(a.id));
            return inHighlighted ? 1 : DIM.node;
          }
          return 1;
        })();

        const tagOpacity = ((): number => {
          if (this.activeTags.size === 0) return 1;
          return this.hasAnyDomainTag(el, this.activeTags) ? 1 : isContainer(el) ? DIM.zone : DIM.node;
        })();

        // The dimmer of the two axes wins: a view and a tag can be active at
        // once, and either one asking for "not this" should be enough to grey
        // an element out.
        return Math.min(viewOpacity, tagOpacity);
      },
      styleOf: (el) => this.styleLibrary.blockStyle(el),
      paints: this.paintRegistry,
      content: (el) => this.resolveContent(el),
    };
  }

  /**
   * Which router draws this edge, chosen from most specific to least:
   * the edge, then the view, then the relation type's style, then whatever
   * the canvas was built with (ADR_20260903 §2.7).
   *
   * The type's style is the level meant to do the work: pinning a shape to a
   * *kind* of relation turns the line's form into a reading cue, while a
   * per-edge override is the exception that has to earn itself.
   */
  private routerFor(edge: DiagramEdge, style: ResolvedEdgeStyle): EdgeRouter {
    const viewChoice = this.doc?.metadata.routing;
    const mode =
      edge.routing ??
      (typeof viewChoice === "string" ? (viewChoice as RoutingMode) : undefined) ??
      style.routing ??
      null;

    if (mode === null) return this.router;
    return this.routers.get(mode) ?? this.router;
  }

  /**
   * "Rip up and reroute": the lines that cross or crowd others are laid again,
   * each seeing every other line. A new route is kept only when it has
   * strictly fewer conflicts, so the passes always settle.
   *
   * One line at a time is not always enough: two lines can hold each other in
   * place — each would move if the other were not there, neither can while it
   * is. So when laying one line again does not help, it and each line it
   * conflicts with are ripped up together and laid in both orders; the pair
   * is kept if the two of them end up with fewer conflicts than before.
   *
   * Bounded by passes, not by time: a time limit would make the picture depend
   * on how fast the machine is. The clock is only a fuse for a huge view.
   */
  private rerouteConflicts(
    order: readonly string[],
    jobs: ReadonlyMap<string, (lanes: readonly RouteZone[], taken: ReadonlyMap<string, number[]>) => Route>,
    routes: Map<string, Route>,
    lanesOf: Map<string, RouteZone[]>,
    endsOf: Map<string, { fromKey: string; from: number; toKey: string; to: number }>,
    record: (id: string, route: Route, fromKey: string, fromSide: Side, toKey: string, toSide: Side) => void,
  ): void {
    const MAX_PASSES = 3;
    const FUSE_MS = 40;
    const started = performance.now();
    const sideOf = (key: string) => key.slice(key.lastIndexOf("#") + 1) as Side;
    const along = (side: Side, p: Point) => (side === "north" || side === "south" ? p.x : p.y);

    /** Conflicts of `pts` with every line except those in `skip`, with some lines replaced. */
    const score = (
      id: string,
      pts: readonly Point[] | undefined,
      replaced: ReadonlyMap<string, readonly Point[] | undefined> = new Map(),
    ): number => {
      if (!pts || pts.length < 2) return 0;
      let n = 0;
      for (const other of order) {
        if (other === id) continue;
        const theirs = replaced.has(other) ? replaced.get(other) : routes.get(other)?.points;
        if (theirs && theirs.length >= 2) n += conflicts(pts, theirs);
      }
      return n;
    };

    /** The lanes and taken spots of every line except the ones being laid again. */
    const surroundings = (without: ReadonlySet<string>) => {
      const lanes: RouteZone[] = [];
      for (const [other, zones] of lanesOf) if (!without.has(other)) lanes.push(...zones);
      const takenNow = new Map<string, number[]>();
      for (const [other, e] of endsOf) {
        if (without.has(other)) continue;
        pushTaken(takenNow, e.fromKey, e.from);
        pushTaken(takenNow, e.toKey, e.to);
      }
      return { lanes, takenNow };
    };

    const keep = (id: string, route: Route) => {
      const ends = endsOf.get(id);
      if (!ends) return;
      record(id, route, ends.fromKey, sideOf(ends.fromKey), ends.toKey, sideOf(ends.toKey));
    };

    /** Lay `first` then `second` from scratch; both ripped up, the rest in place. */
    const layPair = (first: string, second: string): [Route, Route] | null => {
      const jobA = jobs.get(first);
      const jobB = jobs.get(second);
      const endsA = endsOf.get(first);
      if (!jobA || !jobB || !endsA) return null;
      const { lanes, takenNow } = surroundings(new Set([first, second]));
      const a = jobA(lanes, takenNow);
      const pa = a.points;
      if (pa && pa.length >= 2) {
        lanes.push(...laneZones(pa, `lane:${first}`));
        pushTaken(takenNow, endsA.fromKey, along(sideOf(endsA.fromKey), pa[0]!));
        pushTaken(takenNow, endsA.toKey, along(sideOf(endsA.toKey), pa[pa.length - 1]!));
      }
      return [a, jobB(lanes, takenNow)];
    };

    for (let pass = 0; pass < MAX_PASSES; pass++) {
      let improved = false;
      for (const id of order) {
        if (performance.now() - started > FUSE_MS) return;
        const job = jobs.get(id);
        const mine = routes.get(id)?.points;
        const before = score(id, mine);
        if (!job || !endsOf.has(id) || before === 0) continue;

        // Alone first: the cheap move that usually suffices.
        const { lanes, takenNow } = surroundings(new Set([id]));
        const next = job(lanes, takenNow);
        if (score(id, next.points) < before) {
          keep(id, next);
          improved = true;
          continue;
        }

        // Then together with each line it is stuck against, in both orders.
        for (const other of order) {
          if (other === id || !endsOf.has(other)) continue;
          const theirs = routes.get(other)?.points;
          if (!mine || !theirs || conflicts(mine, theirs) === 0) continue;
          const pairBefore = before + score(other, theirs) - conflicts(mine, theirs);
          let done = false;
          for (const [first, second] of [[id, other], [other, id]] as const) {
            const laid = layPair(first, second);
            if (!laid) continue;
            const [a, b] = laid;
            const replaced = new Map([[first, a.points], [second, b.points]]);
            const after =
              score(first, a.points, replaced) + score(second, b.points, replaced) -
              (a.points && b.points ? conflicts(a.points, b.points) : 0);
            if (after < pairBefore) {
              keep(first, a);
              keep(second, b);
              improved = done = true;
              break;
            }
          }
          if (done) break;
        }
      }
      if (!improved) return;
    }
  }

  /**
   * The routing debug picture. Colours carry the meaning, the tooltip of each
   * shape its price: red — may not enter; orange — a container's frame band;
   * blue — a band along a line already drawn; for the selected line, grey
   * grid lines, green slide ranges on its two sides and black ticks where
   * other lines already meet those sides.
   */
  private drawRoutingDebug(scene: RouteScene, lanes: readonly RouteZone[], edge: DebugEdge | null): void {
    const scale = this.viewport.zoom || 1;
    const px = (n: number) => n / scale;
    const rect = (z: RouteZone, fill: string, stroke: string, title: string) => {
      const r = svg("rect", {
        x: z.rect.x, y: z.rect.y, width: z.rect.width, height: z.rect.height,
        fill, stroke, "stroke-width": px(1), "stroke-dasharray": `${px(4)} ${px(3)}`,
      });
      const t = svg("title");
      t.textContent = title;
      r.appendChild(t);
      this.debugLayer.appendChild(r);
    };
    for (const z of scene.zones) {
      if (z.weight === SOLID) rect(z, "rgba(220,38,38,0.10)", "rgba(220,38,38,0.7)", `нельзя: ${z.ownerId}`);
      else rect(z, "rgba(234,88,12,0.14)", "rgba(234,88,12,0.6)", `рамка ${z.ownerId}: вес ${z.weight} за единицу длины`);
    }
    for (const z of lanes) rect(z, "rgba(37,99,235,0.10)", "rgba(37,99,235,0.45)", `линия ${z.ownerId.replace(/^lane:/, "")}: вес ${z.weight} вдоль`);

    if (edge === null) return;
    if (edge.grid) {
      const all = [...scene.zones, ...lanes].map((z) => z.rect);
      const x0 = Math.min(edge.fromRect.x, edge.toRect.x, ...all.map((r) => r.x)) - 40;
      const x1 = Math.max(edge.fromRect.x + edge.fromRect.width, edge.toRect.x + edge.toRect.width, ...all.map((r) => r.x + r.width)) + 40;
      const y0 = Math.min(edge.fromRect.y, edge.toRect.y, ...all.map((r) => r.y)) - 40;
      const y1 = Math.max(edge.fromRect.y + edge.fromRect.height, edge.toRect.y + edge.toRect.height, ...all.map((r) => r.y + r.height)) + 40;
      const line = (xa: number, ya: number, xb: number, yb: number) =>
        this.debugLayer.appendChild(svg("line", { x1: xa, y1: ya, x2: xb, y2: yb, stroke: "rgba(100,116,139,0.45)", "stroke-width": px(0.75) }));
      for (const x of edge.grid.xs) line(x, y0, x, y1);
      for (const y of edge.grid.ys) line(x0, y, x1, y);
    }
    const side = (r: Rect, s: Side, slide: Slide | undefined) => {
      if (!slide) return;
      const horizontal = s === "north" || s === "south";
      const fixed = s === "north" ? r.y : s === "south" ? r.y + r.height : s === "west" ? r.x : r.x + r.width;
      const seg = horizontal
        ? { x1: slide.lo, y1: fixed, x2: slide.hi, y2: fixed }
        : { x1: fixed, y1: slide.lo, x2: fixed, y2: slide.hi };
      const g = svg("line", { ...seg, stroke: "rgba(22,163,74,0.9)", "stroke-width": px(4), "stroke-linecap": "round" });
      const t = svg("title");
      t.textContent = `конец может скользить: ${Math.round(slide.lo)}…${Math.round(slide.hi)}`;
      g.appendChild(t);
      this.debugLayer.appendChild(g);
      for (const v of slide.taken ?? []) {
        const tick = horizontal
          ? { x1: v, y1: fixed - px(8), x2: v, y2: fixed + px(8) }
          : { x1: fixed - px(8), y1: v, x2: fixed + px(8), y2: v };
        this.debugLayer.appendChild(svg("line", { ...tick, stroke: "#111", "stroke-width": px(2) }));
      }
    };
    side(edge.fromRect, edge.fromSide, edge.fromSlide);
    side(edge.toRect, edge.toSide, edge.toSlide);
  }

  /**
   * How far an end may slide along its block's side: the side minus its
   * rounded corners. Rectangles only — on an ellipse or a diamond a point
   * moved along the bounding side is no longer on the outline.
   */
  private slidesFor(
    owner: DiagramElement,
    rect: Rect,
    side: Side,
    inset: number,
    key: "fromSlide" | "toSlide",
    taken: ReadonlyMap<string, number[]>,
  ): Partial<Record<"fromSlide" | "toSlide", Slide>> {
    const shape = this.styleLibrary.blockStyle(owner).shape ?? "rect";
    if (shape !== "rect") return {};
    const margin = Math.max(inset, 0) + 6;
    const horizontal = side === "north" || side === "south";
    const lo = (horizontal ? rect.x : rect.y) + margin;
    const hi = (horizontal ? rect.x + rect.width : rect.y + rect.height) - margin;
    if (!(lo <= hi)) return {};
    return { [key]: { lo, hi, taken: taken.get(`${owner.id}#${side}`) ?? [] } };
  }

  /**
   * Pull apart routes that ended up in the same corridor.
   *
   * Each route was found on its own and knows nothing of its neighbours, so two
   * of them can legitimately choose the same lane and draw as a single line —
   * at which point the reader loses a relation, not a decoration. Separation is
   * therefore its own pass over all the routes at once, with the scene's solid
   * zones acting as the walls a lane may not be pushed into.
   *
   * Curved routes carry no polyline and simply sit this pass out.
   */
  private separateSharedCorridors(routes: Map<string, Route>, scene: RouteScene): void {
    const nudgeable = [...routes.entries()]
      .filter(([, route]) => route.points !== undefined && route.points.length > 2)
      .map(([id, route]) => ({ id, points: route.points! }));
    if (nudgeable.length < 2) return;

    const moved = nudgeRoutes({ routes: nudgeable, walls: nudgeWalls(scene.zones), gap: LANE_GAP });
    for (const [id, points] of moved) {
      const route = routes.get(id);
      if (route === undefined || points.length < 2) continue;
      // The path is rebuilt rather than patched: it is a rendering of the
      // points, and letting the two drift apart is how a line ends up drawn
      // somewhere its own geometry says it is not.
      routes.set(id, {
        ...route,
        points,
        path: route.corners === "rounded" ? filletedPath(points) : polylinePath(points),
      });
    }
  }

  /**
   * The elements one edge is allowed to ignore: its own two ends, and every
   * container holding either of them.
   *
   * Without the ancestors a line could not leave its own zone — the band around
   * that zone's outline would price the only way out. Derived by filtering the
   * shared scene rather than rebuilding it, so the scene stays one thing built
   * once (ADR_20260903 §2.8).
   */
  private exclusionsFor(from: DiagramElement, to: DiagramElement): Set<string> {
    const ids = new Set<string>();
    for (const start of [from, to]) {
      let cursor: DiagramElement | null = start;
      while (cursor !== null) {
        ids.add(cursor.id);
        cursor = cursor.parent;
      }
    }
    return ids;
  }

  /**
   * The renderer that draws this element, outline included.
   *
   * Shape comes from the resolved style, so every call site asks the same
   * question the same way — and a zone, whose renderer is chosen by kind,
   * is unaffected by a block shape it never had.
   */
  private rendererFor(el: DiagramElement): ElementRenderer {
    return this.registry.resolve(el, this.styleLibrary.blockStyle(el).shape);
  }

  /**
   * Which template this element draws with, compiled, plus what it draws.
   *
   * The cascade is placement, then style, then nothing (ADR_20260903 §2.2).
   * "Nothing" is a real answer and the common one: a workspace with no
   * `templates.json` keeps the caption-and-subtitle look it always had.
   */
  /**
   * How tall the box must be for its content template to fit inside it, or
   * null when it draws no template. Measured, not guessed: the same renderer
   * that paints the box lays the rows out, off-screen.
   */
  contentHeight(el: DiagramElement): number | null {
    const content = this.resolveContent(el);
    if (content === null) return null;
    const style = this.styleLibrary.blockStyle(el);
    const box = { x: el.x, y: el.y, width: el.width, height: el.height };
    const drawn = renderContent(content.tree, box, style, content.data, {
      x: DIAGRAM_CONFIG.node.padX,
      top: DIAGRAM_CONFIG.node.contentTop,
    });
    return Math.ceil(drawn.usedHeight + DIAGRAM_CONFIG.node.padX);
  }

  /**
   * A small picture of this box as it would look with another template
   * (`null` — the style's) or another style, for menus to show before anyone
   * commits to it. Drawn by the same renderer, on a copy; the model is untouched.
   */
  previewElement(el: DiagramElement, change: { template?: string | null; styleId?: string | null }): SVGSVGElement {
    const copy: DiagramElement = {
      ...el,
      x: 0,
      y: 0,
      children: [],
      parent: null,
      metadata: { ...el.metadata },
    };
    if (change.template !== undefined) {
      if (change.template === null) delete copy.metadata.template;
      else copy.metadata.template = change.template;
    }
    if (change.styleId !== undefined) copy.styleId = change.styleId ?? undefined;
    copy.height = Math.max(el.height, this.contentHeight(copy) ?? 0);

    const renderer = this.rendererFor(copy);
    const base = this.context();
    const ctx: RenderContext = {
      ...base,
      selectedId: null,
      dropTargetId: null,
      ghostNodeId: null,
      isSelected: () => false,
      isCollapsed: () => false,
      opacity: () => 1,
      isHidden: () => false,
      // The copy's style: `styleOf` resolves by element, and the copy may wear another.
      styleOf: (e) => this.styleLibrary.blockStyle(e),
    };
    const g = renderer.create(copy, ctx);
    const pad = 3;
    const out = svg("svg", {
      class: "semaps-mini",
      viewBox: `${-pad} ${-pad} ${copy.width + pad * 2} ${copy.height + pad * 2}`,
      width: String(Math.round(copy.width * 0.6)),
      height: String(Math.round(copy.height * 0.6)),
    });
    out.appendChild(g);
    return out as SVGSVGElement;
  }

  private resolveContent(el: DiagramElement): ResolvedContent | null {
    const placement = typeof el.metadata.template === "string" ? el.metadata.template : undefined;
    const id = placement ?? this.styleLibrary.blockStyle(el).template ?? null;
    if (id === null) return null;

    const compiled = this._templates.get(id);
    if (compiled === undefined) return null;

    const entity = entityOf(el);
    const members = Array.isArray(entity?.members)
      ? entity.members.map((m) => (typeof m === "string" ? { name: m } : (m as MemberView)))
      : [];

    return {
      tree: compiled.tree,
      data: {
        name: el.label,
        description: typeof el.metadata.description === "string" ? el.metadata.description : undefined,
        members,
        asset: (assetId) => this._assets.peek(assetId),
      },
    };
  }

  /**
   * Whether an element belongs to a tagged domain — its own style, or any
   * ancestor zone's.
   *
   * A domain tag lives on one zone's style, not on every element inside it: a
   * node three levels deep does not carry its own copy of "llm", it belongs to
   * `block.llm` by containment, the same way it belongs to `block.llm`'s
   * colour without carrying that colour itself. Without the ancestor walk, the
   * root zone of a tagged domain would light up and everything nested inside
   * it — its own child zones, every node — would stay dimmed, which is the
   * opposite of what isolating a domain is for.
   */
  private hasAnyDomainTag(el: DiagramElement, tags: ReadonlySet<string>): boolean {
    const own = this.styleLibrary.tagsOf(this.styleLibrary.blockStyleIdFor(el));
    if (own.some((t) => tags.has(t))) return true;
    if (this.doc === null) return false;
    return this.doc.ancestors(el).some((a) =>
      this.styleLibrary.tagsOf(this.styleLibrary.blockStyleIdFor(a)).some((t) => tags.has(t)),
    );
  }

  /**
   * The outermost collapsed ancestor, which is the container actually visible
   * on screen when several nested containers are collapsed at once.
   */
  private collapsedAncestor(el: DiagramElement): DiagramElement | null {
    if (this.doc === null) return null;
    let found: DiagramElement | null = null;
    for (const ancestor of this.doc.ancestors(el)) {
      if (this.collapsed.has(ancestor.id)) found = ancestor;
    }
    return found;
  }

  /** Where an edge end attaches: the element itself, or the collapsed container hiding it. */
  private anchorFor(el: DiagramElement): { owner: DiagramElement; rect: Rect } {
    const hidden = this.collapsedAncestor(el);
    const owner = hidden ?? el;
    const renderer = this.rendererFor(owner);
    return { owner, rect: renderer.visibleRect(owner, this.context()) };
  }

  private renderEdges(ctx: RenderContext): void {
    const doc = this.doc;
    if (doc === null) return;

    const view = this.activeViewId === null
      ? undefined
      : doc.views.find((v) => v.id === this.activeViewId);

    interface Resolved {
      edge: DiagramEdge;
      from: { owner: DiagramElement; rect: Rect };
      to: { owner: DiagramElement; rect: Rect };
      isPotential?: boolean;
    }

    const resolved: Resolved[] = [];
    for (const edge of doc.edges) {
      // Filtered before ports are assigned, not while drawing: a hidden edge
      // that still claimed a port would push the visible ones off centre for
      // no reason anyone could see.
      if (this.hiddenEdgeFamilies.has(this.styleLibrary.edgeStyle(edge).family)) continue;

      const fromEl = doc.element(edge.from);
      const toEl = doc.element(edge.to);
      // An edge naming a missing element is silently skipped (R-MODEL-05).
      if (fromEl === undefined || toEl === undefined) continue;

      const from = this.anchorFor(fromEl);
      const to = this.anchorFor(toEl);
      // Both ends collapsed into the same container: nothing to show.
      if (from.owner === to.owner) continue;

      resolved.push({ edge, from, to, isPotential: false });
    }

    // When ghost focus is active on a node or global overview is enabled, resolve hidden/potential relations
    if (this.ghostNodeId !== null || this.showOverviewShadows) {
      const ghostEl = this.ghostNodeId !== null ? doc.element(this.ghostNodeId) : null;
      const rawGhostEntityId = ghostEl ? (entityOf(ghostEl)?.id ?? null) : null;
      const relations = doc.relations;

      // Fast lookup for canvas element ID by entity ID / element ID
      const entityToCanvasId = new Map<string, string>();
      for (const el of doc.elements()) {
        entityToCanvasId.set(el.id, el.id);
        const rawId = entityOf(el)?.id;
        if (rawId) entityToCanvasId.set(rawId, el.id);
      }

      if (Array.isArray(relations)) {
        for (const rel of relations) {
          const canvasFrom = entityToCanvasId.get(rel.from);
          const canvasTo = entityToCanvasId.get(rel.to);
          if (!canvasFrom || !canvasTo || canvasFrom === canvasTo) continue;

          const fromEl = doc.element(canvasFrom);
          const toEl = doc.element(canvasTo);
          if (fromEl === undefined || toEl === undefined) continue;

          const isNodeGhostMatch =
            this.ghostNodeId !== null &&
            (canvasFrom === this.ghostNodeId ||
              canvasTo === this.ghostNodeId ||
              (rawGhostEntityId && (rel.from === rawGhostEntityId || rel.to === rawGhostEntityId)));

          let shouldInclude = false;
          if (isNodeGhostMatch) {
            // Local focus always shows all relations of the focused node
            shouldInclude = true;
          } else if (this.showOverviewShadows) {
            // Global overview includes relations whose style has overview === true
            const relType = rel.type || rel.relation || "relates";
            const edgeStyle = this.styleLibrary.resolveEdge(rel.styleId || relType);
            if (edgeStyle.overview) {
              shouldInclude = true;
            }
          }

          if (!shouldInclude) continue;

          const relType = rel.type || rel.relation || "relates";
          const alreadyPresent = resolved.some((r) => {
            if (rel.id && r.edge.id === rel.id) return true;
            return (
              r.edge.from === canvasFrom &&
              r.edge.to === canvasTo &&
              r.edge.type === relType &&
              r.edge.label === (rel.label || "")
            );
          });

          if (!alreadyPresent) {
            const from = this.anchorFor(fromEl);
            const to = this.anchorFor(toEl);
            if (from.owner !== to.owner) {
              const potentialEdge: DiagramEdge = {
                id: rel.id || `ghost_${canvasFrom}_${canvasTo}_${relType}_${resolved.length}`,
                from: canvasFrom,
                to: canvasTo,
                type: relType,
                label: rel.label || "",
                styleId: rel.styleId,
              };
              resolved.push({ edge: potentialEdge, from, to, isPotential: true });
            }
          }
        }
      }
    }

    const requests: PortRequest[] = [];
    for (const r of resolved) {
      const fromStyle = this.styleLibrary.blockStyle(r.from.owner);
      const toStyle = this.styleLibrary.blockStyle(r.to.owner);
      const fromInset = this.rendererFor(r.from.owner).cornerInset?.("east", fromStyle) ?? fromStyle.radius;
      const toInset = this.rendererFor(r.to.owner).cornerInset?.("west", toStyle) ?? toStyle.radius;

      requests.push({
        edgeId: r.edge.id, end: "from", ownerId: r.from.owner.id,
        ownerRect: r.from.rect, otherRect: r.to.rect, edgeType: r.edge.type,
        ownerInset: fromInset,
        otherInset: toInset,
      });
      requests.push({
        edgeId: r.edge.id, end: "to", ownerId: r.to.owner.id,
        ownerRect: r.to.rect, otherRect: r.from.rect, edgeType: r.edge.type,
        ownerInset: toInset,
        otherInset: fromInset,
      });
    }
    const ports = this.portAssigner.assign(requests);

    // The scene: everything in the way, priced, built once per repaint
    // (ADR_20260903 §2.8). A block forbids entry outright; a container gives a
    // band along its outline that costs per unit travelled, which is what makes
    // crossing it cheap and hugging it expensive without a rule saying so.
    const zones: RouteZone[] = [];
    for (const el of doc.elements()) {
      if (ctx.isHidden(el)) continue;
      const rect = this.rendererFor(el).visibleRect(el, ctx);
      if (isContainer(el)) zones.push(...borderZones(rect, el.id));
      else zones.push(solidZone(rect, el.id));
    }
    const scene: RouteScene = { zones };

    // Routes are computed for the whole picture before any of them is drawn,
    // because separating lines that share a corridor is a decision about
    // several routes at once — no amount of improving one route in isolation
    // can stop two of them from merging into one stroke.
    const routes = new Map<string, Route>();
    // Where lines already meet each block side, so the next one does not land on top of them.
    const taken = new Map<string, number[]>();
    // Lines already drawn, as bands the next ones would rather not follow — kept per line,
    // so that a line being laid again sees everyone but itself.
    const lanesOf = new Map<string, RouteZone[]>();
    const endsOf = new Map<string, { fromKey: string; from: number; toKey: string; to: number }>();
    const jobs = new Map<string, (lanes: readonly RouteZone[], taken: ReadonlyMap<string, number[]>) => Route>();
    const order: string[] = [];
    let debugEdge: DebugEdge | null = null;

    const record = (id: string, route: Route, fromKey: string, fromSide: Side, toKey: string, toSide: Side) => {
      routes.set(id, route);
      const pts = route.points;
      if (!pts || pts.length < 2) return;
      const along = (side: Side, p: Point) => (side === "north" || side === "south" ? p.x : p.y);
      endsOf.set(id, { fromKey, from: along(fromSide, pts[0]!), toKey, to: along(toSide, pts[pts.length - 1]!) });
      lanesOf.set(id, laneZones(pts, `lane:${id}`));
    };

    for (const r of resolved) {
      const fromSlot = ports.get(portKey(r.edge.id, "from"));
      const toSlot = ports.get(portKey(r.edge.id, "to"));
      if (fromSlot === undefined || toSlot === undefined) continue;

      const fromStyle = this.styleLibrary.blockStyle(r.from.owner);
      const toStyle = this.styleLibrary.blockStyle(r.to.owner);
      const fromInset = this.rendererFor(r.from.owner).cornerInset?.(fromSlot.side, fromStyle) ?? fromStyle.radius;
      const toInset = this.rendererFor(r.to.owner).cornerInset?.(toSlot.side, toStyle) ?? toStyle.radius;

      const edgeStyle = this.styleLibrary.edgeStyle(r.edge);
      const router = this.routerFor(r.edge, edgeStyle);
      const watched = this._debugRouting && this.selection?.kind === "edge" && this.selection.id === r.edge.id;
      const fromKey = `${r.from.owner.id}#${fromSlot.side}`;
      const toKey = `${r.to.owner.id}#${toSlot.side}`;
      const base = {
        from: this.rendererFor(r.from.owner).pointAt(r.from.rect, fromSlot),
        to: this.rendererFor(r.to.owner).pointAt(r.to.rect, toSlot),
        fromSide: fromSlot.side, toSide: toSlot.side,
        fromRect: r.from.rect, toRect: r.to.rect,
        fromInset, toInset,
        fromMarkerOffset: getMarkerOffset(edgeStyle.source.shape, edgeStyle.source.size ?? DIAGRAM_CONFIG.routing.defaultMarkerSize),
        toMarkerOffset: getMarkerOffset(edgeStyle.target.shape, edgeStyle.target.size ?? DIAGRAM_CONFIG.routing.defaultMarkerSize),
      };
      const own = zonesFor(scene, this.exclusionsFor(r.from.owner, r.to.owner));
      const job = (lanes: readonly RouteZone[], takenNow: ReadonlyMap<string, number[]>): Route => {
        const fromSlide = this.slidesFor(r.from.owner, r.from.rect, fromSlot.side, fromInset, "fromSlide", takenNow);
        const toSlide = this.slidesFor(r.to.owner, r.to.rect, toSlot.side, toInset, "toSlide", takenNow);
        if (watched) {
          debugEdge = {
            fromRect: r.from.rect, toRect: r.to.rect,
            fromSide: fromSlot.side, toSide: toSlot.side,
            fromSlide: fromSlide.fromSlide, toSlide: toSlide.toSlide,
            grid: null,
          };
        }
        return router.route({
          ...base,
          zones: [...own, ...lanesNear(lanes, r.from.rect, r.to.rect)],
          ...fromSlide,
          ...toSlide,
          ...(watched ? { onGrid: (xs: readonly number[], ys: readonly number[]) => { if (debugEdge) debugEdge.grid = { xs, ys }; } } : {}),
        });
      };
      jobs.set(r.edge.id, job);
      order.push(r.edge.id);

      // First pass: in order, each line seeing only the ones before it.
      const route = job([...lanesOf.values()].flat(), taken);
      record(r.edge.id, route, fromKey, fromSlot.side, toKey, toSlot.side);
      const ends = endsOf.get(r.edge.id);
      if (ends) {
        pushTaken(taken, ends.fromKey, ends.from);
        pushTaken(taken, ends.toKey, ends.to);
      }
    }

    // Later passes: lay again every line that crosses or crowds another, now
    // seeing all the others, not only the ones laid before it. The first pass
    // depends on order; this is what takes the order out of it.
    this.rerouteConflicts(order, jobs, routes, lanesOf, endsOf, record);
    this.separateSharedCorridors(routes, scene);
    if (this._debugRouting) this.drawRoutingDebug(scene, [...lanesOf.values()].flat(), debugEdge);

    for (const r of resolved) {
      const fromSlot = ports.get(portKey(r.edge.id, "from"));
      const toSlot = ports.get(portKey(r.edge.id, "to"));
      const route = routes.get(r.edge.id);
      if (fromSlot === undefined || toSlot === undefined || route === undefined) continue;

      const style = this.styleLibrary.edgeStyle(r.edge);
      const viewHighlighted =
        view === undefined || view.highlightNodes.length === 0
          ? true
          : view.highlightNodes.includes(r.edge.from) && view.highlightNodes.includes(r.edge.to);
      const tagHighlighted =
        this.activeTags.size === 0 ||
        this.hasAnyDomainTag(r.from.owner, this.activeTags) ||
        this.hasAnyDomainTag(r.to.owner, this.activeTags);

      const isGhostEdge = this.ghostNodeId !== null && (r.edge.from === this.ghostNodeId || r.edge.to === this.ghostNodeId);
      const isGhostActive = this.ghostNodeId !== null;

      let edgeOpacity = viewHighlighted && tagHighlighted ? 1 : DIM.edge;
      if (isGhostActive) {
        if (r.isPotential) {
          edgeOpacity = 0.6;
        } else {
          edgeOpacity = isGhostEdge ? 1 : 0.12;
        }
      }

      const strokeColor = r.isPotential ? "var(--accent)" : style.line.color;
      const strokeWidth = r.isPotential ? 1.6 : style.line.width;
      const strokeDash = r.isPotential ? "5 4" : dashArray(style.line.dash);

      const g = svg("g", {
        class: `semaps-edge${this.selection?.id === r.edge.id ? " is-selected" : ""}${isGhostEdge && !r.isPotential ? " is-ghost-focus" : ""}${r.isPotential ? " is-ghost-potential" : ""}`,
        [EDGE_ATTR]: r.edge.id,
        opacity: edgeOpacity,
        style: r.isPotential ? "cursor: pointer;" : undefined,
      });

      // Invisible fat hit area (18px wide) for easy click/hover on thin or dotted lines
      g.appendChild(
        svg("path", {
          class: "semaps-edge-hit",
          d: route.path,
          fill: "none",
          stroke: "transparent",
          "stroke-width": 18,
          "stroke-linecap": "round",
          "stroke-linejoin": "round",
        }),
      );

      g.appendChild(
        svg("path", {
          class: "semaps-edge-glow",
          d: route.path,
          fill: "none",
          stroke: strokeColor,
          "stroke-width": strokeWidth + 5,
          "stroke-linecap": "round",
          "stroke-linejoin": "round",
        }),
      );

      g.appendChild(
        svg("path", {
          class: "semaps-edge-line",
          d: route.path,
          fill: "none",
          stroke: strokeColor,
          "stroke-width": strokeWidth,
          "stroke-dasharray": strokeDash,
          "stroke-opacity": style.line.opacity === 1 ? null : style.line.opacity,
          "marker-start": ctx.paints.marker(style.source, strokeColor),
          "marker-end": ctx.paints.marker(style.target, strokeColor),
        }),
      );

      if (r.edge.label !== "" && style.label.show) {
        g.appendChild(
          text(
            {
              ...textAttrs(style.label),
              class: "semaps-edge-label",
              x: route.labelAt.x,
              y: route.labelAt.y,
              "text-anchor": style.label.align,
            },
            r.edge.label,
          ),
        );
      }

      // Cardinality/role captions at the ends (ADR_20260903 §2.6). They share
      // the centre label's style — a separate style axis for two more strings
      // is not earned — and the router's own anchor, since only the router
      // knows where its path actually leaves each port.
      if (r.edge.fromLabel && style.label.show && route.fromLabelAt !== undefined) {
        g.appendChild(
          text(
            {
              ...textAttrs(style.label),
              class: "semaps-edge-label semaps-edge-label-from",
              x: route.fromLabelAt.x,
              y: route.fromLabelAt.y,
              "text-anchor": style.label.align,
            },
            r.edge.fromLabel,
          ),
        );
      }
      if (r.edge.toLabel && style.label.show && route.toLabelAt !== undefined) {
        g.appendChild(
          text(
            {
              ...textAttrs(style.label),
              class: "semaps-edge-label semaps-edge-label-to",
              x: route.toLabelAt.x,
              y: route.toLabelAt.y,
              "text-anchor": style.label.align,
            },
            r.edge.toLabel,
          ),
        );
      }

      this.edgesLayer.appendChild(g);
    }
  }

  /** Bounds of everything, using visible rectangles so collapsed containers count small. */
  private visibleBounds(): Rect | null {
    if (this.doc === null) return null;
    const ctx = this.context();
    let out: Rect | null = null;
    for (const el of this.doc.elements()) {
      if (ctx.isHidden(el)) continue;
      const r = this.rendererFor(el).visibleRect(el, ctx);
      out = out === null ? r : unionRect(out, r);
    }
    return out;
  }

  /** Used by the interaction controller. */
  elementRectOf(el: DiagramElement): Rect {
    return elementRect(el);
  }

  setDropTarget(id: string | null): void {
    if (this.dropTargetId === id) return;
    this.dropTargetId = id;
    this.render();
  }

  emitGestureStart(reason: string): void {
    this.events.emit("gesturestart", { reason });
  }

  emitGestureEnd(reason: string): void {
    this.events.emit("gestureend", { reason });
  }
}

function unionRect(a: Rect, b: Rect): Rect {
  const x = Math.min(a.x, b.x);
  const y = Math.min(a.y, b.y);
  return {
    x,
    y,
    width: Math.max(a.x + a.width, b.x + b.width) - x,
    height: Math.max(a.y + a.height, b.y + b.height) - y,
  };
}

function escapeCanvasHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

export function defaultRegistry(): TypeRegistry {
  const registry = new TypeRegistry();
  const box: ElementRenderer = new BoxRenderer();
  const container: ElementRenderer = new ContainerRenderer();
  registry.registerDefault("node", box);
  registry.registerDefault("zone", container);

  // Outlines, keyed by what a style may ask for. "rect" is deliberately absent:
  // it is the default renderer, and registering it here would only give the
  // same answer by a longer route.
  registry.registerShape("ellipse", new EllipseRenderer());
  registry.registerShape("diamond", new DiamondRenderer());
  registry.registerShape("cylinder", new CylinderRenderer());
  registry.registerShape("hexagon", new HexagonRenderer());
  registry.registerShape("actor", new ActorRenderer());
  return registry;
}

export { setAttrs };

/**
 * How badly two polylines get in each other's way: a crossing counts once, a
 * stretch where they run side by side closer than a lane counts twice — that
 * one reads as a single line. Touching at an end does not count: lines into
 * the same side of a block meet it, they do not cross.
 */
function conflicts(a: readonly Point[], b: readonly Point[]): number {
  const NEAR = 12;
  const MIN_RUN = 20;
  let n = 0;
  for (let i = 0; i < a.length - 1; i++) {
    const p = a[i]!;
    const q = a[i + 1]!;
    const pv = Math.abs(p.x - q.x) < 0.5;
    const ph = Math.abs(p.y - q.y) < 0.5;
    if (!pv && !ph) continue;
    for (let j = 0; j < b.length - 1; j++) {
      const r = b[j]!;
      const t = b[j + 1]!;
      const rv = Math.abs(r.x - t.x) < 0.5;
      const rh = Math.abs(r.y - t.y) < 0.5;
      if (pv && rh) n += crosses(p, q, r, t) ? 1 : 0;
      else if (ph && rv) n += crosses(r, t, p, q) ? 1 : 0;
      else if (pv && rv && Math.abs(p.x - r.x) < NEAR) n += overlap(p.y, q.y, r.y, t.y) > MIN_RUN ? 2 : 0;
      else if (ph && rh && Math.abs(p.y - r.y) < NEAR) n += overlap(p.x, q.x, r.x, t.x) > MIN_RUN ? 2 : 0;
    }
  }
  return n;
}

/** Does vertical p–q cross horizontal r–t strictly inside both? */
function crosses(p: Point, q: Point, r: Point, t: Point): boolean {
  const x = p.x;
  const y = r.y;
  const e = 1;
  return (
    x > Math.min(r.x, t.x) + e && x < Math.max(r.x, t.x) - e &&
    y > Math.min(p.y, q.y) + e && y < Math.max(p.y, q.y) - e
  );
}

function overlap(a1: number, a2: number, b1: number, b2: number): number {
  return Math.min(Math.max(a1, a2), Math.max(b1, b2)) - Math.max(Math.min(a1, a2), Math.min(b1, b2));
}

function pushTaken(taken: Map<string, number[]>, key: string, value: number): void {
  const list = taken.get(key);
  if (list) list.push(value);
  else taken.set(key, [value]);
}

/** Only the lanes a route between these two boxes could meet: the search never looks further. */
function lanesNear(lanes: readonly RouteZone[], a: Rect, b: Rect): RouteZone[] {
  const margin = 260;
  const x0 = Math.min(a.x, b.x) - margin;
  const y0 = Math.min(a.y, b.y) - margin;
  const x1 = Math.max(a.x + a.width, b.x + b.width) + margin;
  const y1 = Math.max(a.y + a.height, b.y + b.height) + margin;
  return lanes.filter((z) =>
    z.rect.x <= x1 && z.rect.x + z.rect.width >= x0 && z.rect.y <= y1 && z.rect.y + z.rect.height >= y0);
}

/** What the routing debug picture shows about the selected line. */
interface DebugEdge {
  fromRect: Rect;
  toRect: Rect;
  fromSide: Side;
  toSide: Side;
  fromSlide: Slide | undefined;
  toSlide: Slide | undefined;
  grid: { xs: readonly number[]; ys: readonly number[] } | null;
}
