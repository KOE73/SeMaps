/**
 * The router's fine constants in one place, live.
 *
 * They are a mutable object, not module consts, so the "Line routing" panel can
 * move them and the next render picks the change up: every reader takes the value
 * from `routingTuning` at call time and never keeps a copy. The defaults are what
 * the router was tuned with; a viewer's overrides live in `localStorage` (only
 * the values that differ), so a tuning session survives a reload without ever
 * touching the workspace.
 */

export interface RoutingTuning {
  /**
   * How far a line keeps off a shape it is not attached to. Also half the width of
   * the discouraged band along a container's outline.
   */
  clearance: number;
  /** Cost of a container's border band (and its caption strip), per unit length. */
  borderWeight: number;
  /**
   * A block's halo: a band beyond its clearance where running alongside costs
   * extra, so a line that only passes a block keeps away from its outline
   * instead of tracing it. Crossing the band costs its width — next to nothing.
   */
  haloWidth: number;
  haloWeight: number;
  /**
   * Travel inside a container that does not hold either end, per unit length.
   * Light on purpose: it decides between a run through someone else's container
   * and a run of similar length through the space between containers.
   */
  areaWeight: number;
  /** Spacing between routes that end up sharing a corridor. */
  laneGap: number;
  /** Cost of running along a line already drawn, per unit length. Crossing it costs almost nothing. */
  laneWeight: number;
  /**
   * Width of that band. Wider than the separation pass's gap on purpose: two
   * lines 10 units apart are one stroke at the zoom a whole view is read at,
   * and "separate but indistinguishable" is the same failure as merged.
   */
  laneWidth: number;
  /** What a turn costs, in units of length. Bends read as complexity. */
  bendCost: number;
  /** How far a route leaves a port before it is allowed to turn. */
  stub: number;
  /**
   * What sliding an end to the far end of its side costs; in between it grows with
   * the square of the distance from the middle, measured in half-sides.
   */
  slideCost: number;
  /**
   * What using another side than last time costs. About a bend: enough that two
   * nearly equal routes do not swap sides on every pixel of a drag, little enough
   * that a clearly shorter route on another side still wins.
   */
  sideChangeCost: number;
  /** Landing this close to a line already on the side counts as sharing its spot. */
  takenGap: number;
  /** What sharing a spot costs: more than a bend, so a second line takes the next free spot. */
  takenCost: number;
}

export type RoutingKey = keyof RoutingTuning;

export const ROUTING_DEFAULTS: Readonly<RoutingTuning> = Object.freeze({
  clearance: 8,
  borderWeight: 6,
  haloWidth: 12,
  haloWeight: 1.5,
  areaWeight: 0.5,
  laneGap: 10,
  laneWeight: 3,
  laneWidth: 24,
  bendCost: 40,
  stub: 14,
  slideCost: 20,
  sideChangeCost: 30,
  takenGap: 20,
  takenCost: 60,
});

/** The values in force. Read at call time; change only through `setRoutingTuning`. */
export const routingTuning: RoutingTuning = { ...ROUTING_DEFAULTS };

/** One row of the panel. `label` is a key of `panels.routing.params`. */
export interface RoutingParam {
  readonly key: RoutingKey;
  readonly min: number;
  readonly max: number;
  readonly step: number;
  readonly label: string;
  readonly group: "zones" | "lanes" | "search";
}

export const ROUTING_PARAMS: readonly RoutingParam[] = [
  { key: "clearance", min: 0, max: 30, step: 1, label: "clearance", group: "zones" },
  { key: "borderWeight", min: 0, max: 30, step: 0.5, label: "borderWeight", group: "zones" },
  { key: "haloWidth", min: 0, max: 40, step: 1, label: "haloWidth", group: "zones" },
  { key: "haloWeight", min: 0, max: 10, step: 0.1, label: "haloWeight", group: "zones" },
  { key: "areaWeight", min: 0, max: 5, step: 0.1, label: "areaWeight", group: "zones" },
  { key: "laneGap", min: 2, max: 30, step: 1, label: "laneGap", group: "lanes" },
  { key: "laneWeight", min: 0, max: 20, step: 0.5, label: "laneWeight", group: "lanes" },
  { key: "laneWidth", min: 4, max: 60, step: 1, label: "laneWidth", group: "lanes" },
  { key: "bendCost", min: 0, max: 200, step: 5, label: "bendCost", group: "search" },
  { key: "stub", min: 0, max: 60, step: 1, label: "stub", group: "search" },
  { key: "slideCost", min: 0, max: 100, step: 1, label: "slideCost", group: "search" },
  { key: "sideChangeCost", min: 0, max: 200, step: 5, label: "sideChangeCost", group: "search" },
  { key: "takenGap", min: 0, max: 60, step: 1, label: "takenGap", group: "search" },
  { key: "takenCost", min: 0, max: 300, step: 5, label: "takenCost", group: "search" },
];

const STORAGE_KEY = "semaps.routingTuning";

function save(): void {
  try {
    const diff: Partial<RoutingTuning> = {};
    for (const key of Object.keys(ROUTING_DEFAULTS) as RoutingKey[]) {
      if (routingTuning[key] !== ROUTING_DEFAULTS[key]) diff[key] = routingTuning[key];
    }
    if (Object.keys(diff).length === 0) localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, JSON.stringify(diff));
  } catch {
    /* no storage: the tuning lasts until reload */
  }
}

/** Apply some values (non-finite ones are ignored) and remember the overrides. */
export function setRoutingTuning(partial: Partial<RoutingTuning>): void {
  for (const key of Object.keys(partial) as RoutingKey[]) {
    const value = partial[key];
    if (key in ROUTING_DEFAULTS && typeof value === "number" && Number.isFinite(value)) routingTuning[key] = value;
  }
  save();
}

/** Back to the defaults: everything, or just the given keys. */
export function resetRoutingTuning(keys?: readonly RoutingKey[]): void {
  for (const key of keys ?? (Object.keys(ROUTING_DEFAULTS) as RoutingKey[])) routingTuning[key] = ROUTING_DEFAULTS[key];
  save();
}

function load(): void {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === null) return;
    const got = JSON.parse(raw) as Partial<RoutingTuning>;
    for (const key of Object.keys(got) as RoutingKey[]) {
      const value = got[key];
      if (key in ROUTING_DEFAULTS && typeof value === "number" && Number.isFinite(value)) routingTuning[key] = value;
    }
  } catch {
    /* unreadable or absent: the defaults stay */
  }
}

load();
