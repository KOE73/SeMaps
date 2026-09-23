import { sanitizeSvg } from "./sanitizeSvg.js";

/** File listing what the content directory holds. */
const MANIFEST_FILE = "content/index.json";

/** One picture the registry can hand out, as described by the manifest. */
export interface AssetEntry {
  /** Name used in a template: `@Asset(brain)`. */
  readonly id: string;
  /** Human name for the picker. Falls back to the id. */
  readonly name?: string;
  /** Path relative to the workspace root. */
  readonly file: string;
  readonly description?: string;
}

interface AssetManifest {
  readonly assets?: readonly AssetEntry[];
}

/** A picture that has been fetched, cleaned and is ready to insert. */
export interface LoadedAsset {
  readonly entry: AssetEntry;
  /** Sanitised markup, or empty when the file could not be used at all. */
  readonly svg: string;
  /** What sanitising took out, for the panel to show. */
  readonly removed: readonly string[];
  readonly error?: string;
}

/**
 * The named pictures a template can pull in.
 *
 * Assets are addressed by name rather than embedded in the model, so that the
 * same picture can be reused, renamed once, and previewed in a list — and so
 * that view files stay about geometry instead of carrying kilobytes of markup.
 *
 * Reading is deliberately all this class does. Pictures arrive in the content
 * directory by being put there, not through the editor: that keeps the host's
 * write path to `.json` only (`API.md` §3.3) and keeps arbitrary file upload
 * out of a tool whose whole job is drawing.
 */
export class AssetRegistry {
  private manifest: Promise<readonly AssetEntry[]> | null = null;
  private readonly loaded = new Map<string, Promise<LoadedAsset>>();
  /** Settled results, readable without awaiting — see `peek`. */
  private readonly ready = new Map<string, LoadedAsset>();
  private readonly listeners = new Set<(id: string) => void>();

  constructor(private readonly baseUrl: string = "./") {}

  /** Everything the picker should offer. A missing manifest means "none". */
  list(): Promise<readonly AssetEntry[]> {
    this.manifest ??= this.fetchManifest();
    return this.manifest;
  }

  /**
   * Fetch one picture by name, cleaned and cached.
   *
   * Never rejects: a template naming a picture that has gone missing should
   * draw a block with a complaint in it, not tear down the frame around it.
   */
  async get(id: string): Promise<LoadedAsset> {
    const cached = this.loaded.get(id);
    if (cached !== undefined) return cached;

    const pending = this.fetchAsset(id);
    this.loaded.set(id, pending);
    return pending;
  }

  /**
   * The picture if it is already in hand, otherwise nothing — and a fetch
   * started in the background.
   *
   * Drawing a frame is synchronous while fetching a file is not, and the way
   * out is not to make the canvas async: it is to draw what is known now and
   * repaint when more arrives. First frame after a template names a new
   * picture shows the block without it; `onLoaded` brings the next frame.
   */
  peek(id: string): LoadedAsset | undefined {
    const cached = this.ready.get(id);
    if (cached !== undefined) return cached;

    if (!this.loaded.has(id)) {
      void this.get(id).then((asset) => {
        this.ready.set(id, asset);
        this.listeners.forEach((fn) => fn(id));
      });
    }
    return undefined;
  }

  /** Called when a picture finishes loading, so the canvas can repaint. */
  onLoaded(fn: (id: string) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  /** Drop caches so that a picture edited on disk is picked up. */
  invalidate(): void {
    this.manifest = null;
    this.loaded.clear();
    this.ready.clear();
  }

  private async fetchManifest(): Promise<readonly AssetEntry[]> {
    try {
      const res = await fetch(this.resolve(MANIFEST_FILE));
      if (!res.ok) return [];
      const parsed = (await res.json()) as AssetManifest;
      return parsed.assets?.filter((a) => typeof a.id === "string" && typeof a.file === "string") ?? [];
    } catch {
      // No manifest is a normal state for a workspace that uses no pictures.
      return [];
    }
  }

  private async fetchAsset(id: string): Promise<LoadedAsset> {
    const entries = await this.list();
    const entry = entries.find((a) => a.id === id);
    if (entry === undefined) {
      return { entry: { id, file: "" }, svg: "", removed: [], error: `ресурс «${id}» не найден` };
    }

    try {
      const res = await fetch(this.resolve(entry.file));
      if (!res.ok) {
        return { entry, svg: "", removed: [], error: `HTTP ${res.status}` };
      }
      const report = sanitizeSvg(await res.text());
      return { entry, svg: report.svg, removed: report.removed, error: report.error };
    } catch (e) {
      return { entry, svg: "", removed: [], error: e instanceof Error ? e.message : String(e) };
    }
  }

  private resolve(file: string): URL {
    return new URL(file, new URL(this.baseUrl, location.href));
  }
}
