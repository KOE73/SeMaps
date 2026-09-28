/**
 * The shapes `GET /api/graph/{project}` answers with (docs/API.md §5,
 * core/graph.go). Kept apart from `shell/api.ts`: that file is the tool API
 * used by the other modes, this one is graph-only and loaded lazily with the
 * rest of the mode.
 */

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
  containers?: string[];
  presence: Presence;
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
}

export interface GraphFactsInfo {
  extractor: string;
  run: string;
  finished: string;
  language: string;
}

export interface GraphStats {
  nodes: number;
  edges: number;
  byPresence: Record<string, number>;
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
  facts: GraphFactsInfo[];
  stats: GraphStats;
}

export async function fetchGraph(project: string): Promise<GraphResponse> {
  const res = await fetch(`/api/graph/${encodeURIComponent(project)}?fields=via,position`, { cache: "no-store" });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return (await res.json()) as GraphResponse;
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
