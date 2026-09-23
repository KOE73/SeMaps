import type { DiagramDocument } from "../../model/document.js";
import type { DiagramElement } from "../../model/types.js";
import { FALLBACK_BLOCK, FALLBACK_EDGE } from "../../model/StyleLibrary.js";
import type {
  Paint,
  ResolvedBlockStyle,
  ResolvedEdgeStyle,
  StyleLibrary,
} from "../../model/StyleLibrary.js";

/**
 * Export to Diagrams.net (mxGraph) XML.
 *
 * Containers become swimlanes so that draw.io treats them as real containers,
 * and semantics travel in an `<Object as="data">` child so that meaning — type,
 * semantic id, tags, code reference — survives the round trip rather than being
 * flattened into colours.
 *
 * @param styles Colours come from here. Without it the export falls back to the
 *   library's floor rather than to a second hardcoded palette — there used to be
 *   one here (`ZONE_DEFAULTS`, a white node fill), which meant an exported file
 *   looked nothing like the canvas it came from.
 */
export function exportDrawio(doc: DiagramDocument, styles?: StyleLibrary): string {
  const title = doc.metadata.title;
  const parts: string[] = [];

  parts.push('<?xml version="1.0" encoding="UTF-8"?>');
  parts.push(
    `<mxfile host="app.diagrams.net" agent="SeMaps" version="21.0.0" type="device">`,
  );
  parts.push(`  <diagram id="semaps" name="${attr(title)}">`);
  parts.push(
    '    <mxGraphModel dx="1200" dy="800" grid="1" gridSize="10" guides="1" tooltips="1"' +
      ' connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="1600" pageHeight="1000">',
  );
  parts.push("      <root>");
  parts.push('        <mxCell id="0" />');
  parts.push('        <mxCell id="1" parent="0" />');

  for (const el of doc.elements()) {
    if (el.kind !== "zone") continue;
    const style = blockStyleOf(el, styles);
    const dashed = style.border.dash !== "none" ? "1" : "0";
    const cellStyle =
      `swimlane;startSize=${Math.round(style.header.height)};rounded=1;whiteSpace=wrap;html=1;` +
      `fillColor=${solid(style.fill)};strokeColor=${style.border.color};strokeWidth=${style.border.width};` +
      `dashed=${dashed};collapsible=1;container=1;fontStyle=1;spacingLeft=10;`;

    parts.push(
      `        <mxCell id="${attr(el.id)}" value="${attr(el.label)}" style="${attr(cellStyle)}" vertex="1" parent="1">`,
    );
    parts.push(geometry(el.x, el.y, el.width, el.height));
    parts.push(
      `          <Object type="${attr(el.type)}" semanticId="${attr(el.semanticId ?? "")}"` +
        ` tags="${attr(el.tags.join(","))}" as="data" />`,
    );
    parts.push("        </mxCell>");
  }

  for (const el of doc.elements()) {
    if (el.kind !== "node") continue;
    const parent = el.parent !== null && el.parent.kind === "zone" ? el.parent : null;
    const parentId = parent?.id ?? "1";
    // Children of a swimlane are positioned relative to it.
    const x = parent === null ? el.x : el.x - parent.x;
    const y = parent === null ? el.y : el.y - parent.y;
    const style = blockStyleOf(el, styles);
    const cellStyle =
      `rounded=1;whiteSpace=wrap;html=1;fillColor=${solid(style.fill)};` +
      `strokeColor=${style.border.color};strokeWidth=${style.border.width};fontStyle=1;`;

    parts.push(
      `        <mxCell id="${attr(el.id)}" value="${attr(el.label)}" style="${attr(cellStyle)}" vertex="1" parent="${attr(parentId)}">`,
    );
    parts.push(geometry(x, y, el.width, el.height));
    parts.push(
      `          <Object type="${attr(el.type)}" codeRef="${attr(codeRef(el))}"` +
        ` tags="${attr(el.tags.join(","))}" as="data" />`,
    );
    parts.push("        </mxCell>");
  }

  for (const edge of doc.edges) {
    const style: ResolvedEdgeStyle = styles === undefined ? FALLBACK_EDGE : styles.edgeStyle(edge);
    const cellStyle =
      "edgeStyle=orthogonalEdgeStyle;rounded=1;orthogonalLoop=1;jettySize=auto;html=1;" +
      `strokeColor=${style.line.color};strokeWidth=${style.line.width};` +
      (style.line.dash === "none" ? "" : "dashed=1;");
    parts.push(
      `        <mxCell id="${attr(edge.id)}" value="${attr(edge.label)}" style="${attr(cellStyle)}"` +
        ` edge="1" parent="1" source="${attr(edge.from)}" target="${attr(edge.to)}">`,
    );
    parts.push('          <mxGeometry relative="1" as="geometry" />');
    parts.push(`          <Object type="${attr(edge.type)}" as="data" />`);
    parts.push("        </mxCell>");
  }

  parts.push("      </root>");
  parts.push("    </mxGraphModel>");
  parts.push("  </diagram>");
  parts.push("</mxfile>");
  return parts.join("\n");
}

export function drawioFileName(doc: DiagramDocument): string {
  const title = String(doc.metadata?.title ?? "");
  const base = title.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_|_$/g, "");
  return `${base === "" ? "diagram" : base}.drawio`;
}

function geometry(x: number, y: number, width: number, height: number): string {
  return (
    `          <mxGeometry x="${Math.round(x)}" y="${Math.round(y)}"` +
    ` width="${Math.round(width)}" height="${Math.round(height)}" as="geometry" />`
  );
}

function blockStyleOf(el: DiagramElement, styles: StyleLibrary | undefined): ResolvedBlockStyle {
  return styles === undefined ? FALLBACK_BLOCK : styles.blockStyle(el);
}

/**
 * mxGraph has no gradient in a cell style string worth reproducing, so a
 * gradient exports as its first stop: recognisably the same style, rather than
 * a colour picked out of nowhere.
 */
function solid(p: Paint): string {
  return p.kind === "solid" ? p.color : p.stops[0]?.color ?? "#ffffff";
}

function codeRef(el: DiagramElement): string {
  return typeof el.metadata.codeRef === "string" ? el.metadata.codeRef : "";
}

/**
 * Escape a value for an XML attribute. The original interpolated model text
 * straight into the markup, so a label containing a quote produced a broken
 * file — the same class of bug as D-03.
 */
function attr(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}
