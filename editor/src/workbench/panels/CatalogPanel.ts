import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import type { ProjectEntry } from "../../editor/io/types.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";
import {
  openEditProjectDialog,
  openEditViewDialog,
  openNewProjectDialog,
  openNewViewDialog,
} from "../dialogs/WorkspaceDialogs.js";

export class CatalogPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly catalogSlot: HTMLElement;
  private readonly customCatalogSlot: HTMLElement;
  private readonly customSection: HTMLElement;
  private readonly customLabel: HTMLElement;
  private readonly projectLabel: HTMLElement;
  private readonly addProjectBtn: HTMLElement;
  private readonly hintDiv: HTMLElement;
  private readonly openJsonBtn: HTMLElement;

  constructor(private readonly editor: DiagramEditor) {
    this.catalogSlot = el("div");
    this.customCatalogSlot = el("div");
    this.customLabel = el("div", { class: "section-label sidebar-label", text: i18n.d.panels.catalog.customSection });
    this.customSection = el("div", { attrs: { hidden: "true" } }, [
      this.customLabel,
      this.customCatalogSlot,
    ]);

    this.projectLabel = el("div", { class: "section-label sidebar-label", text: i18n.d.panels.catalog.projectCatalog });
    this.addProjectBtn = el("button", {
      class: "btn btn-small",
      text: i18n.d.panels.catalog.addProject,
      title: i18n.d.commands.newProject.desc,
      on: { click: () => openNewProjectDialog(editor) },
    });
    const scroll = el("div", { class: "sidebar-scroll", attrs: { style: "flex: 1; overflow-y: auto; padding: calc(8px * var(--ui-space));" } }, [
      el("div", { class: "catalog-toolbar" }, [this.projectLabel, this.addProjectBtn]),
      this.catalogSlot,
      this.customSection,
    ]);

    this.hintDiv = el("div", { class: "hint sidebar-label", attrs: { style: "padding: calc(8px * var(--ui-space)); border-top: 1px solid var(--border);" } }, [
      el("div", { text: i18n.d.panels.catalog.hintDragZone }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseZone }),
    ]);

    this.openJsonBtn = el("button", {
      class: "btn full",
      text: i18n.d.panels.catalog.openJsonBtn,
      on: { click: () => editor.openFile() },
    });

    const foot = el("div", { class: "sidebar-foot", attrs: { style: "padding: calc(8px * var(--ui-space)); border-top: 1px solid var(--border);" } }, [
      this.openJsonBtn,
    ]);

    this.element = el(
      "aside",
      {
        class: "sidebar",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [scroll, this.hintDiv, foot],
    );

    i18n.onLanguageChange(() => {
      this.updateLabels();
      this.render();
    });
    editor.workspaceEvents.on("change", () => this.render());
  }

  private updateLabels(): void {
    this.customLabel.textContent = i18n.d.panels.catalog.customSection;
    this.projectLabel.textContent = i18n.d.panels.catalog.projectCatalog;
    this.addProjectBtn.textContent = i18n.d.panels.catalog.addProject;
    this.openJsonBtn.textContent = i18n.d.panels.catalog.openJsonBtn;
    replaceChildren(this.hintDiv,
      el("div", { text: i18n.d.panels.catalog.hintDragZone }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseZone }),
    );
  }

  init(_params: GroupPanelPartInitParameters): void {
    this.render();
  }

  onShow(): void {
    this.render();
  }

  render(): void {
    const { projects } = this.editor.workspace;
    if (projects.length === 0) {
      replaceChildren(
        this.catalogSlot,
        el("div", { class: "catalog-empty", text: i18n.d.panels.catalog.empty }),
      );
      return;
    }
    const current = this.editor.currentView?.file;
    replaceChildren(this.catalogSlot, ...projects.map((project) => this.renderProject(project, current)));
  }

  private renderProject(project: ProjectEntry, current: string | undefined): HTMLElement {
    const t = i18n.d.panels.catalog;
    const tile = (icon: string, theme: string | undefined): HTMLElement =>
      el("span", { class: `catalog-icon${theme ? ` theme-${theme}` : ""}`, text: icon });
    const editBtn = (title: string, onClick: () => void): HTMLElement =>
      el("button", { class: "btn-icon catalog-edit", text: "✎", title, on: { click: onClick } });

    const head = el("div", { class: "catalog-project-head" }, [
      el("div", { class: `catalog-item${project.error ? " is-broken" : ""}`, title: project.error ?? project.id }, [
        tile(project.icon ?? "📁", project.theme),
        el("span", { class: "catalog-text sidebar-label" }, [
          el("span", { class: "catalog-title", text: project.title }),
          el("span", { class: "catalog-subtitle", text: project.error ?? project.subtitle ?? "" }),
        ]),
      ]),
      project.error ? null : editBtn(t.editProject, () => openEditProjectDialog(this.editor, project)),
      el("button", {
        class: "btn-icon",
        text: "＋",
        title: t.addView,
        disabled: Boolean(project.error),
        on: { click: () => openNewViewDialog(this.editor, project.id) },
      }),
    ]);

    const views = project.views.map((view) =>
      el("div", { class: "catalog-row" }, [
        el(
          "button",
          {
            class: `catalog-item${view.file === current ? " is-active" : ""}${view.error ? " is-broken" : ""}`,
            title: view.error ?? view.file,
            dataset: { file: view.file },
            on: { click: (e: MouseEvent) => this.editor.openView(view, e) },
          },
          [
            tile(view.icon ?? "🗺️", view.theme ?? project.theme),
            el("span", { class: "catalog-text sidebar-label" }, [
              el("span", { class: "catalog-title", text: this.editor.viewName(view) }),
              el("span", { class: "catalog-subtitle", text: view.error ?? view.id }),
            ]),
          ],
        ),
        view.error ? null : editBtn(t.editView, () => openEditViewDialog(this.editor, view)),
      ]),
    );

    return el("div", { class: "catalog-project" }, [
      head,
      el("div", { class: "catalog-views" },
        views.length > 0 ? views : [el("div", { class: "catalog-none sidebar-label", text: t.noViews })],
      ),
    ]);
  }
}
