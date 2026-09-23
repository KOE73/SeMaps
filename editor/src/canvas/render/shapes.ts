import { bottom, center, pointOnSide, right } from "../../geometry/rect.js";
import type { BoundarySlot, Point, Rect, Side } from "../../geometry/types.js";
import type { ResolvedBlockStyle } from "../../model/StyleLibrary.js";
import { elementRect, type DiagramElement } from "../../model/types.js";
import { ROLE_ATTR, Role } from "../../interaction/roles.js";
import { svg } from "../svg.js";
import { BoxRenderer } from "./BoxRenderer.js";
import type { ChromeLayout, RenderContext } from "./ElementRenderer.js";

/**
 * Node outlines other than a rectangle.
 *
 * Each one is a `BoxRenderer` with a different outline and a different answer
 * to "where does a line attach": the chrome inside (caption, doc button, link
 * badge) is inherited untouched, because it is the same furniture whatever the
 * silhouette. Adding a shape means adding a subclass here and registering it —
 * the router, the port assigners and the canvas learn nothing about it
 * (ADR_20260901).
 *
 * A shape is chosen by the element's style, not set on each node by hand: a
 * use case is an ellipse because of what it *is*, and saying so once in
 * `styles.json` keeps a hundred nodes consistent.
 */

/**
 * Where a ray from the centre through a box-boundary point leaves a curved or
 * angled outline.
 *
 * Slots arrive in shape-independent terms ("east side, 30% along"). Walking the
 * ray keeps the assigner's ordering and spacing intact while landing the point
 * on the real silhouette, so ports stay evenly spread and never float off it.
 * `radiusAt` returns how far the outline is along that direction, expressed as
 * a scale of the direction vector.
 */
function castFromCentre(
  rect: Rect,
  slot: BoundarySlot,
  scaleFor: (dx: number, dy: number, a: number, b: number) => number,
): Point {
  const c = center(rect);
  const onBox = pointOnSide(rect, slot.side, slot.t);
  const dx = onBox.x - c.x;
  const dy = onBox.y - c.y;
  if (dx === 0 && dy === 0) return c;

  const s = scaleFor(dx, dy, Math.max(rect.width / 2, 0.001), Math.max(rect.height / 2, 0.001));
  return { x: c.x + dx * s, y: c.y + dy * s };
}

/**
 * A shape with no straight run at all needs its ports kept away from the ends
 * of the nominal side, where the outline curves hardest and an arrow would
 * appear to miss it. The inset is a constant because the contract hands the
 * renderer no rectangle to measure (`cornerInset(side, style)`).
 */
const CURVED_INSET = 22;

/**
 * The largest axis-aligned rectangle that fits inside an ellipse of this box.
 *
 * Text placed on the bounding box of a round shape runs out through the sides
 * near the top and bottom, where the outline has already curved away. Half the
 * diagonal is the honest usable width, and it is one multiplication.
 */
function inscribedBox(rect: Rect, factor = Math.SQRT1_2): Rect {
  const w = rect.width * factor;
  const h = rect.height * factor;
  return {
    x: rect.x + (rect.width - w) / 2,
    y: rect.y + (rect.height - h) / 2,
    width: w,
    height: h,
  };
}

/** Use cases, states, anything round. */
export class EllipseRenderer extends BoxRenderer {
  protected override outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    return svg("ellipse", {
      [ROLE_ATTR]: Role.DragHandle,
      cx: el.x + el.width / 2,
      cy: el.y + el.height / 2,
      rx: el.width / 2,
      ry: el.height / 2,
      ...this.paintAttrs(style, ctx),
    });
  }

  override pointAt(rect: Rect, slot: BoundarySlot): Point {
    // (x/a)² + (y/b)² = 1 solved along the ray: one division, exact.
    return castFromCentre(rect, slot, (dx, dy, a, b) =>
      1 / Math.hypot(dx / a, dy / b),
    );
  }

  override cornerInset(): number {
    return CURVED_INSET;
  }

  protected override chrome(el: DiagramElement, style: ResolvedBlockStyle): ChromeLayout {
    const box = inscribedBox(elementRect(el));
    return {
      ...super.chrome(el, style),
      // Buttons ride the inscribed box's top edge: on the bounding box they
      // would hang over empty canvas beside the curve.
      docAnchor: { x: box.x, y: box.y - 2 },
      docGrow: "right",
      badgeAnchor: { x: box.x + box.width, y: box.y - 2 },
      badgeGrow: "left",
      textBox: box,
      padX: 6,
      captionTop: box.height / 2 + 4,
      contentTop: 16,
    };
  }
}

