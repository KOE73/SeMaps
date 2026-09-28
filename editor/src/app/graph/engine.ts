import Graph from "graphology";
import Sigma from "sigma";
import type { NodeDisplayData, EdgeDisplayData } from "sigma/types";
import { animateNodes } from "sigma/utils";
import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import type { GraphEdge, GraphNode, GraphResponse } from "./types.js";
import { type ColorBy, PRESENCE_CODE_COLOR, MODEL_MARKER, edgeRenderColor, edgeSize, legendFor, nodeColor, resetPalette } from "./colors.js";
import { type LayoutKind, applyGroupedLayout, communityGroups, groupKey, runForceLayout, seedCircle } from "./layouts.js";
import { type FilterState, allFilters, buildFilters, nodeVisible } from "./filters.js";
import { GraphPanel, statsLine } from "./panel.js";

const LARGE_DEGREE_LABEL = 6; // nodes with at least this many edges always show a label

/**
 * Everything that touches sigma/graphology: built once per entry into the
 * graph mode, from `GraphResponse`. Loaded through a dynamic `import()` so
 * these dependencies never reach the library bundle (ADR_20260928-2).
 */
export class GraphEngine {
  private readonly graph: Graph;
  private readonly nodeById = new Map<string, GraphNode>();
  private readonly edges: GraphEdge[];
  private renderer!: Sigma;
  private canvas!: HTMLElement;
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

  constructor(private readonly data: GraphResponse) {
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
      this.statsEl,
    ]);

    const canvas = el("div", { class: "graph-canvas" });
    const sideDock = el("div", { class: "graph-side" }, [this.legendEl, this.filtersHost, this.panel.root]);
    const body = el("div", { class: "graph-body" }, [canvas, sideDock]);
    const root = el("div", { class: "graph-mode" }, [toolbar, body]);

    if (this.data.facts.length === 0) {
      root.prepend(this.noticeBar());
    }

    this.canvas = canvas;
    return root;
  }

  /** Mounts sigma and starts the default (force) layout. */
  start(): void {
    this.renderer = new Sigma(this.graph, this.canvas, {
      renderLabels: true,
      labelRenderedSizeThreshold: 8,
      // The canvas is dark (graph.css): sigma's default label is black.
      labelColor: { color: "#e6e8ec" },
      nodeReducer: (node, attrs) => this.nodeReducer(node, attrs),
      edgeReducer: (edge, attrs) => this.edgeReducer(edge, attrs),
    });
    this.wireEvents();
    this.refreshLegend();
    const { root: filtersRoot } = buildFilters(this.data.nodes, [...new Set(this.data.edges.map((e) => e.kind))], this.filters, () => {
      this.renderer.refresh();
    });
    this.filtersHost.replaceChildren(filtersRoot);
    this.statsEl.textContent = statsLine(this.data.nodes.length, this.data.edges.length);

    this.applyLayout();
  }

  destroy(): void {
    this.forceHandle?.stop();
    this.renderer?.kill();
  }

  private noticeBar(): HTMLElement {
    const link = el("a", { text: t.graphOpenExtractors });
    link.href = "#extract";
    return el("p", { class: "graph-notice" }, [t.graphNoFactsNotice + " ", link]);
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
    } else if ((attrs.size as number) >= LARGE_DEGREE_LABEL) {
      res.forceLabel = true;
    }
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
