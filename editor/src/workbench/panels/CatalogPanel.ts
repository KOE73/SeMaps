import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import type { ProjectEntry } from "../../editor/io/types.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";
import { iconEl } from "../../ui/icons.js";
import { iconByKey } from "../../ui/kindIcons.js";
import {
  openEditProjectDialog,
  openEditViewDialog,
  openNewProjectDialog,
  openNewViewDialog,
} from "../dialogs/WorkspaceDialogs.js";

export class CatalogPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly catalogSlot: HTMLElement;
  private readonly projectLabel: HTMLElement;
  private readonly addProjectBtn: HTMLElement;
  private readonly hintDiv: HTMLElement;

  constructor(private readonly editor: DiagramEditor) {
    this.catalogSlot = el("div");

    this.projectLabel = el("div", { class: "section-label sidebar-label", text: i18n.d.panels.catalog.projectCatalog });
    this.addProjectBtn = el("button", {
      class: "btn btn-small",
      title: i18n.d.commands.newProject.desc,
      on: { click: () => openNewProjectDialog(editor) },
    }, [iconEl("plus"), i18n.d.panels.catalog.addProject]);
    const scroll = el("div", { class: "sidebar-scroll", attrs: { style: "flex: 1; overflow-y: auto; padding: calc(8px * var(--ui-space));" } }, [
      el("div", { class: "catalog-toolbar" }, [this.projectLabel, this.addProjectBtn]),
      this.catalogSlot,
    ]);

    this.hintDiv = el("div", { class: "hint sidebar-label", attrs: { style: "padding: calc(8px * var(--ui-space)); border-top: 1px solid var(--border);" } }, [
      el("div", { text: i18n.d.panels.catalog.hintDragContainer }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseContainer }),
    ]);

    this.element = el(
      "aside",
      {
        class: "sidebar",
        attrs: { style: "width: 100%; height: 100%; display: flex; flex-direction: column; overflow: hidden; background: var(--panel);" },
      },
      [scroll, this.hintDiv],
    );

    i18n.onLanguageChange(() => {
      this.updateLabels();
      this.render();
    });
    editor.workspaceEvents.on("change", () => this.render());
  }

  private updateLabels(): void {
    this.projectLabel.textContent = i18n.d.panels.catalog.projectCatalog;
    replaceChildren(this.addProjectBtn, iconEl("plus"), i18n.d.panels.catalog.addProject);
    replaceChildren(this.hintDiv,
      el("div", { text: i18n.d.panels.catalog.hintDragContainer }),
      el("div", { text: i18n.d.panels.catalog.hintDragBlock }),
      el("div", { text: i18n.d.panels.catalog.hintCollapseContainer }),
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
    const dirty=this.editor.dirtyForProject(project.id);
    const authors=(refs: readonly {author:string}[])=>[...new Set(refs.map((r)=>r.author==="human"?"вы":"агент"))].join(", ");
    // The stored icon is a registry key; an unknown one draws the fallback icon.
    const tile = (icon: string, theme: string | undefined): HTMLElement => {
      const span = el("span", { class: `catalog-icon${theme ? ` theme-${theme}` : ""}` });
      span.innerHTML = iconByKey(icon);
      return span;
    };
    const editBtn = (title: string, onClick: () => void): HTMLElement =>
      el("button", { class: "btn-icon catalog-edit", title, on: { click: onClick } }, [iconEl("pencil")]);

    const head = el("div", { class: "catalog-project-head" }, [
      el("div", { class: `catalog-item${project.error ? " is-broken" : ""}`, title: project.error ?? project.id }, [
        tile(project.icon ?? "folder", project.theme),
        el("span", { class: "catalog-text sidebar-label" }, [
          el("span", { class: "catalog-title", text: project.title }),
          el("span", { class: "catalog-subtitle", text: project.error ?? project.subtitle ?? "" }),
        ]),
      ]),
      project.error ? null : editBtn(t.editProject, () => openEditProjectDialog(this.editor, project)),
      el("button", {
        class: "btn-icon",
        title: t.addView,
        disabled: Boolean(project.error),
        on: { click: () => openNewViewDialog(this.editor, project.id) },
      }, [iconEl("plus")]),
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
            tile(view.icon ?? "map", view.theme ?? project.theme),
            el("span", { class: "catalog-text sidebar-label" }, [
              el("span", { class: "catalog-title", text: this.editor.viewName(view) }),
              el("span", { class: "catalog-subtitle", text: view.error ?? view.id }),
            ]),
            (dirty?.views[view.id]?.length ?? 0)>0 ? el("span",{class:"catalog-dirty",title:`Несохранено: ${authors(dirty!.views[view.id]!)}`},[iconEl("circleDot")]) : null,
          ],
        ),
        view.error ? null : el("button", { class: "btn-icon catalog-edit catalog-eye", title: t.showChanges,
          on: { click: () => this.editor.showChanges(view) } }, [iconEl("eye")]),
        view.error ? null : editBtn(t.editView, () => openEditViewDialog(this.editor, view)),
        el("button", { class: "btn-icon catalog-edit", title: t.deleteView, on: { click: () => {
          const caption = this.editor.viewName(view);
          if (!window.confirm(t.confirmDeleteView.replace("{caption}", caption).replace("{id}", view.id))) return;
          this.editor.deleteView(view).catch((err: Error) => this.editor.notify(err.message));
        } } }, [iconEl("trash")]),
      ]),
    );

    return el("div", { class: "catalog-project" }, [
      head,
      (dirty?.registry.length ?? 0)>0 ? el("div",{class:"catalog-registry-dirty",text:`Реестр: изменено ${dirty!.registry.length} (${authors(dirty!.registry)})`}) : null,
      el("div", { class: "catalog-views" },
        views.length > 0 ? views : [el("div", { class: "catalog-none sidebar-label", text: t.noViews })],
      ),
    ]);
  }
}
