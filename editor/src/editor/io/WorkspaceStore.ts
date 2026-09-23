import { parseTextCatalog, serializeTextCatalog } from "../../model/text-provenance.js";
import type { NewProject, NewView, WorkspaceIndex, WorkspaceStore } from "./types.js";

export const PROJECT_ID = /^[a-z][a-z0-9_]*$/;
export const VIEW_ID = /^v_[a-z0-9_]+$/;

type Json = Record<string, unknown>;

/** Refusal the host gives when a name is taken (`create=1`, `/api/move`). */
export class AlreadyExistsError extends Error {}

async function check(res: Response, what: string): Promise<void> {
  if (res.status === 409) throw new AlreadyExistsError(`«${what}» уже существует`);
  if (!res.ok) throw new Error(`${what}: HTTP ${res.status} ${await res.text().catch(() => "")}`.trim());
}

async function saveJson(file: string, data: unknown, create = false): Promise<void> {
  const res = await fetch(`/api/save?file=${encodeURIComponent(file)}${create ? "&create=1" : ""}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data, null, 2),
  });
  await check(res, file);
}

async function move(from: string, to: string): Promise<void> {
  const res = await fetch(`/api/move?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`, { method: "POST" });
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
  constructor(private readonly baseUrl: string = "./") {}

  private async read(file: string): Promise<Json | null> {
    const res = await fetch(new URL(file, new URL(this.baseUrl, location.href)));
    return res.ok ? ((await res.json()) as Json) : null;
  }

  async load(): Promise<WorkspaceIndex> {
    const res = await fetch("/api/workspace");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as WorkspaceIndex;
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
    await this.rewriteTexts(v.project, [v.language], v.id, v.id, v);
    return file;
  }

  async updateProject(oldId: string, p: NewProject): Promise<void> {
    if (!PROJECT_ID.test(p.id)) throw new Error(`Недопустимый id проекта «${p.id}»`);
    if (p.id !== oldId) await move(`projects/${oldId}`, `projects/${p.id}`);
    const dir = `projects/${p.id}/`;
    const manifest = (await this.read(dir + "project.json")) ?? {};
    const next = withLook({ ...manifest, id: p.id, title: p.title }, p);
    if (p.subtitle) next.subtitle = p.subtitle;
    else delete next.subtitle;
    await saveJson(dir + "project.json", next);
    if (p.id === oldId) return;

    const project = (await this.load()).projects.find((x) => x.id === p.id);
    for (const view of project?.views ?? []) {
      const doc = await this.read(view.file);
      if (doc) await saveJson(view.file, { ...doc, project: p.id });
    }
  }

  async updateView(oldId: string, v: NewView, languages: readonly string[]): Promise<string> {
    if (!VIEW_ID.test(v.id)) throw new Error(`Недопустимый id вида «${v.id}»`);
    const dir = `projects/${v.project}/`;
    const file = `${dir}views/${v.id}.view.json`;
    if (v.id !== oldId) await move(`${dir}views/${oldId}.view.json`, file);
    const doc = (await this.read(file)) ?? {};
    await saveJson(file, withLook({ ...doc, id: v.id, axis: v.axis }, v));
    await this.rewriteTexts(v.project, languages.includes(v.language) ? languages : [...languages, v.language], oldId, v.id, v);

    if (v.id !== oldId) {
      const manifest = await this.read(dir + "project.json");
      if (manifest?.defaultView === oldId) await saveJson(dir + "project.json", { ...manifest, defaultView: v.id });
    }
    return file;
  }

  /**
   * Move a view's texts from `oldId` to `newId` in every language, and set its
   * name in the one being edited. Moved values keep their provenance: a rename
   * does not make a translation authored.
   */
  private async rewriteTexts(project: string, languages: readonly string[], oldId: string, newId: string, v: NewView): Promise<void> {
    for (const lang of languages) {
      const textFile = `projects/${project}/text.${lang}.json`;
      const parsed = parseTextCatalog(await this.read(textFile), lang);
      const entries = { ...parsed.entries };
      let changed = false;
      if (oldId !== newId && entries[oldId]) {
        entries[newId] = entries[oldId]!;
        delete entries[oldId];
        parsed.entries[newId] = parsed.entries[oldId]!;
        if (parsed.provenance[oldId]) parsed.provenance[newId] = parsed.provenance[oldId]!;
        changed = true;
      }
      if (lang === v.language && entries[newId]?.name !== v.name) {
        entries[newId] = { ...entries[newId], name: v.name };
        changed = true;
      }
      if (changed) await saveJson(textFile, serializeTextCatalog(lang, entries, parsed));
    }
  }
}
