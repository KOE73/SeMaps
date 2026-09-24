import { pointOnSide } from "../../geometry/rect.js";
import type { BoundarySlot, Point, Rect, Side } from "../../geometry/types.js";
import type { ResolvedBlockStyle, TextStyle } from "../../model/StyleLibrary.js";
import type { DiagramElement } from "../../model/types.js";
import { ELEMENT_ATTR, ROLE_ATTR, Role } from "../../interaction/roles.js";
import { setAttrs, svg, text } from "../svg.js";
import type { ElementRenderer, RenderContext } from "./ElementRenderer.js";
import { alignX, dashArray, textAttrs } from "./textAttrs.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";

/** Left inset of the header caption: clear of the collapse toggle at x+8..x+28. */
const TITLE_PAD = DIAGRAM_CONFIG.container.titlePad;

/**
 * The default container renderer: a titled box with a header that doubles as
 * the drag handle, a collapse toggle, and a semantic id.
 *
 * The body is deliberately inert — only the header drags (R-EDIT-02) — so that
 * the space inside a zone stays available for panning and for grabbing the
 * elements that live in it.
 *
 * Zones used to carry their own colours inline, written into every model by the
 * Go generator; the renderer merged them over a constant table. Both are gone:
 * a zone wears a named style like everything else, and the header's height is
 * part of that style rather than a constant this file owns.
 */
export class ContainerRenderer implements ElementRenderer {
  create(el: DiagramElement, ctx: RenderContext): SVGGElement {
    const g = svg("g", { class: "semaps-zone", [ELEMENT_ATTR]: el.id });
    this.update(g, el, ctx);
    return g;
  }

  update(g: SVGGElement, el: DiagramElement, ctx: RenderContext): void {
    const style = ctx.styleOf(el);
    const collapsed = ctx.isCollapsed(el);
    const selected = ctx.isSelected(el);
    const dropTarget = ctx.dropTargetId === el.id;
    const headerHeight = style.header.height;
    const height = collapsed ? headerHeight : el.height;
    const rect: Rect = { x: el.x, y: el.y, width: el.width, height };

    setAttrs(g, {
      class: [
        "semaps-zone",
        selected ? "is-selected" : "",
        dropTarget ? "is-drop-target" : "",
        collapsed ? "is-collapsed" : "",
      ]
        .filter(Boolean)
        .join(" "),
      opacity: ctx.opacity(el),
      filter: style.shadow ? "url(#semaps-shadow)" : null,
    });
    g.replaceChildren();

    g.appendChild(
      svg("rect", {
        [ROLE_ATTR]: Role.Body,
        class: "semaps-zone-body",
        x: el.x,
        y: el.y,
        width: el.width,
        height,
        rx: style.radius,
        fill: ctx.paints.fill(style.fill),
        stroke: style.border.color,
        "stroke-width": style.border.width,
        style: `--sw: ${style.border.width}`,
        // Collapsed, the box *is* the header: a dashed outline round a title
        // bar reads as a placeholder rather than as a folded zone.
        "stroke-dasharray": collapsed ? null : dashArray(style.border.dash),
        "stroke-opacity": style.border.opacity === 1 ? null : style.border.opacity,
      }),
    );

    g.appendChild(
      svg("rect", {
        [ROLE_ATTR]: Role.DragHandle,
        class: "semaps-zone-header",
        x: el.x,
        y: el.y,
        width: el.width,
        height: headerHeight,
        rx: style.radius,
        fill: ctx.paints.fill(style.header.fill),
      }),
    );

    g.appendChild(this.docToggle(el, ctx));
    g.appendChild(this.collapseToggle(el, collapsed));

    const titleStyle = style.header.text;
    if (titleStyle.show) {
      const childCount = el.children.filter((c) => c.kind === "node").length;
      const icon = style.icon.show ? `${style.icon.glyph} ` : "";
      g.appendChild(
        text(
          {
            ...textAttrs(titleStyle),
            ...alignX(titleStyle, rect, TITLE_PAD),
            y: baseline(el.y, headerHeight, titleStyle),
            class: "semaps-zone-title",
          },
          collapsed
            ? `${icon}${el.label} (${childCount} компонентов)`
            : `${icon}${el.label}`,
        ),
      );
    }

    if (style.subtitle.show && el.semanticId !== undefined && el.semanticId !== "") {
      g.appendChild(
        text(
          {
            ...textAttrs(style.subtitle),
            ...alignX(style.subtitle, rect, 34),
            y: baseline(el.y, headerHeight, style.subtitle),
            class: "semaps-zone-semantic",
          },
          el.semanticId,
        ),
      );
    }
  }

  private docToggle(el: DiagramElement, ctx: RenderContext): SVGGElement {
    const textEntry = ctx.doc ? ctx.doc.getText(el.id, "ru") || ctx.doc.getText(el.id, "en") : undefined;
    const hasDoc = Boolean(textEntry?.doc?.trim());
    return svg(
      "g",
      { [ROLE_ATTR]: Role.DocEdit, class: `semaps-zone-doc${hasDoc ? " has-doc" : ""}`, style: "cursor: pointer;" },
      [
        svg("rect", { x: el.x + 8, y: el.y + 7, width: 20, height: 20, rx: 5, class: "semaps-zone-doc-rect" }),
        text(
          {
            x: el.x + 18,
            y: el.y + 21,
            "font-size": 11,
            "text-anchor": "middle",
            class: "semaps-zone-doc-icon",
            "pointer-events": "none",
          },
          hasDoc ? "📝" : "📄",
        ),
      ],
    );
  }

  private collapseToggle(el: DiagramElement, collapsed: boolean): SVGGElement {
    const rx = el.x + el.width - 28;
    return svg(
      "g",
      { [ROLE_ATTR]: Role.CollapseToggle, class: "semaps-zone-collapse", style: "cursor: pointer;" },
      [
        svg("rect", { x: rx, y: el.y + 7, width: 20, height: 20, rx: 5, class: "semaps-zone-collapse-rect" }),
        text(
          {
            x: rx + 10,
            y: el.y + 22,
            "font-size": 14,
            "text-anchor": "middle",
            "pointer-events": "none",
          },
          collapsed ? "+" : "−",
        ),
      ],
    );
  }

  visibleRect(el: DiagramElement, ctx: RenderContext): Rect {
    return {
      x: el.x,
      y: el.y,
      width: el.width,
      // A collapsed container occupies exactly its header, so this has to be
      // the same number the header is drawn with — hence the style, not the
      // fallback constant (D-01 all over again if they disagree).
      height: ctx.isCollapsed(el) ? ctx.styleOf(el).header.height : el.height,
    };
  }

  pointAt(rect: Rect, slot: BoundarySlot): Point {
    return pointOnSide(rect, slot.side, slot.t);
  }

  cornerInset(_side: Side, style?: ResolvedBlockStyle): number {
    return (style?.radius ?? DIAGRAM_CONFIG.container.defaultRadius) + DIAGRAM_CONFIG.ports.extraCornerGap;
  }
}

/**
 * Baseline that keeps a caption optically centred in the header band.
 *
 * Derived rather than hardcoded so that a style raising `header.height` moves
 * the title with it; the old constants (22 for the title, 21 for the id) were
 * only correct for a 34-high header.
 */
function baseline(top: number, headerHeight: number, style: TextStyle): number {
  return top + headerHeight / 2 + style.size * 0.35;
}
