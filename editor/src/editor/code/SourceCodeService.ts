import Prism from "prismjs";
import "prismjs/components/prism-clike.js";
import "prismjs/components/prism-csharp.js";
import "prismjs/components/prism-javascript.js";
import "prismjs/components/prism-typescript.js";
import "prismjs/components/prism-json.js";
import "prismjs/components/prism-markdown.js";
import "prismjs/components/prism-sql.js";
import "prismjs/components/prism-bash.js";

export interface HighlightResult {
  readonly html: string;
  readonly language: string;
  readonly lineCount: number;
}

export interface CodePreviewResult {
  readonly path: string;
  readonly snippetHtml: string;
  readonly totalLines: number;
  readonly language: string;
}

/**
 * Service responsible for asynchronously fetching source code from the server
 * and formatting it with Prism syntax highlighting and line numbering.
 */
export class SourceCodeService {
  private static readonly cache = new Map<string, string>();
  private static readonly pending = new Map<string, Promise<string>>();
  private static readonly availabilityCache = new Map<string, boolean>();

  /**
   * Check whether a source file is verified to exist on the server.
   * Returns true if verified, false if verified missing/directory, or undefined if not checked yet.
   */
  static isFileAvailable(codeRef: string): boolean | undefined {
    const cleanPath = codeRef.trim().replace(/^\/+/, "");
    if (!cleanPath) return false;
    return this.availabilityCache.get(cleanPath);
  }

  /**
   * Asynchronously validate a batch of code references against the server in the background.
   * Populates availabilityCache and returns true if any new status was established.
   */
  static async validateCodeRefs(codeRefs: readonly string[]): Promise<boolean> {
    const toCheck: string[] = [];
    for (const ref of codeRefs) {
      const clean = ref.trim().replace(/^\/+/, "");
      if (clean && !this.availabilityCache.has(clean)) {
        toCheck.push(clean);
      }
    }

    if (toCheck.length === 0) return false;

    try {
      const res = await fetch("/api/source/check", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(toCheck),
      });
      if (res.ok) {
        const result: Record<string, boolean> = await res.json();
        for (const [key, exists] of Object.entries(result)) {
          this.availabilityCache.set(key, exists);
        }
        return true;
      }
    } catch {
      // Fallback: check individually with HEAD
      await Promise.all(
        toCheck.map(async (ref) => {
          try {
            const r = await fetch(`/api/source?file=${encodeURIComponent(ref)}`, { method: "HEAD" });
            this.availabilityCache.set(ref, r.ok);
          } catch {
            this.availabilityCache.set(ref, false);
          }
        })
      );
      return true;
    }
    return false;
  }

  /**
   * Fetch raw source code text by relative codebase path.
   */
  static async fetchSource(codeRef: string): Promise<string> {
    const cleanPath = codeRef.trim().replace(/^\/+/, "");
    if (!cleanPath) throw new Error("Empty code reference");

    const cached = this.cache.get(cleanPath);
    if (cached !== undefined) return cached;

    const inFlight = this.pending.get(cleanPath);
    if (inFlight !== undefined) return inFlight;

    const promise = (async () => {
      try {
        const url = `/api/source?file=${encodeURIComponent(cleanPath)}`;
        const res = await fetch(url);
        if (!res.ok) {
          this.availabilityCache.set(cleanPath, false);
          throw new Error(`HTTP ${res.status}: ${res.statusText}`);
        }
        const text = await res.text();
        this.cache.set(cleanPath, text);
        this.availabilityCache.set(cleanPath, true);
        return text;
      } catch (err) {
        this.availabilityCache.set(cleanPath, false);
        throw err;
      } finally {
        this.pending.delete(cleanPath);
      }
    })();

    this.pending.set(cleanPath, promise);
    return promise;
  }

  /**
   * Determine programming language name from file extension.
   */
  static detectLanguage(codeRef: string): string {
    const ext = codeRef.split(".").pop()?.toLowerCase() ?? "";
    switch (ext) {
      case "cs":
        return "csharp";
      case "ts":
      case "tsx":
        return "typescript";
      case "js":
      case "jsx":
      case "mjs":
        return "javascript";
      case "json":
        return "json";
      case "md":
        return "markdown";
      case "sql":
        return "sql";
      case "sh":
      case "bash":
        return "bash";
      case "axaml":
      case "xaml":
      case "xml":
      case "html":
      case "svg":
        return "markup";
      case "go":
        return "go";
      default:
        return "clike";
    }
  }

  /**
   * Format language badge display name (e.g. "C#", "TypeScript").
   */
  static getLanguageLabel(codeRef: string): string {
    const lang = this.detectLanguage(codeRef);
    switch (lang) {
      case "csharp":
        return "C#";
      case "typescript":
        return "TypeScript";
      case "javascript":
        return "JavaScript";
      case "json":
        return "JSON";
      case "markdown":
        return "Markdown";
      case "sql":
        return "SQL";
      case "bash":
        return "Bash";
      case "markup":
        return "XML";
      case "go":
        return "Go";
      default:
        return lang.toUpperCase();
    }
  }

  /**
   * Highlight code string with Prism and wrap in structured line numbers.
   */
  static highlight(code: string, codeRef: string): HighlightResult {
    const lang = this.detectLanguage(codeRef);
    const grammar = Prism.languages[lang] ?? Prism.languages.clike ?? Prism.languages.markup ?? ({} as Prism.Grammar);

    const rawLines = code.replace(/\r\n/g, "\n").split("\n");
    const lineCount = rawLines.length;

    const highlightedHtml = Prism.highlight(code, grammar, lang);
    const lines = highlightedHtml.replace(/\r\n/g, "\n").split("\n");

    const lineElements = lines.map((line, idx) => {
      const lineNum = idx + 1;
      return `<div class="semaps-code-line"><span class="semaps-code-line-num" data-line="${lineNum}">${lineNum}</span><span class="semaps-code-line-content">${line || " "}</span></div>`;
    });

    return {
      html: `<div class="semaps-code-lines">${lineElements.join("")}</div>`,
      language: this.getLanguageLabel(codeRef),
      lineCount,
    };
  }

  /**
   * Load source code and format a compact snippet for rich tooltips.
   */
  static async getPreview(codeRef: string, maxLines = 14): Promise<CodePreviewResult> {
    const source = await this.fetchSource(codeRef);
    const lang = this.detectLanguage(codeRef);
    const grammar = Prism.languages[lang] ?? Prism.languages.clike ?? Prism.languages.markup ?? ({} as Prism.Grammar);

    const allLines = source.replace(/\r\n/g, "\n").split("\n");
    const totalLines = allLines.length;

    const previewSlice = allLines.slice(0, maxLines).join("\n");
    const highlightedHtml = Prism.highlight(previewSlice, grammar, lang);
    const lines = highlightedHtml.replace(/\r\n/g, "\n").split("\n");

    const lineElements = lines.map((line, idx) => {
      const lineNum = idx + 1;
      return `<div class="semaps-code-line"><span class="semaps-code-line-num" data-line="${lineNum}">${lineNum}</span><span class="semaps-code-line-content">${line || " "}</span></div>`;
    });

    return {
      path: codeRef,
      snippetHtml: `<div class="semaps-code-lines">${lineElements.join("")}</div>`,
      totalLines,
      language: this.getLanguageLabel(codeRef),
    };
  }
}
