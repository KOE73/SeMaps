import type { WireDocument, EntityCatalog, RelationCatalog, ProjectManifest, ViewDocument } from "../../model/wire-types.js";
import type { SaveTarget } from "./types.js";
import { HttpProjectStore } from "./ProjectStore.js";
import { diffModel, type ModelOp } from "./ModelSync.js";
import { entityDisplayName, type NamedEntity } from "../../model/entityName.js";
import { KindCatalog } from "../../model/KindCatalog.js";

export interface ChangedRef { kind: string; id: string; view?: string; lang?: string; author: string }
export interface DirtySummary { registry: ChangedRef[]; views: Record<string, ChangedRef[]> }
/** A request from the host for a picture of a view; answered with `POST /api/render/{id}`. */
export interface RenderRequest {
  id: string;
  view: string;
  ref?: string;
  rect?: { x: number; y: number; width: number; height: number } | null;
  scale?: number;
  maxSize?: number;
}
export interface ModelEvent { client: string; author: string; changed: ChangedRef[]; dirty: DirtySummary; render?: RenderRequest; projectReloaded?: { oldProject: string; newProject: string; oldView?: string; newView?: string } }

interface Snapshot {
  project: ProjectManifest;
  registry: Record<string, unknown>;
  texts: Record<string, { entries?: Record<string, Record<string, unknown>> }>;
  views: Record<string, unknown>;
  viewFiles: Record<string, string>;
  dirty: DirtySummary;
  /** Ids of the records in the working registry that are not in the saved one (docs/API.md §3.4). */
  unsaved?: { entity?: string[]; relation?: string[] };
  /** What this editor keeps beside the host's snapshot; not on the wire. */
  local?: LocalRecords;
}

/**
 * `unsaved`: the withdrawable records, kept current by our own ops; `known`: the ids the registry
 * had when the snapshot was read (a record outside it, sent now, is a creation); `mine`: the entities
 * this editor created — only those are withdrawn when their block leaves the view, so a block
 * of an agent's creation that a human takes off a view is not silently erased.
 */
interface LocalRecords {
  unsaved: { entity: Set<string>; relation: Set<string> };
  known: { entity: Set<string>; relation: Set<string> };
  mine: Set<string>;
}

const REGISTRY_FILES = { entity: ["entities.json", "entities"], relation: ["relations.json", "relations"] } as const;

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

export class HostModelStore extends HttpProjectStore {
  private readonly client = crypto.randomUUID();
  private readonly key = document.querySelector<HTMLMetaElement>('meta[name="semaps-key"]')?.content ?? "";
  private readonly snapshots = new Map<string, Snapshot>();
  private readonly views = new Map<string, ViewDocument>();
  private readonly baselines = new Map<string, WireDocument>();
  private pending: Promise<void> = Promise.resolve();
  private events: EventSource | null = null;
  private eventProject = "";

  private projectOf(file: string): string {
    const match = /^projects\/([^/]+)\/views\/[^/]+\.view\.json$/.exec(file);
    if (!match) throw new Error(`Некорректный путь вида: ${file}`);
    return match[1]!;
  }

