import type { DockviewApi, SerializedDockview } from "dockview-core";

const STORAGE_KEY = "semaps:dockview-layout-v14";

export interface DockLayoutState {
  dockview?: SerializedDockview;
  visible?: boolean;
}

export class WorkspaceLayoutService {
  private dockview!: DockviewApi;
  private container!: HTMLElement;
  private defaultFactory!: () => void;

  init(
    host: { dockview: DockviewApi; container: HTMLElement },
    defaultFactory: () => void,
  ): void {
    this.dockview = host.dockview;
    this.container = host.container;
    this.defaultFactory = defaultFactory;
  }

  saveLayout(): void {
    if (!this.dockview) return;
    try {
      const state: DockLayoutState = {
        dockview: this.dockview.toJSON(),
        visible: !this.container.classList.contains("is-collapsed"),
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch (err) {
      console.warn("Failed to save dockview layout:", err);
    }
  }

  loadLayout(): boolean {
    if (!this.dockview) return false;
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return false;
    try {
      const parsed = JSON.parse(raw) as DockLayoutState;
      if (!parsed) return false;

      if (parsed.dockview && parsed.dockview.grid && parsed.dockview.panels) {
        this.dockview.fromJSON(parsed.dockview);
      }

      if (parsed.visible === false) {
        this.container.classList.add("is-collapsed");
        this.container.style.display = "none";
      } else {
        this.container.classList.remove("is-collapsed");
        this.container.style.display = "";
      }

      return true;
    } catch (err) {
      console.warn("Failed to restore dockview layout:", err);
      return false;
    }
  }

  resetLayout(): void {
    try {
      localStorage.removeItem(STORAGE_KEY);
      if (this.dockview) this.dockview.clear();
      this.container.classList.remove("is-collapsed");
      this.container.style.display = "";
      this.defaultFactory();
    } catch (err) {
      console.warn("Failed to reset dockview layout:", err);
    }
  }
}
