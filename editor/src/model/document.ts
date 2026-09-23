import { area, containsPoint, containsRect, center } from "../geometry/rect.js";
import type { Point, Rect } from "../geometry/types.js";
import type {
  DiagramEdge,
  DiagramElement,
  DiagramMetadata,
  DiagramView,
} from "./types.js";
import type { EntityEntry, ProjectBundle, RelationEntry } from "./wire-types.js";
import { elementRect, isContainer } from "./types.js";

/**
 * The loaded diagram: a containment tree of elements, a flat list of edges that
 * connect leaf elements, and the view presets.
 *
 * Nothing here knows about SVG, the DOM, or the JSON contract.
 */
export class DiagramDocument {
  readonly metadata: DiagramMetadata;
  readonly views: DiagramView[];
  readonly roots: DiagramElement[];
  readonly edges: DiagramEdge[];

  /**
   * The document this model was parsed from, so that top-level keys the
   * library does not model — `$schema`, `version`, anything a generator adds —
   * survive a load/save round trip.
   */
  readonly raw: Readonly<Record<string, unknown>>;

  private readonly byId = new Map<string, DiagramElement>();

  constructor(init: {
    metadata: DiagramMetadata;
    views: DiagramView[];
    roots: DiagramElement[];
    edges: DiagramEdge[];
    raw?: Readonly<Record<string, unknown>>;
  }) {
    this.metadata = init.metadata;
    this.views = init.views;
    this.roots = init.roots;
    this.edges = init.edges;
    this.raw = init.raw ?? {};
    this.reindex();
  }

  private reindex(): void {
    this.byId.clear();
    for (const el of this.elements()) {
      this.byId.set(el.id, el);
    }
  }

  get bundle(): ProjectBundle | null {
    return ((this.raw as any)?.bundle ?? null) as ProjectBundle | null;
  }

  get entities(): EntityEntry[] {
    const raw = this.bundle?.entities;
    if (Array.isArray(raw)) return raw;
    if (raw && Array.isArray((raw as any).entities)) return (raw as any).entities;
    return [];
  }

  get relations(): RelationEntry[] {
    const raw = this.bundle?.relations;
    if (Array.isArray(raw)) return raw;
    if (raw && Array.isArray((raw as any).relations)) return (raw as any).relations;
    return [];
  }

  getText(
    id: string,
    lang = "ru",
  ): { name?: string; title?: string; doc?: string; description?: string; fromLabel?: string; toLabel?: string } | undefined {
    const bundle = this.bundle;
    if (!bundle) return undefined;
    if (bundle.textRegistries && bundle.textRegistries[lang]?.entries?.[id]) {
      return bundle.textRegistries[lang].entries[id];
    }
    if (bundle.text?.entries?.[id]) {
      return bundle.text.entries[id];
    }
    return undefined;
  }

  setText(
    id: string,
    entry: { name?: string; title?: string; doc?: string; description?: string; fromLabel?: string; toLabel?: string },
    lang = "ru",
  ): void {
    const bundle = this.bundle;
    if (!bundle) return;
    if (!bundle.textRegistries) {
      bundle.textRegistries = {
        [lang]: bundle.text || { entries: {} },
      };
    }
    if (!bundle.textRegistries[lang]) {
      bundle.textRegistries[lang] = { entries: {} };
    }
    bundle.textRegistries[lang].entries[id] = {
      ...bundle.textRegistries[lang].entries[id],
      ...entry,
    };
    if (lang === "ru" || !bundle.text) {
      bundle.text = bundle.textRegistries[lang];
    }
  }

  // ---------------------------------------------------------------- queries

  /** Every element, parents before children, in stable document order. */
  *elements(): Generator<DiagramElement> {
    const walk = function* (list: readonly DiagramElement[]): Generator<DiagramElement> {
      for (const el of list) {
        yield el;
        yield* walk(el.children);
      }
    };
    yield* walk(this.roots);
  }

  element(id: string): DiagramElement | undefined {
    return this.byId.get(id);
  }

  edge(id: string): DiagramEdge | undefined {
    return this.edges.find((e) => e.id === id);
  }

  containers(): DiagramElement[] {
    return [...this.elements()].filter(isContainer);
  }

  leaves(): DiagramElement[] {
    return [...this.elements()].filter((el) => !isContainer(el));
  }

  /** Ancestors from the immediate parent up to the root. */
  ancestors(el: DiagramElement): DiagramElement[] {
    const out: DiagramElement[] = [];
    for (let p = el.parent; p !== null; p = p.parent) out.push(p);
    return out;
  }

  descendants(el: DiagramElement): DiagramElement[] {
    const out: DiagramElement[] = [];
    const walk = (list: readonly DiagramElement[]): void => {
      for (const child of list) {
        out.push(child);
        walk(child.children);
      }
    };
    walk(el.children);
    return out;
  }

