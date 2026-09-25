type Child = Node | string | null | undefined | false;

export interface ElementOptions {
  class?: string;
  text?: string;
  title?: string;
  id?: string;
  type?: string;
  value?: string;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
  hidden?: boolean;
  dataset?: Record<string, string>;
  attrs?: Record<string, string>;
  on?: Partial<{ [K in keyof HTMLElementEventMap]: (e: HTMLElementEventMap[K]) => void }>;
}

/**
 * Build an element. Text always goes through `textContent`, never through
 * markup, so a model value can never be interpreted as HTML (D-03).
 */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  options: ElementOptions = {},
  children: readonly Child[] = [],
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);

  if (options.class !== undefined) node.className = options.class;
  if (options.id !== undefined) node.id = options.id;
  if (options.title !== undefined) node.title = options.title;
  if (options.text !== undefined) node.textContent = options.text;
  if (options.hidden === true) node.hidden = true;

  if (options.type !== undefined && "type" in node) {
    (node as HTMLInputElement).type = options.type;
  }
  if (options.value !== undefined && "value" in node) {
    (node as HTMLInputElement).value = options.value;
  }
  if (options.placeholder !== undefined && "placeholder" in node) {
    (node as HTMLInputElement).placeholder = options.placeholder;
  }
  if (options.rows !== undefined && "rows" in node) {
    (node as unknown as HTMLTextAreaElement).rows = options.rows;
  }
  if (options.disabled === true && "disabled" in node) {
    (node as HTMLButtonElement).disabled = true;
  }

  for (const [key, value] of Object.entries(options.dataset ?? {})) {
    node.dataset[key] = value;
  }
  for (const [key, value] of Object.entries(options.attrs ?? {})) {
    node.setAttribute(key, value);
  }
  for (const [event, handler] of Object.entries(options.on ?? {})) {
    node.addEventListener(event, handler as EventListener);
  }

  for (const child of children) {
    if (child === null || child === undefined || child === false) continue;
    node.appendChild(typeof child === "string" ? document.createTextNode(child) : child);
  }
  return node;
}

export function replaceChildren(host: Element, ...children: readonly Child[]): void {
  host.replaceChildren();
  for (const child of children) {
    if (child === null || child === undefined || child === false) continue;
    host.appendChild(typeof child === "string" ? document.createTextNode(child) : child);
  }
}

export function requireElement<T extends Element = HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (found === null) throw new Error(`Missing element #${id}`);
  return found as unknown as T;
}
