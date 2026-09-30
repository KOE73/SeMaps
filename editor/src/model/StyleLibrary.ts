import type {
  BlockShape,
  EndShape,
  RoutingMode,
  StyleTarget,
  WireEndpoint,
  WireHeader,
  WirePaint,
  WireStroke,
  WireStyle,
  WireStyleSheet,
  WireText,
} from "./style-types.js";
import type { DiagramEdge, DiagramElement } from "./types.js";
import { KindCatalog } from "./KindCatalog.js";
import {
  applyOverride,
  EDGE_OVERRIDE_FIELDS,
  OVERRIDE_FIELDS,
  overrideKey,
  type PlacementOverride,
} from "./override.js";

/**
 * The style library: the single answer to "what does this thing look like".
 *
 * Two layers, deliberately:
 *
 *   WireStyle      — sparse, inheritable, what the file holds and the editor edits.
 *   Resolved*Style — dense, inheritance already flattened, what renderers read.
 *
 * Renderers never see an optional field and never chase `basedOn`. That is what
 * lets a renderer stay a dumb function of a style, and what lets the style
 * editor stay a dumb function of the sparse form.
 */

// --------------------------------------------------------------- resolved forms

export interface GradientStop {
  readonly offset: number;
  readonly color: string;
  readonly opacity: number;
}

export type Paint =
  | { readonly kind: "solid"; readonly color: string }
  | { readonly kind: "linear"; readonly angle: number; readonly stops: readonly GradientStop[] }
  | { readonly kind: "radial"; readonly stops: readonly GradientStop[] };

export interface Stroke {
  readonly color: string;
  readonly width: number;
  readonly dash: string;
  readonly opacity: number;
}

export interface TextStyle {
  readonly family: string;
  readonly size: number;
  readonly weight: number;
  readonly italic: boolean;
  readonly color: string;
  readonly align: "start" | "middle" | "end";
  readonly opacity: number;
  readonly show: boolean;
}

export interface Endpoint {
  readonly shape: EndShape;
  readonly size: number;
  /** null means "take the line's colour". */
  readonly color: string | null;
}

export interface ResolvedBlockStyle {
  readonly id: string;
  readonly name: string;
  readonly fill: Paint;
  readonly border: Stroke;
  readonly radius: number;
  readonly shadow: boolean;
  readonly header: {
    readonly fill: Paint;
    readonly height: number;
    readonly text: TextStyle;
  };
  readonly title: TextStyle;
  readonly subtitle: TextStyle;
  readonly icon: { readonly glyph: string; readonly show: boolean };
  /** Outline to draw and to attach lines to. */
  readonly shape: BlockShape;
  /**
   * Content template id, or null for the built-in "title only" look.
   *
   * Null rather than a default id on purpose: a workspace with no
   * `templates.json` at all must still draw, and it should draw what it drew
   * before templates existed.
   */
  readonly template: string | null;
}

export interface ResolvedEdgeStyle {
  readonly id: string;
  readonly name: string;
  readonly line: Stroke;
  readonly source: Endpoint;
  readonly target: Endpoint;
  readonly label: TextStyle;
  readonly family: "structure" | "flow";
  readonly overview: boolean;
  /**
   * Line shape for this relation type, or null to take the view's choice.
   *
   * Null is the common case and the useful one: most edges should follow the
   * picture's convention, and only types whose meaning has a shape — a tree
   * for inheritance — pin it here.
   */
  readonly routing: RoutingMode | null;
}

// ------------------------------------------------------------------- fallbacks

const SANS = "Inter, system-ui, -apple-system, Segoe UI, sans-serif";
const MONO = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

/**
 * The floor. Every resolution ends here, so a style file may be missing, empty
 * or nonsense and the canvas still draws something legible.
 */
