import type { DiagramEditor } from "../editor/DiagramEditor.js";
import { placeEntities } from "../editor/placeEntity.js";
import { fileOfRef, lineOfRef, realizationsOf } from "../model/realizations.js";
import type { GraphNode } from "../app/graph/types.js";
import type { IdeTransport } from "./transport.js";
import { BridgeError, IdeBridge, objectValue, type IdeAdapter, type ObjectValue } from "./bridge.js";

function strings(value: unknown, field: string): string[] {
  if (!Array.isArray(value) || value.length > 500 || value.some((id) => typeof id !== "string" || !id))
    throw new BridgeError("invalidParams", `${field} must contain up to 500 nonempty strings`);
  return [...new Set(value as string[])];
}

export function sourceAddress(ref: string): ObjectValue {
  const line = lineOfRef(ref);
  return { file: fileOfRef(ref).replace(/\\/g, "/"), ...(line ? { line } : {}) };
}

/** The only module that translates protocol commands into existing editor operations. */
export class EditorIdeAdapter implements IdeAdapter {
  readonly capabilities = ["context.get", "symbols.resolve", "selection.set", "entities.reveal", "entities.place"];
  private previous: { doc: unknown; projectId: unknown; viewId: unknown; mode: unknown } | undefined;
  private contextId = "";
  private causation: string | undefined;
  bridge: IdeBridge | undefined;

  constructor(readonly editor: DiagramEditor, private readonly mode: () => string) {}

  context(): ObjectValue {
    const view = this.editor.currentView;
    const next = { doc: this.editor.canvas.model, projectId: view ? this.editor.projectOf(view)?.id ?? null : null,
      viewId: view?.id ?? null, mode: this.mode() || "diagram" };
    if (!this.previous || Object.keys(next).some((key) => next[key as keyof typeof next] !== this.previous![key as keyof typeof next])) {
      this.previous = next; this.contextId = crypto.randomUUID();
    }
    return { contextId: this.contextId, projectId: next.projectId, viewId: next.viewId, mode: next.mode };
  }

  changed(): void { this.bridge?.event("context.changed", { context: this.context() }); }

  selectionChanged(): void {
    const context = this.context();
    if (context.mode !== "diagram") return;
    const ids = [...this.editor.canvas.selectedIds];
    const objects = ids.map((entityId) => {
      const entity = this.editor.canvas.model?.entities.find((e) => e.id === entityId);
      return { entityId, sources: realizationsOf(entity).map((r) => ({ language: r.lang, symbolId: r.symbol,
        ...(r.ref ? sourceAddress(r.ref) : {}) })) };
    });
    this.bridge?.event("selection.changed", { context, objects, primaryId: this.editor.canvas.selected?.id ?? null }, this.causation);
  }

  private check(params: ObjectValue, signal: AbortSignal): ObjectValue {
    if (signal.aborted) throw new BridgeError("cancelled", "Request cancelled");
    const context = this.context();
    const requested = params.context;
    if (!objectValue(requested) || ["contextId", "projectId", "viewId", "mode"].some((key) => requested[key] !== context[key]))
      throw new BridgeError("staleContext", "The map context changed; read context.get and retry");
    if (context.mode !== "diagram" || !context.projectId || !context.viewId || !this.editor.canvas.model)
      throw new BridgeError("unsupported", "Open a diagram view first");
    return context;
  }

