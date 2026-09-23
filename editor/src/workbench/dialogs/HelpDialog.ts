import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";
import { openModal } from "./Modal.js";

function file(icon: string, name: string, note: string, extra = ""): HTMLElement {
  return el("div", { class: `help-file ${extra}` }, [
    el("span", { class: "help-file-name", text: `${icon} ${name}` }),
    el("span", { class: "help-file-note", text: note }),
  ]);
}

function box(kind: string, head: string, note: string | null, children: HTMLElement[] = []): HTMLElement {
  return el("div", { class: `help-box ${kind}` }, [
    el("div", { class: "help-box-head", text: head }),
    note ? el("div", { class: "help-box-note", text: note }) : null,
    ...children,
  ]);
}

function arrow(label: string): HTMLElement {
  return el("div", { class: "help-arrow" }, [el("span", { text: label }), el("span", { class: "help-arrow-head", text: "▼" })]);
}

/** Folders drawn as nested boxes: what lives where, and what each file is for. */
export function openHelpDialog(): void {
  const h = i18n.d.help;

  const project = box("project", "📁 projects/shop/", h.projectsNote, [
    file("📄", "project.json", h.projectJson),
    file("🧱", "entities.json", h.entitiesJson, "is-coderef"),
    file("🔗", "relations.json", h.relationsJson),
    file("🔤", "text.ru.json · text.en.json", h.textJson),
    box("views", "🗺️ views/", h.viewsNote, [
      el("div", { class: "help-cards" }, [
        el("div", { class: "help-card", text: "v_main.view.json" }),
        el("div", { class: "help-card", text: "v_layers.view.json" }),
        el("div", { class: "help-card", text: "v_flow.view.json" }),
      ]),
    ]),
  ]);

  const workspace = box("workspace", `📂 ${h.workspace}`, h.workspaceNote, [
    box("shared", `🎨 ${h.shared}`, h.sharedNote),
    el("div", { class: "help-projects" }, [project, box("project ghost", "📁 projects/billing/", h.anotherProject)]),
  ]);

  const code = box("code", `💻 ${h.code}`, h.codeNote, [
    el("div", { class: "help-coderef", text: `⟵ ${h.codeRef}` }),
  ]);

  const body = el("div", { class: "help-map" }, [
    el("div", { class: "help-top" }, [
      box("semaps", `⚙️ ${h.projectFile}`, h.projectFileNote),
    ]),
    el("div", { class: "help-grid" }, [
      arrow("workspace: docs/diagrams"),
      arrow("source_root: ."),
      workspace,
      code,
    ]),
    el("div", { class: "help-bottom" }, [
      box("legend", `🧭 ${h.inEditor}`, null, [
        el("ul", {}, [h.mapCatalog, h.mapBase, h.mapRelations, h.mapStyles].map((t) => el("li", { text: t }))),
      ]),
      box("legend", `💡 ${h.ideasTitle}`, null, [
        el("ul", {}, [h.idea1, h.idea2, h.idea3].map((t) => el("li", { text: t }))),
      ]),
    ]),
  ]);

  openModal({ title: h.title, body, width: "1040px" });
}
