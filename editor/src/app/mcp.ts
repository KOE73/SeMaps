import { el } from "../util/dom.js";
import { toolApi, type McpStatus } from "../shell/api.js";
import { fmt, t } from "../shell/strings.js";
import { mcpSandbox } from "./mcpSandbox.js";

/**
 * The MCP mode: what `semaps mcp` gives an agent, whether this project's
 * .mcp.json starts it, the one action there is — writing that entry — and a
 * sandbox that calls the tools as an agent would.
 */
export async function loadMcp(inner: HTMLElement): Promise<void> {
  inner.replaceChildren(el("p", { class: "tool-muted", text: t.loading }));
  try {
    render(inner, await toolApi.mcp());
  } catch (e) {
    inner.replaceChildren(el("p", { class: "tool-error", text: (e as Error).message }));
  }
}

/** Writes the semaps entry into .mcp.json, then shows the new state. */
export async function installMcp(inner: HTMLElement): Promise<void> {
  try {
    render(inner, await toolApi.installMcp());
  } catch (e) {
    inner.querySelector(".mcp-install-error")?.replaceChildren((e as Error).message);
  }
}

function render(inner: HTMLElement, st: McpStatus): void {
  const hero = el("section", { class: "mcp-hero" }, [
    el("div", { class: "mcp-hero-mark", text: "🤖" }),
    el("div", {}, [el("h1", { text: t.mcpTitle }), el("p", { class: "tool-hint", text: t.mcpLead })]),
  ]);

  const why = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpWhyTitle })]),
    el("ul", { class: "mcp-list" }, t.mcpWhy.map((line) => el("li", { text: line }))),
  ]);

  const stateRows: HTMLElement[] = [];
  if (st.error) {
    stateRows.push(el("p", { class: "tool-error", text: st.error }));
  } else if (st.configured) {
    stateRows.push(
      el("p", {}, [
        el("span", { class: "tool-badge is-ok", text: "✓" }),
        el("span", { text: " " + fmt(t.mcpConfigured, { file: st.file, entry: st.entry }) }),
      ]),
    );
  } else {
    const install = el("button", { class: "tool-btn is-primary", text: t.mcpInstall });
    install.addEventListener("click", () => {
      install.disabled = true;
      void installMcp(inner).finally(() => (install.disabled = false));
    });
    stateRows.push(
      el("p", {}, [
        el("span", { class: "tool-badge is-bad", text: "—" }),
        el("span", { text: " " + fmt(t.mcpNotConfigured, { file: st.file }) }),
      ]),
      el("div", { class: "tool-actions" }, [
        install,
        el("span", { class: "tool-muted", text: fmt(t.mcpInstallHint, { file: st.file }) }),
      ]),
      el("p", { class: "tool-error mcp-install-error" }),
    );
  }
  if (!st.onPath) stateRows.push(el("p", { class: "tool-error", text: t.mcpNotOnPath }));

  const copy = el("button", { class: "tool-btn", text: t.mcpCopy });
  copy.addEventListener("click", () => {
    void navigator.clipboard?.writeText(st.snippet).then(() => {
      copy.textContent = t.mcpCopied;
      setTimeout(() => (copy.textContent = t.mcpCopy), 1500);
    });
  });
  const state = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpStateTitle })]),
    ...stateRows,
    el("div", { class: "tool-card-head mcp-manual" }, [
      el("span", { class: "tool-muted", text: fmt(t.mcpManual, { file: st.file }) }),
      el("span", { class: "spacer" }),
      copy,
    ]),
    el("pre", { class: "tool-log", text: st.snippet }),
  ]);

  const ask = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpAskTitle })]),
    el("ul", { class: "mcp-list mcp-ask" }, t.mcpAsk.map((line) => el("li", { text: line }))),
  ]);

  const tools = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpToolsTitle }), el("span", { class: "tool-badge", text: String(st.tools.length) })]),
    el(
      "table",
      { class: "tool-table" },
      [el("tbody", {}, st.tools.map((tool) =>
        el("tr", {}, [el("td", {}, [el("code", { text: tool.name })]), el("td", { class: "tool-muted", text: tool.description })]),
      ))],
    ),
  ]);

  inner.replaceChildren(hero, state, why, ask, mcpSandbox(st.tools), tools);
}
