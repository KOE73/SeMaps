import { el, replaceChildren } from "../util/dom.js";

export interface SearchableOption {
  value: string;
  label: string;
  subtitle?: string;
  icon?: string;
  badge?: string;
}

export interface SearchableSelectOptions {
  options: readonly (SearchableOption | [string, string])[];
  value: string;
  placeholder?: string;
  searchPlaceholder?: string;
  onChange: (value: string) => void;
  class?: string;
  style?: string;
}

export class SearchableSelect {
  readonly root: HTMLElement;
  private readonly triggerBtn: HTMLElement;
  private readonly dropdown: HTMLElement;
  private readonly searchInput: HTMLInputElement;
  private readonly optionsContainer: HTMLElement;

  private options: SearchableOption[];
  private currentValue: string;
  private onChange: (value: string) => void;
  private isOpen = false;

  constructor(opts: SearchableSelectOptions) {
    this.currentValue = opts.value;
    this.onChange = opts.onChange;
    this.options = opts.options.map((opt) => {
      if (Array.isArray(opt)) {
        return { value: opt[0], label: opt[1] };
      }
      return opt;
    });

    this.triggerBtn = el("button", {
      class: `searchable-select-trigger ${opts.class ?? ""}`,
      attrs: { type: "button", style: opts.style ?? "" },
      on: {
        click: (e) => {
          e.stopPropagation();
          this.toggle();
        },
      },
    });

    this.searchInput = el("input", {
      type: "text",
      class: "searchable-select-search",
      placeholder: opts.searchPlaceholder ?? "Поиск (like)…",
      on: {
        input: (e) => {
          this.filter((e.target as HTMLInputElement).value);
        },
        click: (e) => e.stopPropagation(),
        keydown: (e) => {
          if (e.key === "Escape") {
            this.close();
          }
        },
      },
    });

    this.optionsContainer = el("div", { class: "searchable-select-options" });

    this.dropdown = el(
      "div",
      { class: "searchable-select-dropdown", hidden: true },
      [this.searchInput, this.optionsContainer],
    );

    this.root = el(
      "div",
      { class: "searchable-select" },
      [this.triggerBtn, this.dropdown],
    );

    this.updateTriggerText();
    this.renderOptions(this.options);

    document.addEventListener("click", this.onDocumentClick);
  }

  destroy(): void {
    document.removeEventListener("click", this.onDocumentClick);
  }

  setValue(val: string): void {
    this.currentValue = val;
    this.updateTriggerText();
    this.renderOptions(this.options);
  }

  setOptions(options: readonly (SearchableOption | [string, string])[]): void {
    this.options = options.map((opt) => {
      if (Array.isArray(opt)) {
        return { value: opt[0], label: opt[1] };
      }
      return opt;
    });
    this.updateTriggerText();
    this.renderOptions(this.options);
  }

  private onDocumentClick = (e: MouseEvent): void => {
    if (this.isOpen && !this.root.contains(e.target as Node)) {
      this.close();
    }
  };

  private toggle(): void {
    if (this.isOpen) this.close();
    else this.open();
  }

  private open(): void {
    this.isOpen = true;
    this.dropdown.hidden = false;
    this.root.classList.add("is-open");
    this.searchInput.value = "";
    this.renderOptions(this.options);
    setTimeout(() => this.searchInput.focus(), 20);
  }

  private close(): void {
    this.isOpen = false;
    this.dropdown.hidden = true;
    this.root.classList.remove("is-open");
  }

  private updateTriggerText(): void {
    const selected = this.options.find((o) => o.value === this.currentValue);
    const label = selected ? selected.label : "— Выберите —";
    replaceChildren(
      this.triggerBtn,
      el("span", { class: "searchable-select-label", text: label }),
      el("span", { class: "searchable-select-arrow", text: "▾" }),
    );
  }

  private filter(needle: string): void {
    const q = needle.trim().toLowerCase();
    if (!q) {
      this.renderOptions(this.options);
      return;
    }
    const filtered = this.options.filter(
      (o) =>
        String(o?.label ?? "").toLowerCase().includes(q) ||
        String(o?.value ?? "").toLowerCase().includes(q) ||
        (o?.subtitle ? String(o.subtitle).toLowerCase().includes(q) : false),
    );
    this.renderOptions(filtered);
  }

  private renderOptions(list: SearchableOption[]): void {
    if (list.length === 0) {
      replaceChildren(
        this.optionsContainer,
        el("div", { class: "searchable-select-empty", text: "Ничего не найдено" }),
      );
      return;
    }

    replaceChildren(
      this.optionsContainer,
      ...list.map((opt) => {
        const isSelected = opt.value === this.currentValue;
        return el(
          "div",
          {
            class: `searchable-select-option${isSelected ? " is-selected" : ""}`,
            title: opt.subtitle ? `${opt.label} (${opt.subtitle})` : opt.label,
            on: {
              click: (e) => {
                e.stopPropagation();
                this.currentValue = opt.value;
                this.updateTriggerText();
                this.close();
                this.onChange(opt.value);
              },
            },
          },
          [
            opt.icon ? el("span", { class: "searchable-select-opt-icon", text: opt.icon }) : null,
            el("span", { class: "searchable-select-opt-label", text: opt.label }),
            opt.badge ? el("span", { class: "badge chip", text: opt.badge }) : null,
            opt.subtitle ? el("span", { class: "searchable-select-opt-sub mono muted", text: opt.subtitle }) : null,
          ],
        );
      }),
    );
  }
}
