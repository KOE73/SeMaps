import type { DiagramCanvas, Selection } from "../../canvas/DiagramCanvas.js";
import type { DiagramDocument } from "../../model/document.js";
import type { StyleLibrary } from "../../model/StyleLibrary.js";
import type { History } from "../../editor/History.js";
import type { CatalogEntry } from "../../editor/io/types.js";

export interface IDisposable {
  dispose(): void;
}

export type Disposable = IDisposable | (() => void);

export function dispose(d: Disposable): void {
  if (typeof d === "function") {
    d();
  } else if (d && typeof d.dispose === "function") {
    d.dispose();
  }
}

export interface CommandState {
  readonly enabled: boolean;
  readonly visible: boolean;
  readonly checked?: boolean;
  readonly title: string;
  readonly description?: string;
  readonly icon?: string;
  readonly shortcut?: string;
  readonly keyTip?: string;
  readonly category?: string;
}

export interface CommandDefinition {
  readonly id: string;
  readonly title: string;
  readonly description?: string;
  readonly icon?: string;
  readonly category?: string;

  /** Global keyboard shortcut, e.g. "Ctrl+Z", "Ctrl+Shift+B", "Delete" */
  readonly shortcut?: string;

  /** Ribbon Alt navigation key tip, e.g. "B" */
  readonly keyTip?: string;

  /** Whether the shortcut is allowed to fire when focus is in an input/textarea */
  readonly allowInEditable?: boolean;

  /** Executes the command */
  execute(context: CommandContext, args?: unknown): void | Promise<void>;

  /** Determines whether the command is currently executable */
  isEnabled?(context: CommandContext): boolean;

  /** Determines whether the command is visible in UI representations */
  isVisible?(context: CommandContext): boolean;

  /** Determines checked/active state for toggles */
  isChecked?(context: CommandContext): boolean;
}

export interface SelectionService {
  readonly current: Selection | null;
  readonly ids: ReadonlySet<string>;
  readonly hasSelection: boolean;
  select(id: string | null): void;
  clear(): void;
}

export interface IPanelService {
  open(id: string): void;
  close(id: string): void;
  toggle(id: string): void;
  focus(id: string): void;
  isOpen(id: string): boolean;
  isVisible(id: string): boolean;
}

export interface IWorkspaceLayoutService {
  resetLayout(): void;
  saveLayout(): void;
}

export interface DiagramIoService {
  save(): Promise<void>;
  openFile(): void;
  exportDrawio(): void;
  toggleJsonModal(): void;
  copyJson(): Promise<void>;
  applyJson(): void;
}

export interface DiagramEditorFacade {
  readonly canvas: DiagramCanvas;
  readonly styles: StyleLibrary;
  readonly history: History;
  readonly isDirty: boolean;
  readonly isStylesDirty: boolean;
  readonly hasUnsavedChanges: boolean;
  dataLang: string;

  createNode(): void;
  createZone(): void;
  deleteSelection(): void;
  undo(): void;
  redo(): void;
  save(): Promise<void>;
  exportDrawio(): void;
  openFile(): void;
  toggleSidebar(): void;
  toggleJsonModal(): void;
  copyJson(): Promise<void>;
  applyJson(): void;
  applyToggle(name: string, on: boolean): void;
  applyPortAssigner(mode: string): void;
  openDocEditor(targetId?: string | null): void;
  openCodeViewer?(codeRef?: string | null, label?: string): void;
  applyTheme(theme: string): void;
  applyDataLang(lang: string): void;
  applyUiLang?(lang: string): void;
  openTab(tab: "properties" | "edges" | "filters" | "styles" | "base"): void;
  notify(message: string): void;
  openCatalogEntry(entry: CatalogEntry): Promise<void>;
  toggleOverviewShadows?(on?: boolean): boolean;
  isOverviewShadowsEnabled?(): boolean;
}

export interface CommandContext {
  readonly editor: DiagramEditorFacade;
  readonly canvas: DiagramCanvas;
  readonly document: DiagramDocument | null;
  readonly styles: StyleLibrary;
  readonly history: History;
  readonly selection: SelectionService;
  readonly panels: IPanelService;
  readonly workspace: IWorkspaceLayoutService;
  readonly io: DiagramIoService;
  readonly dataLang: string;
  readonly uiLang: string;
  applyUiLang(lang: string): void;
  toggleCanvasFilters?(anchor?: HTMLElement): void;
}
