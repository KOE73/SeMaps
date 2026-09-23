import { el } from "../../util/dom.js";
import { PROJECT_ID, VIEW_ID } from "../../editor/io/WorkspaceStore.js";
import type { ProjectEntry, ViewEntry } from "../../editor/io/types.js";
import type { DiagramEditorFacade } from "../commands/types.js";
import { i18n } from "../i18n/I18nService.js";
import { openModal } from "./Modal.js";

const AXES = ["axis_layer", "axis_subsystem", "axis_process", "axis_security_zone", "axis_deployment"];

const ICONS = [
  "📁", "🗺️", "🧩", "🧠", "⚙️", "🔌", "🗄️", "🌐", "☁️", "🔒", "🛡️", "🧪",
  "📦", "🚀", "👤", "💬", "📊", "🧱", "🔗", "🏭", "🛒", "💳", "📨", "🤖",
];

/** Tints for the icon tile in the catalogue; `.catalog-icon.theme-<id>` in editor.css. */
export const THEMES = ["blue", "green", "teal", "orange", "red", "pink", "purple", "slate"];

const same = (a: string, b: string): boolean => a.trim().toLowerCase() === b.trim().toLowerCase();

function textInput(value = "", placeholder = ""): HTMLInputElement {
  return el("input", { type: "text", value, placeholder, attrs: { style: "width: 100%; box-sizing: border-box;" } });
}

/** One labelled field: the control, a grey hint, and a red line for what is wrong with it. */
interface Row {
  readonly label: string;
  readonly control: HTMLElement;
  readonly hint?: string;
  /** A message when the value is not acceptable, else null. */
  readonly check?: () => string | null;
}

/**
 * Every field is checked on every keystroke. A field shows its message once it
 * has been touched — except a clash with an existing name, which is shown at
 * once — and the submit button stays disabled while anything is wrong. A
 * failure from the server lands on the error line and keeps the dialog open.
 */
function formDialog(title: string, submitLabel: string, rows: Row[], submit: () => Promise<void>): void {
  const t = i18n.d.workspaceDialogs;
  const touched = new Set<Row>();
  const errors = new Map<Row, HTMLElement>();

  const fields = rows.map((row) => {
    const error = el("span", { class: "form-error" });
    errors.set(row, error);
    const mark = (): void => {
      touched.add(row);
      validate();
    };
    row.control.addEventListener("input", mark);
    row.control.addEventListener("change", mark);
    return el("div", { class: "field" }, [
      el("span", { class: "field-label", text: row.label }),
      row.control,
      row.hint ? el("span", { class: "form-hint", text: row.hint }) : null,
      error,
    ]);
  });

  const failure = el("div", { class: "form-error" });
  // The hidden submit button is what makes Enter in any field submit.
  const body = el("form", { class: "form-dialog" }, [...fields, failure, el("button", { type: "submit", hidden: true })]);
  const ok = el("button", { class: "btn btn-primary", type: "button", text: submitLabel });
  const cancel = el("button", { class: "btn", type: "button", text: i18n.d.common.cancel, on: { click: () => modal.close() } });
  const modal = openModal({ title, body, foot: [cancel, ok] });

  function validate(): boolean {
    let valid = true;
    for (const row of rows) {
      const message = row.check?.() ?? null;
      if (message) valid = false;
      const show = message !== null && (touched.has(row) || message !== t.required);
      errors.get(row)!.textContent = show ? message : "";
    }
    ok.disabled = !valid;
    return valid;
  }

  const run = async (): Promise<void> => {
    rows.forEach((row) => touched.add(row));
    if (!validate()) return;
    failure.textContent = "";
    ok.disabled = true;
    try {
      await submit();
      modal.close();
    } catch (err) {
      failure.textContent = (err as Error).message;
      ok.disabled = false;
    }
  };
  body.addEventListener("submit", (e) => {
    e.preventDefault();
    void run();
  });
  ok.addEventListener("click", () => void run());
  validate();
  body.querySelector<HTMLInputElement>("input, select")?.focus();
}

/**
 * The tile shown in the catalogue: a set of built-in icons, a box for any other
 * emoji, a row of tints, and a live preview of the result.
 */
