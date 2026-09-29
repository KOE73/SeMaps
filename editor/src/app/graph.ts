import { el } from "../util/dom.js";
import { fmt, t } from "../shell/strings.js";
import { toast } from "../ui/toast.js";
import { encodeGraphClipboard } from "../model/graphClipboard.js";
import { GraphDock, type GRAPH_PANEL } from "./graph/dock.js";
import { filterStore } from "./graph/filters.js";
import { nodeSelection, workingSet } from "./graph/workset.js";
import { COLOR_BYS, FOCUS_MODES, GROUP_BYS, LAYOUTS, viewSettings } from "./graph/viewSettings.js";

/**
 * The Graph mode: sigma + graphology, on `GET /api/graph/{project}`
 * (PLAN_20260928-2 step 2). `sigma`/`graphology` and friends are loaded with
 * a dynamic `import()` on first entry, so they never reach the library
 * bundle of `@semaps/editor` (ADR_20260928-2) — only `npm run build:app`'s
 * chunk for this mode carries them. The dock (`graph/dock.ts`) and the filter
 * store do not touch sigma and are imported statically.
 *
 * The page is a dock: the canvas in the middle, legend/filters/properties
 * panels beside it. Its controls are the ribbon's commands (`toolModes.ts`),
 * which reach the engine only through the functions exported here.
 *
 * Live follow (PLAN_20260928-2 step 6): a separate `EventSource` — not
 * `HostModelStore`'s own — subscribes to `/api/events?project=<id>` while the
 * mode is shown for that project. An event with a `graph` field triggers a
 * refetch of the same query and a diff applied to the live graphology graph
 * in place (`GraphEngine.applyDiff`); events without `graph` are ignored.
 * Events arriving while a refetch is in flight coalesce into one more
 * refetch, not one per event.
 */

type GraphPanelId = (typeof GRAPH_PANEL)[keyof typeof GRAPH_PANEL];

let projects: { id: string; title?: string }[] = [];
let currentProject = "";
let showMissing = false;
let engine: import("./graph/engine.js").GraphEngine | undefined;
let dock: GraphDock | undefined;
let events: EventSource | undefined;
let renderToken = 0;
let refetchInFlight = false;
let refetchQueued = false;
let notifyRibbon: () => void = () => {};

/** The ribbon's state (checked toggles, select values) is read again after `notify`. */
export function bindGraphRibbon(notify: () => void): void {
  notifyRibbon = notify;
  // One store of colour/layout/grouping/focus: the engine follows it, the ribbon re-reads it.
  viewSettings.onChange(() => {
    engine?.applyView(viewSettings.state);
    notifyRibbon();
  });
  // Filters change from the panel, the ribbon and live updates alike.
  filterStore.onChange(() => notifyRibbon());
}

/** The graph mode is the one shown: Ctrl+C copies its selection. */
let graphActive = false;
let copyKeyBound = false;

function bindCopyKey(): void {
  if (copyKeyBound) return;
  copyKeyBound = true;
  // Graph-mode keys: only while the graph is the shown mode, never in a text field (so the
  // diagram editor's own keys are not touched), and not when the list already took the key.
  window.addEventListener("keydown", (e) => {
    if (!graphActive || e.defaultPrevented || e.altKey) return;
    const target = e.target;
    if (target instanceof HTMLElement && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable)) return;
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === "Escape" && !mod) {
      nodeSelection.clear();
    } else if (mod && !e.shiftKey && e.code === "KeyC") {
      if (window.getSelection()?.toString()) return; // text is selected somewhere: an ordinary copy
      e.preventDefault();
      copyGraphNodes("selection");
    } else if (mod && !e.shiftKey && e.code === "KeyA" && engine) {
      e.preventDefault();
      nodeSelection.selectAll(engine.visibleNodeIds());
    } else if (mod && !e.shiftKey && e.code === "KeyI" && engine) {
      e.preventDefault();
      nodeSelection.invert(engine.visibleNodeIds());
    }
  });
}

