/**
 * Client of the host's tool API (docs/API.md §5): the .semaps settings, the
 * extractors, their runs and the sync of a run. Paths are relative to the
 * project root; the host never hands out absolute ones.
 */
import { hostWriteHeaders } from "../util/hostKey.js";

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
  trigger?: "watch";
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
  watch: boolean;
  command?: string;
  tool: ExtractorTool;
  lastRun?: RunInfo;
}

/**
 * The `mcp` section of the .semaps file (docs/API.md §3): shapes the MCP
 * tools an agent gets. The host answers this with Go's own capitalised field
 * names (not camelCase, despite docs/API.md) — read through these keys as
 * they really come back; `PUT /api/setup` still takes camelCase (tested and
 * confirmed against the running host).
 */
export interface McpSettings {
  Tools: "one" | "narrow";
  Description: "brief" | "standard" | "full";
  Format: string;
  ListCap: number;
  Limit: number;
}

export interface Setup {
  projectFile: string;
  name: string;
  workspace: string;
  sourceRoot: string;
  port: number;
  extractors: ExtractorView[];
  languages: string[];
  mcp: McpSettings;
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
  watch?: boolean;
}

export interface McpSettingsPatch {
  tools?: "one" | "narrow";
  description?: "brief" | "standard" | "full";
  format?: string;
  listCap?: number;
  limit?: number;
}

export interface SettingsPatch {
  name?: string;
  workspace?: string;
  sourceRoot?: string;
  port?: number;
  mcp?: McpSettingsPatch;
}

/** One of the six `tools`×`description` combinations `/api/mcp` reports (PLAN_20260928-7 step 5). */
export interface McpCombo {
  tools: "one" | "narrow";
  description: "brief" | "standard" | "full";
  graphTools: McpTool[];
  instructions: string;
  bytes: number;
  estimateTokens: number;
}

/** GET /api/mcp: does the project's .mcp.json start `semaps mcp`, and what it offers. */
export interface McpStatus {
  file: string;
  exists: boolean;
  configured: boolean;
  entry: string;
  onPath: boolean;
  snippet: string;
  tools: McpTool[];
  mcpTools: "one" | "narrow";
  mcpDescription: "brief" | "standard" | "full";
  combos: McpCombo[];
  error?: string;
}

export interface McpTool {
  name: string;
  description: string;
  /** readOnlyHint: the tool only reads. The sandbox asks before any other. */
  readOnly: boolean;
  inputSchema: JsonSchema;
}

export interface JsonSchema {
  type?: string | string[];
  description?: string;
  properties?: Record<string, JsonSchema>;
  required?: string[];
  items?: JsonSchema;
}

/** POST /api/mcp/call: one sandbox call, the JSON-RPC messages as they went. */
export interface McpCallResult {
  messages: { dir: "out" | "in"; message: unknown }[];
  ms: number;
  isError: boolean;
  error?: string;
}

/** GET /api/graph-formats: the formats, relation vocabulary and template grammar. */
export interface GraphFormats {
  formats: { name: string; description: string; mediaType: string; template?: string }[];
  relations: { name: string; inverse: string; kind: string; typeMatch: string; description: string }[];
  defaultFollow: string[];
  template: { rules: string[]; examples: { text: string; result: string }[] };
  defaults: { format: string; level: { neighbourhood: string; wholeGraph: string } };
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
    headers: { ...(body === undefined ? {} : { "Content-Type": "application/json" }), ...(method === "GET" ? {} : hostWriteHeaders()) },
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
  mcp: () => call<McpStatus>("GET", `${API}/mcp`),
  installMcp: () => call<McpStatus>("POST", `${API}/mcp/install`),
  callMcp: (name: string, args: Record<string, unknown>) =>
    call<McpCallResult>("POST", `${API}/mcp/call`, { name, arguments: args }),
  graphFormats: () => call<GraphFormats>("GET", `${API}/graph-formats`),
  sync: (id: string, dryRun: boolean, noRenames = false) =>
    call<SyncResult>("POST", `${API}/runs/${encodeURIComponent(id)}/sync`, { dryRun, noRenames }),
};
