/**
 * What the graph mode copies and the Схемы editor pastes: nodes of the live code
 * graph, as a versioned JSON text on the system clipboard (the marker below
 * tells the editor it is ours) with an in-app copy as the fallback for when the
 * system clipboard is not readable.
 *
 * Only nodes that stand for a registry entity (`entity`) can be pasted; the
 * rest are for the human to see that a reconcile is needed. Positions are the
 * graph's own (y up), so the editor can keep the arrangement.
 */
export const GRAPH_CLIPBOARD_FORMAT = "semaps.graph-nodes";
export const GRAPH_CLIPBOARD_VERSION = 1;

export interface GraphClipboardNode {
  /** The graph's node id. */
  id: string;
  /** The registry entity id, when the node has one (presence model or both). */
  entity?: string;
  name: string;
  /** The base kind (`class` for `abstract-class`). */
  kind: string;
  x: number;
  y: number;
}

export interface GraphClipboardEdge {
  from: string;
  to: string;
  kind: string;
}

export interface GraphClipboard {
  format: typeof GRAPH_CLIPBOARD_FORMAT;
  version: typeof GRAPH_CLIPBOARD_VERSION;
  nodes: GraphClipboardNode[];
  /** The edge kinds drawn on the graph when it was copied (the edge-kind filter). */
  edgeKinds: string[];
  /** The edges among the copied nodes of those kinds. */
  edges: GraphClipboardEdge[];
}

let lastCopy: GraphClipboard | undefined;

export function encodeGraphClipboard(payload: GraphClipboard): string {
  lastCopy = payload;
  return JSON.stringify(payload);
}

/** The payload in `text`, or null when the text is not ours (or of a version this editor does not know). */
export function decodeGraphClipboard(text: string | undefined): GraphClipboard | null {
  if (!text || !text.includes(GRAPH_CLIPBOARD_FORMAT)) return null;
  try {
    const value = JSON.parse(text) as Partial<GraphClipboard>;
    if (value.format !== GRAPH_CLIPBOARD_FORMAT || value.version !== GRAPH_CLIPBOARD_VERSION || !Array.isArray(value.nodes)) return null;
    return { ...(value as GraphClipboard), edgeKinds: value.edgeKinds ?? [], edges: value.edges ?? [] };
  } catch {
    return null;
  }
}

/** The last payload copied in this page: the fallback for an unreadable system clipboard. */
export function lastGraphCopy(): GraphClipboard | undefined {
  return lastCopy;
}
