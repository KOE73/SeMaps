/** The host places this bearer key in the HTML it serves at /app/. */
export function hostWriteHeaders(): Record<string, string> {
  const key = document.querySelector<HTMLMetaElement>('meta[name="semaps-key"]')?.content;
  return key ? { Authorization: `Bearer ${key}` } : {};
}

let context: string | undefined;
let refreshing: Promise<string | undefined> | undefined;

function contextOf(value: unknown): string | undefined {
  if (!value || typeof value !== "object") return;
  const v = value as Record<string, unknown>;
  const fields = [v.projectFile, v.workspace, v.sourceRoot];
  return fields.every((field) => typeof field === "string") ? JSON.stringify(fields) : undefined;
}

/** Bind recovery to the setup the user actually loaded, never a different project on the same port. */
export function rememberHostContext(setup: unknown): void {
  context = contextOf(setup);
}

async function refreshKey(expected: string): Promise<string | undefined> {
  try {
    const options: RequestInit = { cache: "no-store", redirect: "error", credentials: "same-origin" };
    const [html, setup] = await Promise.all([fetch("/app/", options), fetch("/api/setup", options)]);
    if (!html.ok || !setup.ok || contextOf(await setup.json()) !== expected) return;
    const parsed = new DOMParser().parseFromString(await html.text(), "text/html");
    const key = parsed.querySelector<HTMLMetaElement>('meta[name="semaps-key"]')?.content;
    if (!key || !document.head) return;
    let meta = document.querySelector<HTMLMetaElement>('meta[name="semaps-key"]');
    if (!meta) {
      meta = document.createElement("meta");
      meta.name = "semaps-key";
      document.head.appendChild(meta);
    }
    meta.content = key;
    return key;
  } catch {
    return;
  }
}

/** A key rejection happens before the host executes a write. Retry that rejection once after a safe refresh. */
export async function hostFetch(url: string, init: RequestInit): Promise<Response> {
  if (new URL(url, window.location.href).origin !== window.location.origin) throw new Error("Cross-origin host request refused");
  const method = (init.method ?? "GET").toUpperCase();
  if (method === "GET" || method === "HEAD") return fetch(url, init);
  const expected = context;
  const headers = new Headers(init.headers);
  for (const [name, value] of Object.entries(hostWriteHeaders())) headers.set(name, value);
  const options = { ...init, headers, redirect: "error" as const };
  const response = await fetch(url, options);
  if (response.status !== 401 || !expected || (await response.clone().text()).trim() !== "Missing or invalid host key") return response;
  refreshing ??= refreshKey(expected).finally(() => { refreshing = undefined; });
  const key = await refreshing;
  if (!key || context !== expected || headers.get("Authorization") === `Bearer ${key}`) return response;
  headers.set("Authorization", `Bearer ${key}`);
  return fetch(url, options);
}
