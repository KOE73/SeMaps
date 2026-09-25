import type { WireDocument, EntityCatalog, RelationCatalog, RelationTypeCatalog, ProjectManifest, ViewDocument } from "../../model/wire-types.js";
import type { SaveTarget } from "./types.js";
import { HttpProjectStore } from "./ProjectStore.js";
import { diffModel, type ModelOp } from "./ModelSync.js";

export interface ChangedRef { kind: string; id: string; view?: string; lang?: string; author: string }
export interface DirtySummary { registry: ChangedRef[]; views: Record<string, ChangedRef[]> }
export interface ModelEvent { client: string; author: string; changed: ChangedRef[]; dirty: DirtySummary; projectReloaded?: { oldProject: string; newProject: string; oldView?: string; newView?: string } }

interface Snapshot {
  project: ProjectManifest;
  registry: Record<string, unknown>;
  texts: Record<string, { entries?: Record<string, Record<string, unknown>> }>;
  views: Record<string, unknown>;
  viewFiles: Record<string, string>;
  dirty: DirtySummary;
}

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
      relationTypes: registry["relation-types.json"] as RelationTypeCatalog ?? { relationTypes: [] },
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
      const ops = diffModel(before, copy, view, entities, snapshot.texts);
      if (!ops.length) return;
      const response = await this.request(`/api/model/${encodeURIComponent(project)}/ops`, {
        method: "POST", body: JSON.stringify({ client: this.client, ops }),
      });
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
      if (op.kind === "node" || op.kind === "zone") {
        const key = op.kind === "zone" ? "zones" : (view.placements ? "placements" : "nodes");
        const list = (view[key] ?? []) as unknown as Array<Record<string, unknown>>;
        const index = list.findIndex((item) => item.id === op.id || item.entity === op.id);
        if (op.value === null) { if (index >= 0) list.splice(index, 1); }
        else if (index >= 0) list[index] = op.value;
        else list.push(op.value);
        (view as unknown as Record<string, unknown>)[key] = list;
      } else if (op.kind === "view" && op.value) {
        Object.assign(view, op.value);
      } else if (op.kind === "text" && op.value) {
        const doc = snapshot.texts[op.lang!] ?? { entries: {} };
        (doc.entries ??= {})[op.id] = op.value;
        snapshot.texts[op.lang!] = doc;
      } else if (op.value && ["entity", "relation", "relationType"].includes(op.kind)) {
        const [fileName, key] = op.kind === "entity" ? ["entities.json", "entities"] :
          op.kind === "relation" ? ["relations.json", "relations"] : ["relation-types.json", "relationTypes"];
        const doc = snapshot.registry[fileName] as Record<string, Array<Record<string, unknown>>>;
        const list = doc[key] ??= [];
        const index = list.findIndex((item) => item.id === op.id);
        if (index >= 0) list[index] = op.value;
        else list.push(op.value);
      }
    }
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

  invalidate(project: string): void {
    this.snapshots.delete(project);
    for (const file of this.views.keys()) if (this.projectOf(file) === project) {
      this.views.delete(file); this.baselines.delete(file);
    }
  }

  subscribe(project: string, onEvent: (event: ModelEvent) => void): void {
    if (this.eventProject === project) return;
    this.events?.close(); this.eventProject = project;
    this.events = new EventSource(`/api/events?project=${encodeURIComponent(project)}`);
    this.events.onmessage = (message) => {
      const event = JSON.parse(message.data) as ModelEvent;
      if (event.client === this.client) return;
      const deliver=()=>{this.invalidate(project);onEvent(event)};
      void this.pending.then(deliver,deliver);
    };
  }
}
