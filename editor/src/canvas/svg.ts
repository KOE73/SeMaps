const NS = "http://www.w3.org/2000/svg";

type AttrValue = string | number | undefined | null;

export function svg<K extends keyof SVGElementTagNameMap>(
  tag: K,
  attrs: Record<string, AttrValue> = {},
  children: readonly (SVGElement | null)[] = [],
): SVGElementTagNameMap[K] {
  const el = document.createElementNS(NS, tag);
  setAttrs(el, attrs);
  for (const child of children) {
    if (child !== null) el.appendChild(child);
  }
  return el;
}

export function setAttrs(el: Element, attrs: Record<string, AttrValue>): void {
  for (const [name, value] of Object.entries(attrs)) {
    if (value === undefined || value === null) el.removeAttribute(name);
    else el.setAttribute(name, String(value));
  }
}

/**
 * Set text content directly rather than through markup.
 *
 * Every caption on the canvas comes from a model that may be hand-edited or
 * generated, so it is never trusted as markup (D-03).
 */
export function text(
  attrs: Record<string, AttrValue>,
  content: string,
): SVGTextElement {
  const el = svg("text", attrs);
  el.textContent = content;
  return el;
}

export function clear(el: Element): void {
  while (el.firstChild !== null) el.removeChild(el.firstChild);
}
