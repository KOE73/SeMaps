import { t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { kindIcon } from "../../ui/kindIcons.js";
import type { ColorBy } from "./colors.js";
import type { LayoutKind } from "./layouts.js";
import type { FocusMode, GroupBy } from "./viewSettings.js";

export interface ViewOption<T extends string> {
  readonly value: T;
  readonly label: string;
  /** An icon of the registry (svg). */
  readonly icon: string;
}

/** The entries of «Цвет по», «Раскладка», «Группировать по» and «Подсветка»: the ribbon's selects and the «Вид» panel list the same ones. */
export const COLOR_OPTIONS: readonly ViewOption<ColorBy>[] = [
  { value: "kind", label: t.graphColorKind, icon: kindIcon("symbol", "class") },
  { value: "container", label: t.graphColorContainer, icon: icons.boxMultiple },
  { value: "namespace", label: t.graphColorNamespace, icon: icons.brackets },
  { value: "presence", label: t.graphColorPresence, icon: icons.boxModel },
];

export const LAYOUT_OPTIONS: readonly ViewOption<LayoutKind>[] = [
  { value: "force", label: t.graphLayoutForce, icon: icons.topologyStar3 },
  { value: "grouped", label: t.graphLayoutGrouped, icon: icons.category },
  { value: "hierarchy", label: t.graphLayoutHierarchy, icon: icons.hierarchy },
  { value: "radial", label: t.graphLayoutRadial, icon: icons.target },
  { value: "circlepack", label: t.graphLayoutCirclepack, icon: icons.circleDot },
  { value: "circular", label: t.graphLayoutCircular, icon: icons.circle },
  { value: "random", label: t.graphLayoutRandom, icon: icons.sparkles },
];

export const GROUP_OPTIONS: readonly ViewOption<GroupBy>[] = [
  { value: "assembly", label: t.graphGroupAssembly, icon: icons.pkg },
  { value: "namespace", label: t.graphGroupNamespace, icon: icons.brackets },
  { value: "folder", label: t.graphGroupFolder, icon: icons.folder },
  { value: "containers", label: t.graphGroupContainers, icon: icons.boxMultiple },
  { value: "axis", label: t.graphGroupAxis, icon: icons.layoutList },
  { value: "community", label: t.graphGroupCommunity, icon: icons.topologyStar3 },
  { value: "inheritance", label: t.graphGroupInheritance, icon: icons.binaryTree },
  { value: "components", label: t.graphGroupComponents, icon: icons.link },
];

export const FOCUS_OPTIONS: readonly ViewOption<FocusMode>[] = [
  { value: "dim", label: t.graphFocusDim, icon: icons.eyeOff },
  { value: "selection", label: t.graphFocusSelection, icon: icons.focus2 },
  { value: "soft", label: t.graphFocusSoft, icon: icons.sunglasses },
  { value: "off", label: t.graphFocusOff, icon: icons.circleX },
];
