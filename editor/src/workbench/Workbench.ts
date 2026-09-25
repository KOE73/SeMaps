import { el } from "../util/dom.js";
import { openCanvasMenu, type MenuHost } from "./menus/CanvasMenus.js";
import { DiagramEditor, type DiagramEditorOptions } from "../editor/DiagramEditor.js";
import { CommandRegistry } from "./commands/CommandRegistry.js";
import { createBuiltinCommands } from "./commands/builtinCommands.js";
import type { CommandContext, SelectionService } from "./commands/types.js";
import { ShortcutManager } from "./commands/ShortcutManager.js";
import { Ribbon } from "./ribbon/Ribbon.js";
import { createDefaultRibbonSpec, createDisplaySpec } from "./ribbon/RibbonModel.js";
import { DisplayPanel } from "./ribbon/DisplayPanel.js";
import type { WorkbenchMode } from "./modes.js";
import { DockviewHost } from "./dockview/DockviewHost.js";
import { CanvasFilterManager } from "./filters/CanvasFilterManager.js";
import { i18n } from "./i18n/I18nService.js";

/**
 * Workbench architecture root.
 *
 * Integrates:
 * - Ribbon (top command surface)
 * - Command Center (centralized actions)
 * - Fullscreen Dockview (multi-zone dockable panels around central diagram)
 * - Shortcut Manager (global hotkeys)
 * - Theme/Status/Persistence synchronization
 * - Modes: the diagram editor and whatever else is added (addMode), switched
 *   at the end of the tab row; the gear after them opens the display panel
 */
export class Workbench {
  readonly root: HTMLElement;
  readonly editor: DiagramEditor;
  readonly commands: CommandRegistry;
  readonly shortcuts: ShortcutManager;
  readonly ribbon: Ribbon;
  readonly dockviewHost: DockviewHost;
  readonly filterManager: CanvasFilterManager;
  readonly display: DisplayPanel;

  private readonly modes: WorkbenchMode[] = [];
  private mode: WorkbenchMode;
  private readonly surfaces: HTMLElement;

  constructor(
    private readonly hostElement: HTMLElement,
    options: DiagramEditorOptions,
  ) {
    // 1. Root Workbench Shell
    const root = el("div", {
      class: "workbench-shell",
      attrs: {
        style:
          "display: flex; flex-direction: column; width: 100%; height: 100%; overflow: hidden; background: var(--bg); color: var(--text);",
      },
    });
    this.root = root;

    // 2. Hidden host for DiagramEditor backend logic
    const editorMount = el("div", { attrs: { style: "display: none;" } });
    root.appendChild(editorMount);
    this.editor = new DiagramEditor(editorMount, options);

    // 3. Canvas Visual Modifiers & Filters Manager
    this.filterManager = new CanvasFilterManager(() => this.editor.canvas.hostElement);

    // Hook theme changes to synchronize canvas filter presets per theme
    const origApplyTheme = this.editor.applyTheme.bind(this.editor);
    this.editor.applyTheme = (theme: string) => {
      origApplyTheme(theme);
      this.filterManager.setTheme(theme);
    };

    // 4. Ribbon Container (top)
    const ribbonContainer = el("div", {
      class: "workbench-ribbon-container",
      attrs: { style: "flex-shrink: 0; z-index: 50;" },
    });

    // 5. Fullscreen Dockview Host (main area under Ribbon)
    const dockviewContainer = el("div", {
      class: "workbench-dockview-host dockview-theme-dark",
      attrs: {
        style:
          "flex: 1; min-width: 0; min-height: 0; width: 100%; height: 100%; position: relative; overflow: hidden;",
      },
    });

    // Every mode's surface lives here; one is shown at a time.
    this.surfaces = el("div", {
      class: "workbench-surfaces",
      attrs: { style: "flex: 1; min-width: 0; min-height: 0; position: relative; display: flex;" },
    });
    this.surfaces.appendChild(dockviewContainer);

    root.appendChild(ribbonContainer);
    root.appendChild(this.surfaces);

    // 6. Initialize Command Center
    this.commands = new CommandRegistry();
    this.commands.registerAll(createBuiltinCommands());

    // 7. Initialize Fullscreen Dockview Host
    this.dockviewHost = new DockviewHost(dockviewContainer, this.editor);

    // 8. Connect Command Context Provider
    this.commands.setContextProvider(() => this.createCommandContext());

    // 9. Initialize Global Shortcut Manager
    this.shortcuts = new ShortcutManager(this.commands, () => this.createCommandContext());

    // 10. Initialize Ribbon
    const ribbonSpec = createDefaultRibbonSpec();
    this.mode = {
      id: "",
      get title() { return i18n.d.ribbon.modes.diagrams; },
      tabs: ribbonSpec.tabs,
      surface: dockviewContainer,
      enter: () => this.editor.canvas.render(),
    };
    this.modes.push(this.mode);
    this.ribbon = new Ribbon(
      ribbonSpec,
      this.commands,
      () => this.createCommandContext(),
      {
        quickAccess: () => this.mode.id === "",
        trailing: () => this.renderTrailing(),
      },
    );
    ribbonContainer.appendChild(this.ribbon.element);
    this.display = new DisplayPanel(this.ribbon, createDisplaySpec(), this.commands, () =>
      this.filterManager.renderControls(),
    );

    // 11. Bind events
    this.bindEvents();

    // Auto layout canvas on window resize
    window.addEventListener("resize", () => {
      this.editor.canvas.render();
    });

    // Apply initial filter state to canvas
    setTimeout(() => {
      this.filterManager.apply();
    }, 50);

    // 12. Mount to host
    this.hostElement.appendChild(root);
  }

