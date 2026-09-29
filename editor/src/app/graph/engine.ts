import Graph from "graphology";
import Sigma from "sigma";
import type { NodeDisplayData, EdgeDisplayData } from "sigma/types";
import { animateNodes } from "sigma/utils";
import { el } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import type { GraphEdge, GraphNode, GraphResponse } from "./types.js";
import { type ColorBy, PRESENCE_CODE_COLOR, MODEL_MARKER, baseColor, colorAttribute, setGroupColorSource, edgeRenderColor, edgeSize, legendFor, lighten, nodeColor, resetPalette } from "./colors.js";
import { createNodeBorderProgram } from "@sigma/node-border";
import type { NodeLabelDrawingFunction } from "sigma/rendering";
import {
  type LayoutKind,
  applyGroupedLayout,
  circlePackLayout,
  circularLayout,
  dirOf,
  hierarchyLayout,
  radialLayout,
  randomLayout,
  runForceLayout,
  seedCircle,
} from "./layouts.js";
import { filterStore, nodeVisible } from "./filters.js";
import { OUTSIDE_ZONES, containerDepth, folderDepth, groupNodes, namespaceDepth } from "./grouping.js";
import { type FocusMode, type GroupBy, type LevelGroup, type ViewState, viewSettings } from "./viewSettings.js";
import type { GroupsResponse } from "./types.js";
import { type GraphPanel, statsLine } from "./panel.js";
import type { NodesView } from "./nodesView.js";
import { type Reach, nodeSelection, reachFrom, workingSet } from "./workset.js";
import { graphFilterConfig } from "./filterConfig.js";
import { GRAPH_TUNING } from "./tuning.js";
import { type MenuItem, openContextMenu } from "../../workbench/menus/ContextMenu.js";
import { icons } from "../../ui/icons.js";
import { GRAPH_CLIPBOARD_FORMAT, GRAPH_CLIPBOARD_VERSION, type GraphClipboard, type GraphClipboardNode } from "../../model/graphClipboard.js";
import { kindIcon, kindIconEl } from "../../ui/kindIcons.js";

/** The dock's panels the engine fills: it never owns them, so they outlive it. */
export interface GraphUi {
  readonly legend: HTMLElement;
  readonly panel: GraphPanel;
  readonly nodes: NodesView;
  /** Opens (or focuses) the «Экстракторы» panel. */
  openExtractors(): void;
  /** Copy the highlighted nodes, or the chosen ones, to the clipboard. */
  copy(which: "selection" | "chosen"): void;
}

const LEGEND_GROUP_CAP = GRAPH_TUNING.legendGroupCap;

const nodeSizeOf = (degree: number): number => {
  const s = GRAPH_TUNING.nodeSize;
  return s.base + Math.min(s.max, Math.sqrt(degree) * s.perSqrtDegree);
};

const edgeKey = (from: string, to: string, kind: string): string => `${from}\u0000${to}\u0000${kind}`;

/** Sigma's own label drawing, plus italics for an abstract type's name. */
const drawNodeLabel: NodeLabelDrawingFunction = (context, data, settings) => {
  if (!data.label) return;
  const italic = (data as { italic?: boolean }).italic ? "italic " : "";
  context.fillStyle = settings.labelColor.attribute ? ((data as Record<string, unknown>)[settings.labelColor.attribute] as string) || settings.labelColor.color || "#000" : settings.labelColor.color || "#000";
  context.font = `${italic}${settings.labelWeight} ${settings.labelSize}px ${settings.labelFont}`;
  context.fillText(data.label, data.x + data.size + 3, data.y + settings.labelSize / 3);
};

/** A highlighted or hovered node gets a ring round its circle; its label stays a
 * plain label (sigma's own hover drawing boxes the label in white). */
const drawNodeHover: NodeLabelDrawingFunction = (context, data, settings) => {
  context.beginPath();
  context.arc(data.x, data.y, data.size + 2, 0, Math.PI * 2);
  context.lineWidth = 2;
  context.strokeStyle = "#ffffff";
  context.stroke();
  drawNodeLabel(context, data, settings);
};

/** Abstract: a ring round a pale fill. Static: a double ring. (`ringColor` is set by the node reducer.) */
const nodeProgramClasses = {
  abstract: createNodeBorderProgram({
    borders: [
      { color: { attribute: "ringColor" }, size: { value: 0.22 } },
      { color: { attribute: "color" }, size: { fill: true } },
    ],
  }),
  static: createNodeBorderProgram({
    borders: [
      { color: { attribute: "ringColor" }, size: { value: 0.13 } },
      { color: { transparent: true }, size: { value: 0.1 } },
      { color: { attribute: "ringColor" }, size: { value: 0.13 } },
      { color: { attribute: "color" }, size: { fill: true } },
    ],
  }),
};

/**
 * Everything that touches sigma/graphology: built once per entry into the
 * graph mode, from `GraphResponse`. Loaded through a dynamic `import()` so
 * these dependencies never reach the library bundle (ADR_20260928-2).
 */
export class GraphEngine {
  private readonly graph: Graph;
  private readonly nodeById = new Map<string, GraphNode>();
  private edges: GraphEdge[];
  private renderer!: Sigma;
  private canvas!: HTMLElement;
  private resizeObserver: ResizeObserver | undefined;
  private colorBy: ColorBy;
  private layout: LayoutKind;
  private booted = false;
  private unsubscribeFilters: (() => void) | undefined;
  private hovered: string | undefined;
  private selected: string | undefined;
  private forceHandle: { stop: () => void } | undefined;
  private dragged: string | undefined;
  private readonly panel: GraphPanel;
  private readonly ui: GraphUi;
  private focusMode: FocusMode = "selection";
  private groupBy: GroupBy = "namespace";
  private groupDepths: Record<LevelGroup, number> = { namespace: 1, folder: 1, containers: 1 };
  private groupAxis = "";
  private groupData: GroupsResponse | undefined;
  private adjacency: Map<string, Set<string>> | undefined;
  private focusIds = new Set<string>();
  private focusNear = new Set<string>();
  private unsubscribeSet: (() => void) | undefined;
  private unsubscribeSelection: (() => void) | undefined;
  private readonly legendEl: HTMLElement;
  /** Edges by from/to/kind: what a redrawn edge's size and tooltip read (count, methods). */
  private readonly edgeInfo = new Map<string, GraphEdge>();
  private readonly tipEl = el("div", { class: "graph-edge-tip", hidden: true });
  private readonly statsEl = el("span", {});
  private readonly noticeEl = el("span", { class: "graph-live-notice" });
  private readonly noticeHost = el("div", { class: "graph-notices" });
  private noticeTimer: ReturnType<typeof setTimeout> | undefined;
  private data: GraphResponse;

