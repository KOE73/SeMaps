import "../styles/dockview.css";
import "../styles/editor.css";
import "../styles/canvas.css";
import "../styles/ribbon.css";
import "../styles/canvas-filters.css";
import "../styles/doc-editor.css";
import "../styles/code-viewer.css";
import "../styles/shell.css";

import { Workbench } from "../workbench/Workbench.js";
import { HttpProjectStore, HttpStyleStore, HttpWorkspaceStore } from "../editor/io/index.js";
import { createTopBar } from "../shell/TopBar.js";

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

  // The top bar every page of the tool carries; the workbench lives under it.
  const body = document.createElement("div");
  body.className = "shell-body";
  root.append(createTopBar("editor").element, body);

  const workbench = new Workbench(body, {
    workspace: new HttpWorkspaceStore(MODELS_BASE),
    store: new HttpProjectStore(MODELS_BASE),
    // styles.json sits with the models, not with the app bundle.
    styleStore: new HttpStyleStore(MODELS_BASE),
    // …and so do templates.json and the content directory, which the canvas
    // fetches for itself rather than through a store.
    modelsBase: MODELS_BASE,
  });

  // Handy for console debugging and testing
  Object.assign(window, {
    semapsWorkbench: workbench,
    semapsEditor: workbench.editor,
    semapsCommands: workbench.commands,
  });
}

void main();
