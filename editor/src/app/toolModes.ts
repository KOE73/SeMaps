import { el } from "../util/dom.js";
import type { Workbench } from "../workbench/Workbench.js";
import type { WorkbenchMode } from "../workbench/modes.js";
import type { CommandDefinition } from "../workbench/commands/types.js";
import { focusNewExtractor, loadExtractors } from "./extract.js";
import { loadProject, saveProject } from "./setup.js";
import { installMcp, loadMcp, saveMcp } from "./mcp.js";
import {
  bindGraphRibbon,
  copyGraphNodes,
  graphColorBy,
  graphFocus,
  graphGroupBy,
  setGraphFocus,
  setGraphGroupBy,
  graphLayout,
  graphProject,
  graphProjectOptions,
  graphShowsMissing,
  isGraphPanelOpen,
  leaveGraph,
  loadGraph,
  refreshGraph,
  restartGraphLayout,
  resetGraphPanels,
  resumeGraph,
  setGraphColorBy,
  setGraphLayout,
  setGraphProject,
  toggleGraphMissing,
  toggleGraphPanel,
} from "./graph.js";
import { filterStore } from "./graph/filters.js";
import { filterPreset } from "./graph/filterConfig.js";
import { GRAPH_PANEL } from "./graph/dock.js";
import { graphIcons } from "./graph/icons.js";
import { COLOR_OPTIONS, FOCUS_OPTIONS, GROUP_OPTIONS, LAYOUT_OPTIONS } from "./graph/viewOptions.js";
import { kindIcon } from "../ui/kindIcons.js";
import { t } from "../shell/strings.js";
import { icons } from "../ui/icons.js";

/** Every graph command goes through graph.ts / the filter store; nothing
 * here imports the engine (sigma stays behind the dynamic import). */
/** A preset of graph-filters.json: usable when the data has any of its edge kinds. */
function presetEnabled(id: string): boolean {
  const p = filterPreset(id);
  return p !== undefined && filterStore.hasEdgeKind(p.edgeKinds);
}

function applyPreset(id: string): void {
  const p = filterPreset(id);
  if (p) filterStore.onlyKinds(p.edgeKinds, p.symbolKinds);
}

function graphCommands(): CommandDefinition[] {
  const presence = (id: string, value: "code" | "model" | "both", title: string, icon: string): CommandDefinition => ({
    id,
    title,
    icon,
    isChecked: () => filterStore.state.presence.has(value),
    execute: () => filterStore.toggle("presence", value),
  });
  const panel = (id: string, panelId: (typeof GRAPH_PANEL)[keyof typeof GRAPH_PANEL], title: string, icon: string): CommandDefinition => ({
    id,
    title,
    icon,
    isChecked: () => isGraphPanelOpen(panelId),
    execute: () => toggleGraphPanel(panelId),
  });
  return [
    { id: "tools.graph.refresh", title: t.refresh, description: t.refreshHint, icon: graphIcons.refresh, execute: () => void refreshGraph() },
    { id: "tools.graph.copy.selection", title: t.graphCopySelection, icon: icons.copy, shortcut: "Ctrl+C", execute: () => copyGraphNodes("selection") },
    { id: "tools.graph.copy.chosen", title: t.graphCopyChosen, icon: icons.copyCheck, execute: () => copyGraphNodes("chosen") },
    { id: "tools.graph.project.set", title: t.graphProject, icon: icons.folder, execute: (_c, v) => setGraphProject(String(v)) },
    { id: "tools.graph.missing.toggle", title: t.graphShowMissing, icon: graphIcons.ghost, isChecked: () => graphShowsMissing(), execute: () => toggleGraphMissing() },
    { id: "tools.graph.color.set", title: t.graphColorBy, icon: icons.palette, execute: (_c, v) => setGraphColorBy(String(v)) },
    { id: "tools.graph.focus.set", title: t.graphFocus, icon: graphIcons.focus, execute: (_c, v) => setGraphFocus(String(v)) },
    { id: "tools.graph.groupby.set", title: t.graphGroupBy, icon: icons.category, execute: (_c, v) => setGraphGroupBy(String(v)) },
    { id: "tools.graph.layout.set", title: t.graphLayout, icon: icons.compass, execute: (_c, v) => setGraphLayout(String(v)) },
    { id: "tools.graph.layout.restart", title: t.graphRestartLayout, icon: graphIcons.shuffle, execute: () => restartGraphLayout() },
    { id: "tools.graph.filters.reset", title: t.graphResetFilters, icon: graphIcons.restore, execute: () => filterStore.reset() },
    { id: "tools.graph.filters.all", title: t.graphEnableAll, icon: graphIcons.checks, execute: () => filterStore.enableAll() },
    { id: "tools.graph.filters.none", title: t.graphDisableAll, icon: graphIcons.squareOff, execute: () => filterStore.disableAll() },
    {
      id: "tools.graph.filters.inheritance",
      title: t.graphOnlyInheritance,
      icon: kindIcon("edge", "extends"),
      isEnabled: () => presetEnabled("inheritance"),
      execute: () => applyPreset("inheritance"),
    },
    {
      id: "tools.graph.filters.dependencies",
      title: t.graphInheritanceDependencies,
      icon: graphIcons.hierarchy,
      isEnabled: () => presetEnabled("inheritance-dependencies"),
      execute: () => applyPreset("inheritance-dependencies"),
    },
    {
      id: "tools.graph.filters.calls",
      title: t.graphPresetCalls,
      icon: kindIcon("edge", "calls"),
      isEnabled: () => presetEnabled("calls"),
      execute: () => applyPreset("calls"),
    },
    presence("tools.graph.presence.code", "code", t.graphPresenceCode, kindIcon("presence", "code")),
    presence("tools.graph.presence.model", "model", t.graphPresenceModel, kindIcon("presence", "model")),
    presence("tools.graph.presence.both", "both", t.graphPresenceBoth, kindIcon("presence", "both")),
    panel("tools.graph.panel.legend", GRAPH_PANEL.legend, t.graphLegend, graphIcons.palette),
    panel("tools.graph.panel.filters", GRAPH_PANEL.filters, t.graphFilters, graphIcons.filter),
    panel("tools.graph.panel.view", GRAPH_PANEL.view, t.graphPanelView, graphIcons.view),
    panel("tools.graph.panel.extractors", GRAPH_PANEL.extractors, t.graphPanelExtractors, graphIcons.cpu),
    panel("tools.graph.panel.nodes", GRAPH_PANEL.nodes, t.graphPanelNodes, graphIcons.listSearch),
    panel("tools.graph.panel.properties", GRAPH_PANEL.properties, t.graphPanelTitle, graphIcons.listDetails),
    { id: "tools.graph.panels.reset", title: t.graphResetPanels, icon: graphIcons.layoutBoard, execute: () => resetGraphPanels() },
  ];
}