function lookPicker(icon: string, theme: string | undefined): { element: HTMLElement; icon(): string; theme(): string | undefined } {
  const t = i18n.d.workspaceDialogs;
  let currentTheme = theme;
  const custom = textInput(icon, "🙂");
  custom.maxLength = 8;
  custom.style.width = "64px";
  const preview = el("span", { class: "catalog-icon" });

  const iconButtons = ICONS.map((glyph) =>
    el("button", { type: "button", class: "look-icon", text: glyph, title: glyph, on: { click: () => {
      custom.value = glyph;
      custom.dispatchEvent(new Event("input", { bubbles: true }));
    } } }),
  );
  const swatches = [undefined, ...THEMES].map((id) =>
    el("button", {
      type: "button",
      class: `look-swatch catalog-icon${id ? ` theme-${id}` : ""}`,
      title: id ?? t.noColor,
      text: id ? "" : "∅",
      dataset: { theme: id ?? "" },
      on: { click: (e: MouseEvent) => {
        currentTheme = id;
        (e.currentTarget as HTMLElement).dispatchEvent(new Event("change", { bubbles: true }));
      } },
    }),
  );

  const sync = (): void => {
    const glyph = custom.value.trim();
    preview.textContent = glyph || "·";
    preview.className = `catalog-icon look-preview${currentTheme ? ` theme-${currentTheme}` : ""}`;
    iconButtons.forEach((b) => b.classList.toggle("is-active", b.textContent === glyph));
    swatches.forEach((s) => s.classList.toggle("is-active", (s.dataset.theme || undefined) === currentTheme));
  };

  const element = el("div", { class: "look-picker" }, [
    el("div", { class: "look-row" }, [preview, el("span", { class: "form-hint", text: t.iconPreview })]),
    el("div", { class: "look-icons" }, iconButtons),
    el("div", { class: "look-row" }, [el("span", { class: "form-hint", text: t.iconCustom }), custom]),
    el("div", { class: "look-row" }, [el("span", { class: "form-hint", text: t.colorLabel }), ...swatches]),
  ]);
  element.addEventListener("input", sync);
  element.addEventListener("change", sync);
  sync();
  return { element, icon: () => custom.value.trim(), theme: () => currentTheme };
}

function projectRows(editor: DiagramEditorFacade, existing: ProjectEntry | undefined) {
  const t = i18n.d.workspaceDialogs;
  const others = editor.workspace.projects.filter((p) => p.id !== existing?.id);
  const id = textInput(existing?.id ?? "", "shop");
  const title = textInput(existing?.title ?? "");
  const subtitle = textInput(existing?.subtitle ?? "");
  const look = lookPicker(existing?.icon ?? "📁", existing?.theme);

  const rows: Row[] = [
    { label: `${t.titleLabel} *`, control: title, check: () => {
      if (!title.value.trim()) return t.required;
      return others.some((p) => same(p.title, title.value)) ? t.titleTaken : null;
    } },
    { label: `${t.idLabel} *`, control: id, hint: existing ? `${t.projectIdHint} ${t.renameNote}` : t.projectIdHint, check: () => {
      const v = id.value.trim();
      if (!v) return t.required;
      if (!PROJECT_ID.test(v)) return t.badProjectId;
      return others.some((p) => p.id === v) ? t.idTaken : null;
    } },
    { label: t.subtitleLabel, control: subtitle },
    { label: t.iconLabel, control: look.element, hint: t.iconHint },
  ];
  const value = () => ({
    id: id.value.trim(),
    title: title.value.trim(),
    subtitle: subtitle.value.trim() || undefined,
    icon: look.icon() || undefined,
    theme: look.theme(),
    language: editor.dataLang,
  });
  return { rows, value };
}

export function openNewProjectDialog(editor: DiagramEditorFacade): void {
  const t = i18n.d.workspaceDialogs;
  const { rows, value } = projectRows(editor, undefined);
  formDialog(t.newProjectTitle, t.create, rows, async () => {
    const project = value();
    await editor.createProject(project);
    // A project without a diagram shows nothing; go straight on to the first one.
    queueMicrotask(() => openNewViewDialog(editor, project.id));
  });
}

