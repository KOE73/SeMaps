import type { EntityEntry } from "../model/wire-types.js";
import type { DiagramEditor } from "./DiagramEditor.js";
import { relationShownByDefault } from "../model/relationVisibility.js";
import { KindCatalog } from "../model/KindCatalog.js";
import { canvas } from "../constants/canvas.js";
import { realizationsOf } from "../model/realizations.js";

/** Default box size and the gap between boxes, from the canvas numbers (`/canvas.json`). */
export function boxMetrics(): { WIDTH: number; HEIGHT: number; GAP_X: number; GAP_Y: number } {
  const c = canvas();
  return { WIDTH: c.node.width, HEIGHT: c.node.height, GAP_X: c.gap.node, GAP_Y: c.gap.node };
}
const PER_ROW = 4;

/** A container made or placed in the editor: room for what will go into it (`canvas.json` has only its minimum). */
export const NEW_CONTAINER_SIZE = { width: 420, height: 300 } as const;
const CONTAINER_WIDTH = NEW_CONTAINER_SIZE.width;
const CONTAINER_HEIGHT = NEW_CONTAINER_SIZE.height;

/**
 * The box of a registry entity at (x, y). Frame or block is the kind's to say
 * (CONTRACT.md §8.2). Its name is the display name in the current text
 * language (`entityDisplayName`); its description comes from the view's texts.
 */
export function blockFor(doc: NonNullable<DiagramEditor["canvas"]["model"]>, entity: EntityEntry, x: number, y: number): any {
  const { WIDTH, HEIGHT } = boxMetrics();
  const container = KindCatalog.active.isContainer(entity.kind);
  const text = (doc.bundle?.text?.entries || {})[entity.id] as { description?: string } | undefined;
  const code = realizationsOf(entity);
  return {
    id: entity.id,
    kind: container ? "zone" as const : "node" as const,
    type: entity.kind,
    label: doc.nameOfEntity(entity),
    tags: [],
    metadata: { description: text?.description, ...(code.length > 0 ? { code } : {}) },
    x,
    y,
    width: container ? CONTAINER_WIDTH : WIDTH,
    height: container ? CONTAINER_HEIGHT : HEIGHT,
    parent: null,
    children: [],
    wireOrder: Number.POSITIVE_INFINITY,
    raw: { _entity: entity },
  };
}

/**
 * Put registry entities on the open view as boxes, one undo step for all of
 * them. Several go in a grid starting at `at`, so a whole branch does not land
 * in one heap. Entities already on the view are skipped.
 *
 * Returns the ids actually placed.
 */
export function placeEntities(
  editor: DiagramEditor,
  entities: readonly EntityEntry[],
  at = editor.canvas.viewCenter(),
): string[] {
  const doc = editor.canvas.model;
  if (!doc) return [];
  const { WIDTH, HEIGHT, GAP_X, GAP_Y } = boxMetrics();
  // A grid with a container in it is spaced for the container.
  const anyContainer = entities.some((e) => KindCatalog.active.isContainer(e.kind));
  const cellW = anyContainer ? CONTAINER_WIDTH : WIDTH;
  const cellH = anyContainer ? CONTAINER_HEIGHT : HEIGHT;

  const placed: string[] = [];
  for (const entity of entities) {
    if (doc.element(entity.id) !== undefined || placed.includes(entity.id)) continue;
    const i = placed.length;
    const x = at.x + (i % PER_ROW) * (cellW + GAP_X);
    const y = at.y + Math.floor(i / PER_ROW) * (cellH + GAP_Y);
    const box = blockFor(doc, entity, x, y);
    doc.add(box, doc.containerAt({ x: x + box.width / 2, y: y + box.height / 2 }));
    placed.push(entity.id);
  }

  if (placed.length > 0) {
    drawRelations(editor, placed);
    (editor as any).commit("place-entity");
  }
  return placed;
}

export type Side = "above" | "below" | "right";

/**
 * Place entities next to a box already on the view, so a family lands where
 * the eye expects it: ancestors above, descendants below, the rest to the right.
 */
export function placeAround(
  editor: DiagramEditor,
  anchorId: string,
  entities: readonly EntityEntry[],
  side: Side,
): string[] {
  const doc = editor.canvas.model;
  const anchor = doc?.element(anchorId);
  if (!doc || !anchor) return placeEntities(editor, entities);
  const { WIDTH, HEIGHT, GAP_X, GAP_Y } = boxMetrics();
  const n = entities.filter((e) => doc.element(e.id) === undefined).length;
  if (n === 0) return [];
  const cols = Math.min(n, PER_ROW);
  const rows = Math.ceil(n / PER_ROW);
  const gridW = cols * WIDTH + (cols - 1) * GAP_X;
  const gridH = rows * HEIGHT + (rows - 1) * GAP_Y;
  const cx = anchor.x + anchor.width / 2;
  const cy = anchor.y + anchor.height / 2;
  const at =
    side === "above" ? { x: cx - gridW / 2, y: anchor.y - GAP_Y * 2 - gridH }
    : side === "below" ? { x: cx - gridW / 2, y: anchor.y + anchor.height + GAP_Y * 2 }
    : { x: anchor.x + anchor.width + GAP_X * 2, y: cy - gridH / 2 };
  return placeEntities(editor, entities, at);
}

/**
 * Give the newly placed boxes the lines of their registry relations to
 * everything already on the view: a relation shows once both its ends do
 * (CONTRACT.md §8.5). A view saved with its own `edges` list would otherwise
 * never show a relation that reached the registry after that save.
 * The type's `visibility` in the dictionary and the view's `relations.default` / `except` still decide.
 */
export function drawRelations(editor: DiagramEditor, placed: readonly string[]): void {
  const doc = editor.canvas.model;
  if (!doc) return;
  const policy = doc.bundle?.view?.relations as { default?: string; except?: string[] } | undefined;
  const fresh = new Set(placed);
  const have = new Set(doc.edges.map((e) => e.id));

  for (const r of doc.relations) {
    if (!r || have.has(r.id) || r.from === r.to) continue;
    if (!fresh.has(r.from) && !fresh.has(r.to)) continue;
    if (doc.element(r.from) === undefined || doc.element(r.to) === undefined) continue;
    if (!relationShownByDefault(r, policy, KindCatalog.active)) continue;
    doc.addEdge({
      id: r.id,
      from: r.from,
      to: r.to,
      type: r.type,
      label: "",
      ...(r.origin === undefined ? {} : { origin: r.origin }),
    } as any);
    have.add(r.id);
  }
}
