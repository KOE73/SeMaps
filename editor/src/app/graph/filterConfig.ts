/**
 * The graph's filter defaults and presets, as data: `GET /graph-filters.json`
 * (host/defaults/graph-filters.json, or the workspace's own copy). Kinds named
 * there but absent from the graph are skipped by the consumers, never invented.
 * Until it is loaded — or when the host has none — every list is empty and
 * nothing is preselected, hidden or preset.
 */
export interface FilterPreset {
  id: string;
  edgeKinds: readonly string[];
  symbolKinds: readonly string[];
}

export interface GraphFilterConfig {
  defaultEdgeKinds: readonly string[];
  defaultSymbolKinds: readonly string[];
  /** Edge kinds that form the hierarchy; an edge runs child -> base. */
  hierarchyKinds: readonly string[];
  /** Edge kinds that are calls, shown apart in the side panel. */
  callKinds: readonly string[];
  /** Edge kinds an extractor can be told to print. */
  extractorEdgeKinds: readonly string[];
  presets: readonly FilterPreset[];
}

const EMPTY: GraphFilterConfig = {
  defaultEdgeKinds: [],
  defaultSymbolKinds: [],
  hierarchyKinds: [],
  callKinds: [],
  extractorEdgeKinds: [],
  presets: [],
};

let current: GraphFilterConfig = EMPTY;
let loading: Promise<void> | undefined;

const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : []);

export function graphFilterConfig(): GraphFilterConfig {
  return current;
}

export function filterPreset(id: string): FilterPreset | undefined {
  return current.presets.find((p) => p.id === id);
}

/** Fetches the config once; a failure leaves the empty one. */
export function loadGraphFilterConfig(): Promise<void> {
  loading ??= fetch("/graph-filters.json", { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : Promise.reject(new Error(r.statusText))))
    .then((raw: Record<string, unknown>) => {
      const presets = Array.isArray(raw.presets) ? (raw.presets as Record<string, unknown>[]) : [];
      current = {
        defaultEdgeKinds: strings(raw.defaultEdgeKinds),
        defaultSymbolKinds: strings(raw.defaultSymbolKinds),
        hierarchyKinds: strings(raw.hierarchyKinds),
        callKinds: strings(raw.callKinds),
        extractorEdgeKinds: strings(raw.extractorEdgeKinds),
        presets: presets
          .filter((p) => typeof p.id === "string")
          .map((p) => ({ id: p.id as string, edgeKinds: strings(p.edgeKinds), symbolKinds: strings(p.symbolKinds) })),
      };
    })
    .catch(() => {
      current = EMPTY;
    });
  return loading;
}
