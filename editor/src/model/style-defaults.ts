import type { WireStyle, WireStyleSheet } from "./style-types.js";

/**
 * The starting library — everything that used to be hardcoded, as data.
 *
 * Three tables fed the old renderer: `NODE_STYLES` keyed by node type,
 * `EDGE_STYLES` keyed by edge type, and `layout.Palette` in the Go generator,
 * which wrote colours *into* every zone of every model. All three are here now,
 * under ids that match the `type` values they used to key on, so a model opens
 * looking the same as before without carrying a single colour of its own.
 *
 * This is a fallback, not the source of truth. `styles.json` is — the one
 * shipped in `host/defaults/`, or the workspace's own override. When that file is
 * missing — a bare `@semaps/editor` embedded somewhere with no server — the
 * canvas falls back here instead of going grey.
 */

// --------------------------------------------------------------------- blocks

const BLOCK_DEFAULTS: WireStyle[] = [
  {
    id: "default.node",
    name: "Блок по умолчанию",
    appliesTo: "block",
    description: "Последняя инстанция для узла, тип которого не назвал стиль.",
    fill: "#ffffff",
    border: { color: "#cbd5e1", width: 1.5 },
    radius: 8,
    shadow: true,
    icon: { glyph: "📄" },
  },
  {
    id: "default.zone",
    name: "Зона по умолчанию",
    appliesTo: "block",
    description: "Последняя инстанция для контейнера.",
    fill: "#f8fafc",
    border: { color: "#cbd5e1", width: 2 },
    radius: 12,
    shadow: false,
    header: { fill: "#e2e8f0", height: 34 },
    icon: { show: false },
    // A zone's subtitle is its semanticId, drawn at the far end of the header
    // opposite the title. The renderer used to hardcode that; now that
    // alignment is a style field, the style has to say so or the two captions
    // land on top of each other.
    subtitle: { align: "end" },
  },
];

/**
 * One style per C# declaration kind.
 *
 * These ids are what `parser.NodeType` writes as a node's `type` — the whole
 * point of the change on the Go side. Before it, `class` and `record` both
 * arrived as "component" and were therefore indistinguishable no matter what
 * the styles said; only `enum` and `interface` had a look of their own, which
 * is exactly why those two read clearly and the other three hundred did not.
 */
const CODE_KIND_STYLES: WireStyle[] = [
  {
    id: "class",
    name: "Класс",
    appliesTo: "block",
    description: "Обычный класс: поведение, реализация.",
    fill: "#ffffff",
    border: { color: "#cbd5e1", width: 1.5 },
    radius: 8,
    shadow: true,
    icon: { glyph: "📦" },
  },
  {
    id: "record",
    name: "Record",
    appliesTo: "block",
    description: "Неизменяемые данные. Отличается от класса заливкой и иконкой.",
    basedOn: "class",
    fill: "#f0fdfa",
    border: { color: "#5eead4", width: 1.5 },
    // Data, not behaviour: rounder and flatter than a class on purpose, so the
    // difference survives a screenshot at 40% zoom where the icon is a smudge.
    radius: 14,
    shadow: false,
    icon: { glyph: "🧾" },
  },
  {
    id: "struct",
    name: "Struct",
    appliesTo: "block",
    description: "Значимый тип.",
    basedOn: "record",
    fill: "#f5f3ff",
    border: { color: "#c4b5fd" },
    radius: 4,
    icon: { glyph: "🧱" },
  },
  {
    id: "interface",
    name: "Интерфейс",
    appliesTo: "block",
    description: "Контракт. Пунктирная рамка — как в UML.",
    fill: "#ffffff",
    border: { color: "#60a5fa", width: 1.6, dash: "5,3" },
    radius: 8,
    shadow: true,
    icon: { glyph: "🔌" },
    title: { italic: true },
  },
  {
    id: "enum",
    name: "Enum",
    appliesTo: "block",
    description: "Перечисление.",
    fill: "#fefce8",
    border: { color: "#fde047", width: 1.6 },
    radius: 8,
    shadow: true,
    icon: { glyph: "🔢" },
  },
];

/**
 * The `type` values in the models as committed today.
 *
 * They inherit from the kind styles rather than repeating them, so the models
 * look right before regeneration and identical to the kind styles after it —
 * and when regeneration is done these three can be deleted in one edit.
 */
