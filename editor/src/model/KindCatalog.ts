/**
 * The dictionary of kinds, `kinds.json` as the host merges it (CONTRACT.md §6,
 * ADR_20260927-6, ADR_20260930-2): `GET /api/kinds`.
 *
 * Two sections with one shape. **Entity kinds** — what an entity's `kind`
 * means: name and description per language, the group it comes from, whether it
 * is a container (drawn as a frame that holds other placements) and its base
 * style. **Relation types** — the same for a relation's `type`.
 *
 * The host does the merging of the tool's default with the workspace file, so
 * the editor never reads `kinds.json` itself and never carries a copy of the
 * default.
 *
 * Not an enum: a kind or a relation type outside the catalog is legal (an
 * extractor brings the `nativeKind` of any language); `lookup` just has nothing
 * to say about it, and the editor marks it «не из словаря».
 */

export type LocalizedText = Readonly<Record<string, string>>;

export interface KindEntry {
  readonly id: string;
  readonly name: LocalizedText;
  readonly description?: LocalizedText;
  readonly container?: boolean;
  /** Base style; absent — the style whose id equals the kind (§11.5). */
  readonly style?: string;
}

export interface KindGroup {
  readonly id: string;
  readonly name: LocalizedText;
  readonly description?: LocalizedText;
  readonly kinds: readonly KindEntry[];
}

export interface RelationTypeDef {
  readonly id: string;
  readonly name: LocalizedText;
  readonly description?: LocalizedText;
  /** Base style; absent — the style whose id equals the type (§11.5). */
  readonly style?: string;
}

export interface RelationGroup {
  readonly id: string;
  readonly name: LocalizedText;
  readonly description?: LocalizedText;
  readonly types: readonly RelationTypeDef[];
}

/** The answer of `GET /api/kinds` without `lang`: every text in all its languages. */
export interface KindsDocument {
  readonly groups?: readonly KindGroup[];
  readonly relationGroups?: readonly RelationGroup[];
}

function localized(text: LocalizedText | undefined, lang: string): string | undefined {
  if (text === undefined) return undefined;
  return text[lang] ?? Object.values(text)[0];
}

export class KindCatalog {
  private readonly byKind = new Map<string, { kind: KindEntry; group: KindGroup }>();
  private readonly byRelation = new Map<string, { type: RelationTypeDef; group: RelationGroup }>();

  private constructor(
    private readonly kindGroups: readonly KindGroup[],
    private readonly relGroups: readonly RelationGroup[],
  ) {
    for (const group of kindGroups) for (const kind of group.kinds) this.byKind.set(kind.id, { kind, group });
    for (const group of relGroups) for (const type of group.types) this.byRelation.set(type.id, { type, group });
  }

  /** Nothing known: every kind is «не из словаря» until the host's dictionary arrives. */
  static empty(): KindCatalog {
    return new KindCatalog([], []);
  }

  static parse(doc: KindsDocument | null | undefined): KindCatalog {
    const groups = (doc?.groups ?? [])
      .filter((g) => typeof g?.id === "string" && Array.isArray(g.kinds))
      .map((g) => ({ ...g, name: g.name ?? { en: g.id }, kinds: g.kinds.filter((k) => typeof k?.id === "string" && k.id !== "") }));
    const relationGroups = (doc?.relationGroups ?? [])
      .filter((g) => typeof g?.id === "string" && Array.isArray(g.types))
      .map((g) => ({ ...g, name: g.name ?? { en: g.id }, types: g.types.filter((t) => typeof t?.id === "string" && t.id !== "") }));
    return new KindCatalog(groups, relationGroups);
  }

  /** The catalog in force; replaced once the host's dictionary has been read. */
  static active: KindCatalog = KindCatalog.empty();

  // ------------------------------------------------------------ entity kinds

  groups(): readonly KindGroup[] {
    return this.kindGroups;
  }

  lookup(kind: string): KindEntry | undefined {
    return this.byKind.get(kind)?.kind;
  }

  groupOf(kind: string): KindGroup | undefined {
    return this.byKind.get(kind)?.group;
  }

  isContainer(kind: string): boolean {
    return this.byKind.get(kind)?.kind.container === true;
  }

  /** The kind's name in `lang`, else in any language; the id when it is not in the catalog. */
  name(kind: string, lang: string): string {
    return localized(this.lookup(kind)?.name, lang) ?? kind;
  }

  description(kind: string, lang: string): string {
    return localized(this.lookup(kind)?.description, lang) ?? "";
  }

  /** The base style the catalog names for the kind, if it names one. */
  baseStyle(kind: string): string | undefined {
    return this.lookup(kind)?.style;
  }

  // ---------------------------------------------------------- relation types

  relationGroups(): readonly RelationGroup[] {
    return this.relGroups;
  }

  lookupRelation(type: string): RelationTypeDef | undefined {
    return this.byRelation.get(type)?.type;
  }

  relationGroupOf(type: string): RelationGroup | undefined {
    return this.byRelation.get(type)?.group;
  }

  relationName(type: string, lang: string): string {
    return localized(this.lookupRelation(type)?.name, lang) ?? type;
  }

  relationDescription(type: string, lang: string): string {
    return localized(this.lookupRelation(type)?.description, lang) ?? "";
  }

  relationBaseStyle(type: string): string | undefined {
    return this.lookupRelation(type)?.style;
  }

  // ------------------------------------------------------------------- texts

  groupName(group: { readonly id: string; readonly name: LocalizedText }, lang: string): string {
    return localized(group.name, lang) ?? group.id;
  }

  groupDescription(group: { readonly description?: LocalizedText }, lang: string): string {
    return localized(group.description, lang) ?? "";
  }
}

/** Read the host's merged dictionary. A failure is thrown: the caller says so and goes on with an empty one. */
export async function loadKindCatalog(): Promise<KindCatalog> {
  const res = await fetch("/api/kinds", { cache: "no-store" });
  if (!res.ok) throw new Error(`GET /api/kinds: HTTP ${res.status}`);
  return KindCatalog.parse((await res.json()) as KindsDocument);
}
