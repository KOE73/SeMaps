import type { WireDocument, WireNode, WireZone, ViewDocument } from "../../model/wire-types.js";

export interface ModelOp {
  kind: "entity" | "relation" | "relationType" | "text" | "view" | "zone" | "node";
  id: string;
  view?: string;
  lang?: string;
  value: Record<string, unknown> | null;
}

const same = (a: unknown, b: unknown): boolean => JSON.stringify(a) === JSON.stringify(b);
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const fields = ["name", "title", "description", "doc", "fromLabel", "toLabel"] as const;

function changedPlacement(
  kind: "node" | "zone", id: string, before: WireNode | WireZone | undefined,
  after: WireNode | WireZone | undefined, original: Record<string, unknown> | undefined,
  view: string,
): ModelOp | undefined {
  if (!after) return { kind, id, view, value: null };
  const geometryChanged = !before || ["x", "y", "width", "height", "styleId"].some((field) =>
    !same((before as unknown as Record<string, unknown>)[field], (after as unknown as Record<string, unknown>)[field]))
    || (kind === "node" && (!same((before as WireNode | undefined)?.zone, (after as WireNode).zone)
      || !same(before?.metadata?.template, after.metadata?.template)));
  if (!geometryChanged) return undefined;
  const value: Record<string, unknown> = { ...(original ?? {}) };
  if (!original) {
    if (kind === "node") value.entity = id;
    else value.id = id;
  }
  for (const field of ["x", "y", "width", "height", "styleId"] as const) {
    if (!before || !same(before[field], after[field])) {
      if (after[field] === undefined) delete value[field];
      else value[field] = after[field];
    }
  }
  if (kind === "node" && (!before || !same((before as WireNode).zone, (after as WireNode).zone))) {
    const key = "container" in value ? "container" : "zone";
    value[key] = (after as WireNode).zone ?? null;
  }
  if (kind === "node") {
    const template = after.metadata?.template;
    if (!before || !same(before.metadata?.template, template)) {
      if (typeof template === "string") value.template = template;
      else delete value.template;
    }
  }
  return { kind, id, view, value };
}

/** Compare completed editor actions by object; carry untouched raw keys through. */
export function diffModel(
  before: WireDocument, after: WireDocument, originalView: ViewDocument,
  rawEntities: Array<Record<string, unknown>>, rawTexts: Record<string, { entries?: Record<string, Record<string, unknown>> }>,
): ModelOp[] {
  const view = after.bundle?.view.id ?? originalView.id;
  const ops: ModelOp[] = [];
  const originalNodes = new Map((originalView.nodes ?? originalView.placements ?? []).map((n) => [n.entity ?? n.id ?? "", n as unknown as Record<string, unknown>]));
  const originalZones = new Map((originalView.zones ?? []).map((z) => [z.id, z as unknown as Record<string, unknown>]));
  for (const kind of ["zone", "node"] as const) {
    const previous = new Map((kind === "zone" ? before.zones ?? [] : before.nodes ?? []).map((n) => [n.id, n]));
    const current = new Map((kind === "zone" ? after.zones ?? [] : after.nodes ?? []).map((n) => [n.id, n]));
    for (const id of new Set([...previous.keys(), ...current.keys()])) {
      const op = changedPlacement(kind, id, previous.get(id), current.get(id),
        (kind === "zone" ? originalZones : originalNodes).get(id), view);
      if (op) ops.push(op);
    }
  }
  const originalEntityById = new Map(rawEntities.map((e) => [String(e.id), e]));
  const beforeNodes = new Map((before.nodes ?? []).map((n) => [n.id, n]));
  const primaryLang=after.bundle?.project.languages?.[0] ?? "ru";
  for (const node of after.nodes ?? []) {
    const old = beforeNodes.get(node.id);
    const entity = originalEntityById.get(node.id);
    if (!entity) {
      ops.push({ kind: "entity", id: node.id, value: { id: node.id, name: node.label ?? node.id,
        kind: node.type ?? "Component", origin: "authored", status: "present" } });
    } else if (old && (old.type !== node.type ||
        (old.label !== node.label && !rawTexts[primaryLang]?.entries?.[node.id]?.name))) {
      ops.push({ kind: "entity", id: node.id, value: { ...entity,
        ...(old.label !== node.label && !rawTexts[primaryLang]?.entries?.[node.id]?.name ? { name: node.label ?? node.id } : {}),
        ...(old.type !== node.type ? { kind: node.type ?? entity.kind } : {}) } });
    }
  }
  const beforeRegistries = before.bundle?.textRegistries ?? {};
  const afterRegistries = after.bundle?.textRegistries ?? {};
  for (const [lang, registry] of Object.entries(afterRegistries)) {
    for (const [id, entry] of Object.entries(registry.entries ?? {})) {
      const prior = beforeRegistries[lang]?.entries?.[id] ?? {};
      const changed = fields.filter((field) => entry[field] !== prior[field] &&
        !(id.startsWith("e_") && (field === "name" || field === "title") && !rawTexts[lang]?.entries?.[id]?.[field]));
      if (!changed.length) continue;
      const value = copy(rawTexts[lang]?.entries?.[id] ?? {});
      for (const field of changed) {
        const text = entry[field];
        if (typeof text !== "string" || !text.trim()) throw new Error(`Пустой текст ${id}.${field} нельзя сохранить`);
        value[field] = { v: text, at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"), origin: "authored" };
      }
      ops.push({ kind: "text", id, lang, value });
    }
  }
  const oldRouting = before.metadata?.routing;
  const newRouting = after.metadata?.routing;
  if (!same(before.edges, after.edges) || oldRouting !== newRouting) {
    const value: Record<string, unknown> = { id: view };
    if (oldRouting !== newRouting) value.routing = newRouting ?? null;
    if (!same(before.edges, after.edges)) value.edges = (after.edges ?? []).map((e) => ({
      id: e.id, from: e.from, to: e.to, type: e.type, ...e.styleId ? { styleId: e.styleId } : {},
      ...e.routing ? { routing: e.routing } : {},
    }));
    ops.push({ kind: "view", id: view, view, value });
  }
  return [
    ...ops.filter((op) => op.kind === "entity"),
    ...ops.filter((op) => op.kind === "zone"),
    ...ops.filter((op) => op.kind !== "entity" && op.kind !== "zone"),
  ];
}
