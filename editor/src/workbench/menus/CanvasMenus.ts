import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { edgePreview } from "../../editor/style-preview.js";
import type { RoutingMode } from "../../model/style-types.js";
import { entityOf, type DiagramEdge, type DiagramElement } from "../../model/types.js";
import { i18n } from "../i18n/I18nService.js";
import { openContextMenu, type MenuItem } from "./ContextMenu.js";
import { relationItems } from "./EntityMenu.js";

/** What the menus need from the workbench around the editor. */
export interface MenuHost {
  openPanel(id: "properties" | "styles" | "neighbourhood" | "relations"): void;
  /** Open the Styles panel on this style. */
  openStyleEditor(styleId: string | null): void;
}

const MODES: readonly RoutingMode[] = ["orthogonal", "bezier", "tree-vertical", "tree-horizontal"];
const MODE_ICON: Record<RoutingMode, string> = {
  orthogonal: "┐",
  bezier: "∿",
  "tree-vertical": "┴",
  "tree-horizontal": "├",
};

/** Right click on the canvas: on a box, on a line, or on empty space. */
export function openCanvasMenu(
  editor: DiagramEditor,
  host: MenuHost,
  target: "element" | "edge" | "canvas",
  id: string | null,
  clientX: number,
  clientY: number,
): void {
  const doc = editor.canvas.model;
  if (!doc) return;
  let items: MenuItem[] = [];
  if (target === "element" && id !== null) {
    const el = doc.element(id);
    if (el) items = blockItems(editor, host, el);
  } else if (target === "edge" && id !== null) {
    const edge = doc.edge(id);
    if (edge) items = edgeItems(editor, host, edge);
  } else {
    items = [viewRoutingItem(editor)];
  }
  if (items.length > 0) openContextMenu(items, clientX, clientY);
}

// ------------------------------------------------------------------ boxes

function blockItems(editor: DiagramEditor, host: MenuHost, el: DiagramElement): MenuItem[] {
  const t = i18n.d.canvasMenu;
  const doc = editor.canvas.model!;
  const styles = editor.styles;
  // Choices apply to every selected box of the same sort, not just the one clicked.
  const ids = [...editor.canvas.selectedIds].filter((sid) => doc.element(sid)?.kind === el.kind);
  if (!ids.includes(el.id)) ids.push(el.id);
  const isZone = el.kind === "zone";
  const entity = entityOf(el) ?? doc.entities.find((e) => e.id === el.id);

  const items: MenuItem[] = [];
  const relations = relationItems(editor, el.id);
  if (relations.length > 0) items.push(...relations, { kind: "separator" });

  if (!isZone) {
    const current = typeof el.metadata.template === "string" ? el.metadata.template : null;
    items.push({
      label: t.template,
      icon: "▤",
      submenu: () => [
        {
          label: t.templateFromStyle,
          checked: current === null,
          preview: () => editor.canvas.previewElement(el, { template: null }),
          onSelect: () => editor.applyTemplate(ids, null),
        },
        { kind: "separator" },
        ...editor.canvas.templates.list().map((tpl): MenuItem => ({
          label: tpl.name ?? tpl.id,
          checked: current === tpl.id,
          title: tpl.description,
          preview: () => editor.canvas.previewElement(el, { template: tpl.id }),
          onSelect: () => editor.applyTemplate(ids, tpl.id),
        })),
      ],
    });
  }

  items.push({
    label: t.style,
    icon: "🎨",
    submenu: () => [
      {
        label: t.styleDefault,
        checked: el.styleId === undefined,
        preview: () => editor.canvas.previewElement(el, { styleId: null }),
        onSelect: () => editor.applyStyle(ids, null),
      },
      { kind: "separator" },
      ...styles.list("block").map((s): MenuItem => ({
        label: s.name,
        checked: el.styleId === s.id,
        title: s.style.description,
        preview: () => editor.canvas.previewElement(el, { styleId: s.id }),
        onSelect: () => editor.applyStyle(ids, s.id),
      })),
    ],
  });

  items.push({ kind: "separator" });
  items.push({ label: t.describe, icon: "✎", onSelect: () => editor.openDocEditor(el.id, isZone ? "zone" : "node") });
  const codeRef = typeof el.metadata.codeRef === "string" ? el.metadata.codeRef : entity?.codeRef;
  if (codeRef) items.push({ label: t.code, icon: "💻", note: codeRef.split("/").pop(), onSelect: () => editor.openCodeViewer(codeRef, el.label) });
  items.push({ label: t.properties, icon: "→", onSelect: () => host.openPanel("properties") });
  items.push({ label: t.blockStyle, icon: "→", onSelect: () => host.openStyleEditor(styles.blockStyleIdFor(el)) });
  if (entity) items.push({ label: t.neighbourhood, icon: "🕸️", onSelect: () => host.openPanel("neighbourhood") });
  items.push({ kind: "separator" }, { label: t.remove, icon: "🗑", onSelect: () => editor.deleteSelection() });
  return items;
}

