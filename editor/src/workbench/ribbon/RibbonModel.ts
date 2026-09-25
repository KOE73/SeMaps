import type { RibbonSpec, RibbonTabSpec } from "./types.js";
import type { CommandContext } from "../commands/types.js";
import { i18n } from "../i18n/I18nService.js";

export function createDefaultRibbonSpec(): RibbonSpec {
  return {
    tabs: [
      // ----------------------------------------------------------- Главная
      {
        id: "home",
        get title() { return i18n.d.ribbon.tabs.home; },
        keyTip: "H",
        groups: [
          {
            id: "create",
            get title() { return i18n.d.ribbon.groups.creation; },
            items: [
              { type: "button", command: "diagram.block.add", size: "large" },
              { type: "button", command: "diagram.zone.add", size: "large" },
            ],
          },
          {
            id: "edit",
            get title() { return i18n.d.ribbon.groups.edit; },
            items: [
              { type: "button", command: "edit.delete", size: "medium" },
            ],
          },
          {
            id: "align",
            get title() { return i18n.d.ribbon.groups.align; },
            items: [
              { type: "button", command: "diagram.align.left", size: "small" },
              { type: "button", command: "diagram.align.right", size: "small" },
              { type: "button", command: "diagram.align.top", size: "small" },
              { type: "button", command: "diagram.align.bottom", size: "small" },
              { type: "button", command: "diagram.align.width", size: "small" },
              { type: "button", command: "diagram.align.height", size: "small" },
              { type: "button", command: "diagram.alignEdge.left", size: "small" },
              { type: "button", command: "diagram.alignEdge.right", size: "small" },
              { type: "button", command: "diagram.alignEdge.top", size: "small" },
              { type: "button", command: "diagram.alignEdge.bottom", size: "small" },
            ],
          },
          {
            id: "file",
            get title() { return i18n.d.ribbon.groups.file; },
            items: [
              { type: "button", command: "file.save", size: "large" },
              { type: "button", command: "file.export.drawio", size: "medium" },
            ],
          },
        ],
      },

      // ----------------------------------------------------------- Вставка
      {
        id: "insert",
        get title() { return i18n.d.ribbon.tabs.insert; },
        keyTip: "N",
        groups: [
          {
            id: "elements",
            get title() { return i18n.d.ribbon.groups.creation; },
            items: [
              { type: "button", command: "diagram.block.add", size: "large" },
              { type: "button", command: "diagram.zone.add", size: "large" },
            ],
          },
          {
            id: "workspace",
            get title() { return i18n.d.ribbon.groups.workspace; },
            items: [
              { type: "button", command: "workspace.project.new", size: "large" },
              { type: "button", command: "workspace.view.new", size: "large" },
              { type: "button", command: "workspace.view.edit", size: "medium" },
              { type: "button", command: "workspace.project.edit", size: "medium" },
            ],
          },
          {
            id: "catalogs",
            get title() { return i18n.d.ribbon.groups.panels; },
            items: [
              { type: "button", command: "panel.base.toggle", size: "large" },
              { type: "button", command: "panel.neighbourhood.toggle", size: "large" },
              { type: "button", command: "panel.catalog.toggle", size: "large" },
            ],
          },
        ],
      },

      // --------------------------------------------------------- Диаграмма
      {
        id: "diagram",
        get title() { return i18n.d.ribbon.tabs.diagram; },
        keyTip: "D",
        groups: [
          {
            id: "ports",
            get title() { return i18n.d.ribbon.groups.edgeFamilies; },
            items: [
              {
                type: "select",
                command: "diagram.ports.set",
                get label() { return i18n.d.ribbon.labels.ports; },
                get options() {
                  return [
                    { value: "uniform", label: i18n.d.ribbon.labels.portsUniform },
                    { value: "discrete", label: i18n.d.ribbon.labels.portsDiscrete },
                    { value: "center", label: i18n.d.ribbon.labels.portsCenter },
                  ];
                },
                getValue: () => localStorage.getItem("semaps.ports") || "uniform",
              },
              { type: "toggle", command: "view.edges.toggleStructure", size: "medium" },
            ],
          },
          {
            id: "model",
            get title() { return i18n.d.ribbon.groups.file; },
            items: [
              { type: "button", command: "file.code.toggle", size: "medium" },
            ],
          },
        ],
      },

      // --------------------------------------------------------------- Вид
      {
        id: "view",
        get title() { return i18n.d.ribbon.tabs.view; },
        keyTip: "V",
        groups: [
          {
            id: "canvas_options",
            get title() { return i18n.d.ribbon.groups.canvas; },
            items: [
              {
                type: "select",
                command: "view.strokeScaling.set",
                get label() { return i18n.d.ribbon.labels.strokeScaling; },
                get options() {
                  return [
                    { value: "zoom", label: i18n.d.ribbon.labels.strokeScalingZoom },
                    { value: "soft", label: i18n.d.ribbon.labels.strokeScalingSoft },
                    { value: "fixed", label: i18n.d.ribbon.labels.strokeScalingFixed },
                  ];
                },
                getValue: () => localStorage.getItem("semaps.strokeScaling") || "zoom",
              },
              { type: "toggle", command: "view.shadows.global.toggle", size: "medium" },
              { type: "toggle", command: "view.grid.toggle", size: "small" },
              { type: "toggle", command: "view.snap.toggle", size: "small" },
              { type: "toggle", command: "view.routing.debug", size: "small" },
            ],
          },
          {
            id: "zoom",
            get title() { return i18n.d.ribbon.groups.zoom; },
            items: [
              { type: "button", command: "view.zoom.in", size: "small" },
              { type: "button", command: "view.zoom.out", size: "small" },
              { type: "button", command: "view.zoom.reset", size: "small" },
              { type: "button", command: "view.zoom.fit", size: "small" },
            ],
          },
          {
            id: "panels",
            get title() { return i18n.d.ribbon.groups.panels; },
            items: [
              { type: "toggle", command: "panel.properties.toggle", size: "small" },
              { type: "toggle", command: "panel.relations.toggle", size: "small" },
              { type: "toggle", command: "panel.filters.toggle", size: "small" },
              { type: "toggle", command: "panel.styles.toggle", size: "small" },
              { type: "toggle", command: "panel.catalog.toggle", size: "small" },
              { type: "toggle", command: "panel.base.toggle", size: "small" },
              { type: "toggle", command: "panel.neighbourhood.toggle", size: "small" },
              { type: "separator" },
              { type: "button", command: "workspace.layout.reset", size: "small" },
            ],
          },
        ],
      },

      // ----------------------------------------------------------- Справка
      {
        id: "help",
        get title() { return i18n.d.ribbon.tabs.help; },
        keyTip: "F",
        groups: [
          {
            id: "help",
            get title() { return i18n.d.ribbon.groups.help; },
            items: [
              { type: "button", command: "help.structure", size: "large" },
            ],
          },
        ],
      },

      // ------------------------------------------------ Контекстные вкладки
      {
        id: "context_node",
        get title() { return i18n.d.ribbon.tabs.formatBlock; },
        keyTip: "B",
        contextual: "node",
        groups: [
          {
            id: "node_actions",
            get title() { return i18n.d.ribbon.groups.edit; },
            items: [
              { type: "button", command: "panel.properties.toggle", size: "large" },
              { type: "button", command: "panel.relations.toggle", size: "large" },
              { type: "button", command: "edit.delete", size: "medium" },
            ],
          },
        ],
      },
      {
        id: "context_zone",
        get title() { return i18n.d.ribbon.tabs.formatBlock; },
        keyTip: "Z",
        contextual: "zone",
        groups: [
          {
            id: "zone_actions",
            get title() { return i18n.d.ribbon.groups.edit; },
            items: [
              { type: "button", command: "panel.properties.toggle", size: "large" },
              { type: "button", command: "edit.delete", size: "medium" },
            ],
          },
        ],
      },
      {
        id: "context_edge",
        get title() { return i18n.d.ribbon.tabs.formatEdge; },
        keyTip: "E",
        contextual: "edge",
        groups: [
          {
            id: "edge_actions",
            get title() { return i18n.d.ribbon.groups.edit; },
            items: [
              { type: "button", command: "panel.properties.toggle", size: "large" },
              { type: "button", command: "edit.delete", size: "medium" },
            ],
          },
        ],
      },
    ],
  };
}