  constructor(data: GraphResponse, initialLayout: LayoutKind, ui: GraphUi, initialColorBy: ColorBy = "kind") {
    this.data = data;
    this.layout = initialLayout;
    this.colorBy = initialColorBy;
    const view = viewSettings.state;
    this.focusMode = view.focus;
    this.groupBy = view.groupBy;
    this.groupDepths = { ...view.groupDepths };
    this.groupAxis = view.groupAxis;
    this.ui = ui;
    this.panel = ui.panel;
    this.legendEl = ui.legend;
    for (const n of data.nodes) this.nodeById.set(n.id, n);
    this.edges = data.edges;
    this.rebuildEdgeInfo();
    // Earlier choices for the same project stay; a value not offered before starts included.
    filterStore.sync(data.nodes, data.edges);

    this.graph = new Graph({ type: "directed", multi: true });
    for (const n of data.nodes) {
      this.graph.addNode(n.id, { label: n.name ?? n.id, x: 0, y: 0, size: 3 });
    }
    for (const e of data.edges) {
      if (!this.graph.hasNode(e.from) || !this.graph.hasNode(e.to)) continue;
      this.graph.addEdge(e.from, e.to, { kind: e.kind, via: e.via, presence: e.presence });
    }
    // Node size by degree, computed once the edges are in.
    this.graph.forEachNode((node) => {
      const d = this.graph.degree(node);
      this.graph.setNodeAttribute(node, "size", nodeSizeOf(d));
    });

    seedCircle(this.graph);
  }

  /** Builds the toolbar + canvas + panel DOM. Sigma refuses a container
   * without a width, so it is started by `start()`, once the caller has put
   * the returned element into the page. */
  mount(): HTMLElement {
    // The controls (colour, layout, search, filters) live in the ribbon and
    // the dock's panels; the canvas only carries its notices, stats and the
    // live-diff notice in a small overlay.
    const canvas = el("div", { class: "graph-canvas" });
    const overlay = el("div", { class: "graph-overlay" }, [
      this.noticeHost,
      el("div", { class: "graph-overlay-line" }, [this.statsEl, this.noticeEl]),
    ]);
    const root = el("div", { class: "graph-mode" }, [canvas, overlay, this.tipEl]);

    this.canvas = canvas;
    this.refreshNotices();
    return root;
  }

  /** Mounts sigma and starts the layout — at once when the canvas already
   * has a size, else when the dock first gives it one. */
  start(): void {
    if (this.canvas.clientWidth > 0 && this.canvas.clientHeight > 0) {
      this.boot();
      return;
    }
    const wait = new ResizeObserver(() => {
      if (this.canvas.clientWidth === 0 || this.canvas.clientHeight === 0) return;
      wait.disconnect();
      if (!this.booted && !this.destroyed) this.boot();
    });
    wait.observe(this.canvas);
    this.resizeObserver = wait;
  }

  private destroyed = false;

  private boot(): void {
    this.booted = true;
    this.renderer = new Sigma(this.graph, this.canvas, {
      renderLabels: true,
      // Hovering a calls edge shows who calls what (a tooltip).
      enableEdgeEvents: true,
      nodeProgramClasses,
      defaultDrawNodeLabel: drawNodeLabel,
      // Focus nodes and edges are raised above the rest (zIndex in the reducers).
      zIndex: true,
      defaultDrawNodeHover: drawNodeHover,
      // Looked at the ~900-node demo graph: the default threshold (6) still
      // let mid-size nodes force their way into the label grid once focused;
      // 8 keeps the overview to the biggest nodes while the focus branch
      // below still forces labels on the selection regardless of size.
      labelRenderedSizeThreshold: 8,
      // Fewer, bigger grid cells than the default (100) so the overview
      // shows a few dozen labels at most on the ~900-node demo graph, not a
      // label per node; smaller cells (effectively more labels) apply
      // automatically as the camera zooms in, since the grid is in screen
      // pixels (looked at with the demo graph until the overview stopped
      // being a label cloud).
      labelGridCellSize: 150,
      labelDensity: 1,
      // The canvas is dark (graph.css): sigma's default label is black.
      labelColor: { color: "#e6e8ec" },
      nodeReducer: (node, attrs) => this.nodeReducer(node, attrs),
      edgeReducer: (edge, attrs) => this.edgeReducer(edge, attrs),
    });
    this.wireEvents();
    this.wireBoxSelect();
    this.watchResize();
    // The one place a filter change reaches the picture: sigma, legend, stats.
    this.unsubscribeFilters = filterStore.onChange(() => this.onFiltersChanged());
    // A change of the working set re-lays the visible nodes out as well.
    this.unsubscribeSet = workingSet.onChange(() => {
      this.onFiltersChanged();
      this.applyLayout();
    });
    this.ui.nodes.setSource({
      nodes: () => this.data.nodes,
      colorOf: (n) => nodeColor(n, this.colorBy),
      focus: (id) => this.focusNode(id),
      openMenu: (id, x, y, scope) => this.openNodeMenu(id, x, y, scope),
    });
    // The selection drives Properties and the highlight, wherever it changed.
    this.unsubscribeSelection = nodeSelection.onChange(() => this.onSelectionChanged());
    this.onSelectionChanged();
    this.recolorGroups();
    this.onFiltersChanged();
    this.publishViewInfo();

    this.applyLayout();
  }

  private onSelectionChanged(): void {
    const primary = nodeSelection.primary;
    this.selected = primary && this.nodeById.has(primary) ? primary : undefined;
    if (this.selected) this.showPanel(this.selected);
    else this.panel.clear();
    this.recomputeFocus();
    this.renderer.refresh();
  }

  /** What the picture emphasises: the selection (all of it, when there are several),
   * else the hovered node, and their neighbours. Emphasis only — nothing is hidden
   * by it: every edge the filters allow is drawn. */
  private recomputeFocus(): void {
    this.focusIds = new Set();
    this.focusNear = new Set();
    if (this.focusMode === "off") return;
    const ids: Iterable<string> = nodeSelection.size > 0 ? nodeSelection.members : this.hovered ? [this.hovered] : [];
    for (const id of ids) {
      if (!this.graph.hasNode(id)) continue;
      this.focusIds.add(id);
      for (const n of this.drawnAdjacency().get(id) ?? []) this.focusNear.add(n);
    }
  }

