import type { WireDocument, WirePlacement, ViewDocument } from "../../model/wire-types.js";
import { EDGE_OVERRIDE_FIELDS, OVERRIDE_FIELDS, serializeOverride } from "../../model/override.js";
import { nameIsText, type NamedEntity } from "../../model/entityName.js";

/**
 * One operation of the host's working model (docs/API.md). A placement is
 * named by its entity: `{kind: "placement", view, id: <entity id>, value}`,
 * `value` the whole placement or null to take it off the view.
 */
export interface ModelOp {
  kind: "entity" | "relation" | "text" | "view" | "placement";
  id: string;
  view?: string;
  lang?: string;
  value: Record<string, unknown> | null;
}

const same = (a: unknown, b: unknown): boolean => JSON.stringify(a) === JSON.stringify(b);
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const fields = ["name", "title", "description", "doc", "fromLabel", "toLabel"] as const;

/** A placement's fields as the view file names them, from the editor's placement. */
function fileFields(p: WirePlacement): Record<string, unknown> {
  const template = p.metadata?.template;
  return {
    parent: p.parent ?? null,
    x: p.x,
    y: p.y,
    width: p.width,
    height: p.height,
    styleId: p.styleId,
    override: serializeOverride(OVERRIDE_FIELDS, p.override),
    template: typeof template === "string" ? template : undefined,
    collapsed: p.collapsed,
  };
}

function changedPlacement(
  id: string, before: WirePlacement | undefined, after: WirePlacement | undefined,
  original: Record<string, unknown> | undefined, view: string,
): ModelOp | undefined {
  if (!after) return before ? { kind: "placement", id, view, value: null } : undefined;
  const now = fileFields(after);
  const was = before ? fileFields(before) : undefined;
  const changed = Object.keys(now).filter((key) => !was || !same(was[key], now[key]));
  if (changed.length === 0) return undefined;
  // Keys the editor does not model ride along from the file; a new placement
  // is written in the contract's field order.
  const value: Record<string, unknown> = original ? { ...original } : { entity: id };
  for (const key of Object.keys(now)) {
    if (original && !changed.includes(key)) continue;
    if (now[key] === undefined) delete value[key];
    else value[key] = now[key];
  }
  // Every written placement names its parent, null included (CONTRACT.md §8.2);
  // a file that left the key out gets it on the first write.
  if (!("parent" in value)) value.parent = now.parent;
  return { kind: "placement", id, view, value };
}

/** An edge entry of the view's own `edges` list, in the contract's field order (§8.5). */
function edgeEntry(e: NonNullable<WireDocument["edges"]>[number]): Record<string, unknown> {
  const override = serializeOverride(EDGE_OVERRIDE_FIELDS, e.override);
  return {
    id: e.id, from: e.from, to: e.to, type: e.type,
    ...(e.styleId ? { styleId: e.styleId } : {}),
    ...(override ? { override } : {}),
    ...(e.routing ? { routing: e.routing } : {}),
  };
}

