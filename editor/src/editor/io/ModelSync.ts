import type { WireDocument, WirePlacement, ViewDocument } from "../../model/wire-types.js";
import { EDGE_OVERRIDE_FIELDS, OVERRIDE_FIELDS, serializeOverride } from "../../model/override.js";
import { nameIsText, type NamedEntity } from "../../model/entityName.js";
import { relationShownByDefault } from "../../model/relationVisibility.js";
import { KindCatalog } from "../../model/KindCatalog.js";

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

type WireEdgeOf = NonNullable<WireDocument["edges"]>[number];

/**
 * What a line adds to the relation it draws, on this view: `styleId`, `override`,
 * `routing` in the contract's field order (§8.5); undefined when it adds nothing.
 * Ends, type and visibility are never here — they are the registry's and the rule's.
 */
function ownFields(e: WireEdgeOf | undefined): Record<string, unknown> | undefined {
  if (e === undefined) return undefined;
  const override = serializeOverride(EDGE_OVERRIDE_FIELDS, e.override);
  const out = {
    ...(e.styleId ? { styleId: e.styleId } : {}),
    ...(override ? { override } : {}),
    ...(e.routing ? { routing: e.routing } : {}),
  };
  return Object.keys(out).length > 0 ? out : undefined;
}

/** The own fields of a view file's `edges` entry, whatever else an old file put in it. */
function entryFields(entry: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (entry === undefined) return undefined;
  const out: Record<string, unknown> = {};
  for (const key of ["styleId", "override", "routing"]) if (entry[key] !== undefined) out[key] = entry[key];
  return Object.keys(out).length > 0 ? out : undefined;
}

/**
 * The view's `edges` value when an edge's own look changed, else undefined (send
 * nothing). The value is the overlay only (ADR_20260930-7): an entry per relation
 * that has an own field, no `from`/`to`/`type`; `null` — the host removes the key —
 * when none is left. A hidden line keeps its entry (its look comes back with it);
 * an entry of an old file whose id is not in the registry is not carried over.
 */
function overlayChange(
  before: WireDocument, after: WireDocument, originalView: ViewDocument, withdrawn: ReadonlySet<string>,
): unknown[] | null | undefined {
  const file = new Map<string, Record<string, unknown>>();
  for (const entry of (originalView.edges ?? []) as unknown as Array<Record<string, unknown>>) {
    if (typeof entry?.id === "string" && !file.has(entry.id)) file.set(entry.id, entry);
  }
  const beforeById = new Map((before.edges ?? []).map((e) => [e.id, e]));
  const afterById = new Map((after.edges ?? []).map((e) => [e.id, e]));

  // A line that is gone from the picture is hidden, which changes no look; a line
  // that is there is compared with what it was, or with the file when it is new to the picture.
  // A withdrawn relation leaves no entry behind (ADR_20260930-8).
  let changed = [...file.keys()].some((id) => withdrawn.has(id));
  for (const e of afterById.values()) {
    if (changed) break;
    const b = beforeById.get(e.id);
    const was = b !== undefined ? ownFields(b) : entryFields(file.get(e.id));
    if (!same(was, ownFields(e))) changed = true;
  }
  if (!changed) return undefined;

  const known = new Set([
    ...(after.bundle?.relations?.relations ?? []).map((r) => r.id),
    ...(before.bundle?.relations?.relations ?? []).map((r) => r.id),
  ].filter((id) => !withdrawn.has(id)));
  const list: Array<Record<string, unknown>> = [];
  const emitted = new Set<string>();
  const push = (id: string, own: Record<string, unknown> | undefined): void => {
    emitted.add(id);
    if (own !== undefined) list.push({ id, ...own });
  };
  for (const id of file.keys()) {
    if (!known.has(id)) continue;
    const e = afterById.get(id);
    push(id, e !== undefined ? ownFields(e) : entryFields(file.get(id)));
  }
  for (const e of afterById.values()) if (!emitted.has(e.id) && known.has(e.id)) push(e.id, ownFields(e));

  if (list.length === 0) return originalView.edges ? null : undefined;
  return list;
}

/**
 * The view's `relations` value when what is visible changed, else undefined. Hiding
 * or showing a line is a change of `relations.except` (§8.5, as the host's
 * `set_relation_visible`): for every relation with both ends on the view, the line
 * is there or it is not, and `except` lists those where that differs from the
 * default (type's visibility in the dictionary, else `relations.default`). Entries
 * for relations the view cannot decide (an end is not placed) stay as they were.
 * A relation the editor knew and no longer does (a drawn line undone) is hidden: the
 * registry keeps what it was given — unless it is withdrawn (`withdrawn`, an unsaved
 * creation, ADR_20260930-8): then the view no longer names it at all.
 */
