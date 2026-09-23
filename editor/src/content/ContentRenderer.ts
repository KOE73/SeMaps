import type { Rect } from "../geometry/types.js";
import type { ResolvedBlockStyle } from "../model/StyleLibrary.js";
import { svg, text } from "../canvas/svg.js";
import type { DirectiveNode, SizeValue, TemplateTree } from "./template-types.js";

/**
 * Drawing the inside of a block from a compiled template.
 *
 * This is the whole reason there is a template language: every way of showing
 * an entity — a bare caption, fields and methods, a picture — is this one
 * walker over a tree, not a renderer per kind. A new look is text someone
 * typed, and nothing here changes.
 *
 * Deliberately narrow inputs. The walker knows about a rectangle, a resolved
 * style and a `ContentData` record; it does not know what a `DiagramElement`
 * is, where members came from, or how an asset was fetched. That keeps it
 * testable and keeps the model out of the drawing code.
 */

/** One member of a type, as the template sees it. */
export interface MemberView {
  /** `field`, `method`, `property` — matched by `where=kind:…`. */
  readonly kind?: string;
  readonly name: string;
  readonly type?: string;
  readonly visibility?: string;
  readonly note?: string;
  readonly [key: string]: unknown;
}

/** Everything the directives can draw for one element. */
export interface ContentData {
  readonly name: string;
  readonly description?: string;
  readonly members?: readonly MemberView[];
  /**
   * A picture by name, if it is already loaded. Returning undefined must be
   * survivable: the block draws without it and gets a repaint when it lands.
   */
  asset?(id: string): { readonly svg: string } | undefined;
}

/**
 * Where content sits inside the box it was given.
 *
 * Supplied by the shape rather than assumed, because a diamond's usable
 * interior is not its bounding box and an ellipse's is not either. These are
 * only the fallback for callers that have no opinion.
 */
export interface ContentPadding {
  readonly x: number;
  readonly top: number;
}

const DEFAULT_PADDING: ContentPadding = { x: 10, top: 22 };

/** Vertical rhythm. Kept here so rows and cells cannot disagree about it. */
const ROW_GAP = 3;
const SPACER = 8;
/** Rough width of one character relative to font size, for eliding long text. */
const CHAR_RATIO = 0.55;

export interface ContentRenderResult {
  readonly nodes: readonly SVGElement[];
  /** Height actually used, so a caller can tell whether the block is too small. */
  readonly usedHeight: number;
}

export function renderContent(
  tree: TemplateTree,
  rect: Rect,
  style: ResolvedBlockStyle,
  data: ContentData,
  padding: ContentPadding = DEFAULT_PADDING,
): ContentRenderResult {
  const nodes: SVGElement[] = [];
  const innerWidth = Math.max(rect.width - padding.x * 2, 8);
  let y = rect.y + padding.top;

  for (const row of tree.rows) {
    if (row.spacer) {
      y += SPACER;
      continue;
    }

    // A row of directives that all drew nothing takes no space at all. This is
    // the "empty hides itself" rule: without it a marker interface would show
    // a gap where its methods would have been, and templates would need
    // conditionals to avoid it.
    const drawn = layoutRow(row.cells, rect.x + padding.x, y, innerWidth, style, data);
    if (drawn.nodes.length === 0) continue;

    nodes.push(...drawn.nodes);
    y += drawn.height + ROW_GAP;
  }

  return { nodes, usedHeight: y - rect.y };
}

function layoutRow(
  cells: readonly DirectiveNode[],
  x: number,
  y: number,
  width: number,
  style: ResolvedBlockStyle,
  data: ContentData,
): { nodes: SVGElement[]; height: number } {
  const nodes: SVGElement[] = [];
  let height = 0;

  // Cells split the row: an explicit `w=` takes its share first, the rest
  // divide what is left. Columns are the only layout the language has, so they
  // stay this blunt on purpose.
  const explicit = cells.map((c) => sizeToPx(c.args.geometry.w, width));
  const freeCount = explicit.filter((w) => w === undefined).length;
  const taken = explicit.reduce<number>((sum, w) => sum + (w ?? 0), 0);
  const share = freeCount > 0 ? Math.max((width - taken) / freeCount, 24) : 0;

  let cursor = x;
  cells.forEach((cell, i) => {
    const cellWidth = explicit[i] ?? share;
    const drawn = drawDirective(cell, cursor, y, cellWidth, style, data);
    nodes.push(...drawn.nodes);
    height = Math.max(height, drawn.height);
    cursor += cellWidth;
  });

  return { nodes, height };
}

function drawDirective(
  cell: DirectiveNode,
  x: number,
  y: number,
  width: number,
  style: ResolvedBlockStyle,
  data: ContentData,
): { nodes: SVGElement[]; height: number } {
  switch (cell.name.toLowerCase()) {
    case "name":
      return line(data.name, x, y, width, style.title, "semaps-node-label");

    case "description":
      return line(data.description ?? "", x, y, width, style.subtitle, "semaps-node-subtitle");

    case "members":
      return members(cell, x, y, width, style, data);

    case "asset":
      return asset(cell, x, y, width, data);

    default:
      // An unknown directive is the compiler's finding, not a drawing problem;
      // here it simply contributes nothing.
      return { nodes: [], height: 0 };
  }
}