export const FALLBACK_BLOCK: ResolvedBlockStyle = {
  id: "—",
  name: "Без стиля",
  fill: { kind: "solid", color: "#ffffff" },
  border: { color: "#cbd5e1", width: 1.5, dash: "none", opacity: 1 },
  radius: 8,
  shadow: true,
  header: {
    fill: { kind: "solid", color: "#e2e8f0" },
    height: 34,
    text: {
      family: SANS, size: 13, weight: 700, italic: false,
      color: "#334155", align: "start", opacity: 1, show: true,
    },
  },
  title: {
    family: SANS, size: 12.5, weight: 600, italic: false,
    color: "#1e293b", align: "start", opacity: 1, show: true,
  },
  subtitle: {
    family: MONO, size: 10.5, weight: 400, italic: false,
    color: "#64748b", align: "start", opacity: 1, show: true,
  },
  icon: { glyph: "file-text", show: true },
  shape: "rect",
  template: null,
};

export const FALLBACK_EDGE: ResolvedEdgeStyle = {
  id: "—",
  name: "Без стиля",
  line: { color: "#cbd5e1", width: 1.5, dash: "none", opacity: 1 },
  source: { shape: "none", size: 6, color: null },
  target: { shape: "arrow", size: 6, color: null },
  label: {
    family: SANS, size: 10, weight: 500, italic: false,
    color: "#475569", align: "middle", opacity: 1, show: true,
  },
  family: "flow",
  overview: false,
  routing: null,
};

/**
 * Ids the library looks for when an element resolves to nothing else.
 *
 * They are ordinary styles living in the same file as the rest, so the last
 * resort is editable too — there is no hidden appearance anywhere.
 */
export const DEFAULT_STYLE_IDS = {
  node: "default.node",
  container: "default.container",
  edge: "default.edge",
} as const;

// ------------------------------------------------------------------ the library

export interface StyleListEntry {
  readonly style: WireStyle;
  readonly id: string;
  readonly name: string;
  readonly appliesTo: StyleTarget;
}

export class StyleLibrary {
  private readonly byId = new Map<string, WireStyle>();
  /** Insertion order, so the list panel and the file agree. */
  private order: string[] = [];

  private blockCache = new Map<string, ResolvedBlockStyle>();
  private edgeCache = new Map<string, ResolvedEdgeStyle>();

  /** Fields the file carried that this version does not model, kept for round trips. */
  private sheetExtras: Omit<WireStyleSheet, "styles"> = {};

  static parse(wire: WireStyleSheet | null | undefined): StyleLibrary {
    const lib = new StyleLibrary();
    const { styles, ...extras } = wire ?? {};
    lib.sheetExtras = extras;
    for (const style of styles ?? []) {
      if (typeof style?.id !== "string" || style.id === "") continue;
      lib.put(style);
    }
    return lib;
  }

  serialize(): WireStyleSheet {
    return {
      ...this.sheetExtras,
      version: this.sheetExtras.version ?? 1,
      styles: this.order.map((id) => this.byId.get(id)!),
    };
  }

  // ------------------------------------------------------------------ reading

  get size(): number {
    return this.order.length;
  }

  get(id: string): WireStyle | undefined {
    return this.byId.get(id);
  }

  has(id: string): boolean {
    return this.byId.has(id);
  }

  /**
   * Styles for a target, optionally narrowed by a filter.
   *
   * The filter matches id, name, description and tags, because with a hundred
   * styles the thing the user remembers is rarely the thing that names the
   * style — "пунктир" may live only in the description.
   */
  list(target?: StyleTarget, filter = ""): StyleListEntry[] {
    const needle = filter.trim().toLowerCase();
    const out: StyleListEntry[] = [];
    for (const id of this.order) {
      const style = this.byId.get(id)!;
      const appliesTo = style.appliesTo ?? "block";
      if (target !== undefined && appliesTo !== target) continue;
      if (needle !== "" && !matches(style, needle)) continue;
      out.push({ style, id, name: style.name ?? id, appliesTo });
    }
    return out;
  }

  /**
   * A style's tags, with `basedOn` already walked.
   *
   * Tags are the domain axis, not the colour axis. They inherit through the
   * same chain as everything else: a style based on one tagged `llm` is still
   * tagged `llm` without repeating it, exactly like it inherits its fill.
   */
  tagsOf(id: string | null): readonly string[] {
    if (id === null) return [];
    return this.flatten(id).tags ?? [];
  }

