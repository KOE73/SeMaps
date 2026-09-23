/**
 * The style contract as it exists on disk — `styles.json` (host defaults or workspace override).
 *
 * A style is a *named* look with an identity of its own, shared by every
 * element that resolves to it. Elements no longer carry colours; they carry at
 * most a `styleId`, and usually not even that (see `StyleLibrary.resolve`).
 * Changing a style changes every element wearing it, which is the whole point:
 * before this, "make all records look different from classes" meant editing
 * three hundred objects or a hardcoded table in the renderer.
 *
 * Like `wire-types.ts`, this describes the file and nothing else, and it is
 * deliberately permissive: every field is optional and nothing is validated on
 * load. Missing fields fall back through `basedOn` and then to the built-in
 * defaults, so a style may be as small as `{ "id": "record", "fill": "#eef" }`.
 *
 * Do not use these types outside `StyleLibrary.ts`. Renderers consume the
 * resolved forms (`ResolvedBlockStyle` / `ResolvedEdgeStyle`), which have every
 * field filled in and no inheritance left to chase.
 */

/** Which family of thing a style can dress. */
export type StyleTarget = "block" | "edge";

export interface WireGradientStop {
  /** 0…1 along the gradient axis. */
  offset: number;
  color: string;
  opacity?: number;
}

/**
 * A fill: either a plain CSS colour, or a gradient that has to be materialised
 * into `<defs>` before it can be referenced.
 *
 * The string form is kept as a bare string rather than `{ kind: "solid" }` so
 * that the overwhelmingly common case stays one word in the JSON.
 */
export type WirePaint =
  | string
  | {
      kind: "linear";
      /** Degrees clockwise from "left to right". 90 is top to bottom. */
      angle?: number;
      stops: WireGradientStop[];
    }
  | {
      kind: "radial";
      stops: WireGradientStop[];
    };

export interface WireStroke {
  color?: string;
  width?: number;
  /** SVG dash pattern, or "none". */
  dash?: string;
  opacity?: number;
}

export interface WireText {
  family?: string;
  size?: number;
  /** 100…900, as in CSS. */
  weight?: number;
  italic?: boolean;
  color?: string;
  align?: "start" | "middle" | "end";
  opacity?: number;
  /** Draw this text at all. Turning off a subtitle is a common style choice. */
  show?: boolean;
}

/**
 * Shapes available for an edge end.
 *
 * "none" is a real choice, not an absence: an association line with a head at
 * one end only is how most notations distinguish direction from mere adjacency.
 */
export type EndShape =
  | "none"
  | "arrow"
  | "arrow-open"
  | "triangle"
  | "triangle-hollow"
  | "diamond"
  | "diamond-hollow"
  | "circle"
  | "circle-hollow"
  | "bar";

export interface WireEndpoint {
  shape?: EndShape;
  /** Marker box in user units; roughly the head's length. */
  size?: number;
  /**
   * Head colour. Absent means "follow the line", which is what makes the head
   * and the line stop drifting apart — the old fixed marker table baked a
   * colour into each arrow and nothing kept the two in sync.
   */
  color?: string;
}

export interface WireIcon {
  /** Emoji or any short glyph. */
  glyph?: string;
  show?: boolean;
}

/** Header band of a container. Ignored when the style dresses a leaf node. */
export interface WireHeader {
  fill?: WirePaint;
  height?: number;
  text?: WireText;
}

/**
 * The outline a node is drawn with, and therefore where lines attach to it.
 *
 * A shape belongs to the style rather than to each node because it follows
 * from what the thing *is*: a use case is an ellipse, a store is a cylinder,
 * an external role is an actor. Said once in the style, a hundred nodes stay
 * consistent; said per node, they drift within a week.
 */
export type BlockShape = "rect" | "ellipse" | "diamond" | "cylinder" | "hexagon" | "actor";

/**
 * How an edge's line is drawn between two already-placed ports.
 *
 * A choice, never a result: the polyline itself is recomputed on every repaint
 * and is not stored anywhere (ADR_20260903 §2.7). Keeping the mode here — on
 * the style, keyed by relation type — means inheritance is a tree-shaped line
 * and flow is a curve without anyone choosing per edge.
 */
export type RoutingMode = "bezier" | "orthogonal" | "tree-horizontal" | "tree-vertical";

export interface WireStyle {
  id: string;
  name?: string;
  appliesTo?: StyleTarget;
  description?: string;
  /** Free-form grouping, used by the picker's filter. */
  tags?: string[];
  /**
   * Inherit every unset field from another style. One chain, cycles ignored.
   * With dozens of styles this is what keeps "the record variant of a class"
   * from being a copy that silently rots.
   */
  basedOn?: string;

  // ---- block ----------------------------------------------------------
  fill?: WirePaint;
  border?: WireStroke;
  /** Corner radius. */
  radius?: number;
  shadow?: boolean;
  header?: WireHeader;
  title?: WireText;
  subtitle?: WireText;
  icon?: WireIcon;
  /** Outline; absent means a plain rectangle. */
  shape?: BlockShape;
  /**
   * Id of the content template drawn inside the outline, from `templates.json`.
   *
   * Absent means the built-in "title only" look, which is what every node had
   * before templates existed. A placement may override it for the one node
   * that must show more (or less) than its kind normally does.
   */
  template?: string;

  // ---- edge -----------------------------------------------------------
  line?: WireStroke;
  /** Line shape for this relation type; absent inherits the view's choice. */
  routing?: RoutingMode;
  /** Head at the `from` end. */
  source?: WireEndpoint;
  /** Head at the `to` end. Defaults to a filled arrow. */
  target?: WireEndpoint;
  label?: WireText;
  /**
   * Structure (how the code is assembled) or flow (what happens at runtime).
   *
   * Not decoration: the canvas can hide a whole family at once. In
   * `model-core-full.json` 98 of 119 edges are `implements` or `extends`, and no
   * amount of restyling makes a diagram readable when four fifths of its lines
   * are assembly detail — being able to switch them off is what does.
   */
  family?: "structure" | "flow";
  /** Whether ghost/shadow relations of this style appear in the global bird's-eye overview. */
  overview?: boolean;
}

export interface WireStyleSheet {
  /** Informational; the loader accepts any value. */
  version?: number;
  description?: string;
  styles?: WireStyle[];
}
