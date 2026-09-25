import type { NewProject, NewView, WorkspaceIndex, WorkspaceStore } from "./types.js";
import { hostWriteHeaders } from "../../util/hostKey.js";

export const PROJECT_ID = /^[a-z][a-z0-9_]*$/;
export const VIEW_ID = /^v_[a-z0-9_]+$/;

type Json = Record<string, unknown>;

/** Refusal the host gives when a name is taken (`create=1`, `/api/move`). */
export class AlreadyExistsError extends Error {}

async function check(res: Response, what: string): Promise<void> {
  if (res.status === 409) {
    const detail = await res.text();
    if (detail.includes("сначала сохраните")) throw new Error(detail.trim());
    throw new AlreadyExistsError(`«${what}» уже существует`);
  }
  if (!res.ok) throw new Error(`${what}: HTTP ${res.status} ${await res.text().catch(() => "")}`.trim());
}

async function saveJson(file: string, data: unknown, create = false): Promise<void> {
  const res = await fetch(`/api/save?file=${encodeURIComponent(file)}${create ? "&create=1" : ""}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...hostWriteHeaders() },
    body: JSON.stringify(data, null, 2),
  });
  await check(res, file);
}

async function move(from: string, to: string): Promise<void> {
  const res = await fetch(`/api/move?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`, { method: "POST", headers: hostWriteHeaders() });
  await check(res, to);
}

/** Set or drop optional presentation fields, keeping the rest of the object as written. */
function withLook(doc: Json, look: { icon?: string; theme?: string }): Json {
  const out = { ...doc };
  for (const key of ["icon", "theme"] as const) {
    if (look[key]) out[key] = look[key];
    else delete out[key];
  }
  return out;
}

/**
 * The list of projects and views comes from the host (`GET /api/workspace`),
 * which walks the workspace folder; creating or renaming writes the same files
 * a person would (ADR_20260923-7, ADR_20260923-8).
 */
export class HttpWorkspaceStore implements WorkspaceStore {
  constructor(_baseUrl: string = "./") {}

  private async snapshot(project: string): Promise<{ project: Json; views: Record<string, Json>; texts: Record<string, { entries?: Record<string, Json> }> }> {
    const res = await fetch(`/api/model/${encodeURIComponent(project)}`);
    await check(res, project);
    return await res.json() as { project: Json; views: Record<string, Json>; texts: Record<string, { entries?: Record<string, Json> }> };
  }

  private async ops(project: string, ops: Array<{ kind: string; id: string; lang?: string; view?: string; value: Json }>): Promise<void> {
    if (!ops.length) return;
    const res = await fetch(`/api/model/${encodeURIComponent(project)}/ops`, {
      method: "POST", headers: { "Content-Type": "application/json", ...hostWriteHeaders() },
      body: JSON.stringify({ client: crypto.randomUUID(), ops }),
    });
    await check(res, project);
  }

  async load(): Promise<WorkspaceIndex> {
    const res = await fetch("/api/workspace");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const index = (await res.json()) as WorkspaceIndex;
    const projects = await Promise.all(index.projects.map(async (project) => {
      if (project.error) return project;
      const snapshot = await this.snapshot(project.id);
      const manifest = snapshot.project;
      return {
        ...project,
        title: typeof manifest.title === "string" ? manifest.title : project.title,
        subtitle: typeof manifest.subtitle === "string" ? manifest.subtitle : undefined,
        icon: typeof manifest.icon === "string" ? manifest.icon : undefined,
        theme: typeof manifest.theme === "string" ? manifest.theme : undefined,
        languages: Array.isArray(manifest.languages) ? manifest.languages as string[] : project.languages,
        views: project.views.map((view) => ({
          ...view,
          names: Object.fromEntries(Object.entries(snapshot.texts).flatMap(([lang, doc]) => {
            const name = doc.entries?.[view.id]?.name;
            return typeof name === "string" ? [[lang, name]] : [];
          })),
        })),
      };
    }));
    return { projects };
  }

  async createProject(p: NewProject): Promise<void> {
    if (!PROJECT_ID.test(p.id)) throw new Error(`Недопустимый id проекта «${p.id}»`);
    await saveJson(`projects/${p.id}/project.json`, withLook({
      id: p.id,
      title: p.title,
      ...(p.subtitle ? { subtitle: p.subtitle } : {}),
      contractVersion: 3,
      languages: [p.language],
    }, p), true);
  }

  async createView(v: NewView): Promise<string> {
    if (!VIEW_ID.test(v.id)) throw new Error(`Недопустимый id вида «${v.id}»`);
    const file = `projects/${v.project}/views/${v.id}.view.json`;
    // No `edges` key: a view without one shows the project's relations.
    await saveJson(file, withLook({ id: v.id, project: v.project, axis: v.axis, zones: [], nodes: [] }, v), true);
    await this.setViewName(v.project, v.language, v.id, v.name);
    return file;
  }

  async updateProject(oldId: string, p: NewProject): Promise<void> {
    if (!PROJECT_ID.test(p.id)) throw new Error(`Недопустимый id проекта «${p.id}»`);
    if (p.id !== oldId) await move(`projects/${oldId}`, `projects/${p.id}`);
    const manifest = (await this.snapshot(p.id)).project;
    const next = withLook({ ...manifest, id: p.id, title: p.title }, p);
    if (p.subtitle) next.subtitle = p.subtitle;
    else delete next.subtitle;
    await this.ops(p.id, [{ kind: "project", id: p.id, value: next }]);
  }

  async updateView(oldId: string, v: NewView, _languages: readonly string[]): Promise<string> {
    if (!VIEW_ID.test(v.id)) throw new Error(`Недопустимый id вида «${v.id}»`);
    const dir = `projects/${v.project}/`;
    const file = `${dir}views/${v.id}.view.json`;
    if (v.id !== oldId) await move(`${dir}views/${oldId}.view.json`, file);
    const snapshot = await this.snapshot(v.project);
    const doc = snapshot.views[v.id] ?? {};
    await this.ops(v.project, [{ kind: "view", id: v.id, view: v.id, value: withLook({ ...doc, id: v.id, axis: v.axis }, v) }]);
    await this.setViewName(v.project, v.language, v.id, v.name);
    return file;
  }

  /**
   * Move a view's texts from `oldId` to `newId` in every language, and set its
   * name in the one being edited. Moved values keep their provenance: a rename
   * does not make a translation authored.
   */
  private async setViewName(project: string, lang: string, id: string, name: string): Promise<void> {
    const current = (await this.snapshot(project)).texts[lang]?.entries?.[id] ?? {};
    const oldName = current.name as { v?: string } | undefined;
    if (oldName?.v !== name) await this.ops(project, [{ kind: "text", id, lang, value: {
      ...current, name: { v: name, origin: "authored", at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z") },
    } }]);
  }
}