const LEGACY_TYPE_STYLES: WireStyle[] = [
  {
    id: "component",
    name: "Component (устар.)",
    appliesTo: "block",
    description: "Старый обобщённый тип: класс и record до перегенерации моделей.",
    basedOn: "class",
  },
  {
    id: "service",
    name: "Service (устар.)",
    appliesTo: "block",
    description: "Старый тип для интерфейсов.",
    basedOn: "interface",
    icon: { glyph: "⚙️" },
  },
  {
    id: "concept",
    name: "Concept (устар.)",
    appliesTo: "block",
    description: "Старый тип для enum и ручных концептов.",
    basedOn: "enum",
    icon: { glyph: "💡" },
  },
];

/** Hand-authored node types: things a human puts on a diagram directly. */
const AUTHORED_STYLES: WireStyle[] = [
  {
    id: "note",
    name: "Заметка",
    appliesTo: "block",
    fill: "#fef9c3",
    border: { color: "#fde047", width: 1.5 },
    radius: 6,
    shadow: false,
    icon: { glyph: "📝" },
    title: { weight: 500 },
  },
  {
    id: "security-component",
    name: "Безопасность",
    appliesTo: "block",
    fill: "#fff1f2",
    border: { color: "#fca5a5", width: 2 },
    icon: { glyph: "🛡️" },
  },
  {
    id: "tool",
    name: "Инструмент",
    appliesTo: "block",
    fill: "#ffffff",
    border: { color: "#bfdbfe", width: 1.5 },
    icon: { glyph: "🔧" },
  },
  {
    id: "database",
    name: "Хранилище",
    appliesTo: "block",
    fill: "#ffffff",
    border: { color: "#d8b4fe", width: 1.5 },
    icon: { glyph: "💾" },
  },
  {
    id: "external-system",
    name: "Внешняя система",
    appliesTo: "block",
    fill: "#fffbeb",
    border: { color: "#fde68a", width: 1.5 },
    icon: { glyph: "🌐" },
  },
];

/**
 * Zone themes, lifted verbatim from `layout.Palette` in the Go generator.
 *
 * Same colours, same names — so the generator can stop writing a five-field
 * `style` object onto all 46 zones and write `"styleId": "zone.green"` instead.
 */
const ZONE_THEMES: ReadonlyArray<readonly [string, string, string, string]> = [
  ["green", "#f0fdf4", "#86efac", "#dcfce7"],
  ["blue", "#eff6ff", "#93c5fd", "#dbeafe"],
  ["fuchsia", "#fdf4ff", "#f0abfc", "#fae8ff"],
  ["red", "#fff1f2", "#fca5a5", "#ffe4e6"],
  ["yellow", "#fefce8", "#fde047", "#fef9c3"],
  ["slate", "#f8fafc", "#cbd5e1", "#e2e8f0"],
  ["violet", "#f5f3ff", "#c4b5fd", "#ede9fe"],
  ["amber", "#fffbeb", "#fde68a", "#fef3c7"],
  ["sky", "#f0f9ff", "#7dd3fc", "#e0f2fe"],
  ["cyan", "#ecfeff", "#67e8f9", "#cffafe"],
  ["lime", "#f7fee7", "#bef264", "#ecfccb"],
  ["pink", "#fdf2f8", "#f9a8d4", "#fce7f3"],
  ["gray", "#f1f5f9", "#94a3b8", "#e2e8f0"],
];

const ZONE_STYLES: WireStyle[] = [
  ...ZONE_THEMES.map(([theme, fill, stroke, header]): WireStyle => ({
    id: `zone.${theme}`,
    name: `Зона · ${theme}`,
    appliesTo: "block",
    basedOn: "default.zone",
    fill,
    border: { color: stroke, width: 2 },
    header: { fill: header },
  })),
  {
    id: "zone.unplaced",
    name: "Зона · нераспределённое",
    appliesTo: "block",
    description: "Парковка для типов, которым не нашлось места. Намеренно кричащая.",
    basedOn: "default.zone",
    fill: "#fff7ed",
    border: { color: "#fb923c", width: 3, dash: "8 4" },
    header: { fill: "#ffedd5" },
  },
  {
    id: "boundary",
    name: "Граница (по типу зоны)",
    appliesTo: "block",
    description: "Стиль по умолчанию для зон типа boundary, если styleId не задан.",
    basedOn: "default.zone",
  },
  {
    id: "subsystem",
    name: "Подсистема",
    appliesTo: "block",
    basedOn: "zone.blue",
  },
];