/**
 * Copy nodes of the graph — the highlighted ones or the chosen ones — for the
 * Схемы editor's paste: JSON text on the system clipboard (with an in-app copy
 * as the fallback), and a toast saying how many, and how many are in the model.
 */
export function copyGraphNodes(which: "selection" | "chosen"): void {
  const ids = which === "selection" ? [...nodeSelection.members] : [...workingSet.members];
  const payload = engine?.clipboardPayload(ids);
  if (!payload || payload.nodes.length === 0) {
    toast(t.graphCopyNothing);
    return;
  }
  const text = encodeGraphClipboard(payload);
  const inModel = payload.nodes.filter((n) => n.entity).length;
  const said = fmt(t.graphCopied, { n: String(payload.nodes.length), m: String(inModel) });
  // The in-app copy is already kept; the system clipboard may refuse (no focus, no permission).
  void navigator.clipboard.writeText(text).then(
    () => toast(said),
    () => toast(said),
  );
}

export async function loadGraph(inner: HTMLElement): Promise<void> {
  graphActive = true;
  bindCopyKey();
  inner.replaceChildren(el("p", { class: "tool-muted", text: t.loading }));
  if (projects.length === 0) {
    const { fetchProjects } = await import("./graph/types.js");
    projects = await fetchProjects().catch(() => []);
  }
  if (!currentProject) currentProject = projects[0]?.id ?? "";
  if (!currentProject) {
    inner.replaceChildren(el("p", { class: "tool-error", text: t.noProjectFile }));
    return;
  }
  if (!dock) {
    const host = el("div", {
      class: "workbench-dockview-host dockview-theme-dark graph-dock-host",
      attrs: { style: "flex: 1; min-width: 0; min-height: 0; width: 100%; position: relative; overflow: hidden;" },
    });
    inner.replaceChildren(host);
    dock = new GraphDock(host);
    dock.onPanelStateChange(() => notifyRibbon());
    // A run has ended: fetch the graph at once (the host's event will find it already current).
    dock.ui.extractors.onRunFinished = () => void onGraphEvent();
    dock.copyHandler = copyGraphNodes;
  }
  notifyRibbon();
  await render();
}

/** Reloads the current project's graph from the host (the ribbon's refresh). */
export async function refreshGraph(): Promise<void> {
  if (!currentProject || !dock) return;
  await render();
}

/** Closes the live subscription; called when another mode is selected. */
export function leaveGraph(): void {
  graphActive = false;
  events?.close();
  events = undefined;
}

/** Back in the mode after `leaveGraph`: follows the host again and catches up. */
export function resumeGraph(): void {
  graphActive = true;
  if (!engine || events) return;
  subscribe();
  void onGraphEvent();
}

// ---------------------------------------------------- what the ribbon reads

export function graphProjectOptions(): { value: string; label: string }[] {
  return projects.map((p) => ({ value: p.id, label: p.title && p.title !== p.id ? `${p.id} — ${p.title}` : p.id }));
}
export const graphProject = (): string => currentProject;
export const graphShowsMissing = (): boolean => showMissing;
export const graphColorBy = (): string => viewSettings.state.colorBy;
export const graphLayout = (): string => viewSettings.state.layout;
export const graphFocus = (): string => viewSettings.state.focus;
export const graphGroupBy = (): string => viewSettings.state.groupBy;
export const isGraphPanelOpen = (id: GraphPanelId): boolean => dock?.panels.isOpen(id) ?? false;

// ---------------------------------------------------- what the ribbon does

export function setGraphProject(id: string): void {
  if (!id || id === currentProject) return;
  currentProject = id;
  filterStore.clear(); // another project: its own kinds, so no old choices
  workingSet.clear(); // ... and its own nodes
  nodeSelection.clear();
  void render();
}

