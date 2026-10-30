import type { IdeTransport } from "./transport.js";

export type ObjectValue = Record<string, unknown>;
export interface IdeMessage extends ObjectValue {
  protocol: "semaps.ide";
  version: 1;
  sessionId: string;
  kind: "request" | "response" | "event";
  id?: string;
  requestId?: string;
  method?: string;
  params?: ObjectValue;
  result?: ObjectValue;
  error?: { code: string; message: string };
}
export const objectValue = (value: unknown): value is ObjectValue =>
  typeof value === "object" && value !== null && !Array.isArray(value);
const nonempty = (value: unknown): value is string => typeof value === "string" && value.length > 0;

export function validMessage(value: unknown): value is IdeMessage {
  if (!objectValue(value) || value.protocol !== "semaps.ide" || value.version !== 1 || !nonempty(value.sessionId)) return false;
  const fields = new Set(["protocol", "version", "sessionId", "kind", "id", "requestId", "method", "params", "result", "error", "origin", "causationId"]);
  if (Object.keys(value).some((key) => !fields.has(key))) return false;
  if (["origin", "causationId"].some((key) => key in value && !nonempty(value[key]))) return false;
  if (value.kind === "response") {
    if (!nonempty(value.requestId) || ["id", "method", "params"].some((key) => key in value)) return false;
    if (("result" in value) === ("error" in value)) return false;
    return "result" in value ? objectValue(value.result) : objectValue(value.error)
      && nonempty(value.error.code) && typeof value.error.message === "string"
      && Object.keys(value.error).every((key) => ["code", "message", "data"].includes(key));
  }
  return (value.kind === "request" || value.kind === "event") && nonempty(value.id)
    && typeof value.method === "string" && /^[a-z][a-zA-Z]*\.[a-z][a-zA-Z]*$/.test(value.method)
    && (!('params' in value) || objectValue(value.params))
    && !["requestId", "result", "error"].some((key) => key in value);
}

export class BridgeError extends Error {
  constructor(readonly code: string, message: string) { super(message); }
}

export interface IdeAdapter {
  readonly capabilities: readonly string[];
  context(): ObjectValue;
  execute(method: string, params: ObjectValue, signal: AbortSignal, requestId: string): Promise<ObjectValue> | ObjectValue;
}

/** Correlation, sessions and bounded replay protection shared by every transport. */
export class IdeBridge {
  private session = "";
  private ready = false;
  private peerCapabilities = new Set<string>();
  private readonly completed = new Map<string, { fingerprint: string; message: IdeMessage }>();
  private readonly running = new Map<string, { fingerprint: string; controller: AbortController }>();
  private readonly outgoing = new Map<string, { resolve: (value: ObjectValue) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>();
  private readonly unsubscribe: () => void;
  private sequence = 0;

  constructor(private readonly transport: IdeTransport, private readonly adapter: IdeAdapter) {
    this.unsubscribe = transport.subscribe((message) => { void this.receive(message); });
  }

  get connected(): boolean { return this.ready; }
  supports(method: string): boolean { return this.connected && this.peerCapabilities.has(method); }

  event(method: string, params: ObjectValue, causationId?: string): void {
    if (!this.connected) return;
    this.transport.send({ protocol: "semaps.ide", version: 1, sessionId: this.session,
      kind: "event", id: `editor-${++this.sequence}`, method, params,
      origin: causationId ? "ide" : "editor", ...(causationId ? { causationId } : {}) });
  }

  request(method: string, params: ObjectValue, timeoutMs = 3000): Promise<ObjectValue> {
    if (!this.supports(method)) return Promise.reject(new BridgeError("unsupported", method));
    const id = `editor-${++this.sequence}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.outgoing.delete(id); reject(new BridgeError("timeout", method)); }, timeoutMs);
      this.outgoing.set(id, { resolve, reject, timer });
      this.transport.send({ protocol: "semaps.ide", version: 1, sessionId: this.session, kind: "request", id, method, params });
    });
  }

  private reset(): void {
    for (const item of this.running.values()) item.controller.abort();
    this.running.clear(); this.completed.clear(); this.peerCapabilities.clear();
    for (const item of this.outgoing.values()) {
      clearTimeout(item.timer); item.reject(new BridgeError("disconnected", "IDE session closed"));
    }
    this.outgoing.clear(); this.session = ""; this.ready = false;
  }

  dispose(): void { this.event("bridge.disconnected", {}); this.reset(); this.unsubscribe(); }

  private async receive(value: unknown): Promise<void> {
    if (!validMessage(value)) return;
    if (value.kind === "response") {
      if (value.sessionId !== this.session) return;
      const pending = this.outgoing.get(value.requestId!);
      if (!pending) return;
      this.outgoing.delete(value.requestId!); clearTimeout(pending.timer);
      if (value.error) pending.reject(new BridgeError(value.error.code, value.error.message));
      else pending.resolve(value.result!);
      return;
    }
    if (value.kind !== "request") return;
    const method = value.method!;
    if (method !== "bridge.initialize" && (value.sessionId !== this.session || !this.ready)) return;
    if (method === "bridge.initialize" && value.sessionId !== this.session) {
      this.reset(); this.session = value.sessionId;
    }
    const id = value.id!;
    const fingerprint = JSON.stringify([method, value.params ?? {}]);
    const known = this.completed.get(id) ?? this.running.get(id);
    if (known) {
      if (known.fingerprint !== fingerprint) this.respond(value, { code: "duplicateId", message: "Request id reused with different parameters" });
      else if ("message" in known) this.transport.send(known.message);
      return;
    }
    const controller = new AbortController();
    this.running.set(id, { fingerprint, controller });
    const session = this.session;
    let response: IdeMessage;
    try {
      const params = value.params ?? {};
      let result: ObjectValue;
      if (method === "bridge.initialize") {
        if (params.capabilities !== undefined && (!Array.isArray(params.capabilities) || params.capabilities.some((x) => typeof x !== "string")))
          throw new BridgeError("invalidParams", "capabilities must be an array of method names");
        this.peerCapabilities = new Set((params.capabilities ?? []) as string[]);
        this.ready = true;
        result = { capabilities: [...this.adapter.capabilities, "bridge.cancel"], context: this.adapter.context() };
      } else if (method === "bridge.cancel") {
        if (!nonempty(params.requestId)) throw new BridgeError("invalidParams", "requestId is required");
        this.running.get(params.requestId)?.controller.abort(); result = { cancelled: true };
      } else {
        if (!this.adapter.capabilities.includes(method)) throw new BridgeError("unsupported", method);
        result = await this.adapter.execute(method, params, controller.signal, id);
      }
      if (controller.signal.aborted) throw new BridgeError("cancelled", "Request cancelled");
      response = { protocol: "semaps.ide", version: 1, sessionId: session, kind: "response", requestId: id, result };
    } catch (error) {
      response = { protocol: "semaps.ide", version: 1, sessionId: session, kind: "response", requestId: id,
        error: { code: error instanceof BridgeError ? error.code : controller.signal.aborted ? "cancelled" : "failed",
          message: error instanceof Error ? error.message : "Bridge operation failed" } };
    }
    if (this.session !== session || this.running.get(id)?.controller !== controller) return;
    this.running.delete(id); this.completed.set(id, { fingerprint, message: response });
    if (this.completed.size > 200) this.completed.delete(this.completed.keys().next().value!);
    this.transport.send(response);
  }

  private respond(request: IdeMessage, error: { code: string; message: string }): void {
    this.transport.send({ protocol: "semaps.ide", version: 1, sessionId: this.session, kind: "response", requestId: request.id, error });
  }
}
