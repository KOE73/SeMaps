import { el } from "../../util/dom.js";

export type MenuItem =
  | {
      kind?: "item";
      label: string;
      icon?: string;
      /** Small grey text on the right: a count, a shortcut. */
      note?: string;
      title?: string;
      disabled?: boolean;
      /** Marks something already done, e.g. an entity already on the view. */
      checked?: boolean;
      onSelect?: () => void;
      /** A small picture under the label: how the choice will look. Built when the menu opens. */
      preview?: () => Node;
      /** Opens to the side on hover; the item itself then does nothing on click. */
      submenu?: () => MenuItem[];
    }
  | { kind: "separator" }
  | { kind: "header"; label: string };

let current: { close: () => void } | null = null;

/**
 * A plain context menu at a screen point, with submenus opening to the side.
 * One at a time: opening another closes this one. Closes on Escape, on a click
 * anywhere outside, on scroll or resize, and after an item is chosen.
 */
export function openContextMenu(items: MenuItem[], clientX: number, clientY: number): void {
  current?.close();

  const panels: HTMLElement[] = [];
  const abort = new AbortController();

  const close = (): void => {
    abort.abort();
    for (const p of panels) p.remove();
    panels.length = 0;
    if (current?.close === close) current = null;
  };
  current = { close };

  /** Drop every panel deeper than `level`, so a new submenu replaces a sibling's. */
  const trim = (level: number): void => {
    while (panels.length > level + 1) panels.pop()!.remove();
  };

  const place = (panel: HTMLElement, x: number, y: number, flipFrom?: DOMRect): void => {
    document.body.appendChild(panel);
    const r = panel.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let left = x;
    if (left + r.width > vw - 4) left = flipFrom ? flipFrom.left - r.width : vw - 4 - r.width;
    const top = Math.max(4, Math.min(y, vh - 4 - r.height));
    panel.style.left = `${Math.max(4, left)}px`;
    panel.style.top = `${top}px`;
  };

  const build = (list: MenuItem[], level: number): HTMLElement => {
    const panel = el("div", { class: "ctx-menu", attrs: { role: "menu" } });
    for (const item of list) {
      if (item.kind === "separator") {
        panel.appendChild(el("div", { class: "ctx-sep" }));
        continue;
      }
      if (item.kind === "header") {
        panel.appendChild(el("div", { class: "ctx-header", text: item.label }));
        continue;
      }
      const row = el("div", {
        class: `ctx-item${item.disabled ? " is-disabled" : ""}${item.submenu ? " has-sub" : ""}`,
        title: item.title,
        attrs: { role: "menuitem" },
      }, [
        el("span", { class: "ctx-icon", text: item.checked ? "✓" : item.icon ?? "" }),
        item.preview
          ? el("span", { class: "ctx-main" }, [
              el("span", { class: "ctx-label", text: item.label }),
              el("span", { class: "ctx-preview" }, [item.preview()]),
            ])
          : el("span", { class: "ctx-label", text: item.label }),
        item.note ? el("span", { class: "ctx-note", text: item.note }) : null,
        item.submenu ? el("span", { class: "ctx-arrow", text: "▸" }) : null,
      ]);

      row.addEventListener("mouseenter", () => {
        trim(level);
        for (const r of panel.querySelectorAll(".is-open")) r.classList.remove("is-open");
        if (!item.submenu || item.disabled) return;
        row.classList.add("is-open");
        const sub = build(item.submenu(), level + 1);
        panels.push(sub);
        const rr = row.getBoundingClientRect();
        place(sub, rr.right - 2, rr.top - 4, rr);
      });
      if (!item.submenu && !item.disabled && item.onSelect) {
        row.addEventListener("click", () => {
          close();
          item.onSelect!();
        });
      }
      panel.appendChild(row);
    }
    return panel;
  };

  const root = build(items, 0);
  panels.push(root);
  place(root, clientX, clientY);

  const signal = abort.signal;
  // Deferred: the right click that opened the menu must not also close it.
  setTimeout(() => {
    if (signal.aborted) return;
    window.addEventListener("mousedown", (e) => {
      if (!panels.some((p) => p.contains(e.target as Node))) close();
    }, { signal, capture: true });
    window.addEventListener("keydown", (e) => {
      if (e.key === "Escape") { e.stopPropagation(); close(); }
    }, { signal, capture: true });
    window.addEventListener("resize", close, { signal });
    window.addEventListener("blur", close, { signal });
    window.addEventListener("wheel", (e) => {
      if (!panels.some((p) => p.contains(e.target as Node))) close();
    }, { signal, capture: true, passive: true });
  }, 0);
}
