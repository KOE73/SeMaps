import type { CommandContext } from "../commands/types.js";

export type RibbonItemSize = "large" | "medium" | "small";

export interface RibbonButtonSpec {
  readonly type: "button";
  readonly command: string;
  readonly size?: RibbonItemSize;
  readonly label?: string;
  readonly icon?: string;
}

export interface RibbonToggleSpec {
  readonly type: "toggle";
  readonly command: string;
  readonly size?: RibbonItemSize;
  readonly label?: string;
  readonly icon?: string;
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
