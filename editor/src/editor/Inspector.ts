import type { DiagramCanvas, Selection } from "../canvas/DiagramCanvas.js";
import type { StyleLibrary } from "../model/StyleLibrary.js";
import type { StyleTarget } from "../model/style-types.js";
import type { DiagramEdge, DiagramElement } from "../model/types.js";
import { isContainer } from "../model/types.js";
import { el, replaceChildren } from "../util/dom.js";
import { field, select, type Option } from "./fields.js";
import { blockPreview, edgePreview } from "./style-preview.js";
import { SourceCodeService } from "./code/SourceCodeService.js";
import { i18n } from "../workbench/i18n/I18nService.js";

export interface InspectorHost {
  readonly canvas: DiagramCanvas;
  /** The style library, so the inspector can offer what actually exists. */
  readonly styles: StyleLibrary;
  /** Apply a field edit and mark the document dirty. */
  dataLang: string;
  editField(apply: () => void, options?: { rerender?: boolean; reselect?: boolean }): void;
  deleteSelection(): void;
  addEdgeFromSelection(targetId: string, type: string, label: string): void;
  deleteEdge(edgeId: string): void;
  /** Switch the right-hand panel to the Styles tab with this style open. */
  openStyleTab(styleId: string): void;
  openTab(tab: "properties" | "edges" | "filters" | "styles" | "base"): void;
  openDocEditor(targetId?: string | null, kind?: "node" | "zone" | "edge"): void;
  openCodeViewer?(codeRef?: string | null, label?: string): void;
}

/**
 * The right-hand properties panel.
 *
 * Rebuilt from the model on selection change, and updated in place while a
 * gesture is running so the coordinate readout tracks the drag.
 *
 * It no longer paints anything itself. The three inline colour fields that used
 * to live here are gone: an element's look is a *named style*, and the choice
 * offered here is which one it wears — so that changing how records look is one
 * edit rather than three hundred.
 */
export class Inspector {
  private coordsNode: HTMLElement | null = null;

  constructor(
    private readonly badge: HTMLElement,
    private readonly body: HTMLElement,
    private readonly host: InspectorHost,
  ) {}

  render(selection: Selection | null): void {
    this.coordsNode = null;
    const doc = this.host.canvas.model;

    if (selection === null || doc === null) {
      this.renderEmpty();
      return;
    }

    if (selection.kind === "edge") {
      let edge = doc.edge(selection.id);
      let isVisibleOnCanvas = true;

      if (edge === undefined) {
        const relations = doc.relations;
        const rel = Array.isArray(relations)
          ? relations.find((r) => r.id === selection.id)
          : undefined;

        if (rel) {
          isVisibleOnCanvas = false;
          edge = {
            id: rel.id,
            from: rel.from,
            to: rel.to,
            type: rel.type || rel.relation || "relates",
            label: rel.label || "",
            styleId: rel.styleId,
          };
        } else if (selection.id.startsWith("ghost_")) {
          const parts = selection.id.split("_");
          if (parts.length >= 3 && parts[1] && parts[2]) {
            isVisibleOnCanvas = false;
            edge = {
              id: selection.id,
              from: parts[1],
              to: parts[2],
              type: "call",
              label: "",
            };
          }
        }
      }

      if (edge === undefined) {
        this.renderEmpty();
        return;
      }
      this.renderEdge(edge, isVisibleOnCanvas);
      return;
    }

    const element = doc.element(selection.id);
    if (element === undefined) {
      this.renderEmpty();
      return;
    }
    this.renderElement(element);
  }

  /** Live coordinate readout during a drag or resize (R-INSP-03). */
  updateGeometry(element: DiagramElement): void {
    if (this.coordsNode === null) return;
    this.coordsNode.textContent = formatGeometry(element);
  }

  private renderEmpty(): void {
    this.badge.textContent = i18n.d.common.notSelected;
    this.badge.className = "badge";
    replaceChildren(
      this.body,
      el("div", { class: "inspector-empty" }, [
        el("p", { class: "inspector-empty-icon", text: "👆" }),
        el("p", {
          text: i18n.d.panels.properties.empty,
        }),
      ]),
    );
  }

