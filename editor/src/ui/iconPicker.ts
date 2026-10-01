import { el } from "../util/dom.js";
import { t } from "../shell/strings.js";
import { tablerNames } from "./iconSet.js";
import { iconByKey } from "./kindIcons.js";

export interface IconPickerOptions {
  /** The stored value: a Tabler name of the set, a kind key, or nothing. */
  value: string | undefined;
  /** What an empty value stands for (an inherited style's icon): shown greyed. */
  inherited?: string;
  /** Offer «по умолчанию» (an empty value). */
  allowClear?: boolean;
  onChange(value: string | undefined): void;
}

const MAX_SHOWN = 96;

/**
 * A searchable icon picker over the registry's set (`ui/iconSet.ts`): the icon
 * now in effect and its name, a search box, a grid of matching icons. A click
 * stores the icon's name. It replaces the emoji pickers: what is stored is
 * always a key the registry can draw.
 */
export function iconPicker(options: IconPickerOptions): HTMLElement {
  let value = options.value;
  const names = tablerNames();

  const current = el("span", { class: "icon-picker-current" });
  const label = el("span", { class: "icon-picker-name" });
  const search = el("input", { class: "icon-picker-search", type: "text", placeholder: t.iconPickerSearch }) as HTMLInputElement;
  const grid = el("div", { class: "icon-picker-grid" });
  const clear = options.allowClear
    ? el("button", { type: "button", class: "btn icon-picker-clear", text: t.iconPickerClear })
    : null;

  const paintCurrent = (): void => {
    current.innerHTML = iconByKey(value ?? options.inherited);
    label.textContent = value ?? (options.inherited ? `${options.inherited}` : "");
    label.classList.toggle("is-inherited", value === undefined);
  };

  const paintGrid = (): void => {
    const words = search.value.toLowerCase().split(/\s+/).filter(Boolean);
    const matches = names.filter((n) => words.every((w) => n.includes(w)));
    if (matches.length === 0) {
      grid.replaceChildren(el("span", { class: "form-hint", text: t.iconPickerNone }));
      return;
    }
    grid.replaceChildren(
      ...matches.slice(0, MAX_SHOWN).map((name) => {
        const b = el("button", { type: "button", class: `icon-picker-item${name === value ? " is-active" : ""}`, title: name, attrs: { "aria-label": name } });
        b.innerHTML = iconByKey(name);
        b.addEventListener("click", () => {
          value = name;
          options.onChange(name);
          paintCurrent();
          paintGrid();
        });
        return b;
      }),
      ...(matches.length > MAX_SHOWN ? [el("span", { class: "form-hint", text: `+${matches.length - MAX_SHOWN}` })] : []),
    );
  };

  search.addEventListener("input", paintGrid);
  // Typing in the search box must not submit or close the dialog around the picker.
  search.addEventListener("keydown", (e) => {
    if (e.key === "Enter") e.preventDefault();
  });
  clear?.addEventListener("click", () => {
    value = undefined;
    options.onChange(undefined);
    paintCurrent();
    paintGrid();
  });

  paintCurrent();
  paintGrid();
  return el("div", { class: "icon-picker" }, [el("div", { class: "icon-picker-head" }, [current, label, clear]), search, grid]);
}
