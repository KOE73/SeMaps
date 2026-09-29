import { createDockview, type DockviewApi, type IContentRenderer } from "dockview-core";
import { PanelService } from "../../workbench/dockview/PanelService.js";
import { WorkspaceLayoutService } from "../../workbench/dockview/WorkspaceLayoutService.js";
import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import { FiltersView, filterStore } from "./filters.js";
import { GraphPanel } from "./panel.js";
import { NodesView } from "./nodesView.js";
import { ExtractorsView } from "./extractorsView.js";

/**
 * The graph mode's dock: the canvas in the middle and the legend, filters and
 * properties panels in one group to its right, moved/tabbed/closed/reopened
 * like the diagram editor's (`DockviewHost`). It reuses the editor's
 * `PanelService` and `WorkspaceLayoutService` and keeps its own layout under
 * its own key. It never imports the engine (sigma stays behind the dynamic
 * `import()`, ADR_20260928-2).
 *
 * The panels' contents are built once here and outlive a re-created engine;
 * a panel closed and reopened gets the same element again, and the engine
 * only fills what it is given (`ui`).
 */
export const GRAPH_PANEL = {
  canvas: "graph",
  legend: "graph-legend",
  filters: "graph-filters",
  properties: "graph-properties",
  nodes: "graph-nodes",
  extractors: "graph-extractors",
} as const;

// v2: the default moved to «Узлы» on the right and the other panels on the left.
const STORAGE_KEY = "semaps:graph-dockview-layout-v2";

export interface GraphDockUi {
  readonly legend: HTMLElement;
  readonly panel: GraphPanel;
  readonly nodes: NodesView;
  readonly extractors: ExtractorsView;
  openExtractors(): void;
  copy(which: "selection" | "chosen"): void;
}

export class GraphDock {
  readonly panels = new PanelService({
    centerId: GRAPH_PANEL.canvas,
    rightIds: [GRAPH_PANEL.nodes],
    groups: [
      { ids: [GRAPH_PANEL.filters, GRAPH_PANEL.legend, GRAPH_PANEL.properties, GRAPH_PANEL.extractors], side: "left", width: 320 },
      { ids: [GRAPH_PANEL.nodes], side: "right", width: 340 },
    ],
  });
  readonly layout = new WorkspaceLayoutService(STORAGE_KEY);
  readonly ui: GraphDockUi;
  /** Where the engine puts its canvas (and where messages go). */
  readonly canvasSlot = el("div", { class: "graph-canvas-slot" });
  /** Set by graph.ts, which owns the engine the copy is read from. */
  copyHandler: (which: "selection" | "chosen") => void = () => {};
  private readonly dockview: DockviewApi;
  private readonly elements: Record<string, HTMLElement>;

  constructor(container: HTMLElement) {
    const legend = el("div", { class: "graph-legend" });
    const panel = new GraphPanel();
    const filters = new FiltersView(filterStore);
    const nodes = new NodesView();
    const extractors = new ExtractorsView();
    this.ui = {
      legend,
      panel,
      nodes,
      extractors,
      openExtractors: () => this.panels.focus(GRAPH_PANEL.extractors),
      copy: (which) => this.copyHandler(which),
    };
    this.elements = {
      [GRAPH_PANEL.canvas]: this.canvasSlot,
      [GRAPH_PANEL.legend]: el("div", { class: "graph-dock-pane" }, [legend]),
      [GRAPH_PANEL.filters]: el("div", { class: "graph-dock-pane" }, [filters.root]),
      [GRAPH_PANEL.properties]: el("div", { class: "graph-dock-pane" }, [panel.root]),
      [GRAPH_PANEL.nodes]: el("div", { class: "graph-dock-pane graph-dock-pane-fill" }, [nodes.root]),
      [GRAPH_PANEL.extractors]: el("div", { class: "graph-dock-pane" }, [extractors.root]),
    };

    this.dockview = createDockview(container, {
      createComponent: (options) => this.panels.createRenderer(options.name),
    });
    this.panels.init({ dockview: this.dockview, container });

    const titles: [string, string][] = [
      [GRAPH_PANEL.canvas, t.graphPanelCanvas],
      [GRAPH_PANEL.legend, t.graphLegend],
      [GRAPH_PANEL.filters, t.graphFilters],
      [GRAPH_PANEL.properties, t.graphPanelTitle],
      [GRAPH_PANEL.nodes, t.graphPanelNodes],
      [GRAPH_PANEL.extractors, t.graphPanelExtractors],
    ];
    for (const [id, title] of titles) {
      this.panels.register({
        id,
        title,
        minWidth: id === GRAPH_PANEL.canvas ? 200 : 100,
        minHeight: id === GRAPH_PANEL.canvas ? 150 : 80,
        createRenderer: () => this.renderer(id),
      });
    }

    this.layout.init({ dockview: this.dockview, container }, () => this.setupDefaultLayout());
    if (!this.layout.loadLayout() || !this.dockview.getPanel(GRAPH_PANEL.canvas)) this.setupDefaultLayout();

    this.dockview.onDidLayoutChange(() => this.layout.saveLayout());
    // The canvas is the centre and cannot stay closed.
    this.dockview.onDidRemovePanel((e) => {
      if (e.id !== GRAPH_PANEL.canvas) return;
      setTimeout(() => {
        if (!this.dockview.getPanel(GRAPH_PANEL.canvas)) {
          this.dockview.addPanel({ id: GRAPH_PANEL.canvas, component: GRAPH_PANEL.canvas, title: t.graphPanelCanvas, minimumWidth: 200, minimumHeight: 150 });
        }
      }, 50);
    });
  }

  /** Called whenever a panel opens, closes or changes focus. */
  onPanelStateChange(listener: () => void): void {
    this.panels.onPanelStateChange(listener);
  }

  private renderer(id: string): IContentRenderer {
    return { element: this.elements[id]!, init: () => {} };
  }

  private setupDefaultLayout(): void {
    this.dockview.clear();
    const canvas = this.dockview.addPanel({
      id: GRAPH_PANEL.canvas,
      component: GRAPH_PANEL.canvas,
      title: t.graphPanelCanvas,
      minimumWidth: 200,
      minimumHeight: 150,
    });
    canvas.group.locked = "no-drop-target";

    // The right edge: the node list. The left: everything that configures the graph.
    this.dockview.addPanel({
      id: GRAPH_PANEL.nodes,
      component: GRAPH_PANEL.nodes,
      title: t.graphPanelNodes,
      position: { direction: "right", referencePanel: canvas },
      initialWidth: 340,
      minimumWidth: 100,
      minimumHeight: 80,
    });
    const left = this.dockview.addPanel({
      id: GRAPH_PANEL.filters,
      component: GRAPH_PANEL.filters,
      title: t.graphFilters,
      position: { direction: "left", referencePanel: canvas },
      initialWidth: 320,
      minimumWidth: 100,
      minimumHeight: 80,
    });
    for (const [id, title] of [
      [GRAPH_PANEL.legend, t.graphLegend],
      [GRAPH_PANEL.properties, t.graphPanelTitle],
      [GRAPH_PANEL.extractors, t.graphPanelExtractors],
    ] as const) {
      this.dockview.addPanel({
        id,
        component: id,
        title,
        position: { direction: "within", referencePanel: left },
        minimumWidth: 100,
        minimumHeight: 80,
      });
    }
    left.api.setActive();
    canvas.api.setActive();
  }
}
