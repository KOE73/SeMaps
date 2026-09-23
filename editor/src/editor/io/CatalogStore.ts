import type { CatalogEntry, CatalogFile } from "./types.js";

const CATALOG_FILE = "catalog.json";

/**
 * Loads the catalog of diagram schemas from catalog.json.
 */
export async function loadCatalog(baseUrl: string = "./"): Promise<CatalogEntry[]> {
  try {
    const res = await fetch(new URL(CATALOG_FILE, new URL(baseUrl, location.href)));
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = (await res.json()) as CatalogFile;
    return data.schemas ?? [];
  } catch (err) {
    console.warn("Каталог схем не загружен:", err);
    return [];
  }
}

/**
 * Saves the catalog of diagram schemas back to catalog.json.
 */
export async function saveCatalog(schemas: CatalogEntry[]): Promise<void> {
  const fileData: CatalogFile = { schemas };
  const res = await fetch(`/api/save?file=${encodeURIComponent(CATALOG_FILE)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(fileData, null, 2),
  });
  if (!res.ok) throw new Error(`Не удалось сохранить ${CATALOG_FILE}: HTTP ${res.status}`);
}

export class HttpCatalogStore {
  constructor(private readonly baseUrl: string = "./") {}

  load(): Promise<CatalogEntry[]> {
    return loadCatalog(this.baseUrl);
  }

  save(schemas: CatalogEntry[]): Promise<void> {
    return saveCatalog(schemas);
  }
}
