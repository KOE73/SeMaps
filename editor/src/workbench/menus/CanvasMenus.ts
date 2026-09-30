import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { edgePreview } from "../../editor/style-preview.js";
import { variantLabel } from "../../editor/kindSelects.js";
import type { RoutingMode } from "../../model/style-types.js";
import { entityOf, type DiagramEdge, type DiagramElement } from "../../model/types.js";
import { i18n } from "../i18n/I18nService.js";
import { icons } from "../../ui/icons.js";
import { t as shellStrings } from "../../shell/strings.js";
import { openContextMenu, type MenuItem } from "./ContextMenu.js";
import { relationItems } from "./EntityMenu.js";

/** What the menus need from the workbench around the editor. */
export interface MenuHost {
  openPanel(id: "properties" | "styles" | "neighbourhood" | "relations"): void;
  /** Open the Styles panel on this style. */
  openStyleEditor(styleId: string | null): void;
  /** Menu entry for a registered command: its title, icon and enabled state. */
  command(id: string): MenuItem;
}

const ALIGN_COMMANDS = [
  "diagram.align.left", "diagram.align.right", "diagram.align.top",
  "diagram.align.bottom", "diagram.align.width", "diagram.align.height",
] as const;

const ALIGN_EDGE_COMMANDS = [
  "diagram.alignEdge.left", "diagram.alignEdge.right",
  "diagram.alignEdge.top", "diagram.alignEdge.bottom",
] as const;

const MODES: readonly RoutingMode[] = ["orthogonal", "bezier", "tree-vertical", "tree-horizontal"];
const MODE_ICON: Record<RoutingMode, string> = {
  orthogonal: icons.cornerRightDown,
  bezier: icons.waveSine,
  "tree-vertical": icons.hierarchy,
  "tree-horizontal": icons.hierarchy2,
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
    else if (doc.relations.some((r) => r.id === id)) items = ghostEdgeItems(editor, host, id);
  } else {
    items = [viewRoutingItem(editor), { kind: "separator" }, pasteItem(editor)];
  }
  if (items.length > 0) openContextMenu(items, clientX, clientY);
}

/** Nodes copied from the graph go onto the view (into the selected zone, when there is one). */
function pasteItem(editor: DiagramEditor): MenuItem {
  return { label: shellStrings.graphPaste, icon: icons.clipboard, note: "Ctrl+V", onSelect: () => void editor.pasteGraph() };
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
  items.push({ label: "Копировать ссылку", icon: icons.link, onSelect: () => editor.copyLink(ids.length > 0 ? ids : [el.id]) }, { kind: "separator" });
  const relations = relationItems(editor, el.id);
  if (relations.length > 0) items.push(...relations, { kind: "separator" });

  if (!isZone) {
    const current = typeof el.metadata.template === "string" ? el.metadata.template : null;
    items.push({
      label: t.template,
      icon: icons.template,
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

  // The variants of the box's own type (ADR_20260927-7), only when there is a choice.
  const target = isZone ? "container" : "block";
  const variants = styles.stylesOf(el.type, target);
  if (variants.length > 1) {
    const current = styles.blockStyleIdFor(el);
    const base = styles.baseStyleOf(el.type, target);
    items.push({
      label: t.style,
      icon: icons.palette,
      submenu: () => variants.map((s): MenuItem => ({
        label: variantLabel(s.name, s.id === base),
        checked: current === s.id,
        title: s.style.description,
        preview: () => editor.canvas.previewElement(el, { styleId: s.id === base ? null : s.id }),
        onSelect: () => editor.applyKindAndStyle(ids, el.type, s.id),
      })),
    });
  }

  if (editor.canvas.selectedIds.size > 1) {
    items.push({ label: t.align, icon: icons.arrowBarToLeft, submenu: () => [
      ...ALIGN_COMMANDS.map((id) => host.command(id)),
      { kind: "separator" },
      ...ALIGN_EDGE_COMMANDS.map((id) => host.command(id)),
    ] });
  }

  items.push({ kind: "separator" });
  items.push({ label: t.describe, icon: icons.pencil, onSelect: () => editor.openDocEditor(el.id, isZone ? "zone" : "node") });
  const codeRef = typeof el.metadata.codeRef === "string" ? el.metadata.codeRef : entity?.codeRef;
  if (codeRef) items.push({ label: t.code, icon: icons.code, note: codeRef.split("/").pop(), onSelect: () => editor.openCodeViewer(codeRef, el.label) });
  items.push({ label: t.properties, icon: icons.listDetails, onSelect: () => host.openPanel("properties") });
  items.push({ label: t.blockStyle, icon: icons.palette, onSelect: () => host.openStyleEditor(styles.blockStyleIdFor(el)) });
  if (entity) items.push({ label: t.neighbourhood, icon: icons.hierarchy, onSelect: () => host.openPanel("neighbourhood") });
  if (isZone) items.push(pasteItem(editor));
  items.push({ kind: "separator" }, { label: t.remove, icon: icons.trash, onSelect: () => editor.deleteSelection() });
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
  const variants = styles.stylesOf(edge.type, "edge");
  const base = styles.baseStyleOf(edge.type, "edge");
  const currentStyle = styles.edgeStyleIdFor(edge);

  return [
    { label: t.hideEdge, icon: icons.eyeOff, title: t.toggleEdgeHint, onSelect: () => editor.setEdgeShown(edge.id, false) },
    { kind: "separator" },
    {
      label: t.lineShape,
      icon: MODE_ICON[edge.routing ?? "orthogonal"],
      note: edge.routing ? modeName(edge.routing) : t.inherited,
      submenu: () => [
        {
          label: t.resetShape,
          icon: icons.refresh,
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
    // The variants of the line's own relation type (ADR_20260930-2), only when there is a choice.
    ...(variants.length > 1
      ? [{
          label: t.style,
          icon: icons.palette,
          submenu: (): MenuItem[] => variants.map((s): MenuItem => ({
            label: variantLabel(s.name, s.id === base),
            checked: currentStyle === s.id,
            title: s.style.description,
            preview: () => edgePreview(styles.resolveEdge(s.id)),
            onSelect: () => editor.applyRelationTypeAndStyle(ids, edge.type, s.id),
          })),
        } satisfies MenuItem]
      : []),
    { kind: "separator" },
    { label: t.describe, icon: icons.pencil, onSelect: () => editor.openDocEditor(edge.id, "edge") },
    { label: t.properties, icon: icons.listDetails, onSelect: () => host.openPanel("relations") },
    { label: t.edgeStyle, icon: icons.palette, onSelect: () => host.openStyleEditor(styles.edgeStyleIdFor(edge)) },
    { kind: "separator" },
    { label: t.remove, icon: icons.trash, onSelect: () => editor.deleteSelection() },
  ];
}

/** A ghost line: known in the registry, not on the view. */
function ghostEdgeItems(editor: DiagramEditor, host: MenuHost, id: string): MenuItem[] {
  const t = i18n.d.canvasMenu;
  return [
    { label: t.showEdge, icon: icons.eye, title: t.toggleEdgeHint, onSelect: () => editor.setEdgeShown(id, true) },
    { kind: "separator" },
    { label: t.describe, icon: icons.pencil, onSelect: () => editor.openDocEditor(id, "edge") },
    { label: t.properties, icon: icons.listDetails, onSelect: () => host.openPanel("relations") },
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
