import type { DiagramElement } from "../../model/types.js";
import type { ElementRenderer } from "./ElementRenderer.js";

/**
 * Maps an element's `type` to the renderer that draws it.
 *
 * A registry rather than a class hierarchy, deliberately: to add a type from
 * outside this library you register a renderer under a string key, without
 * subclassing anything or knowing how the built-in ones are put together.
 */
export class TypeRegistry {
  private readonly byType = new Map<string, ElementRenderer>();
  private readonly byKind = new Map<string, ElementRenderer>();
  private readonly byShape = new Map<string, ElementRenderer>();

  /** Register a renderer for one specific `type` value. */
  register(type: string, renderer: ElementRenderer): this {
    this.byType.set(type, renderer);
    return this;
  }

  /**
   * Register the fallback used for any element of this kind whose `type` has
   * no renderer of its own. Every kind must have one.
   */
  registerDefault(kind: DiagramElement["kind"], renderer: ElementRenderer): this {
    this.byKind.set(kind, renderer);
    return this;
  }

  /**
   * Register the renderer that draws one outline (`ellipse`, `actor`, …).
   *
   * Shape is a property of the *style*, not of the element's type: a use case
   * is an ellipse because of what it is, and one line in `styles.json` then
   * keeps every use case consistent. So resolution consults the shape the
   * style resolved to before it falls back to the type.
   */
  registerShape(shape: string, renderer: ElementRenderer): this {
    this.byShape.set(shape, renderer);
    return this;
  }

  /**
   * @param shape Outline named by the element's resolved style, if any.
   *   A renderer registered for the element's exact `type` still wins: that is
   *   the escape hatch for a type whose drawing is special beyond its outline.
   */
  resolve(el: DiagramElement, shape?: string): ElementRenderer {
    const exact = this.byType.get(el.type);
    if (exact !== undefined) return exact;

    if (shape !== undefined) {
      const shaped = this.byShape.get(shape);
      if (shaped !== undefined) return shaped;
    }

    const fallback = this.byKind.get(el.kind);
    if (fallback !== undefined) return fallback;

    throw new Error(`No renderer registered for ${el.kind} "${el.type}" (element ${el.id})`);
  }

  has(type: string): boolean {
    return this.byType.has(type);
  }
}
