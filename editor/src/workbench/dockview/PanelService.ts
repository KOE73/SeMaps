import type {
  DockviewApi,
  IDockviewPanel,
  IContentRenderer,
} from "dockview-core";
import type { IPanelService } from "../commands/types.js";

export interface PanelDescriptor {
  readonly id: string;
  readonly title: string;
  readonly icon?: string;
  readonly minWidth?: number;
  readonly minHeight?: number;
  createRenderer(): IContentRenderer;
}

/** Which panel is the centre and which ones share the right-hand group. */
export interface PanelLayoutIds {
  readonly centerId: string;
  readonly rightIds: readonly string[];
  /** Docks with their own side (the graph mode has one left and one right): a
   * panel of a group opens tabbed into that group, or on its side of the centre. */
  readonly groups?: readonly { readonly ids: readonly string[]; readonly side: "left" | "right"; readonly width?: number }[];
}

const EDITOR_IDS: PanelLayoutIds = {
  centerId: "diagram",
  rightIds: ["properties", "relations", "filters", "styles", "base", "neighbourhood"],
};

export class PanelService implements IPanelService {
  private readonly descriptors = new Map<string, PanelDescriptor>();
  private readonly listeners = new Set<() => void>();

  private dockview!: DockviewApi;

  constructor(private readonly ids: PanelLayoutIds = EDITOR_IDS) {}

  init(options: { dockview: DockviewApi; container?: HTMLElement }): void {
    this.dockview = options.dockview;

    this.dockview.onDidActivePanelChange(() => this.notify());
    this.dockview.onDidAddPanel(() => this.notify());
    this.dockview.onDidRemovePanel(() => this.notify());
  }

  register(descriptor: PanelDescriptor): void {
    this.descriptors.set(descriptor.id, descriptor);
  }

  getDescriptor(id: string): PanelDescriptor | undefined {
    return this.descriptors.get(id);
  }

  listDescriptors(): readonly PanelDescriptor[] {
    return Array.from(this.descriptors.values());
  }

  createRenderer(id: string): IContentRenderer {
    const desc = this.descriptors.get(id);
    if (desc === undefined) {
      const fallback = document.createElement("div");
      fallback.textContent = `Panel "${id}" not found`;
      return {
        element: fallback,
        init: () => {},
      };
    }
    return desc.createRenderer();
  }

  open(id: string): void {
    const desc = this.descriptors.get(id);
    if (!desc) {
      console.warn(`Cannot open unknown panel "${id}"`);
      return;
    }

    const existing = this.dockview.getPanel(id);
    if (existing !== undefined) {
      existing.api.setActive();
      return;
    }

    const diagram = this.dockview.getPanel(this.ids.centerId);

    if (id === "catalog") {
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        position: diagram ? { direction: "left", referencePanel: diagram } : undefined,
        initialWidth: 240,
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
      return;
    }

    // The unsaved changes open as a tab beside the catalog, else on its side of the diagram.
    if (id === "changes") {
      const catalog = this.dockview.getPanel("catalog");
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        position: catalog ? { direction: "within", referencePanel: catalog }
          : diagram ? { direction: "left", referencePanel: diagram } : undefined,
        initialWidth: 240,
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
      return;
    }

    const group = this.ids.groups?.find((g) => g.ids.includes(id));
    if (group) {
      const mate = group.ids.map((pid) => this.dockview.getPanel(pid)).find((p) => p !== undefined);
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        position: mate
          ? { direction: "within", referencePanel: mate }
          : diagram
            ? { direction: group.side, referencePanel: diagram }
            : undefined,
        ...(mate ? {} : { initialWidth: group.width ?? 320 }),
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
      return;
    }

    const rightPanels = this.ids.rightIds;
    let existingRight: IDockviewPanel | undefined;
    for (const pid of rightPanels) {
      const p = this.dockview.getPanel(pid);
      if (p !== undefined) {
        existingRight = p;
        break;
      }
    }

    if (existingRight !== undefined) {
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        position: { direction: "within", referencePanel: existingRight },
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
    } else if (diagram !== undefined) {
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        position: { direction: "right", referencePanel: diagram },
        initialWidth: 320,
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
    } else {
      this.dockview.addPanel({
        id: desc.id,
        component: desc.id,
        title: desc.title,
        minimumWidth: desc.minWidth ?? 100,
        minimumHeight: desc.minHeight ?? 80,
      });
    }
  }

  close(id: string): void {
    const panel = this.dockview.getPanel(id);
    if (panel !== undefined) {
      this.dockview.removePanel(panel);
    }
  }

  /** Shown → hide; hidden behind a tab or closed → show. Focus does not matter. */
  toggle(id: string): void {
    if (this.isVisible(id)) {
      this.close(id);
    } else {
      this.open(id);
    }
  }

  focus(id: string): void {
    const panel = this.dockview.getPanel(id);
    if (panel !== undefined) {
      panel.api.setActive();
    } else {
      this.open(id);
    }
  }

  isOpen(id: string): boolean {
    return this.dockview.getPanel(id) !== undefined;
  }

  isVisible(id: string): boolean {
    const panel = this.dockview.getPanel(id);
    return panel !== undefined && panel.api.isVisible;
  }

  getPanel(id: string): IDockviewPanel | undefined {
    return this.dockview.getPanel(id);
  }

  toggleLeftSidebar(): void {
    this.toggle("catalog");
  }

  toggleRightSidebar(): void {
    const rightPanels = this.ids.rightIds;
    const anyOpen = rightPanels.some((pid) => this.isOpen(pid));
    if (anyOpen) {
      for (const pid of rightPanels) {
        this.close(pid);
      }
    } else {
      this.open("properties");
    }
  }

  onPanelStateChange(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}
