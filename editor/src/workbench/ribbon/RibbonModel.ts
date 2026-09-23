import type { RibbonSpec } from "./types.js";
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
            id: "file",
            get title() { return i18n.d.ribbon.groups.file; },
            items: [
              { type: "button", command: "file.save", size: "large" },
              { type: "button", command: "file.open", size: "medium" },
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
            id: "catalogs",
            get title() { return i18n.d.ribbon.groups.panels; },
            items: [
              { type: "button", command: "panel.base.toggle", size: "large" },
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
              { type: "button", command: "view.filters.visual.toggle", size: "medium" },
              { type: "toggle", command: "view.shadows.global.toggle", size: "medium" },
              { type: "toggle", command: "view.grid.toggle", size: "small" },
              { type: "toggle", command: "view.snap.toggle", size: "small" },
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
                getValue: () => "cream",
              },
              { type: "separator" },
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
              { type: "separator" },
              { type: "button", command: "workspace.layout.reset", size: "small" },
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
