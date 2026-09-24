/**
 * SeMaps project format types.
 *
 * Describes the single unified project structure on disk:
 * - project.json (manifest)
 * - entities.json (entities catalog)
 * - relations.json (relations catalog)
 * - text.<lang>.json (localized descriptions and names)
 * - views/<view_id>.view.json (view layouts: zones, nodes, edges)
 */

import type { RoutingMode } from "./style-types.js";
import type { ParsedTextCatalog } from "./text-provenance.js";

export interface WireMetadata {
  type?: string;
  kind?: string;
  description?: string;
  codeRef?: string;
  responsibilities?: string[];
  [key: string]: unknown;
}

export interface WireZone {
  id: string;
  name?: string;
  type?: string;
  semanticId?: string;
  tags?: string[];
  x: number;
  y: number;
  width: number;
  height: number;
  styleId?: string;
  metadata?: WireMetadata;
}

export interface WireNode {
  id: string;
  label?: string;
  type?: string;
  /** Declared parent zone id. Null, absent, or dangling all mean "not declared". */
  zone?: string | null;
  x: number;
  y: number;
  width: number;
  height: number;
  tags?: string[];
  styleId?: string;
  metadata?: WireMetadata;
}

export interface WireEdge {
  id: string;
  from: string;
  to: string;
  label?: string;
  /** Cardinality/role caption at the `from` end, e.g. "1" (ADR_20260903 §2.6). */
  fromLabel?: string;
  /** Cardinality/role caption at the `to` end, e.g. "0..*". */
  toLabel?: string;
  type?: string;
  styleId?: string;
  points?: Array<{ x: number; y: number }>;
  /**
   * Where this edge came from. A generated (`code`) edge carries no text at
   * all — not `label`, not `fromLabel`/`toLabel` — because `sync` would
   * overwrite it and it would forever read as a stale translation
   * (ADR_20260831 §2.13). Absent means "authored" for edges built by hand
   * on a view, which never had a reason to declare it.
   */
  origin?: "code" | "authored";
  /**
   * Line shape chosen for this one edge.
   *
   * Only the choice is stored. The polyline the router computes from it never
   * reaches a file: it would be stale the first time the algorithm improved,
   * and it would fill diffs with movement nobody made (ADR_20260903 §2.7).
   */
  routing?: RoutingMode;
}

export interface WireView {
  id: string;
  name?: string;
  icon?: string;
  description?: string;
  highlightZones?: string[];
  highlightNodes?: string[];
}

export interface WireDocumentMetadata {
  title?: string;
  subtitle?: string;
  layout?: string;
  description?: string;
  [key: string]: unknown;
}

export interface WireDocument {
  metadata?: WireDocumentMetadata;
  views?: WireView[];
  zones?: WireZone[];
  nodes?: WireNode[];
  edges?: WireEdge[];
  bundle?: ProjectBundle;
}

// ------------------------------------------------------------- Project Bundle

export interface EntityEntry {
  id: string;
  name: string;
  kind: string;
  origin?: "code" | "authored";
  status?: "present" | "missing" | "planned";
  namespace?: string;
  codeRef?: string;
  /**
   * Members of the type, for templates that show more than a caption.
   *
   * One list with a `kind` tag rather than separate arrays for fields,
   * properties and columns: the difference between a C# `int` and a SQL
   * `numeric(4,2)` is the *value* of `type`, not a different way of drawing a
   * row, and a template selects with `where=kind:field` (ADR_20260903 §2.4).
   *
   * Plain strings are still accepted, because that is what the field held
   * before and every existing project writes `[]`.
   */
  members?: (EntityMember | string)[];
  [key: string]: unknown;
}

export interface EntityMember {
  /** `field`, `method`, `property` — matched by a template's `where=`. */
  kind?: string;
  name: string;
  type?: string;
  visibility?: string;
  note?: string;
  [key: string]: unknown;
}

export interface EntityCatalog {
  entities: EntityEntry[];
}

export interface RelationEvidence {
  codeRef?: string;
  symbol?: string;
  line?: number;
}

export interface RelationEntry {
  id: string;
  from: string;
  to: string;
  type: string;
  relation?: string;
  label?: string;
  /** Cardinality/role captions at each end (ADR_20260903 §2.6); absent for `origin: "code"`. */
  fromLabel?: string;
  toLabel?: string;
  styleId?: string;
  origin?: "code" | "authored";
  status?: "present" | "missing";
  evidence?: RelationEvidence[];
  points?: Array<{ x: number; y: number }>;
  [key: string]: unknown;
}

export interface RelationCatalog {
  relations: RelationEntry[];
}

