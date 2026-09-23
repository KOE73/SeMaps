/**
 * Compiler for the block-content mini-language (ADR_20260903 §2.3).
 *
 * `compileTemplate` never throws. Every malformed construct — an unknown
 * directive, an unclosed bracket, a stray argument — becomes an entry in
 * `errors` and parsing keeps going on a best-effort basis, because the
 * caller's whole reason for existing is a text box the human is actively
 * typing into: the text is wrong on almost every keystroke, and the panel is
 * expected to keep the last good picture on screen plus an error marker
 * rather than blank the canvas (the `$schema draft/2020-12` incident this
 * ADR cites by name). A throwing compiler would force every caller to
 * reinvent that fallback; a non-throwing one makes it the default.
 *
 * Pure and DOM-free by design — this has to be testable and callable from a
 * settings panel without pulling in the canvas.
 */

import {
  type CompileResult,
  type DirectiveArgs,
  type DirectiveNode,
  type GeometryArgs,
  type SizeValue,
  type TemplateError,
  type TemplateRow,
  type TemplateTree,
} from "./template-types.js";
import { type DirectiveRegistry, createDefaultDirectiveRegistry } from "./directive-registry.js";

const IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;
const GEOMETRY_NAMES = new Set(["w", "h", "align"]);

/** One line of source, kept with its 1-based number so errors can point back at the text. */
interface SourceLine {
  text: string;
  line: number;
}

/** A raw slice of a line — a cell or an argument token — before it is parsed further. */
interface Span {
  text: string;
  line: number;
  /** 1-based column where `text` starts in the original line. */
  column: number;
}

export function compileTemplate(text: string, registry: DirectiveRegistry = createDefaultDirectiveRegistry()): CompileResult {
  const errors: TemplateError[] = [];
  const lines = splitLines(text);
  const rows: TemplateRow[] = [];

  for (const line of lines) {
    if (line.text.trim() === "") {
      rows.push({ kind: "row", cells: [], spacer: true, line: line.line });
      continue;
    }

    const cellSpans = splitTopLevel(line.text, line.line, 1, "|");
    const cells: DirectiveNode[] = [];
    for (const span of cellSpans) {
      const trimmed = trimSpan(span);
      if (trimmed.text === "") continue; // `@A | | @B` — an empty cell is not an error, just nothing there
      const node = parseDirective(trimmed, registry, errors);
      if (node) cells.push(node);
    }
    rows.push({ kind: "row", cells, spacer: false, line: line.line });
  }

  const tree: TemplateTree = { kind: "template", rows };
  return { tree, errors };
}

// ---------------------------------------------------------------- splitting

function splitLines(text: string): SourceLine[] {
  return text.split(/\r\n|\r|\n/).map((t, i) => ({ text: t, line: i + 1 }));
}

/**
 * Splits `text` on a top-level separator, ignoring separators nested inside
 * `[...]` or `(...)`. Used for both `|` (cells within a row) and `,`
 * (arguments within a directive's parentheses) — same rule, different
 * character, so one function serves both.
 */
function splitTopLevel(text: string, line: number, startColumn: number, separator: string): Span[] {
  const spans: Span[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (ch === "[" || ch === "(") depth++;
    else if (ch === "]" || ch === ")") depth = Math.max(0, depth - 1);
    else if (ch === separator && depth === 0) {
      spans.push({ text: text.slice(start, i), line, column: startColumn + start });
      start = i + 1;
    }
  }
  spans.push({ text: text.slice(start), line, column: startColumn + start });
  return spans;
}

/** Trims a span's text while keeping its column pointed at the first non-blank character. */
function trimSpan(span: Span): Span {
  const leading = span.text.length - span.text.trimStart().length;
  return { text: span.text.trim(), line: span.line, column: span.column + leading };
}

// --------------------------------------------------------- directive parsing

function parseDirective(span: Span, registry: DirectiveRegistry, errors: TemplateError[]): DirectiveNode | undefined {
  const match = /^@([A-Za-z_][A-Za-z0-9_]*)/.exec(span.text);
  if (!match) {
    errors.push({ line: span.line, column: span.column, message: `ожидалась директива, начинающаяся с "@": "${span.text}"` });
    return undefined;
  }

  const name = match[1] ?? "";
  const nameEnd = match[0].length;
  const signature = registry.get(name);
  if (!signature) {
    errors.push({ line: span.line, column: span.column, message: `неизвестная директива @${name}` });
  }

  const rest = span.text.slice(nameEnd).trim();
  const args: DirectiveArgs = { named: {}, geometry: {} };

  if (rest !== "") {
    if (rest[0] !== "(" || !rest.endsWith(")")) {
      errors.push({
        line: span.line,
        column: span.column + nameEnd,
        message: `аргументы директивы @${name} должны быть в круглых скобках`,
      });
    } else {
      // Column of the character right after "(", for argument spans below.
      const argsColumn = span.column + nameEnd + 1;
      const inner = rest.slice(1, -1);
      if (!isBalanced(inner)) {
        errors.push({ line: span.line, column: span.column + nameEnd, message: `не закрыта скобка в @${name}` });
      } else {
        parseArgs(inner, span.line, argsColumn, name, signature?.positional.shape, args, errors);
      }
    }
  }

  if (signature) validateAgainstSignature(name, signature, span, args, errors);

  return { kind: "directive", name, args, line: span.line, column: span.column };
}

/** `[` and `(` must balance within the parenthesised body — an unclosed inner bracket is the common typo mid-edit. */
function isBalanced(text: string): boolean {
  let depth = 0;
  for (const ch of text) {
    if (ch === "[" || ch === "(") depth++;
    else if (ch === "]" || ch === ")") {
      depth--;
      if (depth < 0) return false;
    }
  }
  return depth === 0;
}

