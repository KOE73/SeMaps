/**
 * The content-template contract — in-memory shape only.
 *
 * A template is text typed by a human into a settings panel with a live
 * preview (ADR_20260903 §2.3). The text is the source of truth; this tree is
 * a throwaway result of compiling it for one repaint. It is never
 * serialised, never round-tripped, never diffed — only the text is. Do not
 * add a "stringify this tree back to a template" function: if one is ever
 * needed, something upstream is holding the tree instead of the text, which
 * is exactly the mistake this module exists to prevent.
 */

/** A size in the template's geometry arguments: either pixels or a percentage. */
export type SizeValue = { kind: "px"; value: number } | { kind: "percent"; value: number };

/**
 * `w`, `h`, `align` parse identically for every directive — they describe the
 * layout slot the directive's output sits in, not what the directive does.
 * Absent means "let the layout decide"; there is no directive-specific
 * default to fall back to here.
 */
export interface GeometryArgs {
  w?: SizeValue;
  h?: SizeValue;
  align?: string;
}

/**
 * Everything a directive call carried, split into the one shared shape.
 *
 * `positional` is the single "what to show" argument (ADR_20260903 §2.3): a
 * bare word (`brain`), or a bracketed list (`[name, type]`). `named` holds
 * everything else verbatim as strings, plus bare flags (`collapsed`) as
 * `true` — interpreting them (`where=kind:field`, a flag's meaning) is the
 * directive's job, not the parser's. `geometry` is named args lifted out
 * because every directive shares their meaning and parsing.
 */
export interface DirectiveArgs {
  positional?: string | string[];
  named: Record<string, string | true>;
  geometry: GeometryArgs;
}

export interface DirectiveNode {
  kind: "directive";
  /** Name as written after `@`, e.g. "Members". Case as typed, not normalised. */
  name: string;
  args: DirectiveArgs;
  /** 1-based line and column in the source text, for error markers in the panel. */
  line: number;
  column: number;
}

/** A cell today is exactly one directive call — the grammar has no other content. */
export type TemplateCell = DirectiveNode;

export interface TemplateRow {
  kind: "row";
  /** Cells left to right, split from `|`. Empty when every cell in the line failed to parse. */
  cells: TemplateCell[];
  /** A blank source line: a deliberate vertical gap, not a row with nothing in it. */
  spacer: boolean;
  line: number;
}

export interface TemplateTree {
  kind: "template";
  rows: TemplateRow[];
}

export interface TemplateError {
  line: number;
  column: number;
  /** Russian, matching the rest of the UI — this is shown next to the text the human just typed. */
  message: string;
}

/**
 * What `compileTemplate` returns, always, no matter how broken the text is.
 *
 * There is no throwing variant. The panel is expected to keep painting the
 * last `tree` it got a clean-enough version of, plus an error marker, rather
 * than blank the canvas on every keystroke — see ADR_20260903 §2.3 and the
 * `$schema draft/2020-12` incident it cites.
 */
export interface CompileResult {
  tree: TemplateTree;
  errors: TemplateError[];
}
