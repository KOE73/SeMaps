import type { ColorBy } from "./colors.js";
import type { LayoutKind } from "./layouts.js";

/** How focus (the selection, or the hovered node) is shown: `dim` the rest, only dim with a `selection`, `soft` dimming, or `off`. */
export type FocusMode = "dim" | "selection" | "soft" | "off";

/** What the `grouped` layout groups by (`grouping.ts`). */
export type GroupBy = "assembly" | "namespace" | "folder" | "containers" | "axis" | "community" | "inheritance" | "components";

/** The groupings that have levels (namespace/folder segments, container nesting): each keeps its own. */
export type LevelGroup = "namespace" | "folder" | "containers";
export const LEVEL_GROUPS: readonly LevelGroup[] = ["namespace", "folder", "containers"];

export const COLOR_BYS: readonly ColorBy[] = ["kind", "presence", "group"];
export const LAYOUTS: readonly LayoutKind[] = ["force", "grouped", "hierarchy", "radial", "circlepack", "circular", "random"];
export const GROUP_BYS: readonly GroupBy[] = ["assembly", "namespace", "folder", "containers", "axis", "community", "inheritance", "components"];
export const FOCUS_MODES: readonly FocusMode[] = ["dim", "selection", "soft", "off"];

export interface ViewState {
  colorBy: ColorBy;
  layout: LayoutKind;
  groupBy: GroupBy;
  /** Per grouping: the namespace/folder segments, or container nesting level, that make one group (1 = coarsest). */
  groupDepths: Record<LevelGroup, number>;
  /** The axis (`axis_*`) `groupBy: "axis"` reads; "" = the first the project has. */
  groupAxis: string;
  focus: FocusMode;
}

/** What the data allows the inline controls to offer; set by the engine once it knows the data. */
export interface ViewInfo {
  namespaceDepth: number;
  folderDepth: number;
  containerDepth: number;
  axes: string[];
}

const KEY = "semaps:graph-view";

const DEFAULTS: ViewState = { colorBy: "kind", layout: "force", groupBy: "namespace", groupDepths: { namespace: 1, folder: 1, containers: 1 }, groupAxis: "", focus: "selection" };

function read(): ViewState {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "{}") as Partial<ViewState>;
    return {
      colorBy: COLOR_BYS.includes(raw.colorBy as ColorBy) ? (raw.colorBy as ColorBy) : DEFAULTS.colorBy,
      layout: LAYOUTS.includes(raw.layout as LayoutKind) ? (raw.layout as LayoutKind) : DEFAULTS.layout,
      groupBy: GROUP_BYS.includes(raw.groupBy as GroupBy) ? (raw.groupBy as GroupBy) : DEFAULTS.groupBy,
      groupDepths: Object.fromEntries(
        LEVEL_GROUPS.map((g) => {
          const v = raw.groupDepths?.[g];
          return [g, Number.isInteger(v) && (v as number) >= 1 ? (v as number) : 1];
        }),
      ) as Record<LevelGroup, number>,
      groupAxis: typeof raw.groupAxis === "string" ? raw.groupAxis : DEFAULTS.groupAxis,
      focus: FOCUS_MODES.includes(raw.focus as FocusMode) ? (raw.focus as FocusMode) : DEFAULTS.focus,
    };
  } catch {
    return { ...DEFAULTS };
  }
}

/**
 * The one store of the graph's view settings: colour, layout, grouping and
 * focus. The ribbon's selects and the «Вид» panel both read and change them
 * only here; the engine follows through `onChange`. Kept in the browser, per user.
 */
export class ViewSettings {
  private current: ViewState = read();
  private info: ViewInfo = { namespaceDepth: 1, folderDepth: 1, containerDepth: 1, axes: [] };
  private readonly listeners = new Set<() => void>();

  get state(): Readonly<ViewState> {
    return this.current;
  }

  get available(): Readonly<ViewInfo> {
    return this.info;
  }

  set(patch: Partial<ViewState>): void {
    const next = { ...this.current, ...patch };
    if (JSON.stringify(next) === JSON.stringify(this.current)) return;
    this.current = next;
    try {
      localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
      // not kept, but applied now
    }
    this.emit();
  }

  setInfo(info: ViewInfo): void {
    if (JSON.stringify(info) === JSON.stringify(this.info)) return;
    this.info = info;
    this.emit();
  }

  onChange(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const l of [...this.listeners]) l();
  }
}

export const viewSettings = new ViewSettings();
