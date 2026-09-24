import type { DiagramDocument } from "../model/document.js";
import type { EntityEntry } from "../model/wire-types.js";

export type Dir = "out" | "in";

/** Every neighbour reached from one entity by one relation type in one direction. */
export interface Arm {
  type: string;
  dir: Dir;
  entities: EntityEntry[];
}

/**
 * The relation types every extractor prints (EXTRACTOR.md §2.2): kinship that
 * belongs to what a type *is*, not to what one project chose to record.
 * Only these may be named by the editor; every other type is open-ended.
 */
export const STRUCTURAL = ["extends", "implements", "contains"] as const;

/**
 * The entity's arms in the registry, grouped by type and direction, outgoing
 * first. `types`/`dirs` narrow it; a missing filter lets everything through.
 */
export function armsOf(
  doc: DiagramDocument,
  id: string,
  filter: { hiddenTypes?: ReadonlySet<string>; dirs?: ReadonlySet<Dir> } = {},
): Arm[] {
  const byId = entityIndex(doc);
  const byKey = new Map<string, Arm>();
  for (const r of doc.relations) {
    if (!r || r.from === r.to || filter.hiddenTypes?.has(r.type)) continue;
    let dir: Dir | null = null;
    let other = "";
    if (r.from === id) { dir = "out"; other = r.to; }
    else if (r.to === id) { dir = "in"; other = r.from; }
    if (dir === null || (filter.dirs && !filter.dirs.has(dir))) continue;
    const entity = byId.get(other);
    if (!entity) continue;
    const key = `${r.type}|${dir}`;
    let arm = byKey.get(key);
    if (!arm) byKey.set(key, (arm = { type: r.type, dir, entities: [] }));
    if (!arm.entities.includes(entity)) arm.entities.push(entity);
  }
  const arms = [...byKey.values()];
  for (const a of arms) a.entities.sort((x, y) => nameOf(doc, x).localeCompare(nameOf(doc, y)));
  return arms.sort((a, b) => (a.dir === b.dir ? a.type.localeCompare(b.type) : a.dir === "out" ? -1 : 1));
}

/**
 * Everyone reached by following one relation type in one direction as far as
 * it goes: all ancestors, all descendants. Nearest first; cycles end the walk.
 */
export function closure(doc: DiagramDocument, id: string, type: string, dir: Dir): EntityEntry[] {
  return rings(doc, id, type, dir).flat();
}

/**
 * The same walk, kept ring by ring: `[0]` the direct neighbours, `[1]` theirs,
 * and so on — so "two levels of descendants" is `rings(...).slice(0, 2)`.
 */
export function rings(doc: DiagramDocument, id: string, type: string, dir: Dir): EntityEntry[][] {
  const byId = entityIndex(doc);
  const next = new Map<string, string[]>();
  for (const r of doc.relations) {
    if (!r || r.type !== type || r.from === r.to) continue;
    const [a, b] = dir === "out" ? [r.from, r.to] : [r.to, r.from];
    const list = next.get(a);
    if (list) list.push(b);
    else next.set(a, [b]);
  }
  const seen = new Set([id]);
  const out: EntityEntry[][] = [];
  let ring = [id];
  while (ring.length > 0) {
    const nextRing: string[] = [];
    const found: EntityEntry[] = [];
    for (const at of ring) {
      for (const n of next.get(at) ?? []) {
        if (seen.has(n)) continue;
        seen.add(n);
        const e = byId.get(n);
        if (e) found.push(e);
        nextRing.push(n);
      }
    }
    if (found.length > 0) out.push(found);
    ring = nextRing;
  }
  return out;
}

/** The name the user sees: the text catalogue's, else the registry's. */
export function nameOf(doc: DiagramDocument, e: EntityEntry): string {
  const texts = doc.bundle?.text?.entries || {};
  return String((texts[e.id] as { name?: string } | undefined)?.name || e.name || e.id);
}

function entityIndex(doc: DiagramDocument): Map<string, EntityEntry> {
  return new Map(doc.entities.map((e) => [e.id, e]));
}
