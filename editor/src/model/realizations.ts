/**
 * Realizations of an entity (`code[]`) and evidence of a relation
 * (`evidence[]`): an entity is the intent, code is 0..N realizations of it, one
 * per language. Both are optional arrays, absent when there is nothing.
 *
 * A realization of an external symbol (`io.Writer`) has no `ref`: there is no
 * file of ours to open, so it gets no code button.
 */

import type { CodeRealization, RelationEntry, RelationEvidence, RelationVia } from "./wire-types.js";

const LANG_TAGS: Record<string, string> = { go: "go", csharp: "c#", typescript: "ts" };

/** Well-formed realizations of an entity, with or without a file; never undefined. */
export function realizationsOf(entity: { code?: unknown } | null | undefined): CodeRealization[] {
  const code = entity?.code;
  if (!Array.isArray(code)) return [];
  return code.filter((c): c is CodeRealization => typeof c === "object" && c !== null);
}

/** The file of a realization or an evidence entry, trimmed; "" when there is none. */
export function refOf(r: { ref?: string } | null | undefined): string {
  return typeof r?.ref === "string" ? r.ref.trim() : "";
}

/** The file of a `ref`, without its `#..` / `:..` anchor; the anchor is the position inside the file. */
export function fileOfRef(ref: string): string {
  return ref.trim().split(/[#:]/, 1)[0]!.trim();
}

/** The line a `ref`'s anchor names (`#L12`, `#12`, `:12`, `#L12-L20`); undefined when it names none. */
export function lineOfRef(ref: string): number | undefined {
  const m = /[#:]L?(\d+)/.exec(ref);
  return m ? Number(m[1]) : undefined;
}

/** Realizations that name a file — the ones a code button can open. */
export function fileRealizations(entity: { code?: unknown } | null | undefined): CodeRealization[] {
  return realizationsOf(entity).filter((r) => refOf(r) !== "");
}

/**
 * Short label for a "view code" button: go → `go`, csharp → `c#`,
 * typescript → `ts`, any other language as is, and an entry without a
 * language (a hand-written file link) → the extension of its file.
 */
export function langTag(r: { lang?: string; ref?: string }): string {
  if (r.lang) return LANG_TAGS[r.lang] ?? r.lang;
  const file = fileOfRef(refOf(r)).split("/").pop() ?? "";
  const dot = file.lastIndexOf(".");
  return dot > 0 ? file.slice(dot + 1).toLowerCase() : file;
}

/** Well-formed evidence entries of a relation; never undefined. */
export function evidenceOf(rel: Pick<RelationEntry, "evidence"> | null | undefined): RelationEvidence[] {
  const evidence = rel?.evidence;
  if (!Array.isArray(evidence)) return [];
  return evidence.filter((e): e is RelationEvidence => typeof e === "object" && e !== null);
}

/** The member signature of a relation: the first evidence entry that has one. */
export function relationVia(rel: Pick<RelationEntry, "evidence"> | null | undefined): RelationVia | undefined {
  return evidenceOf(rel).find((e) => e.via)?.via;
}

/** Text an entity is found by in search: its realizations' symbols and files. */
export function realizationSearchText(entity: { code?: unknown } | null | undefined): string {
  return realizationsOf(entity).map((r) => `${r.symbol ?? ""} ${r.ref ?? ""}`).join(" ");
}