  /** A style with its `basedOn` chain walked: every field it ends up with, still sparse. */
  effective(id: string): WireStyle {
    return this.flatten(id);
  }

  // ------------------------------------------------- style belongs to a type

  /**
   * The types a style belongs to: its own `forKinds`, never inherited through
   * `basedOn` (CONTRACT.md §11.5, ADR_20260927-7, ADR_20260930-2). Entity kinds
   * for a block or container style, relation types for an edge style. Empty —
   * a style of no type.
   */
  forKindsOf(id: string): readonly string[] {
    return this.byId.get(id)?.forKinds ?? [];
  }

  /** What a style is for: `block` when the file does not say. */
  targetOf(id: string): StyleTarget {
    return this.byId.get(id)?.appliesTo ?? "block";
  }

  /** `appliesTo` of the styles a kind's placements wear: `container` for a container kind. */
  static targetOfKind(kind: string): StyleTarget {
    return KindCatalog.active.isContainer(kind) ? "container" : "block";
  }

  /** `appliesTo` of the styles an element wears. */
  static targetOfElement(el: Pick<DiagramElement, "kind">): StyleTarget {
    return el.kind === "zone" ? "container" : "block";
  }

  /** Whether a style is one of the three fallbacks, worn by a type that has no style; offered to no type. */
  static isFallback(id: string): boolean {
    return id === DEFAULT_STYLE_IDS.node || id === DEFAULT_STYLE_IDS.container || id === DEFAULT_STYLE_IDS.edge;
  }

  /**
   * Whether a style belongs to `key` — an entity kind or, for `target: "edge"`,
   * a relation type: its `appliesTo` is `target` and its own `forKinds` names
   * `key` (CONTRACT.md §11.5).
   */
  fits(id: string, key: string, target: StyleTarget): boolean {
    return this.byId.has(id) && this.targetOf(id) === target && this.forKindsOf(id).includes(key);
  }

  /**
   * A style that belongs to no type — no `forKinds` of its own and not a
   * fallback. Never offered in a picker; the Styles panel lists it under
   * «Без типа» until it is given a type.
   */
  isKindless(id: string): boolean {
    return this.byId.has(id) && this.forKindsOf(id).length === 0 && !StyleLibrary.isFallback(id);
  }

  /**
   * The type's base style: the one the kinds catalog names, else the style
   * whose id is the type, else the fallback of the sort (§11.5). Never written
   * as an explicit `styleId`. `null` — not even the fallback exists.
   *
   * A candidate of another sort (an edge style named like a block kind) is not
   * taken: `security` the entity kind and `security` the relation type are two
   * things.
   */
  baseStyleOf(key: string, target: StyleTarget): string | null {
    const catalog = KindCatalog.active;
    const named = target === "edge" ? catalog.relationBaseStyle(key) : catalog.baseStyle(key);
    if (named !== undefined && this.byId.has(named) && this.targetOf(named) === target) return named;
    if (this.byId.has(key) && this.targetOf(key) === target) return key;
    const fallback = target === "edge" ? DEFAULT_STYLE_IDS.edge : target === "container" ? DEFAULT_STYLE_IDS.container : DEFAULT_STYLE_IDS.node;
    return this.byId.has(fallback) ? fallback : null;
  }

  /**
   * The styles a placement of `key` may wear, base first, then the type's other
   * styles in file order. What every picker offers — and only this. A type
   * whose base is only the fallback has no styles of its own to offer.
   */
  stylesOf(key: string, target: StyleTarget): StyleListEntry[] {
    const base = this.baseStyleOf(key, target);
    const out: StyleListEntry[] = [];
    if (base !== null && !StyleLibrary.isFallback(base)) {
      const style = this.byId.get(base)!;
      out.push({ style, id: base, name: style.name ?? base, appliesTo: target });
    }
    for (const entry of this.list(target)) {
      if (entry.id !== base && this.forKindsOf(entry.id).includes(key)) out.push(entry);
    }
    return out;
  }

  // ------------------------------------------------------------- resolution

