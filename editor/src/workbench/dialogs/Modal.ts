import { el } from "../../util/dom.js";

export interface ModalHandle {
  readonly card: HTMLElement;
  close(): void;
}

/** A centred card over a dimmed page; Escape and a click outside close it. */
export function openModal(options: {
  title: string;
  body: HTMLElement;
  foot?: HTMLElement[];
  width?: string;
}): ModalHandle {
  const closeBtn = el("button", { class: "btn-icon", text: "✕", on: { click: () => close() } });
  const card = el("div", { class: "modal-card", attrs: { style: `max-width: ${options.width ?? "480px"};` } }, [
    el("div", { class: "modal-head" }, [el("h3", { text: options.title }), closeBtn]),
    el("div", { class: "modal-body" }, [options.body]),
    ...(options.foot ? [el("div", { class: "modal-foot", attrs: { style: "justify-content: flex-end; gap: 8px;" } }, options.foot)] : []),
  ]);
  const backdrop = el("div", { class: "modal" }, [card]);

  const onKey = (e: KeyboardEvent): void => {
    if (e.key === "Escape") close();
  };
  backdrop.addEventListener("mousedown", (e) => {
    if (e.target === backdrop) close();
  });
  window.addEventListener("keydown", onKey);
  document.body.appendChild(backdrop);

  function close(): void {
    window.removeEventListener("keydown", onKey);
    backdrop.remove();
  }
  return { card, close };
}
