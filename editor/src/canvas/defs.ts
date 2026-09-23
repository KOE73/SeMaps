import { svg } from "./svg.js";

/**
 * The one definition every element shares: the drop shadow.
 *
 * There used to be a fixed list of eight markers here, each with a colour baked
 * into its path. That is why an edge's line and its head were two independent
 * colours nobody kept in agreement — `semaps-arrow-security` was red whatever the
 * line under it happened to be — and why "a violet dashed arrow" could not be
 * expressed without editing this file. Heads are now built by `PaintRegistry`
 * on demand and named after their own content, so asking for the same head
 * twice returns the same id, and a head follows its line by default.
 *
 * The shadow stays: it carries no colour, every box that wants it wants the
 * same one, and it is referenced by id from the renderers.
 */
export function createDefs(): SVGDefsElement {
  const shadow = svg("filter", {
    id: "semaps-shadow", x: "-5%", y: "-5%", width: "115%", height: "115%",
  }, [
    svg("feDropShadow", { dx: 0, dy: 2, stdDeviation: 3, "flood-opacity": 0.08 }),
  ]);

  return svg("defs", {}, [shadow]);
}
