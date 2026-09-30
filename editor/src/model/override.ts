import type { WireStyle } from "./style-types.js";

/**
 * `override` — a partial style on one placement, or on one edge entry of a
 * view (CONTRACT.md §11.6).
 *
 * The one table of what may be overridden, mirroring `core/override.go`
 * (`OverrideFields`, `EdgeOverrideFields`). Everything that deals with an
 * override — reading it from a view, drawing it, writing it back, the
 * «Выделить» section of Properties — walks these tables and nothing else, so a
 * new overridable field is one line here (plus its label under
 * `panels.properties.overrideFields` in the locales, keyed by `key`) and one
 * line in `core/override.go`.
 */

/** How the Inspector edits a field. */
export type OverrideInput = "color" | "dash" | "glyph" | "width";

export interface OverrideField {
  /** Dotted path, as in the contract table; also the i18n label key. */
  readonly key: string;
  readonly path: readonly string[];
  readonly input: OverrideInput;
  /** Meaningful only on a container (the header band); the Inspector hides it on a block. */
  readonly containerOnly?: boolean;
}

/** What a placement (a block or a container) may override — `core.OverrideFields`. */
export const OVERRIDE_FIELDS: readonly OverrideField[] = [
  { key: "fill", path: ["fill"], input: "color" },
  { key: "border.color", path: ["border", "color"], input: "color" },
  { key: "border.dash", path: ["border", "dash"], input: "dash" },
  { key: "header.fill", path: ["header", "fill"], input: "color", containerOnly: true },
  { key: "icon.glyph", path: ["icon", "glyph"], input: "glyph" },
];

/** What an edge entry of a view may override — `core.EdgeOverrideFields`. */
export const EDGE_OVERRIDE_FIELDS: readonly OverrideField[] = [
  { key: "line.color", path: ["line", "color"], input: "color" },
  { key: "line.width", path: ["line", "width"], input: "width" },
  { key: "line.dash", path: ["line", "dash"], input: "dash" },
];

/** An override as the file holds it: a sparse subset of a style. */
export type PlacementOverride = Readonly<Record<string, unknown>>;

type Bag = Record<string, unknown>;

const isBag = (v: unknown): v is Bag => typeof v === "object" && v !== null && !Array.isArray(v);

function read(bag: unknown, path: readonly string[]): unknown {
  let cursor: unknown = bag;
  for (const key of path) {
    if (!isBag(cursor)) return undefined;
    cursor = cursor[key];
  }
  return cursor;
}

function write(bag: Bag, path: readonly string[], value: unknown): void {
  let cursor = bag;
  for (const key of path.slice(0, -1)) {
    const next = cursor[key];
    if (!isBag(next)) cursor[key] = {};
    cursor = cursor[key] as Bag;
  }
  cursor[path[path.length - 1]!] = value;
}

/** Every leaf path present in `raw`, dotted. */
function leaves(raw: unknown, prefix: readonly string[] = []): string[] {
  if (!isBag(raw)) return prefix.length > 0 ? [prefix.join(".")] : [];
  const out: string[] = [];
  for (const [key, value] of Object.entries(raw)) out.push(...leaves(value, [...prefix, key]));
  return out;
}

/** The value a field holds, or undefined. A width is a number, everything else a string. */
export function overrideValue(
  override: PlacementOverride | undefined,
  field: OverrideField,
): string | number | undefined {
  const v = read(override, field.path);
  if (field.input === "width") return typeof v === "number" && Number.isFinite(v) ? v : undefined;
  return typeof v === "string" ? v : undefined;
}

const empty = (v: string | number | undefined): boolean => v === undefined || (typeof v === "string" && v.trim() === "");

/**
 * Build an override from a table's fields only, in table order. Empty —
 * undefined, so an object that overrides nothing carries no `override` key.
 */
function build(
  table: readonly OverrideField[],
  get: (field: OverrideField) => string | number | undefined,
): PlacementOverride | undefined {
  const out: Bag = {};
  let any = false;
  for (const field of table) {
    const v = get(field);
    if (empty(v)) continue;
    write(out, field.path, v);
    any = true;
  }
  return any ? out : undefined;
}

/**
 * Read `override` from a view file. Fields outside the table, or of the wrong
 * type, are dropped and named in `rejected` so the caller can say so.
 */
export function parseOverride(
  raw: unknown,
  table: readonly OverrideField[],
): { value: PlacementOverride | undefined; rejected: string[] } {
  if (raw === undefined || raw === null) return { value: undefined, rejected: [] };
  if (!isBag(raw)) return { value: undefined, rejected: ["override"] };
  const byKey = new Map(table.map((f) => [f.key, f]));
  const rejected = leaves(raw).filter((key) => {
    const field = byKey.get(key);
    return field === undefined || overrideValue(raw, field) === undefined;
  });
  return { value: build(table, (field) => overrideValue(raw, field)), rejected };
}

/** Set (or clear, with undefined) one field; returns the new override or undefined when empty. */
export function withOverride(
  table: readonly OverrideField[],
  override: PlacementOverride | undefined,
  field: OverrideField,
  value: string | number | undefined,
): PlacementOverride | undefined {
  return build(table, (f) => (f.key === field.key ? value : overrideValue(override, f)));
}

/** What goes into the view file: table fields only, table order; undefined when empty. */
export function serializeOverride(
  table: readonly OverrideField[],
  override: PlacementOverride | undefined,
): PlacementOverride | undefined {
  return override === undefined ? undefined : build(table, (f) => overrideValue(override, f));
}

/** The style to draw with: `style` with the override's fields laid on top. */
export function applyOverride(
  table: readonly OverrideField[],
  style: WireStyle,
  override: PlacementOverride | undefined,
): WireStyle {
  if (override === undefined) return style;
  const out = structuredClone(style) as unknown as Bag;
  for (const field of table) {
    const v = overrideValue(override, field);
    if (v !== undefined) write(out, field.path, v);
  }
  return out as unknown as WireStyle;
}

/** Stable key of an override, for caching resolved styles. */
export function overrideKey(table: readonly OverrideField[], override: PlacementOverride | undefined): string {
  return override === undefined ? "" : table.map((f) => String(overrideValue(override, f) ?? "")).join("\u0001");
}

/** An override's value in a style's own fields (what the style gives when the override is empty). */
export function styleValue(style: WireStyle | undefined, field: OverrideField): string | number | undefined {
  return style === undefined ? undefined : overrideValue(style as unknown as PlacementOverride, field);
}
