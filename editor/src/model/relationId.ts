/**
 * The id of a relation drawn by hand: `r_<from>_<to>_<type>` with the `e_` of the
 * ends dropped and the type reduced to lower-case words, `_2`, `_3`… when taken —
 * the shape the host's `add_relation` and the sync mint (CONTRACT.md §4), so an
 * id made here is not told apart from one made there.
 */
export function newRelationId(from: string, to: string, type: string, taken: ReadonlySet<string>): string {
  const slug = (s: string): string => (s.toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? []).join("_");
  const strip = (id: string): string => (id.startsWith("e_") ? id.slice(2) : id);
  const base = `r_${strip(from)}_${strip(to)}_${slug(type)}`;
  let id = base;
  for (let n = 2; taken.has(id); n++) id = `${base}_${n}`;
  return id;
}
