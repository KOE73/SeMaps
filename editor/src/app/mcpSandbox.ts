import { el } from "../util/dom.js";
import { toolApi, type JsonSchema, type McpCallResult, type McpTool } from "../shell/api.js";
import { fmt, t } from "../shell/strings.js";

/**
 * The MCP sandbox: pick a tool, edit its arguments, call it on this project
 * through the host (POST /api/mcp/call) and read the JSON-RPC messages as they
 * went. Arguments start from the tool's own input schema; a writing tool asks
 * for a confirmation, since the call is real.
 */
export function mcpSandbox(tools: readonly McpTool[]): HTMLElement {
  const select = el("select", {}) as HTMLSelectElement;
  for (const tool of tools) {
    select.appendChild(el("option", { text: `${tool.name}  ·  ${tool.readOnly ? t.mcpRead : t.mcpWrite}`, attrs: { value: tool.name } }));
  }
  const description = el("p", { class: "tool-muted mcp-play-desc" });
  const params = el("table", { class: "tool-table mcp-play-params" });
  const args = el("textarea", { class: "mcp-play-args", attrs: { spellcheck: "false", rows: "6" } }) as HTMLTextAreaElement;
  const confirm = el("input", { type: "checkbox" }) as HTMLInputElement;
  const writeNote = el("label", { class: "tool-check mcp-play-write" }, [
    confirm,
    el("span", { text: `${t.mcpPlayWrite} ${t.mcpPlayWriteOk}` }),
  ]);
  const callBtn = el("button", { class: "tool-btn is-primary", text: t.mcpPlayCall });
  const status = el("span", { class: "tool-muted" });
  const result = el("div", { class: "mcp-play-result" });

  const current = (): McpTool | undefined => tools.find((tool) => tool.name === select.value);
  const drafts = new Map<string, string>(); // edited arguments, per tool

  const showTool = (): void => {
    const tool = current();
    if (!tool) return;
    description.textContent = tool.description;
    args.value = drafts.get(tool.name) ?? JSON.stringify(example(tool.inputSchema), null, 2);
    const props = Object.entries(tool.inputSchema.properties ?? {});
    const required = new Set(tool.inputSchema.required ?? []);
    params.replaceChildren(
      el("tbody", {}, props.map(([name, p]) =>
        el("tr", {}, [
          el("td", {}, [el("code", { text: name + (required.has(name) ? " *" : "") })]),
          el("td", { class: "tool-muted", text: typeName(p) }),
          el("td", { class: "tool-muted", text: p.description ?? "" }),
        ]),
      )),
    );
    params.hidden = props.length === 0;
    writeNote.hidden = tool.readOnly;
    confirm.checked = false;
    updateCall();
  };
  const updateCall = (): void => {
    callBtn.disabled = !current()?.readOnly && !confirm.checked;
  };

  select.addEventListener("change", showTool);
  confirm.addEventListener("change", updateCall);
  args.addEventListener("input", () => {
    const tool = current();
    if (tool) drafts.set(tool.name, args.value);
  });
  args.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !callBtn.disabled) callBtn.click();
  });

  callBtn.addEventListener("click", async () => {
    const tool = current();
    if (!tool) return;
    let parsed: unknown;
    try {
      parsed = args.value.trim() === "" ? {} : JSON.parse(args.value);
      if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("{…}");
    } catch (e) {
      status.className = "tool-error";
      status.textContent = fmt(t.mcpPlayBadJson, { error: (e as Error).message });
      return;
    }
    callBtn.disabled = true;
    status.className = "tool-muted";
    status.textContent = "…";
    try {
      const res = await toolApi.callMcp(tool.name, parsed as Record<string, unknown>);
      showResult(res, status, result);
    } catch (e) {
      status.className = "tool-error";
      status.textContent = fmt(t.mcpPlayFailed, { error: (e as Error).message });
      result.replaceChildren();
    } finally {
      confirm.checked = false;
      updateCall();
    }
  });

  showTool();
  return el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpPlayTitle })]),
    el("p", { class: "tool-hint", text: t.mcpPlayHint }),
    el("div", { class: "tool-form" }, [el("label", { text: t.mcpPlayTool }), select]),
    description,
    params,
    el("label", { class: "tool-muted mcp-play-label", text: t.mcpPlayArgs }),
    args,
    writeNote,
    el("div", { class: "tool-actions" }, [callBtn, status]),
    result,
  ]);
}

function showResult(res: McpCallResult, status: HTMLElement, target: HTMLElement): void {
  if (res.error) {
    status.className = "tool-error";
    status.textContent = fmt(t.mcpPlayFailed, { error: res.error });
  } else if (res.isError) {
    status.className = "tool-error";
    status.textContent = fmt(t.mcpPlayToolError, { ms: String(res.ms) });
  } else {
    status.className = "tool-badge is-ok";
    status.textContent = fmt(t.mcpPlayOk, { ms: String(res.ms) });
  }
  const blocks: HTMLElement[] = [];
  for (const m of res.messages) {
    blocks.push(
      el("div", { class: "mcp-play-dir", text: m.dir === "out" ? t.mcpPlayOut : t.mcpPlayIn }),
      el("pre", { class: "tool-log", text: JSON.stringify(m.message, null, 2) }),
    );
  }
  // The answer's text content is usually JSON itself: shown decoded, apart.
  const answer = res.messages.find((m) => m.dir === "in")?.message as
    | { result?: { content?: { type: string; text?: string }[] } }
    | undefined;
  const text = answer?.result?.content?.find((c) => c.type === "text")?.text;
  if (text !== undefined) {
    let pretty = text;
    try {
      pretty = JSON.stringify(JSON.parse(text), null, 2);
    } catch {
      // plain text stays as it is
    }
    blocks.push(el("div", { class: "mcp-play-dir", text: t.mcpPlayContent }), el("pre", { class: "tool-log", text: pretty }));
  }
  target.replaceChildren(...blocks);
}

/** A starting value for a schema: the required fields, with empty values of their type. */
function example(schema: JsonSchema): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const name of schema.required ?? []) {
    out[name] = emptyOf(schema.properties?.[name] ?? {});
  }
  return out;
}

function emptyOf(schema: JsonSchema): unknown {
  switch (primary(schema)) {
    case "boolean":
      return false;
    case "integer":
    case "number":
      return 0;
    case "array":
      return [];
    case "object":
      return example(schema);
    default:
      return "";
  }
}

function primary(schema: JsonSchema): string | undefined {
  const type = schema.type;
  return Array.isArray(type) ? type.find((x) => x !== "null") : type;
}

function typeName(schema: JsonSchema): string {
  const type = primary(schema) ?? "any";
  return type === "array" && schema.items ? `${primary(schema.items) ?? "any"}[]` : type;
}
