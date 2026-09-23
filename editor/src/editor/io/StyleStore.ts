import { builtinStyleSheet } from "../../model/style-defaults.js";
import type { WireStyleSheet } from "../../model/style-types.js";
import type { StyleStore } from "./types.js";

/** File name the stylesheet occupies in the workspace models directory. */
const STYLE_FILE = "styles.json";

/**
 * Reads and writes `styles.json` over the HTTP host API.
 *
 * A missing file (404) returns the built-in stylesheet fallback so the editor
 * is always fully functional out of the box.
 */
export class HttpStyleStore implements StyleStore {
  constructor(private readonly baseUrl: string = "./") {}

  async load(): Promise<WireStyleSheet> {
    const res = await fetch(new URL(STYLE_FILE, new URL(this.baseUrl, location.href)));
    if (res.status === 404) return builtinStyleSheet();
    if (!res.ok) throw new Error(`Не удалось загрузить ${STYLE_FILE}: HTTP ${res.status}`);
    return (await res.json()) as WireStyleSheet;
  }

  async save(sheet: WireStyleSheet): Promise<void> {
    const res = await fetch(`/api/save?file=${encodeURIComponent(STYLE_FILE)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sheet, null, 2),
    });
    if (!res.ok) throw new Error(`Сервер ответил HTTP ${res.status}`);
  }
}
