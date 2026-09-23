import type { DiagramDocument } from "./document.js";
import type { DiagramEdge, DiagramElement } from "./types.js";
import { entityOf } from "./types.js";

export interface ResolvedRelation {
  id: string;
  from: string;
  to: string;
  type: string;
  label: string;
  styleId?: string;
  visible: boolean;
  edge?: DiagramEdge;
  raw?: any;
}

export interface ElementRelationsSummary {
  total: number;
  visible: number;
  items: ResolvedRelation[];
}

export function resolveElementRelations(
  doc: DiagramDocument | null,
  element: DiagramElement,
): ElementRelationsSummary {
  if (!doc) return { total: 0, visible: 0, items: [] };

  const canvasEdges = doc.edges.filter((e) => e.from === element.id || e.to === element.id);
  const relations = doc.relations;
  const rawEntityId = entityOf(element)?.id;

  const map = new Map<string, ResolvedRelation>();

  // 1. First add all edges present on the canvas
  for (const edge of canvasEdges) {
    map.set(edge.id, {
      id: edge.id,
      from: edge.from,
      to: edge.to,
      type: edge.type,
      label: edge.label || "",
      styleId: edge.styleId,
      visible: true,
      edge,
    });
  }

  // 2. Then merge relations from the registry/code
  if (Array.isArray(relations)) {
    for (const rel of relations) {
      const fromMatch = rel.from === element.id || (rawEntityId && rel.from === rawEntityId);
      const toMatch = rel.to === element.id || (rawEntityId && rel.to === rawEntityId);
      if (fromMatch || toMatch) {
        const canvasFrom = fromMatch ? element.id : rel.from;
        const canvasTo = toMatch ? element.id : rel.to;

        if (canvasFrom === canvasTo) continue;

        // Check if there is already a canvas edge matching this relation
        const existing = [...map.values()].find(
          (k) =>
            k.id === rel.id ||
            (k.from === canvasFrom && k.to === canvasTo && k.type === (rel.type || rel.relation)),
        );

        if (!existing) {
          const relId = rel.id || `rel_${canvasFrom}_${canvasTo}_${rel.type || "rel"}`;
          map.set(relId, {
            id: relId,
            from: canvasFrom,
            to: canvasTo,
            type: rel.type || rel.relation || "relates",
            label: rel.label || "",
            styleId: rel.styleId,
            visible: false,
            raw: rel,
          });
        } else if (!existing.raw) {
          existing.raw = rel;
        }
      }
    }
  }

  const items = Array.from(map.values());
  const visible = items.filter((r) => r.visible).length;
  const total = items.length;

  return { total, visible, items };
}
