import type { DiagramCanvas, Selection } from "../canvas/DiagramCanvas.js";
import type { StyleLibrary } from "../model/StyleLibrary.js";
import type { StyleTarget, WireStyle } from "../model/style-types.js";
import type { DiagramEdge, DiagramElement } from "../model/types.js";
import { entityOf, isContainer } from "../model/types.js";
import { el, replaceChildren } from "../util/dom.js";
import { colorField, field, select, type Option } from "./fields.js";
import { blockPreview, edgePreview } from "./style-preview.js";
import { SourceCodeService } from "./code/SourceCodeService.js";
import { i18n } from "../workbench/i18n/I18nService.js";
import { iconEl } from "../ui/icons.js";
import { iconElByKey } from "../ui/kindIcons.js";
import { iconPicker } from "../ui/iconPicker.js";
import { KindCatalog } from "../model/KindCatalog.js";
import { HIGHLIGHT_PALETTE, LINE_PALETTE } from "../model/highlight-palette.js";
import {
  EDGE_OVERRIDE_FIELDS,
  OVERRIDE_FIELDS,
  overrideValue,
  styleValue,
  withOverride,
  type OverrideField,
  type PlacementOverride,
} from "../model/override.js";
import { kindSelect, relationTypeSelect, variantLabel } from "./kindSelects.js";
import { nameIsText } from "../model/entityName.js";
import { evidenceOf, langTag, realizationsOf, refOf } from "../model/realizations.js";

export interface InspectorHost {
  readonly canvas: DiagramCanvas;
  /** The style library, so the inspector can offer what actually exists. */
  readonly styles: StyleLibrary;
  /** Apply a field edit and mark the document dirty. */
  dataLang: string;
  /** Copy the link of the selection: `view#id`. */
  copyLink?(ids?: readonly string[]): void;
  editField(apply: () => void, options?: { rerender?: boolean; reselect?: boolean }): void;
  deleteSelection(): void;
  /**
   * The one way to set kind and style of blocks and containers (ADR_20260927-7):
   * `styleId` null or the kind's base style removes the explicit style; a kind
   * from code stays; changing the kind drops the explicit style.
   */
  applyKindAndStyle(ids: readonly string[], kind: string, styleId: string | null): void;
  /**
   * The same for relations (ADR_20260930-2): the relation type and the style
   * of that type together; changing the type drops the explicit style.
   */
  applyRelationTypeAndStyle(ids: readonly string[], type: string, styleId: string | null): void;
  /** Set a content template on boxes, growing them to fit (null — the style's). */
  applyTemplate(ids: readonly string[], template: string | null): void;
  addEdgeFromSelection(targetId: string, type: string, label: string): void;
  deleteEdge(edgeId: string): void;
  /** Show a registry relation on the view, or hide it back to a ghost. */
  setEdgeShown(id: string, shown: boolean): void;
  /** Switch the right-hand panel to the Styles tab with this style open. */
  openStyleTab(styleId: string): void;
  openTab(tab: "properties" | "edges" | "filters" | "styles" | "base"): void;
  openDocEditor(targetId?: string | null, kind?: "node" | "zone" | "edge"): void;
  openCodeViewer?(ref?: string | null, label?: string): void;
  /** Copy the entity id to the clipboard and say so with a toast. */
  copyId?(id: string): void;
}

