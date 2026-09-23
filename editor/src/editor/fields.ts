import { el } from "../util/dom.js";

/**
 * Form controls shared by the inspector and the style editor.
 *
 * They exist as one module because the two panels edit the same kinds of value
 * with different meanings: the inspector edits a model field, where empty is a
 * value, and the style editor edits a sparse `WireStyle`, where empty means
 * "inherit". Everything below comes in both flavours, and the `optional*`
 * family is the one that hands back `undefined` instead of `""` — without it a
 * cleared field would shadow an inherited value with nothing, which reads as a
 * missing colour rather than an inherited one.
 */

export type Option = readonly [value: string, label: string];

export function select(
  options: readonly Option[],
  selected: string,
  onChange: (value: string) => void,
): HTMLSelectElement {
  return el(
    "select",
    { on: { change: (e) => onChange((e.target as HTMLSelectElement).value) } },
    options.map(([value, label]) => {
      const option = el("option", { value, text: label });
      if (value === selected) option.selected = true;
      return option;
    }),
  );
}

/** A labelled row. `label` is a caption, never markup (D-03). */
export function field(label: string, ...controls: readonly (Node | null)[]): HTMLElement {
  return el("label", { class: "field" }, [
    el("span", { class: "field-label", text: label }),
    ...controls,
  ]);
}

/**
 * Colour: a swatch and the authoritative text next to it.
 *
 * The text field is what the model gets, because `input[type=color]` can only
 * hold a six-digit hex and would quietly destroy a named colour, an `rgb()` or
 * a CSS variable the moment it was focused. The swatch is a picker for it, and
 * shows `placeholder` when the field is empty so an inherited colour is still
 * visible rather than showing as grey.
 */
export function colorField(
  label: string,
  value: string,
  placeholder: string,
  onChange: (value: string) => void,
): HTMLElement {
  const text = el("input", {
    class: "mono",
    type: "text",
    value,
    placeholder,
    on: {
      input: (e) => {
        const next = (e.target as HTMLInputElement).value;
        swatch.value = toSwatchValue(next === "" ? placeholder : next);
        onChange(next);
      },
    },
  });

  const swatch = el("input", {
    class: "color-swatch",
    type: "color",
    title: "Выбрать цвет",
    value: toSwatchValue(value === "" ? placeholder : value),
    on: {
      input: (e) => {
        const next = (e.target as HTMLInputElement).value;
        text.value = next;
        onChange(next);
      },
    },
  });

  return el("label", { class: "field" }, [
    el("span", { class: "field-label", text: label }),
    el("div", { class: "color-field-row" }, [swatch, text]),
  ]);
}

/** Colour where clearing the field means "inherit". */
export function optionalColorField(
  label: string,
  value: string | undefined,
  inherited: string,
  onChange: (value: string | undefined) => void,
): HTMLElement {
  return colorField(label, value ?? "", inherited, (next) =>
    onChange(next.trim() === "" ? undefined : next),
  );
}

/** Free text where clearing the field means "inherit". */
export function optionalTextField(
  label: string,
  value: string | undefined,
  inherited: string,
  onChange: (value: string | undefined) => void,
): HTMLElement {
  return field(
    label,
    el("input", {
      type: "text",
      value: value ?? "",
      placeholder: inherited,
      on: {
        input: (e) => {
          const next = (e.target as HTMLInputElement).value;
          onChange(next.trim() === "" ? undefined : next);
        },
      },
    }),
  );
}

export interface NumberBounds {
  min?: number;
  max?: number;
  step?: number;
}

/** A number where clearing the field means "inherit". */
export function optionalNumberField(
  label: string,
  value: number | undefined,
  inherited: number,
  onChange: (value: number | undefined) => void,
  bounds: NumberBounds = {},
): HTMLElement {
  const attrs: Record<string, string> = {};
  if (bounds.min !== undefined) attrs.min = String(bounds.min);
  if (bounds.max !== undefined) attrs.max = String(bounds.max);
  attrs.step = String(bounds.step ?? 1);

  return field(
    label,
    el("input", {
      type: "number",
      value: value === undefined ? "" : String(value),
      placeholder: String(inherited),
      attrs,
      on: {
        input: (e) => {
          const raw = (e.target as HTMLInputElement).value.trim();
          const parsed = Number(raw);
          // A half-typed "-" or "1e" parses to NaN; treated as "not yet a
          // value" rather than written into the style as a broken number.
          onChange(raw === "" || !Number.isFinite(parsed) ? undefined : parsed);
        },
      },
    }),
  );
}

/**
 * A boolean with three states, not two.
 *
 * A checkbox cannot say "inherit" — it is on or off, and whichever way it
 * lands the style would gain an explicit value it never asked for, silently
 * detaching the field from its `basedOn` parent. So booleans in a sparse style
 * are a three-way select, with the inherited answer spelled out in the empty
 * option.
 */
export function optionalBoolField(
  label: string,
  value: boolean | undefined,
  inherited: boolean,
  onChange: (value: boolean | undefined) => void,
): HTMLElement {
  const current = value === undefined ? "" : value ? "yes" : "no";
  return field(
    label,
    select(
      [
        ["", `— как в основе (${inherited ? "да" : "нет"}) —`],
        ["yes", "да"],
        ["no", "нет"],
      ],
      current,
      (next) => onChange(next === "" ? undefined : next === "yes"),
    ),
  );
}

/** A choice from a fixed set where the empty option means "inherit". */
export function optionalSelectField<T extends string>(
  label: string,
  value: T | undefined,
  options: readonly Option[],
  inheritedLabel: string,
  onChange: (value: T | undefined) => void,
): HTMLElement {
  return field(
    label,
    select([["", `— как в основе (${inheritedLabel}) —`], ...options], value ?? "", (next) =>
      onChange(next === "" ? undefined : (next as T)),
    ),
  );
}

/**
 * `<input type="color">` only ever holds a 6-digit hex, and rejects anything
 * else outright rather than falling back — so a named color, an rgb(), or an
 * empty "use the theme default" value all need translating into something it
 * can display. The canvas resolves the real color via CSS, so this normalized
 * value only has to be visually close enough for the swatch to be useful as a
 * picker; the authoritative value stays in the text field next to it.
 */
export function toSwatchValue(value: string): string {
  const trimmed = value.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) return trimmed;
  if (/^#[0-9a-fA-F]{3}$/.test(trimmed)) {
    const [, r, g, b] = trimmed;
    return `#${r}${r}${g}${g}${b}${b}`;
  }
  if (trimmed === "") return "#cbd5e1";

  const probe = document.createElement("span");
  probe.style.color = "";
  probe.style.color = trimmed;
  if (probe.style.color === "") return "#cbd5e1";
  probe.style.display = "none";
  document.body.appendChild(probe);
  const rgb = getComputedStyle(probe).color;
  document.body.removeChild(probe);
  const match = rgb.match(/\d+/g);
  if (match === null || match.length < 3) return "#cbd5e1";
  const [r, g, b] = match.map((n) => Number(n).toString(16).padStart(2, "0"));
  return `#${r}${g}${b}`;
}
