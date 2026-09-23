import type { CommandRegistry } from "../commands/CommandRegistry.js";
import type { CommandContext } from "../commands/types.js";
import type {
  RibbonTabSpec,
  RibbonGroupSpec,
  RibbonItemSpec,
  RibbonButtonSpec,
  RibbonToggleSpec,
  RibbonSelectSpec,
  RibbonThemeGallerySpec,
} from "./types.js";
import { el } from "../../util/dom.js";

export class RibbonRenderer {
  constructor(
    private readonly registry: CommandRegistry,
    private readonly getContext: () => CommandContext,
  ) {}

  renderTabsHeader(
    tabs: readonly RibbonTabSpec[],
    activeTabId: string,
    onSelectTab: (id: string) => void,
  ): HTMLElement {
    const ctx = this.getContext();
    const currentKind = ctx.selection.current?.kind;

    const visibleTabs = tabs.filter((tab) => {
      if (!tab.contextual) return true;
      return tab.contextual === currentKind;
    });

    const header = el("div", { class: "ribbon-header-inner" });

    // 1. Quick Access Toolbar (Save, Undo, Redo)
    const qat = el("div", { class: "ribbon-qat" });
    const qatCommands = ["file.save", "edit.undo", "edit.redo"];
    for (const cmdId of qatCommands) {
      const state = this.registry.getState(cmdId, ctx);
      const dataset: Record<string, string> = {
        tooltipCommand: cmdId,
        commandId: cmdId,
      };
      if (state.keyTip) dataset.keyTip = state.keyTip;

      const btn = el("button", {
        class: "ribbon-qat-btn",
        dataset,
        attrs: !state.enabled ? { disabled: "true" } : {},
        on: {
          click: (e: MouseEvent) => {
            e.stopPropagation();
            void this.registry.execute(cmdId, btn);
          },
        },
      });

      const icon = state.icon || "⚙️";
      const iconSpan = el("span", { class: "ribbon-qat-icon" });
      if (icon.startsWith("<svg")) {
        iconSpan.innerHTML = icon;
      } else {
        iconSpan.textContent = icon;
      }
      btn.appendChild(iconSpan);
      qat.appendChild(btn);
    }
    header.appendChild(qat);

    // 2. Tabs Bar
    const tabBar = el("div", { class: "ribbon-tab-bar" });
    for (const tab of visibleTabs) {
      const isActive = tab.id === activeTabId;
      const isContextual = Boolean(tab.contextual);

      const dataset: Record<string, string> = { tabId: tab.id };
      if (tab.keyTip) dataset.keyTip = tab.keyTip;

      const tabBtn = el(
        "button",
        {
          class: `ribbon-tab-btn ${isActive ? "is-active" : ""} ${isContextual ? "is-contextual" : ""}`,
          text: tab.title,
          dataset,
          on: {
            click: () => onSelectTab(tab.id),
          },
        },
      );

      tabBar.appendChild(tabBtn);
    }
    header.appendChild(tabBar);

    return header;
  }

  renderTabContent(tab: RibbonTabSpec): HTMLElement {
    const container = el("div", { class: "ribbon-tab-page" });

    for (const group of tab.groups) {
      const groupEl = this.renderGroup(group);
      container.appendChild(groupEl);
    }

    return container;
  }

  private renderGroup(group: RibbonGroupSpec): HTMLElement {
    const itemsContainer = el("div", { class: "ribbon-group-items" });

    for (const item of group.items) {
      const itemEl = this.renderItem(item);
      if (itemEl) itemsContainer.appendChild(itemEl);
    }

    const titleEl = el("div", { class: "ribbon-group-title", text: group.title });

    return el("div", { class: "ribbon-group", dataset: { groupId: group.id } }, [
      itemsContainer,
      titleEl,
    ]);
  }

  private renderItem(item: RibbonItemSpec): HTMLElement | null {
    const ctx = this.getContext();

    if (item.type === "separator") {
      return el("div", { class: "ribbon-separator" });
    }

    if (item.type === "button") {
      return this.renderButton(item, ctx);
    }

    if (item.type === "toggle") {
      return this.renderToggle(item, ctx);
    }

    if (item.type === "select") {
      return this.renderSelect(item, ctx);
    }

    if (item.type === "theme-gallery") {
      return this.renderThemeGallery(item, ctx);
    }

    return null;
  }

