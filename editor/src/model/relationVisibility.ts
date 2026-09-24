import type { RelationTypeCatalog } from "./wire-types.js";

/**
 * Whether a registry relation is shown on a view nobody decided it for
 * (CONTRACT.md §8.5): the view's `except` flips the default, and the default is
 * the relation type's own `visibility` when it has one, else the view's
 * `relations.default`. A type without `visibility` follows the view.
 */
export function relationShownByDefault(
  relation: { id: string; type?: string },
  policy: { default?: string; except?: readonly string[] } | undefined,
  types: RelationTypeCatalog | undefined,
): boolean {
  const own = types?.relationTypes?.find((t) => t.id === relation.type)?.visibility;
  const hidden = (own ?? policy?.default) === "hidden";
  return hidden === (policy?.except ?? []).includes(relation.id);
}