/**
 * How the tool looks, not what it shows: theme, density, languages; the
 * canvas effects follow it in the same panel.
 * Not a tab — the gear at the end of the tab row opens it in every mode.
 */
export function createDisplaySpec(): RibbonTabSpec {
  return {
    id: "display",
    get title() { return i18n.d.ribbon.tabs.settings; },
    groups: [
      {
        id: "appearance",
        get title() { return i18n.d.ribbon.groups.appearance; },
        items: [
          {
            type: "theme-gallery",
            command: "view.theme.set",
            themes: [
              { id: "cream", name: "Cream" },
              { id: "dark", name: "Dark" },
              { id: "emerald", name: "Emerald" },
              { id: "light", name: "Light" },
            ],
            getValue: () => localStorage.getItem("semaps:theme") || "cream",
          },
        ],
      },
      {
        id: "interface",
        get title() { return i18n.d.ribbon.groups.interface; },
        items: [
          {
            type: "select",
            command: "view.density.set",
            get label() { return i18n.d.ribbon.labels.density; },
            options: [
              { value: "nano", label: "Nano" },
              { value: "mini", label: "Mini" },
              { value: "norm", label: "Norm" },
            ],
            getValue: () => localStorage.getItem("semaps.density") || "norm",
          },
          {
            type: "select",
            command: "view.uiLang.set",
            get label() { return i18n.d.ribbon.labels.uiLang; },
            options: [
              { value: "ru", label: "Русский (RU)" },
              { value: "en", label: "English (EN)" },
            ],
            getValue: (ctx: CommandContext) => ctx.uiLang || i18n.currentLanguage,
          },
          {
            type: "select",
            command: "view.lang.set",
            get label() { return i18n.d.ribbon.labels.dataLang; },
            options: [
              { value: "ru", label: "RU" },
              { value: "en", label: "EN" },
            ],
            getValue: (ctx: CommandContext) => ctx.dataLang || "ru",
          },
        ],
      },
    ],
  };
}