/**
 * In-memory text: plain strings, because that is what every consumer wants.
 * The provenance that accompanies them on disk lives in `ProjectBundle.textFiles`
 * (see `text-provenance.ts`).
 */
export interface TextCatalog {
  entries: Record<
    string,
    { name?: string; title?: string; doc?: string; description?: string; fromLabel?: string; toLabel?: string }
  >;
}

/**
 * Authored vocabulary of relation types.
 *
 * A separate file rather than entries in `entities.json` because that catalogue
 * is partly generated from code: regeneration would drown a dozen hand-written
 * lines in churn. A relation type is also never placed on a canvas, which is
 * what `entities.json` is a registry of.
 *
 * Name and description live in the text catalogues under the type's `rt_` id.
 */
export interface RelationTypeEntry {
  id: string;
  /** Generated by `sync` from code, or written by a human. */
  origin?: "code" | "authored";
  styleId?: string;
  /** Default on views for relations of this type; absent — the view's `relations.default`. */
  visibility?: "visible" | "hidden";
  [key: string]: unknown;
}

export interface RelationTypeCatalog {
  relationTypes: RelationTypeEntry[];
}

/**
 * Something wrong with the model as loaded, reported rather than thrown.
 *
 * A file that has fallen behind the contract — a view generated before axes
 * existed, a stale copy served from a cache — used to make the whole project
 * unopenable. The rule still holds (an unclassified container says nothing),
 * but it is enforced by refusing to *read* that containment and saying so,
 * not by refusing to show the diagram.
 */
export interface ModelIssue {
  kind: "view-without-axis";
  /** Human-readable, shown in the editor's banner. */
  message: string;
}

export interface ProjectManifest {
  id: string;
  title: string;
  /**
   * Axis to assume for views that do not declare one. Lets a project whose
   * views are regenerated by tooling keep its containment readable without
   * every generator having to learn about axes.
   */
  defaultAxis?: string;
  subtitle?: string;
  defaultView?: string;
  languages?: string[];
  icon?: string;
  theme?: string;
  order?: number;
  [key: string]: unknown;
}

export interface ViewZonePlacement {
  id: string;
  container?: string | null;
  parent?: string | null;
  x: number;
  y: number;
  width: number;
  height: number;
  styleId?: string;
  collapsed?: boolean;
}

export interface ViewNodePlacement {
  id?: string;
  entity?: string;
  container?: string | null;
  zone?: string | null;
  x: number;
  y: number;
  width?: number;
  height?: number;
  styleId?: string;
  /**
   * Content template for this one placement, overriding the style's choice.
   * The exception: one node that must show more, or less, than its kind does.
   */
  template?: string;
}

export interface ViewEdgePlacement {
  id: string;
  from: string;
  to: string;
  type?: string;
  relation?: string;
  label?: string;
  styleId?: string;
  points?: Array<{ x: number; y: number }>;
  /** Line shape for this edge alone; see `WireEdge.routing`. */
  routing?: RoutingMode;
}

export interface ViewDocument {
  id: string;
  project: string;
  /**
   * What this view's containers classify — `axis_layer`, `axis_project`,
   * `axis_security_zone`, ... Required: without it, a node sitting inside a
   * rectangle is unreadable, because "belongs to this assembly", "runs in this
   * process" and "moved here so the arrows don't cross" look identical.
   *
   * Two views on different axes may legitimately put one node in different
   * containers. Two views on the same axis may not — that is a contradiction,
   * and the model check reports it.
   */
  axis?: string;
  /**
   * The line shape this picture uses unless a relation type or a single edge
   * says otherwise. A convention of the drawing, not of the model.
   */
  routing?: RoutingMode;
  /** How the view's row looks in the catalogue (ADR_20260923-7). */
  icon?: string;
  theme?: string;
  order?: number;
  zones?: ViewZonePlacement[];
  nodes?: ViewNodePlacement[];
  placements?: ViewNodePlacement[];
  edges?: ViewEdgePlacement[];
  [key: string]: unknown;
}

export interface ProjectBundle {
  project: ProjectManifest;
  entities: EntityCatalog;
  relations: RelationCatalog;
  text: TextCatalog;
  textRegistries?: Record<string, TextCatalog>;
  /**
   * Provenance of each text catalogue as loaded, kept so that saving can leave
   * untouched values exactly as they were and re-stamp only what changed.
   */
  textFiles?: Record<string, ParsedTextCatalog>;
  relationTypes?: RelationTypeCatalog;
  view: ViewDocument;
  /**
   * The axis actually in force for this view: its own, or the project's default.
   * Undefined means the containment in this view carries no declared meaning and
   * must not be read as one.
   */
  resolvedAxis?: string;
  issues?: ModelIssue[];
}