function policyChange(
  before: WireDocument, after: WireDocument, originalView: ViewDocument, withdrawn: ReadonlySet<string>,
): Record<string, unknown> | undefined {
  const policy = (originalView.relations ?? {}) as unknown as { default?: string; except?: string[]; [k: string]: unknown };
  const oldExcept = Array.isArray(policy.except) ? policy.except : [];
  const placed = new Set((after.placements ?? []).map((p) => p.id));
  const shown = new Set((after.edges ?? []).map((e) => e.id));
  const registry = new Map<string, { id: string; from: string; to: string; type?: string }>();
  for (const r of before.bundle?.relations?.relations ?? []) if (!withdrawn.has(r.id)) registry.set(r.id, r);
  for (const r of after.bundle?.relations?.relations ?? []) if (!withdrawn.has(r.id)) registry.set(r.id, r);

  const decided = new Set<string>();
  const wanted: string[] = [];
  for (const r of registry.values()) {
    if (!placed.has(r.from) || !placed.has(r.to)) continue;
    decided.add(r.id);
    const byDefault = relationShownByDefault(r, { ...(policy.default === undefined ? {} : { default: policy.default }) }, KindCatalog.active);
    if (shown.has(r.id) !== byDefault) wanted.push(r.id);
  }
  const next = [
    ...oldExcept.filter((id) => !withdrawn.has(id) && (!decided.has(id) || wanted.includes(id))),
    ...wanted.filter((id) => !oldExcept.includes(id)),
  ];
  const stay = next.length === oldExcept.length && next.every((id) => oldExcept.includes(id));
  if (stay) return undefined;
  const out: Record<string, unknown> = { ...policy };
  // As the host writes it: the key stays once the view has had it.
  if (next.length > 0 || "except" in policy) out.except = next;
  return out;
}

/**
 * The records the host holds only in its unsaved working state — the ones a `null`
 * op may withdraw (docs/API.md §3.4, ADR_20260930-8). Empty sets: nothing is
 * withdrawn, every removal is a hide.
 */
export interface UnsavedRecords { entity: ReadonlySet<string>; relation: ReadonlySet<string> }

/**
 * Compare completed editor actions by object; carry untouched raw keys through.
 *
 * A relation that the baseline had and `after` no longer has (a drawn line undone),
 * and an entity whose placement left the view, are withdrawn when `unsaved` names them:
 * `{kind, id, value: null}`, with the view's mentions of the relation taken away in the
 * same batch (its texts go with the record, in the host). A record of the saved registry
 * is never withdrawn: a line is hidden through `relations.except`, a block leaves
 * the view and stays in the registry.
 */
export function diffModel(
  before: WireDocument, after: WireDocument, originalView: ViewDocument,
  rawEntities: Array<Record<string, unknown>>, rawTexts: Record<string, { entries?: Record<string, Record<string, unknown>> }>,
  rawRelations: Array<Record<string, unknown>> = [],
  unsaved: UnsavedRecords = { entity: new Set(), relation: new Set() },
): ModelOp[] {
  const view = after.bundle?.view.id ?? originalView.id;
  const originals = new Map((originalView.placements ?? []).map((p) => [p.entity, p as Record<string, unknown>]));
  const previous = new Map((before.placements ?? []).map((p) => [p.id, p]));
  const current = new Map((after.placements ?? []).map((p) => [p.id, p]));

  // What this action takes back: an unsaved entity whose block left the view, and every unsaved
  // relation that is gone from the model or that ended at such an entity.
  const withdrawnEntities = new Set([...previous.keys()].filter((id) => !current.has(id) && unsaved.entity.has(id)));
  const afterRelationIds = new Set((after.bundle?.relations?.relations ?? []).map((r) => r.id));
  const withdrawnRelations = new Set<string>();
  const known = new Map<string, { from: string; to: string }>();
  for (const r of [...rawRelations as Array<{ id: string; from: string; to: string }>, ...(before.bundle?.relations?.relations ?? [])]) {
    known.set(String(r.id), { from: String(r.from), to: String(r.to) });
  }
  for (const [id, ends] of known) {
    if (!unsaved.relation.has(id)) continue;
    const gone = (before.bundle?.relations?.relations ?? []).some((r) => r.id === id) && !afterRelationIds.has(id);
    if (gone || withdrawnEntities.has(ends.from) || withdrawnEntities.has(ends.to)) withdrawnRelations.add(id);
  }

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
    } else if (!old && raw?.origin !== "code") {
      // (`raw` cannot tell: the loaded document shares the registry's array, so a relation
      // drawn just now is already in it. A send of one the host has is an idempotent upsert.)
      // A line drawn in the editor is a relation of the registry, sent with the batch
      // that carries its look and its visibility (ADR_20260930-7), as a drawn block brings its entity.
      registryOps.push({ kind: "relation", id: r.id, value: { id: r.id, from: r.from, to: r.to,
        type: r.type, origin: "authored", status: "present" } });
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
  const value: Record<string, unknown> = { id: view };
  if (oldRouting !== newRouting) value.routing = newRouting ?? null;
  const edges = overlayChange(before, after, originalView, withdrawnRelations);
  if (edges !== undefined) value.edges = edges;
  const relations = policyChange(before, after, originalView, withdrawnRelations);
  if (relations !== undefined) value.relations = relations;
  if (Object.keys(value).length > 1) viewOps.push({ kind: "view", id: view, view, value });

  const withdrawOps: ModelOp[] = [
    ...[...withdrawnRelations].map((id): ModelOp => ({ kind: "relation", id, value: null })),
    ...[...withdrawnEntities].map((id): ModelOp => ({ kind: "entity", id, value: null })),
  ];
  return [...entityOps, ...registryOps, ...orderPlacements(placementOps, current), ...textOps, ...viewOps, ...withdrawOps];
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
