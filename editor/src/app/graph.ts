import { el } from "../util/dom.js";
import { t } from "../shell/strings.js";

/**
 * The Graph mode: sigma + graphology, on `GET /api/graph/{project}`
 * (PLAN_20260928-2 step 2). `sigma`/`graphology` and friends are loaded with
 * a dynamic `import()` on first entry, so they never reach the library
 * bundle of `@semaps/editor` (ADR_20260928-2) — only `npm run build:app`'s
 * chunk for this mode carries them.
 */

let projects: { id: string; title?: string }[] = [];
let currentProject = "";
let engine: { destroy(): void } | undefined;

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

async function render(inner: HTMLElement): Promise<void> {
  engine?.destroy();
  engine = undefined;
  inner.replaceChildren(el("p", { class: "tool-muted", text: t.loading }));

  const [{ fetchGraph }, { GraphEngine }] = await Promise.all([import("./graph/types.js"), import("./graph/engine.js")]);

  let data;
  try {
    data = await fetchGraph(currentProject);
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

  const eng = new GraphEngine(data);
  const mounted = eng.mount();
  mounted.querySelector(".graph-toolbar")?.prepend(el("label", { class: "graph-field" }, [t.graphProject, projectSelect]));
  engine = eng;
  inner.replaceChildren(mounted);
  eng.start();
}
