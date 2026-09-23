/**
 * What is left of the old visual vocabulary once looks became data.
 *
 * `NODE_STYLES`, `ZONE_DEFAULTS` and the edge table used to live here as
 * hardcoded records: changing how a record differs from a class meant editing
 * the renderer. They are now ordinary styles in `model/style-defaults.ts`,
 * resolved through `StyleLibrary` and overridable from the workspace `styles.json`.
 *
 * Only the two things that are not a *look* remain.
 */

/**
 * Fallback height of a container's header.
 *
 * The real height is `ResolvedBlockStyle.header.height` — a style may make its
 * header taller — so anything holding a style must read it from there. This
 * constant is for code that has no style in hand and must still guess, and any
 * such guess can disagree with what is actually drawn.
 */
export const HEADER_HEIGHT = 34;

/** Opacity applied to elements outside the active view (R-VIEW-03/04/05). */
export const DIM = {
  zone: 0.25,
  node: 0.2,
  edge: 0.15,
} as const;
