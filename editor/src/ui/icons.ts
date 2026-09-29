import { el } from "../util/dom.js";
import { tablerSvg } from "./iconSet.js";

/**
 * The app's UI icons by semantic key: Tabler outline SVGs (`stroke="currentColor"`,
 * so they follow the theme and the active state). A command's `icon`, a ribbon
 * item's icon and every `iconEl(...)` take one of these; Tabler names are
 * resolved only in `iconSet.ts`. There is no emoji in the UI — not even the
 * icon of a project, view or block style, which is a registry key too
 * (`kindIcons.ts`, `iconByKey`).
 *
 * These strings end up in the library bundle too, because the workbench (its
 * commands, panels and menus) is part of it.
 */
const names = {
  alert: "alert-triangle",
  anchor: "anchor",
  arrowDown: "arrow-down",
  arrowLeft: "arrow-left",
  arrowRight: "arrow-right",
  arrowUp: "arrow-up",
  shuffle: "arrows-shuffle",
  books: "books",
  boxModel: "box-model",
  box: "box",
  bank: "building-bank",
  bulb: "bulb",
  category: "category",
  check: "check",
  checks: "checks",
  circleCheck: "circle-check",
  circleDot: "circle-dot",
  clipboard: "clipboard",
  code: "code",
  compass: "compass",
  copy: "copy",
  database: "database",
  save: "device-floppy",
  download: "download",
  eyeOff: "eye-off",
  doc: "file-text",
  filter: "filter",
  folder: "folder",
  folders: "folders",
  ghost: "ghost",
  help: "help-circle",
  hierarchy: "hierarchy",
  hierarchy2: "hierarchy-2",
  hourglass: "hourglass",
  language: "language",
  layoutBoard: "layout-board",
  link: "link",
  listDetails: "list-details",
  magnet: "magnet",
  map: "map",
  maximize: "maximize",
  minus: "minus",
  notes: "notes",
  palette: "palette",
  pencil: "pencil",
  pin: "pin",
  plug: "plug",
  plus: "plus",
  pointer: "pointer",
  quote: "quote",
  refresh: "refresh",
  restore: "restore",
  robot: "robot",
  search: "search",
  settings: "settings",
  sparkles: "sparkles",
  squareOff: "square-off",
  sun: "sun",
  table: "table",
  trash: "trash",
  world: "world",
  close: "x",
  zoomIn: "zoom-in",
  zoomOut: "zoom-out",
  fileExport: "file-export",
  fileCode: "file-code",
  folderOpen: "folder-open",
  waveSine: "wave-sine",
  gridDots: "grid-dots",
  layoutList: "layout-list",
  deviceDesktopCode: "device-desktop-code",
  arrowBarToLeft: "arrow-bar-to-left",
  arrowBarToRight: "arrow-bar-to-right",
  arrowBarToUp: "arrow-bar-to-up",
  arrowBarToDown: "arrow-bar-to-down",
  arrowBarLeft: "arrow-bar-left",
  arrowBarRight: "arrow-bar-right",
  arrowBarUp: "arrow-bar-up",
  arrowBarDown: "arrow-bar-down",
  arrowsHorizontal: "arrows-horizontal",
  arrowsVertical: "arrows-vertical",
  arrowBackUp: "arrow-back-up",
  inbox: "inbox",
  moon: "moon",
  edit: "edit",
  eye: "eye",
  sunglasses: "sunglasses",
  focus2: "focus-2",
  playlistAdd: "playlist-add",
  playlistX: "playlist-x",
  listSearch: "list-search",
  listCheck: "list-check",
  arrowsDiff: "arrows-diff",
  arrowForwardUp: "arrow-forward-up",
  loader2: "loader-2",
  clock: "clock",
  playerPlay: "player-play",
  route: "route",
  circleX: "circle-x",
  cpu: "cpu",
  chevronRight: "chevron-right",
  dots: "dots",
  chevronDown: "chevron-down",
  cornerRightDown: "corner-right-down",
  template: "template",
} as const;

export const icons = Object.fromEntries(Object.entries(names).map(([key, name]) => [key, tablerSvg(name)])) as { [K in keyof typeof names]: string };

export type IconName = keyof typeof icons;

/** The icon as an inline `<svg>` string (a command's `icon`, an `innerHTML` template). */
export function iconSvg(name: IconName): string {
  return icons[name];
}

/** The icon as an element sized to the surrounding text (1.15em). */
export function iconEl(name: IconName, cls = ""): HTMLElement {
  const span = el("span", { class: cls ? `ui-icon ${cls}` : "ui-icon" });
  span.innerHTML = icons[name];
  return span;
}
