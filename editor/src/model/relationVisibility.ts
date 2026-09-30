import type { KindCatalog } from "./KindCatalog.js";

/**
 * Whether a registry relation is shown on a view nobody decided it for
 * (CONTRACT.md §8.5): the view's `except` flips the default, and the default is
 * the relation type's own `visibility` in the dictionary when it has one, else
 * the view's `relations.default`. A type without `visibility`, and a type
 * outside the dictionary, follow the view.
 */
export function relationShownByDefault(
  relation: { id: string; type?: string },
  policy: { default?: string; except?: readonly string[] } | undefined,
  kinds: KindCatalog,
): boolean {
  const own = relation.type === undefined ? undefined : kinds.relationVisibility(relation.type);
  const hidden = (own ?? policy?.default) === "hidden";
  return hidden === (policy?.except ?? []).includes(relation.id);
}
