/**
 * The name of an entity, as the editor shows it.
 *
 * An entity from code (or without an origin) keeps its `name` in
 * `entities.json`, untranslated and not editable here. The name of an
 * `authored` entity — a container drawn by hand, a component, an actor — is a
 * text: the translatable field `name` under its id in `text.<lang>.json`. So a
 * name is looked up in the current text language, then in any other language
 * (the project's `languages` order first, then the rest by code), and when no
 * language has one, the id stands in for it.
 *
 * This is the one place that rule is written down: everything that shows an
 * entity's name asks here.
 */

/** What is read of one language's texts: the value of `name` is a string or a `{v}` provenance value. */
export type NameTexts = Readonly<Record<string, {
  readonly entries?: Readonly<Record<string, { readonly name?: unknown } | undefined>>;
} | undefined>>;

/** The part of an entity record a name depends on. */
export interface NamedEntity {
  readonly id: string;
  readonly name?: string | undefined;
  readonly origin?: string | undefined;
}

/**
 * Whether this entity's name is a text — and so editable in the editor and
 * written as a text op. True for an authored entity and for one not yet in the
 * registry (just drawn); false for one from code or without an origin, whose
 * name is the registry's and is not translated.
 */
export function nameIsText(entity: NamedEntity | undefined): boolean {
  return entity === undefined || entity.origin === "authored";
}

function textValue(value: unknown): string {
  if (typeof value === "string") return value.trim() === "" ? "" : value;
  const v = (value as { v?: unknown } | null | undefined)?.v;
  return typeof v === "string" && v.trim() !== "" ? v : "";
}

/**
 * The language a text is written in: `lang` when the project has it, else the
 * project's first language. (The editor's data language is a per-viewer choice
 * and may name a language this project does not have.)
 */
export function textLanguageOf(languages: readonly string[], lang: string): string {
  return languages.length === 0 || languages.includes(lang) ? lang : languages[0]!;
}

/**
 * The display name of the entity `id` in text language `lang`.
 *
 * @param entity The registry record, or undefined for an entity not (yet) in the registry — treated as authored.
 * @param texts The text catalogues by language.
 * @param languages The project's languages in order.
 */
export function entityDisplayName(
  entity: NamedEntity | undefined,
  id: string,
  texts: NameTexts | undefined,
  lang: string,
  languages: readonly string[] = [],
): string {
  const own = typeof entity?.name === "string" ? entity.name.trim() : "";
  if (entity !== undefined && entity.origin !== "authored" && own !== "") return entity.name as string;

  const order = new Set<string>([lang, ...languages, ...Object.keys(texts ?? {}).sort()]);
  for (const l of order) {
    const name = textValue(texts?.[l]?.entries?.[id]?.name);
    if (name !== "") return name;
  }
  return own !== "" ? (entity!.name as string) : id;
}
