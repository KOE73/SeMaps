import { el, replaceChildren } from "../util/dom.js";
import type { DiagramEditor } from "./DiagramEditor.js";
import { i18n } from "../workbench/i18n/I18nService.js";

export class FiltersPanel {
  private tagQuery: string = "";

  constructor(
    private readonly container: HTMLElement,
    private readonly editor: DiagramEditor
  ) {}

  render(): void {
    const canvas = this.editor.canvas;
    const doc = canvas.model;
    if (!doc) {
      replaceChildren(this.container, el("div", { class: "inspector-empty", text: i18n.d.panels.filters.empty }));
      return;
    }

    const sections: HTMLElement[] = [];

    // -------------------------------------------------------- 1. Views / Ракурсы
    const views = doc.views || [];
    const activeView = canvas.activeView;

    const viewsList = el("div", {
      class: "filters-views-list",
      attrs: { style: "display: flex; flex-direction: column; gap: 4px; padding: 6px 10px 10px;" },
    }, [
      el(
        "button",
        {
          class: `btn full ${activeView === null || activeView === "all" ? "btn-primary is-active" : ""}`,
          attrs: { style: "text-align: left; justify-content: flex-start; gap: 8px;" },
          on: {
            click: () => {
              canvas.setView(null);
              this.render();
            },
          },
        },
        [
          el("span", { text: "🏛" }),
          el("span", { text: i18n.d.panels.filters.defaultView }),
        ]
      ),
      ...views.map((v) => {
        const isActive = activeView === v.id;
        return el(
          "button",
          {
            class: `btn full ${isActive ? "btn-primary is-active" : ""}`,
            attrs: { style: "text-align: left; justify-content: flex-start; gap: 8px;", title: v.description || "" },
            on: {
              click: () => {
                canvas.setView(v.id);
                this.render();
              },
            },
          },
          [
            el("span", { text: v.icon || "🔹" }),
            el("span", { text: v.name || v.id }),
          ]
        );
      }),
    ]);

    const viewsSection = el("div", {
      class: "filters-section",
      attrs: { style: "border-bottom: 1px solid var(--border);" },
    }, [
      el("div", {
        attrs: { style: "padding: 10px 10px 4px; display: flex; justify-content: space-between; align-items: center;" },
      }, [
        el("span", { class: "section-label", text: i18n.d.panels.filters.perspectivesHeader }),
      ]),
      viewsList,
    ]);
    sections.push(viewsSection);

    // -------------------------------------------------------- 2. Tags / Теги
    const tags = canvas.tagsInUse();
    const activeTags = canvas.highlightTags;
    const styles = canvas.styles;

    const counts = new Map<string, number>();
    for (const element of doc.elements()) {
      const elTags = styles.tagsOf(styles.blockStyleIdFor(element));
      for (const t of elTags) {
        counts.set(t, (counts.get(t) || 0) + 1);
      }
    }
    for (const edge of doc.edges) {
      const edgeTags = styles.tagsOf(styles.edgeStyleIdFor(edge));
      for (const t of edgeTags) {
        counts.set(t, (counts.get(t) || 0) + 1);
      }
    }

    const q = this.tagQuery.toLowerCase();
    const filteredTags = this.tagQuery
      ? tags.filter((t) => String(t ?? "").toLowerCase().includes(q))
      : tags;

    const tagListContainer = el("div", {
      class: "filters-tag-list",
      attrs: { style: "padding: 6px 10px 10px; display: flex; flex-wrap: wrap; gap: 6px; max-height: 240px; overflow-y: auto;" },
    });

    const tagsHead = el("div", {
      attrs: { style: "padding: 10px 10px 4px; display: flex; flex-direction: column; gap: 6px;" },
    }, [
      el("div", { attrs: { style: "display: flex; justify-content: space-between; align-items: center;" } }, [
        el("span", { class: "section-label", text: i18n.d.panels.filters.tagsHeader }),
        activeTags.size > 0
          ? el("button", {
              class: "btn btn-small",
              attrs: { style: "padding: 2px 8px; font-size: 11px;" },
              text: `${i18n.d.common.undo} (${activeTags.size})`,
              on: {
                click: () => {
                  canvas.clearHighlightTags();
                  this.render();
                },
              },
            })
          : null,
      ]),
      tags.length > 5
        ? el("input", {
            type: "text",
            class: "input-strong",
            placeholder: i18n.d.panels.styles.filterPlaceholder,
            value: this.tagQuery,
            on: {
              input: (e: Event) => {
                this.tagQuery = (e.target as HTMLInputElement).value;
                this.renderTagList(tagListContainer, filteredTags, counts, activeTags);
              },
            },
          })
        : null,
    ]);

    this.renderTagList(tagListContainer, filteredTags, counts, activeTags);

    const tagsSection = el("div", {
      class: "filters-section",
      attrs: { style: "border-bottom: 1px solid var(--border);" },
    }, [
      tagsHead,
      tagListContainer,
    ]);
    sections.push(tagsSection);

    // -------------------------------------------------------- 3. Relations / Связи
    const relationsSection = el("div", {
      class: "filters-section",
      attrs: { style: "padding: 10px;" },
    }, [
      el("div", { attrs: { style: "margin-bottom: 6px;" } }, [
        el("span", { class: "section-label", text: i18n.d.panels.filters.connectionsHeader }),
      ]),
      el("label", { class: "check", attrs: { style: "display: flex; align-items: center; gap: 8px; cursor: pointer;" } }, [
        el("input", {
          type: "checkbox",
          attrs: !canvas.isEdgeFamilyHidden("structure") ? { checked: "checked" } : {},
          on: {
            change: (e: Event) => {
              const checked = (e.target as HTMLInputElement).checked;
              canvas.setEdgeFamilyHidden("structure", !checked);
            },
          },
        }),
        el("span", { text: `🔗 ${i18n.d.panels.relations.implementsType}, ${i18n.d.panels.relations.extendsType}, ${i18n.d.panels.relations.composesType}` }),
      ]),
    ]);
    sections.push(relationsSection);

    replaceChildren(this.container, ...sections);
  }

