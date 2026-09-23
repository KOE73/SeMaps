import type { CommandRegistry } from "../commands/CommandRegistry.js";
import { el } from "../../util/dom.js";

export class TooltipManager {
  private readonly tooltipEl: HTMLElement;
  private readonly titleEl: HTMLElement;
  private readonly descEl: HTMLElement;
  private readonly shortcutEl: HTMLElement;
  private currentTarget: HTMLElement | null = null;
  private showTimer: number | null = null;

  constructor(private readonly registry: CommandRegistry) {
    this.titleEl = el("div", { class: "workbench-tooltip-title" });
    this.descEl = el("div", { class: "workbench-tooltip-desc" });
    this.shortcutEl = el("span", { class: "workbench-tooltip-shortcut" });

    this.tooltipEl = el("div", {
      class: "workbench-tooltip",
      attrs: {
        style: `
          position: fixed;
          display: none;
          z-index: 10000;
          max-width: 260px;
          padding: 8px 10px;
          background: var(--panel-alt, #252526);
          color: var(--text, #ffffff);
          border: 1px solid var(--line, #3a3a3c);
          border-radius: 6px;
          box-shadow: 0 4px 16px rgba(0,0,0,0.35);
          font-family: inherit;
          pointer-events: none;
          font-size: 12px;
          line-height: 1.35;
        `,
      },
    }, [
      el("div", { attrs: { style: "display: flex; justify-content: space-between; align-items: center; gap: 8px; margin-bottom: 3px;" } }, [
        this.titleEl,
        this.shortcutEl,
      ]),
      this.descEl,
    ]);

    document.body.appendChild(this.tooltipEl);
    this.bindGlobal();
  }

  private bindGlobal(): void {
    document.addEventListener("pointerover", (e) => {
      const target = (e.target as Element)?.closest<HTMLElement>("[data-tooltip-command]");
      if (target && target !== this.currentTarget) {
        this.showFor(target);
      }
    });

    document.addEventListener("pointerout", (e) => {
      const target = (e.target as Element)?.closest<HTMLElement>("[data-tooltip-command]");
      if (target && target === this.currentTarget) {
        this.hide();
      }
    });

    document.addEventListener("pointerdown", () => {
      this.hide();
    });
  }

  private showFor(target: HTMLElement): void {
    const cmdId = target.dataset.tooltipCommand;
    if (!cmdId) return;

    const state = this.registry.getState(cmdId);
    if (!state.title && !state.description) return;

    this.currentTarget = target;
    if (this.showTimer !== null) window.clearTimeout(this.showTimer);

    this.showTimer = window.setTimeout(() => {
      this.titleEl.textContent = state.title;
      this.titleEl.style.fontWeight = "600";

      if (state.description) {
        this.descEl.textContent = state.description;
        this.descEl.style.display = "block";
        this.descEl.style.color = "var(--muted, #9aa0a6)";
      } else {
        this.descEl.style.display = "none";
      }

      if (state.shortcut) {
        this.shortcutEl.textContent = state.shortcut;
        this.shortcutEl.style.display = "inline-block";
        this.shortcutEl.style.fontSize = "10.5px";
        this.shortcutEl.style.fontFamily = "var(--mono, monospace)";
        this.shortcutEl.style.padding = "1px 5px";
        this.shortcutEl.style.borderRadius = "4px";
        this.shortcutEl.style.background = "var(--bg, #1e1e1e)";
        this.shortcutEl.style.border = "1px solid var(--border, #3a3a3c)";
      } else {
        this.shortcutEl.style.display = "none";
      }

      this.tooltipEl.style.display = "block";
      this.positionTooltip(target);
    }, 350);
  }

  private hide(): void {
    if (this.showTimer !== null) {
      window.clearTimeout(this.showTimer);
      this.showTimer = null;
    }
    this.currentTarget = null;
    this.tooltipEl.style.display = "none";
  }

  private positionTooltip(target: HTMLElement): void {
    const targetRect = target.getBoundingClientRect();
    const tooltipRect = this.tooltipEl.getBoundingClientRect();

    let left = targetRect.left + (targetRect.width - tooltipRect.width) / 2;
    let top = targetRect.bottom + 6;

    if (left + tooltipRect.width > window.innerWidth - 10) {
      left = window.innerWidth - tooltipRect.width - 10;
    }
    if (left < 10) left = 10;

    if (top + tooltipRect.height > window.innerHeight - 10) {
      top = targetRect.top - tooltipRect.height - 6;
    }

    this.tooltipEl.style.left = `${left}px`;
    this.tooltipEl.style.top = `${top}px`;
  }
}