  private renderElement(element: DiagramElement): void {
    const container = isContainer(element);
    const count = this.host.canvas.selectedIds.size;

    this.badge.textContent =
      count > 1
        ? `${i18n.d.common.all}: ${count}`
        : `${container ? "CONTAINER" : "NODE"}: ${element.type}`;
    this.badge.className = `badge ${container ? "badge-zone" : "badge-node"}`;

    const coords = el("span", { class: "mono coords", text: formatGeometry(element) });
    this.coordsNode = coords;

    replaceChildren(
      this.body,
      count > 1
        ? el("div", {
            class: "panel panel-info",
            text: i18n.format(i18n.d.panels.properties.multiSelected, { count, label: element.label }),
          })
        : null,
      el("div", { class: "field-row" }, [
        el("span", { class: "mono muted", text: `ID: ${element.id}` }),
        coords,
      ]),
      this.labelField(element),
      el("div", { class: "grid-2" }, [
        this.typeField(element),
        this.parentField(element),
      ]),
      container ? this.containerPanel(element) : null,
      this.blockStylePicker(element),
      this.templatePicker(element),
      this.descriptionField(element),
      this.codeRefField(element),
      container ? null : el("div", { class: "panel-section" }, [
        el("button", {
          class: "btn full",
          text: i18n.format(i18n.d.panels.properties.blockRelationsBtn, {
            count: this.host.canvas.model?.outgoingEdges(element.id).length ?? 0,
          }),
          on: { click: () => this.host.openTab("edges") },
        }),
      ]),
      this.deleteButton(container),
    );
  }

