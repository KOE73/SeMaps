import type { CommandRegistry } from "../commands/CommandRegistry.js";
import type { CommandContext } from "../commands/types.js";
import type { RibbonSpec } from "./types.js";
import { RibbonRenderer } from "./RibbonRenderer.js";
import { TooltipManager } from "./TooltipManager.js";
import { KeyTipsManager } from "./KeyTipsManager.js";
import { el, replaceChildren } from "../../util/dom.js";

export class Ribbon {
  readonly element: HTMLElement;
  readonly tooltips: TooltipManager;
  readonly keyTips: KeyTipsManager;

  private readonly tabsHeaderContainer: HTMLElement;
  private readonly tabPageContainer: HTMLElement;
  private readonly renderer: RibbonRenderer;
  private activeTabId = "home";

  constructor(
    private readonly spec: RibbonSpec,
    registry: CommandRegistry,
    private readonly getContext: () => CommandContext,
  ) {
    this.renderer = new RibbonRenderer(registry, getContext);

    this.tabsHeaderContainer = el("div", { class: "ribbon-header-row" });
    this.tabPageContainer = el("div", { class: "ribbon-page-row" });

    this.element = el(
      "div",
      {
        class: "workbench-ribbon",
        attrs: {
          style: "display: flex; flex-direction: column; background: var(--panel, #252526); border-bottom: 1px solid var(--line, #3a3a3c); flex-shrink: 0; z-index: 50;",
        },
      },
      [this.tabsHeaderContainer, this.tabPageContainer],
    );

    this.tooltips = new TooltipManager(registry);
    this.keyTips = new KeyTipsManager(this, registry);

    registry.onStateChanged(() => {
      this.updateContextualTabs();
      this.render();
    });

    this.render();
  }

  selectTab(id: string): void {
    this.activeTabId = id;
    this.render();
  }

  private updateContextualTabs(): void {
    const ctx = this.getContext();
    const kind = ctx.selection.current?.kind;

    if (kind === "node") {
      // Auto-suggest contextual tab if not currently in a core editing flow
      if (this.activeTabId.startsWith("context_") && this.activeTabId !== "context_node") {
        this.activeTabId = "context_node";
      }
    } else if (kind === "zone") {
      if (this.activeTabId.startsWith("context_") && this.activeTabId !== "context_zone") {
        this.activeTabId = "context_zone";
      }
    } else if (kind === "edge") {
      if (this.activeTabId.startsWith("context_") && this.activeTabId !== "context_edge") {
        this.activeTabId = "context_edge";
      }
    } else {
      // No selection: if we were on a contextual tab, return to home
      if (this.activeTabId.startsWith("context_")) {
        this.activeTabId = "home";
      }
    }
  }

  render(): void {
    const activeTab = this.spec.tabs.find((t) => t.id === this.activeTabId) || this.spec.tabs[0];
    if (!activeTab) return;

    // 1. Render Tabs Bar
    const header = this.renderer.renderTabsHeader(this.spec.tabs, this.activeTabId, (id) => {
      this.selectTab(id);
    });
    replaceChildren(this.tabsHeaderContainer, header);

    // 2. Render Active Tab Page
    const page = this.renderer.renderTabContent(activeTab);
    replaceChildren(this.tabPageContainer, page);
  }

  getTabHeaderNodes(): { id: string; node: HTMLElement; keyTip?: string }[] {
    const nodes: { id: string; node: HTMLElement; keyTip?: string }[] = [];
    for (const btn of this.tabsHeaderContainer.querySelectorAll<HTMLElement>("[data-tab-id]")) {
      const id = btn.dataset.tabId;
      if (!id) continue;
      const tabSpec = this.spec.tabs.find((t) => t.id === id);
      nodes.push({
        id,
        node: btn,
        keyTip: btn.dataset.keyTip || tabSpec?.keyTip,
      });
    }
    return nodes;
  }

  getCommandNodesInActiveTab(): { commandId: string; node: HTMLElement; keyTip?: string }[] {
    const nodes: { commandId: string; node: HTMLElement; keyTip?: string }[] = [];
    for (const btn of this.tabPageContainer.querySelectorAll<HTMLElement>("[data-command-id]")) {
      const commandId = btn.dataset.commandId;
      if (!commandId) continue;
      nodes.push({
        commandId,
        node: btn,
        keyTip: btn.dataset.keyTip,
      });
    }
    return nodes;
  }
}