  /**
   * Which style an element wears (CONTRACT.md §11.5).
   *
   * The placement's `styleId` first, then the kind's base style — the one the
   * kinds catalog names, else the style whose id is the kind — then
   * `default.node` / `default.container`. So a kind says what it looks like
   * once, while a human can still pin one particular box to another style of
   * the same kind.
   */
  blockStyleIdFor(el: Pick<DiagramElement, "kind" | "type" | "styleId">): string | null {
    if (el.styleId !== undefined && this.byId.has(el.styleId)) return el.styleId;
    return this.baseStyleOf(el.type, StyleLibrary.targetOfElement(el));
  }

  /**
   * The same cascade for a line: the edge entry's `styleId`, the relation
   * type's base style, the style whose id is the type, `default.edge`.
   */
  edgeStyleIdFor(edge: Pick<DiagramEdge, "type" | "styleId">): string | null {
    if (edge.styleId !== undefined && this.byId.has(edge.styleId)) return edge.styleId;
    return this.baseStyleOf(edge.type, "edge");
  }

  /** The look an element is drawn with: its style, with its `override` on top (§11.6). */
  blockStyle(el: DiagramElement): ResolvedBlockStyle {
    return this.resolveBlock(this.blockStyleIdFor(el), el.override);
  }

  /** The look a line is drawn with: its style, with its entry's `override` on top (§11.6). */
  edgeStyle(edge: DiagramEdge): ResolvedEdgeStyle {
    return this.resolveEdge(this.edgeStyleIdFor(edge), edge.override);
  }

  resolveBlock(id: string | null, override?: PlacementOverride): ResolvedBlockStyle {
    const key = `${id ?? ""}|${overrideKey(OVERRIDE_FIELDS, override)}`;
    const cached = this.blockCache.get(key);
    if (cached !== undefined) return cached;
    const resolved =
      id === null && override === undefined
        ? FALLBACK_BLOCK
        : buildBlock(
            id ?? FALLBACK_BLOCK.id,
            applyOverride(OVERRIDE_FIELDS, id === null ? { id: FALLBACK_BLOCK.id } : this.flatten(id), override),
          );
    this.blockCache.set(key, resolved);
    return resolved;
  }

  resolveEdge(id: string | null, override?: PlacementOverride): ResolvedEdgeStyle {
    const key = `${id ?? ""}|${overrideKey(EDGE_OVERRIDE_FIELDS, override)}`;
    const cached = this.edgeCache.get(key);
    if (cached !== undefined) return cached;
    const resolved =
      id === null && override === undefined
        ? FALLBACK_EDGE
        : buildEdge(
            id ?? FALLBACK_EDGE.id,
            applyOverride(EDGE_OVERRIDE_FIELDS, id === null ? { id: FALLBACK_EDGE.id } : this.flatten(id), override),
          );
    this.edgeCache.set(key, resolved);
    return resolved;
  }

  /**
   * Collapse a `basedOn` chain into one sparse style, nearest wins.
   *
   * A cycle stops at the first repeat rather than throwing: a broken style file
   * should make one style look wrong, not take the whole canvas down.
   */
  private flatten(id: string): WireStyle {
    const chain: WireStyle[] = [];
    const seen = new Set<string>();
    let cursor: string | undefined = id;
    while (cursor !== undefined && !seen.has(cursor)) {
      seen.add(cursor);
      const style = this.byId.get(cursor);
      if (style === undefined) break;
      chain.push(style);
      cursor = style.basedOn;
    }
    // Furthest ancestor first, so nearer overrides land on top.
    let out: WireStyle = { id };
    for (const style of chain.reverse()) out = mergeStyle(out, style);
    // `forKinds` is what the style is *for*, not how it looks: never inherited
    // (ADR_20260927-7) — «Компонент» drawn like a class is not a class style.
    const own = this.byId.get(id)?.forKinds;
    if (own === undefined) delete out.forKinds;
    else out.forKinds = own;
    return out;
  }

  // ------------------------------------------------------------------ writing

