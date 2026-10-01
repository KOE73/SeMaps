/**
 * The shapes `GET /api/graph/{project}` answers with (docs/API.md §5,
 * core/graph.go). Kept apart from `shell/api.ts`: that file is the tool API
 * used by the other modes, this one is graph-only and loaded lazily with the
 * rest of the mode.
 */

import { parseNativeKind } from "../../ui/kindIcons.js";
import { loadGraphFilterConfig } from "./filterConfig.js";

export interface Span {
  line: number;
  endLine?: number;
}

export interface Via {
  member?: string;
  memberKind?: string;
  modifiers?: string[];
  text?: string;
  path?: string[];
  cardinality?: string; // one, optional, many, keyed
  mutability?: string;
  deferred?: boolean;
}

export type Presence = "code" | "model" | "both";

export interface GraphNode {
  id: string;
  symbol?: string;
  kind?: string;
  nativeKind?: string;
  name?: string;
  namespace?: string;
  /** The name of the node's assembly (project). */
  assembly?: string;
  visibility?: string;
  file?: string;
  line?: number;
  endLine?: number;
  spans?: Span[];
  memberLines?: Record<string, number>;
  language?: string;
  extractor?: string;
  entity?: string;
  status?: string;
  /**
   * The one container the host resolved for the node: the node that `contains` it
   * and whose kind is a container kind (ADR_20260930_contract_graph-containers-from-contains).
   * At most one item; empty when there is none or it is ambiguous — nothing is guessed.
   */
  containers?: string[];
  /** This node is itself a container: its kind is a container kind of kinds.json. */
  container?: boolean;
  /** The containers the node lies in, outermost first: `containers` and then that container's own, up. Computed by the filter store. */
  containerPath?: string[];
  presence: Presence;
  /** The base kind of `nativeKind` (`class` for `abstract-class`) and its modifiers (`abstract`), parsed once by `fetchGraph`. */
  symbolKind?: string;
  modifiers?: string[];
  /** Blind spots (reflection, `dynamic`); on a type, gathered from its methods. */
  dynamic?: { kind: string; line: number; method?: string; file?: string }[];
}

export interface GraphEdge {
  from: string;
  to: string;
  kind: string;
  type: string;
  via?: Via;
  line?: number;
  file?: string;
  relation?: string;
  presence: Presence;
  /** Only on an edge lift=types made: how many method edges it merges, and their methods. */
  count?: number;
  fromMethods?: string[];
  toMethods?: string[];
}

export interface GraphFactsInfo {
  extractor: string;
  run?: string;
  finished?: string;
  language: string;
  lastRun?: string;
  lastRunFailed?: boolean;
}

export interface GraphStats {
  nodes: number;
  edges: number;
  byPresence: Record<string, number>;
  hiddenMissing?: { nodes: number; edges: number };
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
  facts: GraphFactsInfo[];
  stats: GraphStats;
}

export interface GraphDiff {
  addedNodes?: string[];
  removedNodes?: string[];
  changedNodes?: string[];
  addedEdges?: { from: string; to: string; kind: string }[];
  removedEdges?: { from: string; to: string; kind: string }[];
}

export async function fetchGraph(project: string, missing = false): Promise<GraphResponse> {
  // The filter defaults must be known before the first `filterStore.sync`.
  await loadGraphFilterConfig();
  const res = await fetch(`/api/graph/${encodeURIComponent(project)}?fields=via,position,dynamic${missing ? "&missing=1" : ""}`, { cache: "no-store" });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  const graph = (await res.json()) as GraphResponse;
  // A modifier is a look, not a kind: split `abstract-class` into `class` + [abstract] once, here.
  for (const n of graph.nodes) {
    const { base, modifiers } = parseNativeKind(n.nativeKind);
    n.symbolKind = base ?? n.kind;
    n.modifiers = modifiers;
  }
  return graph;
}

/**
 * `GET /api/graph/{project}/groups`: the containers of the graph with their nesting (`contains` between
 * container nodes; ids are graph node ids) and, per axis, the container placement each entity sits in
 * on the views of that axis (ids are entity ids) (docs/API.md §5).
 */
export interface GroupsResponse {
  containers: { id: string; name?: string; parent?: string }[];
  axes: { axis: string; containers: { id: string; name?: string; parent?: string }[]; of: Record<string, string> }[];
}

export async function fetchGroups(project: string): Promise<GroupsResponse> {
  const res = await fetch(`/api/graph/${encodeURIComponent(project)}/groups`, { cache: "no-store" });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return (await res.json()) as GroupsResponse;
}

export interface WorkspaceProject {
  id: string;
  title?: string;
}

export async function fetchProjects(): Promise<WorkspaceProject[]> {
  const w = (await fetch("/api/workspace", { cache: "no-store" }).then((r) => r.json())) as {
    projects?: WorkspaceProject[];
  };
  return w.projects ?? [];
}