  private renderEdge(edge: DiagramEdge, isVisibleOnCanvas = true): void {
    const doc = this.host.canvas.model;
    const fromEl = doc?.element(edge.from);
    const toEl = doc?.element(edge.to);

    this.badge.textContent = isVisibleOnCanvas ? `EDGE: ${edge.type}` : `GHOST EDGE: ${edge.type}`;
    this.badge.className = `badge ${isVisibleOnCanvas ? "badge-edge" : "badge-node"}`;

    const statusToggle = el("div", {
      class: "field-row",
      attrs: {
        style:
          "background: var(--panel-alt); padding: 8px 12px; border-radius: 8px; border: 1px solid var(--line); margin-bottom: 8px; justify-content: space-between; align-items: center;",
      },
    }, [
      el("span", { text: i18n.d.panels.properties.edgeDisplayOnCanvas, attrs: { style: "font-weight: 600; font-size: 12px;" } }),
      el("div", {
        class: "pill-group",
        attrs: { style: "display: flex; gap: 4px;" },
      }, [
        el("button", {
          class: `btn btn-small ${isVisibleOnCanvas ? "btn-primary" : ""}`,
          text: i18n.d.panels.properties.edgeEnabled,
          attrs: { style: isVisibleOnCanvas ? "font-weight: 700;" : "opacity: 0.7;" },
          on: {
            click: () => {
              if (!isVisibleOnCanvas && doc) {
                doc.addEdge({
                  id: edge.id,
                  from: edge.from,
                  to: edge.to,
                  type: edge.type,
                  label: edge.label,
                  styleId: edge.styleId,
                });
                (this.host as any).commit("show-edge");
                this.renderEdge(edge, true);
              }
            },
          },
        }),
        el("button", {
          class: `btn btn-small ${!isVisibleOnCanvas ? "btn-secondary" : ""}`,
          text: i18n.d.panels.properties.edgeGhost,
          attrs: { style: !isVisibleOnCanvas ? "font-weight: 700; border-color: var(--accent); color: var(--accent);" : "opacity: 0.7;" },
          on: {
            click: () => {
              if (isVisibleOnCanvas && doc) {
                doc.removeEdge(edge.id);
                (this.host as any).commit("hide-edge");
                this.renderEdge(edge, false);
              }
            },
          },
        }),
      ]),
    ]);

    replaceChildren(
      this.body,
      statusToggle,
      el("div", { class: "field-row" }, [
        el("span", { class: "mono muted", text: `ID: ${edge.id}` }),
        el("span", { class: "mono coords", text: `${fromEl?.label || edge.from} ➔ ${toEl?.label || edge.to}` }),
      ]),
      el("label", { class: "field" }, [
        el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between; margin-bottom: 2px;" } }, [
          el("span", { class: "field-label", text: i18n.d.panels.properties.edgeLabelTitle }),
          el("span", { class: "chip chip-lang", text: (this.host.dataLang || "ru").toUpperCase() }),
        ]),
        el("input", {
          type: "text",
          value: edge.label,
          placeholder: i18n.d.panels.properties.edgeLabelPlaceholder,
          on: {
            input: (e) => {
              const value = (e.target as HTMLInputElement).value;
              edge.label = value;
              this.host.canvas.model?.setText(edge.id, { name: value, description: value }, this.host.dataLang || "ru");
              if (isVisibleOnCanvas) {
                this.host.editField(() => {
                  edge.label = value;
                }, { rerender: true });
              } else {
                const rels = doc?.relations;
                const r = Array.isArray(rels) ? rels.find((x) => x.id === edge.id) : null;
                if (r) r.label = value;
                this.host.canvas.render();
              }
            },
          },
        }),
      ]),
      // A code-origin edge carries no text at all (ADR_20260831 §2.13): its
      // meaning is the type, regenerated by `sync` on every run, so offering
      // an input here would invite an edit that the next sync silently drops.
      edge.origin === "code"
        ? el("div", { class: "muted italic", text: i18n.d.panels.properties.edgeCodeOriginNotice })
        : el("div", { class: "grid-2" }, [
            el("label", { class: "field" }, [
              el("span", { class: "field-label", text: i18n.d.panels.properties.edgeFromLabelTitle }),
              el("input", {
                type: "text",
                value: edge.fromLabel ?? "",
                placeholder: i18n.d.panels.properties.edgeFromLabelPlaceholder,
                on: {
                  input: (e) => {
                    const value = (e.target as HTMLInputElement).value;
                    edge.fromLabel = value;
                    this.host.canvas.model?.setText(edge.id, { fromLabel: value }, this.host.dataLang || "ru");
                    if (isVisibleOnCanvas) {
                      this.host.editField(() => {
                        edge.fromLabel = value;
                      }, { rerender: true });
                    } else {
                      this.host.canvas.render();
                    }
                  },
                },
              }),
            ]),
            el("label", { class: "field" }, [
              el("span", { class: "field-label", text: i18n.d.panels.properties.edgeToLabelTitle }),
              el("input", {
                type: "text",
                value: edge.toLabel ?? "",
                placeholder: i18n.d.panels.properties.edgeToLabelPlaceholder,
                on: {
                  input: (e) => {
                    const value = (e.target as HTMLInputElement).value;
                    edge.toLabel = value;
                    this.host.canvas.model?.setText(edge.id, { toLabel: value }, this.host.dataLang || "ru");
                    if (isVisibleOnCanvas) {
                      this.host.editField(() => {
                        edge.toLabel = value;
                      }, { rerender: true });
                    } else {
                      this.host.canvas.render();
                    }
                  },
                },
              }),
            ]),
          ]),
      el("div", { class: "field" }, [
        el("button", {
          class: "btn full",
          attrs: { style: "display: flex; align-items: center; justify-content: center; gap: 6px;" },
          text: `📄 ${i18n.d.dialogs.docEditor.openEditorBtn}`,
          on: {
            click: () => this.host.openDocEditor(edge.id, "edge"),
          },
        }),
      ]),
      el("label", { class: "field" }, [
        el("span", { class: "field-label", text: i18n.d.panels.properties.edgeTypeTitle }),
        select(this.edgeTypeOptions(edge.type), edge.type, (value) => {
          edge.type = value;
          if (isVisibleOnCanvas) {
            this.host.editField(() => {
              edge.type = value;
            }, { rerender: true, reselect: true });
          } else {
            const rels = doc?.relations;
            const r = Array.isArray(rels) ? rels.find((x) => x.id === edge.id) : null;
            if (r) {
              r.type = value;
              r.relation = value;
            }
            this.host.canvas.render();
          }
        }),
      ]),
      this.edgeStylePicker(edge),
      el("div", { class: "field" }, [
        el("button", {
          class: "btn btn-danger full",
          text: i18n.d.panels.properties.deleteEdgeBtn,
          on: {
            click: () => {
              if (isVisibleOnCanvas) {
                this.host.deleteEdge(edge.id);
              }
              const rels = doc?.relations;
              if (Array.isArray(rels)) {
                const idx = rels.findIndex((x) => x.id === edge.id);
                if (idx >= 0) rels.splice(idx, 1);
              }
              (this.host as any).commit("delete-edge");
              this.host.canvas.select(null);
            },
          },
        }),
      ]),
    );
  }

  // ----------------------------------------------------------------- fields

  private labelField(element: DiagramElement): HTMLElement {
    const lang = (this.host.dataLang || "ru").toUpperCase();
    return el("label", { class: "field" }, [
      el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between; margin-bottom: 2px;" } }, [
        el("span", { class: "field-label", text: i18n.d.panels.properties.labelTitle }),
        el("span", { class: "chip chip-lang", text: lang }),
      ]),
      el("input", {
        class: "input-strong",
        type: "text",
        value: element.label,
        on: {
          input: (e) => {
            const value = (e.target as HTMLInputElement).value;
            this.host.editField(() => {
              element.label = value;
              this.host.canvas.model?.setText(element.id, { name: value, title: value }, this.host.dataLang || "ru");
            }, { rerender: true });
          },
        },
      }),
    ]);
  }

  private typeField(element: DiagramElement): HTMLElement {
    return el("label", { class: "field" }, [
      el("span", { class: "field-label", text: i18n.d.panels.properties.typeTitle }),
      select(this.typeOptions(element.type), element.type, (value) => {
        this.host.editField(
          () => {
            element.type = value;
            element.metadata.type = value;
          },
          { rerender: true, reselect: true },
        );
      }),
    ]);
  }

  private typeOptions(current: string): Option[] {
    const lib = this.host.styles;
    const options: Option[] = lib.list("block").map((entry) => {
      const glyph = lib.resolveBlock(entry.id).icon.glyph;
      return [entry.id, `${glyph} ${entry.name}`] as Option;
    });
    if (!options.some(([value]) => value === current)) options.push([current, `❓ ${current}`]);
    return options;
  }

  private edgeTypeOptions(current: string): Option[] {
    const options: Option[] = this.host.styles
      .list("edge")
      .map((entry) => [entry.id, entry.name] as Option);
    if (!options.some(([value]) => value === current)) options.push([current, `❓ ${current}`]);
    return options;
  }

  private parentField(element: DiagramElement): HTMLElement {
    const parent = element.parent;
    const label = isContainer(element)
      ? i18n.d.panels.properties.isZone
      : parent === null
        ? i18n.d.panels.properties.outsideZones
        : parent.label;

    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: i18n.d.panels.properties.parentTitle }),
      el("div", { class: "readonly-box", text: label }),
    ]);
  }

  private containerPanel(element: DiagramElement): HTMLElement {
    const collapsed = this.host.canvas.isCollapsed(element);
    const count = element.children.filter((c) => !isContainer(c)).length;

    return el("div", { class: "panel panel-info" }, [
      el("span", { text: i18n.format(i18n.d.panels.properties.nestedCount, { count }) }),
      el("button", {
        class: "btn btn-small",
        text: collapsed ? i18n.d.panels.properties.expand : i18n.d.panels.properties.collapse,
        on: { click: () => this.host.canvas.toggleCollapse(element.id) },
      }),
    ]);
  }

  // ---------------------------------------------------------- style picking

  private blockStylePicker(element: DiagramElement): HTMLElement {
    const lib = this.host.styles;
    const activeId = lib.blockStyleIdFor(element);
    const explicit = element.styleId !== undefined && lib.has(element.styleId);
    const source = explicit
      ? i18n.d.panels.properties.styleExplicit
      : lib.has(element.type)
        ? i18n.format(i18n.d.panels.properties.styleByType, { type: element.type })
        : i18n.d.panels.properties.styleDefault;

    return this.stylePicker("block", activeId, source, explicit, {
      pick: (id) => {
        this.host.editField(
          () => {
            element.styleId = id;
          },
          { rerender: true, reselect: true },
        );
      },
      reset: () => {
        this.host.editField(
          () => {
            element.styleId = undefined;
          },
          { rerender: true, reselect: true },
        );
      },
    });
  }

  /**
   * Which content template this element draws with (ADR_20260903 §2.2).
   *
   * The cascade is placement → style → nothing, so the override written here
   * is `metadata.template` — read back by `DiagramCanvas.resolveContent` —
   * and "по умолчанию" clears the key entirely rather than writing an empty
   * string: an empty string is a real (if useless) template id, and would
   * stop the element from falling back to its style's template at all.
   */
  private templatePicker(element: DiagramElement): HTMLElement {
    const templates = this.host.canvas.templates.list();
    const styleTemplate = this.host.styles.resolveBlock(this.host.styles.blockStyleIdFor(element)).template;
    const override = typeof element.metadata.template === "string" ? element.metadata.template : undefined;

    const inheritedLabel =
      styleTemplate !== null
        ? templates.find((t) => t.id === styleTemplate)?.name ?? styleTemplate
        : i18n.d.panels.properties.templateNone;

    const options: Option[] = [
      ["", i18n.format(i18n.d.panels.properties.templateInherit, { source: inheritedLabel })],
      ...templates.map((t) => [t.id, t.name ?? t.id] as Option),
    ];

    return el("div", { class: "panel-section" }, [
      field(
        i18n.d.panels.properties.templateTitle,
        select(options, override ?? "", (value) => {
          this.host.editField(
            () => {
              if (value === "") delete element.metadata.template;
              else element.metadata.template = value;
            },
            { rerender: true },
          );
        }),
      ),
    ]);
  }

  private edgeStylePicker(edge: DiagramEdge): HTMLElement {
    const lib = this.host.styles;
    const activeId = lib.edgeStyleIdFor(edge);
    const explicit = edge.styleId !== undefined && lib.has(edge.styleId);
    const source = explicit
      ? i18n.d.panels.properties.styleExplicit
      : lib.has(edge.type)
        ? i18n.format(i18n.d.panels.properties.styleByType, { type: edge.type })
        : i18n.d.panels.properties.styleDefault;

    return this.stylePicker("edge", activeId, source, explicit, {
      pick: (id) => {
        this.host.editField(
          () => {
            edge.styleId = id;
          },
          { rerender: true, reselect: true },
        );
      },
      reset: () => {
        this.host.editField(
          () => {
            edge.styleId = undefined;
          },
          { rerender: true, reselect: true },
        );
      },
    });
  }

  private stylePicker(
    target: StyleTarget,
    activeId: string | null,
    source: string,
    explicit: boolean,
    actions: { pick: (id: string) => void; reset: () => void },
  ): HTMLElement {
    const lib = this.host.styles;
    const rows = el("div", { class: "style-rows style-rows-compact" });

    const renderRows = (filter: string): void => {
      const entries = lib.list(target, filter);
      if (entries.length === 0) {
        replaceChildren(
          rows,
          el("div", { class: "muted italic style-empty", text: i18n.d.panels.properties.styleNotFound }),
        );
        return;
      }
      replaceChildren(
        rows,
        ...entries.map((entry) =>
          el(
            "div",
            {
              class: `style-row${entry.id === activeId ? " is-active" : ""}`,
              title: entry.style.description ?? "",
              on: { click: () => actions.pick(entry.id) },
            },
            [
              el("div", { class: "style-row-preview" }, [
                target === "block"
                  ? blockPreview(lib.resolveBlock(entry.id))
                  : edgePreview(lib.resolveEdge(entry.id)),
              ]),
              el("div", { class: "style-row-text" }, [
                el("span", { class: "style-row-name", text: entry.name }),
                el("span", { class: "mono muted style-row-id", text: entry.id }),
              ]),
            ],
          ),
        ),
      );
    };
    renderRows("");

    return el("div", { class: "panel-section" }, [
      el("div", { class: "field-row" }, [
        el("span", { class: "field-label accent", text: i18n.d.panels.properties.styleTitle }),
        el("span", { class: "muted", text: source }),
      ]),
      activeId === null
        ? el("div", { class: "muted italic", text: i18n.d.panels.properties.styleEmptyLib })
        : el("div", { class: "style-current" }, [
            el("div", { class: "style-row-preview" }, [
              target === "block"
                ? blockPreview(lib.resolveBlock(activeId))
                : edgePreview(lib.resolveEdge(activeId)),
            ]),
            el("div", { class: "style-row-text" }, [
              el("span", {
                class: "style-row-name",
                text: lib.get(activeId)?.name ?? activeId,
              }),
              el("span", { class: "mono muted style-row-id", text: activeId }),
            ]),
          ]),
      el("input", {
        class: "style-filter",
        type: "text",
        placeholder: i18n.d.panels.properties.styleFilterPlaceholder,
        on: {
          input: (e) => renderRows((e.target as HTMLInputElement).value),
        },
      }),
      rows,
      el("div", { class: "field-row gap" }, [
        el("button", {
          class: "btn btn-small full",
          text: i18n.d.panels.properties.styleResetToType,
          title: i18n.d.panels.properties.styleResetTitle,
          disabled: !explicit,
          on: { click: () => actions.reset() },
        }),
        el("button", {
          class: "btn btn-small full",
          text: i18n.d.panels.properties.styleEditBtn,
          disabled: activeId === null,
          on: {
            click: () => {
              if (activeId !== null) this.host.openStyleTab(activeId);
            },
          },
        }),
      ]),
    ]);
  }

  // ------------------------------------------------------------------ rest

  private descriptionField(element: DiagramElement): HTMLElement {
    const currentLang = this.host.dataLang || "ru";
    const langBadge = currentLang.toUpperCase();
    const textEntry = this.host.canvas.model?.getText(element.id, currentLang);
    const value = textEntry?.description || (typeof element.metadata.description === "string" ? element.metadata.description : "");
    const hasDoc = Boolean(textEntry?.doc?.trim());

    return el("div", { class: "field-group" }, [
      el("label", { class: "field" }, [
        el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between; margin-bottom: 2px;" } }, [
          el("span", { class: "field-label", text: i18n.d.panels.properties.descriptionTitle }),
          el("span", { class: "chip chip-lang", text: langBadge }),
        ]),
        el("textarea", {
          rows: 2,
          value,
          placeholder: i18n.d.panels.properties.descriptionPlaceholder,
          on: {
            input: (e) => {
              const next = (e.target as HTMLTextAreaElement).value;
              this.host.editField(() => {
                element.metadata.description = next;
                this.host.canvas.model?.setText(element.id, { description: next }, currentLang);
              });
            },
          },
        }),
      ]),
      el("div", { class: "field", attrs: { style: "margin-top: 4px;" } }, [
        el("button", {
          class: `btn full ${hasDoc ? "btn-secondary" : ""}`,
          attrs: { style: "display: flex; align-items: center; justify-content: center; gap: 6px;" },
          text: `📄 ${i18n.d.dialogs.docEditor.openEditorBtn}${hasDoc ? " (✓)" : ""}`,
          on: {
            click: () => this.host.openDocEditor(element.id, isContainer(element) ? "zone" : "node"),
          },
        }),
      ]),
    ]);
  }

  private codeRefField(element: DiagramElement): HTMLElement {
    const value = typeof element.metadata.codeRef === "string" ? element.metadata.codeRef : "";
    const isAvailable = value.trim() ? SourceCodeService.isFileAvailable(value.trim()) : false;
    const showCodeBtn = Boolean(value.trim()) && isAvailable !== false;

    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: i18n.d.panels.properties.codeRefTitle }),
      el("div", { attrs: { style: "display: flex; gap: 6px; align-items: center;" } }, [
        el("input", {
          class: "mono input-code",
          attrs: { style: "flex: 1; min-width: 0;" },
          type: "text",
          value,
          placeholder: i18n.d.panels.properties.codeRefPlaceholder,
          on: {
            input: (e) => {
              const next = (e.target as HTMLInputElement).value;
              this.host.editField(() => {
                element.metadata.codeRef = next;
              });
            },
          },
        }),
        showCodeBtn
          ? el("button", {
              class: "btn btn-secondary",
              title: "Просмотреть исходный код (без сохранения)",
              text: "💻",
              attrs: { style: "padding: 4px 8px; flex-shrink: 0;" },
              on: {
                click: () => this.host.openCodeViewer?.(value, element.label),
              },
            })
          : null,
      ].filter(Boolean) as HTMLElement[]),
    ]);
  }

  private deleteButton(container: boolean): HTMLElement {
    return el("div", { class: "panel-section" }, [
      el("button", {
        class: "btn btn-danger full",
        text: i18n.format(i18n.d.panels.properties.deleteItemBtn, {
          item: container ? "zone" : "block",
        }),
        on: { click: () => this.host.deleteSelection() },
      }),
    ]);
  }
}

function formatGeometry(element: DiagramElement): string {
  return `X:${Math.round(element.x)} Y:${Math.round(element.y)} (${Math.round(element.width)}×${Math.round(element.height)})`;
}
