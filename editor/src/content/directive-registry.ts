/**
 * Directives are named and registered, not hardcoded into the parser — the
 * ADR's whole bet is that a new way to show something is a registry entry
 * plus a renderer, never a parser change. This file only declares what a
 * directive's positional argument looks like; the rendering side (which
 * reads `where=`, draws `@Asset`'s picture, …) is a separate stream and does
 * not live here.
 */

/**
 * What shape the single positional argument takes, if any.
 *
 * This is the only thing the parser needs to know about a directive: it
 * decides whether a bare word (`brain`) is that directive's "what to show"
 * value or just a flag (`collapsed`), and whether a bracketed list is legal.
 * See the disambiguation note in `parser.ts` for why this matters —
 * `@Members(collapsed)` and `@Asset(brain, …)` look identical at the token
 * level and only the signature tells them apart.
 */
export type PositionalShape =
  /** No positional argument at all — a bare word or list here is an error. */
  | "none"
  /** A single bare word, e.g. `@Asset(brain, …)`. */
  | "value"
  /** A bracketed list only, e.g. `@Members([name, type], …)`. */
  | "list"
  /** Either form is accepted. */
  | "either";

export interface DirectiveSignature {
  /** Name as it appears after `@`. Matched case-sensitively. */
  name: string;
  /** Shown in a directive picker; Russian, matching the rest of the UI. */
  description: string;
  positional: {
    shape: PositionalShape;
    required: boolean;
  };
}

/**
 * Known directives, keyed by name. A plain map, not a class with hidden
 * state, because the only operations are "is this name known" and "list them
 * for a picker" — nothing here needs encapsulating.
 */
export class DirectiveRegistry {
  private readonly byName = new Map<string, DirectiveSignature>();

  register(signature: DirectiveSignature): void {
    this.byName.set(signature.name, signature);
  }

  get(name: string): DirectiveSignature | undefined {
    return this.byName.get(name);
  }

  has(name: string): boolean {
    return this.byName.has(name);
  }

  list(): DirectiveSignature[] {
    return [...this.byName.values()];
  }
}

/**
 * The first-wave directives from ADR_20260903 §2.3. Rendering for these does
 * not exist yet (stream D) — this only reserves their names and argument
 * shapes so the compiler can validate against them today.
 */
export function createDefaultDirectiveRegistry(): DirectiveRegistry {
  const registry = new DirectiveRegistry();

  registry.register({
    name: "Name",
    description: "Каноническое имя сущности",
    positional: { shape: "none", required: false },
  });

  registry.register({
    name: "Description",
    description: "Короткое описание из текстового каталога",
    positional: { shape: "none", required: false },
  });

  registry.register({
    name: "Members",
    description: "Члены типа (поля, методы, колонки) с фильтром where=",
    positional: { shape: "list", required: false },
  });

  registry.register({
    name: "Asset",
    description: "Именованный ресурс из реестра содержимого",
    positional: { shape: "value", required: true },
  });

  return registry;
}
