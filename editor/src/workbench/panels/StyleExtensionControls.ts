import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

const text = (ru: string, en: string): string => i18n.currentLanguage === "ru" ? ru : en;
export interface NamedChoice { id: string; name: string }
let helpId = 0;

export function extensionHelp(title: string, description: string): HTMLElement {
  const id = `style-help-${++helpId}`;
  const popup = el("div", { class: "extension-help-popup", id, attrs: { popover: "auto" } }, [
    el("strong", { text: title }), el("p", { text: description }),
  ]);
  const trigger = el("button", {
    class: "extension-help-button", text: "i", type: "button",
    attrs: { "aria-label": title, "aria-controls": id, "aria-expanded": "false" },
    dataset: { tooltipTitle: title, tooltipDesc: description },
    on: { click: () => {
      popup.togglePopover();
      if (popup.matches(":popover-open")) {
        const anchor = trigger.getBoundingClientRect();
        const bounds = popup.getBoundingClientRect();
        popup.style.left = `${Math.max(8, Math.min(anchor.left, window.innerWidth - bounds.width - 8))}px`;
        popup.style.top = `${Math.max(8, Math.min(anchor.bottom + 6, window.innerHeight - bounds.height - 8))}px`;
      }
    } },
  });
  popup.addEventListener("toggle", () => trigger.setAttribute("aria-expanded", String(popup.matches(":popover-open"))));
  return el("span", { class: "extension-help" }, [trigger, popup]);
}

export function choiceCaption(choice: NamedChoice): HTMLElement {
  return el("span", { class: "extension-caption" }, [
    el("strong", { text: choice.name }), el("em", { text: choice.id }),
  ]);
}

export function changePickedIds(ids: readonly string[], id: string, checked: boolean): string[] {
  return checked ? ids.includes(id) ? [...ids] : [...ids, id] : ids.filter(value => value !== id);
}
export function movePickedId(ids: readonly string[], id: string, direction: -1 | 1): string[] {
  const next = [...ids], index = next.indexOf(id), target = index + direction;
  if (index >= 0 && target >= 0 && target < next.length) [next[index], next[target]] = [next[target]!, next[index]!];
  return next;
}

/** Checkboxes store IDs; labels and filtering never change references or order. */
export function choicePicker(label: string, choices: NamedChoice[], initial: readonly string[], change: (ids: string[]) => void,
  options: { ordered?: boolean; custom?: boolean; emptyHint?: string } = {}): HTMLElement {
  let selected = [...initial];
  const available = new Map(choices.map(choice => [choice.id, choice]));
  const rows = el("div", { class: "extension-choices" });
  const search = el("input", { type: "search", placeholder: text("Поиск по названию или ID…", "Search name or ID…"), attrs: { "aria-label": `${text("Поиск", "Search")}: ${label}` }, on: { input: () => render() } });
  const hint = el("div", { class: "muted extension-choice-hint" });
  const commit = (next: string[], focusId?: string): void => {
    selected = next; change([...selected]); render();
    if (focusId) [...rows.querySelectorAll<HTMLInputElement>("input[type=checkbox]")].find(input => input.dataset.choiceId === focusId)?.focus({ preventScroll: true });
  };
  const render = (): void => {
    rows.replaceChildren();
    hint.textContent = selected.length ? `${text("Выбрано", "Selected")}: ${selected.length}` : options.emptyHint ?? text("Ничего не выбрано", "Nothing selected");
    const all = new Map(available);
    selected.forEach(id => { if (!all.has(id)) all.set(id, { id, name: text("Сохранённое значение", "Stored value") }); });
    const matches = [...all.values()].filter(choice => `${choice.name} ${choice.id}`.toLocaleLowerCase().includes(search.value.toLocaleLowerCase()));
    if (options.ordered) matches.sort((a, b) => (selected.indexOf(a.id) < 0 ? Infinity : selected.indexOf(a.id)) - (selected.indexOf(b.id) < 0 ? Infinity : selected.indexOf(b.id)));
    for (const choice of matches) {
      const index = selected.indexOf(choice.id);
      const checkbox = el("input", { type: "checkbox", dataset: { choiceId: choice.id }, attrs: { "aria-label": `${choice.name} ${choice.id}`, ...(index >= 0 ? { checked: "" } : {}) }, on: { change: e => commit(changePickedIds(selected, choice.id, (e.target as HTMLInputElement).checked), choice.id) } });
      const row = el("div", { class: `extension-choice${index >= 0 ? " is-selected" : ""}` }, [
        el("label", { class: "extension-check-label" }, [checkbox, options.ordered && index >= 0 ? el("span", { class: "extension-order", text: String(index + 1) }) : null, choiceCaption(choice)]),
      ]);
      if (options.ordered && index >= 0) {
        for (const direction of [-1, 1] as const) row.append(el("button", {
          class: "btn extension-order-button", type: "button", text: direction < 0 ? "↑" : "↓",
          title: `${direction < 0 ? text("Раньше", "Earlier") : text("Позже", "Later")}: ${choice.name}`,
          disabled: index + direction < 0 || index + direction >= selected.length,
          on: { click: () => commit(movePickedId(selected, choice.id, direction)) },
        }));
      }
      rows.append(row);
    }
    if (!matches.length) rows.append(el("p", { class: "muted", text: text("Нет подходящих вариантов", "No matching options") }));
  };
  const field = el("fieldset", { class: "extension-picker" }, [el("legend", { text: label }), search, hint, rows]);
  if (options.custom) {
    const custom = el("input", { placeholder: text("Другое значение…", "Other value…"), attrs: { "aria-label": `${label}: ${text("другое значение", "other value")}` } });
    field.append(el("div", { class: "extension-actions" }, [custom, el("button", { class: "btn", text: text("Добавить", "Add"), on: { click: () => {
      const id = custom.value.trim(); if (!id) return;
      available.set(id, available.get(id) ?? { id, name: id }); custom.value = ""; commit(changePickedIds(selected, id, true));
    } } })]));
  }
  render();
  return field;
}