  /** Adds a mode after the diagrams; its button appears at the end of the tab row. */
  addMode(mode: WorkbenchMode): void {
    mode.surface.hidden = true;
    this.surfaces.appendChild(mode.surface);
    this.modes.push(mode);
    this.ribbon.render();
  }

  /** Shows a mode by id ("" — the diagrams); an unknown id opens the diagrams. */
  selectMode(id: string): void {
    const next = this.modes.find((m) => m.id === id) ?? this.modes[0]!;
    this.display.close();
    this.mode = next;
    for (const m of this.modes) m.surface.hidden = m !== next;
    this.shortcuts.enabled = next.id === "";
    this.ribbon.setSpec({ tabs: next.tabs });
    next.enter?.();
    const url = new URL(location.href);
    url.hash = next.id;
    history.replaceState(null, "", next.id ? url : url.href.replace(/#$/, ""));
  }

  /** The end of the tab row: the modes (when there is more than one), then the gear. */
  private renderTrailing(): HTMLElement[] {
    const out: HTMLElement[] = [];
    if (this.modes.length > 1) {
      const switcher = el("div", { class: "ribbon-modes", attrs: { role: "tablist" } });
      for (const m of this.modes) {
        switcher.appendChild(
          el("button", {
            class: "ribbon-mode-btn" + (m === this.mode ? " is-active" : ""),
            text: m.title,
            attrs: { role: "tab", "aria-selected": String(m === this.mode) },
            on: { click: () => this.selectMode(m.id) },
          }),
        );
      }
      out.push(switcher);
    }
    const gear = el("button", {
      class: "ribbon-gear-btn",
      text: "⚙",
      attrs: { title: i18n.d.ribbon.modes.display, "aria-label": i18n.d.ribbon.modes.display },
      on: { click: (e: MouseEvent) => this.display.toggle(e.currentTarget as HTMLElement) },
    });
    out.push(gear);
    return out;
  }

  private bindEvents(): void {
    // Right click: a box's family and look, a line's shape and style, the view's line shape.
    const panels = this.dockviewHost.panelService;
    const menuHost: MenuHost = {
      openPanel: (id) => panels.open(id),
      openStyleEditor: (styleId) => {
        panels.open("styles");
        this.editor.openStyle(styleId);
      },
      command: (id) => {
        const state = this.commands.getState(id);
        return {
          label: state.title,
          icon: state.icon,
          note: state.shortcut,
          disabled: !state.enabled,
          onSelect: () => void this.commands.execute(id),
        };
      },
    };
    this.editor.canvas.events.on("contextmenu", ({ target, id, clientX, clientY }) => {
      openCanvasMenu(this.editor, menuHost, target, id, clientX, clientY);
    });

    // Re-evaluate ribbon and commands on selection change
    this.editor.canvas.events.on("select", () => {
      this.commands.notifyStateChanged();
    });

    // Re-evaluate on model/history change
    this.editor.canvas.events.on("modelchange", () => {
      this.commands.notifyStateChanged();
    });

    // Opening a view or changing the workspace enables the view/project commands.
    this.editor.workspaceEvents.on("change", () => {
      this.commands.notifyStateChanged();
    });

    // Re-evaluate on panel visibility change
    this.dockviewHost.panelService.onPanelStateChange(() => {
      this.commands.notifyStateChanged();
      this.editor.canvas.render();
    });

    // Re-render ribbon and update panel titles on UI language change
    i18n.onLanguageChange(() => {
      this.commands.notifyStateChanged();
      this.ribbon.render();
      this.dockviewHost.updateTitles();
    });
  }

  private createCommandContext(): CommandContext {
    const canvas = this.editor.canvas;
    const selection = canvas.selected;

    const selectionService: SelectionService = {
      current: selection,
      ids: canvas.selectedIds,
      hasSelection: selection !== null,
      select: (id) => canvas.select(id),
      clear: () => canvas.select(null),
    };

    return {
      editor: this.editor,
      canvas,
      document: canvas.model,
      styles: this.editor.styles,
      history: this.editor.history,
      selection: selectionService,
      panels: this.dockviewHost.panelService,
      workspace: this.dockviewHost.layoutService,
      io: this.editor,
      dataLang: this.editor.dataLang,
      uiLang: i18n.currentLanguage,
      applyUiLang: (lang) => {
        i18n.setLanguage(lang as any);
      },
      toggleCanvasFilters: (anchor) => {
        this.filterManager.toggleFlyout(anchor);
      },
    };
  }
}
