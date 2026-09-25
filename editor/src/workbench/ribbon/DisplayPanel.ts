import { el } from "../../util/dom.js";
import type { CommandRegistry } from "../commands/CommandRegistry.js";
import type { Ribbon } from "./Ribbon.js";
import type { RibbonTabSpec } from "./types.js";

/**
 * The gear's panel: the look of the tool, in every mode. It floats under the
 * gear, renders the display groups with the ribbon's own renderer, then the
 * canvas effects laid out in full, and follows
 * command state while open; a click outside or Escape closes it.
 */
export class DisplayPanel {
  private panel: HTMLElement | null = null;
  private backdrop: HTMLElement | null = null;

  constructor(
    private readonly ribbon: Ribbon,
    private readonly spec: RibbonTabSpec,
    registry: CommandRegistry,
    /** Laid out after the groups: controls that are not commands (the canvas effects). */
    private readonly extra?: () => HTMLElement,
  ) {
    registry.onStateChanged(() => this.fill());
    window.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && this.panel) this.close();
    });
  }

  get isOpen(): boolean {
    return this.panel !== null;
  }

  toggle(anchor: HTMLElement): void {
    if (this.panel) this.close();
    else this.open(anchor);
  }

  close(): void {
    this.panel?.remove();
    this.backdrop?.remove();
    this.panel = this.backdrop = null;
  }

  private open(anchor: HTMLElement): void {
    this.backdrop = el("div", { class: "display-panel-backdrop", on: { mousedown: () => this.close() } });
    this.panel = el("div", { class: "display-panel", attrs: { role: "dialog", "aria-label": this.spec.title } });
    // Right edge under the gear, which sits at the right end of the row. Placed
    // once: the ribbon redraws its buttons on every state change.
    const r = anchor.getBoundingClientRect();
    this.panel.style.top = `${Math.round(r.bottom + 4)}px`;
    this.panel.style.right = `${Math.max(8, Math.round(window.innerWidth - r.right))}px`;
    document.body.append(this.backdrop, this.panel);
    this.fill();
  }

  private fill(): void {
    if (!this.panel) return;
    this.panel.replaceChildren(
      el("div", { class: "display-panel-title", text: this.spec.title }),
      this.ribbon.renderGroups(this.spec),
    );
    if (this.extra) this.panel.appendChild(this.extra());
  }
}
