import type { EntityEntry } from "../model/wire-types.js";
import type { DiagramEditor } from "./DiagramEditor.js";
import { relationShownByDefault } from "../model/relationVisibility.js";

const WIDTH = 180;
const HEIGHT = 60;
const GAP_X = 40;
const GAP_Y = 40;
const PER_ROW = 4;

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
  const texts = doc.bundle?.text?.entries || {};

  const placed: string[] = [];
  for (const entity of entities) {
    if (doc.element(entity.id) !== undefined || placed.includes(entity.id)) continue;
    const text = texts[entity.id] as { name?: string; description?: string } | undefined;
    const i = placed.length;
    const x = at.x + (i % PER_ROW) * (WIDTH + GAP_X);
    const y = at.y + Math.floor(i / PER_ROW) * (HEIGHT + GAP_Y);
    doc.add(
      {
        id: entity.id,
        kind: "node" as const,
        type: entity.kind,
        label: text?.name || entity.name || entity.id,
        tags: [],
        metadata: { description: text?.description, codeRef: entity.codeRef },
        x,
        y,
        width: WIDTH,
        height: HEIGHT,
        parent: null,
        children: [],
        wireOrder: Number.POSITIVE_INFINITY,
        raw: { _entity: entity },
      } as any,
      doc.containerAt({ x: x + WIDTH / 2, y: y + HEIGHT / 2 }),
    );
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
 * The type's `visibility` and the view's `relations.default` / `except` still decide.
 */
function drawRelations(editor: DiagramEditor, placed: readonly string[]): void {
  const doc = editor.canvas.model;
  if (!doc) return;
  const policy = doc.bundle?.view?.relations as { default?: string; except?: string[] } | undefined;
  const types = doc.bundle?.relationTypes;
  const fresh = new Set(placed);
  const have = new Set(doc.edges.map((e) => e.id));

  for (const r of doc.relations) {
    if (!r || have.has(r.id) || r.from === r.to) continue;
    if (!fresh.has(r.from) && !fresh.has(r.to)) continue;
    if (doc.element(r.from) === undefined || doc.element(r.to) === undefined) continue;
    if (!relationShownByDefault(r, policy, types)) continue;
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