  /** True when `maybeAncestor` is `el` itself or any ancestor of it. */
  contains(maybeAncestor: DiagramElement, el: DiagramElement): boolean {
    for (let p: DiagramElement | null = el; p !== null; p = p.parent) {
      if (p === maybeAncestor) return true;
    }
    return false;
  }

  edgesOf(elementId: string): DiagramEdge[] {
    return this.edges.filter((e) => e.from === elementId || e.to === elementId);
  }

  outgoingEdges(elementId: string): DiagramEdge[] {
    return this.edges.filter((e) => e.from === elementId);
  }

  /** Bounding box of everything, or null when the document is empty. */
  bounds(): Rect | null {
    let min: Point | null = null;
    let max: Point | null = null;
    for (const el of this.elements()) {
      const r = elementRect(el);
      min = min === null ? { x: r.x, y: r.y } : { x: Math.min(min.x, r.x), y: Math.min(min.y, r.y) };
      const rx = r.x + r.width;
      const ry = r.y + r.height;
      max = max === null ? { x: rx, y: ry } : { x: Math.max(max.x, rx), y: Math.max(max.y, ry) };
    }
    if (min === null || max === null) return null;
    return { x: min.x, y: min.y, width: max.x - min.x, height: max.y - min.y };
  }

  // ------------------------------------------------------------ containment

  /**
   * The innermost container whose rectangle holds `point`, ignoring any
   * container in `exclude` and its descendants.
   *
   * "Innermost" is decided by area, not by array order. That is the fix for
   * D-04: under the old first-match scan the winner depended on where the zone
   * happened to sit in the JSON array.
   */
  containerAt(point: Point, exclude?: DiagramElement | null): DiagramElement | null {
    let best: DiagramElement | null = null;
    for (const el of this.elements()) {
      if (!isContainer(el)) continue;
      if (exclude && this.contains(exclude, el)) continue;
      if (!containsPoint(elementRect(el), point)) continue;
      if (best === null || area(elementRect(el)) < area(elementRect(best))) {
        best = el;
      }
    }
    return best;
  }

  /**
   * The innermost container that fully encloses `rect`. Used when rebuilding
   * the tree from geometry (R-CONT-02).
   */
  containerEnclosing(rect: Rect, exclude?: DiagramElement | null): DiagramElement | null {
    let best: DiagramElement | null = null;
    for (const el of this.elements()) {
      if (!isContainer(el)) continue;
      if (exclude && this.contains(exclude, el)) continue;
      if (!containsRect(elementRect(el), rect)) continue;
      if (best === null || area(elementRect(el)) < area(elementRect(best))) {
        best = el;
      }
    }
    return best;
  }

  // -------------------------------------------------------------- mutation

  /**
   * Move `el` under `parent` (or to the root when null), keeping its absolute
   * coordinates. Refuses to create a cycle.
   */
  reparent(el: DiagramElement, parent: DiagramElement | null): boolean {
    if (parent !== null && (parent === el || this.contains(el, parent))) return false;
    if (el.parent === parent) return false;

    this.detach(el);
    if (parent === null) {
      this.roots.push(el);
    } else {
      parent.children.push(el);
    }
    el.parent = parent;
    return true;
  }

  private detach(el: DiagramElement): void {
    const siblings = el.parent === null ? this.roots : el.parent.children;
    const at = siblings.indexOf(el);
    if (at >= 0) siblings.splice(at, 1);
  }

  add(el: DiagramElement, parent: DiagramElement | null): void {
    el.parent = parent;
    if (parent === null) this.roots.push(el);
    else parent.children.push(el);
    this.byId.set(el.id, el);
  }

  /**
   * Remove an element. Its children are re-attached to its parent rather than
   * deleted — deleting a zone must not delete the components inside it
   * (R-CRUD-05). Edges touching removed leaves go with them (R-CRUD-04).
   */
  remove(el: DiagramElement): void {
    for (const child of [...el.children]) {
      this.reparent(child, el.parent);
    }
    this.detach(el);
    this.byId.delete(el.id);

    const survives = (e: DiagramEdge): boolean => e.from !== el.id && e.to !== el.id;
    for (let i = this.edges.length - 1; i >= 0; i--) {
      const e = this.edges[i];
      if (e && !survives(e)) this.edges.splice(i, 1);
    }
  }

  removeEdge(edgeId: string): void {
    const at = this.edges.findIndex((e) => e.id === edgeId);
    if (at >= 0) this.edges.splice(at, 1);
  }

  addEdge(edge: DiagramEdge): void {
    this.edges.push(edge);
  }

  /** Centre of an element, in model coordinates. */
  centerOf(el: DiagramElement): Point {
    return center(elementRect(el));
  }
}