export function toggleGraphMissing(): void {
  showMissing = !showMissing;
  void render();
}

// Colour, layout, grouping and focus live in `viewSettings`; the ribbon and the «Вид» panel
// only set them there, and the engine follows (bindGraphRibbon subscribes it).
export function setGraphColorBy(v: string): void {
  if (COLOR_BYS.includes(v as never)) viewSettings.set({ colorBy: v as never });
}

export function setGraphLayout(v: string): void {
  if (LAYOUTS.includes(v as never)) viewSettings.set({ layout: v as never });
}

export function setGraphFocus(v: string): void {
  if (FOCUS_MODES.includes(v as never)) viewSettings.set({ focus: v as never });
}

export function setGraphGroupBy(v: string): void {
  // Grouping only means something in the «группами» layout: choosing one switches to it.
  if (GROUP_BYS.includes(v as never)) viewSettings.set({ groupBy: v as never, layout: "grouped" });
}

export function restartGraphLayout(): void {
  engine?.restartLayout();
}

export function toggleGraphPanel(id: GraphPanelId): void {
  dock?.panels.toggle(id);
}

export function resetGraphPanels(): void {
  dock?.layout.resetLayout();
}

// ------------------------------------------------------------------ render

async function render(): Promise<void> {
  const token = ++renderToken;
  const d = dock;
  if (!d) return;
  events?.close();
  events = undefined;
  engine?.destroy();
  engine = undefined;
  d.canvasSlot.replaceChildren(el("p", { class: "tool-muted graph-message", text: t.loading }));

  const [{ fetchGraph }, { GraphEngine }] = await Promise.all([import("./graph/types.js"), import("./graph/engine.js")]);

  let data;
  try {
    data = await fetchGraph(currentProject, showMissing);
  } catch (err) {
    if (token === renderToken) d.canvasSlot.replaceChildren(el("p", { class: "tool-error graph-message", text: (err as Error).message }));
    return;
  }
  if (token !== renderToken) return; // a newer render took over

  const eng = new GraphEngine(data, viewSettings.state.layout, d.ui, viewSettings.state.colorBy);
  const mounted = eng.mount();
  engine = eng;
  d.canvasSlot.replaceChildren(mounted);
  eng.start();
  // The containers' nesting and the zones per axis, for «группировать по»; the graph is usable without them.
  void import("./graph/types.js")
    .then(({ fetchGroups }) => fetchGroups(currentProject))
    .then((groups) => {
      if (engine === eng) eng.setGroupData(groups);
    })
    .catch(() => undefined);
  d.ui.extractors.setGraph(data, currentProject);
  notifyRibbon();

  subscribe();
}

function subscribe(): void {
  events?.close();
  events = new EventSource(`/api/events?project=${encodeURIComponent(currentProject)}`);
  events.onmessage = (message) => {
    let payload: { graph?: unknown } | undefined;
    try {
      payload = JSON.parse(message.data) as { graph?: unknown };
    } catch {
      return;
    }
    if (!payload?.graph) return;
    void onGraphEvent();
  };
}

async function onGraphEvent(): Promise<void> {
  if (refetchInFlight) {
    refetchQueued = true;
    return;
  }
  refetchInFlight = true;
  try {
    const { fetchGraph } = await import("./graph/types.js");
    const data = await fetchGraph(currentProject, showMissing);
    if (engine) engine.announceDiff(engine.applyDiff(data));
    dock?.ui.extractors.setGraph(data, currentProject);
  } catch (err) {
    // Transient (e.g. the host briefly restarting the extractor watcher) or
    // a real bug in applyDiff; either way the next event or a manual
    // refresh will catch up, but this is logged so a real bug is visible.
    console.error("graph live update failed", err);
  } finally {
    refetchInFlight = false;
    if (refetchQueued) {
      refetchQueued = false;
      void onGraphEvent();
    }
  }
}