interface TextLook {
  readonly family: string;
  readonly size: number;
  readonly weight: number;
  readonly italic: boolean;
  readonly color: string;
  readonly opacity: number;
  readonly show: boolean;
}

function line(
  value: string,
  x: number,
  y: number,
  width: number,
  look: TextLook,
  cls: string,
): { nodes: SVGElement[]; height: number } {
  if (value.trim() === "" || !look.show) return { nodes: [], height: 0 };

  const height = look.size * 1.35;
  return {
    nodes: [
      text(
        {
          x,
          y: y + look.size,
          "font-family": look.family,
          "font-size": look.size,
          "font-weight": look.weight,
          "font-style": look.italic ? "italic" : null,
          fill: look.color,
          opacity: look.opacity === 1 ? null : look.opacity,
          class: cls,
        },
        elide(value, width, look.size),
      ),
    ],
    height,
  };
}

function members(
  cell: DirectiveNode,
  x: number,
  y: number,
  width: number,
  style: ResolvedBlockStyle,
  data: ContentData,
): { nodes: SVGElement[]; height: number } {
  const all = data.members ?? [];
  const selected = all.filter((m) => matches(m, cell.args.named["where"]));
  if (selected.length === 0) return { nodes: [], height: 0 };

  // Which fields of a member to print. The default is the name alone, so that
  // `@Members(where=kind:field)` is already useful without spelling out a list.
  const positional = cell.args.positional;
  const columns = Array.isArray(positional) ? positional : positional ? [positional] : ["name"];

  const look = style.subtitle;
  const step = look.size * 1.4;
  const nodes: SVGElement[] = [];

  // `collapsed` shows the count instead of the list. The list itself is not
  // lost: the block's popover shows it on click, which is what keeps the
  // block's own height — and therefore the hand-made layout — unchanged.
  if (cell.args.named["collapsed"] === true) {
    return line(`▸ ${selected.length} членов`, x, y, width, look, "semaps-node-members-collapsed");
  }

  selected.forEach((m, i) => {
    const value = columns.map((c) => stringify(m[c])).filter((s) => s !== "").join(": ");
    if (value === "") return;
    nodes.push(
      text(
        {
          x,
          y: y + look.size + i * step,
          "font-family": look.family,
          "font-size": look.size,
          "font-weight": look.weight,
          fill: look.color,
          class: "semaps-node-member",
        },
        elide(value, width, look.size),
      ),
    );
  });

  return { nodes, height: nodes.length * step };
}

/**
 * `where=kind:field` — one declarative filter, no branching in the language.
 * Anything unparseable matches everything, so a typo shows too much rather
 * than silently emptying the block.
 */
function matches(member: MemberView, where: string | true | undefined): boolean {
  if (where === undefined || where === true) return true;
  const [field, expected] = where.split(":", 2);
  if (field === undefined || expected === undefined) return true;
  return stringify(member[field.trim()]).toLowerCase() === expected.trim().toLowerCase();
}

function asset(
  cell: DirectiveNode,
  x: number,
  y: number,
  width: number,
  data: ContentData,
): { nodes: SVGElement[]; height: number } {
  const id = typeof cell.args.positional === "string" ? cell.args.positional : undefined;
  if (id === undefined || data.asset === undefined) return { nodes: [], height: 0 };

  const loaded = data.asset(id);
  if (loaded === undefined || loaded.svg === "") return { nodes: [], height: 0 };

  const height = sizeToPx(cell.args.geometry.h, 200) ?? 80;
  const boxWidth = sizeToPx(cell.args.geometry.w, width) ?? width;

  // The picture is dropped in as sanitised markup inside its own <svg> so that
  // its viewBox does the scaling: the asset decides its aspect, the template
  // decides the space it gets.
  const holder = svg("g", { class: "semaps-node-asset", transform: `translate(${x} ${y})` });
  holder.innerHTML = wrap(loaded.svg, boxWidth, height);
  return { nodes: [holder], height };
}

function wrap(inner: string, width: number, height: number): string {
  // The sanitiser already stripped width/height from the root, so setting them
  // here is enough to size it; preserveAspectRatio keeps it from stretching.
  return inner.replace(
    /^<svg/i,
    `<svg width="${width}" height="${height}" preserveAspectRatio="xMidYMid meet"`,
  );
}

function sizeToPx(size: SizeValue | undefined, basis: number): number | undefined {
  if (size === undefined) return undefined;
  return size.kind === "px" ? size.value : (basis * size.value) / 100;
}

function stringify(value: unknown): string {
  return value === undefined || value === null ? "" : String(value);
}

/** Cut text that cannot fit, rather than letting it run past the outline. */
function elide(value: string, width: number, fontSize: number): string {
  const max = Math.max(Math.floor(width / (fontSize * CHAR_RATIO)), 3);
  return value.length <= max ? value : `${value.slice(0, max - 1)}…`;
}
