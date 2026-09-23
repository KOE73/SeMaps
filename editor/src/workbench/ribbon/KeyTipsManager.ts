import type { CommandRegistry } from "../commands/CommandRegistry.js";
import type { Ribbon } from "./Ribbon.js";
import { el } from "../../util/dom.js";

type KeyTipLevel = "none" | "tabs" | "commands";

interface KeyTipBadge {
  readonly element: HTMLElement;
  readonly key: string;
  readonly action: () => void;
}

export class KeyTipsManager {
  private level: KeyTipLevel = "none";
  private activeBadges: KeyTipBadge[] = [];
  private readonly container: HTMLElement;

  constructor(
    private readonly ribbon: Ribbon,
    private readonly registry: CommandRegistry,
  ) {
    this.container = el("div", {
      class: "workbench-keytips-overlay",
      attrs: {
        style: "position: fixed; inset: 0; pointer-events: none; z-index: 9999;",
      },
    });
    document.body.appendChild(this.container);

    this.bind();
  }

  private bind(): void {
    window.addEventListener("keydown", (e) => {
      if (this.isEditableTarget(e.target)) return;

      if (e.key === "Alt") {
        e.preventDefault();
        if (this.level === "none") {
          this.showTabsLevel();
        } else {
          this.hide();
        }
        return;
      }

      if (this.level === "none") return;

      if (e.key === "Escape") {
        e.preventDefault();
        if (this.level === "commands") {
          this.showTabsLevel();
        } else {
          this.hide();
        }
        return;
      }

      const key = e.key.toUpperCase();
      const match = this.activeBadges.find((b) => b.key.toUpperCase() === key);

      if (match) {
        e.preventDefault();
        e.stopPropagation();
        match.action();
      }
    });

    window.addEventListener("pointerdown", (e) => {
      if (this.level !== "none") {
        const target = e.target as HTMLElement;
        if (!target.closest(".workbench-ribbon")) {
          this.hide();
        }
      }
    });
  }

  showTabsLevel(): void {
    this.clearBadges();
    this.level = "tabs";

    const tabNodes = this.ribbon.getTabHeaderNodes();
    for (const { id, node, keyTip } of tabNodes) {
      if (!keyTip) continue;
      this.createBadge(node, keyTip, () => {
        this.ribbon.selectTab(id);
        this.showCommandsLevel();
      });
    }
  }

  showCommandsLevel(): void {
    this.clearBadges();
    this.level = "commands";

    const commandNodes = this.ribbon.getCommandNodesInActiveTab();
    for (const { commandId, node, keyTip } of commandNodes) {
      const tip = keyTip || this.registry.get(commandId)?.keyTip;
      if (!tip) continue;
      this.createBadge(node, tip, () => {
        this.hide();
        void this.registry.execute(commandId);
      });
    }
  }

  hide(): void {
    this.clearBadges();
    this.level = "none";
  }

  private createBadge(target: HTMLElement, keyText: string, action: () => void): void {
    const rect = target.getBoundingClientRect();
    const badge = el(
      "div",
      {
        class: "keytip-badge",
        text: keyText,
        attrs: {
          style: `
            position: fixed;
            left: ${Math.round(rect.left + rect.width / 2)}px;
            top: ${Math.round(rect.bottom - 6)}px;
            transform: translate(-50%, 0);
            background: #ffffff;
            color: #000000;
            border: 1px solid #222222;
            border-radius: 3px;
            padding: 1px 4px;
            font-size: 11px;
            font-weight: 700;
            font-family: var(--mono, monospace);
            box-shadow: 0 2px 6px rgba(0,0,0,0.4);
            pointer-events: none;
          `,
        },
      },
    );

    this.container.appendChild(badge);
    this.activeBadges.push({ element: badge, key: keyText, action });
  }

  private clearBadges(): void {
    this.container.innerHTML = "";
    this.activeBadges = [];
  }

  private isEditableTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    const tag = target.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable;
  }
}