// ------------------------------------------------------------------ lines

function edgeItems(editor: DiagramEditor, host: MenuHost, edge: DiagramEdge): MenuItem[] {
  const t = i18n.d.canvasMenu;
  const doc = editor.canvas.model!;
  const styles = editor.styles;
  const ids = [...editor.canvas.selectedIds].filter((sid) => doc.edge(sid) !== undefined);
  if (!ids.includes(edge.id)) ids.push(edge.id);
  const edges = ids.map((sid) => doc.edge(sid)!).filter(Boolean);
  const own = edges.some((e) => e.routing !== undefined);

  return [
    {
      label: t.lineShape,
      icon: MODE_ICON[edge.routing ?? "orthogonal"],
      note: edge.routing ? modeName(edge.routing) : t.inherited,
      submenu: () => [
        {
          label: t.resetShape,
          icon: "↺",
          title: t.resetShapeHint,
          disabled: !own,
          onSelect: () => editor.setEdgeRouting(ids, null),
        },
        { kind: "separator" },
        ...MODES.map((m): MenuItem => ({
          label: modeName(m),
          icon: MODE_ICON[m],
          checked: edge.routing === m,
          onSelect: () => editor.setEdgeRouting(ids, m),
        })),
      ],
    },
    viewRoutingItem(editor),
    {
      label: t.style,
      icon: "🎨",
      submenu: () => [
        {
          label: t.styleByType,
          checked: edge.styleId === undefined,
          preview: () => edgePreview(styles.resolveEdge(styles.has(edge.type) ? edge.type : null)),
          onSelect: () => editor.applyStyle(ids, null),
        },
        { kind: "separator" },
        ...styles.list("edge").map((s): MenuItem => ({
          label: s.name,
          checked: edge.styleId === s.id,
          title: s.style.description,
          preview: () => edgePreview(styles.resolveEdge(s.id)),
          onSelect: () => editor.applyStyle(ids, s.id),
        })),
      ],
    },
    { kind: "separator" },
    { label: t.describe, icon: "✎", onSelect: () => editor.openDocEditor(edge.id, "edge") },
    { label: t.properties, icon: "→", onSelect: () => host.openPanel("relations") },
    { label: t.edgeStyle, icon: "→", onSelect: () => host.openStyleEditor(styles.edgeStyleIdFor(edge)) },
    { kind: "separator" },
    { label: t.remove, icon: "🗑", onSelect: () => editor.deleteSelection() },
  ];
}

// ------------------------------------------------------------------- view

/** The line shape of the whole view: every line without its own choice follows it. */
function viewRoutingItem(editor: DiagramEditor): MenuItem {
  const t = i18n.d.canvasMenu;
  const current = (editor.canvas.model?.metadata as { routing?: RoutingMode } | undefined)?.routing ?? null;
  return {
    label: t.viewShape,
    icon: MODE_ICON[current ?? "bezier"],
    note: current ? modeName(current) : t.byType,
    submenu: () => [
      {
        label: t.byType,
        title: t.byTypeHint,
        checked: current === null,
        onSelect: () => editor.setViewRouting(null),
      },
      { kind: "separator" },
      ...MODES.map((m): MenuItem => ({
        label: modeName(m),
        icon: MODE_ICON[m],
        checked: current === m,
        onSelect: () => editor.setViewRouting(m),
      })),
    ],
  };
}

function modeName(m: RoutingMode): string {
  const names = i18n.d.canvasMenu.modes;
  return m === "orthogonal" ? names.orthogonal
    : m === "bezier" ? names.bezier
    : m === "tree-vertical" ? names.treeVertical
    : names.treeHorizontal;
}