  /** Neighbours over the edges drawn now only (kind filter on, both ends visible): a hidden edge lights nothing up. */
  private drawnAdjacency(): Map<string, Set<string>> {
    if (this.adjacency) return this.adjacency;
    const kinds = filterStore.state.edgeKinds;
    const map = new Map<string, Set<string>>();
    const link = (a: string, b: string) => (map.get(a) ?? map.set(a, new Set()).get(a)!).add(b);
    for (const e of this.edges) {
      if (!kinds.has(e.kind)) continue;
      const from = this.nodeById.get(e.from);
      const to = this.nodeById.get(e.to);
      if (!from || !to || !this.isShown(from) || !this.isShown(to)) continue;
      link(e.from, e.to);
      link(e.to, e.from);
    }
    return (this.adjacency = map);
  }

  /** Whether everything outside the focus is dimmed: not at all («без подсветки»), only with a
   * selection («только выделение», the default: hover just emphasises), or always with a focus. */
  private dimming(): boolean {
    if (this.focusMode === "off") return false;
    if (this.focusMode === "selection") return nodeSelection.size > 0 && this.focusIds.size > 0;
    return this.focusIds.size > 0;
  }

  /** How much of a colour stays when dimmed: «мягко» keeps over half of it. */
  private dimKeep(): number {
    return this.focusMode === "soft" ? GRAPH_TUNING.dimKeepSoft : GRAPH_TUNING.dimKeepStrong;
  }

  getFocusMode(): FocusMode {
    return this.focusMode;
  }

  setFocusMode(mode: FocusMode): void {
    this.focusMode = mode;
    if (!this.booted) return;
    this.recomputeFocus();
    this.renderer.refresh();
  }

  /** The universe: what the pre-filter lets through. */
  private inUniverse(id: string): boolean {
    const n = this.nodeById.get(id);
    return !!n && nodeVisible(n, filterStore.state);
  }

  /** Drawn: in the universe and, when there is a working set, in it. */
  private isShown(n: GraphNode): boolean {
    return nodeVisible(n, filterStore.state) && (workingSet.size === 0 || workingSet.has(n.id));
  }

  private onFiltersChanged(): void {
    this.adjacency = undefined; // what is drawn changed: so does who is a neighbour
    if (this.colorBy === "group" && this.groupBy === "components") this.recolorGroups(); // components follow the drawn edges
    this.recomputeFocus();
    this.refreshLegend();
    this.refreshStats();
    this.renderer.refresh();
  }

  /** Counts what is shown after filtering; the hidden-missing note is the host's. */
  private refreshStats(): void {
    const f = filterStore.state;
    let nodes = 0;
    for (const n of this.data.nodes) if (this.isShown(n)) nodes++;
    let edges = 0;
    for (const e of this.edges) {
      const from = this.nodeById.get(e.from);
      const to = this.nodeById.get(e.to);
      if (from && to && f.edgeKinds.has(e.kind) && this.isShown(from) && this.isShown(to)) edges++;
    }
    this.statsEl.textContent = statsLine(nodes, edges, this.data.stats.hiddenMissing);
  }

  getColorBy(): ColorBy {
    return this.colorBy;
  }

  setColorBy(colorBy: ColorBy): void {
    this.colorBy = colorBy;
    this.recolorGroups();
    resetPalette();
    this.refreshLegend();
    this.renderer?.refresh();
    if (this.booted) this.ui.nodes.refresh();
  }

  /**
   * Follows the view settings store (ribbon selects and the «Вид» panel both
   * write it): colour and focus apply at once; a changed layout, or the grouping
   * of the `grouped` layout, lays the nodes out again.
   */
  applyView(s: Readonly<ViewState>): void {
    if (s.focus !== this.focusMode) this.setFocusMode(s.focus);
    const groupingChanged = s.groupBy !== this.groupBy || s.groupDepths[s.groupBy as LevelGroup] !== this.groupDepths[this.groupBy as LevelGroup] || s.groupAxis !== this.groupAxis;
    const relayout = s.layout !== this.layout || (s.layout === "grouped" && groupingChanged);
    this.layout = s.layout;
    this.groupBy = s.groupBy;
    this.groupDepths = { ...s.groupDepths };
    this.groupAxis = s.groupAxis;
    // Colour by group follows the grouping (and is recomputed when either changes).
    if (s.colorBy !== this.colorBy) this.setColorBy(s.colorBy);
    else if (s.colorBy === "group" && groupingChanged) this.recolor();
    if (relayout && this.booted) this.applyLayout();
  }

  /** The group of every node, by the current «Группировать по» — the keys the grouped layout uses. */
  private recolorGroups(): void {
    if (this.colorBy !== "group") return;
    const map = groupNodes(this.groupBy, this.graph, {
      nodeOf: (id) => this.nodeById.get(id),
      depth: this.groupDepths[this.groupBy as LevelGroup] ?? 1,
      axis: this.groupAxis,
      groups: this.groupData,
      drawnKinds: filterStore.state.edgeKinds,
    });
    setGroupColorSource((n) => {
      const key = map.get(n.id);
      return key === undefined || key === "" ? "—" : key === OUTSIDE_ZONES ? t.graphGroupOutside : key;
    });
    resetPalette();
  }

  private recolor(): void {
    this.recolorGroups();
    this.refreshLegend();
    this.renderer?.refresh();
    if (this.booted) this.ui.nodes.refresh();
  }

  /** The containers' nesting and the zones per axis (`/groups`), which «группировать по» needs. */
  setGroupData(groups: GroupsResponse | undefined): void {
    this.groupData = groups;
    this.publishViewInfo();
    const usesGroups = this.groupBy === "containers" || this.groupBy === "axis";
    if (this.booted && this.colorBy === "group" && usesGroups) this.recolor();
    if (this.booted && this.layout === "grouped" && usesGroups) this.applyLayout();
  }

  /** What the panel's inline controls may offer: depths the data has, the axes the project declares. */
  private publishViewInfo(): void {
    viewSettings.setInfo({
      namespaceDepth: namespaceDepth(this.data.nodes),
      folderDepth: folderDepth(this.data.nodes),
      containerDepth: containerDepth(this.groupData),
      axes: this.groupData?.axes.map((a) => a.axis) ?? [],
    });
  }

  /** Width over height of the graph canvas; 1 while it is not laid out. */
  private canvasAspect(): number {
    const d = this.renderer?.getDimensions();
    return d && d.width > 0 && d.height > 0 ? d.width / d.height : 1;
  }

  restartLayout(): void {
    if (this.booted) this.applyLayout();
  }

  /** Sigma only resizes on `window`'s own `resize` event (checked in
   * `sigma/dist/sigma.esm.js`); it has no observer of its own container. The
   * tool-page layout resizes the canvas whenever the ribbon, the side dock or
   * the window itself changes without necessarily firing a window resize
   * (e.g. first mount before web fonts/layout settle), so sigma is told
   * about it explicitly here. */
  private watchResize(): void {
    this.resizeObserver = new ResizeObserver(() => {
      this.renderer.resize();
      this.renderer.refresh();
    });
    this.resizeObserver.observe(this.canvas);
  }

