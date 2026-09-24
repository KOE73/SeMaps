/**
 * Client of the host's tool API (docs/API.md §5): the .semaps settings, the
 * extractors, their runs and the sync of a run. Paths are relative to the
 * project root; the host never hands out absolute ones.
 */

export interface RuntimeInfo {
  name: string;
  ok: boolean;
  version?: string;
  hint?: string;
}

export interface ExtractorTool {
  language: string;
  found: boolean;
  source?: "command" | "bundled" | "path";
  where?: string;
  runtime?: RuntimeInfo;
  problem?: string;
}

export interface RunStats {
  symbols: number;
  edges: number;
  symbolKinds: Record<string, number>;
  edgeKinds: Record<string, number>;
  language: string;
}

export interface RunInfo {
  id: string;
  extractor: string;
  project: string;
  language: string;
  started: string;
  finished?: string;
  seconds?: number;
  state: "running" | "done" | "failed";
  exitCode: number;
  error?: string;
  stats?: RunStats;
}

export interface ExtractorView {
  id: string;
  language: string;
  project: string;
  root: string;
  include: string[];
  exclude: string[];
  edges: string[];
  command?: string;
  tool: ExtractorTool;
  lastRun?: RunInfo;
}

export interface Setup {
  projectFile: string;
  name: string;
  workspace: string;
  sourceRoot: string;
  port: number;
  extractors: ExtractorView[];
  languages: string[];
}

export interface Tools {
  projectFile: string;
  shipped: string[];
  runtimes: RuntimeInfo[];
  extractors: ExtractorView[];
}

export interface SyncReport {
  project: string;
  language: string;
  symbols: number;
  edges: number;
  dryRun: boolean;
  broken: string[] | null;
  ambiguous: string[] | null;
  renames: string[] | null;
  gone: string[] | null;
  added: string[] | null;
  changed: string[] | null;
  written: string[] | null;
}

export interface SyncResult {
  report: SyncReport;
  exitCode: number;
  empty: boolean;
}

export interface ExtractorPatch {
  language?: string;
  project?: string;
  root?: string;
  include?: string[];
  exclude?: string[];
  edges?: string[];
}

export interface SettingsPatch {
  name?: string;
  workspace?: string;
  sourceRoot?: string;
  port?: number;
}

/** The host answered with an error; `message` is its text. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function call<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  if (!res.ok) throw new ApiError(res.status, (await res.text()).trim() || res.statusText);
  return (await res.json()) as T;
}

const API = "/api";

export const toolApi = {
  setup: () => call<Setup>("GET", `${API}/setup`),
  tools: () => call<Tools>("GET", `${API}/tools`),
  saveSettings: (patch: SettingsPatch) => call<Setup>("PUT", `${API}/setup`, patch),
  saveExtractor: (id: string, patch: ExtractorPatch) =>
    call<Setup>("PUT", `${API}/setup/extractors/${encodeURIComponent(id)}`, patch),
  removeExtractor: (id: string) => call<Setup>("DELETE", `${API}/setup/extractors/${encodeURIComponent(id)}`),
  runs: (extractor?: string) =>
    call<RunInfo[]>("GET", `${API}/runs${extractor ? `?extractor=${encodeURIComponent(extractor)}` : ""}`),
  startRun: (extractor: string) => call<RunInfo>("POST", `${API}/runs`, { extractor }),
  run: (id: string) => call<RunInfo>("GET", `${API}/runs/${encodeURIComponent(id)}`),
  log: (id: string, offset: number) =>
    call<{ text: string; offset: number; state: RunInfo["state"] }>(
      "GET",
      `${API}/runs/${encodeURIComponent(id)}/log?offset=${offset}`,
    ),
  sync: (id: string, dryRun: boolean, noRenames = false) =>
    call<SyncResult>("POST", `${API}/runs/${encodeURIComponent(id)}/sync`, { dryRun, noRenames }),
};