  /** Insert or replace, keeping list position for a replacement. */
  put(style: WireStyle): void {
    if (!this.byId.has(style.id)) this.order.push(style.id);
    this.byId.set(style.id, style);
    this.invalidate();
  }

  remove(id: string): boolean {
    if (!this.byId.delete(id)) return false;
    this.order = this.order.filter((x) => x !== id);
    // Anything that inherited from it keeps its own fields and falls back to
    // the defaults for the rest — a dangling `basedOn` is not an error.
    this.invalidate();
    return true;
  }

  /**
   * Copy a style under a fresh id.
   *
   * The copy is flat-by-reference, not `basedOn` the original: cloning is what
   * people reach for when they want to diverge, and a clone that silently
   * tracked its source would surprise them later.
   */
  clone(id: string, newId?: string, newName?: string): WireStyle | null {
    const source = this.byId.get(id);
    if (source === undefined) return null;
    const targetId = newId ?? this.freeId(id);
    const copy: WireStyle = {
      ...structuredClone(source),
      id: targetId,
      name: newName ?? `${source.name ?? id} (копия)`,
    };
    this.put(copy);
    return copy;
  }

  /**
   * Change a style's id, repointing everything that named it.
   *
   * Returns false rather than clobbering when the new id is taken — silently
   * merging two styles is the kind of edit nobody can undo by hand.
   */
  rename(id: string, newId: string): boolean {
    if (id === newId) return true;
    const style = this.byId.get(id);
    if (style === undefined || this.byId.has(newId)) return false;

    this.byId.delete(id);
    this.byId.set(newId, { ...style, id: newId });
    this.order = this.order.map((x) => (x === id ? newId : x));
    // Replaced rather than mutated: a caller still holding the object it handed
    // to `put()` must not watch a field change under it because something
    // elsewhere was renamed.
    for (const [otherId, other] of [...this.byId]) {
      if (other.basedOn === id) this.byId.set(otherId, { ...other, basedOn: newId });
    }
    this.invalidate();
    return true;
  }

  /** An id not yet taken, derived from `base`. */
  freeId(base: string): string {
    if (!this.byId.has(base)) return base;
    for (let i = 2; ; i += 1) {
      const candidate = `${base}-${i}`;
      if (!this.byId.has(candidate)) return candidate;
    }
  }

  /** Ids that name this style as their parent. */
  dependents(id: string): string[] {
    return this.order.filter((x) => this.byId.get(x)?.basedOn === id);
  }

  private invalidate(): void {
    this.blockCache.clear();
    this.edgeCache.clear();
  }
}

// ------------------------------------------------------------------- merging

