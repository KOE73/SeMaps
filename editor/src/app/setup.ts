import { el } from "../util/dom.js";
import { toolApi, type SettingsPatch, type Setup, type Tools } from "../shell/api.js";
import { fmt, t } from "../shell/strings.js";

/**
 * The Project mode: the project's .semaps settings and what the host finds on
 * this machine. Extractors have their own mode.
 */
export async function loadProject(inner: HTMLElement): Promise<void> {
  inner.replaceChildren(el("p", { class: "tool-muted", text: t.loading }));
  const setup = await toolApi.setup().catch(() => undefined);
  if (!setup) {
    inner.replaceChildren(el("p", { class: "tool-error", text: t.noProjectFile }));
    return;
  }
  const tools = await toolApi.tools().catch(() => undefined);
  render(inner, setup, tools);
}

/** Saves the form, as its own button does. */
export function saveProject(inner: HTMLElement): void {
  inner.querySelector<HTMLButtonElement>(".tool-btn.is-primary")?.click();
}

function render(inner: HTMLElement, setup: Setup, tools: Tools | undefined): void {
  const field = (value: string, type = "text") => el("input", { type, value });
  const name = field(setup.name);
  const workspace = field(setup.workspace);
  const sourceRoot = field(setup.sourceRoot);
  const port = field(setup.port ? String(setup.port) : "", "number");
  const status = el("span", { class: "tool-muted" });
  const save = el("button", { class: "tool-btn is-primary", text: t.save });
  save.addEventListener("click", async () => {
    save.disabled = true;
    status.className = "tool-muted";
    status.textContent = "";
    try {
      // Only what was changed: a key nobody touched stays as it is written.
      const patch: SettingsPatch = {};
      if (name.value !== setup.name) patch.name = name.value;
      if (workspace.value !== setup.workspace) patch.workspace = workspace.value;
      if (sourceRoot.value !== setup.sourceRoot) patch.sourceRoot = sourceRoot.value;
      if (port.value.trim() !== "" && Number(port.value) !== setup.port) patch.port = Number(port.value);
      const next = await toolApi.saveSettings(patch);
      render(inner, next, tools);
      document.title = `${next.name || next.projectFile} — SeMaps`;
      return;
    } catch (e) {
      status.className = "tool-error";
      status.textContent = (e as Error).message;
    } finally {
      save.disabled = false;
    }
  });

  const settings = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-form" }, [
      el("label", { text: t.name }),
      name,
      el("label", { text: t.workspace }),
      workspace,
      el("label", { text: t.sourceRoot }),
      sourceRoot,
      el("label", { text: t.port }),
      port,
    ]),
    el("div", { class: "tool-actions" }, [save, status]),
  ]);

  const toolRows: HTMLElement[] = [];
  if (tools) {
    toolRows.push(
      el("tr", {}, [
        el("td", { class: "tool-muted", text: t.shipped }),
        el("td", { text: tools.shipped.length ? tools.shipped.join(", ") : t.none }),
      ]),
    );
    for (const r of tools.runtimes) {
      toolRows.push(
        el("tr", {}, [
          el("td", { class: "tool-muted", text: r.name }),
          el("td", {}, [
            el("span", { class: "tool-badge " + (r.ok ? "is-ok" : "is-bad"), text: r.ok ? r.version || "ok" : t.runtimeMissing }),
            r.ok || !r.hint ? null : el("span", { class: "tool-muted", text: "  " + r.hint }),
          ]),
        ]),
      );
    }
  }
  const toolsCard = el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [el("h2", { text: t.toolsTitle })]),
    el("table", { class: "tool-table" }, [el("tbody", {}, toolRows)]),
  ]);

  inner.replaceChildren(
    el("h1", { text: t.setupTitle }),
    el("p", { class: "tool-hint", text: fmt(t.setupHint, { file: setup.projectFile }) }),
    settings,
    toolsCard,
  );
}