  async execute(method: string, params: ObjectValue, signal: AbortSignal, requestId: string): Promise<ObjectValue> {
    if (method === "context.get") return { context: this.context(), selection: [...this.editor.canvas.selectedIds] };
    const context = this.check(params, signal);
    if (method === "symbols.resolve") {
      if (!Array.isArray(params.symbols) || params.symbols.length > 500 || params.symbols.some((s) => !objectValue(s)
        || typeof s.language !== "string" || typeof s.symbolId !== "string" || !s.symbolId))
        throw new BridgeError("invalidParams", "symbols must contain language and symbolId");
      const response = await fetch(`/api/graph/${encodeURIComponent(String(context.projectId))}?fields=position`, { signal, cache: "no-store" });
      if (!response.ok) throw new BridgeError("failed", `Graph lookup failed (${response.status})`);
      const graph = await response.json() as { nodes: GraphNode[] };
      this.check(params, signal);
      const doc = this.editor.canvas.model!;
      return { context, results: params.symbols.map((value) => {
        const symbol = value as ObjectValue;
        const nodes = graph.nodes.filter((node) => node.language === symbol.language && node.symbol === symbol.symbolId);
        const entities = doc.entities.filter((entity) => realizationsOf(entity).some((r) => r.lang === symbol.language && r.symbol === symbol.symbolId));
        const candidates = [...new Set([...entities.map((e) => e.id), ...nodes.map((n) => n.entity).filter((id): id is string => !!id)])]
          .filter((id) => doc.entities.some((e) => e.id === id))
          .map((entityId) => ({ entityId, onView: doc.element(entityId) !== undefined }));
        return { symbol, graphNodeIds: nodes.map((node) => node.id), candidates,
          status: candidates.length > 1 ? "ambiguous" : candidates.length === 1 ? candidates[0]!.onView ? "alreadyOnView" : "resolved"
            : nodes.length > 0 ? "notInModel" : "notFound" };
      }) };
    }
    const ids = strings(params.entityIds, "entityIds");
    const doc = this.editor.canvas.model!;
    const missing = ids.filter((id) => !doc.entities.some((entity) => entity.id === id));
    if (missing.length) throw new BridgeError("notInModel", `Unknown entities: ${missing.join(", ")}`);
    this.causation = requestId;
    try {
      if (method === "selection.set") {
        if (ids.some((id) => !doc.element(id))) throw new BridgeError("notOnView", "Selection requires existing placements");
        const primary = params.primaryId;
        if (primary !== undefined && (typeof primary !== "string" || !ids.includes(primary))) throw new BridgeError("invalidParams", "primaryId must be selected");
        this.editor.canvas.selectMany(primary ? [...ids.filter((id) => id !== primary), primary as string] : ids);
        return { selected: ids, context };
      }
      if (method === "entities.reveal") return { context, results: ids.map((entityId) => ({ entityId,
        status: this.editor.canvas.reveal(entityId) ? "revealed" : "notOnView" })) };
      if (method === "entities.place") {
        let at;
        if (params.at !== undefined) {
          if (!objectValue(params.at) || typeof params.at.x !== "number" || !Number.isFinite(params.at.x)
            || typeof params.at.y !== "number" || !Number.isFinite(params.at.y)) throw new BridgeError("invalidParams", "at requires finite x/y");
          const { x, y, space } = params.at;
          if (space === "model") at = { x, y };
          else if (space === "client") {
            const rect = this.editor.canvas.hostElement.getBoundingClientRect();
            if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) throw new BridgeError("invalidParams", "Drop point is outside the canvas");
            at = this.editor.canvas.viewport.toModel({ x: x - rect.left, y: y - rect.top });
          } else throw new BridgeError("invalidParams", "at.space must be model or client");
        }
        this.check(params, signal);
        const placed = placeEntities(this.editor, ids.map((id) => doc.entities.find((entity) => entity.id === id)!), at);
        this.editor.canvas.selectMany(ids);
        return { context, results: ids.map((entityId) => ({ entityId, status: placed.includes(entityId) ? "placed" : "alreadyOnView" })) };
      }
      throw new BridgeError("unsupported", method);
    } finally { this.causation = undefined; }
  }
}

export function attachEditorBridge(editor: DiagramEditor, transport: IdeTransport, mode: () => string,
  subscribeMode?: (changed: () => void) => () => void): () => void {
  const adapter = new EditorIdeAdapter(editor, mode);
  const bridge = new IdeBridge(transport, adapter); adapter.bridge = bridge;
  const off = [editor.canvas.events.on("select", () => adapter.selectionChanged()),
    editor.workspaceEvents.on("change", () => adapter.changed())];
  if (subscribeMode) off.push(subscribeMode(() => adapter.changed()));
  const previous = editor.sourceOpener;
  const opener = async (ref: string, label?: string): Promise<boolean> => {
    if (!bridge.supports("source.open")) return false;
    const context = adapter.context();
    try { return (await bridge.request("source.open", { ...sourceAddress(ref), label, context })).opened === true; }
    catch { return false; }
  };
  editor.sourceOpener = opener;
  return () => { off.forEach((dispose) => dispose()); bridge.dispose(); if (editor.sourceOpener === opener) editor.sourceOpener = previous; };
}
