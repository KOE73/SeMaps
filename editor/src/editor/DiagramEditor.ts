import type { RoutingMode } from "../model/style-types.js";
import { DiagramCanvas, type Selection } from "../canvas/DiagramCanvas.js";
import type { StrokeScaling } from "../canvas/Viewport.js";
import { layoutFreeKey } from "../util/keys.js";
import { placeEntities } from "./placeEntity.js";
import {
  CenterPortAssigner,
  DiscretePortAssigner,
  UniformPortAssigner,
} from "../canvas/ports/assigners.js";
import { DiagramDocument } from "../model/document.js";
import { StyleLibrary } from "../model/StyleLibrary.js";
import { builtinStyleSheet } from "../model/style-defaults.js";
import type { WireStyleSheet } from "../model/style-types.js";
import { parseDocument, serializeDocument } from "../model/wire.js";
import type { ModelIssue, WireDocument } from "../model/wire-types.js";
import type { DiagramElement } from "../model/types.js";
import { snap } from "../geometry/rect.js";
import { History } from "./History.js";
import { Inspector, type InspectorHost } from "./Inspector.js";
import { EdgesPanel } from "./EdgesPanel.js";
import { StyleEditor } from "./StyleEditor.js";
import { StyleList, type StylePanelHost } from "./StyleList.js";
import { BasePanel } from "./BasePanel.js";
import { FiltersPanel } from "./FiltersPanel.js";
import {
  drawioFileName,
  exportDrawio,
  HttpProjectStore,
  HostModelStore,
  type DirtySummary,
  type ModelEvent,
  HttpStyleStore,
  download,
  readJsonFile,
  HttpWorkspaceStore,
  type ModelStore,
  type NewProject,
  type NewView,
  type ProjectEntry,
  type StyleStore,
  type ViewEntry,
  type WorkspaceIndex,
  type WorkspaceStore,
} from "./io/index.js";
import { el } from "../util/dom.js";
import { Emitter } from "../util/emitter.js";
import { DocEditorDialog, type DocTargetKind } from "./doc/DocEditorDialog.js";
import { CodeViewerDialog } from "./code/CodeViewerDialog.js";

export type { ProjectEntry, ViewEntry, WorkspaceIndex };

export interface DiagramEditorOptions {
  /** Where the list of projects and views comes from, and where new ones go. */
  workspace?: WorkspaceStore;
  store?: ModelStore;
  styleStore?: StyleStore;
  /**
   * Where the workspace files live, relative to the page.
   *
   * The bundle is served from `app/` while the models sit one level up, so
   * anything the canvas fetches for itself — `templates.json`, `content/` —
   * needs the same base the stores were given. Defaulting to "./" would make
   * the canvas ask `app/templates.json`, which is a 404 that shows up as
   * "templates silently do nothing".
   */
  modelsBase?: string;
}

import { DIAGRAM_CONFIG } from "../constants/diagram-constants.js";

/** How long a text field must be quiet before its edits become a history step. */
const FIELD_EDIT_QUIET_MS = DIAGRAM_CONFIG.interaction.fieldEditQuietMs;

const INSPECTOR_WIDTH_KEY = "semaps:inspector-width";
const MIN_INSPECTOR_WIDTH = 320;
const STYLE_LIST_WIDTH_KEY = "semaps:style-list-width";
const MIN_STYLE_LIST_WIDTH = 180;
const MIN_STYLE_EDITOR_WIDTH = 220;

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

type Slot =
  | "canvas" | "catalog" | "custom-catalog" | "custom-catalog-section"
  | "inspector-badge" | "inspector-body" | "edges-body" | "filters-body" | "title" | "zoom"
  | "json-modal" | "json-text" | "file-input" | "drop-hint" | "sidebar"
  | "styles-body" | "style-list" | "style-editor" | "style-pane-resizer"
  | "inspector" | "inspector-resizer"
  | "tab-base" | "base-search" | "base-body" | "base-list";

type Tab = "properties" | "edges" | "filters" | "styles" | "base";

/**
 * One history entry.
 *
 * Styles travel with the document because they are edited from the same panel
 * and belong to the same "what I just did". Snapshotting only the document —
 * which is what this used to do — meant Ctrl+Z after recolouring a style undid
 * whatever came *before* it and threw the style edit away without a trace.
 */
interface Snapshot {
  doc: WireDocument;
  styles: WireStyleSheet;
}

/**
 * The application: a canvas plus everything around it.
 *
 * Owns the things a reusable canvas must not know about — where models come
 * from, where they are saved, what undo means, what the toolbar looks like.
 * All of it talks to the canvas through its public API and its events.
 */
import type { DiagramEditorFacade } from "../workbench/commands/types.js";


export type EdgeSide = "left" | "right" | "top" | "bottom";
export type AlignMode = "left" | "right" | "top" | "bottom" | "width" | "height";

export class DiagramEditor implements InspectorHost, StylePanelHost, DiagramEditorFacade {
  readonly canvas: DiagramCanvas;

  private readonly root: HTMLElement;
  private readonly slots = new Map<Slot, HTMLElement>();
  readonly history = new History();
  readonly docEditor: DocEditorDialog;
  readonly codeViewer: CodeViewerDialog;
  private readonly inspector: Inspector;
  private readonly edgesPanel: EdgesPanel;
  private readonly filtersPanel: FiltersPanel;
  private readonly styleList: StyleList;
  private readonly styleEditor: StyleEditor;
  private readonly basePanel: BasePanel;
  private readonly store: ModelStore;
  private readonly styleStore: StyleStore;

  private readonly workspaceStore: WorkspaceStore;
  workspace: WorkspaceIndex = { projects: [] };
  currentView: ViewEntry | null = null;
  get currentViewId(): string | null { return this.currentView?.id ?? null; }
  readonly workspaceEvents = new Emitter<{ change: null }>();
  private dirty = false;
  private readonly modelDirty = new Map<string, DirtySummary>();
  private metadataProject: string | null = null;
  private gestureActive = false;
  private deferredEvent: ModelEvent | null = null;
  /**
   * Tracked apart from `dirty` because the two have different destinations and
   * different failure modes: a model may be bound to no file at all while the
   * styles edited through it are perfectly saveable.
   */
  private stylesDirty = false;
  /** Pending coalesced canvas redraw after a burst of style edits. */
  private redrawHandle: number | null = null;
  private styleLibrary: StyleLibrary;

  /** Banner over the canvas reporting what is wrong with the open model. */
  private banner: HTMLElement | null = null;

  /** State before the current burst of typing, held until the burst settles. */
  private fieldEditSnapshot: string | null = null;
  private fieldEditTimer: number | null = null;

  constructor(root: HTMLElement, options: DiagramEditorOptions = {}) {
    this.root = root;
    this.workspaceStore = options.workspace ?? new HttpWorkspaceStore("./");
    this.store = options.store ?? new HttpProjectStore("./");
    this.styleStore = options.styleStore ?? new HttpStyleStore("./");

    for (const node of root.querySelectorAll<HTMLElement>("[data-slot]")) {
      this.slots.set(node.dataset.slot as Slot, node);
    }

    // The built-in sheet stands in until the real one arrives: the canvas is
    // constructed synchronously and must never exist without a library, and a
    // library that fails to load is not a reason to show a grey diagram.
    this.styleLibrary = StyleLibrary.parse(builtinStyleSheet());

    this.canvas = new DiagramCanvas(this.slot("canvas"), {
      styles: this.styleLibrary,
      modelsBase: options.modelsBase,
    });
    this.docEditor = new DocEditorDialog(this);
    this.codeViewer = new CodeViewerDialog();
    this.inspector = new Inspector(
      this.slot("inspector-badge"),
      this.slot("inspector-body"),
      this,
    );
    this.edgesPanel = new EdgesPanel(this.slot("edges-body"), this);
    this.filtersPanel = new FiltersPanel(this.slot("filters-body"), this);
    this.styleList = new StyleList(this.slot("style-list"), this);
    this.styleEditor = new StyleEditor(this.slot("style-editor"), this);
    this.basePanel = new BasePanel(
      this.slot("base-search") as HTMLInputElement,
      this.slot("base-list"),
      this
    );

    this.bindCanvas();
    this.bindActions();
    this.bindKeyboard();
    this.bindFiles();
    this.bindInspectorResize();
    this.bindStyleListResize();
    this.initTheme();
    this.initDensity();
    this.initPorts();
    this.initStrokeScaling();
    this.initLang();
    this.setTab("properties");

    void this.start();
  }

