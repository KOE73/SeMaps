/**
 * Making a foreign SVG safe to put inside the editor's own window.
 *
 * Content assets are named pictures pulled from a shared registry and drawn
 * *inside* a block, in the same document as the canvas and the panels. An SVG
 * is not an image in that setting — it is markup, and markup can carry script,
 * event handlers and outbound references. A picture someone dropped into the
 * registry must not be able to read the page it decorates.
 *
 * The rule here is a whitelist, not a blacklist: anything this file does not
 * name is dropped. A blacklist of known-bad tags fails the day a new one is
 * invented, and the failure is silent — exactly the shape of bug nobody finds
 * by looking at a diagram.
 */

/** Elements allowed to survive. Everything else is dropped with its subtree. */
const ALLOWED_ELEMENTS = new Set([
  "svg", "g", "defs", "symbol", "use", "title", "desc",
  "path", "rect", "circle", "ellipse", "line", "polyline", "polygon",
  "text", "tspan", "textPath",
  "linearGradient", "radialGradient", "stop",
  "clipPath", "mask", "marker", "pattern",
]);

/**
 * Attributes allowed to survive, on any element.
 *
 * Presentation attributes only. Notably absent: every `on*` handler, `href`
 * and `xlink:href` in their general form (see below), and anything that names
 * a script or a stylesheet.
 */
const ALLOWED_ATTRIBUTES = new Set([
  "id", "class", "transform", "viewBox", "xmlns", "version",
  "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry",
  "width", "height", "d", "points", "offset", "dx", "dy",
  "fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity",
  "stroke-linecap", "stroke-linejoin", "stroke-dasharray", "stroke-dashoffset",
  "opacity", "color", "stop-color", "stop-opacity",
  "font-family", "font-size", "font-weight", "font-style", "letter-spacing",
  "text-anchor", "dominant-baseline", "alignment-baseline", "white-space",
  "gradientUnits", "gradientTransform", "spreadMethod",
  "clip-path", "clip-rule", "mask", "marker-start", "marker-mid", "marker-end",
  "patternUnits", "patternContentUnits", "preserveAspectRatio",
  "vector-effect", "paint-order", "shape-rendering",
]);

/**
 * `url(...)` in an attribute value is only safe when it points inside this same
 * document. `url(#glow)` is a local gradient; anything else reaches out.
 */
const SAFE_LOCAL_URL = /^url\(\s*['"]?#[^)'"]+['"]?\s*\)$/;

/** What a sanitising pass did, so a caller can say why a picture looks wrong. */
export interface SanitizeReport {
  /** The cleaned markup, ready to insert. Empty when nothing survived. */
  readonly svg: string;
  /** Element and attribute names that were dropped, each named once. */
  readonly removed: readonly string[];
  /** Set when the input could not be parsed as SVG at all. */
  readonly error?: string;
}

/**
 * Clean one SVG document.
 *
 * Returns markup and a list of what was taken out rather than throwing: a bad
 * asset should show up as a picture with a complaint next to it, not as a
 * broken editor. The caller decides how loudly to report.
 */
export function sanitizeSvg(source: string): SanitizeReport {
  const removed = new Set<string>();

  // A parser error inside an SVG document is reported as a <parsererror>
  // element rather than an exception, so it has to be looked for explicitly.
  const parsed = new DOMParser().parseFromString(source, "image/svg+xml");
  const failure = parsed.querySelector("parsererror");
  if (failure !== null) {
    return { svg: "", removed: [], error: "не разбирается как SVG" };
  }

  const root = parsed.documentElement;
  if (root === null || root.localName !== "svg") {
    return { svg: "", removed: [], error: "корневой элемент не <svg>" };
  }

  scrub(root, removed);

  // Sizing belongs to the template (`@Asset(brain, h=300)`), not to the file:
  // an asset that keeps its own width fights the block it was placed in. The
  // viewBox stays, because that is what makes scaling meaningful at all.
  root.removeAttribute("width");
  root.removeAttribute("height");

  return {
    svg: new XMLSerializer().serializeToString(root),
    removed: [...removed].sort(),
  };
}

/** Depth-first pass; children are walked from a copy, since the list shrinks. */
function scrub(el: Element, removed: Set<string>): void {
  for (const child of [...el.children]) {
    if (!ALLOWED_ELEMENTS.has(child.localName)) {
      removed.add(`<${child.localName}>`);
      child.remove();
      continue;
    }
    scrub(child, removed);
  }

  for (const attr of [...el.attributes]) {
    if (!isAttributeSafe(el, attr)) {
      removed.add(attr.name);
      el.removeAttributeNode(attr);
    }
  }
}

function isAttributeSafe(el: Element, attr: Attr): boolean {
  const name = attr.name.toLowerCase();

  // Handlers first: `onload` on a bare <g> is the whole reason this file exists.
  if (name.startsWith("on")) return false;

  // `href` survives only as a local reference, and only where it means one:
  // <use href="#icon"> is how a symbol is instantiated, while the same
  // attribute pointing outward is a fetch the viewer never asked for.
  if (name === "href" || name === "xlink:href") {
    return (el.localName === "use" || el.localName === "textPath")
      && attr.value.startsWith("#");
  }

  // `style` is allowed for plain declarations but must not reach outside the
  // document, which `url(...)` is the only way to do from CSS here.
  if (name === "style") {
    return !/url\s*\(/i.test(attr.value) && !/@import/i.test(attr.value);
  }

  if (!ALLOWED_ATTRIBUTES.has(attr.name)) return false;

  // A whitelisted attribute can still carry a reference: `fill="url(http://…)"`.
  if (/url\s*\(/i.test(attr.value)) return SAFE_LOCAL_URL.test(attr.value.trim());

  return true;
}
