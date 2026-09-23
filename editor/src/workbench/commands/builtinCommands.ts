import type { CommandDefinition } from "./types.js";
import { i18n } from "../i18n/I18nService.js";

const ICONS = {
  SAVE: `<svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor"><path d="M2 1a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V4.5L11.5 1H2zm1 1h7.5L13 4.5V14H3V2zm2 1v3h6V3H5zm1 1h4v1H6V4zm-1 5v4h6V9H5zm1 1h4v2H6v-2z"/></svg>`,
  UNDO: `<svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor"><path d="M4.8 4L1.5 7.3l3.3 3.3V8.2c3.5 0 6.2 1.3 7.7 4.3-.5-4.2-3.1-6.5-7.7-6.5V4z"/></svg>`,
  REDO: `<svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor"><path d="M11.2 4l3.3 3.3-3.3 3.3V8.2c-3.5 0-6.2 1.3-7.7 4.3.5-4.2 3.1-6.5 7.7-6.5V4z"/></svg>`,
};

export function createBuiltinCommands(): CommandDefinition[] {
  return [
    // ----------------------------------------------------------------- File
    {
      id: "file.save",
      get title() { return i18n.d.commands.save.title; },
      get description() { return i18n.d.commands.save.desc; },
      icon: ICONS.SAVE,
      category: "File",
      shortcut: "Ctrl+S",
      keyTip: "S",
      isEnabled: (ctx) => ctx.editor.hasUnsavedChanges,
      execute: (ctx) => ctx.io.save(),
    },
    {
      id: "file.open",
      get title() { return i18n.d.commands.openFile.title; },
      get description() { return i18n.d.commands.openFile.desc; },
      icon: "📂",
      category: "File",
      shortcut: "Ctrl+O",
      keyTip: "O",
      execute: (ctx) => ctx.io.openFile(),
    },
    {
      id: "file.export.drawio",
      get title() { return i18n.d.commands.exportDrawio.title; },
      get description() { return i18n.d.commands.exportDrawio.desc; },
      icon: "📐",
      category: "File",
      keyTip: "D",
      isEnabled: (ctx) => ctx.document !== null,
      execute: (ctx) => ctx.io.exportDrawio(),
    },
    {
      id: "file.code.toggle",
      get title() { return i18n.d.commands.openJson.title; },
      get description() { return i18n.d.commands.openJson.desc; },
      icon: "📄",
      category: "File",
      keyTip: "J",
      isEnabled: (ctx) => ctx.document !== null,
      execute: (ctx) => ctx.io.toggleJsonModal(),
    },

    // ----------------------------------------------------------------- Edit
    {
      id: "edit.undo",
      get title() { return i18n.d.commands.undo.title; },
      get description() { return i18n.d.commands.undo.desc; },
      icon: ICONS.UNDO,
      category: "Edit",
      shortcut: "Ctrl+Z",
      keyTip: "U",
      isEnabled: (ctx) => ctx.history.canUndo,
      execute: (ctx) => ctx.editor.undo(),
    },
    {
      id: "edit.redo",
      get title() { return i18n.d.commands.redo.title; },
      get description() { return i18n.d.commands.redo.desc; },
      icon: ICONS.REDO,
      category: "Edit",
      shortcut: "Ctrl+Y, Ctrl+Shift+Z",
      keyTip: "R",
      isEnabled: (ctx) => ctx.history.canRedo,
      execute: (ctx) => ctx.editor.redo(),
    },
    {
      id: "edit.delete",
      get title() { return i18n.d.commands.deleteSelection.title; },
      get description() { return i18n.d.commands.deleteSelection.desc; },
      icon: "🗑",
      category: "Edit",
      shortcut: "Delete, Backspace",
      keyTip: "X",
      isEnabled: (ctx) => ctx.selection.hasSelection,
      execute: (ctx) => ctx.editor.deleteSelection(),
    },
    {
      id: "diagram.doc.edit",
      get title() { return i18n.d.dialogs.docEditor.openEditorBtn; },
      get description() { return i18n.d.dialogs.docEditor.editDocTooltip; },
      icon: "📄",
      category: "Edit",
      shortcut: "F2, Ctrl+D",
      keyTip: "M",
      isEnabled: (ctx) => ctx.selection.hasSelection,
      execute: (ctx) => ctx.editor.openDocEditor(),
    },
    {
      id: "diagram.code.view",
      title: "Просмотр исходного кода",
      description: "Открыть просмотрщик исходного кода для выбранного элемента",
      icon: "💻",
      category: "Edit",
      shortcut: "F3, Ctrl+Alt+C",
      keyTip: "C",
      isEnabled: (ctx) => {
        const sel = ctx.canvas.selectedElement();
        return Boolean(sel && typeof sel.metadata?.codeRef === "string" && sel.metadata.codeRef.trim());
      },
      execute: (ctx) => ctx.editor.openCodeViewer?.(),
    },

    // -------------------------------------------------------------- Diagram
    {
      id: "diagram.block.add",
      get title() { return i18n.d.commands.createNode.title; },
      get description() { return i18n.d.commands.createNode.desc; },
      icon: "➕",
      category: "Diagram",
      shortcut: "Ctrl+Shift+B",
      keyTip: "B",
      isEnabled: (ctx) => ctx.document !== null,
      execute: (ctx) => ctx.editor.createNode(),
    },
    {
      id: "diagram.zone.add",
      get title() { return i18n.d.commands.createZone.title; },
      get description() { return i18n.d.commands.createZone.desc; },
      icon: "📦",
      category: "Diagram",
      shortcut: "Ctrl+Shift+Z",
      keyTip: "Z",
      isEnabled: (ctx) => ctx.document !== null,
      execute: (ctx) => ctx.editor.createZone(),
    },
    {
      id: "diagram.ports.set",
      get title() { return i18n.d.commands.portsSet.title; },
      get description() { return i18n.d.commands.portsSet.desc; },
      icon: "⚓",
      category: "Diagram",
      keyTip: "P",
      execute: (ctx, args) => ctx.editor.applyPortAssigner(String(args || "uniform")),
    },

    // ----------------------------------------------------------------- View
    {
      id: "view.grid.toggle",
      get title() { return i18n.d.commands.toggleGrid.title; },
      get description() { return i18n.d.commands.toggleGrid.desc; },
      icon: "▦",
      category: "View",
      shortcut: "Ctrl+'",
      keyTip: "G",
      isChecked: (ctx) => ctx.canvas.hostElement.classList.contains("with-grid"),
      execute: (ctx) => {
        const next = !ctx.canvas.hostElement.classList.contains("with-grid");
        ctx.editor.applyToggle("grid", next);
      },
    },
    {
      id: "view.snap.toggle",
      get title() { return i18n.d.commands.toggleSnap.title; },
      get description() { return i18n.d.commands.toggleSnap.desc; },
      icon: "🧲",
      category: "View",
      shortcut: "Ctrl+Shift+S",
      keyTip: "N",
      isChecked: (ctx) => ctx.canvas.gridStep > 0,
      execute: (ctx) => {
        const next = ctx.canvas.gridStep === 0;
        ctx.editor.applyToggle("snap", next);
      },
    },
    {
      id: "view.filters.visual.toggle",
      get title() { return i18n.d.commands.toggleCanvasFilters.title; },
      get description() { return i18n.d.commands.toggleCanvasFilters.desc; },
      icon: "✨",
      category: "View",
      keyTip: "E",
      execute: (ctx, args) => {
        if (ctx.toggleCanvasFilters) {
          ctx.toggleCanvasFilters(args as HTMLElement | undefined);
        }
      },
    },
    {
      id: "view.shadows.global.toggle",
      get title() { return i18n.d.commands.toggleOverviewShadows.title; },
      get description() { return i18n.d.commands.toggleOverviewShadows.desc; },
      icon: "👻",
      category: "View",
      shortcut: "Ctrl+Shift+G",
      keyTip: "G",
      isChecked: (ctx) => Boolean(ctx.editor.isOverviewShadowsEnabled?.() ?? ctx.canvas.isOverviewShadowsEnabled),
      execute: (ctx) => {
        if (ctx.editor.toggleOverviewShadows) {
          ctx.editor.toggleOverviewShadows();
        } else {
          ctx.canvas.toggleOverviewShadows();
        }
      },
    },
    {
      id: "view.zoom.in",
      get title() { return i18n.d.commands.zoomIn.title; },
      get description() { return i18n.d.commands.zoomIn.desc; },
      icon: "➕",
      category: "View",
      shortcut: "Ctrl+=",
      keyTip: "+",
      execute: (ctx) => ctx.canvas.zoomBy(1.2),
    },
    {
      id: "view.zoom.out",
      get title() { return i18n.d.commands.zoomOut.title; },
      get description() { return i18n.d.commands.zoomOut.desc; },
      icon: "➖",
      category: "View",
      shortcut: "Ctrl+-",
      keyTip: "-",
      execute: (ctx) => ctx.canvas.zoomBy(0.8),
    },
    {
      id: "view.zoom.reset",
      get title() { return i18n.d.commands.zoomReset.title; },
      get description() { return i18n.d.commands.zoomReset.desc; },
      icon: "🔍",
      category: "View",
      shortcut: "Ctrl+0",
      keyTip: "0",
      execute: (ctx) => ctx.canvas.resetZoom(),
    },
    {
      id: "view.zoom.fit",
      get title() { return i18n.d.commands.fitToScreen.title; },
      get description() { return i18n.d.commands.fitToScreen.desc; },
      icon: "⛶",
      category: "View",
      shortcut: "Ctrl+Shift+0",
      keyTip: "F",
      execute: (ctx) => ctx.canvas.fit(),
    },
    {
      id: "view.theme.set",
      get title() { return i18n.d.ribbon.labels.theme; },
      description: "Переключить цветовую тему оформления",
      icon: "🎨",
      category: "View",
      keyTip: "T",
      execute: (ctx, args) => ctx.editor.applyTheme(String(args || "cream")),
    },
    {
      id: "view.uiLang.set",
      get title() { return i18n.d.ribbon.labels.uiLang; },
      description: "Переключить язык интерфейса редактора (RU / EN)",
      icon: "🌐",
      category: "View",
      keyTip: "U",
      execute: (ctx, args) => ctx.applyUiLang(String(args || "ru")),
    },
    {
      id: "view.lang.set",
      get title() { return i18n.d.ribbon.labels.dataLang; },
      description: "Переключить язык данных схемы (RU / EN)",
      icon: "🗂️",
      category: "View",
      keyTip: "K",
      execute: (ctx, args) => ctx.editor.applyDataLang(String(args || "ru")),
    },
    {
      id: "view.edges.toggleStructure",
      get title() { return i18n.d.commands.toggleStructureEdges.title; },
      get description() { return i18n.d.commands.toggleStructureEdges.desc; },
      icon: "🔗",
      category: "View",
      keyTip: "E",
      isChecked: (ctx) => !ctx.canvas.isEdgeFamilyHidden("structure"),
      execute: (ctx) => {
        const isHidden = ctx.canvas.isEdgeFamilyHidden("structure");
        ctx.editor.applyToggle("structure-edges", isHidden);
      },
    },

    // --------------------------------------------------------------- Panels
    {
      id: "panel.properties.toggle",
      get title() { return i18n.d.panels.properties.title; },
      get description() { return i18n.d.commands.toggleProperties.desc; },
      icon: "⚙️",
      category: "Panels",
      shortcut: "Alt+1",
      keyTip: "P1",
      isChecked: (ctx) => ctx.panels.isOpen("properties"),
      execute: (ctx) => ctx.panels.toggle("properties"),
    },
    {
      id: "panel.relations.toggle",
      get title() { return i18n.d.panels.relations.title; },
      get description() { return i18n.d.commands.toggleRelations.desc; },
      icon: "🔗",
      category: "Panels",
      shortcut: "Alt+2",
      keyTip: "P2",
      isChecked: (ctx) => ctx.panels.isOpen("relations"),
      execute: (ctx) => ctx.panels.toggle("relations"),
    },
    {
      id: "panel.filters.toggle",
      get title() { return i18n.d.panels.filters.title; },
      get description() { return i18n.d.commands.toggleFilters.desc; },
      icon: "🔍",
      category: "Panels",
      shortcut: "Alt+3",
      keyTip: "P3",
      isChecked: (ctx) => ctx.panels.isOpen("filters"),
      execute: (ctx) => ctx.panels.toggle("filters"),
    },
    {
      id: "panel.styles.toggle",
      get title() { return i18n.d.panels.styles.title; },
      get description() { return i18n.d.commands.toggleStyles.desc; },
      icon: "🎨",
      category: "Panels",
      shortcut: "Alt+4",
      keyTip: "P4",
      isChecked: (ctx) => ctx.panels.isOpen("styles"),
      execute: (ctx) => ctx.panels.toggle("styles"),
    },
    {
      id: "panel.catalog.toggle",
      get title() { return i18n.d.panels.catalog.title; },
      get description() { return i18n.d.commands.toggleCatalog.desc; },
      icon: "📚",
      category: "Panels",
      shortcut: "Alt+5",
      keyTip: "P5",
      isChecked: (ctx) => ctx.panels.isOpen("catalog"),
      execute: (ctx) => ctx.panels.toggle("catalog"),
    },
    {
      id: "panel.base.toggle",
      get title() { return i18n.d.panels.base.title; },
      get description() { return i18n.d.commands.toggleBase.desc; },
      icon: "🗄️",
      category: "Panels",
      shortcut: "Alt+6",
      keyTip: "P6",
      isChecked: (ctx) => ctx.panels.isOpen("base"),
      execute: (ctx) => ctx.panels.toggle("base"),
    },

    // ------------------------------------------------------------ Workspace
    {
      id: "workspace.layout.reset",
      get title() { return i18n.d.commands.resetLayout.title; },
      get description() { return i18n.d.commands.resetLayout.desc; },
      icon: "🔄",
      category: "Workspace",
      keyTip: "WR",
      execute: (ctx) => ctx.workspace.resetLayout(),
    },
  ];
}