function parseArgs(
  inner: string,
  line: number,
  column: number,
  directiveName: string,
  positionalShape: import("./directive-registry.js").PositionalShape | undefined,
  args: DirectiveArgs,
  errors: TemplateError[],
): void {
  const tokens = splitTopLevel(inner, line, column, ",")
    .map(trimSpan)
    .filter((t) => t.text !== "");

  let positionalClaimed = args.positional !== undefined;

  for (const token of tokens) {
    if (token.text.startsWith("[")) {
      const list = parseList(token, directiveName, errors);
      if (list === undefined) continue;
      if (positionalClaimed) {
        errors.push({ line: token.line, column: token.column, message: `у @${directiveName} уже есть основной аргумент` });
        continue;
      }
      args.positional = list;
      positionalClaimed = true;
      continue;
    }

    const eq = findTopLevelEquals(token.text);
    if (eq >= 0) {
      const rawName = token.text.slice(0, eq).trim();
      const rawValue = token.text.slice(eq + 1).trim();
      if (!IDENTIFIER.test(rawName)) {
        errors.push({ line: token.line, column: token.column, message: `некорректное имя аргумента: "${rawName}"` });
        continue;
      }
      if (rawValue === "") {
        errors.push({ line: token.line, column: token.column, message: `аргумент "${rawName}" без значения` });
        continue;
      }
      assignNamed(rawName, rawValue, token, args, errors);
      continue;
    }

    // A bare word: either the positional value, or a flag — decided by what
    // this directive's positional shape allows. `@Members(collapsed)` and
    // `@Asset(brain)` are the same token shape; only the signature tells
    // them apart (see `PositionalShape` doc in directive-registry.ts).
    if (!IDENTIFIER.test(token.text)) {
      errors.push({ line: token.line, column: token.column, message: `некорректный аргумент: "${token.text}"` });
      continue;
    }

    const acceptsBareValue = positionalShape === "value" || positionalShape === "either" || positionalShape === undefined;
    if (!positionalClaimed && acceptsBareValue) {
      args.positional = token.text;
      positionalClaimed = true;
    } else {
      args.named[token.text] = true;
    }
  }
}

function parseList(token: Span, directiveName: string, errors: TemplateError[]): string[] | undefined {
  if (!token.text.endsWith("]")) {
    errors.push({ line: token.line, column: token.column, message: `не закрыт список в @${directiveName}` });
    return undefined;
  }
  const body = token.text.slice(1, -1);
  const items = splitTopLevel(body, token.line, token.column + 1, ",")
    .map(trimSpan)
    .map((t) => t.text)
    .filter((t) => t !== "");
  return items;
}

/** `=` inside a value (e.g. never happens today, but `where=kind:field` has a `:` not `=`) — first top-level `=` wins. */
function findTopLevelEquals(text: string): number {
  let depth = 0;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (ch === "[" || ch === "(") depth++;
    else if (ch === "]" || ch === ")") depth = Math.max(0, depth - 1);
    else if (ch === "=" && depth === 0) return i;
  }
  return -1;
}

function assignNamed(name: string, value: string, token: Span, args: DirectiveArgs, errors: TemplateError[]): void {
  if (GEOMETRY_NAMES.has(name)) {
    assignGeometry(name as keyof GeometryArgs, value, token, args.geometry, errors);
    return;
  }
  if (name in args.named) {
    errors.push({ line: token.line, column: token.column, message: `аргумент "${name}" указан дважды` });
  }
  args.named[name] = value;
}

function assignGeometry(name: keyof GeometryArgs, value: string, token: Span, geometry: GeometryArgs, errors: TemplateError[]): void {
  if (name === "align") {
    if (geometry.align !== undefined) {
      errors.push({ line: token.line, column: token.column, message: `аргумент "align" указан дважды` });
    }
    geometry.align = value;
    return;
  }

  const size = parseSize(value);
  if (!size) {
    errors.push({
      line: token.line,
      column: token.column,
      message: `неверное значение "${name}": ожидалось число (px) или проценты, получено "${value}"`,
    });
    return;
  }
  if (geometry[name] !== undefined) {
    errors.push({ line: token.line, column: token.column, message: `аргумент "${name}" указан дважды` });
  }
  geometry[name] = size;
}

function parseSize(value: string): SizeValue | undefined {
  if (/^\d+(\.\d+)?%$/.test(value)) return { kind: "percent", value: parseFloat(value) };
  if (/^\d+(\.\d+)?$/.test(value)) return { kind: "px", value: parseFloat(value) };
  return undefined;
}

function validateAgainstSignature(
  name: string,
  signature: import("./directive-registry.js").DirectiveSignature,
  span: Span,
  args: DirectiveArgs,
  errors: TemplateError[],
): void {
  const { shape, required } = signature.positional;

  if (args.positional === undefined) {
    if (required) {
      errors.push({ line: span.line, column: span.column, message: `директиве @${name} нужен основной аргумент` });
    }
    return;
  }

  const isList = Array.isArray(args.positional);
  if (shape === "none") {
    errors.push({ line: span.line, column: span.column, message: `директива @${name} не принимает основной аргумент` });
  } else if (shape === "value" && isList) {
    errors.push({ line: span.line, column: span.column, message: `директива @${name} ожидает значение, а не список` });
  } else if (shape === "list" && !isList) {
    errors.push({ line: span.line, column: span.column, message: `директива @${name} ожидает список в квадратных скобках` });
  }
}
