import { el } from "../util/dom.js";
import type { Workbench } from "../workbench/Workbench.js";
import type { WorkbenchMode } from "../workbench/modes.js";
import type { CommandDefinition } from "../workbench/commands/types.js";
import { focusNewExtractor, loadExtractors } from "./extract.js";
import { loadProject, saveProject } from "./setup.js";
import { installMcp, loadMcp } from "./mcp.js";
import { t } from "../shell/strings.js";

/**
 * The modes after the diagrams, present only when the host was started from a
 * .semaps file: Extractors, Project and MCP. Each is a page in a scrolling surface,
 * loaded when first entered, with a ribbon tab of its own.
 */
export function addToolModes(workbench: Workbench): void {
  const extract = page();
  const project = page();
  const mcp = page();

  const commands: CommandDefinition[] = [
    {
      id: "tools.extract.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: "⟳",
      execute: () => void loadExtractors(extract.inner),
    },
    {
      id: "tools.extract.add",
      title: t.addExtractor,
      icon: "➕",
      execute: () => focusNewExtractor(extract.inner),
    },
    {
      id: "tools.project.save",
      title: t.save,
      icon: "💾",
      execute: () => saveProject(project.inner),
    },
    {
      id: "tools.project.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: "⟳",
      execute: () => void loadProject(project.inner),
    },
    {
      id: "tools.mcp.install",
      title: t.mcpInstall,
      icon: "🔌",
      execute: () => void installMcp(mcp.inner),
    },
    {
      id: "tools.mcp.refresh",
      title: t.refresh,
      description: t.refreshHint,
      icon: "⟳",
      execute: () => void loadMcp(mcp.inner),
    },
  ];
  workbench.commands.registerAll(commands);

  workbench.addMode(mode("extract", t.navExtract, extract, () => loadExtractors(extract.inner), [
    { id: "extractors", title: t.navExtract, items: [
      { type: "button", command: "tools.extract.add", size: "large" },
      { type: "button", command: "tools.extract.refresh", size: "large" },
    ] },
  ]));
  workbench.addMode(mode("project", t.navSetup, project, () => loadProject(project.inner), [
    { id: "project", title: t.navSetup, items: [
      { type: "button", command: "tools.project.save", size: "large" },
      { type: "button", command: "tools.project.refresh", size: "large" },
    ] },
  ]));
  workbench.addMode(mode("mcp", t.navMcp, mcp, () => loadMcp(mcp.inner), [
    { id: "mcp", title: t.navMcp, items: [
      { type: "button", command: "tools.mcp.install", size: "large" },
      { type: "button", command: "tools.mcp.refresh", size: "large" },
    ] },
  ]));
}

interface Page {
  readonly surface: HTMLElement;
  readonly inner: HTMLElement;
}

function page(): Page {
  const inner = el("div", { class: "tool-page-inner" });
  const surface = el("main", { class: "tool-page" }, [inner]);
  return { surface, inner };
}

function mode(
  id: string,
  title: string,
  p: Page,
  load: () => Promise<void>,
  groups: WorkbenchMode["tabs"][number]["groups"],
): WorkbenchMode {
  let loaded = false;
  return {
    id,
    title,
    surface: p.surface,
    tabs: [{ id, title, groups }],
    enter: () => {
      if (loaded) return;
      loaded = true;
      void load();
    },
  };
}
