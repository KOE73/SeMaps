import { pointOnSide } from "../../geometry/rect.js";
import type { BoundarySlot, Point, Rect, Side } from "../../geometry/types.js";
import type { DiagramElement } from "../../model/types.js";
import type { ResolvedBlockStyle } from "../../model/StyleLibrary.js";
import { elementRect } from "../../model/types.js";
import { ELEMENT_ATTR, ROLE_ATTR, Role } from "../../interaction/roles.js";
import { setAttrs, svg, text } from "../svg.js";
import type { ChromeLayout, ElementRenderer, RenderContext } from "./ElementRenderer.js";
import { alignX, dashArray, textAttrs } from "./textAttrs.js";
import { resolveElementRelations } from "../../model/relations-resolver.js";
import { renderContent } from "../../content/ContentRenderer.js";
import { SourceCodeService } from "../../editor/code/SourceCodeService.js";
import { DIAGRAM_CONFIG } from "../../constants/diagram-constants.js";

/**
 * The default leaf renderer: a rounded rectangle with a caption, a subtitle and
 * nothing else. Covers every node type in the current models — they differ only
 * in the style they resolve to, which this renderer reads and does not choose.
 */
const NODE_LAYOUT = DIAGRAM_CONFIG.node;

export class BoxRenderer implements ElementRenderer {
  create(el: DiagramElement, ctx: RenderContext): SVGGElement {
    const g = svg("g", {
      class: "semaps-node",
      [ELEMENT_ATTR]: el.id,
    });
    this.update(g, el, ctx);
    return g;
  }

  update(g: SVGGElement, el: DiagramElement, ctx: RenderContext): void {
    const style = ctx.styleOf(el);
    const selected = ctx.isSelected(el);

    setAttrs(g, {
      class: `semaps-node${selected ? " is-selected" : ""}`,
      opacity: ctx.opacity(el),
      filter: style.shadow ? "url(#semaps-shadow)" : null,
    });
    g.replaceChildren();

    // 1. Background / Drag handle
    g.appendChild(this.outline(el, style, ctx));

    // 2. Top Bar Zone: Controls (Doc button on left, Code button, Relations count on right)
    const chrome = this.chrome(el, style);
    const textEntry = ctx.doc ? ctx.doc.getText(el.id, "ru") || ctx.doc.getText(el.id, "en") : undefined;
    const hasDoc = Boolean(textEntry?.doc?.trim());
    // Buttons are laid out from an anchor plus a direction, so that a shape
    // with no room to the right of its anchor can stack them the other way.
    const docWidth = NODE_LAYOUT.docButtonWidth;
    const docX = chrome.docGrow === "right" ? chrome.docAnchor.x : chrome.docAnchor.x - docWidth;
    const docY = chrome.docAnchor.y;

    const docGroup = svg("g", {
      class: `semaps-node-doc${hasDoc ? " has-doc" : ""}`,
      [ROLE_ATTR]: Role.DocEdit,
      style: "cursor: pointer;",
    }, [
      svg("rect", {
        x: docX,
        y: docY,
        width: NODE_LAYOUT.docButtonWidth,
        height: NODE_LAYOUT.topBarHeight,
        rx: 4,
        class: "semaps-node-doc-rect",
      }),
      text(
        {
          x: docX + 9,
          y: docY + 10,
          "text-anchor": "middle",
          "font-size": "9px",
          class: "semaps-node-doc-icon",
          "pointer-events": "none",
        },
        hasDoc ? "📝" : "📄",
      ),
    ]);
    g.appendChild(docGroup);

    // 2b. Code button if codeRef is present and available
    const codeRef = typeof el.metadata?.codeRef === "string" ? el.metadata.codeRef.trim() : "";
    const isAvailable = codeRef ? SourceCodeService.isFileAvailable(codeRef) : false;
    if (codeRef && isAvailable !== false) {
      const step = NODE_LAYOUT.docButtonWidth + 4;
      const codeX = chrome.docGrow === "right" ? docX + step : docX - step;
      const codeGroup = svg("g", {
        class: "semaps-node-code",
        [ROLE_ATTR]: Role.CodeView,
        style: "cursor: pointer;",
      }, [
        svg("rect", {
          x: codeX,
          y: docY,
          width: NODE_LAYOUT.codeButtonWidth,
          height: NODE_LAYOUT.topBarHeight,
          rx: 4,
          class: "semaps-node-code-rect",
        }),
        text(
          {
            x: codeX + 9,
            y: docY + 10,
            "text-anchor": "middle",
            "font-size": "9px",
            class: "semaps-node-code-icon",
            "pointer-events": "none",
          },
          "💻",
        ),
      ]);
      g.appendChild(codeGroup);
    }

    // Link count badge in the top-right corner
    const relSummary = resolveElementRelations(ctx.doc, el);
    const visibleCount = relSummary.visible;
    const total = relSummary.total;

    if (total > 0) {
      const isGhost = ctx.ghostNodeId === el.id;
      const badgeText = `${total}/${visibleCount}`;
      const bw = Math.max(22, badgeText.length * 6 + 8);
      const bx = chrome.badgeGrow === "left" ? chrome.badgeAnchor.x - bw : chrome.badgeAnchor.x;
      const by = chrome.badgeAnchor.y;

      const badgeGroup = svg("g", {
        class: `semaps-node-badge${isGhost ? " is-active" : ""}`,
        [ROLE_ATTR]: Role.GhostToggle,
        style: "cursor: pointer;",
      }, [
        svg("rect", {
          x: bx,
          y: by,
          width: bw,
          height: NODE_LAYOUT.topBarHeight,
          rx: 7,
          class: "semaps-node-badge-rect",
        }),
        text(
          {
            x: bx + bw / 2,
            y: by + 10,
            "text-anchor": "middle",
            "font-size": "9px",
            "font-family": "monospace",
            "font-weight": "700",
            class: "semaps-node-badge-text",
            "pointer-events": "none",
          },
          badgeText,
        ),
      ]);
      g.appendChild(badgeGroup);
    }

    // 3. Interior. A template, when the style or the placement names one,
    // replaces the built-in caption and subtitle wholesale — it is the same
    // region of the block, drawn from text someone wrote instead of from two
    // hardcoded lines. Without one, nothing below changes.
    const content = ctx.content(el);
    if (content !== null) {
      const drawn = renderContent(content.tree, chrome.textBox, style, content.data, {
        x: chrome.padX,
        top: chrome.contentTop,
      });
      drawn.nodes.forEach((n) => g.appendChild(n));
      return;
    }

    // 4. Title Zone (strictly below top bar, without icon prefix)
    if (style.title.show) {
      g.appendChild(
        text(
          {
            ...textAttrs(style.title),
            ...alignX(style.title, chrome.textBox, chrome.padX),
            y: chrome.textBox.y + chrome.captionTop,
            class: "semaps-node-label",
          },
          el.label,
        ),
      );
    }

    // 5. Subtitle Zone (strictly below title)
    if (style.subtitle.show) {
      const subtitle = typeof el.metadata.type === "string" ? el.metadata.type : el.type;
      g.appendChild(
        text(
          {
            ...textAttrs(style.subtitle),
            ...alignX(style.subtitle, chrome.textBox, chrome.padX),
            y: chrome.textBox.y + chrome.captionTop + NODE_LAYOUT.subtitleY(el.height > 60)
              - NODE_LAYOUT.titleY(el.height > 60),
            class: "semaps-node-subtitle",
          },
          subtitle,
        ),
      );
    }
  }