  private renderThemeGallery(spec: RibbonThemeGallerySpec, ctx: CommandContext): HTMLElement {
    const currentTheme = spec.getValue(ctx);

    const container = el("div", { class: "ribbon-theme-gallery" });

    for (const theme of spec.themes) {
      const isActive = theme.id === currentTheme;

      const preview = el("div", { class: "ribbon-theme-preview", dataset: { theme: theme.id } }, [
        el("div", { class: "preview-header" }, [
          el("div", { class: "preview-tab is-active" }),
          el("div", { class: "preview-tab" }),
        ]),
        el("div", { class: "preview-body" }, [
          el("div", { class: "preview-sidebar left" }),
          el("div", { class: "preview-canvas" }, [
            el("div", { class: "preview-node n1" }, [
              el("div", { class: "preview-node-badge" }),
            ]),
            el("div", { class: "preview-edge" }),
            el("div", { class: "preview-node n2" }),
          ]),
          el("div", { class: "preview-sidebar right" }),
        ]),
      ]);

      const label = el("span", { class: "ribbon-theme-label", text: theme.name });

      const card = el(
        "div",
        {
          class: `ribbon-theme-card ${isActive ? "is-active" : ""}`,
          title: `Theme: ${theme.name}`,
          on: {
            click: () => {
              void this.registry.execute(spec.command, theme.id);
            },
          },
        },
        [preview, label],
      );

      container.appendChild(card);
    }

    return container;
  }

  private renderButton(spec: RibbonButtonSpec, ctx: CommandContext): HTMLElement {
    const state = this.registry.getState(spec.command, ctx);
    const size = spec.size ?? "medium";
    const label = spec.label || state.title;
    const icon = spec.icon || state.icon || "⚙️";

    const dataset: Record<string, string> = {
      tooltipCommand: spec.command,
      commandId: spec.command,
    };
    if (state.keyTip) dataset.keyTip = state.keyTip;

    const iconSpan = el("span", { class: "ribbon-btn-icon" });
    if (icon.startsWith("<svg")) {
      iconSpan.innerHTML = icon;
    } else {
      iconSpan.textContent = icon;
    }

    const children: HTMLElement[] = [iconSpan];

    if (size !== "small") {
      children.push(el("span", { class: "ribbon-btn-label", text: label }));
    }

    const btn = el(
      "button",
      {
        class: `ribbon-btn ribbon-btn-${size}`,
        dataset,
        attrs: !state.enabled ? { disabled: "true" } : {},
        on: {
          click: () => {
            void this.registry.execute(spec.command, btn);
          },
        },
      },
      children,
    );

    return btn;
  }

  private renderToggle(spec: RibbonToggleSpec, ctx: CommandContext): HTMLElement {
    const state = this.registry.getState(spec.command, ctx);
    const size = spec.size ?? "medium";
    const label = spec.label || state.title;
    const icon = spec.icon || state.icon || "🔘";
    const isChecked = Boolean(state.checked);

    const dataset: Record<string, string> = {
      tooltipCommand: spec.command,
      commandId: spec.command,
    };
    if (state.keyTip) dataset.keyTip = state.keyTip;

    const iconSpan = el("span", { class: "ribbon-btn-icon" });
    if (icon.startsWith("<svg")) {
      iconSpan.innerHTML = icon;
    } else {
      iconSpan.textContent = icon;
    }

    const children: HTMLElement[] = [iconSpan];

    if (size !== "small") {
      children.push(el("span", { class: "ribbon-btn-label", text: label }));
    }

    const btn = el(
      "button",
      {
        class: `ribbon-btn ribbon-btn-${size} ribbon-toggle ${isChecked ? "is-checked" : ""}`,
        dataset,
        attrs: !state.enabled ? { disabled: "true" } : {},
        on: {
          click: () => {
            void this.registry.execute(spec.command);
          },
        },
      },
      children,
    );

    return btn;
  }

  private renderSelect(spec: RibbonSelectSpec, ctx: CommandContext): HTMLElement {
    const currentVal = spec.getValue(ctx);

    const selectEl = el(
      "select",
      {
        class: "ribbon-select",
        on: {
          change: (e) => {
            const val = (e.target as HTMLSelectElement).value;
            void this.registry.execute(spec.command, val);
          },
        },
      },
      spec.options.map((opt) =>
        el("option", {
          attrs: opt.value === currentVal ? { selected: "true", value: opt.value } : { value: opt.value },
          text: opt.label,
        }),
      ),
    );

    return el("div", { class: "ribbon-select-wrapper" }, [
      el("span", { class: "ribbon-select-label", text: spec.label }),
      selectEl,
    ]);
  }
}
