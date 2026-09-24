/**
 * Derive edge labels from member relation signatures (`via`).
 *
 * For relations with `origin: "code"`, the edge label is computed from the member's
 * structure rather than from authored text (ADR_20260924-4 §7).
 */

import type { RelationVia } from "./wire-types.js";

/**
 * Format: `<member>: <text>` plus ` [<path joined by ", ">]` when path non-empty.
 *
 * Example: `activeRuns: ConcurrentDictionary<long, Task<…>> [value]`
 * where the text might be truncated for display.
 *
 * @param via The relation signature containing member, text, and path.
 * @param maxTextLength Optional maximum length for the type text before truncation.
 * @returns The formatted label, or empty string if insufficient data.
 */
export function deriveLabelFromVia(via: RelationVia | undefined, maxTextLength = 60): string {
  if (!via) return "";
  if (!via.member || !via.text) return "";

  let label = `${via.member}: ${via.text}`;

  // Truncate long type text reasonably
  if (label.length > maxTextLength) {
    const available = maxTextLength - via.member.length - 2; // 2 for ": "
    if (available > 10) {
      const truncated = via.text.substring(0, available - 3) + "…";
      label = `${via.member}: ${truncated}`;
    }
  }

  // Append path if present
  if (via.path && via.path.length > 0) {
    label += ` [${via.path.join(", ")}]`;
  }

  return label;
}

/**
 * Format cardinality at the target end of a code-origin relation.
 *
 * Maps cardinality from `via` to UML-style multiplicity labels.
 *
 * @param cardinality The cardinality value from `via`.
 * @returns The label for the target end (from `toLabel`), or empty if not applicable.
 */
export function cardinalityToLabel(cardinality: string | undefined): string {
  switch (cardinality) {
    case "many":
    case "keyed":
      return "0..*";
    case "optional":
      return "0..1";
    case "one":
    default:
      return "";
  }
}
