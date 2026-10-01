import type { CommandContext } from "../commands/types.js";

/*
 * Ribbon structure. A mode declares data only: tabs -> groups -> items. One
 * renderer (RibbonRenderer) and one stylesheet (ribbon.css) decide the look;
 * modes add neither DOM nor CSS to the ribbon.
 *
 * Items: `button` and `toggle` run a registered command (a toggle shows the
 * command's `isChecked`); `select` runs it with the chosen value; `separator`;
 * `theme-gallery`. A command's `icon` is a Tabler svg string (ui/icons.ts).
 * Sizes: `large` (icon over caption), `medium` (icon beside caption), `small`
 * (icon only). `showLabel: false` drops the caption of a large/medium item
 * (icon only). The tooltip is the app's own, from the command's title and
 * description; items carry an aria-label, never a native `title`.
 */

export type RibbonItemSize = "large" | "medium" | "small";

export interface RibbonButtonSpec {
  readonly type: "button";
  readonly command: string;
  readonly size?: RibbonItemSize;
  readonly label?: string;
  /** An emoji or an inline `<svg …>` string. */
  readonly icon?: string;
  /** `false`: icon only, the title shows as a tooltip. */
  readonly showLabel?: boolean;
}

export interface RibbonToggleSpec {
  readonly type: "toggle";
  readonly command: string;
  readonly size?: RibbonItemSize;
  readonly label?: string;
  readonly icon?: string;
  readonly showLabel?: boolean;
}

export interface RibbonSelectOption {
  readonly value: string;
  readonly label: string;
}

export interface RibbonSelectSpec {
  readonly type: "select";
  readonly command: string;
  readonly label: string;
  readonly options: readonly RibbonSelectOption[];
  readonly getValue: (context: CommandContext) => string;
  /** Greyed out while it does not apply (a choice that still works, and may switch the mode it applies in). */
  readonly dimmed?: (context: CommandContext) => boolean;
}

export interface RibbonSeparatorSpec {
  readonly type: "separator";
}

export interface RibbonThemeGalleryOption {
  readonly id: string;
  readonly name: string;
}

export interface RibbonThemeGallerySpec {
  readonly type: "theme-gallery";
  readonly command: string;
  readonly themes: readonly RibbonThemeGalleryOption[];
  readonly getValue: (context: CommandContext) => string;
}

export type RibbonItemSpec =
  | RibbonButtonSpec
  | RibbonToggleSpec
  | RibbonSelectSpec
  | RibbonSeparatorSpec
  | RibbonThemeGallerySpec;

export interface RibbonGroupSpec {
  readonly id: string;
  readonly title: string;
  readonly items: readonly RibbonItemSpec[];
}

export interface RibbonTabSpec {
  readonly id: string;
  readonly title: string;
  readonly keyTip?: string;
  readonly contextual?: "node" | "zone" | "edge";
  readonly groups: readonly RibbonGroupSpec[];
}

export interface RibbonSpec {
  readonly tabs: readonly RibbonTabSpec[];
}
