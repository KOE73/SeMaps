import "../styles/dockview.css";
import "../styles/editor.css";
import "../styles/canvas.css";
import "../styles/ribbon.css";
import "../styles/canvas-filters.css";
import "../styles/doc-editor.css";
import "../styles/code-viewer.css";
import "../styles/shell.css";

import { Workbench } from "../workbench/Workbench.js";
import { HostModelStore, HttpProjectStore, HttpStyleStore, HttpWorkspaceStore } from "../editor/io/index.js";
import { toolApi } from "../shell/api.js";
import { addToolModes } from "./toolModes.js";

/**
 * Application entry point with Workbench Architecture.
 *
 * In dev the models directory is Vite's public dir, so models sit at the root.
 * The built app is served by the host (`host/`) at `/app/` from the tool's own
 * folder, and the workspace is served at the root — one level up.
 */
const MODELS_BASE = import.meta.env.DEV ? "./" : "../";

async function main(): Promise<void> {
  const root = document.getElementById("app");
  if (root === null) throw new Error("Missing #app root");

  const workbench = new Workbench(root, {
    workspace: new HttpWorkspaceStore(MODELS_BASE),
    store: import.meta.env.DEV ? new HttpProjectStore(MODELS_BASE) : new HostModelStore(MODELS_BASE),
    // styles.json sits with the models, not with the app bundle.
    styleStore: new HttpStyleStore(MODELS_BASE),
    // …and so do templates.json and the content directory, which the canvas
    // fetches for itself rather than through a store.
    modelsBase: MODELS_BASE,
  });

  // With a .semaps file the host also serves the tool modes; the hash of the
  // URL (#extract, #project) says which mode to open.
  const setup = await toolApi.setup().catch(() => undefined);
  if (setup) {
    document.title = `${setup.name || setup.projectFile} — SeMaps`;
    addToolModes(workbench);
  }
  if (!location.hash.startsWith("#v_")) workbench.selectMode(location.hash.slice(1));

  // Handy for console debugging and testing
  Object.assign(window, {
    semapsWorkbench: workbench,
    semapsEditor: workbench.editor,
    semapsCommands: workbench.commands,
  });
}

void main();