  /**
   * Where this shape's buttons, badge and text go.
   *
   * The rectangle's answer is the one the canvas grew up with: buttons in the
   * top-left corner, badge in the top-right, text across the whole box. Every
   * other shape overrides it, because "6px in from the corner" means nothing
   * on an outline whose corner is a point or a curve — that is exactly how a
   * diamond ended up with its controls floating in empty space outside itself.
   */
  protected chrome(el: DiagramElement, _style: ResolvedBlockStyle): ChromeLayout {
    const tall = el.height > 60;
    return {
      docAnchor: { x: el.x + 6, y: el.y + NODE_LAYOUT.topBarY },
      docGrow: "right",
      badgeAnchor: { x: el.x + el.width - 6, y: el.y + NODE_LAYOUT.topBarY },
      badgeGrow: "left",
      textBox: elementRect(el),
      padX: NODE_LAYOUT.padX,
      captionTop: NODE_LAYOUT.titleY(tall),
      contentTop: NODE_LAYOUT.contentTop,
    };
  }

  /**
   * The element's own outline, and the thing the pointer grabs to drag it.
   *
   * Split out from `update` so that a differently shaped node — an ellipse, a
   * cylinder, an actor — is a subclass that swaps this one method, rather than
   * a second copy of the chrome above it. Doc button, code button, link badge,
   * caption and subtitle are the same furniture whatever the outline is, and
   * duplicating them per shape is how they drift apart.
   */
  protected outline(el: DiagramElement, style: ResolvedBlockStyle, ctx: RenderContext): SVGElement {
    return svg("rect", {
      [ROLE_ATTR]: Role.DragHandle,
      x: el.x,
      y: el.y,
      width: el.width,
      height: el.height,
      rx: style.radius,
      ...this.paintAttrs(style, ctx),
    });
  }

  /** Fill and stroke as the style resolved them; shared by every outline. */
  protected paintAttrs(style: ResolvedBlockStyle, ctx: RenderContext): Record<string, unknown> {
    return {
      fill: ctx.paints.fill(style.fill),
      stroke: style.border.color,
      "stroke-width": style.border.width,
      style: `--sw: ${style.border.width}`,
      "stroke-dasharray": dashArray(style.border.dash),
      "stroke-opacity": style.border.opacity === 1 ? null : style.border.opacity,
    };
  }

  visibleRect(el: DiagramElement): Rect {
    return elementRect(el);
  }

  pointAt(rect: Rect, slot: BoundarySlot): Point {
    return pointOnSide(rect, slot.side, slot.t);
  }

  cornerInset(_side: Side, style?: ResolvedBlockStyle): number {
    return (style?.radius ?? DIAGRAM_CONFIG.node.defaultRadius) + DIAGRAM_CONFIG.ports.extraCornerGap;
  }
}