/**
 * The right-hand properties panel.
 *
 * Rebuilt from the model on selection change, and updated in place while a
 * gesture is running so the coordinate readout tracks the drag.
 *
 * It paints nothing itself. What a thing IS — its type — is chosen from the
 * dictionary; how it looks follows from the type: its base style, and only the
 * variants of that type when it has more than one (ADR_20260927-7,
 * ADR_20260930-2). One box that must stand out gets an `override` (the
 * «Выделить» section), not a style of its own.
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
            type: rel.type,
            label: rel.label || "",
            ...(rel.origin === undefined ? {} : { origin: rel.origin }),
          };
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
        el("p", { class: "inspector-empty-icon",  }, [iconEl("pointer", "ui-icon-lg")]),
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
        : `${container ? "CONTAINER" : "NODE"}: ${KindCatalog.active.name(element.type, i18n.currentLanguage)}`;
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
        this.copyLinkButton(element.id),
        coords,
      ]),
      this.labelField(element),
      el("div", { class: "grid-2" }, [
        this.kindField(element),
        this.parentField(element),
      ]),
      container ? this.containerPanel(element) : null,
      this.styleSection(element),
      this.overrideSection(element),
      this.templatePicker(element),
      this.descriptionField(element),
      this.realizationsField(element),
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

    this.badge.textContent = `${isVisibleOnCanvas ? "EDGE" : "GHOST EDGE"}: ${KindCatalog.active.relationName(edge.type, i18n.currentLanguage)}`;
    this.badge.className = `badge ${isVisibleOnCanvas ? "badge-edge" : "badge-node"}`;

    const statusToggle = el("div", {
      class: "field-row",
      attrs: {
        style:
          "background: var(--panel-alt); padding: calc(8px * var(--ui-space)) calc(12px * var(--ui-space)); border-radius: 8px; border: 1px solid var(--line); margin-bottom: calc(8px * var(--ui-space)); justify-content: space-between; align-items: center;",
      },
    }, [
      el("span", { text: i18n.d.panels.properties.edgeDisplayOnCanvas, attrs: { style: "font-weight: 600; font-size: calc(12px * var(--ui-text));" } }),
      el("div", {
        class: "pill-group",
        attrs: { style: "display: flex; gap: calc(4px * var(--ui-space));" },
      }, [
        el("button", {
          class: `btn btn-small ${isVisibleOnCanvas ? "btn-primary" : ""}`,
          attrs: { style: isVisibleOnCanvas ? "font-weight: 700;" : "opacity: 0.7;" },
          on: {
            click: () => {
              if (!isVisibleOnCanvas && doc) this.host.setEdgeShown(edge.id, true);
            },
          },
        }, [iconEl("eye"), i18n.d.panels.properties.edgeEnabled]),
        el("button", {
          class: `btn btn-small ${!isVisibleOnCanvas ? "btn-secondary" : ""}`,
          attrs: { style: !isVisibleOnCanvas ? "font-weight: 700; border-color: var(--accent); color: var(--accent);" : "opacity: 0.7;" },
          on: {
            click: () => {
              if (isVisibleOnCanvas && doc) this.host.setEdgeShown(edge.id, false);
            },
          },
        }, [iconEl("eyeOff"), i18n.d.panels.properties.edgeGhost]),
      ]),
    ]);

    replaceChildren(
      this.body,
      statusToggle,
      el("div", { class: "field-row" }, [
        el("span", { class: "mono muted", text: `ID: ${edge.id}` }),
        el("span", { class: "mono coords", text: `${fromEl?.label || edge.from} →${toEl?.label || edge.to}` }),
      ]),
      this.evidenceField(edge),
      el("label", { class: "field" }, [
        el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between; margin-bottom: calc(2px * var(--ui-space));" } }, [
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
          attrs: { style: "display: flex; align-items: center; justify-content: center; gap: calc(6px * var(--ui-space));" },
          on: {
            click: () => this.host.openDocEditor(edge.id, "edge"),
          },
        }, [iconEl("doc"), i18n.d.dialogs.docEditor.openEditorBtn]),
      ]),
      this.relationTypeField(edge),
      isVisibleOnCanvas
        ? this.edgeStyleSection(edge)
        : el("div", { class: "muted italic", text: i18n.d.panels.properties.edgeGhostStyleHint }),
      isVisibleOnCanvas ? this.edgeOverrideSection(edge) : null,
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

  /** Copies the link to this object: paste it to an agent. */
  private copyLinkButton(id: string): HTMLElement {
    const btn = el("button", { class: "btn", attrs: { title: "Скопировать ссылку на объект (для агента)" } }, [iconEl("link")]);
    btn.addEventListener("click", () => this.host.copyLink?.([id]));
    return btn;
  }

  /**
   * The entity's name, with its id beside it (read-only, with a copy button).
   *
   * The name of an authored entity is a text — `name` in `text.<lang>.json` —
   * so it is edited per language (the chip says which) and never touches the
   * entity record. A name read from code is the code's: shown, not edited.
   */
  private labelField(element: DiagramElement): HTMLElement {
    const t = i18n.d.panels.properties;
    const doc = this.host.canvas.model;
    const entity = entityOf(element) ?? doc?.entities.find((e) => e.id === element.id);
    const editable = doc !== null && nameIsText(entity);

    const name = editable
      ? el("label", { class: "field", attrs: { style: "min-width: 0;" } }, [
          el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between;" } }, [
            el("span", { class: "field-label", text: t.labelTitle }),
            el("span", { class: "chip chip-lang", text: doc.textLang.toUpperCase() }),
          ]),
          el("input", {
            class: "input-strong",
            type: "text",
            value: element.label,
            on: {
              input: (e) => {
                const value = (e.target as HTMLInputElement).value;
                // The host refuses an empty name: nothing is sent, the old name stays.
                if (value.trim() === "") return;
                this.host.editField(() => {
                  element.label = value;
                  doc.setText(element.id, { name: value }, doc.textLang);
                }, { rerender: true });
              },
              change: (e) => {
                const input = e.target as HTMLInputElement;
                if (input.value.trim() === "") input.value = element.label;
              },
            },
          }),
        ])
      : el("div", { class: "field", attrs: { style: "min-width: 0;" } }, [
          el("span", { class: "field-label", text: t.labelTitle }),
          el("div", { class: "readonly-box input-strong", text: element.label, title: t.nameFromCode }),
        ]);

    const id = el("div", { class: "field", attrs: { style: "min-width: 0;" } }, [
      el("span", { class: "field-label", text: t.idTitle }),
      el("div", { class: "field-row gap", attrs: { style: "justify-content: flex-start;" } }, [
        el("div", { class: "readonly-box mono", text: element.id, title: element.id, attrs: { style: "flex: 1; min-width: 0;" } }),
        el("button", {
          class: "btn btn-secondary btn-small",
          title: t.copyIdTitle,
          attrs: { style: "flex-shrink: 0;" },
          on: { click: () => this.host.copyId?.(element.id) },
        }, [iconEl("copy")]),
      ]),
    ]);

    return el("div", { class: "grid-2", attrs: { style: "align-items: end;" } }, [name, id]);
  }

  /**
   * What the element is — the entity's `kind` — as opposed to how it looks,
   * which is the style section's job. Chosen from the dictionary by group
   * (CONTRACT.md §6); a container offers only container kinds and a block only
   * the others, because turning one into the other is not a kind edit. A kind
   * outside the dictionary is shown and marked «не из словаря». A kind read from
   * code is shown, not edited: a class does not become an enum because it was
   * drawn differently.
   */
  private kindField(element: DiagramElement): HTMLElement {
    const t = i18n.d.panels.properties;
    const kinds = KindCatalog.active;
    const lang = i18n.currentLanguage;
    const label = el("span", { class: "field-label", text: t.typeTitle });
    const known = kinds.lookup(element.type) !== undefined;
    const description = kinds.description(element.type, lang);

    if (entityOf(element)?.origin === "code") {
      return el("div", { class: "field" }, [
        label,
        el("div", {
          class: "readonly-box",
          text: known
            ? `${kinds.name(element.type, lang)} · ${element.type}`
            : `${element.type} (${t.kindNotInCatalog})`,
          title: [description, t.kindFromCode].filter(Boolean).join("\n\n"),
        }),
      ]);
    }

    const picker = kindSelect(element.type, isContainer(element), (value) => {
      this.host.applyKindAndStyle(this.targetIds(element), value, null);
    });
    return el("label", { class: "field", title: description || t.kindHint }, [label, picker]);
  }

  /**
   * The relation's type — `type` of the relation (CONTRACT.md §4) — chosen from
   * the dictionary of relation types by group, the same way an entity's kind is.
   * Changing it drops the explicit style. A type read from code stays the code's.
   */
  private relationTypeField(edge: DiagramEdge): HTMLElement {
    const t = i18n.d.panels.properties;
    const catalog = KindCatalog.active;
    const lang = i18n.currentLanguage;
    const label = el("span", { class: "field-label", text: t.edgeTypeTitle });
    const description = catalog.relationDescription(edge.type, lang);
    const rel = this.host.canvas.model?.relations.find((r) => r.id === edge.id);

    if (edge.origin === "code" || rel?.origin === "code") {
      const known = catalog.lookupRelation(edge.type) !== undefined;
      return el("div", { class: "field" }, [
        label,
        el("div", {
          class: "readonly-box",
          text: known ? `${catalog.relationName(edge.type, lang)} · ${edge.type}` : `${edge.type} (${t.kindNotInCatalog})`,
          title: [description, t.edgeTypeFromCode].filter(Boolean).join("\n\n"),
        }),
      ]);
    }

    const picker = relationTypeSelect(edge.type, (value) => {
      this.host.applyRelationTypeAndStyle(this.edgeTargetIds(edge), value, null);
    });
    return el("label", { class: "field", title: description || t.edgeTypeHint }, [label, picker]);
  }

  private parentField(element: DiagramElement): HTMLElement {
    // Blocks and containers alike lie in a container, or in none (CONTRACT.md §8.2).
    const parent = element.parent;
    const label = parent === null ? i18n.d.panels.properties.outsideContainers : parent.label;

    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: i18n.d.panels.properties.parentTitle }),
      el("div", { class: "readonly-box", text: label }),
    ]);
  }

  private containerPanel(element: DiagramElement): HTMLElement {
    const collapsed = this.host.canvas.isCollapsed(element);
    const count = element.children.filter((c) => !isContainer(c)).length;

    return el("div", { class: "panel panel-info" }, [
      el("span", { attrs: { style: "display: inline-flex; align-items: center; gap: 4px;" } }, [
        iconEl("box"),
        i18n.format(i18n.d.panels.properties.nestedCount, { count }),
      ]),
      el("button", {
        class: "btn btn-small",
        text: collapsed ? i18n.d.panels.properties.expand : i18n.d.panels.properties.collapse,
        on: { click: () => this.host.canvas.toggleCollapse(element.id) },
      }),
    ]);
  }

  // ---------------------------------------------------------- style picking

  /** What a kind or style choice applies to: the selection when it holds this element, else the element. */
  private targetIds(element: DiagramElement): string[] {
    const selected = [...this.host.canvas.selectedIds];
    return selected.includes(element.id) ? selected : [element.id];
  }

  /** The same for a line. */
  private edgeTargetIds(edge: DiagramEdge): string[] {
    const selected = [...this.host.canvas.selectedIds];
    return selected.includes(edge.id) ? selected : [edge.id];
  }

  /**
   * «Стиль»: the variants of the element's kind (ADR_20260927-7). A kind with
   * one style has nothing to choose — the line just says which it is; with
   * more, a row of cards, the base first. Choosing goes through the one
   * mechanism, `applyKindAndStyle`.
   */
  private styleSection(element: DiagramElement): HTMLElement {
    const lib = this.host.styles;
    const target: StyleTarget = isContainer(element) ? "container" : "block";
    const kind = element.type;
    const activeId = lib.blockStyleIdFor(element);
    const ids = this.targetIds(element);
    return this.variantsSection({
      activeId,
      base: lib.baseStyleOf(kind, target),
      variants: lib.stylesOf(kind, target).map((v) => ({
        id: v.id,
        name: v.name,
        description: v.style.description ?? "",
        preview: blockPreview(lib.resolveBlock(v.id), { container: target === "container" }),
      })),
      choose: (id) => this.host.applyKindAndStyle(ids, kind, id),
    });
  }

  /**
   * The style of a line: the variants of the relation's type, exactly as for a
   * block (ADR_20260930-2). Choosing goes through `applyRelationTypeAndStyle`.
   */
  private edgeStyleSection(edge: DiagramEdge): HTMLElement {
    const lib = this.host.styles;
    const ids = this.edgeTargetIds(edge);
    return this.variantsSection({
      activeId: lib.edgeStyleIdFor(edge),
      base: lib.baseStyleOf(edge.type, "edge"),
      variants: lib.stylesOf(edge.type, "edge").map((v) => ({
        id: v.id,
        name: v.name,
        description: v.style.description ?? "",
        preview: edgePreview(lib.resolveEdge(v.id)),
      })),
      choose: (id) => this.host.applyRelationTypeAndStyle(ids, edge.type, id),
    });
  }

  /** The «Стиль» header and, only when the type has more than one style, the cards of its variants. */
  private variantsSection(opts: {
    activeId: string | null;
    base: string | null;
    variants: ReadonlyArray<{ id: string; name: string; description: string; preview: SVGElement }>;
    choose: (id: string) => void;
  }): HTMLElement {
    const lib = this.host.styles;
    const p = i18n.d.panels.properties;
    const { activeId, base, variants } = opts;

    const head = el("div", { class: "field-row gap style-head" }, [
      el("span", { class: "field-label accent", text: `${p.styleTitle}:` }),
      el("span", {
        class: "style-head-name",
        text: activeId === null ? p.styleEmptyLib : lib.get(activeId)?.name ?? activeId,
        title: activeId ?? "",
      }),
      el("button", {
        class: "btn btn-small",
        text: p.styleEditBtn,
        disabled: activeId === null,
        on: { click: () => { if (activeId !== null) this.host.openStyleTab(activeId); } },
      }),
    ]);
    if (variants.length <= 1) return el("div", { class: "panel-section" }, [head]);

    const cards = el("div", { class: "style-variants" }, variants.map((v) =>
      el("button", {
        class: `style-variant${v.id === activeId ? " is-active" : ""}`,
        title: [v.id === base ? p.styleBaseTitle : "", v.description, v.id].filter(Boolean).join("\n"),
        on: { click: () => { if (v.id !== activeId) opts.choose(v.id); } },
      }, [
        el("span", { class: "style-variant-preview" }, [v.preview]),
        el("span", { class: "style-variant-name", text: variantLabel(v.name, v.id === base) }),
      ])));
    return el("div", { class: "panel-section" }, [head, cards]);
  }

  // --------------------------------------------------------------- override

  /**
   * «Выделить»: the placement's `override` (CONTRACT.md §11.6) — make this one
   * box stand out without a style made for it. The fields are the table in
   * `override.ts`, one control per input kind, each with a reset; the
   * placeholder is what the style gives. A palette of ready colours sets fill,
   * border and (on a container) header in one click.
   */
  private overrideSection(element: DiagramElement): HTMLElement {
    const p = i18n.d.panels.properties;
    const lib = this.host.styles;
    const styleId = lib.blockStyleIdFor(element);
    const style = styleId === null ? undefined : lib.effective(styleId);
    const container = isContainer(element);
    const fields = OVERRIDE_FIELDS.filter((f) => f.containerOnly !== true || container);

    const write = (change: (o: PlacementOverride | undefined) => PlacementOverride | undefined, reselect: boolean): void => {
      this.host.editField(() => {
        const next = change(element.override);
        if (next === undefined) delete element.override;
        else element.override = next;
      }, { rerender: true, reselect });
    };

    // The palette sets the colour fields of the table in one edit.
    const colourFields = fields.filter((f) => f.key === "fill" || f.key === "border.color" || f.key === "header.fill");
    const paint = (c: (typeof HIGHLIGHT_PALETTE)[number] | null): void => {
      write((current) => {
        let next = current;
        for (const f of colourFields) {
          const value = c === null ? undefined : f.key === "fill" ? c.fill : f.key === "border.color" ? c.border : c.header;
          next = withOverride(OVERRIDE_FIELDS, next, f, value);
        }
        return next;
      }, true);
    };
    const fillField = OVERRIDE_FIELDS.find((f) => f.key === "fill")!;
    const currentFill = overrideValue(element.override, fillField);
    const painted = colourFields.some((f) => overrideValue(element.override, f) !== undefined);
    const palette = el("div", { class: "highlight-palette", title: p.highlightPaletteTitle }, [
      ...HIGHLIGHT_PALETTE.map((c) => el("button", {
        class: `highlight-swatch${currentFill === c.fill ? " is-active" : ""}`,
        title: c.id,
        attrs: { style: `background: ${c.fill}; border-color: ${c.border};` },
        on: { click: () => paint(c) },
      })),
      el("button", {
        class: "btn-icon highlight-reset",
        text: p.highlightPaletteReset,
        title: p.highlightPaletteResetTitle,
        disabled: !painted,
        on: { click: () => paint(null) },
      }),
    ]);

    return el("div", { class: "panel-section" }, [
      el("div", { class: "field-row" }, [
        el("span", { class: "field-label accent", text: p.overrideTitle, title: p.overrideHint }),
      ]),
      palette,
      ...fields.map((f) => this.overrideRow(f, element.override, style, (value, reselect) =>
        write((current) => withOverride(OVERRIDE_FIELDS, current, f, value), reselect))),
    ]);
  }

  /** The same for a line: colour, width and dash of this one edge on this view. */
  private edgeOverrideSection(edge: DiagramEdge): HTMLElement {
    const p = i18n.d.panels.properties;
    const lib = this.host.styles;
    const styleId = lib.edgeStyleIdFor(edge);
    const style = styleId === null ? undefined : lib.effective(styleId);
    const colour = EDGE_OVERRIDE_FIELDS.find((f) => f.key === "line.color")!;

    const write = (change: (o: PlacementOverride | undefined) => PlacementOverride | undefined, reselect: boolean): void => {
      this.host.editField(() => {
        const next = change(edge.override);
        if (next === undefined) delete edge.override;
        else edge.override = next;
      }, { rerender: true, reselect });
    };
    const paint = (color: string | undefined): void =>
      write((current) => withOverride(EDGE_OVERRIDE_FIELDS, current, colour, color), true);
    const current = overrideValue(edge.override, colour);

    const palette = el("div", { class: "highlight-palette", title: p.highlightPaletteTitle }, [
      ...LINE_PALETTE.map((c) => el("button", {
        class: `highlight-swatch${current === c.color ? " is-active" : ""}`,
        title: c.id,
        attrs: { style: `background: ${c.color}; border-color: ${c.color};` },
        on: { click: () => paint(c.color) },
      })),
      el("button", {
        class: "btn-icon highlight-reset",
        text: p.highlightPaletteReset,
        title: p.highlightPaletteResetTitle,
        disabled: current === undefined,
        on: { click: () => paint(undefined) },
      }),
    ]);

    return el("div", { class: "panel-section" }, [
      el("div", { class: "field-row" }, [
        el("span", { class: "field-label accent", text: p.overrideTitle, title: p.edgeOverrideHint }),
      ]),
      palette,
      ...EDGE_OVERRIDE_FIELDS.map((f) => this.overrideRow(f, edge.override, style, (value, reselect) =>
        write((c) => withOverride(EDGE_OVERRIDE_FIELDS, c, f, value), reselect))),
    ]);
  }

  /**
   * One field of an override table: the control of its input kind and a reset.
   * `set(undefined)` clears the field; the placeholder is what the style gives.
   */
  private overrideRow(
    f: OverrideField,
    override: PlacementOverride | undefined,
    style: WireStyle | undefined,
    set: (value: string | number | undefined, reselect: boolean) => void,
  ): HTMLElement {
    const p = i18n.d.panels.properties;
    const label = p.overrideFields[f.key] ?? f.key;
    const value = overrideValue(override, f);
    const given = styleValue(style, f);
    const inherited = given === undefined ? "" : String(given);

    let control: HTMLElement;
    if (f.input === "color") {
      control = colorField(label, value === undefined ? "" : String(value), inherited, (next) =>
        set(next.trim() === "" ? undefined : next, false));
    } else if (f.input === "width") {
      control = field(label, el("input", {
        type: "number",
        value: value === undefined ? "" : String(value),
        placeholder: inherited,
        attrs: { min: "0.5", step: "0.5" },
        on: {
          input: (e) => {
            const raw = (e.target as HTMLInputElement).value.trim();
            const parsed = Number(raw);
            set(raw === "" || !Number.isFinite(parsed) || parsed <= 0 ? undefined : parsed, false);
          },
        },
      }));
    } else if (f.input === "glyph") {
      // An icon is a key of the registry (ADR_20260929): chosen from its set, never typed.
      const shown = typeof value === "string" ? value : undefined;
      const current = iconElByKey(shown ?? (inherited || undefined), "override-icon-current");
      const name = el("span", { class: `mono override-icon-name${shown === undefined ? " is-inherited" : ""}`, text: shown ?? inherited });
      control = el("div", { class: "field" }, [
        el("span", { class: "field-label", text: label }),
        el("details", { class: "override-icon" }, [
          el("summary", {}, [current, name, el("span", { class: "muted", text: p.overrideIconPick })]),
          iconPicker({
            value: shown,
            ...(inherited === "" ? {} : { inherited }),
            allowClear: true,
            onChange: (next) => set(next, true),
          }),
        ]),
      ]);
    } else {
      control = field(label, el("input", {
        type: "text",
        class: "mono",
        value: value === undefined ? "" : String(value),
        placeholder: inherited || p.overrideDashPlaceholder,
        attrs: { spellcheck: "false" },
        on: {
          input: (e) => {
            const next = (e.target as HTMLInputElement).value;
            set(next.trim() === "" ? undefined : next, false);
          },
        },
      }));
    }
    control.style.flex = "1";
    control.style.minWidth = "0";
    return el("div", { class: "field-row gap", attrs: { style: "align-items: flex-end;" } }, [
      control,
      el("button", {
        class: "btn-icon",
        title: p.overrideReset,
        disabled: value === undefined,
        on: { click: () => set(undefined, true) },
      }, [iconEl("restore")]),
    ]);
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
          this.host.applyTemplate([element.id], value === "" ? null : value);
        }),
      ),
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
        el("div", { attrs: { style: "display: flex; align-items: center; justify-content: space-between; margin-bottom: calc(2px * var(--ui-space));" } }, [
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
      el("div", { class: "field", attrs: { style: "margin-top: calc(4px * var(--ui-space));" } }, [
        el("button", {
          class: `btn full ${hasDoc ? "btn-secondary" : ""}`,
          attrs: { style: "display: flex; align-items: center; justify-content: center; gap: calc(6px * var(--ui-space));" },
          on: {
            click: () => this.host.openDocEditor(element.id, isContainer(element) ? "zone" : "node"),
          },
        }, [iconEl("doc"), i18n.d.dialogs.docEditor.openEditorBtn, hasDoc ? iconEl("check") : null]),
      ]),
    ]);
  }

  /** The entity's realizations: read-only, one row and one code button each. */
  private realizationsField(element: DiagramElement): HTMLElement {
    const p = i18n.d.panels.properties;
    const code = realizationsOf(element.metadata);
    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: p.realizationsTitle }),
      code.length === 0
        ? el("span", { class: "muted", text: p.realizationsEmpty })
        : el("div", { class: "code-realizations", attrs: { style: "display: flex; flex-direction: column; gap: calc(4px * var(--ui-space));" } },
            code.map((r) => this.codeRow(r, [r.symbol ?? "", refOf(r)], element.label))),
    ]);
  }

  /**
   * One realization or evidence entry: [language tag: a code button when there
   * is a file to open] then details, dimmed and struck through when missing.
   */
  private codeRow(r: { lang?: string; ref?: string; status?: string }, details: string[], label: string): HTMLElement {
    const p = i18n.d.panels.properties;
    const missing = r.status === "missing";
    const ref = refOf(r);
    const canOpen = ref !== "" && SourceCodeService.isFileAvailable(ref) !== false;
    const tag = langTag(r) || "·";
    const strike = missing ? " text-decoration: line-through;" : "";
    return el("div", {
      class: `field-row code-realization${missing ? " is-missing" : ""}`,
      title: missing ? `${ref} — ${p.codeMissing}` : ref,
      attrs: { style: `justify-content: flex-start; gap: calc(6px * var(--ui-space));${missing ? " opacity: 0.55;" : ""}` },
    }, [
      canOpen
        ? el("button", {
            class: "btn btn-secondary btn-small",
            title: `${p.viewCodeTitle}: ${ref}`,
            text: tag,
            attrs: { style: `flex-shrink: 0;${strike}` },
            on: { click: () => this.host.openCodeViewer?.(ref, label) },
          })
        : el("span", { class: "chip chip-lang", text: tag, attrs: { style: `flex-shrink: 0;${strike}` } }),
      ...details.filter(Boolean).map((d) => el("span", {
        class: "mono muted",
        text: d,
        attrs: { style: `min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;${strike}` },
      })),
      missing ? el("span", { class: "muted", text: p.codeMissing }) : null,
    ]);
  }

  /** A relation's evidence, one row per language: its file (when it has one) and the member it comes through. */
  private evidenceField(edge: DiagramEdge): HTMLElement | null {
    const doc = this.host.canvas.model;
    const rel = doc?.relations.find((r) => r.id === edge.id);
    const evidence = evidenceOf(rel);
    if (evidence.length === 0 || doc === null) return null;
    const label = `${doc.entityName(edge.from)} → ${doc.entityName(edge.to)}`;
    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: i18n.d.panels.properties.evidenceTitle }),
      el("div", { class: "code-realizations", attrs: { style: "display: flex; flex-direction: column; gap: calc(4px * var(--ui-space));" } },
        evidence.map((e) => this.codeRow(e, [e.via?.member ?? "", e.via?.text ?? "", e.symbol ?? "", refOf(e)], label))),
    ]);
  }

  private deleteButton(container: boolean): HTMLElement {
    return el("div", { class: "panel-section" }, [
      el("button", {
        class: "btn btn-danger full",
        text: i18n.format(i18n.d.panels.properties.deleteItemBtn, {
          item: container ? i18n.d.panels.properties.itemContainer : i18n.d.panels.properties.itemBlock,
        }),
        on: { click: () => this.host.deleteSelection() },
      }),
    ]);
  }
}

function formatGeometry(element: DiagramElement): string {
  return `X:${Math.round(element.x)} Y:${Math.round(element.y)} (${Math.round(element.width)}×${Math.round(element.height)})`;
}
