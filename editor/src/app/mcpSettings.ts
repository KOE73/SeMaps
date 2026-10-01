import { el } from "../util/dom.js";
import { toolApi, type GraphFormats, type McpCombo, type McpStatus, type Setup } from "../shell/api.js";
import { fmt, t } from "../shell/strings.js";

type ToolSet = McpStatus["mcpTools"];
type DescLevel = McpStatus["mcpDescription"];

/**
 * "Настройки для агента": the two axes that shape the MCP tool set (набор
 * инструментов, подробность описаний), the graph defaults (формат, порог
 * списка, предел узлов), a live preview of exactly what an agent would
 * receive at the combination currently picked in the form, and saving
 * through `PUT /api/setup`. All six `tools`×`description` combinations
 * (bytes, tokens, full tool text) come from `st.combos` already — no request
 * to the host as the radios change.
 *
 * `onSaved` reloads the whole MCP page after a successful save, so the
 * neighbouring cards (server tools table, snippet) show the new settings too.
 */
export function mcpSettingsSection(st: McpStatus, setup: Setup, formats: GraphFormats, onSaved: () => void): HTMLElement {
  const savedTools = st.mcpTools;
  const savedDescription = st.mcpDescription;
  const savedFormat = setup.mcp.format;
  const savedListCap = setup.mcp.listCap;
  const savedLimit = setup.mcp.limit;

  let curTools: ToolSet = savedTools;
  let curDescription: DescLevel = savedDescription;

  const comboFor = (tools: ToolSet, description: DescLevel): McpCombo => {
    const combo = st.combos.find((c) => c.tools === tools && c.description === description);
    if (!combo) throw new Error(`no combo for ${tools}/${description}`);
    return combo;
  };
  const costText = (c: McpCombo) => fmt(t.mcpCost, { bytes: String(c.bytes), tokens: String(c.estimateTokens) });

  // Tool set radios ---------------------------------------------------
  const toolsCosts = new Map<ToolSet, HTMLElement>();
  const toolsInputs = new Map<ToolSet, HTMLInputElement>();
  const toolsGroup = el(
    "div",
    { class: "mcp-set-options" },
    ([
      ["one", t.mcpToolSetOne, t.mcpToolSetOneFor],
      ["narrow", t.mcpToolSetNarrow, t.mcpToolSetNarrowFor],
    ] as const).map(([value, label, hint]) => {
      const input = el("input", { type: "radio", attrs: { name: "mcp-tools", value } }) as HTMLInputElement;
      input.checked = value === curTools;
      toolsInputs.set(value, input);
      const cost = el("span", { class: "tool-muted mcp-set-cost" });
      toolsCosts.set(value, cost);
      input.addEventListener("change", () => {
        if (input.checked) {
          curTools = value;
          update();
        }
      });
      return el("label", { class: "mcp-set-option" }, [
        input,
        el("div", {}, [el("div", { text: label }), el("div", { class: "tool-muted mcp-set-hint", text: hint }), cost]),
      ]);
    }),
  );

  // Description level radios ------------------------------------------
  const descCosts = new Map<DescLevel, HTMLElement>();
  const descInputs = new Map<DescLevel, HTMLInputElement>();
  const descGroup = el(
    "div",
    { class: "mcp-set-options" },
    ([
      ["brief", t.mcpDescBrief, t.mcpDescBriefHint],
      ["standard", t.mcpDescStandard, t.mcpDescStandardHint],
      ["full", t.mcpDescFull, t.mcpDescFullHint],
    ] as const).map(([value, label, hint]) => {
      const input = el("input", { type: "radio", attrs: { name: "mcp-description", value } }) as HTMLInputElement;
      input.checked = value === curDescription;
      descInputs.set(value, input);
      const cost = el("span", { class: "tool-muted mcp-set-cost" });
      descCosts.set(value, cost);
      input.addEventListener("change", () => {
        if (input.checked) {
          curDescription = value;
          update();
        }
      });
      return el("label", { class: "mcp-set-option" }, [
        input,
        el("div", {}, [el("div", { text: label }), el("div", { class: "tool-muted mcp-set-hint", text: hint }), cost]),
      ]);
    }),
  );

  // Graph defaults ------------------------------------------------------
  const formatSelect = el("select", {}) as HTMLSelectElement;
  for (const f of formats.formats) formatSelect.appendChild(el("option", { text: f.name, attrs: { value: f.name } }));
  formatSelect.value = savedFormat;
  const listCap = el("input", { type: "number", value: String(savedListCap), attrs: { min: "1" } }) as HTMLInputElement;
  const limit = el("input", { type: "number", value: String(savedLimit), attrs: { min: "1" } }) as HTMLInputElement;

  // Preview --------------------------------------------------------------
  const previewSize = el("span", { class: "tool-badge" });
  const previewBody = el("pre", { class: "tool-log mcp-set-preview" });

  // Save -------------------------------------------------------------
  const unsaved = el("span", { class: "tool-badge is-bad", text: t.mcpUnsaved });
  const status = el("span", { class: "tool-muted" });
  const save = el("button", { class: "tool-btn is-primary mcp-settings-save", text: t.save });
  const note = el("ul", { class: "mcp-list mcp-set-note", attrs: { hidden: "" } }, t.mcpSaveNote.map((line) => el("li", { text: line })));

  const isDirty = () =>
    curTools !== savedTools ||
    curDescription !== savedDescription ||
    formatSelect.value !== savedFormat ||
    Number(listCap.value) !== savedListCap ||
    Number(limit.value) !== savedLimit;

  const update = (): void => {
    for (const [value, cost] of toolsCosts) cost.textContent = costText(comboFor(value, curDescription));
    for (const [value, cost] of descCosts) cost.textContent = costText(comboFor(curTools, value));
    for (const [value, input] of toolsInputs) input.checked = value === curTools;
    for (const [value, input] of descInputs) input.checked = value === curDescription;

    const combo = comboFor(curTools, curDescription);
    previewSize.textContent = fmt(t.mcpPreviewSize, { bytes: String(combo.bytes), tokens: String(combo.estimateTokens) });
    const lines: string[] = [combo.instructions.trim(), ""];
    for (const tool of combo.graphTools) {
      lines.push(`## ${tool.name}`, tool.description, "");
    }
    previewBody.textContent = lines.join("\n").trimEnd();

    const dirty = isDirty();
    unsaved.hidden = !dirty;
    save.disabled = !dirty;
  };

  formatSelect.addEventListener("change", update);
  listCap.addEventListener("input", update);
  limit.addEventListener("input", update);

  save.addEventListener("click", async () => {
    save.disabled = true;
    status.className = "tool-muted";
    status.textContent = "";
    note.hidden = true;
    try {
      const listCapNum = Number(listCap.value);
      const limitNum = Number(limit.value);
      await toolApi.saveSettings({
        mcp: {
          tools: curTools,
          description: curDescription,
          format: formatSelect.value,
          listCap: Number.isFinite(listCapNum) && listCapNum > 0 ? listCapNum : savedListCap,
          limit: Number.isFinite(limitNum) && limitNum > 0 ? limitNum : savedLimit,
        },
      });
      status.className = "tool-badge is-ok";
      status.textContent = t.mcpSaved;
      note.hidden = false;
      onSaved();
    } catch (e) {
      status.className = "tool-error";
      status.textContent = (e as Error).message;
      save.disabled = !isDirty();
    }
  });

  update();

  return el("section", { class: "tool-card mcp-set" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.mcpSettingsTitle })]),
    el("p", { class: "tool-hint", text: t.mcpSettingsHint }),
    el("div", { class: "mcp-set-grid" }, [
      el("div", {}, [
        el("h3", { text: t.mcpToolSet }),
        toolsGroup,
        el("h3", { text: t.mcpDescLevel }),
        descGroup,
        el("h3", { text: t.mcpDefaultsTitle }),
        el("div", { class: "tool-form" }, [
          el("label", { text: t.mcpDefaultFormat }),
          formatSelect,
        ]),
        el("p", { class: "tool-hint mcp-set-field-hint", text: t.mcpDefaultFormatHint }),
        el("div", { class: "tool-form" }, [
          el("label", { text: t.mcpDefaultListCap }),
          listCap,
        ]),
        el("p", { class: "tool-hint mcp-set-field-hint", text: t.mcpDefaultListCapHint }),
        el("div", { class: "tool-form" }, [
          el("label", { text: t.mcpDefaultLimit }),
          limit,
        ]),
        el("p", { class: "tool-hint mcp-set-field-hint", text: t.mcpDefaultLimitHint }),
        el("div", { class: "tool-actions" }, [save, unsaved, status]),
        el("div", {}, [el("h3", { text: t.mcpSaveTitle }), note]),
      ]),
      el("div", {}, [
        el("div", { class: "tool-card-head" }, [el("h3", { text: t.mcpPreviewTitle }), previewSize]),
        el("p", { class: "tool-hint", text: t.mcpPreviewHint }),
        previewBody,
      ]),
    ]),
  ]);
}
