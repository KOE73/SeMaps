import type { BoundarySlot, Point, Rect, Side } from "../../geometry/types.js";
import type { DiagramDocument } from "../../model/document.js";
import type { ResolvedBlockStyle } from "../../model/StyleLibrary.js";
import type { DiagramElement } from "../../model/types.js";
import type { ContentData } from "../../content/ContentRenderer.js";
import type { TemplateTree } from "../../content/template-types.js";
import type { PaintRegistry } from "./PaintRegistry.js";

/**
 * What a renderer is told about the world outside the element it draws.
 * Presentation state only — never the model's own data.
 */
export interface RenderContext {
  readonly doc: DiagramDocument;
  /** The element that drives the inspector when several are selected. */
  readonly selectedId: string | null;
  readonly dropTargetId: string | null;
  readonly ghostNodeId?: string | null;
  isSelected(el: DiagramElement): boolean;
  isCollapsed(el: DiagramElement): boolean;
  /** View highlighting, 0..1 (R-VIEW-03/04). */
  opacity(el: DiagramElement): number;
  /** Whether the element is hidden because an ancestor is collapsed. */
  isHidden(el: DiagramElement): boolean;

  /**
   * The element's look, fully resolved: no optional fields, no inheritance
   * left to chase, no lookup for the renderer to get wrong.
   *
   * A renderer must not reach for the style library itself. Resolution is one
   * decision — styleId, then type, then the per-kind default — and it lives in
   * one place so that "why is this box grey" has one answer.
   */
  styleOf(el: DiagramElement): ResolvedBlockStyle;

  /**
   * Turns a resolved paint or arrow head into something an SVG attribute can
   * hold, materialising gradients and markers into `<defs>` on the way.
   */
  readonly paints: PaintRegistry;

  /**
   * The content template chosen for this element, already compiled, together
   * with the data its directives read.
   *
   * Null means "no template" — draw the built-in caption and subtitle, which
   * is what every node looked like before templates existed. A renderer must
   * not reach for the template library itself, for the same reason it must not
   * reach for the style library: resolution (placement, then style, then
   * nothing) is one decision and lives in one place.
   */
  content(el: DiagramElement): ResolvedContent | null;
}

/** A compiled template plus the element's own data, ready to draw. */
export interface ResolvedContent {
  readonly tree: TemplateTree;
  readonly data: ContentData;
}

/**
 * Where a shape puts the furniture that is not the shape: the doc and code
 * buttons, the relation badge, and the region text is allowed to occupy.
 *
 * A rectangle can take all of it at fixed offsets from its corners, which is
 * why the original code had those offsets baked in. A diamond cannot: its
 * corners are points, and a button placed at "x + 6" lands outside the
 * silhouette entirely. So the offsets move to the shape, which is the only
 * thing that knows where its own inside is.
 *
 * Growth direction matters as much as the anchor. Buttons accumulate — doc,
 * then code, then whatever comes next — and near a diamond's apex the only
 * room is *away* from it: left for one group, right for the other.
 */
export interface ChromeLayout {
  /** Corner the first button occupies. */
  readonly docAnchor: Point;
  /** Which way further buttons stack from `docAnchor`. */
  readonly docGrow: "left" | "right";
  /** Edge the relation badge is pinned to. */
  readonly badgeAnchor: Point;
  /** Which way the badge extends from `badgeAnchor`. */
  readonly badgeGrow: "left" | "right";
  /** Region captions and template content are laid out in. */
  readonly textBox: Rect;
  /** Horizontal breathing room inside `textBox`. */
  readonly padX: number;
  /** Baseline offset of the built-in caption from `textBox.y`. */
  readonly captionTop: number;
  /** Top offset for template content, which brings its own first line. */
  readonly contentTop: number;
}

/**
 * How one kind of thing looks and where lines attach to it.
 *
 * The containment tree is pure geometry; a renderer decides everything visual
 * about its own type and nothing about anyone else's. Adding a new type means
 * registering a renderer — no change anywhere else in the library.
 */
export interface ElementRenderer {
  /** Build the element's group from scratch. */
  create(el: DiagramElement, ctx: RenderContext): SVGGElement;

  /**
   * Bring an existing group up to date.
   *
   * Today every renderer may simply rebuild — that is what the original code
   * did on every frame. The method exists so that incremental rendering can be
   * added later inside renderers, without touching the code that calls them.
   */
  update(g: SVGGElement, el: DiagramElement, ctx: RenderContext): void;

  /**
   * Optional decoration drawn above every element rather than inside this
   * one's group — a resize grip that must stay grabbable even where a
   * neighbouring element overlaps this one's corner (R-REND-01).
   *
   * Returning null, or omitting the method, means the renderer needs none.
   */
  overlay?(el: DiagramElement, ctx: RenderContext): SVGGElement | null;

  /**
   * The rectangle the element actually occupies on screen, which is not always
   * its model rectangle — a collapsed container draws only its header.
   */
  visibleRect(el: DiagramElement, ctx: RenderContext): Rect;

  /**
   * Where a boundary slot lands on this shape.
   *
   * Port assignment works in shape-independent terms ("east side, 30% along");
   * turning that into a point is the shape's job. A box interpolates along an
   * edge, an ellipse walks an arc — same assignment algorithm, both correct.
   */
  pointAt(rect: Rect, slot: BoundarySlot): Point;

  /**
   * Safe corner inset (px) for this shape on the given side.
   * Tells port assigners and routers where the straight segment ends and the corner
   * or curvature begins.
   */
  cornerInset?(side: Side, style?: ResolvedBlockStyle): number;
}
