// Imported as text, not as a JSON module: the file lies outside this package,
// and a JSON module would pull it into the declaration build's root.
import defaultStylesText from "../../../host/defaults/styles.json?raw";
import type { WireStyleSheet } from "./style-types.js";

/**
 * The starting library — the tool's own `host/defaults/styles.json`, bundled
 * here by import so the editor and the Go host read ONE file instead of two
 * copies that drift.
 *
 * This is a fallback, not the source of truth. `styles.json` is — the one
 * shipped in `host/defaults/`, or the workspace's own override; the host serves
 * either at `/styles.json`. When that request fails, or before it has answered
 * (the canvas is constructed synchronously and must never exist without a
 * library), the canvas draws with this sheet instead of going grey.
 */
const builtin = JSON.parse(defaultStylesText) as WireStyleSheet;

export function builtinStyleSheet(): WireStyleSheet {
  return structuredClone(builtin);
}
