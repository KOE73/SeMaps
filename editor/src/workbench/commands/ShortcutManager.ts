import type { CommandRegistry } from "./CommandRegistry.js";
import type { CommandContext, IDisposable } from "./types.js";

export class ShortcutManager implements IDisposable {
  private readonly abort = new AbortController();

  constructor(
    private readonly registry: CommandRegistry,
    private readonly contextProvider: () => CommandContext,
  ) {
    this.bind();
  }

  dispose(): void {
    this.abort.abort();
  }

  private bind(): void {
    window.addEventListener(
      "keydown",
      (e: KeyboardEvent) => {
        this.handleKeyDown(e);
      },
      { signal: this.abort.signal },
    );
  }

  private handleKeyDown(e: KeyboardEvent): void {
    const isEditable = this.isEditableTarget(e.target);
    const eventKey = this.normalizeKey(e);
    const modCtrl = e.ctrlKey || e.metaKey;
    const modShift = e.shiftKey;
    const modAlt = e.altKey;

    for (const cmd of this.registry.list()) {
      if (!cmd.shortcut) continue;

      if (this.matchesShortcut(cmd.shortcut, modCtrl, modShift, modAlt, eventKey, e.key)) {
        if (isEditable && !cmd.allowInEditable) {
          // Let text input handle its own cursor, editing and hotkeys
          continue;
        }

        const context = this.contextProvider();
        if (cmd.isEnabled && !cmd.isEnabled(context)) {
          // Command disabled, prevent default if it is an app shortcut
          e.preventDefault();
          return;
        }

        e.preventDefault();
        e.stopPropagation();
        void this.registry.execute(cmd.id);
        return;
      }
    }
  }

  private isEditableTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    const tag = target.tagName;
    return (
      tag === "INPUT" ||
      tag === "TEXTAREA" ||
      tag === "SELECT" ||
      target.isContentEditable
    );
  }

  private normalizeKey(e: KeyboardEvent): string {
    const key = e.key.toLowerCase();
    return key;
  }

  private matchesShortcut(
    shortcutString: string,
    modCtrl: boolean,
    modShift: boolean,
    modAlt: boolean,
    eventKey: string,
    rawKey: string,
  ): boolean {
    // A command may specify multiple comma-separated shortcuts: e.g. "Ctrl+Y, Ctrl+Shift+Z"
    const variants = shortcutString.split(",").map((s) => s.trim());

    for (const variant of variants) {
      if (this.matchesSingleShortcut(variant, modCtrl, modShift, modAlt, eventKey, rawKey)) {
        return true;
      }
    }
    return false;
  }

  private matchesSingleShortcut(
    shortcut: string,
    modCtrl: boolean,
    modShift: boolean,
    modAlt: boolean,
    eventKey: string,
    rawKey: string,
  ): boolean {
    const parts = shortcut.split("+").map((p) => p.trim());
    let reqCtrl = false;
    let reqShift = false;
    let reqAlt = false;
    let reqKey = "";

    for (const part of parts) {
      const p = part.toLowerCase();
      if (p === "ctrl" || p === "cmd" || p === "control" || p === "meta") {
        reqCtrl = true;
      } else if (p === "shift") {
        reqShift = true;
      } else if (p === "alt" || p === "option") {
        reqAlt = true;
      } else {
        reqKey = p;
      }
    }

    if (modCtrl !== reqCtrl) return false;
    if (modShift !== reqShift) return false;
    if (modAlt !== reqAlt) return false;

    if (!reqKey) return false;

    if (reqKey === "delete") {
      return eventKey === "delete";
    }
    if (reqKey === "backspace") {
      return eventKey === "backspace";
    }
    if (reqKey === "esc" || reqKey === "escape") {
      return eventKey === "escape";
    }
    if (reqKey === "enter" || reqKey === "return") {
      return eventKey === "enter";
    }
    if (reqKey === "space") {
      return eventKey === " " || eventKey === "spacebar";
    }

    // Direct key match
    if (eventKey === reqKey) return true;
    if (rawKey.toLowerCase() === reqKey) return true;

    return false;
  }
}
