import { ROLE_ATTR, Role } from "../../interaction/roles.js";
import type { AutomaticStyleLayer } from "../../model/style-extensions.js";
import { automaticStyleLines, automaticStyleTitle } from "../../ui/automaticStyle.js";
import { svg } from "../svg.js";
import { iconGlyph } from "./iconGlyph.js";

export function styleIndicator(layers: readonly AutomaticStyleLayer[], x: number, y: number, width: number, height: number, kind: "node" | "zone" = "node"): SVGGElement {
  const size = kind === "zone" ? 14 : 11;
  return svg("g", {
    class: `semaps-style-indicator semaps-${kind}-doc`, [ROLE_ATTR]: Role.StyleInfo,
    role: "button", tabindex: 0, "aria-label": [automaticStyleTitle(), ...automaticStyleLines(layers)].join("\n"),
    style: "cursor: pointer;",
  }, [
    svg("rect", { x, y, width, height, rx: kind === "zone" ? 5 : 4, class: `semaps-${kind}-doc-rect` }),
    iconGlyph("brush", x + (width - size) / 2, y + (height - size) / 2, size, `semaps-${kind}-doc-icon`),
  ]);
}