export function openEditProjectDialog(editor: DiagramEditorFacade, project: ProjectEntry): void {
  const t = i18n.d.workspaceDialogs;
  const { rows, value } = projectRows(editor, project);
  formDialog(t.editProjectTitle, t.save, rows, () => editor.updateProject(project.id, value()));
}

function viewDialog(editor: DiagramEditorFacade, projectId: string | undefined, existing: ViewEntry | undefined): void {
  const t = i18n.d.workspaceDialogs;
  const projects = editor.workspace.projects.filter((p) => !p.error);
  if (projects.length === 0) {
    openNewProjectDialog(editor);
    return;
  }
  const current = editor.currentView ? editor.projectOf(editor.currentView)?.id : undefined;
  const selected = projectId ?? current ?? projects[0]!.id;
  const projectOf = (pid: string): ProjectEntry => projects.find((p) => p.id === pid)!;
  // A name is written in the language it is shown in, when the project has it.
  const langOf = (p: ProjectEntry): string => (p.languages.includes(editor.dataLang) ? editor.dataLang : p.languages[0] ?? editor.dataLang);

  const project = el("select", { attrs: { style: "width: 100%;" } },
    projects.map((p) => {
      const option = el("option", { value: p.id, text: `${p.icon ?? "📁"} ${p.title}` });
      if (p.id === selected) option.selected = true;
      return option;
    }),
  );
  project.disabled = existing !== undefined;
  const id = textInput(existing?.id ?? (projectOf(selected).views.length > 0 ? "v_" : "v_main"));
  const name = textInput(existing ? editor.viewName(existing) : "");
  const axis = textInput(existing?.axis ?? "", "axis_subsystem");
  axis.setAttribute("list", "semaps-axes");
  const axisList = el("datalist", { id: "semaps-axes" }, AXES.map((a) => el("option", { value: a })));
  const look = lookPicker(existing?.icon ?? "🗺️", existing?.theme);

  const siblings = (): ViewEntry[] => projectOf(project.value).views.filter((v) => v.file !== existing?.file);
  const normId = (): string => {
    const v = id.value.trim();
    return v && !v.startsWith("v_") ? `v_${v}` : v;
  };

  const rows: Row[] = [
    { label: t.projectLabel, control: project },
    { label: `${t.nameLabel} *`, control: name, check: () => {
      if (!name.value.trim()) return t.required;
      return siblings().some((v) => same(editor.viewName(v), name.value)) ? t.nameTaken : null;
    } },
    { label: `${t.idLabel} *`, control: id, hint: existing ? `${t.viewIdHint} ${t.renameNote}` : t.viewIdHint, check: () => {
      const v = normId();
      if (!v || v === "v_") return t.required;
      if (!VIEW_ID.test(v)) return t.badViewId;
      return siblings().some((s) => s.id === v) ? t.idTaken : null;
    } },
    { label: `${t.axisLabel} *`, control: el("div", {}, [axis, axisList]), hint: t.axisHint,
      check: () => (axis.value.trim() ? null : t.required) },
    { label: t.iconLabel, control: look.element, hint: t.iconHint },
  ];

  const submit = async (): Promise<void> => {
    const target = projectOf(project.value);
    const view = {
      project: target.id,
      id: normId(),
      name: name.value.trim(),
      axis: axis.value.trim(),
      icon: look.icon() || undefined,
      theme: look.theme(),
      language: langOf(target),
    };
    if (existing) await editor.updateView(existing.id, view);
    else await editor.createView(view);
  };
  formDialog(existing ? t.editViewTitle : t.newViewTitle, existing ? t.save : t.create, rows, submit);
  if (!existing) name.focus();
}

export function openNewViewDialog(editor: DiagramEditorFacade, projectId?: string): void {
  viewDialog(editor, projectId, undefined);
}

export function openEditViewDialog(editor: DiagramEditorFacade, view: ViewEntry): void {
  viewDialog(editor, editor.projectOf(view)?.id, view);
}