/** Compare completed editor actions by object; carry untouched raw keys through. */
export function diffModel(
  before: WireDocument, after: WireDocument, originalView: ViewDocument,
  rawEntities: Array<Record<string, unknown>>, rawTexts: Record<string, { entries?: Record<string, Record<string, unknown>> }>,
  rawRelations: Array<Record<string, unknown>> = [],
): ModelOp[] {
  const view = after.bundle?.view.id ?? originalView.id;
  const originals = new Map((originalView.placements ?? []).map((p) => [p.entity, p as Record<string, unknown>]));
  const previous = new Map((before.placements ?? []).map((p) => [p.id, p]));
  const current = new Map((after.placements ?? []).map((p) => [p.id, p]));

  const placementOps: ModelOp[] = [];
  for (const id of new Set([...previous.keys(), ...current.keys()])) {
    const op = changedPlacement(id, previous.get(id), current.get(id), originals.get(id), view);
    if (op) placementOps.push(op);
  }

  const entityOps: ModelOp[] = [];
  const originalEntityById = new Map(rawEntities.map((e) => [String(e.id), e]));
  for (const p of after.placements ?? []) {
    const old = previous.get(p.id);
    const entity = originalEntityById.get(p.id);
    if (!entity) {
      // A placement drawn in the editor brings its entity with it; one that was
      // already on the view without a registry record is not minted one.
      if (old) continue;
      // No `name`: the name of an authored entity is a text, sent with the same batch (below).
      entityOps.push({ kind: "entity", id: p.id, value: { id: p.id,
        kind: p.type ?? "", origin: "authored", status: "present" } });
    } else if (old) {
      // A kind read from code is the code's to change; the diagram never overrides it.
      // A name is never an entity edit: an authored one is a text op, one from code is not editable.
      const retyped = old.type !== p.type && entity.origin !== "code";
      if (retyped) {
        entityOps.push({ kind: "entity", id: p.id, value: { ...entity, kind: p.type ?? entity.kind } });
      }
    }
  }

  // A relation's type is a string on the relation (relations.json); what the type
  // means is the dictionary's, the project lists none (CONTRACT.md §5).
  const registryOps: ModelOp[] = [];
  const beforeRelations = new Map((before.bundle?.relations?.relations ?? []).map((r) => [r.id, r]));
  const rawRelationById = new Map(rawRelations.map((r) => [String(r.id), r]));
  for (const r of after.bundle?.relations?.relations ?? []) {
    const old = beforeRelations.get(r.id);
    const raw = rawRelationById.get(r.id);
    if (old && raw && old.type !== r.type && raw.origin !== "code") {
      registryOps.push({ kind: "relation", id: r.id, value: { ...raw, type: r.type } });
    }
  }

  // The name of an authored entity is its text `name` (per language); the name of an entity from
  // code stays in entities.json, untranslated, and is not the diagram's to change. `title` is
  // no field of an entity at all.
  const entityIds = new Set([...originalEntityById.keys(), ...current.keys()]);
  const textOps: ModelOp[] = [];
  const beforeRegistries = before.bundle?.textRegistries ?? {};
  const afterRegistries = after.bundle?.textRegistries ?? {};
  for (const [lang, registry] of Object.entries(afterRegistries)) {
    for (const [id, entry] of Object.entries(registry.entries ?? {})) {
      const prior = beforeRegistries[lang]?.entries?.[id] ?? {};
      const isEntity = entityIds.has(id);
      const known = originalEntityById.get(id) as NamedEntity | undefined;
      // An entity not in the registry gets one only when drawn now (above), so only then is its name a text.
      const nameIsAText = known !== undefined ? nameIsText(known) : current.has(id) && !previous.has(id);
      const changed = fields.filter((field) => {
        if (entry[field] === prior[field]) return false;
        if (!isEntity) return true;
        if (field === "title") return false;
        // An empty name is refused by the host: the editor never sends one, the old name stays.
        if (field === "name") return nameIsAText && (entry.name ?? "").trim() !== "";
        return true;
      });
      if (!changed.length) continue;
      const value = copy(rawTexts[lang]?.entries?.[id] ?? {});
      for (const field of changed) {
        const text = entry[field];
        if (typeof text !== "string" || !text.trim()) throw new Error(`Пустой текст ${id}.${field} нельзя сохранить`);
        value[field] = { v: text, at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"), origin: "authored" };
      }
      textOps.push({ kind: "text", id, lang, value });
    }
  }

  const viewOps: ModelOp[] = [];
  const oldRouting = before.metadata?.routing;
  const newRouting = after.metadata?.routing;
  if (!same(before.edges, after.edges) || oldRouting !== newRouting) {
    const value: Record<string, unknown> = { id: view };
    if (oldRouting !== newRouting) value.routing = newRouting ?? null;
    if (!same(before.edges, after.edges)) value.edges = (after.edges ?? []).map(edgeEntry);
    viewOps.push({ kind: "view", id: view, view, value });
  }

  return [...entityOps, ...registryOps, ...orderPlacements(placementOps, current), ...textOps, ...viewOps];
}

/**
 * The host checks the batch as a whole (docs/API.md §3.4), so order is not a
 * rule; it is kept readable anyway — outer placements first, removals last,
 * after the children of a removed container have moved to its parent (a
 * container is removed only with its children moved out in the same batch).
 */
function orderPlacements(ops: ModelOp[], current: ReadonlyMap<string, WirePlacement>): ModelOp[] {
  const depth = (id: string): number => {
    let d = 0;
    const seen = new Set<string>();
    for (let p = current.get(id)?.parent; p && !seen.has(p); p = current.get(p)?.parent) { seen.add(p); d += 1; }
    return d;
  };
  const kept = ops.filter((op) => op.value !== null).sort((a, b) => depth(a.id) - depth(b.id));
  return [...kept, ...ops.filter((op) => op.value === null)];
}
