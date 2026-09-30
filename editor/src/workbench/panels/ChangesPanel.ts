import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import type { ChangedRef } from "../../editor/io/HostModelStore.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";
import { iconEl, type IconName } from "../../ui/icons.js";

/** What a changed object looks like in the list: keys of the one UI icon registry. */
const KIND_ICONS: Record<string, IconName> = {
  placement: "box", view: "map", entity: "category", relation: "link", relationType: "listDetails", text: "pencil", project: "folder",
};

/**
 * What is unsaved in the open view's project, live (ADR_20260926-2): first what
 * is drawn — placements (blocks and containers), view properties, the open view
 * first — then the registry. A row leads to its object.
 */
export class ChangesPanel implements IContentRenderer {
  readonly element: HTMLElement;
  private readonly body: HTMLElement;

  constructor(private readonly editor: DiagramEditor) {
    this.body = el("div", { class: "changes-body" });
    this.element = el("aside", { class: "sidebar changes-panel" }, [this.body]);
    editor.workspaceEvents.on("change", () => this.render());
    i18n.onLanguageChange(() => this.render());
  }

  init(_params: GroupPanelPartInitParameters): void { this.render(); }
  onShow(): void { this.render(); }

  private render(): void {
    const t = i18n.d.panels.changes;
    const view = this.editor.currentView;
    const project = view ? this.editor.projectOf(view) : undefined;
    if (!view || !project) { replaceChildren(this.body, el("div", { class: "changes-empty", text: t.noView })); return; }
    const dirty = this.editor.dirtyForProject(project.id);
    const filter = this.editor.changesFilter;
    const filterView = filter ? project.views.find((v) => v.id === filter) : undefined;

    const head = el("div", { class: "changes-head" }, [
      el("div", { class: "changes-title", text: filterView
        ? i18n.format(t.onlyView, { view: this.editor.viewName(filterView) })
        : i18n.format(t.allOf, { project: project.title }) }),
      filterView ? el("button", { class: "btn btn-small", text: t.showAll, on: { click: () => this.editor.showChanges(null) } }) : null,
    ]);

    const viewIds = Object.keys(dirty?.views ?? {})
      .filter((id) => (dirty!.views[id]?.length ?? 0) > 0 && (!filter || id === filter))
      .sort((a, b) => Number(b === view.id) - Number(a === view.id));
    const groups: HTMLElement[] = viewIds.map((id) => {
      const entry = project.views.find((v) => v.id === id);
      return this.group("map", entry ? this.editor.viewName(entry) : id, dirty!.views[id]!, project.id);
    });
    // Narrowed to a view, the registry part keeps what is drawn on it and the view's own texts.
    const drawn = new Set((filter ? dirty?.views[filter] ?? [] : []).map((r) => r.id));
    const registry = (dirty?.registry ?? []).filter((r) => !filter || drawn.has(r.id) || r.id === filter);
    if (registry.length > 0) groups.push(this.group("books", t.registry, registry, project.id));

    replaceChildren(this.body, head, ...(groups.length > 0 ? groups : [el("div", { class: "changes-empty", text: t.none })]));
  }

  private group(glyph: IconName, title: string, refs: readonly ChangedRef[], project: string): HTMLElement {
    const t = i18n.d.panels.changes;
    const author = (a: string): string => a === "human" ? t.you : a === "sync" ? t.sync : t.agent;
    const kind = (k: string): string => (t.kinds as Record<string, string>)[k] ?? k;
    return el("section", { class: "changes-group" }, [
      el("div", { class: "section-label sidebar-label" }, [iconEl(glyph), el("span", { text: `${title} · ${refs.length}` })]),
      ...refs.map((ref) => el("button", {
        class: `changes-row author-${ref.author === "human" ? "human" : "agent"}`,
        title: `${ref.kind} ${ref.id}${ref.lang ? ` (${ref.lang})` : ""}`,
        on: { click: () => void this.editor.revealChange(ref) },
      }, [
        iconEl(KIND_ICONS[ref.kind] ?? "circleDot", "changes-icon"),
        el("span", { class: "changes-text" }, [
          el("span", { class: "changes-name", text: this.editor.describeChange(project, ref) }),
          el("span", { class: "changes-kind", text: `${kind(ref.kind)}${ref.lang ? ` · ${ref.lang}` : ""}` }),
        ]),
        el("span", { class: "changes-author", text: author(ref.author) }),
      ])),
    ]);
  }
}