  openTab(tab: Tab): void {
    this.setTab(tab);
  }

  openDocEditor(targetId?: string | null, kind?: DocTargetKind): void {
    const id = targetId || this.canvas.selected?.id;
    if (!id) {
      this.notify("Выберите элемент или связь для редактирования документации");
      return;
    }
    this.docEditor.open(id, kind);
  }

  openCodeViewer(codeRef?: string | null, label?: string): void {
    if (!codeRef) {
      const selected = this.canvas.selectedElement();
      if (selected && typeof selected.metadata?.codeRef === "string") {
        codeRef = selected.metadata.codeRef;
        label = label || selected.label;
      }
    }
    if (!codeRef) {
      this.notify("У выбранного элемента не указана ссылка на исходный код (codeRef)");
      return;
    }
    void this.codeViewer.open(codeRef, label);
  }

  /**
   * Load the style library, then the first model.
   *
   * Ordered, not parallel: `parseDocument` migrates a zone's inline colours
   * into the library as it parses, so a model opened against the placeholder
   * library would mint its imported styles into a library about to be thrown
   * away — and the zones would come back wearing ids that no longer exist.
   */
  private async start(): Promise<void> {
    try {
      this.setStyleLibrary(StyleLibrary.parse(await this.styleStore.load()));
    } catch (err) {
      this.notify(
        `Библиотека стилей не загружена: ${(err as Error).message}\n` +
          "Используются встроенные стили. Сохранение стилей перезапишет файл на сервере.",
      );
    }
    this.styleList.render();

    await this.reloadWorkspace();
    const hashView = location.hash.match(/^#(v_[^?]+)/)?.[1];
    const first = this.workspace.projects.flatMap((p) => p.views).find((v) => v.id === hashView && !v.error)
      ?? this.workspace.projects.flatMap((p) => p.views).find((v) => !v.error);
    if (first !== undefined) await this.loadView(first);
    const highlight = new URLSearchParams(location.hash.split("?")[1] ?? "").get("highlight")?.split(",").filter(Boolean);
    if (highlight?.length) { this.canvas.select(highlight[0]!); for(const id of highlight.slice(1)) this.canvas.toggleSelected(id); }
  }

  // ------------------------------------------------------------- workspace

  async reloadWorkspace(): Promise<void> {
    try {
      this.workspace = await this.workspaceStore.load();
    } catch (err) {
      this.workspace = { projects: [] };
      this.notify(`Список проектов не получен от сервера: ${(err as Error).message}`);
    }
    this.workspaceEvents.emit("change", null);
  }

  /** The view's name in the data language, else in any language, else its id. */
  viewName(view: ViewEntry): string {
    return view.names[this.dataLang] ?? Object.values(view.names)[0] ?? view.id;
  }

  projectOf(view: ViewEntry): ProjectEntry | undefined {
    return this.workspace.projects.find((p) => p.views.some((v) => v.file === view.file));
  }

  dirtyForProject(project: string): DirtySummary | undefined { return this.modelDirty.get(project); }

  /** Open a view, asking first if the current one has unsaved changes. */
  openView(view: ViewEntry, pos?: { clientX: number; clientY: number }): void {
    if (this.currentView?.file === view.file) return;
    if (!(this.store instanceof HostModelStore) && this.hasUnsavedChanges) {
      const at = pos ?? { clientX: window.innerWidth / 2, clientY: window.innerHeight / 2 };
      this.confirmDiscardOrSave(at, () => this.loadView(view));
    } else {
      void this.loadView(view);
    }
  }

  async createProject(project: NewProject): Promise<void> {
    if (this.workspace.projects.some((p) => p.id === project.id)) {
      throw new Error(`Проект «${project.id}» уже есть`);
    }
    await this.workspaceStore.createProject(project);
    await this.reloadWorkspace();
  }

  /**
   * Refuse to touch a project whose open view has unsaved edits: saving it
   * afterwards would write to the old path and bring back the old names.
   */
  private guardOpen(projectId: string): boolean {
    const open = this.currentView ? this.projectOf(this.currentView)?.id === projectId : false;
    if (open && (this.dirty || !!this.modelDirty.get(projectId)?.registry.length || !!Object.keys(this.modelDirty.get(projectId)?.views ?? {}).length)) {
      throw new Error("В открытой схеме этого проекта есть несохранённые изменения. Сохраните их (Ctrl+S) и повторите.");
    }
    return open;
  }

  /** Reopen the current view from where it lives now, so its bundle has the new ids and names. */
  private async reopen(file: string): Promise<void> {
    const view = this.workspace.projects.flatMap((p) => p.views).find((v) => v.file === file);
    if (view) await this.loadView(view);
  }

  async updateProject(oldId: string, project: NewProject): Promise<void> {
    if (project.id !== oldId && this.workspace.projects.some((p) => p.id === project.id)) {
      throw new Error(`Проект «${project.id}» уже есть`);
    }
    const open = project.id !== oldId ? this.guardOpen(oldId) : this.currentView ? this.projectOf(this.currentView)?.id === oldId : false;
    await this.workspaceStore.updateProject(oldId, project);
    if (this.store instanceof HostModelStore) {
      this.metadataProject = project.id;
      this.modelDirty.set(project.id, await this.store.dirty(project.id));
      this.markDirty();
    }
    await this.reloadWorkspace();
    if (open && this.currentView) {
      await this.reopen(this.currentView.file.replace(`projects/${oldId}/`, `projects/${project.id}/`));
    }
  }

  async updateView(oldId: string, view: NewView): Promise<void> {
    const project = this.workspace.projects.find((p) => p.id === view.project);
    if (!project) throw new Error(`Проекта «${view.project}» нет`);
    if (view.id !== oldId && project.views.some((v) => v.id === view.id)) {
      throw new Error(`Вид «${view.id}» в проекте «${view.project}» уже есть`);
    }
    const open = view.id !== oldId ? this.guardOpen(project.id) : this.currentView ? this.projectOf(this.currentView)?.id === project.id : false;
    const wasCurrent = this.currentView?.file === project.views.find((v) => v.id === oldId)?.file;
    const file = await this.workspaceStore.updateView(oldId, view, project.languages);
    await this.reloadWorkspace();
    if (wasCurrent) await this.reopen(file);
    else if (open && this.currentView) await this.reopen(this.currentView.file);
  }

  async createView(view: NewView): Promise<void> {
    const project = this.workspace.projects.find((p) => p.id === view.project);
    if (project?.views.some((v) => v.id === view.id)) {
      throw new Error(`Вид «${view.id}» в проекте «${view.project}» уже есть`);
    }
    const file = await this.workspaceStore.createView(view);
    await this.reloadWorkspace();
    const created = this.workspace.projects.flatMap((p) => p.views).find((v) => v.file === file);
    if (created) this.openView(created);
  }

  // ---------------------------------------------------------------- wiring

  private slot(name: Slot): HTMLElement {
    let node = this.slots.get(name);
    if (node === undefined) {
      if (name === "file-input" || name === "base-search") {
        node = document.createElement("input");
      } else if (name === "json-text") {
        node = document.createElement("textarea");
      } else {
        node = document.createElement("div");
      }
      this.slots.set(name, node);
    }
    return node;
  }

  private bindCanvas(): void {
    this.canvas.events.on("select", (selection) => {
      // Moving on commits whatever was being typed, so the step belongs to the
      // element it was typed into.
      this.flushFieldEdit();
      this.inspector.render(selection);
      this.edgesPanel.render();
      this.syncToolbar(selection);
    });

    this.canvas.events.on("gesturestart", () => {
      this.flushFieldEdit();
      this.gestureActive = true;
      this.history.begin(this.snapshot());
    });

    this.canvas.events.on("gestureend", () => {
      this.gestureActive = false;
      if (this.history.end(this.snapshot())) { this.syncToolbar(this.canvas.selected); this.queueModelSync(); }
      if (this.deferredEvent) { const event=this.deferredEvent; this.deferredEvent=null; void this.receiveModelEvent(event); }
    });

    this.canvas.events.on("modelchange", () => {
      this.markDirty();
      const element = this.canvas.selectedElement();
      if (element !== null) this.inspector.updateGeometry(element);
    });

    this.canvas.events.on("collapse", () => {
      this.inspector.render(this.canvas.selected);
      this.edgesPanel.render();
    });

    // Double click on a line: a ghost comes onto the view, a shown one goes back to a ghost.
    this.canvas.events.on("edgeToggle", ({ id }) => {
      this.setEdgeShown(id, !this.isEdgeShown(id));
    });

    this.canvas.events.on("openDocEditor", (payload: { id: string; kind?: DocTargetKind }) => {
      this.openDocEditor(payload.id, payload.kind);
    });

    this.canvas.events.on("openCodeViewer", (payload: { id: string; codeRef: string; label?: string }) => {
      this.openCodeViewer(payload.codeRef, payload.label);
    });

    this.canvas.events.on("viewport", (state) => {
      this.slot("zoom").textContent = `${Math.round(state.zoom * 100)}%`;
    });
  }

  private bindActions(): void {
    this.root.addEventListener("click", (e) => {
      const target = e.target;
      if (!(target instanceof Element)) return;
      const actionNode = target.closest<HTMLElement>("[data-action]");
      if (actionNode === null || actionNode.getAttribute("disabled") !== null) return;
      const action = actionNode.dataset.action;
      if (action !== undefined) {
        if (action === "open-file") {
          const rect = actionNode.getBoundingClientRect();
          const pos = { clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
          if (this.hasUnsavedChanges) {
            this.confirmDiscardOrSave(pos, () => {
              (this.slot("file-input") as HTMLInputElement).click();
            });
          } else {
            (this.slot("file-input") as HTMLInputElement).click();
          }
          return;
        }
        void this.runAction(action);
      }
    });

    this.root.addEventListener("change", (e) => {
      const target = e.target;
      if (target instanceof HTMLInputElement) {
        const toggle = target.dataset.toggle;
        if (toggle !== undefined) this.applyToggle(toggle, target.checked);
        return;
      }
      if (target instanceof HTMLSelectElement) {
        if (target.dataset.select === "ports") {
          this.applyPortAssigner(target.value);
        } else if (target.dataset.select === "theme") {
          this.applyTheme(target.value);
        } else if (target.dataset.select === "stroke-scaling") {
          this.applyStrokeScaling(target.value);
        } else if (target.dataset.select === "data-lang") {
          this.applyDataLang(target.value);
        }
      }
    });
  }

  dataLang = "ru";

  private initLang(): void {
    const saved = localStorage.getItem("semaps.dataLang") || "ru";
    this.applyDataLang(saved);
  }

  applyDataLang(lang: string): void {
    this.dataLang = lang;
    this.canvas.dataLang = lang;
    localStorage.setItem("semaps.dataLang", lang);
    this.workspaceEvents.emit("change", null);
    const select = this.root.querySelector<HTMLSelectElement>("[data-select='data-lang']");
    if (select && select.value !== lang) {
      select.value = lang;
    }
    this.inspector.render(this.canvas.selected);
  }

  /** Per-viewer preference, like ports: the model never learns about it. */
  private initStrokeScaling(): void {
    this.applyStrokeScaling(localStorage.getItem("semaps.strokeScaling") || "zoom");
  }

  applyStrokeScaling(mode: string): void {
    const next: StrokeScaling = mode === "fixed" || mode === "soft" ? mode : "zoom";
    localStorage.setItem("semaps.strokeScaling", next);
    this.canvas.viewport.strokeScaling = next;
    const select = this.root.querySelector<HTMLSelectElement>("[data-select='stroke-scaling']");
    if (select && select.value !== next) select.value = next;
  }

  /** UI density, a per-viewer preference like the theme; styles key off data-density. */
  private initDensity(): void {
    this.applyDensity(localStorage.getItem("semaps.density") || "norm");
  }

  applyDensity(density: string): void {
    const next = density === "nano" || density === "mini" ? density : "norm";
    document.documentElement.setAttribute("data-density", next);
    localStorage.setItem("semaps.density", next);
    const select = this.root.querySelector<HTMLSelectElement>("[data-select='density']");
    if (select && select.value !== next) select.value = next;
  }

  private initTheme(): void {
    const saved = localStorage.getItem("semaps:theme") || "cream";
    this.applyTheme(saved);
  }

  applyTheme(theme: string): void {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("semaps.theme", theme);
    localStorage.setItem("semaps:theme", theme);
    const select = this.root.querySelector<HTMLSelectElement>("[data-select='theme']");
    if (select && select.value !== theme) {
      select.value = theme;
    }
  }

  private async runAction(action: string): Promise<void> {
    switch (action) {
      case "create-node": return this.createNode();
      case "create-zone": return this.createZone();
      case "delete": return this.deleteSelection();
      case "undo": return this.undo();
      case "redo": return this.redo();
      case "save": return this.save();
      case "export-drawio": return this.exportDrawio();
      case "fit": return this.canvas.fit();
      case "zoom-in": return this.canvas.zoomBy(1.2);
      case "zoom-out": return this.canvas.zoomBy(0.8);
      case "zoom-reset": return this.canvas.resetZoom();
      case "toggle-sidebar": return this.toggleSidebar();
      case "open-file": return this.slot("file-input").click();
      case "toggle-json": return this.toggleJsonModal();
      case "copy-json": return this.copyJson();
      case "apply-json": return this.applyJson();
      case "tab-properties": return this.setTab("properties");
      case "tab-edges": return this.setTab("edges");
      case "tab-filters": return this.setTab("filters");
      case "tab-styles": return this.setTab("styles");
      case "tab-base": return this.setTab("base");
      default: return;
    }
  }

  /**
   * Swap which half of the right-hand panel is showing.
   *
   * Both halves stay in the DOM and are only hidden, so switching tabs cannot
   * touch the canvas selection — the properties form is still bound to whatever
   * is selected when the user comes back to it.
   */
  private setTab(tab: Tab): void {
    this.slot("inspector-body").hidden = tab !== "properties";
    this.slot("edges-body").hidden = tab !== "edges";
    this.slot("filters-body").hidden = tab !== "filters";
    this.slot("styles-body").hidden = tab !== "styles";
    this.slot("base-body").hidden = tab !== "base";
    this.slot("inspector-badge").hidden = tab !== "properties" && tab !== "edges";

    for (const node of this.root.querySelectorAll<HTMLElement>("[data-tab]")) {
      node.classList.toggle("is-active", node.dataset.tab === tab);
    }

    if (tab === "properties") this.inspector.render(this.canvas.selected);
    else if (tab === "edges") this.edgesPanel.render();
    else if (tab === "filters") this.filtersPanel.render();
    else if (tab === "styles") this.styleList.render();
    else if (tab === "base") this.basePanel.render();
  }

  applyToggle(name: string, on: boolean): void {
    switch (name) {
      case "grid":
        this.slot("canvas").classList.toggle("with-grid", on);
        return;
      case "snap":
        this.canvas.gridStep = on ? 10 : 0;
        return;
      case "structure-edges":
        // Not a style question, which is why no style can answer it: in
        // model-core-full.json 100 of 119 edges are implements/extends, and the
        // 19 that describe runtime behaviour are invisible inside them however
        // they are coloured. Presentation state only — nothing is written.
        this.canvas.setEdgeFamilyHidden("structure", !on);
        return;
      case "shadows":
      case "ghost-edges":
        this.canvas.toggleOverviewShadows(on);
        return;
      default:
        return;
    }
  }

  toggleOverviewShadows(on?: boolean): boolean {
    return this.canvas.toggleOverviewShadows(on);
  }

  isOverviewShadowsEnabled(): boolean {
    return this.canvas.isOverviewShadowsEnabled;
  }

  /**
   * Swap how edge ends are placed along a side.
   *
   * "center" is the default and reproduces the original renderer: every end
   * sits in the middle of the facing side, so several edges between the same
   * pair overlap. The others spread them out, ordering both ends by the same
   * key so the lines stay parallel instead of crossing.
   *
   * Nothing is stored in the model — placement is a pure function of it, which
   * is why this can be switched freely and why the JSON never learns about it.
   */
  private initPorts(): void {
    const saved = localStorage.getItem("semaps.ports") || "uniform";
    this.applyPortAssigner(saved);
  }

  applyPortAssigner(id: string): void {
    localStorage.setItem("semaps.ports", id);
    const select = this.root.querySelector<HTMLSelectElement>("[data-select='ports']");
    if (select && select.value !== id) {
      select.value = id;
    }
    switch (id) {
      case "center":
        this.canvas.setPortAssigner(new CenterPortAssigner());
        return;
      case "discrete":
        this.canvas.setPortAssigner(new DiscretePortAssigner());
        return;
      case "uniform":
      default:
        this.canvas.setPortAssigner(new UniformPortAssigner());
        return;
    }
  }

  private bindKeyboard(): void {
    window.addEventListener("keydown", (e) => {
      const target = e.target;
      if (target instanceof HTMLElement) {
        const tag = target.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) return;
      }

      const mod = e.ctrlKey || e.metaKey;
      const key = layoutFreeKey(e);

      if (mod && key === "z") {
        e.preventDefault();
        if (e.shiftKey) this.redo();
        else this.undo();
      } else if (mod && key === "y") {
        e.preventDefault();
        this.redo();
      } else if (e.key === "Delete" || e.key === "Backspace") {
        if (this.canvas.selected === null) return;
        e.preventDefault();
        this.deleteSelection();
      } else if (e.key === "F2" || (mod && key === "d")) {
        if (this.canvas.selected !== null) {
          e.preventDefault();
          this.openDocEditor(this.canvas.selected.id);
        }
      }
    });
  }

  private bindFiles(): void {
    const input = this.slot("file-input") as unknown as HTMLInputElement;
    input.addEventListener("change", () => {
      const file = input.files?.[0];
      if (file !== undefined) void this.loadFile(file);
      input.value = "";
    });

    const hint = this.slot("drop-hint");
    window.addEventListener("dragover", (e) => {
      e.preventDefault();
      if (!e.dataTransfer?.types.includes("application/semaps-entity")) {
        hint.hidden = false;
      }
    });
    window.addEventListener("dragleave", (e) => {
      if (e.relatedTarget === null) hint.hidden = true;
    });
    window.addEventListener("drop", (e) => {
      e.preventDefault();
      hint.hidden = true;

      const entityData = e.dataTransfer?.getData("application/semaps-entity");
      if (entityData) {
        try {
          const payload = JSON.parse(entityData);
          if (payload && payload.id) {
            const at = this.canvas.toModel(e.clientX, e.clientY);
            const [id] = placeEntities(this, [payload.entity], at);
            if (id !== undefined) this.canvas.select(id);
            this.basePanel.render();
          }
        } catch (err) {}
        return;
      }
      const file = e.dataTransfer?.files[0];
      if (file === undefined) return;
      if (!file.name.endsWith(".json")) {
        this.notify("Перетащите файл с расширением .json");
        return;
      }
      if (this.hasUnsavedChanges) {
        this.confirmDiscardOrSave({ clientX: e.clientX, clientY: e.clientY }, () => {
          void this.loadFile(file);
        });
      } else {
        void this.loadFile(file);
      }
    });
  }

  /**
   * Drag the strip between the canvas and the right panel to resize it.
   *
   * The panel was a fixed 380px, which was fine for a short property list but
   * cramped for the style editor's gradient stops and per-end arrow controls —
   * exactly the fields a wide monitor has room for. Width lives in
   * localStorage, not the model: it is a per-viewer convenience, the same
   * category as which sidebar is collapsed, not something a saved diagram
   * should carry.
   */
  private bindInspectorResize(): void {
    this.bindColumnResize({
      handle: this.slot("inspector-resizer"),
      panel: this.slot("inspector"),
      storageKey: INSPECTOR_WIDTH_KEY,
      min: MIN_INSPECTOR_WIDTH,
      max: () => this.maxInspectorWidth(),
      // The handle sits to the panel's left, so dragging it left (negative
      // delta) must widen the panel: growth is the inverse of pointer motion.
      grow: "left",
    });
  }

  /**
   * The same drag, one level in: the style list against its own editor form,
   * inside whatever width the inspector panel currently has. Two independent
   * resizers because the outer one trades the panel against the canvas, and
   * this one trades the list against the form — different questions, so a
   * width for the panel does not answer "how much of it goes to the list".
   */
  private bindStyleListResize(): void {
    const list = this.slot("style-list");
    this.bindColumnResize({
      handle: this.slot("style-pane-resizer"),
      panel: list,
      storageKey: STYLE_LIST_WIDTH_KEY,
      min: MIN_STYLE_LIST_WIDTH,
      max: () => this.slot("styles-body").getBoundingClientRect().width - MIN_STYLE_EDITOR_WIDTH,
      // Here the handle sits to the panel's right, so dragging it right
      // (positive delta) is what widens it — motion and growth agree.
      grow: "right",
    });
  }

  /**
   * One draggable column-width divider. `grow` says which side of the handle
   * the resized panel is on, which is the one thing that differs between the
   * two current uses — everything else (clamping, persistence, the dragging
   * class) is identical and not worth writing twice.
   */
  private bindColumnResize(options: {
    handle: HTMLElement;
    panel: HTMLElement;
    storageKey: string;
    min: number;
    max: () => number;
    grow: "left" | "right";
  }): void {
    const { handle, panel, storageKey, min, max, grow } = options;
    const sign = grow === "left" ? -1 : 1;

    const stored = Number(window.localStorage.getItem(storageKey));
    if (Number.isFinite(stored) && stored > 0) {
      panel.style.width = `${clamp(stored, min, max())}px`;
    }

    handle.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      const startX = e.clientX;
      const startWidth = panel.getBoundingClientRect().width;
      handle.setPointerCapture(e.pointerId);
      handle.classList.add("is-dragging");

      const onMove = (move: PointerEvent): void => {
        const width = clamp(startWidth + sign * (move.clientX - startX), min, max());
        panel.style.width = `${width}px`;
      };
      const onUp = (): void => {
        handle.classList.remove("is-dragging");
        handle.releasePointerCapture(e.pointerId);
        handle.removeEventListener("pointermove", onMove);
        handle.removeEventListener("pointerup", onUp);
        window.localStorage.setItem(storageKey, panel.getBoundingClientRect().width.toFixed(0));
      };
      handle.addEventListener("pointermove", onMove);
      handle.addEventListener("pointerup", onUp);
    });
  }

  /** Leaves room for the sidebar and a usable sliver of canvas either way. */
  private maxInspectorWidth(): number {
    return Math.max(MIN_INSPECTOR_WIDTH, window.innerWidth - 480);
  }

  // ----------------------------------------------------------- model loading

  private async loadView(view: ViewEntry): Promise<void> {
    const title = this.viewName(view);
    try {
      const wire = await this.store.load(view.file);
      this.currentView = view;
      this.loadWire(wire, title);
      if (this.store instanceof HostModelStore) {
        const project=this.projectOf(view)?.id;
        if (project) {
          this.store.subscribe(project,(event)=>{ if(this.gestureActive){this.deferredEvent=event;return} void this.receiveModelEvent(event); });
          const dirty=await this.store.dirty(project); this.modelDirty.set(project,dirty);
          this.dirty=dirty.registry.length>0 || Object.values(dirty.views).some((refs)=>refs.length>0);
          this.syncSaveButton();
        }
      }
      this.workspaceEvents.emit("change", null);
    } catch (err) {
      // A failed fetch is surfaced rather than papered over with a stale
      // embedded copy of the model, which is what the original did (D-11).
      this.notify(`Не удалось открыть «${title}»: ${(err as Error).message}`);
    }
  }

  private async receiveModelEvent(event: ModelEvent): Promise<void> {
    const project=this.currentView && this.projectOf(this.currentView)?.id;
    if (!project) return;
    const reload = event.projectReloaded;
    this.modelDirty.set(reload?.newProject ?? project,event.dirty);
    if (reload) {
      await this.reloadWorkspace();
      const file = this.currentView?.file
        .replace(`projects/${reload.oldProject}/`, `projects/${reload.newProject}/`)
        .replace(`/views/${reload.oldView}.view.json`, `/views/${reload.newView}.view.json`);
      const view = this.workspace.projects.flatMap((p) => p.views).find((candidate) => candidate.file === file);
      if (view) await this.loadView(view);
    } else if (this.currentView) {
      if (event.changed.some((ref) => ref.kind === "project" || ref.kind === "text")) await this.reloadWorkspace();
      await this.loadView(this.currentView);
    }
    this.workspaceEvents.emit("change",null);
  }

  private queueModelSync(): void {
    if (!(this.store instanceof HostModelStore) || !this.currentView || !this.canvas.model) return;
    const store=this.store;
    void store.sync(this.currentView.file,serializeDocument(this.canvas.model)).then(async()=>{
      const project=this.projectOf(this.currentView!)?.id;
      if(project){this.modelDirty.set(project,await store.dirty(project));this.workspaceEvents.emit("change",null)}
    }).catch((err)=>this.notify(`Не удалось передать изменение хосту: ${(err as Error).message}`));
  }

  async loadFile(file: File): Promise<void> {
    try {
      const wire = await readJsonFile(file);
      this.currentView = null;
      this.workspaceEvents.emit("change", null);
      this.loadWire(wire, file.name);
      this.addCustomCatalogEntry(file.name, wire);
    } catch (err) {
      this.notify((err as Error).message);
    }
  }

  private loadWire(wire: WireDocument, title: string): void {
    // Anything half-typed belongs to the model being replaced, not the new one.
    if (this.fieldEditTimer !== null) window.clearTimeout(this.fieldEditTimer);
    this.fieldEditTimer = null;
    this.fieldEditSnapshot = null;

    // Parsed against the live library so that a model still carrying inline
    // zone colours is migrated into named styles as it loads (see wire.ts).
    const doc = parseDocument(wire, this.styleLibrary);
    this.canvas.setModel(doc);
    if(this.store instanceof HostModelStore && this.currentView){this.store.confirmLoaded(this.currentView.file,serializeDocument(doc))}
    this.history.reset(this.snapshot());
    this.dirty = false;
    this.syncSaveButton();
    this.slot("title").textContent = title;
    this.renderViews(doc);
    this.renderTags();
    this.syncToolbar(null);
    // Migration may have minted styles; the list must show them.
    this.styleList.render();
    this.basePanel.render();
    this.showModelIssues(wire.bundle?.issues ?? []);
  }

  /**
   * Show what is wrong with the model that was just opened, without standing in
   * the way of working with it.
   *
   * A modal would be the wrong shape here: these are properties of the file, not
   * of anything the user just did, and they stay true until the file is fixed.
   * The banner sits over the canvas, states the consequence, and can be
   * dismissed for the session.
   */
  private showModelIssues(issues: readonly ModelIssue[]): void {
    this.banner?.remove();
    this.banner = null;
    if (issues.length === 0) return;

    const banner = document.createElement("div");
    banner.className = "semaps-model-banner";

    const text = document.createElement("div");
    text.className = "semaps-model-banner-text";
    for (const issue of issues) {
      const line = document.createElement("p");
      line.textContent = issue.message;
      text.appendChild(line);
    }

    const close = document.createElement("button");
    close.type = "button";
    close.className = "semaps-model-banner-close";
    close.title = "Скрыть";
    close.textContent = "×";
    close.addEventListener("click", () => {
      banner.remove();
      this.banner = null;
    });

    banner.append(text, close);
    this.canvas.hostElement.appendChild(banner);
    this.banner = banner;
  }

  private snapshot(): string {
    const doc = this.canvas.model;
    if (doc === null) return "";
    const state: Snapshot = {
      doc: serializeDocument(doc),
      styles: this.styleLibrary.serialize(),
    };
    return JSON.stringify(state);
  }

  private restore(snapshot: string): void {
    if (snapshot === "") return;
    const state = JSON.parse(snapshot) as Snapshot;

    // The library is rebuilt, not mutated, so a style deleted since the
    // snapshot comes back and one added since it goes away. Everything holding
    // a library must therefore read it through `this.styles`, never cache it.
    this.styleLibrary = StyleLibrary.parse(state.styles);
    const doc = parseDocument(state.doc, this.styleLibrary);
    this.canvas.setStyles(this.styleLibrary);
    this.canvas.replaceModel(doc);
    this.renderTags();

    this.markDirty();
    this.markStylesDirty();
    this.styleList.setActive(this.styleEditor.openId);
    this.styleEditor.render();
    this.syncToolbar(this.canvas.selected);
  }

  private setStyleLibrary(library: StyleLibrary): void {
    this.styleLibrary = library;
    this.canvas.setStyles(library);
    this.renderTags();
  }

  // ---------------------------------------------------------------- catalog

  private addCustomCatalogEntry(name: string, wire: WireDocument): void {
    this.slot("custom-catalog-section").hidden = false;
    this.slot("custom-catalog").appendChild(
      el(
        "button",
        {
          class: "catalog-item",
          on: {
            click: (e: MouseEvent) => {
              if (this.hasUnsavedChanges) {
                this.confirmDiscardOrSave({ clientX: e.clientX, clientY: e.clientY }, () => {
                  this.currentView = null;
                  this.loadWire(wire, name);
                });
              } else {
                this.currentView = null;
                this.loadWire(wire, name);
              }
            },
          },
        },
        [
          el("span", { class: "catalog-icon theme-blue", text: "📄" }),
          el("span", { class: "catalog-text sidebar-label" }, [
            el("span", { class: "catalog-title", text: name }),
            el("span", { class: "catalog-subtitle", text: "Пользовательский файл" }),
          ]),
        ],
      ),
    );
  }

  private renderViews(_doc: DiagramDocument): void {
    this.filtersPanel.render();
  }

  /**
   * Re-renders the filters panel when tags in use can have changed:
   * on model load and after any style edit (a rename, a retag, a new style).
   */
  renderTags(): void {
    this.filtersPanel.render();
  }

  // ---------------------------------------------------------------- editing

  createNode(): void {
    const doc = this.canvas.model;
    if (doc === null) return;
    const at = this.canvas.viewCenter();
    const x = snap(at.x, 10);
    const y = snap(at.y, 10);

    const node: DiagramElement = {
      id: `node_${Date.now().toString(36)}`,
      kind: "node",
      type: "concept",
      label: "Новый блок",
      tags: [],
      metadata: { type: "Concept / Блок", description: "Пользовательский блок архитектуры." },
      x, y, width: 190, height: 60,
      parent: null,
      children: [],
      wireOrder: Number.POSITIVE_INFINITY,
    };

    const target = doc.containerAt({ x: x + 95, y: y + 30 });
    doc.add(node, target);
    this.commit("create-node");
    this.canvas.select(node.id);
  }

  createZone(): void {
    const doc = this.canvas.model;
    if (doc === null) return;
    const at = this.canvas.viewCenter();
    const id = `zone_${Date.now().toString(36)}`;

    const zone: DiagramElement = {
      id,
      kind: "zone",
      type: "boundary",
      label: "Новая область / Слой",
      semanticId: `zone.${id}`,
      tags: [],
      metadata: { description: "Пользовательский логический контейнер." },
      x: snap(at.x, 10), y: snap(at.y, 10), width: 420, height: 300,
      // A named style, not five inline colours. The slate theme is what the old
      // hardcoded literals here spelled out, so a new zone looks the same as
      // before while now being repaintable in one place.
      styleId: "zone.slate",
      parent: null,
      children: [],
      wireOrder: Number.POSITIVE_INFINITY,
    };

    doc.add(zone, null);
    this.commit("create-zone");
    this.canvas.select(zone.id);
  }

  /**
   * Line the selected boxes up on one edge, or give them one width or height.
   *
   * Edges go to the outermost selected edge; sizes follow the primary, the box
   * touched last. A box whose container is selected too is left alone: it
   * travels with its container, which is what moving a container does anyway.
   */
  alignSelection(mode: AlignMode): void {
    const doc = this.canvas.model;
    if (doc === null) return;
    const selected = this.canvas.selectedElements();
    const set = new Set(selected);
    const boxes = selected.filter((el) => {
      for (let p = el.parent; p !== null; p = p.parent) if (set.has(p)) return false;
      return true;
    });
    if (boxes.length < 2) return;
    const primary = this.canvas.selectedElement() ?? boxes[0]!;

    const left = Math.min(...boxes.map((b) => b.x));
    const top = Math.min(...boxes.map((b) => b.y));
    const right = Math.max(...boxes.map((b) => b.x + b.width));
    const bottom = Math.max(...boxes.map((b) => b.y + b.height));

    for (const el of boxes) {
      let dx = 0;
      let dy = 0;
      switch (mode) {
        case "left": dx = left - el.x; break;
        case "right": dx = right - (el.x + el.width); break;
        case "top": dy = top - el.y; break;
        case "bottom": dy = bottom - (el.y + el.height); break;
        case "width": el.width = primary.width; break;
        case "height": el.height = primary.height; break;
      }
      if (dx === 0 && dy === 0) continue;
      for (const moved of [el, ...doc.descendants(el)]) {
        moved.x += dx;
        moved.y += dy;
      }
    }
    this.commit("align");
  }

  /**
   * Stretch one side of every selected box to the outermost such side, leaving
   * the opposite side where it is: the boxes change size, not position.
   * Contents of a stretched container stay put.
   */
  alignEdges(side: EdgeSide): void {
    const boxes = this.canvas.selectedElements();
    if (this.canvas.model === null || boxes.length < 2) return;

    switch (side) {
      case "left": {
        const to = Math.min(...boxes.map((b) => b.x));
        for (const b of boxes) { b.width += b.x - to; b.x = to; }
        break;
      }
      case "right": {
        const to = Math.max(...boxes.map((b) => b.x + b.width));
        for (const b of boxes) b.width = to - b.x;
        break;
      }
      case "top": {
        const to = Math.min(...boxes.map((b) => b.y));
        for (const b of boxes) { b.height += b.y - to; b.y = to; }
        break;
      }
      case "bottom": {
        const to = Math.max(...boxes.map((b) => b.y + b.height));
        for (const b of boxes) b.height = to - b.y;
        break;
      }
    }
    this.commit("align-edges");
  }

  deleteSelection(): void {
    const doc = this.canvas.model;
    const selection = this.canvas.selected;
    if (doc === null || selection === null) return;

    if (selection.kind === "edge") {
      doc.removeEdge(selection.id);
    } else {
      // Everything selected goes, not just the primary.
      for (const element of this.canvas.selectedElements()) {
        doc.remove(element);
      }
    }

    this.canvas.select(null);
    this.commit("delete");
  }

  deleteEdge(edgeId: string): void {
    const doc = this.canvas.model;
    if (doc === null) return;
    doc.removeEdge(edgeId);
    // The selection may have been the edge just removed; dropping it keeps the
    // inspector from binding to something that no longer exists (D-05).
    if (this.canvas.selected?.id === edgeId) this.canvas.select(null);
    this.commit("delete-edge");
    this.inspector.render(this.canvas.selected);
  }

  /** Whether the relation `id` is drawn on this view (not only known, as a ghost). */
  isEdgeShown(id: string): boolean {
    return this.canvas.model?.edge(id) !== undefined;
  }

  /**
   * Show a registry relation on the view, or hide it back to a ghost. The one
   * way the inspector, the line's menu and a double click all go (CONTRACT.md
   * §8.5: a view shows what its edge list holds).
   */
  setEdgeShown(id: string, shown: boolean): void {
    const doc = this.canvas.model;
    if (doc === null || this.isEdgeShown(id) === shown) return;
    if (shown) {
      const rel = doc.relations.find((r) => r.id === id);
      if (rel === undefined) return;
      doc.addEdge({
        id: rel.id,
        from: rel.from,
        to: rel.to,
        type: rel.type || (rel as { relation?: string }).relation || "relates",
        label: (rel as { label?: string }).label || "",
        ...(rel.styleId === undefined ? {} : { styleId: rel.styleId }),
      });
      this.commit("show-edge");
    } else {
      doc.removeEdge(id);
      this.commit("hide-edge");
    }
    this.inspector.render(this.canvas.selected);
  }

  addEdgeFromSelection(targetId: string, type: string, label: string): void {
    const doc = this.canvas.model;
    const selection = this.canvas.selected;
    if (doc === null || selection === null || selection.kind === "edge") return;

    doc.addEdge({
      id: `edge_${Date.now().toString(36)}`,
      from: selection.id,
      to: targetId,
      label,
      type,
    });
    this.commit("add-edge");
    this.inspector.render(selection);
  }

  /**
   * Apply an inspector field edit.
   *
   * Typing produces one history step per burst, not one per keystroke: the
   * state before the first change is held, and committed once the field has
   * been quiet for a moment or something else needs the history. Before this,
   * text edits mutated the model without recording anything, so undo jumped
   * straight past them and silently discarded the typing (D-02).
   */
  editField(apply: () => void, options: { rerender?: boolean; reselect?: boolean } = {}): void {
    if (this.fieldEditSnapshot === null) {
      this.fieldEditSnapshot = this.snapshot();
    }

    apply();
    this.markDirty();
    if (options.rerender === true) this.canvas.render();
    if (options.reselect === true) this.inspector.render(this.canvas.selected);

    if (this.fieldEditTimer !== null) window.clearTimeout(this.fieldEditTimer);
    this.fieldEditTimer = window.setTimeout(() => this.flushFieldEdit(), FIELD_EDIT_QUIET_MS);
  }

  /**
   * Turn a burst of typing into one history step.
   *
   * Called whenever something else is about to touch the history, so that a
   * half-finished edit can never end up straddling another action.
   */
  private flushFieldEdit(): void {
    if (this.fieldEditTimer !== null) {
      window.clearTimeout(this.fieldEditTimer);
      this.fieldEditTimer = null;
    }
    const before = this.fieldEditSnapshot;
    this.fieldEditSnapshot = null;
    if (before === null) return;

    const now = this.snapshot();
    if (before === now) return;

    // Seed the stack with the pre-edit state when this is the first change
    // after a load, so undo has somewhere to go back to.
    this.history.begin(before);
    if (this.history.end(now)) { this.syncToolbar(this.canvas.selected); this.queueModelSync(); }
  }

  // ------------------------------------------------------------- styles API

  /** The library everything on screen resolves its look through. */
  get styles(): StyleLibrary {
    return this.styleLibrary;
  }

  /**
   * A field edit inside a style.
   *
   * Deliberately the same machinery as `editField`: a burst of typing into a
   * colour box is one history step, and a style edit interleaved with a model
   * edit lands in the right order because both flush through the same timer.
   * The canvas is redrawn whole, because a style has no single owner — every
   * element wearing it changes at once, which is the property styles exist for.
   * Whole is expensive, though, so the redraw is coalesced to one frame: a
   * dragged colour picker fires this dozens of times a second, and on
   * `model-core-full.json` that is 428 nodes rebuilt per mouse move.
   */
  editStyle(apply: () => void): void {
    if (this.fieldEditSnapshot === null) {
      this.fieldEditSnapshot = this.snapshot();
    }

    apply();
    this.markStylesDirty();
    this.scheduleRedraw();

    if (this.fieldEditTimer !== null) window.clearTimeout(this.fieldEditTimer);
    this.fieldEditTimer = window.setTimeout(() => this.flushFieldEdit(), FIELD_EDIT_QUIET_MS);
  }

  /**
   * Redraw once for however many style edits arrived before the next frame.
   *
   * Only the drawing is delayed — the library, the dirty flag and the history
   * are all updated synchronously, so nothing can observe a stale model.
   */
  private scheduleRedraw(): void {
    if (this.redrawHandle !== null) return;
    this.redrawHandle = window.requestAnimationFrame(() => {
      this.redrawHandle = null;
      this.canvas.setStyles();
      // A retag is exactly the edit this bar exists to reflect, and the bar is
      // a cheap DOM diff — not worth a second coalescing path of its own.
      this.renderTags();
    });
  }

  private cancelScheduledRedraw(): void {
    if (this.redrawHandle === null) return;
    window.cancelAnimationFrame(this.redrawHandle);
    this.redrawHandle = null;
  }

  /** Create, clone, rename or delete: one discrete step, never coalesced. */
  commitStyle(apply: () => void): void {
    this.flushFieldEdit();
    apply();
    this.markStylesDirty();
    // Immediate, and any frame still pending is dropped: a discrete action must
    // not be overtaken by a redraw queued before it happened.
    this.cancelScheduledRedraw();
    this.canvas.setStyles();
    this.renderTags();
    this.history.push(this.snapshot());
    this.syncToolbar(this.canvas.selected);
  }

  onOpenStyle?: (id: string | null) => void;

  openStyle(id: string | null): void {
    this.styleList.setActive(id);
    this.styleEditor.open(id);
    this.onOpenStyle?.(id);
  }

  /**
   * Draw these boxes with a content template (`null` — back to the style's),
   * one undo step. A box too short for its new content grows to fit: picking
   * "class" and getting members painted over the neighbours below is no use.
   * Boxes never shrink here; a height someone chose stays theirs.
   */
  applyTemplate(ids: readonly string[], template: string | null): void {
    const doc = this.canvas.model;
    if (!doc) return;
    for (const id of ids) {
      const el = doc.element(id);
      if (!el || el.kind === "zone") continue;
      if (template === null) delete el.metadata.template;
      else el.metadata.template = template;
      const need = this.canvas.contentHeight(el);
      if (need !== null && need > el.height) el.height = need;
    }
    this.commit("template");
    this.inspector.render(this.canvas.selected);
  }

  /** Give these boxes (or these edges) a named style, one undo step; `null` — back to the default. */
  applyStyle(ids: readonly string[], styleId: string | null): void {
    const doc = this.canvas.model;
    if (!doc) return;
    for (const id of ids) {
      const target = doc.element(id) ?? doc.edge(id);
      if (!target) continue;
      target.styleId = styleId ?? undefined;
      const el = doc.element(id);
      const need = el ? this.canvas.contentHeight(el) : null;
      if (el && need !== null && need > el.height) el.height = need;
    }
    this.commit("style");
    this.inspector.render(this.canvas.selected);
  }

  /** Line shape of these edges alone (`null` — back to the view's / type's), one undo step. */
  setEdgeRouting(ids: readonly string[], mode: RoutingMode | null): void {
    const doc = this.canvas.model;
    if (!doc) return;
    for (const id of ids) {
      const edge = doc.edge(id);
      if (!edge) continue;
      if (mode === null) delete edge.routing;
      else edge.routing = mode;
    }
    this.commit("edge-routing");
  }

  /** Line shape for the whole view (`null` — each relation type's own), one undo step. */
  setViewRouting(mode: RoutingMode | null): void {
    const doc = this.canvas.model;
    if (!doc) return;
    const meta = doc.metadata as { routing?: RoutingMode };
    if (mode === null) delete meta.routing;
    else meta.routing = mode;
    this.commit("view-routing");
  }

  /** From the inspector's "править стиль" button. */
  openStyleTab(styleId: string): void {
    this.setTab("styles");
    this.openStyle(styleId);
  }

  private commit(reason: string): void {
    this.flushFieldEdit();
    this.canvas.notifyModelChanged(reason);
    this.history.push(this.snapshot());
    this.markDirty();
    this.queueModelSync();
    this.syncToolbar(this.canvas.selected);
    this.basePanel.render();
    this.edgesPanel.render();
  }

  get isDirty(): boolean {
    return this.dirty;
  }

  get isStylesDirty(): boolean {
    return this.stylesDirty;
  }

  undo(): void {
    this.flushFieldEdit();
    const snapshot = this.history.undo();
    if (snapshot !== null) { this.restore(snapshot); this.queueModelSync(); }
  }

  redo(): void {
    this.flushFieldEdit();
    const snapshot = this.history.redo();
    if (snapshot !== null) { this.restore(snapshot); this.queueModelSync(); }
  }

  // ------------------------------------------------------------------- i/o

  /**
   * Save whatever is dirty — the model, the styles, or both.
   *
   * The two are reported separately on purpose. Styles are shared across every
   * model in the catalogue and are always bound to a file; a model opened by
   * hand is bound to none. Rolling both into one "не удалось сохранить" would
   * mean the user could not tell whether the style work they just did survived.
   */
  async save(): Promise<void> {
    this.flushFieldEdit();
    if (this.store instanceof HostModelStore && this.currentView) {
      try {
        this.queueModelSync();
        const project=this.projectOf(this.currentView)?.id;
        if(project){
          const summary=await this.store.dirty(project);
          this.modelDirty.set(project,summary);
          const agentRegistry=summary.registry.filter((r)=>r.author==="agent" || r.author==="sync").length;
          const agentViews=Object.values(summary.views).flat().filter((r)=>r.author==="agent" || r.author==="sync").length;
          if(agentRegistry+agentViews>0){
            const humanRegistry=summary.registry.length-agentRegistry;
            const views=Object.entries(summary.views).map(([id,refs])=>`${id}: вы ${refs.filter((r)=>r.author==="human").length}, агент ${refs.filter((r)=>r.author!=="human").length}`).join("\n");
            if(!window.confirm(`Сохранить всё в проекте ${project}?\nРеестр: вы ${humanRegistry}, агент ${agentRegistry}.\nВиды:\n${views}`)) return;
          }
        }
      }catch(err){this.notify(`Не удалось получить сводку сохранения: ${(err as Error).message}`);return}
    }
    const problems: string[] = [];
    let stylesSaved = false;

    if (this.stylesDirty) {
      try {
        await this.styleStore.save(this.styleLibrary.serialize());
        this.stylesDirty = false;
        stylesSaved = true;
      } catch (err) {
        problems.push(`Стили не сохранены: ${(err as Error).message}`);
      }
    }

    const doc = this.canvas.model;
    if (this.store instanceof HostModelStore && this.metadataProject && (!this.currentView || this.projectOf(this.currentView)?.id !== this.metadataProject)) {
      try {
        await this.store.saveProject(this.metadataProject);
        this.modelDirty.delete(this.metadataProject);
        this.metadataProject = null;
        this.dirty = false;
      } catch (err) {
        problems.push(`Проект не сохранён: ${(err as Error).message}`);
      }
    }
    if ((this.dirty || (this.store instanceof HostModelStore && this.currentView && this.modelDirty.has(this.projectOf(this.currentView)?.id ?? ""))) && doc !== null) {
      if (this.currentView === null) {
        problems.push(
          "Схема загружена вручную и не привязана к файлу на сервере — она не сохранена.",
        );
      } else {
        try {
          await this.store.save({ file: this.currentView.file }, serializeDocument(doc));
          this.dirty = false;
          const project=this.projectOf(this.currentView)?.id;
          if(project){this.modelDirty.delete(project);if(this.metadataProject===project)this.metadataProject=null;}
        } catch (err) {
          problems.push(`Схема не сохранена: ${(err as Error).message}`);
        }
      }
    }

    this.syncSaveButton();

    if (problems.length === 0) {
      this.flashSaved();
      return;
    }
    this.notify(
      [
        stylesSaved ? "Библиотека стилей сохранена." : null,
        ...problems,
        "Проверьте, запущен ли сервер.",
      ]
        .filter((line): line is string => line !== null)
        .join("\n"),
    );
  }

  async discardCurrentView(): Promise<void> {
    if(!(this.store instanceof HostModelStore)||!this.currentView)return;
    const project=this.projectOf(this.currentView)?.id;if(!project)return;
    if(!window.confirm(`Отменить несохранённые изменения вида ${this.currentView.id}?`))return;
    try{await this.store.discard(project,"view",this.currentView.id);await this.loadView(this.currentView)}
    catch(err){this.notify(`Не удалось отменить изменения вида: ${(err as Error).message}`)}
  }

  async discardRegistry(): Promise<void> {
    if(!(this.store instanceof HostModelStore)||!this.currentView)return;
    const project=this.projectOf(this.currentView)?.id;if(!project)return;
    if(!window.confirm(`Отменить все несохранённые изменения реестра проекта ${project}?`))return;
    try{await this.store.discard(project,"registry");await this.loadView(this.currentView)}
    catch(err){this.notify(`Не удалось отменить изменения реестра: ${(err as Error).message}`)}
  }

  private flashSaved(): void {
    const button = this.root.querySelector<HTMLElement>('[data-action="save"]');
    if (button === null) return;
    button.textContent = "✅ Сохранено";
    window.setTimeout(() => {
      button.textContent = "💾 Сохранить";
      this.syncSaveButton();
    }, 2000);
  }

  exportDrawio(): void {
    const doc = this.canvas.model;
    if (doc === null) return;
    download(drawioFileName(doc), exportDrawio(doc, this.styleLibrary), "application/xml");
  }

  toggleJsonModal(): void {
    let modal = this.slots.get("json-modal");
    let editor = this.slots.get("json-text") as HTMLTextAreaElement | undefined;
    if (!modal || !modal.parentElement) {
      editor = el("textarea", {
        attrs: {
          spellcheck: "false",
          style: "width: 100%; height: 360px; font-family: monospace; font-size: calc(12px * var(--ui-text)); background: var(--bg, #1a1a1a); color: var(--text, #fff); border: 1px solid var(--border, #3a3a3c); border-radius: 4px; padding: calc(8px * var(--ui-space)); resize: vertical; box-sizing: border-box;",
        },
      }) as HTMLTextAreaElement;
      this.slots.set("json-text", editor);

      const closeBtn = el("button", { class: "btn-icon", text: "✕", on: { click: () => { modal!.hidden = true; } } });
      const copyBtn = el("button", { class: "btn", text: "Копировать", on: { click: () => { void this.copyJson(); } } });
      const applyBtn = el("button", { class: "btn btn-primary", text: "Применить к холсту", on: { click: () => { this.applyJson(); } } });

      modal = el("div", { class: "modal", attrs: { hidden: "true" } }, [
        el("div", { class: "modal-card", attrs: { style: "width: 600px; max-width: 90vw;" } }, [
          el("div", { class: "modal-head", attrs: { style: "display: flex; justify-content: space-between; align-items: center; padding: calc(10px * var(--ui-space)) calc(14px * var(--ui-space)); border-bottom: 1px solid var(--border, #3a3a3c);" } }, [
            el("h3", { text: "Модель диаграммы (JSON)", attrs: { style: "margin: 0; font-size: calc(14px * var(--ui-text));" } }),
            closeBtn,
          ]),
          el("div", { class: "modal-body", attrs: { style: "padding: calc(14px * var(--ui-space));" } }, [editor]),
          el("div", { class: "modal-foot", attrs: { style: "display: flex; justify-content: flex-end; gap: calc(8px * var(--ui-space)); padding: calc(10px * var(--ui-space)) calc(14px * var(--ui-space)); border-top: 1px solid var(--border, #3a3a3c);" } }, [
            copyBtn,
            applyBtn,
          ]),
        ]),
      ]);
      this.slots.set("json-modal", modal);
      document.body.appendChild(modal);
    }

    const doc = this.canvas.model;
    if (modal.hidden) {
      if (editor) editor.value = doc === null ? "" : JSON.stringify(serializeDocument(doc), null, 2);
      modal.hidden = false;
    } else {
      modal.hidden = true;
    }
  }

  async copyJson(): Promise<void> {
    const editor = this.slot("json-text") as unknown as HTMLTextAreaElement;
    try {
      await navigator.clipboard.writeText(editor.value);
      this.notify("JSON скопирован в буфер обмена");
    } catch {
      this.notify("Не удалось получить доступ к буферу обмена");
    }
  }

  applyJson(): void {
    const editor = this.slot("json-text") as unknown as HTMLTextAreaElement;
    try {
      const wire = JSON.parse(editor.value) as WireDocument;
      const title = wire.metadata?.title ?? "Пользовательская схема";
      this.currentView = null;
      this.loadWire(wire, title);
      this.slot("json-modal").hidden = true;
    } catch (err) {
      this.notify(`Ошибка в формате JSON: ${(err as Error).message}`);
    }
  }

  // ------------------------------------------------------------------- ui

  toggleSidebar(): void {
    this.slot("sidebar").classList.toggle("is-collapsed");
  }

  private markDirty(): void {
    this.dirty = true;
    this.syncSaveButton();
  }

  private markStylesDirty(): void {
    this.stylesDirty = true;
    this.syncSaveButton();
  }

  private syncSaveButton(): void {
    const isDirty = this.dirty || this.stylesDirty;
    const saveBtn = this.root.querySelector<HTMLButtonElement>('[data-action="save"]');
    if (saveBtn !== null) {
      saveBtn.disabled = !isDirty;
      saveBtn.classList.toggle("btn-save-dirty", isDirty);
      saveBtn.title = isDirty
        ? "Сохранить изменения на сервер (Ctrl+S)"
        : "Все изменения сохранены";
    }
  }

  /**
   * Shows a custom confirmation popup positioned so that the "Отменить" button
   * is placed directly under the mouse pointer coordinates (clientX, clientY).
   */
  private confirmDiscardOrSave(
    pos: { clientX: number; clientY: number },
    onProceed: (saveFirst: boolean) => Promise<void> | void,
  ): void {
    const backdrop = el("div", { class: "confirm-popover-backdrop" });

    const saveBtn = el("button", {
      class: "btn btn-success full",
      attrs: { style: "padding: calc(8px * var(--ui-space)) calc(12px * var(--ui-space)); font-size: calc(12px * var(--ui-text)); font-weight: 600;" },
      text: "💾 Сохранить и открыть",
      on: {
        click: async (ev: MouseEvent) => {
          ev.stopPropagation();
          cleanup();
          await this.save();
          await onProceed(true);
        },
      },
    });

    const cancelBtn = el("button", {
      class: "btn full",
      attrs: { style: "padding: calc(8px * var(--ui-space)) calc(12px * var(--ui-space)); font-size: calc(12px * var(--ui-text)); font-weight: 600; background: var(--panel-alt); border: 1px solid var(--line);" },
      text: "↩ Отменить",
      on: {
        click: (ev: MouseEvent) => {
          ev.stopPropagation();
          cleanup();
        },
      },
    });

    const discardBtn = el("button", {
      class: "btn btn-danger full",
      attrs: { style: "padding: calc(7px * var(--ui-space)) calc(12px * var(--ui-space)); font-size: calc(12px * var(--ui-text)); font-weight: 500;" },
      text: "🗑 Загрузить без сохранения",
      on: {
        click: async (ev: MouseEvent) => {
          ev.stopPropagation();
          cleanup();
          await onProceed(false);
        },
      },
    });

    const card = el("div", { class: "confirm-popover-card" }, [
      el("div", { class: "confirm-popover-title" }, [
        el("span", { text: "⚠️" }),
        el("span", { text: "Несохранённые изменения" }),
      ]),
      el("p", { class: "confirm-popover-msg", text: "В текущей схеме есть несохранённые правки. Что сделать перед переключением?" }),
      el("div", { class: "confirm-popover-actions" }, [
        saveBtn,
        cancelBtn,
        discardBtn,
      ]),
    ]);

    backdrop.appendChild(card);
    document.body.appendChild(backdrop);

    // Measure and position so cancelBtn is exactly centered on (pos.clientX, pos.clientY)
    const cardRect = card.getBoundingClientRect();
    const cancelRect = cancelBtn.getBoundingClientRect();
    const cancelOffsetY = cancelRect.top - cardRect.top + cancelRect.height / 2;
    const cancelOffsetX = cancelRect.left - cardRect.left + cancelRect.width / 2;

    let left = pos.clientX - cancelOffsetX;
    let top = pos.clientY - cancelOffsetY;

    left = Math.max(10, Math.min(window.innerWidth - cardRect.width - 10, left));
    top = Math.max(10, Math.min(window.innerHeight - cardRect.height - 10, top));

    card.style.left = `${left}px`;
    card.style.top = `${top}px`;

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        cleanup();
      }
    };

    const cleanup = () => {
      window.removeEventListener("keydown", onKeyDown);
      backdrop.remove();
    };

    backdrop.addEventListener("click", (e) => {
      if (e.target === backdrop) cleanup();
    });

    window.addEventListener("keydown", onKeyDown);
  }

  private syncToolbar(selection: Selection | null): void {
    this.setDisabled("undo", !this.history.canUndo);
    this.setDisabled("redo", !this.history.canRedo);
    this.setDisabled("delete", selection === null);
  }

  /** Whether the model or the style library has unsaved changes (R-STATE-01). */
  get hasUnsavedChanges(): boolean {
    return this.dirty || this.stylesDirty;
  }

  private setDisabled(action: string, disabled: boolean): void {
    const button = this.root.querySelector<HTMLButtonElement>(`[data-action="${action}"]`);
    if (button !== null) button.disabled = disabled;
  }

  notify(message: string): void {
    window.alert(message);
  }

  /** Object references for agents: `view#id`, several joined by commas; `project/view#id` when there are several projects. */
  linkFor(ids: readonly string[]): string {
    const view = this.currentView;
    if (view === null || ids.length === 0) return "";
    const project = this.workspace.projects.length > 1 ? this.projectOf(view)?.id : undefined;
    return ids.map((id) => `${project ? project + "/" : ""}${view.id}#${id}`).join(", ");
  }

  /** Copy the link of the selection (or of `ids`) and say so with a short-lived toast. */
  copyLink(ids?: readonly string[]): void {
    const link = this.linkFor(ids ?? [...this.canvas.selectedIds]);
    if (link === "") return;
    void navigator.clipboard.writeText(link).then(() => this.toast(`Ссылка скопирована: ${link}`));
  }

  toast(message: string): void {
    const box = document.createElement("div");
    box.textContent = message;
    box.setAttribute("role", "status");
    Object.assign(box.style, { position: "fixed", left: "50%", bottom: "24px", transform: "translateX(-50%)", padding: "8px 14px", background: "#222", color: "#fff", borderRadius: "6px", font: "13px sans-serif", zIndex: "10000", maxWidth: "80vw" });
    document.body.append(box);
    setTimeout(() => box.remove(), 2200);
  }
}