  /** The currently selected layout, so a caller re-creating the engine (e.g.
   * on a filter change that refetches data) can restore it. */
  getLayout(): LayoutKind {
    return this.layout;
  }

  destroy(): void {
    this.destroyed = true;
    this.unsubscribeFilters?.();
    this.unsubscribeSet?.();
    this.unsubscribeSelection?.();
    if (this.booted) this.ui.nodes.setSource(undefined);
    this.resizeObserver?.disconnect();
    this.forceHandle?.stop();
    clearTimeout(this.noticeTimer);
    this.renderer?.kill();
  }

  // -------------------------------------------------------- live updates

  /** Applies the difference between the currently held graph and a freshly
   * fetched one, in place: gone nodes/edges are dropped, new ones are added
   * next to their already-placed neighbours (or the centre, with a small
   * random offset), changed ones get their data replaced, and everything
   * that stays KEEPS its coordinates — no layout runs on its own
   * (PLAN_20260928-2 step 6). Selection and camera are untouched, except the
   * selection is cleared when the selected node is gone. Returns the counts
   * for the toolbar's transient notice. */
  applyDiff(next: GraphResponse): { addedNodes: number; removedNodes: number; addedEdges: number; removedEdges: number } {
    const removedNodeIds = this.graph.nodes().filter((id) => !next.nodes.some((n) => n.id === id));
    for (const id of removedNodeIds) {
      if (this.graph.hasNode(id)) this.graph.dropNode(id);
      this.nodeById.delete(id);
    }

    const addedNodes = next.nodes.filter((n) => !this.graph.hasNode(n.id));
    for (const n of addedNodes) {
      const { x, y } = this.placementFor(n, next);
      this.graph.addNode(n.id, { label: n.name ?? n.id, x, y, size: 3 });
    }
    for (const n of next.nodes) this.nodeById.set(n.id, n);

    const nextEdgeKey = (e: GraphEdge) => `${e.from}\u0000${e.to}\u0000${e.kind}`;
    const oldEdgeKeys = new Set(this.edges.map(nextEdgeKey));
    const nextEdgeKeys = new Set(next.edges.map(nextEdgeKey));
    let removedEdges = 0;
    for (const e of this.edges) {
      if (nextEdgeKeys.has(nextEdgeKey(e))) continue;
      // An edge whose endpoint node was just dropped above is already gone
      // (graphology drops a node's edges with it) — graph.edges(from, to)
      // throws for a missing node, so this only looks at edges between
      // nodes that still exist.
      if (!this.graph.hasNode(e.from) || !this.graph.hasNode(e.to)) { removedEdges++; continue; }
      const key = this.graph.edges(e.from, e.to).find((eid) => this.graph.getEdgeAttribute(eid, "kind") === e.kind);
      if (key) { this.graph.dropEdge(key); removedEdges++; }
    }
    let addedEdges = 0;
    for (const e of next.edges) {
      if (oldEdgeKeys.has(nextEdgeKey(e))) continue;
      if (!this.graph.hasNode(e.from) || !this.graph.hasNode(e.to)) continue;
      this.graph.addEdge(e.from, e.to, { kind: e.kind, via: e.via, presence: e.presence });
      addedEdges++;
    }
    this.edges = next.edges.filter((e) => this.graph.hasNode(e.from) && this.graph.hasNode(e.to));
    this.rebuildEdgeInfo();
    this.adjacency = undefined;

    // Degree-based size, recomputed for everyone — cheap next to a layout.
    this.graph.forEachNode((node) => {
      const d = this.graph.degree(node);
      this.graph.setNodeAttribute(node, "size", nodeSizeOf(d));
    });

    this.data = next;
    this.publishViewInfo();
    this.recolorGroups();

    if (this.selected && removedNodeIds.includes(this.selected)) {
      this.selected = undefined;
      this.panel.clear();
    }

    resetPalette();
    this.refreshNotices();
    const existing = new Set(next.nodes.map((n) => n.id));
    workingSet.prune(existing);
    nodeSelection.prune(existing);
    this.recomputeFocus();
    // Merge filter selections through the store (a value already offered
    // keeps the user's choice, a newly offered one starts included); its
    // change event refreshes sigma, legend and stats.
    filterStore.sync(next.nodes, next.edges);
    if (!this.booted) {
      this.refreshLegend();
      this.refreshStats();
    } else {
      this.ui.nodes.refresh();
    }

    return { addedNodes: addedNodes.length, removedNodes: removedNodeIds.length, addedEdges, removedEdges };
  }

  /** Where a new node lands: the average position of its already-placed
   * neighbours in the new edge list, or the centre when it has none, with a
   * small random offset so several new nodes do not stack exactly. */
  private placementFor(n: GraphNode, next: GraphResponse): { x: number; y: number } {
    const neighbourIds = next.edges
      .filter((e) => e.from === n.id || e.to === n.id)
      .map((e) => (e.from === n.id ? e.to : e.from))
      .filter((id) => this.graph.hasNode(id));
    const jitter = () => (Math.random() - 0.5) * 8;
    if (neighbourIds.length === 0) return { x: jitter(), y: jitter() };
    let sx = 0, sy = 0;
    for (const id of neighbourIds) { sx += this.graph.getNodeAttribute(id, "x"); sy += this.graph.getNodeAttribute(id, "y"); }
    return { x: sx / neighbourIds.length + jitter(), y: sy / neighbourIds.length + jitter() };
  }

  /** The toolbar's transient "N added/removed" notice (a few seconds). */
  announceDiff(counts: { addedNodes: number; removedNodes: number; addedEdges: number; removedEdges: number }): void {
    if (counts.addedNodes === 0 && counts.removedNodes === 0 && counts.addedEdges === 0 && counts.removedEdges === 0) return;
    clearTimeout(this.noticeTimer);
    this.noticeEl.textContent = fmt(t.graphLiveDiff, {
      addedNodes: String(counts.addedNodes),
      removedNodes: String(counts.removedNodes),
      addedEdges: String(counts.addedEdges),
      removedEdges: String(counts.removedEdges),
    });
    this.noticeTimer = setTimeout(() => { this.noticeEl.textContent = ""; }, 5000);
  }

