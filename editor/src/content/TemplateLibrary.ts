import { compileTemplate } from "./parser.js";
import type { CompileResult } from "./template-types.js";

/** Where the shared template registry lives, next to `styles.json`. */
const TEMPLATE_FILE = "templates.json";

/**
 * One named template as it sits on disk.
 *
 * The text is stored as an array of lines rather than one string with escaped
 * newlines: this file is edited by hand and read in diffs, and `"@Name\n@Members(…)"`
 * is unreadable in both. Joining on load is the whole cost of that.
 */
export interface WireTemplate {
  id: string;
  name?: string;
  description?: string;
  lines: string[];
}

export interface WireTemplateSheet {
  version?: number;
  description?: string;
  templates?: WireTemplate[];
}

/**
 * The named content templates a style or a placement can point at.
 *
 * Compilation is cached per id and thrown away when the text changes, because
 * the text is the source of truth and the tree is a per-repaint artefact
 * (ADR_20260903 §2.3). Nothing here ever writes a tree back out.
 */
export class TemplateLibrary {
  private sheet: WireTemplateSheet = { templates: [] };
  private readonly compiled = new Map<string, CompileResult>();

  constructor(private readonly baseUrl: string = "./") {}

  /**
   * Read the registry. A missing file is normal — a workspace that has never
   * used a template still draws, with the built-in caption-only look.
   */
  async load(): Promise<void> {
    try {
      const res = await fetch(new URL(TEMPLATE_FILE, new URL(this.baseUrl, location.href)));
      this.sheet = res.ok ? ((await res.json()) as WireTemplateSheet) : { templates: [] };
    } catch {
      this.sheet = { templates: [] };
    }
    this.compiled.clear();
  }

  list(): readonly WireTemplate[] {
    return this.sheet.templates ?? [];
  }

  text(id: string): string | undefined {
    const found = this.list().find((t) => t.id === id);
    return found === undefined ? undefined : found.lines.join("\n");
  }

  /**
   * The compiled form, or undefined when no such template exists.
   *
   * Errors are part of the result, never an exception: a template with a typo
   * still draws the rows that parsed, and the panel shows what went wrong.
   */
  get(id: string): CompileResult | undefined {
    const cached = this.compiled.get(id);
    if (cached !== undefined) return cached;

    const source = this.text(id);
    if (source === undefined) return undefined;

    const result = compileTemplate(source);
    this.compiled.set(id, result);
    return result;
  }

  /**
   * Replace one template's text, in memory.
   *
   * Used by the editing panel on every keystroke, which is why it is cheap and
   * why saving is a separate, deliberate act: a template is shared by every
   * element wearing the style, and half-typed text must not reach the file.
   */
  setText(id: string, source: string): CompileResult {
    const templates = this.sheet.templates ?? (this.sheet.templates = []);
    const existing = templates.find((t) => t.id === id);
    const lines = source.split(/\r?\n/);
    if (existing === undefined) templates.push({ id, lines });
    else existing.lines = lines;

    const result = compileTemplate(source);
    this.compiled.set(id, result);
    return result;
  }

  async save(): Promise<void> {
    const res = await fetch(`/api/save?file=${encodeURIComponent(TEMPLATE_FILE)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(this.sheet, null, 2),
    });
    if (!res.ok) throw new Error(`Сервер ответил HTTP ${res.status}`);
  }
}
