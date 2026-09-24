import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { armsOf, nameOf, rings, STRUCTURAL, type Dir } from "../../editor/relatives.js";
import { placeAround, type Side } from "../../editor/placeEntity.js";
import { entityOf } from "../../model/types.js";
import type { EntityEntry } from "../../model/wire-types.js";
import { i18n } from "../i18n/I18nService.js";
import type { MenuItem } from "./ContextMenu.js";

/**
 * The relation part of a box's right-click menu, for a box that stands for a
 * registry entity (empty for any other box).
 *
 * On top, kinship: what a type has by its nature — ancestors, descendants,
 * interfaces, implementations, contents — one click adds the whole family,
 * walked as far as it goes. Below, every other relation type the entity has,
 * each opening onto the list of who is at the other end.
 */
export function relationItems(editor: DiagramEditor, elementId: string): MenuItem[] {
  const doc = editor.canvas.model;
  const element = doc?.element(elementId);
  if (!doc || !element) return [];
  const id = entityOf(element)?.id ?? element.id;
  if (!doc.entities.some((e) => e.id === id)) return [];

  const t = i18n.d.entityMenu;
  const placed = (e: EntityEntry) => doc.element(e.id) !== undefined;
  const add = (list: EntityEntry[], side: Side) => () => {
    // What was just added stays selected, so it can be dragged aside in one go.
    const placed = placeAround(editor, elementId, list, side);
    if (placed.length > 0) editor.canvas.selectMany(placed);
  };

  // What each kind of kin is, how to find it, and where it lands.
  const kin: { label: string; icon: string; type: string; dir: Dir; deep: boolean; side: Side }[] = [
    { label: t.ancestors, icon: "⬆", type: "extends", dir: "out", deep: true, side: "above" },
    { label: t.descendants, icon: "⬇", type: "extends", dir: "in", deep: true, side: "below" },
    { label: t.interfaces, icon: "◇", type: "implements", dir: "out", deep: true, side: "above" },
    { label: t.implementations, icon: "◆", type: "implements", dir: "in", deep: true, side: "below" },
    { label: t.containers, icon: "▣", type: "contains", dir: "in", deep: false, side: "above" },
    { label: t.contents, icon: "▤", type: "contains", dir: "out", deep: false, side: "below" },
  ];

  const items: MenuItem[] = [];
  const kinItems: MenuItem[] = [];
  const arms = armsOf(doc, id);
  const count = (list: EntityEntry[], missing: EntityEntry[]) =>
    missing.length === 0 ? t.allPlaced : missing.length === list.length ? String(list.length) : `${missing.length} / ${list.length}`;

  for (const k of kin) {
    const levels = k.deep
      ? rings(doc, id, k.type, k.dir)
      : [arms.find((a) => a.type === k.type && a.dir === k.dir)?.entities ?? []].filter((l) => l.length > 0);
    if (levels.length === 0) continue;
    const list = levels.flat();
    const missing = list.filter((e) => !placed(e));
    const item: MenuItem = {
      label: k.label,
      icon: k.icon,
      note: count(list, missing),
      disabled: missing.length === 0,
      title: list.map((e) => nameOf(doc, e)).join("\n"),
    };
    if (levels.length === 1) {
      item.onSelect = add(missing, k.side);
    } else {
      // Deeper than one ring: choose how far, nearest first; the last is everyone.
      item.submenu = () => levels.map((_, i): MenuItem => {
        const upTo = levels.slice(0, i + 1).flat();
        const miss = upTo.filter((e) => !placed(e));
        return {
          label: i === levels.length - 1 ? t.allLevels : t.levels(i + 1),
          note: count(upTo, miss),
          disabled: miss.length === 0,
          title: upTo.map((e) => nameOf(doc, e)).join("\n"),
          onSelect: add(miss, k.side),
        };
      });
    }
    kinItems.push(item);
  }
  if (kinItems.length > 0) items.push({ kind: "header", label: t.kin }, ...kinItems);

  const other = arms.filter((a) => !(STRUCTURAL as readonly string[]).includes(a.type));
  if (other.length > 0) {
    if (items.length > 0) items.push({ kind: "separator" });
    items.push({ kind: "header", label: t.other });
    const names = i18n.d.panels.neighbourhood.arms as Record<string, { out: string; in: string } | undefined>;
    for (const arm of other) {
      const missing = arm.entities.filter((e) => !placed(e));
      items.push({
        label: names[arm.type]?.[arm.dir] ?? (arm.dir === "out" ? `${arm.type} →` : `← ${arm.type}`),
        icon: arm.dir === "out" ? "→" : "←",
        note: String(arm.entities.length),
        submenu: () => [
          {
            label: t.addAll,
            note: missing.length === 0 ? t.allPlaced : String(missing.length),
            disabled: missing.length === 0,
            onSelect: add(missing, "right"),
          },
          { kind: "separator" },
          ...arm.entities.map((e): MenuItem => ({
            label: nameOf(doc, e),
            note: e.kind,
            checked: placed(e),
            title: placed(e) ? t.onView : undefined,
            onSelect: placed(e) ? () => editor.canvas.select(e.id) : add([e], "right"),
          })),
        ],
      });
    }
  }

  if (items.length === 0) items.push({ label: t.nothing, disabled: true });
  return items;
}
