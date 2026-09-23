import type { WireDocument } from "../../model/wire-types.js";
import type { ModelStore, SaveTarget } from "./types.js";

/**
 * Loads and saves standalone single-file diagram models.
 */
export class HttpStandaloneStore implements ModelStore {
  constructor(private readonly baseUrl: string = "./") {}

  async load(file: string): Promise<WireDocument> {
    const res = await fetch(new URL(file, new URL(this.baseUrl, location.href)));
    if (!res.ok) throw new Error(`Не удалось загрузить ${file}: HTTP ${res.status}`);
    return (await res.json()) as WireDocument;
  }

  async save(target: SaveTarget, wire: WireDocument): Promise<void> {
    const res = await fetch(`/api/save?file=${encodeURIComponent(target.file)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(wire, null, 2),
    });
    if (!res.ok) throw new Error(`Сервер ответил HTTP ${res.status}`);
  }
}
