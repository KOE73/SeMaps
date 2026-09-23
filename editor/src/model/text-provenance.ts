/**
 * Text catalogue v3: values that carry their own provenance.
 *
 * On disk every translatable field is an object, not a string:
 *
 *   "c_llm": { "description": { "v": "...", "at": "2026-08-31T14:22:05Z",
 *                               "origin": "authored" } }
 *   "c_llm": { "description": { "v": "...", "at": "...", "origin": "translated",
 *                               "from": "ru", "fromHash": "a3f19c" } }
 *
 * In memory the editor keeps plain strings — every panel, renderer and inspector
 * wants `t.name`, not `t.name.v` — and the provenance travels beside them in a
 * side table keyed by id and field. Saving re-joins the two.
 *
 * Hashing deliberately does not live here. The editor never translates: a value
 * a human edited is `authored` by definition, and authored values carry no
 * `from`/`fromHash`. Hashes are only ever computed by the Node-side tooling that
 * does translate, which keeps one implementation instead of two that must agree.
 *
 * See ADR_20260831_diagrams_text-provenance-and-view-axes.
 */

/**
 * The fields a catalogue entry may carry.
 *
 * `fromLabel`/`toLabel` are the cardinality/role captions at a relation's two
 * ends (ADR_20260903 §2.6) — they read and write exactly like every other
 * field here. This module is deliberately blind to what a key means: it does
 * not know `r_*` is a relation or that only relations use these two fields.
 */
export const TEXT_FIELDS = ["name", "title", "description", "doc", "fromLabel", "toLabel"] as const;
export type TextField = (typeof TEXT_FIELDS)[number];

/** Where one value in one language came from. */
export interface TextProvenance {
  /** ISO-8601 UTC, always with `Z`. Informational: checks compare hashes. */
  at?: string;
  origin: "authored" | "translated";
  /** Source language, present only on translated values. */
  from?: string;
  /** Hash of the normalised source text at translation time. */
  fromHash?: string;
}

/** One value on disk. */
export interface TextValue extends TextProvenance {
  v: string;
}

/** Flat, provenance-free view of one entry — what the UI reads and writes. */
export type FlatTextEntry = Partial<Record<TextField, string>>;

/** Provenance for one entry, field by field. */
export type EntryProvenance = Partial<Record<TextField, TextProvenance>>;

export interface ParsedTextCatalog {
  contractVersion: number;
  language: string;
  entries: Record<string, FlatTextEntry>;
  provenance: Record<string, EntryProvenance>;
}

const CURRENT_CONTRACT_VERSION = 3;

function isTextValue(x: unknown): x is TextValue {
  return typeof x === "object" && x !== null && typeof (x as { v?: unknown }).v === "string";
}

/**
 * Read a catalogue in either shape.
 *
 * v2 stored bare strings and no provenance. Those load as values with no
 * provenance at all rather than as fabricated `authored` ones: claiming
 * authorship for text whose origin was never recorded would make the first
 * check report a clean slate that isn't one.
 */
export function parseTextCatalog(raw: unknown, fallbackLanguage = "ru"): ParsedTextCatalog {
  const doc = (raw ?? {}) as {
    contractVersion?: number;
    language?: string;
    entries?: Record<string, unknown>;
  };

  const entries: Record<string, FlatTextEntry> = {};
  const provenance: Record<string, EntryProvenance> = {};

  for (const [id, rawEntry] of Object.entries(doc.entries ?? {})) {
    if (typeof rawEntry !== "object" || rawEntry === null) continue;
    const flat: FlatTextEntry = {};
    const prov: EntryProvenance = {};

    for (const field of TEXT_FIELDS) {
      const value = (rawEntry as Record<string, unknown>)[field];
      if (typeof value === "string") {
        flat[field] = value;
      } else if (isTextValue(value)) {
        flat[field] = value.v;
        prov[field] = {
          origin: value.origin ?? "authored",
          ...(value.at ? { at: value.at } : {}),
          ...(value.from ? { from: value.from } : {}),
          ...(value.fromHash ? { fromHash: value.fromHash } : {}),
        };
      }
    }

    entries[id] = flat;
    if (Object.keys(prov).length > 0) provenance[id] = prov;
  }

  return {
    contractVersion: doc.contractVersion ?? 2,
    language: doc.language ?? fallbackLanguage,
    entries,
    provenance,
  };
}

/**
 * Write a catalogue back out, joining strings with their provenance.
 *
 * A value whose text differs from what was loaded is re-stamped as `authored`
 * now, and loses any `from`/`fromHash` it had: a human just overwrote a
 * translation, so it is no longer one. A value that did not change keeps its
 * record byte for byte, so opening and saving an untouched project produces no
 * diff.
 */
export function serializeTextCatalog(
  language: string,
  entries: Record<string, FlatTextEntry>,
  parsed: ParsedTextCatalog | null,
  now: () => string = utcNow,
): { contractVersion: number; language: string; entries: Record<string, Record<string, TextValue>> } {
  const out: Record<string, Record<string, TextValue>> = {};

  for (const [id, entry] of Object.entries(entries)) {
    const written: Record<string, TextValue> = {};

    for (const field of TEXT_FIELDS) {
      const text = entry[field];
      if (typeof text !== "string" || text.length === 0) continue;

      const before = parsed?.entries[id]?.[field];
      const prov = parsed?.provenance[id]?.[field];

      if (before === text && prov) {
        written[field] = {
          v: text,
          ...(prov.at ? { at: prov.at } : {}),
          origin: prov.origin,
          ...(prov.from ? { from: prov.from } : {}),
          ...(prov.fromHash ? { fromHash: prov.fromHash } : {}),
        };
      } else {
        written[field] = { v: text, at: now(), origin: "authored" };
      }
    }

    if (Object.keys(written).length > 0) out[id] = written;
  }

  return { contractVersion: CURRENT_CONTRACT_VERSION, language, entries: out };
}

/** ISO-8601 in UTC, seconds precision, always `Z`. */
export function utcNow(): string {
  return new Date().toISOString().replace(/\.\d{3}Z$/, "Z");
}
