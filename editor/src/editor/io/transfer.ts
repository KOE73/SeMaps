/**
 * Re-exporting stores and utilities from modular files in editor/io/
 * for backwards compatibility.
 */
export * from "./types.js";
export * from "./CatalogStore.js";
export * from "./StyleStore.js";
export * from "./ProjectStore.js";
export { HttpProjectStore as HttpModelStore } from "./ProjectStore.js";
export * from "./StandaloneStore.js";
export * from "./fileUtils.js";
export * from "./drawio.js";