// ---------------------------------------------------------------------- edges

/**
 * Structure recedes, flow speaks.
 *
 * In `model-core-full.json` 98 of 119 edges are `implements` or `extends`. At
 * the old weights that is a grey thicket with four coloured lines lost inside
 * it. These are deliberately thinner and paler than the flow styles: assembly
 * is background information, and what happens at runtime is the thing anyone
 * opens the diagram to see.
 */
const EDGE_STYLES: WireStyle[] = [
  {
    id: "default.edge",
    name: "Связь по умолчанию",
    appliesTo: "edge",
    line: { color: "#cbd5e1", width: 1.5 },
    target: { shape: "arrow", size: 6 },
  },

  {
    id: "implements",
    name: "Реализация интерфейса",
    appliesTo: "edge",
    description: "Класс реализует контракт. Самая частая связь — держим тихой.",
    family: "structure",
    line: { color: "#cbd5e1", width: 1.2, dash: "6,4" },
    target: { shape: "triangle-hollow", size: 9 },
  },
  {
    id: "realizes",
    name: "Реализация (realizes)",
    appliesTo: "edge",
    basedOn: "implements",
  },
  {
    id: "extends",
    name: "Наследование класса",
    appliesTo: "edge",
    family: "structure",
    line: { color: "#94a3b8", width: 1.4 },
    target: { shape: "triangle-hollow", size: 10 },
  },
  {
    id: "composes",
    name: "Владение / композиция",
    appliesTo: "edge",
    // The diamond belongs at the owner's end, not the part's — the old fixed
    // marker table could only put a head on `marker-end`, so composition was
    // drawn backwards everywhere it appeared.
    description: "Ромб на стороне владельца, как в UML.",
    family: "structure",
    line: { color: "#0f766e", width: 1.8 },
    source: { shape: "diamond", size: 12 },
    target: { shape: "none" },
  },

  {
    id: "call",
    name: "Вызов",
    appliesTo: "edge",
    family: "flow",
    line: { color: "#64748b", width: 1.6, dash: "4,4" },
    target: { shape: "arrow", size: 7 },
  },
  {
    id: "data-flow",
    name: "Поток данных",
    appliesTo: "edge",
    family: "flow",
    line: { color: "#3b82f6", width: 2.2 },
    target: { shape: "arrow", size: 8 },
    label: { color: "#1d4ed8", weight: 600 },
  },
  {
    id: "event",
    name: "Событие / уведомление",
    appliesTo: "edge",
    family: "flow",
    line: { color: "#ea580c", width: 2, dash: "2,3" },
    target: { shape: "arrow-open", size: 9 },
    label: { color: "#c2410c", weight: 600 },
  },
  {
    id: "security",
    name: "Проверка прав / аудит",
    appliesTo: "edge",
    family: "flow",
    line: { color: "#f43f5e", width: 1.8, dash: "3,3" },
    target: { shape: "arrow", size: 7 },
    label: { color: "#be123c", weight: 600 },
  },
  {
    id: "storage",
    name: "Запись / чтение хранилища",
    appliesTo: "edge",
    description: "Головки с обеих сторон: чтение и запись — одно ребро.",
    family: "flow",
    line: { color: "#a855f7", width: 2.2 },
    source: { shape: "circle-hollow", size: 7 },
    target: { shape: "arrow", size: 8 },
    label: { color: "#7e22ce", weight: 600 },
  },
];

export const BUILTIN_STYLES: readonly WireStyle[] = [
  ...BLOCK_DEFAULTS,
  ...CODE_KIND_STYLES,
  ...LEGACY_TYPE_STYLES,
  ...AUTHORED_STYLES,
  ...ZONE_STYLES,
  ...EDGE_STYLES,
];

export function builtinStyleSheet(): WireStyleSheet {
  return {
    version: 1,
    description:
      "Библиотека стилей SeMaps. Стиль применяется ко всем элементам, " +
      "чей type совпадает с его id, либо к тем, кто назвал его в styleId.",
    styles: structuredClone(BUILTIN_STYLES) as WireStyle[],
  };
}