  private renderTagList(
    container: HTMLElement,
    tags: string[],
    counts: Map<string, number>,
    activeTags: ReadonlySet<string>
  ): void {
    const canvas = this.editor.canvas;

    if (tags.length === 0) {
      replaceChildren(
        container,
        el("div", { class: "inspector-empty", attrs: { style: "width: 100%; font-size: 12px;" }, text: this.tagQuery ? "Теги не найдены" : "В этой схеме нет тегов" })
      );
      return;
    }

    replaceChildren(
      container,
      ...tags.map((tag) => {
        const isActive = activeTags.has(tag);
        const count = counts.get(tag) || 0;
        return el(
          "button",
          {
            class: `tag-pill ${isActive ? "is-active" : ""}`,
            attrs: {
              style: `
                display: inline-flex;
                align-items: center;
                gap: 6px;
                padding: 4px 9px;
                border-radius: 12px;
                font-family: var(--mono, monospace);
                font-size: 11px;
                font-weight: 500;
                cursor: pointer;
                transition: all 0.15s ease;
                border: 1px solid ${isActive ? "var(--accent, #d97706)" : "var(--border, #e2e8f0)"};
                background: ${isActive ? "var(--accent, #d97706)" : "var(--panel-alt, #f8fafc)"};
                color: ${isActive ? "#ffffff" : "var(--text, #1e293b)"};
              `,
            },
            dataset: { tagId: tag },
            on: {
              click: () => {
                canvas.toggleHighlightTag(tag);
                this.render();
              },
            },
          },
          [
            el("span", { text: `#${tag}` }),
            el("span", {
              attrs: {
                style: `
                  font-size: 10px;
                  opacity: 0.85;
                  padding: 1px 4px;
                  border-radius: 6px;
                  background: ${isActive ? "rgba(255,255,255,0.25)" : "var(--bg, #f1f5f9)"};
                `,
              },
              text: String(count),
            }),
          ]
        );
      })
    );
  }
}
