import type { WireDocument } from "../../model/wire-types.js";

/**
 * Offer a generated file to the user as a browser download.
 */
export function download(fileName: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * Read a locally selected JSON file in the browser.
 */
export function readJsonFile(file: File): Promise<WireDocument> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("Не удалось прочитать файл"));
    reader.onload = () => {
      try {
        resolve(JSON.parse(String(reader.result)) as WireDocument);
      } catch (err) {
        reject(new Error(`Ошибка разбора JSON: ${(err as Error).message}`));
      }
    };
    reader.readAsText(file);
  });
}