function matches(style: WireStyle, needle: string): boolean {
  const haystack = [
    String(style.id ?? ""),
    String(style.name ?? ""),
    String(style.description ?? ""),
    ...(style.tags ?? []).map((t) => String(t ?? "")),
    ...(style.forKinds ?? []).map((k) => String(k ?? "")),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(needle);
}

/** `override` wins field by field; nested groups merge rather than replace. */
function mergeStyle(base: WireStyle, override: WireStyle): WireStyle {
  return {
    ...base,
    ...stripUndefined(override),
    border: mergeGroup(base.border, override.border),
    line: mergeGroup(base.line, override.line),
    title: mergeGroup(base.title, override.title),
    subtitle: mergeGroup(base.subtitle, override.subtitle),
    label: mergeGroup(base.label, override.label),
    icon: mergeGroup(base.icon, override.icon),
    source: mergeGroup(base.source, override.source),
    target: mergeGroup(base.target, override.target),
    header: mergeHeader(base.header, override.header),
  };
}

function mergeHeader(
  base: WireHeader | undefined,
  override: WireHeader | undefined,
): WireHeader | undefined {
  if (base === undefined) return override;
  if (override === undefined) return base;
  return {
    ...base,
    ...stripUndefined(override),
    text: mergeGroup(base.text, override.text),
  };
}

function mergeGroup<T extends object>(base: T | undefined, override: T | undefined): T | undefined {
  if (base === undefined) return override;
  if (override === undefined) return base;
  return { ...base, ...stripUndefined(override) };
}

/**
 * Drop keys explicitly set to `undefined`.
 *
 * Without this, `{ color: undefined }` from a half-filled editor form would
 * shadow an inherited colour with nothing — the field would look "cleared" in
 * one place and inherited in another depending on how it was typed.
 */
function stripUndefined<T extends object>(value: T): Partial<T> {
  const out: Record<string, unknown> = {};
  for (const [key, v] of Object.entries(value)) {
    if (v !== undefined) out[key] = v;
  }
  return out as Partial<T>;
}

// ------------------------------------------------------------------ building

function buildBlock(id: string, s: WireStyle): ResolvedBlockStyle {
  const base = FALLBACK_BLOCK;
  return {
    id,
    name: s.name ?? id,
    fill: paint(s.fill, base.fill),
    border: stroke(s.border, base.border),
    radius: s.radius ?? base.radius,
    shadow: s.shadow ?? base.shadow,
    header: {
      fill: paint(s.header?.fill, base.header.fill),
      height: s.header?.height ?? base.header.height,
      text: textStyle(s.header?.text, base.header.text),
    },
    title: textStyle(s.title, base.title),
    subtitle: textStyle(s.subtitle, base.subtitle),
    icon: {
      glyph: s.icon?.glyph ?? base.icon.glyph,
      show: s.icon?.show ?? base.icon.show,
    },
    shape: s.shape ?? base.shape,
    template: s.template ?? base.template,
  };
}

function buildEdge(id: string, s: WireStyle): ResolvedEdgeStyle {
  const base = FALLBACK_EDGE;
  return {
    id,
    name: s.name ?? id,
    line: stroke(s.line, base.line),
    source: endpoint(s.source, base.source),
    target: endpoint(s.target, base.target),
    label: textStyle(s.label, base.label),
    family: s.family ?? base.family,
    overview: s.overview ?? base.overview,
    routing: s.routing ?? base.routing,
  };
}

export function paint(value: WirePaint | undefined, fallback: Paint): Paint {
  if (value === undefined) return fallback;
  if (typeof value === "string") {
    return value.trim() === "" ? fallback : { kind: "solid", color: value };
  }
  const stops = (value.stops ?? []).map((s) => ({
    offset: clamp01(s.offset),
    color: s.color,
    opacity: s.opacity ?? 1,
  }));
  // A gradient with fewer than two stops is not a gradient; rather than draw a
  // blank box, fall back to something visible.
  if (stops.length < 2) return fallback;
  if (value.kind === "radial") return { kind: "radial", stops };
  return { kind: "linear", angle: value.angle ?? 90, stops };
}

function stroke(value: WireStroke | undefined, fallback: Stroke): Stroke {
  return {
    color: value?.color ?? fallback.color,
    width: value?.width ?? fallback.width,
    dash: value?.dash ?? fallback.dash,
    opacity: value?.opacity ?? fallback.opacity,
  };
}

function textStyle(value: WireText | undefined, fallback: TextStyle): TextStyle {
  return {
    family: value?.family ?? fallback.family,
    size: value?.size ?? fallback.size,
    weight: value?.weight ?? fallback.weight,
    italic: value?.italic ?? fallback.italic,
    color: value?.color ?? fallback.color,
    align: value?.align ?? fallback.align,
    opacity: value?.opacity ?? fallback.opacity,
    show: value?.show ?? fallback.show,
  };
}

function endpoint(value: WireEndpoint | undefined, fallback: Endpoint): Endpoint {
  return {
    shape: value?.shape ?? fallback.shape,
    size: value?.size ?? fallback.size,
    color: value?.color ?? fallback.color,
  };
}

function clamp01(n: number): number {
  return Number.isFinite(n) ? Math.min(1, Math.max(0, n)) : 0;
}

/** Stable key for a paint, used to name the gradient it materialises into. */
export function paintKey(p: Paint): string {
  if (p.kind === "solid") return `s:${p.color}`;
  const stops = p.stops.map((x) => `${x.offset}|${x.color}|${x.opacity}`).join(";");
  return p.kind === "linear" ? `l:${p.angle}:${stops}` : `r:${stops}`;
}
