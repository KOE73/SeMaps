import type { RibbonTabSpec } from "./ribbon/types.js";

/**
 * A mode of the workbench: what the ribbon offers and what fills the area
 * under it. The diagram editor is one mode; the tool pages (extractors, the
 * .semaps project) are others. Switching a mode swaps the tabs and shows its
 * surface; nothing is rebuilt, so a mode keeps its state while hidden.
 */
export interface WorkbenchMode {
  /** Also the URL hash that opens it; the diagram mode has none. */
  readonly id: string;
  readonly title: string;
  readonly tabs: readonly RibbonTabSpec[];
  /** The area under the ribbon while the mode is active. */
  readonly surface: HTMLElement;
  /** Every time the mode is entered; the first time it may load its data. */
  enter?(): void;
}