/**
 * The modes after the diagrams, present only when the host was started from a
 * .semaps file: Extractors, Project and MCP. Each is a page in a scrolling surface,
 * loaded when first entered, with a ribbon tab of its own.
 */
export function addToolModes(workbench: Workbench): void {
  const extract = page();
  const project = page();
  const mcp = page();
  const graphInner = el("div", { class: "graph-page-inner" });
  const graph: Page = { surface: el("main", { class: "tool-page graph-surface" }, [graphInner]), inner: graphInner };

  const commands: CommandDefinition[] = [
    {
      id: "tools.extract.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: icons.refresh,
      execute: () => void loadExtractors(extract.inner),
    },
    {
      id: "tools.extract.add",
      title: t.addExtractor,
      icon: icons.plus,
      execute: () => focusNewExtractor(extract.inner),
    },
    {
      id: "tools.project.save",
      title: t.save,
      icon: icons.save,
      execute: () => saveProject(project.inner),
    },
    {
      id: "tools.project.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: icons.refresh,
      execute: () => void loadProject(project.inner),
    },
    ...graphCommands(),
    {
      id: "tools.mcp.install",
      title: t.mcpInstall,
      icon: icons.plug,
      execute: () => void installMcp(mcp.inner),
    },
    {
      id: "tools.mcp.save",
      title: t.save,
      icon: icons.save,
      execute: () => saveMcp(mcp.inner),
    },
    {
      id: "tools.mcp.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: icons.refresh,
      execute: () => void loadMcp(mcp.inner),
    },
  ];
  workbench.commands.registerAll(commands);
  bindGraphRibbon(() => workbench.commands.notifyStateChanged());

  workbench.addMode(mode("extract", t.navExtract, extract, () => loadExtractors(extract.inner), [
    { id: "extractors", title: t.navExtract, items: [
      { type: "button", command: "tools.extract.add", size: "large" },
      { type: "button", command: "tools.extract.refresh", size: "large" },
    ] },
  ]));
  workbench.addMode(mode("project", t.navSetup, project, () => loadProject(project.inner), [
    { id: "project", title: t.navSetup, items: [
      { type: "button", command: "tools.project.save", size: "large" },
      { type: "button", command: "tools.project.refresh", size: "large" },
    ] },
  ]));
  workbench.addMode(mode("graph", t.navGraph, graph, () => loadGraph(graph.inner), [
    { id: "graph", title: t.graphGroupGraph, items: [
      { type: "button", command: "tools.graph.refresh", size: "large", showLabel: false },
      {
        type: "select",
        command: "tools.graph.project.set",
        label: t.graphProject,
        get options() { return graphProjectOptions(); },
        getValue: () => graphProject(),
      },
      { type: "toggle", command: "tools.graph.missing.toggle", size: "medium", showLabel: false },
      { type: "separator" },
      { type: "button", command: "tools.graph.copy.selection", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.copy.chosen", size: "medium", showLabel: false },
    ] },
    { id: "graph-view", title: t.graphGroupView, items: [
      {
        type: "select",
        command: "tools.graph.color.set",
        label: t.graphColorBy,
        // The same lists the «Вид» panel shows (viewOptions.ts), over the same store.
        options: COLOR_OPTIONS.map(({ value, label }) => ({ value, label })),
        getValue: () => graphColorBy(),
      },
      {
        type: "select",
        command: "tools.graph.focus.set",
        label: t.graphFocus,
        options: FOCUS_OPTIONS.map(({ value, label }) => ({ value, label })),
        getValue: () => graphFocus(),
      },
      {
        type: "select",
        command: "tools.graph.layout.set",
        label: t.graphLayout,
        options: LAYOUT_OPTIONS.map(({ value, label }) => ({ value, label })),
        getValue: () => graphLayout(),
      },
      {
        type: "select",
        command: "tools.graph.groupby.set",
        label: t.graphGroupBy,
        options: GROUP_OPTIONS.map(({ value, label }) => ({ value, label })),
        getValue: () => graphGroupBy(),
        // Grouping shapes the «группами» layout and the colour «по группе»: greyed under neither, and choosing one switches the layout over.
        dimmed: () => graphLayout() !== "grouped" && graphColorBy() !== "group",
      },
      { type: "button", command: "tools.graph.layout.restart", size: "large", showLabel: false },
    ] },
    { id: "graph-filters", title: t.graphGroupFilters, items: [
      { type: "toggle", command: "tools.graph.panel.filters", size: "large", showLabel: false },
      { type: "button", command: "tools.graph.filters.reset", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.filters.all", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.filters.none", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.filters.inheritance", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.filters.dependencies", size: "medium", showLabel: false },
      { type: "button", command: "tools.graph.filters.calls", size: "medium", showLabel: false },
      { type: "separator" },
      { type: "toggle", command: "tools.graph.presence.code", size: "medium", showLabel: false },
      { type: "toggle", command: "tools.graph.presence.model", size: "medium", showLabel: false },
      { type: "toggle", command: "tools.graph.presence.both", size: "medium", showLabel: false },
    ] },
    { id: "graph-panels", title: t.graphGroupPanels, items: [
      { type: "toggle", command: "tools.graph.panel.legend", size: "small", showLabel: false },
      { type: "toggle", command: "tools.graph.panel.filters", size: "small", showLabel: false },
      { type: "toggle", command: "tools.graph.panel.view", size: "small", showLabel: false },
      { type: "toggle", command: "tools.graph.panel.extractors", size: "small", showLabel: false },
      { type: "toggle", command: "tools.graph.panel.nodes", size: "small", showLabel: false },
      { type: "toggle", command: "tools.graph.panel.properties", size: "small", showLabel: false },
      { type: "separator" },
      { type: "button", command: "tools.graph.panels.reset", size: "small", showLabel: false },
    ] },
  ], leaveGraph, resumeGraph));
  workbench.addMode(mode("mcp", t.navMcp, mcp, () => loadMcp(mcp.inner), [
    { id: "mcp", title: t.navMcp, items: [
      { type: "button", command: "tools.mcp.save", size: "large" },
      { type: "button", command: "tools.mcp.install", size: "large" },
      { type: "button", command: "tools.mcp.refresh", size: "large" },
    ] },
  ]));
}

interface Page {
  readonly surface: HTMLElement;
  readonly inner: HTMLElement;
}

function page(): Page {
  const inner = el("div", { class: "tool-page-inner" });
  const surface = el("main", { class: "tool-page" }, [inner]);
  return { surface, inner };
}

function mode(
  id: string,
  title: string,
  p: Page,
  load: () => Promise<void>,
  groups: WorkbenchMode["tabs"][number]["groups"],
  leave?: () => void,
  resume?: () => void,
): WorkbenchMode {
  let loaded = false;
  return {
    id,
    title,
    surface: p.surface,
    tabs: [{ id, title, groups }],
    enter: () => {
      if (loaded) {
        resume?.();
        return;
      }
      loaded = true;
      void load();
    },
    leave,
  };
}