  private refreshNotices(): void {
    const bars: HTMLElement[] = [];
    // Both notices lead to the «Экстракторы» panel, where the run happens.
    const opener = () => {
      const link = el("a", { text: t.graphOpenExtractorsPanel });
      link.href = "#";
      link.addEventListener("click", (e) => {
        e.preventDefault();
        this.ui.openExtractors();
      });
      return link;
    };
    if (this.data.facts.length === 0) {
      bars.push(el("p", { class: "graph-notice" }, [t.graphNoFactsNotice + " ", opener()]));
    }
    for (const f of this.data.facts) {
      if (!f.lastRunFailed) continue;
      const link = opener();
      // With no earlier successful run there are no facts to fall back on: say so.
      const text = f.finished ? t.graphRunFailedNotice : t.graphRunFailedNoFacts;
      bars.push(el("p", { class: "graph-notice is-warn" }, [fmt(text, { extractor: f.extractor }) + " ", link]));
    }
    this.noticeHost.replaceChildren(...bars);
  }

  // ------------------------------------------------------------ rendering

  private nodeReducer(node: string, attrs: Record<string, unknown>): Partial<NodeDisplayData> {
    const n = this.nodeById.get(node);
    const res: Partial<NodeDisplayData> = { ...(attrs as unknown as NodeDisplayData) };
    if (!n || !this.isShown(n)) {
      res.hidden = true;
      return res;
    }
    res.color = nodeColor(n, this.colorBy);
    const label = (n.presence === "model" ? MODEL_MARKER : "") + (n.name ?? n.id);
    res.label = label;

    // A modifier is a look: abstract = a ring in the kind's colour round a pale
    // fill and an italic name (the UML convention the Схемы editor's interface
    // follows); static = a double ring. Kept subtle; sealed etc. look like the kind.
    const mods = n.modifiers ?? [];
    const look = mods.includes("static") ? "static" : mods.includes("abstract") ? "abstract" : undefined;
    if (look) {
      const ring = res.color as string;
      res.type = look;
      (res as Record<string, unknown>).ringColor = ring;
      res.color = lighten(baseColor(n, this.colorBy), look === "abstract" ? 0.78 : 0.6);
    }
    if (mods.includes("abstract")) (res as Record<string, unknown>).italic = true;

    const inSelection = nodeSelection.has(node);
    if (inSelection) {
      // Highlighted nodes stand out on the canvas too.
      res.highlighted = true;
      res.forceLabel = true;
      res.size = ((res.size as number | undefined) ?? 3) * 1.4;
      res.zIndex = 2;
    }
    if (this.focusIds.size > 0) {
      const inFocus = this.focusIds.has(node) || this.focusNear.has(node);
      if (inSelection) {
        // never faded
      } else if (!inFocus) {
        if (this.dimming()) {
          const keep = this.dimKeep();
          res.color = fade(res.color as string, keep);
          if (look) (res as Record<string, unknown>).ringColor = fade((res as Record<string, unknown>).ringColor as string, keep);
          res.label = null;
          res.zIndex = 0;
        }
      } else {
        // On top of the rest, and its label forced (only along edges that are drawn).
        res.forceLabel = true;
        res.zIndex = 1;
      }
    }
    // No unfocused branch forcing a label by degree: that defeated sigma's
    // own label grid (every node above the threshold showed a label at once,
    // an unreadable cloud at overview — the defect this page had). Left
    // alone, the grid (labelGridCellSize/labelDensity above) already favours
    // the biggest node per screen cell, so only the largest nodes are
    // labelled zoomed out, with more appearing as cells get smaller on
    // screen while zooming in.
    return res;
  }

  private edgeReducer(edge: string, attrs: Record<string, unknown>): Partial<EdgeDisplayData> {
    const res: Partial<EdgeDisplayData> = { ...(attrs as unknown as EdgeDisplayData) };
    const ext = this.graph.extremities(edge);
    const from = this.nodeById.get(ext[0]);
    const to = this.nodeById.get(ext[1]);
    if (!from || !to || !this.isShown(from) || !this.isShown(to)) {
      res.hidden = true;
      return res;
    }
    const kind = this.graph.getEdgeAttribute(edge, "kind") as string;
    if (!filterStore.state.edgeKinds.has(kind)) {
      res.hidden = true;
      return res;
    }
    const via = this.graph.getEdgeAttribute(edge, "via") as GraphEdge["via"];
    const info = this.edgeInfo.get(edgeKey(ext[0], ext[1], kind));
    res.color = edgeRenderColor({ kind, via } as GraphEdge);
    res.size = edgeSize({ via, count: info?.count });

    // Every edge the filters allow is drawn. Focus only emphasises: the edges of
    // the focused nodes in full colour and a little thicker, the others dimmed.
    if (this.focusIds.size > 0) {
      if (this.focusIds.has(ext[0]) || this.focusIds.has(ext[1])) {
        res.size = (res.size ?? 2) * 1.5;
        res.zIndex = 1;
      } else if (this.dimming()) {
        res.color = fade(res.color as string, this.dimKeep());
        res.zIndex = 0;
      }
    }
    return res;
  }