  private async request(path: string, init?: RequestInit): Promise<Response> {
    const response = await fetch(path, { ...init, headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(init?.method && init.method !== "GET" ? { Authorization: `Bearer ${this.key}` } : {}),
      ...init?.headers,
    } });
    if (!response.ok) throw new Error(`${path}: HTTP ${response.status}: ${await response.text()}`);
    return response;
  }

  override async load(file: string): Promise<WireDocument> {
    const project = this.projectOf(file);
    let snapshot = this.snapshots.get(project);
    if (!snapshot) {
      snapshot = await (await this.request(`/api/model/${encodeURIComponent(project)}`)).json() as Snapshot;
      const idsOf = (kind: "entity" | "relation"): Set<string> => {
        const [name, key] = REGISTRY_FILES[kind];
        const list = (snapshot!.registry[name] as Record<string, Array<{ id?: unknown }>> | undefined)?.[key] ?? [];
        return new Set(list.map((r) => String(r.id)));
      };
      snapshot.local = {
        unsaved: { entity: new Set(snapshot.unsaved?.entity ?? []), relation: new Set(snapshot.unsaved?.relation ?? []) },
        known: { entity: idsOf("entity"), relation: idsOf("relation") },
        mine: new Set(),
      };
      this.snapshots.set(project, snapshot);
    }
    const viewId = Object.entries(snapshot.viewFiles ?? {}).find(([, path]) => path === file)?.[0]
      ?? file.split("/").at(-1)!.replace(/\.view\.json$/, "");
    const view = await (await this.request(`/api/model/${encodeURIComponent(project)}/views/${encodeURIComponent(viewId)}`)).json() as ViewDocument;
    this.views.set(file, clone(view));
    const registry = snapshot.registry;
    const wire = await this.loadProjectBundle(file, view, {
      project: snapshot.project,
      entities: registry["entities.json"] as EntityCatalog ?? { entities: [] },
      relations: registry["relations.json"] as RelationCatalog ?? { relations: [] },
      texts: snapshot.texts,
    });
    this.baselines.set(file, clone(wire));
    return wire;
  }

  confirmLoaded(file: string, wire: WireDocument): void {
    this.baselines.set(file, clone(wire));
  }

  async sync(file: string, wire: WireDocument): Promise<void> {
    const copy = clone(wire);
    this.pending = this.pending.catch(() => {}).then(async () => {
      const project = this.projectOf(file);
      const before = this.baselines.get(file);
      const view = this.views.get(file);
      const snapshot = this.snapshots.get(project);
      if (!before || !view || !snapshot) return;
      const entities = (snapshot.registry["entities.json"] as EntityCatalog)?.entities ?? [];
      const relations = (snapshot.registry["relations.json"] as RelationCatalog)?.relations ?? [];
      const local = snapshot.local!;
      const withdrawable = { entity: new Set([...local.unsaved.entity].filter((id) => local.mine.has(id))), relation: local.unsaved.relation };
      let ops = diffModel(before, copy, view, entities, snapshot.texts, relations, withdrawable);
      if (!ops.length) return;
      const post = (list: ModelOp[]): Promise<Response> => this.request(`/api/model/${encodeURIComponent(project)}/ops`, {
        method: "POST", body: JSON.stringify({ client: this.client, ops: list }),
      });
      let response: Response;
      try {
        response = await post(ops);
      } catch (e) {
        // A withdrawal the host refuses (the record is named somewhere this editor cannot see, say
        // on another view) falls back to what a removal always was: hide the line, take the block off the view.
        if (!ops.some((op) => op.value === null && (op.kind === "entity" || op.kind === "relation")) || !/HTTP 422/.test((e as Error).message)) throw e;
        ops = diffModel(before, copy, view, entities, snapshot.texts, relations);
        if (!ops.length) return;
        response = await post(ops);
      }
      const result = await response.json() as { dirty: DirtySummary };
      snapshot.dirty = result.dirty;
      this.applyLocal(file, ops);
      this.baselines.set(file, copy);
    });
    return this.pending;
  }

  private applyLocal(file: string, ops: ModelOp[]): void {
    const snapshot = this.snapshots.get(this.projectOf(file))!;
    const view = this.views.get(file)!;
    for (const op of ops) {
      if (op.kind === "placement") {
        const list = (view.placements ?? []) as unknown as Array<Record<string, unknown>>;
        const index = list.findIndex((item) => item.entity === op.id);
        if (op.value === null) { if (index >= 0) list.splice(index, 1); }
        else if (index >= 0) list[index] = op.value;
        else list.push(op.value);
        view.placements = list as unknown as ViewDocument["placements"];
      } else if (op.kind === "view" && op.value) {
        // `null` removes the key, as the host applies it (docs/API.md).
        for (const [key, value] of Object.entries(op.value)) {
          if (value === null) delete (view as Record<string, unknown>)[key];
          else (view as Record<string, unknown>)[key] = value;
        }
      } else if (op.kind === "text" && op.value) {
        const doc = snapshot.texts[op.lang!] ?? { entries: {} };
        (doc.entries ??= {})[op.id] = op.value;
        snapshot.texts[op.lang!] = doc;
      } else if (op.kind === "entity" || op.kind === "relation") {
        const [fileName, key] = REGISTRY_FILES[op.kind];
        const local = snapshot.local!;
        const doc = (snapshot.registry[fileName] ??= {}) as Record<string, Array<Record<string, unknown>>>;
        const list = doc[key] ??= [];
        const index = list.findIndex((item) => item.id === op.id);
        if (op.value === null) {
          // A withdrawn creation is gone as if never made: from the registry, from the
          // unsaved set, and with it every text under its id (the host drops them itself).
          if (index >= 0) list.splice(index, 1);
          local.unsaved[op.kind].delete(op.id);
          local.mine.delete(op.id);
          for (const doc of Object.values(snapshot.texts)) if (doc.entries) delete doc.entries[op.id];
        } else {
          if (!local.known[op.kind].has(op.id) && !local.unsaved[op.kind].has(op.id)) {
            local.unsaved[op.kind].add(op.id);
            if (op.kind === "entity") local.mine.add(op.id);
          }
          if (index >= 0) list[index] = op.value;
          else list.push(op.value);
        }
      }
    }
  }

  /** A human name for a changed object, from the working snapshot; its id when there is none. */
  describe(project: string, ref: ChangedRef, lang: string): string {
    const snapshot = this.snapshots.get(project);
    const text = (id: string): string | undefined => {
      const name = snapshot?.texts[lang]?.entries?.[id]?.name as { v?: string } | string | undefined;
      return typeof name === "string" ? name : name?.v;
    };
    const record = (file: string, key: string, id: string): Record<string, unknown> | undefined =>
      ((snapshot?.registry[file] as Record<string, Array<Record<string, unknown>>> | undefined)?.[key] ?? [])
        .find((r) => r.id === id);
    // An entity's name: the registry's for one from code, its text `name` for an authored one (entityDisplayName).
    const entity = (id: string): string => entityDisplayName(
      record("entities.json", "entities", id) as NamedEntity | undefined, id, snapshot?.texts, lang, snapshot?.project.languages ?? []);
    const relation = (id: string): string => {
      const r = record("relations.json", "relations", id);
      if (!r) return id;
      const type = String(r.type ?? r.relation ?? "");
      // A relation type is named by the dictionary, its id when it is not there.
      return `${entity(String(r.from))} → ${entity(String(r.to))} · ${KindCatalog.active.relationName(type, lang)}`;
    };
    const id = ref.id;
    if (ref.kind === "project") return snapshot?.project.title ?? id;
    if (id.startsWith("r_")) return relation(id);
    // A text under an entity's id is a change of that entity: its name resolved, whichever language it is in.
    if (id.startsWith("e_") || ref.kind === "entity" || ref.kind === "placement" || record("entities.json", "entities", id)) return entity(id);
    return text(id) ?? id;
  }

  async dirty(project: string): Promise<DirtySummary> {
    await this.pending;
    return (await (await this.request(`/api/model/${encodeURIComponent(project)}/save`)).json()) as DirtySummary;
  }

  override async save(target: SaveTarget, wire: WireDocument): Promise<void> {
    await this.sync(target.file, wire);
    const project = this.projectOf(target.file);
    await this.saveProject(project);
  }

  async saveProject(project: string): Promise<void> {
    await this.pending;
    await this.request(`/api/model/${encodeURIComponent(project)}/save`, { method: "POST" });
    this.invalidate(project);
  }

  async discard(project: string, scope: "view" | "registry" | "all", id?: string): Promise<void> {
    await this.pending;
    await this.request(`/api/model/${encodeURIComponent(project)}/discard`, {
      method: "POST", body: JSON.stringify({ scope, id }),
    });
    this.invalidate(project);
  }

  /**
   * Remove authored records from the registry (ADR_20261001). The host builds the whole batch —
   * placements and entries on every view, the entity's own relations when `cascade` — with the
   * rule the MCP tool `remove` uses; this editor sends ids only. The local snapshot is dropped:
   * the caller reloads the view.
   */
  async removeRecords(project: string, ids: string[], cascade: boolean): Promise<void> {
    await this.pending;
    await this.request(`/api/model/${encodeURIComponent(project)}/remove`, {
      method: "POST", body: JSON.stringify({ client: this.client, ids, cascade }),
    });
    this.invalidate(project);
  }

  invalidate(project: string): void {
    this.snapshots.delete(project);
    for (const file of this.views.keys()) if (this.projectOf(file) === project) {
      this.views.delete(file); this.baselines.delete(file);
    }
  }

  /** Who draws a view when the host asks for a picture of it (set by the app once the editor exists). */
  renderer: ((request: RenderRequest) => Promise<unknown>) | null = null;

  private async answerRender(request: RenderRequest): Promise<void> {
    let body: unknown;
    try {
      body = this.renderer ? await this.renderer(request) : { error: "this editor cannot render" };
    } catch (e) {
      body = { error: (e as Error).message };
    }
    try {
      await this.request(`/api/render/${encodeURIComponent(request.id)}`, { method: "POST", body: JSON.stringify(body) });
    } catch { /* the host stopped waiting */ }
  }

  subscribe(project: string, onEvent: (event: ModelEvent) => void): void {
    if (this.eventProject === project) return;
    this.events?.close(); this.eventProject = project;
    this.events = new EventSource(`/api/events?project=${encodeURIComponent(project)}&render=1`);
    this.events.onmessage = (message) => {
      const event = JSON.parse(message.data) as ModelEvent;
      // A render request is not a model change: answer it, do not reload anything.
      if (event.render) { void this.answerRender(event.render); return; }
      if (event.client === this.client) return;
      const deliver=()=>{this.invalidate(project);onEvent(event)};
      void this.pending.then(deliver,deliver);
    };
  }
}
