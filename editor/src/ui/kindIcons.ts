import { el } from "../util/dom.js";
import { FALLBACK_ICON, hasTabler, tablerSvg } from "./iconSet.js";

/**
 * The one registry of kind icons: a kind of symbol, of edge, of presence or of
 * visibility has the SAME Tabler icon wherever the app shows it — the node
 * list, filters, legend, Properties, menus, ribbon presets, extractor chips,
 * the editor's kind chips and the drawn blocks. Ask `kindIcon(group, key)`.
 *
 * `iconByKey` is for a *stored* icon — a block style's `icon.glyph`, a
 * project's or view's `icon`: the value is a Tabler name of the set
 * (`ui/iconSet.ts`, what the icon picker offers) or a kind key of any group
 * below. Anything else, an old emoji included, gets the one fallback icon;
 * there is no emoji rendering path.
 */
export type KindGroup = "symbol" | "edge" | "presence" | "visibility";

const registry: Record<KindGroup, Record<string, string>> = {
  symbol: {
    class: "square-letter-c",
    interface: "square-letter-i",
    struct: "square-letter-s",
    record: "square-letter-r",
    "record-struct": "square-rounded-letter-r",
    "record struct": "square-rounded-letter-r",
    enum: "square-letter-e",
    delegate: "square-letter-d",
    namespace: "brackets",
    assembly: "package",
    module: "stack-2",
    type: "box",
    function: "math-function",
    method: "function",
  },
  edge: {
    extends: "binary-tree",
    implements: "plug-connected",
    contains: "box-multiple",
    holds: "paperclip",
    uses: "arrow-right",
    injects: "needle",
    depends: "link",
    calls: "phone-call",
    constructs: "hammer",
    overrides: "replace",
  },
  presence: {
    code: "code",
    model: "box-model",
    both: "link",
  },
  visibility: {
    public: "world",
    protected: "shield-half",
    internal: "home",
    private: "lock",
  },
};

/** What the extractor may put in front of a base kind in `nativeKind` (docs/extractors/csharp.md). */
const KIND_MODIFIERS = new Set(["abstract", "static", "sealed", "partial", "readonly", "unsafe", "virtual"]);

/**
 * `nativeKind` is `<modifier>-<kind>` (`abstract-class`, `static-class`,
 * `sealed-record`…): the modifier is not a kind of its own, it is a look. This
 * splits it once into the base kind (`class`) and the modifiers (`abstract`).
 */
export function parseNativeKind(native: string | undefined): { base: string | undefined; modifiers: string[] } {
  if (!native) return { base: undefined, modifiers: [] };
  const parts = native.split("-");
  const modifiers: string[] = [];
  while (parts.length > 1 && KIND_MODIFIERS.has(parts[0]!)) modifiers.push(parts.shift()!);
  return { base: parts.join("-"), modifiers };
}

export function kindIcon(group: KindGroup, key: string | undefined): string {
  const k = group === "symbol" ? parseNativeKind(key).base : key;
  return tablerSvg(k ? registry[group][k.toLowerCase()] : undefined);
}

/** The kind's icon as an element sized to the surrounding text. */
export function kindIconEl(group: KindGroup, key: string | undefined, cls = ""): HTMLElement {
  const span = el("span", { class: cls ? `ui-icon ${cls}` : "ui-icon" });
  span.innerHTML = kindIcon(group, key);
  return span;
}

/** The keys a group knows, for a UI that lists them all. */
export function knownKinds(group: KindGroup): string[] {
  return Object.keys(registry[group]);
}

/** The icon of a stored value: a Tabler name of the set, else a kind key, else the fallback. */
export function iconByKey(value: string | undefined): string {
  if (!value) return tablerSvg(FALLBACK_ICON);
  if (hasTabler(value)) return tablerSvg(value);
  for (const group of Object.keys(registry) as KindGroup[]) {
    const name = registry[group][value.toLowerCase()];
    if (name) return tablerSvg(name);
  }
  return tablerSvg(FALLBACK_ICON);
}

/** `iconByKey` as an element sized to the surrounding text. */
export function iconElByKey(value: string | undefined, cls = ""): HTMLElement {
  const span = el("span", { class: cls ? `ui-icon ${cls}` : "ui-icon" });
  span.innerHTML = iconByKey(value);
  return span;
}