  private refreshLegend(): void {
    const shown = this.data.nodes.filter((n) => this.isShown(n));
    let entries = legendFor(shown, this.colorBy);
    let more = 0;
    if (this.colorBy === "group") {
      // Groups can be hundreds: the biggest ones, then «ещё N».
      const size = new Map<string, number>();
      for (const n of shown) size.set(colorAttribute(n, "group"), (size.get(colorAttribute(n, "group")) ?? 0) + 1);
      entries = entries.sort((a, b) => (size.get(b.label) ?? 0) - (size.get(a.label) ?? 0));
      more = Math.max(0, entries.length - LEGEND_GROUP_CAP);
      entries = entries.slice(0, LEGEND_GROUP_CAP);
    }
    this.legendEl.replaceChildren(
      el(
        "div",
        { class: "graph-legend-list" },
        [...entries.map((it) =>
          el("div", { class: "graph-legend-item" }, [
            el("span", { class: "graph-swatch", attrs: { style: `background:${it.swatch}` } }),
            // The colour says which group; the icon says which kind — the same icon as everywhere.
            this.colorBy === "kind" ? kindIconEl("symbol", it.label) : this.colorBy === "presence" ? kindIconEl("presence", it.label) : null,
            it.label,
          ]),
        ),
        ...(more > 0 ? [el("div", { class: "graph-legend-item tool-muted", text: fmt(t.graphLegendMore, { n: String(more) }) })] : [])],
      ),
      el("div", { class: "graph-legend-list" }, [
        el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch", attrs: { style: `background:${PRESENCE_CODE_COLOR}` } }), kindIconEl("presence", "code"), t.graphPresenceCode]),
        el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch graph-swatch-model" }), kindIconEl("presence", "model"), MODEL_MARKER + t.graphPresenceModel]),
        // Modifiers are looks, not kinds: only those actually on the canvas get a line.
        ...(shown.some((n) => n.modifiers?.includes("abstract"))
          ? [el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch graph-swatch-abstract" }), el("em", { text: t.graphLegendAbstract })])]
          : []),
        ...(shown.some((n) => n.modifiers?.includes("static"))
          ? [el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch graph-swatch-static" }), t.graphLegendStatic])]
          : []),
      ]),
    );
  }

  // -------------------------------------------------------------- events

  private wireEvents(): void {
    this.renderer.on("enterNode", ({ node }) => {
      this.hovered = node;
      this.recomputeFocus();
      this.renderer.refresh();
    });
    this.renderer.on("leaveNode", () => {
      if (this.dragged) return;
      this.hovered = undefined;
      this.recomputeFocus();
      this.renderer.refresh();
    });
    this.wireDrag();
    // A click selects (the list scrolls to it, Properties shows it); Ctrl toggles.
    // The selection is sticky: an empty spot of the canvas does not clear it.
    this.renderer.on("clickNode", ({ node, event }) => {
      const original = event.original;
      if ("ctrlKey" in original && (original.ctrlKey || original.metaKey)) nodeSelection.toggle(node);
      else nodeSelection.setOnly(node);
    });
    this.renderer.on("enterEdge", ({ edge, event }) => this.showEdgeTip(edge, event.x, event.y));
    this.renderer.on("leaveEdge", () => {
      this.tipEl.hidden = true;
    });
    // The page's own menu, not the browser's.
    this.canvas.addEventListener("contextmenu", (e) => e.preventDefault());
    this.renderer.on("rightClickNode", ({ node, event }) => {
      const o = event.original as MouseEvent;
      this.openNodeMenu(node, o.clientX, o.clientY);
    });
    // A click on an empty spot (sigma does not report a pan as a click) clears the selection.
    this.renderer.on("clickStage", () => nodeSelection.clear());
    this.renderer.on("rightClickStage", ({ event }) => {
      const o = event.original as MouseEvent;
      openContextMenu(
        [
          { label: t.graphMenuShowAll, icon: icons.eye, disabled: workingSet.size === 0, onSelect: () => workingSet.clear() },
          { kind: "separator" },
          ...this.selectionItems(),
        ],
        o.clientX,
        o.clientY,
      );
    });
  }

  /** The nodes drawn now: what Ctrl+A selects and Ctrl+I inverts within. */
  visibleNodeIds(): string[] {
    return this.data.nodes.filter((n) => this.isShown(n)).map((n) => n.id);
  }

  /** «Выделить все / Инвертировать / Снять выделение», within `scope` (the drawn nodes, or the rows listed). */
  private selectionItems(scope?: readonly string[]): MenuItem[] {
    const ids = () => scope ?? this.visibleNodeIds();
    return [
      { label: t.graphSelectAll, icon: icons.checks, note: "Ctrl+A", onSelect: () => nodeSelection.selectAll(ids()) },
      { label: t.graphInvertSelection, icon: icons.arrowsDiff, note: "Ctrl+I", onSelect: () => nodeSelection.invert(ids()) },
      { label: t.graphNodesClearSelection, icon: icons.squareOff, note: "Esc", disabled: nodeSelection.size === 0, onSelect: () => nodeSelection.clear() },
    ];
  }

  /**
   * Shift+drag on an empty spot draws a rectangle; the nodes inside become the
   * selection (Ctrl+Shift+drag adds to it). A plain drag still pans and a drag
   * that starts on a node still moves it: sigma is not told about the press only
   * when Shift is held and no node is under the pointer.
   */
  private wireBoxSelect(): void {
    const canvas = this.canvas;
    canvas.addEventListener(
      "mousedown",
      (e) => {
        if (!e.shiftKey || e.button !== 0 || this.hovered !== undefined) return;
        e.preventDefault();
        e.stopPropagation();
        const rect = canvas.getBoundingClientRect();
        const start = { x: e.clientX - rect.left, y: e.clientY - rect.top };
        const box = el("div", { class: "graph-box" });
        canvas.parentElement?.appendChild(box);
        const place = (x: number, y: number) => {
          box.style.left = `${Math.min(start.x, x)}px`;
          box.style.top = `${Math.min(start.y, y)}px`;
          box.style.width = `${Math.abs(x - start.x)}px`;
          box.style.height = `${Math.abs(y - start.y)}px`;
        };
        place(start.x, start.y);
        const move = (m: MouseEvent) => place(m.clientX - rect.left, m.clientY - rect.top);
        const up = (u: MouseEvent) => {
          window.removeEventListener("mousemove", move);
          window.removeEventListener("mouseup", up, true);
          box.remove();
          const end = { x: u.clientX - rect.left, y: u.clientY - rect.top };
          if (Math.abs(end.x - start.x) < 4 && Math.abs(end.y - start.y) < 4) return;
          const [x0, x1] = [Math.min(start.x, end.x), Math.max(start.x, end.x)];
          const [y0, y1] = [Math.min(start.y, end.y), Math.max(start.y, end.y)];
          const inside = this.visibleNodeIds().filter((id) => {
            const p = this.renderer.graphToViewport({ x: this.graph.getNodeAttribute(id, "x") as number, y: this.graph.getNodeAttribute(id, "y") as number });
            return p.x >= x0 && p.x <= x1 && p.y >= y0 && p.y <= y1;
          });
          if (u.ctrlKey || u.metaKey) nodeSelection.add(inside);
          else nodeSelection.selectAll(inside);
        };
        window.addEventListener("mousemove", move);
        window.addEventListener("mouseup", up, true);
      },
      true,
    );
  }

  private rebuildEdgeInfo(): void {
    this.edgeInfo.clear();
    for (const e of this.edges) this.edgeInfo.set(edgeKey(e.from, e.to, e.kind), e);
  }

  /** "A → B calls ×N: M1, M2 → N1" for a calls/constructs edge. */
  private showEdgeTip(edge: string, x: number, y: number): void {
    const [from, to] = this.graph.extremities(edge);
    const kind = this.graph.getEdgeAttribute(edge, "kind") as string;
    const info = this.edgeInfo.get(edgeKey(from, to, kind));
    if (!info || (!info.count && !info.fromMethods?.length && !info.toMethods?.length)) return;
    const name = (id: string) => this.nodeById.get(id)?.name ?? id;
    const methods = info?.fromMethods?.length || info?.toMethods?.length ? `: ${(info.fromMethods ?? []).join(", ")} → ${(info.toMethods ?? []).join(", ")}` : "";
    const times = info?.count && info.count > 1 ? ` ×${info.count}` : "";
    this.tipEl.textContent = `${name(from)} → ${name(to)} ${kind}${times}${methods}`;
    this.tipEl.style.left = `${x + 12}px`;
    this.tipEl.style.top = `${y + 12}px`;
    this.tipEl.hidden = false;
  }

  // ----------------------------------------------------------- node menu

  /** Edge kinds present in the data, the inheritance ones first. */
  private edgeKindsOrdered(): string[] {
    const kinds = [...new Set(this.edges.map((e) => e.kind))];
    const HIERARCHY_KINDS = graphFilterConfig().hierarchyKinds;
    const rank = (k: string) => (HIERARCHY_KINDS.includes(k) ? HIERARCHY_KINDS.indexOf(k) : HIERARCHY_KINDS.length);
    return kinds.sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
  }

  private openNodeMenu(node: string, x: number, y: number, scope?: readonly string[]): void {
    if (!this.nodeById.has(node)) return;
    // On a highlighted node the menu acts on the whole selection (even members
    // a search hides in the list); on any other it first becomes the selection.
    if (!nodeSelection.has(node)) nodeSelection.setOnly(node);
    const targets = [...nodeSelection.members];

    const found = (reach: Reach, kinds: readonly string[] | null, all: boolean): Set<string> => {
      const out = new Set<string>();
      for (const id of targets) {
        for (const f of reachFrom(id, reach, kinds, all, this.edges, (n) => this.inUniverse(n))) out.add(f);
      }
      return out;
    };
    // What a traversal finds becomes chosen and highlighted, so the next step continues from it.
    const take = (reach: Reach, kinds: readonly string[] | null, all: boolean) => () => {
      const result = found(reach, kinds, all);
      workingSet.add([...targets, ...result]);
      nodeSelection.add(result);
    };
    const step = (label: string, icon: string, reach: Reach, all: boolean): MenuItem => {
      const hierarchy = graphFilterConfig().hierarchyKinds;
      const n = found(reach, hierarchy, all).size;
      return { label, icon, note: n ? String(n) : undefined, disabled: n === 0, onSelect: take(reach, hierarchy, all) };
    };

    openContextMenu(
      [
        step(t.graphMenuDescendants1, icons.arrowDown, "descendants", false),
        step(t.graphMenuDescendantsAll, icons.arrowDown, "descendants", true),
        step(t.graphMenuAncestors1, icons.arrowUp, "ancestors", false),
        step(t.graphMenuAncestorsAll, icons.arrowUp, "ancestors", true),
        { kind: "separator" },
        { label: t.graphMenuChoose, icon: icons.playlistAdd, onSelect: () => workingSet.add(targets) },
        { label: t.graphMenuKeepSelected, icon: icons.focus2, onSelect: () => workingSet.setTo(nodeSelection.members) },
        { label: t.graphMenuUnchoose, icon: icons.playlistX, onSelect: () => workingSet.remove(targets) },
        { kind: "separator" },
        ...this.selectionItems(scope),
        { kind: "separator" },
        { label: t.graphCopySelection, icon: icons.copy, note: "Ctrl+C", onSelect: () => this.ui.copy("selection") },
        { label: t.graphCopyChosen, icon: icons.copyCheck, disabled: workingSet.size === 0, onSelect: () => this.ui.copy("chosen") },
        { kind: "separator" },
        this.kindsItem(t.graphMenuDescendantsBy, icons.arrowDown, "descendants", found, take),
        this.kindsItem(t.graphMenuAncestorsBy, icons.arrowUp, "ancestors", found, take),
        this.kindsItem(t.graphMenuNeighbours, icons.arrowsDiff, "neighbours", found, take),
      ],
      x,
      y,
    );
  }

  /** «… по связи ▸ вид ▸ 1 уровень / все уровни» (neighbours: one level, both
   * directions). Other than inheritance, the label shows which end is followed. */
  private kindsItem(
    label: string,
    icon: string,
    reach: Reach,
    found: (reach: Reach, kinds: readonly string[] | null, all: boolean) => Set<string>,
    take: (reach: Reach, kinds: readonly string[] | null, all: boolean) => () => void,
  ): MenuItem {
    const kindItem = (name: string, kinds: readonly string[] | null): MenuItem => {
      const dir = kinds && !graphFilterConfig().hierarchyKinds.includes(kinds[0]!) && reach !== "neighbours" ? (reach === "descendants" ? " ←" : " →") : "";
      const first = found(reach, kinds, false).size;
      const item: MenuItem = {
        label: name + dir,
        icon: kinds ? kindIcon("edge", kinds[0]) : undefined,
        note: first ? String(first) : undefined,
        disabled: first === 0,
      };
      if (reach === "neighbours") {
        item.onSelect = take(reach, kinds, false);
      } else {
        item.submenu = () => [
          { label: t.graphMenuFirstLevel, note: String(first), onSelect: take(reach, kinds, false) },
          { label: t.graphMenuAllLevels, note: String(found(reach, kinds, true).size), onSelect: take(reach, kinds, true) },
        ];
      }
      return item;
    };
    return {
      label,
      icon,
      submenu: () => [kindItem(t.graphMenuAllKinds, null), { kind: "separator" }, ...this.edgeKindsOrdered().map((k) => kindItem(k, [k]))],
    };
  }

  /**
   * What copying these nodes puts on the clipboard: for each node its registry
   * entity (when it has one), name, base kind and current position, plus the
   * edge kinds drawn now and the edges among the copied nodes of those kinds.
   */
  clipboardPayload(ids: readonly string[]): GraphClipboard {
    const nodes: GraphClipboardNode[] = [];
    for (const id of new Set(ids)) {
      const n = this.nodeById.get(id);
      if (!n || !this.graph.hasNode(id)) continue;
      nodes.push({
        id,
        ...(n.entity && n.presence !== "code" ? { entity: n.entity } : {}),
        name: n.name ?? id,
        kind: n.symbolKind ?? n.kind ?? "",
        x: this.graph.getNodeAttribute(id, "x") as number,
        y: this.graph.getNodeAttribute(id, "y") as number,
      });
    }
    const copied = new Set(nodes.map((n) => n.id));
    const edgeKinds = [...filterStore.state.edgeKinds];
    const drawn = new Set(edgeKinds);
    const edges = this.edges.filter((e) => copied.has(e.from) && copied.has(e.to) && drawn.has(e.kind)).map((e) => ({ from: e.from, to: e.to, kind: e.kind }));
    return { format: GRAPH_CLIPBOARD_FORMAT, version: GRAPH_CLIPBOARD_VERSION, nodes, edgeKinds, edges };
  }

  /** Centres the camera on a node and selects it (double click in the list). */
  focusNode(id: string): void {
    if (!this.booted || !this.nodeById.has(id)) return;
    nodeSelection.setOnly(id);
    // The camera works in sigma's framed coordinates, not in the graph's own.
    const pos = this.renderer.getNodeDisplayData(id);
    if (pos) this.renderer.getCamera().animate({ x: pos.x, y: pos.y, ratio: 0.3 }, { duration: 400 });
    this.renderer.refresh();
  }

  /** Drag a node to see where its edges go; the position is not kept and the
   * next layout run overwrites it. */
  private wireDrag(): void {
    this.renderer.on("downNode", ({ node, event }) => {
      // Only the primary button drags: a right click opens the menu instead.
      const original = event.original;
      if ("button" in original && original.button !== 0) return;
      this.forceHandle?.stop();
      this.forceHandle = undefined;
      this.dragged = node;
      this.hovered = node;
      this.recomputeFocus();
      // Without a fixed bbox sigma re-fits the view to the moving node.
      if (!this.renderer.getCustomBBox()) this.renderer.setCustomBBox(this.renderer.getBBox());
    });
    const captor = this.renderer.getMouseCaptor();
    captor.on("mousemovebody", (e) => {
      if (!this.dragged) return;
      const pos = this.renderer.viewportToGraph(e);
      this.graph.setNodeAttribute(this.dragged, "x", pos.x);
      this.graph.setNodeAttribute(this.dragged, "y", pos.y);
      e.preventSigmaDefault();
      e.original.preventDefault();
      e.original.stopPropagation();
    });
    captor.on("mouseup", () => {
      this.dragged = undefined;
    });
  }

  private showPanel(node: string): void {
    const n = this.nodeById.get(node);
    if (n) this.panel.show(n, this.edges, this.nodeById);
  }

  // ------------------------------------------------------------- layouts

  /** The nodes a layout works on: all of them while there is no working set
   * (so a later filter change shows nodes that already have a place), else
   * only the shown ones — set ∩ universe. Never the hidden ones. */
  private layoutSubgraph(): Graph {
    const sub = new Graph({ type: "directed", multi: true });
    const scoped = workingSet.size > 0;
    this.graph.forEachNode((id, attrs) => {
      const n = this.nodeById.get(id);
      if (n && (!scoped || this.isShown(n))) sub.addNode(id, { ...attrs });
    });
    this.graph.forEachEdge((_edge, attrs, source, target) => {
      if (sub.hasNode(source) && sub.hasNode(target)) sub.addEdge(source, target, { ...attrs });
    });
    return sub;
  }

  /** The selected node if it is laid out, else the first member of the
   * working set that is, else the node with the most edges. */
  private radialCentre(sub: Graph): string {
    if (this.selected && sub.hasNode(this.selected)) return this.selected;
    for (const id of workingSet.members) if (sub.hasNode(id)) return id;
    let best = sub.nodes()[0]!;
    sub.forEachNode((id) => {
      if (sub.degree(id) > sub.degree(best)) best = id;
    });
    return best;
  }

  /** Parks the nodes a set-scoped layout leaves out at the centre of the
   * laid-out ones, so they do not stretch sigma's view of the rest. */
  private parkHidden(sub: Graph, positions?: Record<string, { x: number; y: number }>): void {
    if (workingSet.size === 0) return;
    let sx = 0, sy = 0, n = 0;
    sub.forEachNode((id, a) => {
      const p = positions?.[id] ?? a;
      sx += p.x;
      sy += p.y;
      n++;
    });
    const cx = n ? sx / n : 0, cy = n ? sy / n : 0;
    this.graph.forEachNode((id) => {
      if (sub.hasNode(id)) return;
      this.graph.setNodeAttribute(id, "x", cx);
      this.graph.setNodeAttribute(id, "y", cy);
    });
  }

  private applyLayout(): void {
    this.forceHandle?.stop();
    this.forceHandle = undefined;
    // A drag fixed the view's bounds; a new layout may need a new frame.
    this.renderer.setCustomBBox(null);

    const sub = this.layoutSubgraph();
    if (sub.order === 0) {
      this.renderer.refresh();
      return;
    }

    if (this.layout === "force") {
      seedCircle(sub);
      this.parkHidden(sub);
      const copyBack = () => {
        sub.forEachNode((id, a) => {
          if (!this.graph.hasNode(id)) return;
          this.graph.setNodeAttribute(id, "x", a.x);
          this.graph.setNodeAttribute(id, "y", a.y);
        });
        this.renderer.refresh();
      };
      this.forceHandle = runForceLayout(sub, () => this.renderer.refresh(), copyBack);
      return;
    }

    switch (this.layout) {
      case "hierarchy":
        hierarchyLayout(sub, graphFilterConfig().hierarchyKinds, this.canvasAspect());
        break;
      case "radial":
        // The centre is picked when the layout runs, not followed afterwards:
        // selecting another node re-centres only on "Пересчитать раскладку".
        radialLayout(sub, this.radialCentre(sub), filterStore.state.edgeKinds);
        break;
      case "circlepack":
        circlePackLayout(sub, (id) => {
          const n = this.nodeById.get(id);
          return [dirOf(n?.file), n?.namespace ?? ""];
        });
        break;
      case "circular":
        circularLayout(sub);
        break;
      case "random":
        randomLayout(sub);
        break;
      case "grouped":
        applyGroupedLayout(
          sub,
          groupNodes(this.groupBy, sub, {
            nodeOf: (id) => this.nodeById.get(id),
            depth: this.groupDepths[this.groupBy as LevelGroup] ?? 1,
            axis: this.groupAxis,
            groups: this.groupData,
            drawnKinds: filterStore.state.edgeKinds,
          }),
        );
        break;
    }
    const targets: Record<string, { x: number; y: number }> = {};
    sub.forEachNode((node, attrs) => {
      targets[node] = { x: attrs.x, y: attrs.y };
    });
    this.parkHidden(sub, targets);
    animateNodes(this.graph, targets, { duration: 500 });
  }
}

/** The canvas's own background (graph.css `--canvas-bg`): dimming mixes toward it. */
const CANVAS_BG = GRAPH_TUNING.canvasBg;

/** A colour dimmed by mixing it into the canvas background — an opaque colour, since
 * the WebGL nodes and edges drop alpha and an rgba would read as a light smear. */
function fade(color: string, keep: number = GRAPH_TUNING.dimKeepStrong): string {
  let r: number, g: number, b: number;
  const m = /^rgba?\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)/.exec(color);
  if (m) [r, g, b] = [Number(m[1]), Number(m[2]), Number(m[3])];
  else {
    const n = parseInt(color.slice(1), 16);
    [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  const mix = (c: number, bg: number) => Math.round(bg + (c - bg) * keep);
  return `rgb(${mix(r, CANVAS_BG[0])}, ${mix(g, CANVAS_BG[1])}, ${mix(b, CANVAS_BG[2])})`;
}