/** Decisions, forks. */
export class DiamondRenderer extends BoxRenderer {
  protected override outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    const cx = el.x + el.width / 2;
    const cy = el.y + el.height / 2;
    return svg("path", {
      [ROLE_ATTR]: Role.DragHandle,
      d: `M ${cx} ${el.y} L ${right(el)} ${cy} L ${cx} ${bottom(el)} L ${el.x} ${cy} Z`,
      ...this.paintAttrs(style, ctx),
    });
  }

  override pointAt(rect: Rect, slot: BoundarySlot): Point {
    // |x|/a + |y|/b = 1 — the diamond's edges are straight, so this is exact.
    return castFromCentre(rect, slot, (dx, dy, a, b) =>
      1 / (Math.abs(dx) / a + Math.abs(dy) / b),
    );
  }

  override cornerInset(): number {
    return CURVED_INSET;
  }

  protected override chrome(el: DiagramElement, style: ResolvedBlockStyle): ChromeLayout {
    const cx = el.x + el.width / 2;
    // A diamond has no corner to tuck a button into — the top is a single
    // point. So the two groups straddle that apex and grow away from each
    // other, which keeps them on the outline's shoulders instead of over the
    // empty triangles beside it, and leaves room for more buttons later.
    return {
      ...super.chrome(el, style),
      docAnchor: { x: cx - 6, y: el.y + 4 },
      docGrow: "left",
      badgeAnchor: { x: cx + 6, y: el.y + 4 },
      badgeGrow: "right",
      textBox: inscribedBox(elementRect(el), 0.62),
      padX: 4,
      captionTop: el.height * 0.19 + 4,
      contentTop: 14,
    };
  }
}

/** Stores: a database drawn the way every database has been drawn since 1975. */
export class CylinderRenderer extends BoxRenderer {
  /** Half-height of the elliptical cap, capped so short boxes stay readable. */
  private capHeight(el: DiagramElement): number {
    return Math.min(el.height * 0.18, 14);
  }

  protected override outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    const cap = this.capHeight(el);
    const r = right(el);
    const b = bottom(el);
    const paint = this.paintAttrs(style, ctx);

    // Body and lid are one group so the whole silhouette drags as one thing.
    return svg("g", { [ROLE_ATTR]: Role.DragHandle }, [
      svg("path", {
        d:
          `M ${el.x} ${el.y + cap} ` +
          `A ${el.width / 2} ${cap} 0 0 1 ${r} ${el.y + cap} ` +
          `L ${r} ${b - cap} ` +
          `A ${el.width / 2} ${cap} 0 0 1 ${el.x} ${b - cap} Z`,
        ...paint,
      }),
      svg("path", {
        d: `M ${el.x} ${el.y + cap} A ${el.width / 2} ${cap} 0 0 0 ${r} ${el.y + cap}`,
        ...paint,
        fill: "none",
      }),
    ]);
  }

  override cornerInset(_side: Side): number {
    // The sides are straight; only the caps bulge, and the inset that keeps an
    // arrow off them is about the cap's own height.
    return 16;
  }

  protected override chrome(el: DiagramElement, style: ResolvedBlockStyle): ChromeLayout {
    // Everything clears the lid: a button drawn at the top of the bounding box
    // sits on the cap's arc, where it reads as part of the drum.
    const cap = this.capHeight(el);
    const top = el.y + cap * 1.8;
    return {
      ...super.chrome(el, style),
      docAnchor: { x: el.x + 8, y: top },
      badgeAnchor: { x: el.x + el.width - 8, y: top },
      textBox: { x: el.x, y: top, width: el.width, height: el.height - cap * 2.8 },
      captionTop: 20,
      contentTop: 14,
    };
  }
}

