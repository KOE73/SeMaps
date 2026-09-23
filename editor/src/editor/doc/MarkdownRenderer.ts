import { marked } from "marked";

marked.use({
  gfm: true,
  breaks: true,
  renderer: {
    link(token) {
      const href = token.href;
      const title = token.title ? ` title="${escapeHtml(token.title)}"` : "";
      const text = this.parser.parseInline(token.tokens);
      const isExternal = /^https?:\/\//i.test(href);
      const targetAttr = isExternal ? ' target="_blank" rel="noopener noreferrer"' : "";
      return `<a href="${href}"${title}${targetAttr}>${text}</a>`;
    },
  },
});

/**
 * Safely parse markdown string to HTML.
 */
export function renderMarkdown(markdown: string): string {
  if (!markdown || !markdown.trim()) {
    return "";
  }
  try {
    const html = marked.parse(markdown);
    return typeof html === "string" ? html : "";
  } catch (err) {
    console.error("Failed to render markdown:", err);
    return `<div class="semaps-md-error">${escapeHtml(markdown)}</div>`;
  }
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
