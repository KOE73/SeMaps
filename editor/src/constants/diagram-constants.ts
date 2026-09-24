/**
 * ============================================================================
 * SeMaps global layout & Geometry Constants
 * ============================================================================
 * Centralized repository for all visual layout metrics, corner insets,
 * spacing gaps, geometry constraints, and interaction timings.
 *
 * Tweak values here to experiment with visual aesthetics, port distribution,
 * and component density across the entire diagram editor.
 */

export const DIAGRAM_CONFIG = {
  // ---------------------------------------------------------------- Ports & Edges
  ports: {
    /** Default corner inset (px) for shapes without an explicit cornerInset implementation */
    defaultCornerInset: 8,

    /** Additional safety margin added to shape corner radius in px */
    extraCornerGap: 0,

    /** Minimum distance between adjacent connection ports on a side in px */
    minPortGap: 16,

    /** Exclusion halo radius around anchored straight connection lines in px */
    haloRadius: 14,

    /** Default port step for DiscretePortAssigner in px */
    discreteStep: 20,
  },

  // ---------------------------------------------------------------- Node Layout
  node: {
    /** Top bar vertical offset from top border (px) */
    topBarY: 5,

    /** Height of top bar control items (doc/code buttons, badges) in px */
    topBarHeight: 14,

    /** Width of the [📄] doc button in px */
    docButtonWidth: 18,

    /** Width of the [💻] code button in px */
    codeButtonWidth: 18,

    /** Horizontal padding inside the node in px */
    padX: 10,

    /** Default corner radius for standard nodes in px */
    defaultRadius: 8,

    /** Y position for element title (tall / normal) in px */
    titleY: (tall: boolean): number => (tall ? 36 : 33),

    /** Y position for semantic subtitle in px */
    subtitleY: (tall: boolean): number => (tall ? 52 : 47),

    /**
     * Top offset for template-drawn content in px.
     *
     * Smaller than `titleY` on purpose: a template's first row *is* its
     * caption, so it starts right below the button bar instead of leaving a
     * gap sized for a caption that is already part of the content.
     */
    contentTop: 22,

    /** Minimum allowed size for a node { width, height } in px */
    minSize: { width: 100, height: 40 },
  },

  // ----------------------------------------------------------- Container / Zone
  container: {
    /** Height of zone header bar in px */
    headerHeight: 28,

    /** Inner padding for child elements in px */
    padding: 16,

    /** Left inset of the header caption: clear of the collapse toggle */
    titlePad: 36,

    /** Minimum allowed size for a zone { width, height } in px */
    minSize: { width: 160, height: 100 },

    /** Default corner radius for container zones in px */
    defaultRadius: 10,
  },

  // ------------------------------------------------------------------ Routing
  routing: {
    /** Below this perpendicular offset in px, bezier collapses to a straight line */
    bezierStraightThreshold: 6,

    /** Maximum control point offset distance for bezier curves in px */
    bezierMaxHandle: 60,

    /** Default marker / arrowhead size */
    defaultMarkerSize: 12,

    /** How far an end label sits from the port along the line, in px. */
    endLabelAlong: 14,

    /** How far an end label sits off the line (perpendicular), so it clears the stroke. */
    endLabelPerp: 9,
  },

  // ----------------------------------------------------------------- Handles & Grid
  handles: {
    /** Base size of resize grips in px */
    size: 10,

    /** Default grid step for snapping in px */
    defaultGridStep: 10,
  },

  // ----------------------------------------------------------------- UI Timings
  interaction: {
    /** Delay in ms before edge floating controls hide after mouse leave */
    edgeControlsHideDelayMs: 350,
    /** Delay in ms before the edge control bubble appears: only when the mouse rests on a line, not when it crosses one */
    edgeControlsShowDelayMs: 500,

    /** Time in ms of typing inactivity before committing a text field edit to history */
    fieldEditQuietMs: 600,

    /** Rich tooltip hover offset in px */
    tooltipOffset: 14,
  },
} as const;

export type DiagramConfig = typeof DIAGRAM_CONFIG;
