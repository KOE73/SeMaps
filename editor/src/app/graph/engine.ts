import Graph from "graphology";
import Sigma from "sigma";
import type { NodeDisplayData, EdgeDisplayData } from "sigma/types";
import { animateNodes } from "sigma/utils";
import { el } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import type { GraphEdge, GraphNode, GraphResponse } from "./types.js";
import { type ColorBy, PRESENCE_CODE_COLOR, MODEL_MARKER, edgeRenderColor, edgeSize, legendFor, nodeColor, resetPalette } from "./colors.js";
import { type LayoutKind, applyGroupedLayout, communityGroups, groupKey, runForceLayout, seedCircle } from "./layouts.js";
import { type FilterState, allFilters, buildFilters, nodeVisible } from "./filters.js";
import { GraphPanel, statsLine } from "./panel.js";

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
  private colorBy: ColorBy = "kind";
  private layout: LayoutKind = "force";
  private filters: FilterState;
  private hovered: string | undefined;
  private selected: string | undefined;
  private forceHandle: { stop: () => void } | undefined;
  private readonly panel = new GraphPanel();
  private readonly legendEl = el("div", { class: "graph-legend" });
  private readonly filtersHost = el("div", {});
  private readonly statsEl = el("span", { class: "tool-muted" });
  private readonly noticeEl = el("span", { class: "graph-live-notice" });
  private readonly noticeHost = el("div", { class: "graph-notices" });
  private noticeTimer: ReturnType<typeof setTimeout> | undefined;
  private data: GraphResponse;

  constructor(data: GraphResponse) {
    this.data = data;
    for (const n of data.nodes) this.nodeById.set(n.id, n);
    this.edges = data.edges;
    this.filters = allFilters(data.nodes, [...new Set(data.edges.map((e) => e.kind))]);

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
      this.graph.setNodeAttribute(node, "size", 3 + Math.min(18, Math.sqrt(d) * 2.4));
    });

    seedCircle(this.graph);
  }

  /** Builds the toolbar + canvas + panel DOM. Sigma refuses a container
   * without a width, so it is started by `start()`, once the caller has put
   * the returned element into the page. */
  mount(): HTMLElement {
    const search = el("input", { class: "graph-search", placeholder: t.graphSearchPlaceholder }) as HTMLInputElement;
    search.addEventListener("input", () => this.search(search.value));

    const colorSelect = el("select", {}) as HTMLSelectElement;
    const colorOptions: [ColorBy, string][] = [
      ["kind", t.graphColorKind],
      ["container", t.graphColorContainer],
      ["namespace", t.graphColorNamespace],
      ["presence", t.graphColorPresence],
    ];
    for (const [v, label] of colorOptions) colorSelect.appendChild(el("option", { value: v, text: label }));
    colorSelect.value = this.colorBy;
    colorSelect.addEventListener("change", () => {
      this.colorBy = colorSelect.value as ColorBy;
      resetPalette();
      this.refreshLegend();
      this.renderer.refresh();
    });

    const layoutSelect = el("select", {}) as HTMLSelectElement;
    const layoutOptions: [LayoutKind, string][] = [
      ["force", t.graphLayoutForce],
      ["folder", t.graphLayoutFolder],
      ["namespace", t.graphLayoutNamespace],
      ["community", t.graphLayoutCommunity],
      ["container", t.graphLayoutContainer],
    ];
    for (const [v, label] of layoutOptions) layoutSelect.appendChild(el("option", { value: v, text: label }));
    layoutSelect.value = this.layout;
    layoutSelect.addEventListener("change", () => {
      this.layout = layoutSelect.value as LayoutKind;
      this.applyLayout();
    });

    const restart = el("button", { class: "tool-btn", text: t.graphRestartLayout });
    restart.addEventListener("click", () => this.applyLayout());

    const toolbar = el("div", { class: "graph-toolbar" }, [
      el("label", { class: "graph-field" }, [search]),
      el("label", { class: "graph-field" }, [t.graphColorBy, colorSelect]),
      el("label", { class: "graph-field" }, [t.graphLayout, layoutSelect]),
      restart,
      el("span", { class: "spacer" }),
      this.noticeEl,
      this.statsEl,
    ]);

    const canvas = el("div", { class: "graph-canvas" });
    const sideDock = el("div", { class: "graph-side" }, [this.legendEl, this.filtersHost, this.panel.root]);
    const body = el("div", { class: "graph-body" }, [canvas, sideDock]);
    const root = el("div", { class: "graph-mode" }, [toolbar, body]);
    root.prepend(this.noticeHost);

    this.canvas = canvas;
    this.refreshNotices();
    return root;
  }

  /** Mounts sigma and starts the default (force) layout. */
  start(): void {
    this.renderer = new Sigma(this.graph, this.canvas, {
      renderLabels: true,
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
    this.watchResize();
    this.refreshLegend();
    const { root: filtersRoot } = buildFilters(this.data.nodes, [...new Set(this.data.edges.map((e) => e.kind))], this.filters, () => {
      this.renderer.refresh();
    });
    this.filtersHost.replaceChildren(filtersRoot);
    this.statsEl.textContent = statsLine(this.data.nodes.length, this.data.edges.length, this.data.stats.hiddenMissing);

    this.applyLayout();
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

  destroy(): void {
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
    const oldSymbolKinds = new Set(this.data.nodes.map((n) => n.kind).filter((k): k is string => !!k));
    const oldVisibility = new Set(this.data.nodes.map((n) => n.visibility).filter((v): v is string => !!v));
    const oldEdgeKinds = new Set(this.edges.map((e) => e.kind));
    const oldContainers = new Set(this.data.nodes.flatMap((n) => n.containers ?? []));

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

    // Degree-based size, recomputed for everyone — cheap next to a layout.
    this.graph.forEachNode((node) => {
      const d = this.graph.degree(node);
      this.graph.setNodeAttribute(node, "size", 3 + Math.min(18, Math.sqrt(d) * 2.4));
    });

    this.data = next;

    // Merge filter selections: a value already offered keeps the user's
    // choice, a newly offered one starts included.
    const merge = (selected: Set<string>, oldAvailable: Set<string>, nowAvailable: Set<string>) => {
      const out = new Set<string>();
      for (const v of nowAvailable) if (!oldAvailable.has(v) || selected.has(v)) out.add(v);
      return out;
    };
    const newSymbolKinds = new Set(next.nodes.map((n) => n.kind).filter((k): k is string => !!k));
    const newVisibility = new Set(next.nodes.map((n) => n.visibility).filter((v): v is string => !!v));
    const newEdgeKinds = new Set(next.edges.map((e) => e.kind));
    const newContainers = new Set(next.nodes.flatMap((n) => n.containers ?? []));
    this.filters.symbolKinds = merge(this.filters.symbolKinds, oldSymbolKinds, newSymbolKinds);
    this.filters.visibility = merge(this.filters.visibility, oldVisibility, newVisibility);
    this.filters.edgeKinds = merge(this.filters.edgeKinds, oldEdgeKinds, newEdgeKinds);
    if (this.filters.container && !newContainers.has(this.filters.container)) this.filters.container = "";
    void oldContainers;

    if (this.selected && removedNodeIds.includes(this.selected)) {
      this.selected = undefined;
      this.panel.clear();
    }

    resetPalette();
    this.refreshLegend();
    this.refreshNotices();
    const { root: filtersRoot } = buildFilters(next.nodes, [...newEdgeKinds], this.filters, () => this.renderer.refresh());
    this.filtersHost.replaceChildren(filtersRoot);
    this.statsEl.textContent = statsLine(next.nodes.length, next.edges.length, next.stats.hiddenMissing);
    this.renderer.refresh();

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
    if (this.data.facts.length === 0) {
      const link = el("a", { text: t.graphOpenExtractors });
      link.href = "#extract";
      bars.push(el("p", { class: "graph-notice" }, [t.graphNoFactsNotice + " ", link]));
    }
    for (const f of this.data.facts) {
      if (!f.lastRunFailed) continue;
      const link = el("a", { text: t.graphOpenExtractors });
      link.href = "#extract";
      bars.push(el("p", { class: "graph-notice is-warn" }, [fmt(t.graphRunFailedNotice, { extractor: f.extractor }) + " ", link]));
    }
    this.noticeHost.replaceChildren(...bars);
  }

  // ------------------------------------------------------------ rendering

  private nodeReducer(node: string, attrs: Record<string, unknown>): Partial<NodeDisplayData> {
    const n = this.nodeById.get(node);
    const res: Partial<NodeDisplayData> = { ...(attrs as unknown as NodeDisplayData) };
    if (!n || !nodeVisible(n, this.filters)) {
      res.hidden = true;
      return res;
    }
    res.color = nodeColor(n, this.colorBy);
    const label = (n.presence === "model" ? MODEL_MARKER : "") + (n.name ?? n.id);
    res.label = label;

    const focus = this.selected ?? this.hovered;
    if (focus) {
      const isFocus = node === focus;
      const isNeighbor = this.graph.areNeighbors(node, focus);
      if (!isFocus && !isNeighbor) {
        res.color = fade(res.color as string);
        res.label = null;
        res.zIndex = 0;
      } else {
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
    if (!from || !to || !nodeVisible(from, this.filters) || !nodeVisible(to, this.filters)) {
      res.hidden = true;
      return res;
    }
    const kind = this.graph.getEdgeAttribute(edge, "kind") as string;
    if (!this.filters.edgeKinds.has(kind)) {
      res.hidden = true;
      return res;
    }
    const via = this.graph.getEdgeAttribute(edge, "via") as GraphEdge["via"];
    res.color = edgeRenderColor({ kind, via } as GraphEdge);
    res.size = edgeSize({ via } as GraphEdge);

    const focus = this.selected ?? this.hovered;
    if (focus && ext[0] !== focus && ext[1] !== focus) {
      res.hidden = true;
    }
    return res;
  }

  private refreshLegend(): void {
    const entries = legendFor(this.data.nodes, this.colorBy);
    this.legendEl.replaceChildren(
      el("h2", { text: t.graphLegend }),
      el(
        "div",
        { class: "graph-legend-list" },
        entries.map((it) => el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch", attrs: { style: `background:${it.swatch}` } }), it.label])),
      ),
      el("div", { class: "graph-legend-list" }, [
        el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch", attrs: { style: `background:${PRESENCE_CODE_COLOR}` } }), t.graphPresenceCode]),
        el("div", { class: "graph-legend-item" }, [el("span", { class: "graph-swatch graph-swatch-model" }), MODEL_MARKER + t.graphPresenceModel]),
      ]),
    );
  }

  // -------------------------------------------------------------- events

  private wireEvents(): void {
    this.renderer.on("enterNode", ({ node }) => {
      this.hovered = node;
      this.renderer.refresh();
    });
    this.renderer.on("leaveNode", () => {
      this.hovered = undefined;
      this.renderer.refresh();
    });
    this.renderer.on("clickNode", ({ node }) => {
      this.selected = node;
      this.showPanel(node);
      this.renderer.refresh();
    });
    this.renderer.on("clickStage", () => {
      this.selected = undefined;
      this.panel.clear();
      this.renderer.refresh();
    });
  }

  private showPanel(node: string): void {
    const n = this.nodeById.get(node);
    if (n) this.panel.show(n, this.edges, this.nodeById);
  }

  private search(query: string): void {
    const q = query.trim().toLowerCase();
    if (!q) return;
    const found = this.data.nodes.find((n) => (n.name ?? n.id).toLowerCase().includes(q));
    if (!found) return;
    this.selected = found.id;
    this.showPanel(found.id);
    // The camera works in sigma's framed coordinates, not in the graph's own.
    const pos = this.renderer.getNodeDisplayData(found.id);
    if (pos) this.renderer.getCamera().animate({ x: pos.x, y: pos.y, ratio: 0.3 }, { duration: 400 });
    this.renderer.refresh();
  }

  // ------------------------------------------------------------- layouts

  private applyLayout(): void {
    this.forceHandle?.stop();
    this.forceHandle = undefined;

    if (this.layout === "force") {
      seedCircle(this.graph);
      this.forceHandle = runForceLayout(this.graph, () => this.renderer.refresh());
      return;
    }

    const groupOf = this.layout === "community" ? communityGroups(this.graph) : new Map(this.graph.nodes().map((n) => [n, groupKey(this.nodeById.get(n)!, this.layout as "folder" | "namespace" | "container")]));
    const before = new Graph();
    before.import(this.graph.export());
    applyGroupedLayout(before, groupOf);
    const targets: Record<string, { x: number; y: number }> = {};
    before.forEachNode((node, attrs) => {
      targets[node] = { x: attrs.x, y: attrs.y };
    });
    animateNodes(this.graph, targets, { duration: 500 });
  }
}

function fade(hex: string): string {
  if (hex.startsWith("rgb")) return hex.replace("rgb(", "rgba(").replace(")", ", 0.15)");
  const n = parseInt(hex.slice(1), 16);
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  return `rgba(${r}, ${g}, ${b}, 0.15)`;
}
