import { el } from "../util/dom.js";
import { t } from "../shell/strings.js";

/**
 * The Graph mode: sigma + graphology, on `GET /api/graph/{project}`
 * (PLAN_20260928-2 step 2). `sigma`/`graphology` and friends are loaded with
 * a dynamic `import()` on first entry, so they never reach the library
 * bundle of `@semaps/editor` (ADR_20260928-2) — only `npm run build:app`'s
 * chunk for this mode carries them.
 *
 * Live follow (PLAN_20260928-2 step 6): a separate `EventSource` — not
 * `HostModelStore`'s own — subscribes to `/api/events?project=<id>` while the
 * mode is shown for that project. An event with a `graph` field triggers a
 * refetch of the same query and a diff applied to the live graphology graph
 * in place (`GraphEngine.applyDiff`); events without `graph` are ignored.
 * Events arriving while a refetch is in flight coalesce into one more
 * refetch, not one per event.
 */

let projects: { id: string; title?: string }[] = [];
let currentProject = "";
let showMissing = false;
let engine: import("./graph/engine.js").GraphEngine | undefined;
let events: EventSource | undefined;
let refetchInFlight = false;
let refetchQueued = false;

export async function loadGraph(inner: HTMLElement): Promise<void> {
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
  await render(inner);
}

/** Reloads the current project's graph from the host (the ribbon's refresh). */
export async function refreshGraph(inner: HTMLElement): Promise<void> {
  if (!currentProject) return;
  await render(inner);
}

/** Closes the live subscription; called when another mode is selected. */
export function leaveGraph(): void {
  events?.close();
  events = undefined;
}

async function render(inner: HTMLElement): Promise<void> {
  events?.close();
  events = undefined;
  engine?.destroy();
  engine = undefined;
  inner.replaceChildren(el("p", { class: "tool-muted", text: t.loading }));

  const [{ fetchGraph }, { GraphEngine }] = await Promise.all([import("./graph/types.js"), import("./graph/engine.js")]);

  let data;
  try {
    data = await fetchGraph(currentProject, showMissing);
  } catch (err) {
    inner.replaceChildren(el("p", { class: "tool-error", text: (err as Error).message }));
    return;
  }

  const projectSelect = el("select", { class: "graph-project-select" }) as HTMLSelectElement;
  for (const p of projects) projectSelect.appendChild(el("option", { value: p.id, text: p.title && p.title !== p.id ? `${p.id} — ${p.title}` : p.id }));
  projectSelect.value = currentProject;
  projectSelect.addEventListener("change", () => {
    currentProject = projectSelect.value;
    void render(inner);
  });

  const missingCb = el("input", { type: "checkbox" }) as HTMLInputElement;
  missingCb.checked = showMissing;
  missingCb.addEventListener("change", () => {
    showMissing = missingCb.checked;
    void render(inner);
  });

  const eng = new GraphEngine(data);
  const mounted = eng.mount();
  mounted.querySelector(".graph-toolbar")?.prepend(
    el("label", { class: "graph-field" }, [t.graphProject, projectSelect]),
    el("label", { class: "tool-check" }, [missingCb, t.graphShowMissing]),
  );
  engine = eng;
  inner.replaceChildren(mounted);
  eng.start();

  events = new EventSource(`/api/events?project=${encodeURIComponent(currentProject)}`);
  events.onmessage = (message) => {
    let payload: { graph?: unknown } | undefined;
    try {
      payload = JSON.parse(message.data) as { graph?: unknown };
    } catch {
      return;
    }
    if (!payload?.graph) return;
    void onGraphEvent(fetchGraph);
  };
}

async function onGraphEvent(fetchGraph: (project: string, missing?: boolean) => Promise<import("./graph/types.js").GraphResponse>): Promise<void> {
  if (refetchInFlight) {
    refetchQueued = true;
    return;
  }
  refetchInFlight = true;
  try {
    const data = await fetchGraph(currentProject, showMissing);
    if (engine) engine.announceDiff(engine.applyDiff(data));
  } catch (err) {
    // Transient (e.g. the host briefly restarting the extractor watcher) or
    // a real bug in applyDiff; either way the next event or a manual
    // refresh will catch up, but this is logged so a real bug is visible.
    console.error("graph live update failed", err);
  } finally {
    refetchInFlight = false;
    if (refetchQueued) {
      refetchQueued = false;
      void onGraphEvent(fetchGraph);
    }
  }
}
