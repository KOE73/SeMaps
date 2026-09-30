import { KindCatalog } from "../model/KindCatalog.js";
import type { StyleLibrary } from "../model/StyleLibrary.js";
import { el } from "../util/dom.js";
import { i18n } from "../workbench/i18n/I18nService.js";
import { select } from "./fields.js";

/** A style's name as a variant of its type: the type's base style says so in words. */
export function variantLabel(name: string, isBase: boolean): string {
  return isBase ? `${name} (${i18n.d.panels.properties.styleBaseMark})` : name;
}

/**
 * The two drop-downs of the dictionary of types (CONTRACT.md §6): entity kinds
 * and relation types, grouped by the dictionary's groups. One place, so the
 * Properties panel, the Relations panel and anything that follows offer the same
 * list in the same order.
 *
 * A value outside the dictionary is not an error: it is shown, first, and marked
 * «не из словаря» — the dictionary is not an enum.
 */

function optionFor(id: string, name: string, description: string, selected: boolean): HTMLOptionElement {
  const option = el("option", { value: id, text: name, title: description });
  option.selected = selected;
  return option;
}

function unknownOption(id: string): HTMLOptionElement {
  const option = el("option", {
    value: id,
    text: `${id || "—"} (${i18n.d.panels.properties.kindNotInCatalog})`,
  });
  option.selected = true;
  return option;
}

/**
 * Entity kinds by group. A container offers only container kinds and a block
 * only the others: turning one into the other is not a kind edit.
 */
export function kindSelect(current: string, container: boolean, onChange: (kind: string) => void): HTMLSelectElement {
  const catalog = KindCatalog.active;
  const lang = i18n.currentLanguage;
  const picker = el("select", {
    on: {
      change: (e) => {
        const value = (e.target as HTMLSelectElement).value;
        if (value !== "" && value !== current) onChange(value);
      },
    },
  });
  if (catalog.lookup(current) === undefined) picker.append(unknownOption(current));
  for (const group of catalog.groups()) {
    const fitting = group.kinds.filter((k) => (k.container === true) === container);
    if (fitting.length === 0) continue;
    const optgroup = el("optgroup");
    optgroup.label = catalog.groupName(group, lang);
    optgroup.title = catalog.groupDescription(group, lang);
    for (const kind of fitting) {
      optgroup.append(optionFor(kind.id, catalog.name(kind.id, lang), catalog.description(kind.id, lang), kind.id === current));
    }
    picker.append(optgroup);
  }
  return picker;
}

/**
 * Relation types by group. With `placeholder`, an empty `current` shows that
 * text as its first, empty option — nothing is chosen for the person.
 */
export function relationTypeSelect(
  current: string,
  onChange: (type: string) => void,
  placeholder?: string,
): HTMLSelectElement {
  const catalog = KindCatalog.active;
  const lang = i18n.currentLanguage;
  const picker = el("select", {
    on: {
      change: (e) => {
        const value = (e.target as HTMLSelectElement).value;
        if (value !== "" && value !== current) onChange(value);
      },
    },
  });
  if (placeholder !== undefined && current === "") {
    const option = el("option", { value: "", text: placeholder });
    option.selected = true;
    picker.append(option);
  } else if (catalog.lookupRelation(current) === undefined) {
    picker.append(unknownOption(current));
  }
  for (const group of catalog.relationGroups()) {
    if (group.types.length === 0) continue;
    const optgroup = el("optgroup");
    optgroup.label = catalog.groupName(group, lang);
    optgroup.title = catalog.groupDescription(group, lang);
    for (const type of group.types) {
      optgroup.append(optionFor(type.id, `${catalog.relationName(type.id, lang)} · ${type.id}`, catalog.relationDescription(type.id, lang), type.id === current));
    }
    picker.append(optgroup);
  }
  return picker;
}

/**
 * The styles of one relation type as a select, its base style first as «по типу»
 * (the base is never written, CONTRACT.md §11.5). Null when the type has nothing
 * to choose between: a type with one style shows no style field at all.
 */
export function edgeVariantSelect(
  styles: StyleLibrary,
  type: string,
  current: string | undefined,
  onChange: (styleId: string | null) => void,
): HTMLSelectElement | null {
  const variants = styles.stylesOf(type, "edge");
  if (variants.length <= 1) return null;
  const base = styles.baseStyleOf(type, "edge");
  return select(
    variants.map((v) => [v.id === base ? "" : v.id, variantLabel(v.name, v.id === base)] as const),
    current ?? "",
    (value) => onChange(value === "" ? null : value),
  );
}
