import type { ExtensionCollection, ExtensionEdit } from "../../model/style-extensions.js";
import type { WireStyleSheet, WireStyle } from "../../model/style-types.js";
import type { StyleStore } from "./types.js";
import type { ChangedRef } from "./HostModelStore.js";
import type { KindsDocument } from "../../model/KindCatalog.js";
import { hostFetch } from "../../util/hostKey.js";

export interface StyleState { sheet: WireStyleSheet; dirty: ChangedRef[]; revision: number; kinds: KindsDocument }
export interface StyleEdit { action: "create" | "update" | "rename" | "delete"; id: string; definition?: WireStyle; patch?: Record<string, unknown>; newId?: string; replacement?: string }

/** Recursive patch: absent fields survive, null removes an own value. */
export function stylePatch(before: Record<string, unknown>, after: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
    const a = before[key], b = after[key];
    if (JSON.stringify(a) === JSON.stringify(b)) continue;
    if (b === undefined) out[key] = null;
    else if (a && b && typeof a === "object" && typeof b === "object" && !Array.isArray(a) && !Array.isArray(b)) out[key] = stylePatch(a as Record<string, unknown>, b as Record<string, unknown>);
    else out[key] = b;
  }
  return out;
}

export class HostStyleStore implements StyleStore {
  state: StyleState | null = null;
  private queued: WireStyleSheet = {};
  private pending: Promise<void> = Promise.resolve();
  constructor(private readonly client: string) {}
  private async request(path: string, init?: RequestInit): Promise<Response> {
    const response = await hostFetch(path, { ...init, headers: { "Content-Type": "application/json" } });
    if (!response.ok) throw new Error(`${path}: HTTP ${response.status}: ${await response.text()}`);
    return response;
  }
  accept(state: StyleState): void { this.state = state; this.queued = structuredClone(state.sheet); }
  async load(): Promise<WireStyleSheet> { await this.pending; this.accept(await (await this.request("/api/styles")).json() as StyleState); return this.state!.sheet; }
  async flush(): Promise<void> { await this.pending; }
  sync(sheet: WireStyleSheet): Promise<void> {
    const before = new Map((this.queued.styles ?? []).map((s) => [s.id, s]));
    const after = new Map((sheet.styles ?? []).map((s) => [s.id, s]));
    const edits: StyleEdit[] = [];
    for (const [id, style] of after) {
      const old = before.get(id);
      if (!old) edits.push({ action: "create", id, definition: style });
      else { const patch = stylePatch(old as unknown as Record<string, unknown>, style as unknown as Record<string, unknown>); if (Object.keys(patch).length) edits.push({ action: "update", id, patch }); }
    }
    for (const id of before.keys()) if (!after.has(id)) edits.push({ action: "delete", id });
    this.queued = structuredClone(sheet);
    return this.enqueue(edits);
  }
  private enqueue(edits: StyleEdit[]): Promise<void> {
    const operation = this.pending.then(async () => {
      for (const edit of edits) {
        const detail = await (await this.request("/api/styles", { method: "POST", body: JSON.stringify({ ...edit, client: this.client, expectedRevision: this.state?.revision }) })).json() as { revision: number; state: StyleState };
        this.state=detail.state;
      }
    });
    this.pending = operation.catch(() => {});
    return operation;
  }
  async edit(edit: StyleEdit): Promise<WireStyleSheet> { await this.enqueue([edit]); return this.load(); }
  async extensionEdit(collection: ExtensionCollection, edit: ExtensionEdit): Promise<void> {
    const operation = this.pending.then(async () => {
      const body = { ...edit, client: this.client, expectedRevision: edit.expectedRevision ?? this.state?.revision };
      const response = await (await this.request(collection === "modifiers" ? "/api/style-modifiers" : "/api/style-rules", { method: "POST", body: JSON.stringify(body) })).json() as { state: StyleState };
      this.accept(response.state);
    });
    this.pending = operation.catch(() => {});
    return operation;
  }
  async save(_sheet: WireStyleSheet): Promise<void> { await this.pending; this.accept(await (await this.request("/api/styles/save", { method: "POST" })).json() as StyleState); }
  async discard(): Promise<WireStyleSheet> { await this.pending; this.accept(await (await this.request("/api/styles/discard", { method: "POST" })).json() as StyleState); return this.state!.sheet; }
}
