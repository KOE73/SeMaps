import type { GraphEdge } from "./types.js";

/**
 * The working set of the graph mode: the nodes the user picked out of the
 * "universe" (what the pre-filter — symbol kinds, presence, visibility,
 * container — lets through). An empty set means the whole universe is shown;
 * a non-empty one shows only set ∩ universe. It is changed only through the
 * methods below; whoever draws or lists subscribes to `onChange`.
 */
export class WorkingSet {
  private ids = new Set<string>();
  private readonly listeners = new Set<() => void>();

  get size(): number {
    return this.ids.size;
  }

  has(id: string): boolean {
    return this.ids.has(id);
  }

  get members(): ReadonlySet<string> {
    return this.ids;
  }

  add(ids: Iterable<string>): void {
    let changed = false;
    for (const id of ids) {
      if (!this.ids.has(id)) {
        this.ids.add(id);
        changed = true;
      }
    }
    if (changed) this.emit();
  }

  remove(ids: Iterable<string>): void {
    let changed = false;
    for (const id of ids) changed = this.ids.delete(id) || changed;
    if (changed) this.emit();
  }

  toggle(id: string): void {
    if (this.ids.has(id)) this.ids.delete(id);
    else this.ids.add(id);
    this.emit();
  }

  /** Replaces the whole set. */
  setTo(ids: Iterable<string>): void {
    this.ids = new Set(ids);
    this.emit();
  }

  clear(): void {
    if (this.ids.size === 0) return;
    this.ids = new Set();
    this.emit();
  }

  /** Drops members that no longer exist (a live update removed the node). */
  prune(existing: ReadonlySet<string>): void {
    let changed = false;
    for (const id of [...this.ids]) {
      if (!existing.has(id)) {
        this.ids.delete(id);
        changed = true;
      }
    }
    if (changed) this.emit();
  }

  onChange(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const l of [...this.listeners]) l();
  }
}

export const workingSet = new WorkingSet();

/**
 * The list selection («Выделение», not «Выбранные»): the nodes the user has
 * highlighted, in the «Узлы» list and on the canvas. Sticky: a search, a
 * change of the working set or a re-layout leave it alone; a plain click
 * (`setOnly`) replaces it and only an explicit `clear` empties it. `primary`
 * is the node last clicked — what the Properties panel shows.
 */
export class NodeSelection {
  private ids = new Set<string>();
  private lead: string | undefined;
  private readonly listeners = new Set<() => void>();

  get size(): number {
    return this.ids.size;
  }

  get members(): ReadonlySet<string> {
    return this.ids;
  }

  get primary(): string | undefined {
    return this.lead;
  }

  has(id: string): boolean {
    return this.ids.has(id);
  }

  /** A plain click: this node alone. */
  setOnly(id: string): void {
    this.ids = new Set([id]);
    this.lead = id;
    this.emit();
  }

  /** Ctrl+click. */
  toggle(id: string): void {
    if (this.ids.delete(id)) {
      if (this.lead === id) this.lead = [...this.ids].pop();
    } else {
      this.ids.add(id);
      this.lead = id;
    }
    this.emit();
  }

  /** Adds nodes (a range, the result of a traversal); the lead stays unless there is none. */
  add(ids: Iterable<string>): void {
    let changed = false;
    for (const id of ids) {
      if (!this.ids.has(id)) {
        this.ids.add(id);
        this.lead ??= id;
        changed = true;
      }
    }
    if (changed) this.emit();
  }

  clear(): void {
    if (this.ids.size === 0) return;
    this.ids = new Set();
    this.lead = undefined;
    this.emit();
  }

  /** Ctrl+A: exactly these nodes (the ones visible / listed now). */
  selectAll(ids: Iterable<string>): void {
    this.ids = new Set(ids);
    this.lead = [...this.ids].pop();
    this.emit();
  }

  /** Ctrl+I: within `scope` (the visible / listed nodes) what was selected is not, and the other way round. */
  invert(scope: Iterable<string>): void {
    this.ids = new Set([...scope].filter((id) => !this.ids.has(id)));
    this.lead = [...this.ids].pop();
    this.emit();
  }

  /** Drops members that no longer exist (a live update removed the node). */
  prune(existing: ReadonlySet<string>): void {
    let changed = false;
    for (const id of [...this.ids]) {
      if (!existing.has(id)) {
        this.ids.delete(id);
        changed = true;
      }
    }
    if (this.lead && !this.ids.has(this.lead)) this.lead = [...this.ids].pop();
    if (changed) this.emit();
  }

  onChange(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const l of [...this.listeners]) l();
  }
}

export const nodeSelection = new NodeSelection();

/** Edge kinds that form the inheritance hierarchy; an edge runs child -> base. */
export const HIERARCHY_KINDS: readonly string[] = ["implements", "extends"];

export type Reach = "descendants" | "ancestors" | "neighbours";

/**
 * Nodes reached from `start` along edges whose kind is in `kinds` (all kinds
 * when null), walking only through nodes `inUniverse` accepts — a hidden node
 * breaks the chain. Edges run from -> to and, for `extends`/`implements`, from
 * is the child: descendants follow edges against their direction (incoming),
 * ancestors follow them (outgoing), neighbours both, one level only. `start`
 * itself is not part of the result.
 */
export function reachFrom(
  start: string,
  reach: Reach,
  kinds: readonly string[] | null,
  allLevels: boolean,
  edges: readonly GraphEdge[],
  inUniverse: (id: string) => boolean,
): Set<string> {
  const out = new Map<string, string[]>();
  const inc = new Map<string, string[]>();
  for (const e of edges) {
    if (kinds && !kinds.includes(e.kind)) continue;
    (out.get(e.from) ?? out.set(e.from, []).get(e.from)!).push(e.to);
    (inc.get(e.to) ?? inc.set(e.to, []).get(e.to)!).push(e.from);
  }
  const step = (id: string): string[] =>
    reach === "descendants" ? inc.get(id) ?? [] : reach === "ancestors" ? out.get(id) ?? [] : [...(inc.get(id) ?? []), ...(out.get(id) ?? [])];

  const found = new Set<string>();
  const seen = new Set<string>([start]);
  let frontier = [start];
  while (frontier.length > 0) {
    const next: string[] = [];
    for (const id of frontier) {
      for (const n of step(id)) {
        if (seen.has(n) || !inUniverse(n)) continue;
        seen.add(n);
        found.add(n);
        next.push(n);
      }
    }
    if (!allLevels || reach === "neighbours") break;
    frontier = next;
  }
  return found;
}