/** Anything that wants to read as a step or a stage rather than a thing. */
export class HexagonRenderer extends BoxRenderer {
  protected override outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    const notch = Math.min(el.width * 0.15, 24);
    const cy = el.y + el.height / 2;
    const r = right(el);
    const b = bottom(el);
    return svg("path", {
      [ROLE_ATTR]: Role.DragHandle,
      d:
        `M ${el.x + notch} ${el.y} L ${r - notch} ${el.y} L ${r} ${cy} ` +
        `L ${r - notch} ${b} L ${el.x + notch} ${b} L ${el.x} ${cy} Z`,
      ...this.paintAttrs(style, ctx),
    });
  }

  override cornerInset(side: Side): number {
    // North and south lose their ends to the notches; east and west are points.
    return side === "east" || side === "west" ? CURVED_INSET : 26;
  }

  protected override chrome(el: DiagramElement, style: ResolvedBlockStyle): ChromeLayout {
    // The slanted ends eat the top corners, so both groups step inside by the
    // notch — otherwise a button straddles the diagonal edge, which is what
    // made the "Компиляция" node's controls sit on its own outline.
    const notch = Math.min(el.width * 0.15, 24);
    return {
      ...super.chrome(el, style),
      docAnchor: { x: el.x + notch + 4, y: el.y + 5 },
      badgeAnchor: { x: el.x + el.width - notch - 4, y: el.y + 5 },
      textBox: { x: el.x + notch, y: el.y, width: el.width - notch * 2, height: el.height },
      padX: 6,
    };
  }
}

/** People and external roles: the UML actor, drawn rather than approximated. */
export class ActorRenderer extends BoxRenderer {
  /** Room kept at the bottom for the name. */
  private static readonly CAPTION_STRIP = 22;

  protected override outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    const cx = el.x + el.width / 2;
    const headR = Math.min(el.width, el.height - ActorRenderer.CAPTION_STRIP) * 0.16;
    const headY = el.y + headR + 4;
    const shoulder = headY + headR + 6;
    const hip = el.y + el.height * 0.62;
    const armSpan = Math.min(el.width * 0.32, 34);
    const legSpan = Math.min(el.width * 0.26, 28);
    // The feet stop above the caption strip: an actor's name belongs under the
    // figure, the way every UML diagram since 1997 has drawn it, and the name
    // used to land across the head instead.
    const feet = bottom(el) - ActorRenderer.CAPTION_STRIP - 4;
    const paint = this.paintAttrs(style, ctx);
    const strokeOnly = { ...paint, fill: "none" };

    return svg("g", { [ROLE_ATTR]: Role.DragHandle }, [
      // An invisible pad so the figure is grabbable in the gaps between limbs;
      // without it the drag target is a few one-pixel strokes.
      svg("rect", {
        x: el.x, y: el.y, width: el.width, height: el.height,
        fill: "transparent", stroke: "none",
      }),
      svg("circle", { cx, cy: headY, r: headR, ...paint }),
      svg("path", {
        d:
          `M ${cx} ${shoulder} L ${cx} ${hip} ` +
          `M ${cx - armSpan} ${shoulder + 10} L ${cx + armSpan} ${shoulder + 10} ` +
          `M ${cx} ${hip} L ${cx - legSpan} ${feet} ` +
          `M ${cx} ${hip} L ${cx + legSpan} ${feet}`,
        ...strokeOnly,
        "stroke-linecap": "round",
      }),
    ]);
  }

  override cornerInset(): number {
    // The figure floats inside its box, so lines may attach anywhere on it.
    return 0;
  }

  protected override chrome(el: DiagramElement, style: ResolvedBlockStyle): ChromeLayout {
    const strip = ActorRenderer.CAPTION_STRIP;
    return {
      ...super.chrome(el, style),
      textBox: { x: el.x, y: bottom(el) - strip, width: el.width, height: strip },
      padX: 2,
      captionTop: 12,
      contentTop: 4,
    };
  }
}
