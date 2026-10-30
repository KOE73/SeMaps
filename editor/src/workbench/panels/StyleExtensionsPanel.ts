import type { IContentRenderer, GroupPanelPartInitParameters } from "dockview-core";
import type { DiagramEditor } from "../../editor/DiagramEditor.js";
import { StyleEditor } from "../../editor/StyleEditor.js";
import { StyleSelection } from "../../editor/StyleSelection.js";
import { StyleLibrary } from "../../model/StyleLibrary.js";
import type { StyleModifier, StyleRule, StyleSelector, SelectorField, ExtensionCollection } from "../../model/style-extensions.js";
import type { StyleAppearance } from "../../model/style-extensions.js";
import { idsFromText } from "../../model/style-extensions.js";
import type { StyleTarget, WireStyle } from "../../model/style-types.js";
import { stylePatch } from "../../editor/io/HostStyleStore.js";
import { el, replaceChildren } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";
import { choiceCaption, choicePicker, extensionHelp, type NamedChoice } from "./StyleExtensionControls.js";

const text = (ru: string, en: string): string => i18n.currentLanguage === "ru" ? ru : en;
const button = (label: string, action: () => void, description = ""): HTMLButtonElement => el("button", { class: "btn btn-small", text: label, dataset: { tooltipTitle: label, tooltipDesc: description }, on: { click: action } });
function input(label: string, value: string, change: (v: string) => void, multiline = false): HTMLElement {
  return el("label", { class: "field" }, [el("span", { class: "field-label", text: label }), el(multiline ? "textarea" : "input", { value, on: { input: e => change((e.target as HTMLInputElement).value) } })]);
}
function choose(options: Array<[string, string]>, value: string, change: (v: string) => void, label?: string): HTMLSelectElement {
  return el("select", { attrs: label ? { "aria-label": label } : {}, on: { change: e => change((e.target as HTMLSelectElement).value) } }, options.map(([v, name]) => el("option", { value: v, text: name, attrs: v === value ? { selected: "" } : {} })));
}

/** UI operates on an explicit draft and the host's versioned selector catalogue. */
export class StyleExtensionsEditor {
  private draft: StyleModifier | StyleRule | null = null;
  private original: StyleModifier | StyleRule | null = null;
  private revision = 0;
  private dirty = false;
  private family: StyleTarget = "block";
  private fields: SelectorField[] = [];
  private readonly status = el("div", { class: "muted", attrs: { role: "status" } });
  private readonly results = el("div");
  private readonly draftStatus = el("span", { class: "muted" });
  private readonly selection = new StyleSelection();
  constructor(private list: HTMLElement, private form: HTMLElement, private editor: DiagramEditor, private collection: ExtensionCollection) {
    this.form.classList.add("extension-form");
    this.list.classList.add("extension-list");
    editor.libraryEvents.on("loaded", () => {
      this.renderList();
      if (!this.dirty) {
        this.selection.reconcile(id => this.definitions().some(d => d.id === id));
        if (this.selection.id) this.open(this.selection.id);
        else { this.draft = null; this.original = null; this.renderForm(); }
      }
      else if (this.dirty && this.revision !== editor.styleRevision) this.status.textContent = text("Библиотека на хосте изменилась. Ваши правки остались в форме. Скопируйте нужные значения, нажмите «Отменить правки», затем внесите их заново.", "The host library changed. Your edits remain in the form. Copy values you need, discard edits, then re-enter them.");
    });
    editor.libraryEvents.on("extension", change => {
      if(change.collection !== this.collection || this.dirty) return;
      if(change.action === "rename" && change.newId && this.selection.rename(change.id, change.newId)) {
        if(this.draft)this.draft.id=change.newId;
        if(this.original)this.original.id=change.newId;
        if(this.definitions().some(d=>d.id===change.newId))this.open(change.newId);
      }
      if(change.action === "delete") { this.selection.remove(change.id); if(this.draft?.id===change.id){this.draft=null;this.original=null;this.renderForm();} }
    });
    editor.canvas.events.on("select", () => { if (this.collection === "modifiers") this.renderAssignment(); });
    void editor.extensionRequest("/api/style-selectors").then((catalog: { fields: SelectorField[] }) => { this.fields = catalog.fields; if (!this.dirty) this.renderForm(); }).catch(e => this.fail(e));
    i18n.onLanguageChange(() => { this.renderList(); this.renderForm(); });
    this.renderList(); this.renderForm();
  }
  private definitions(): Array<StyleModifier | StyleRule> { return this.editor.styles.serialize()[this.collection] ?? []; }
  private changed(): void { this.dirty = true; this.status.textContent = ""; this.draftStatus.textContent = text("Есть неприменённые изменения", "Unapplied changes"); }
  private fail(error: unknown): void { this.status.textContent = (error as Error).message; }
  private run(action: () => Promise<void>): void { void action().catch(e => this.fail(e)); }
  open(id: string): void {
    if (this.dirty && this.draft?.id !== id) { this.status.textContent = text("Сначала нажмите «Применить изменения» или «Отменить правки».", "First apply changes or discard edits."); return; }
    const definition = this.definitions().find(v => v.id === id);
    this.draft = definition ? structuredClone(definition) : null;
    this.original = definition ? structuredClone(definition) : null;
    this.selection.open(definition ? id : null);
    this.revision = this.editor.styleRevision; this.dirty = false;
    this.status.textContent = ""; this.results.replaceChildren();
    this.renderList(); this.renderForm();
  }
  private create(): void {
    if (this.dirty) { this.status.textContent = text("Сначала нажмите «Применить изменения» или «Отменить правки».", "First apply changes or discard edits."); return; }
    const stem = this.collection === "modifiers" ? "modifier" : "rule";
    let id = stem; for (let n = 2; this.definitions().some(d => d.id === id); n++) id = `${stem}.${n}`;
    this.draft = this.collection === "modifiers" ? { id, name: text("Новый модификатор", "New modifier"), patches: { block: {} } } : { id, name: text("Новое правило", "New rule"), selectorVersion: 1, selector: { field: "namespace", op: "within", value: "My.Namespace" }, modifierIds: [], enabled: true, order: 0 };
    this.original = null; this.selection.open(null); this.revision = this.editor.styleRevision; this.changed(); this.renderList(); this.renderForm();
  }
  private async apply(): Promise<void> {
    if (!this.draft) return;
    const definition = structuredClone(this.draft);
    // Rename is a separate atomic reference operation; never embed an id change in update.
    if (this.original && definition.id !== this.original.id) throw new Error(text("Для изменения ID используйте «Переименовать».", "Use Rename to change the ID."));
    await this.editor.editStyleExtension(this.collection, this.original ? { action: "update", id: definition.id, patch: stylePatch(this.original as unknown as Record<string, unknown>, definition as unknown as Record<string, unknown>), expectedRevision: this.revision } : { action: "create", id: definition.id, definition, expectedRevision: this.revision });
    this.dirty = false; this.open(definition.id);
    this.status.textContent = text("Применено ко всем редакторам. Для записи на диск сохраните библиотеку в панели «Изменения».", "Applied to all editors. Save the library in Changes to write it to disk.");
  }
  private renderList(): void {
    const create = button(text("+ Создать", "+ Create"), () => this.create());
    create.classList.add("btn-primary", "style-create-button");
    replaceChildren(this.list, create, ...this.definitions().map(d => el("button", {
      class: `btn extension-list-item${this.selection.id === d.id ? " is-selected" : ""}`,
      attrs: { "aria-pressed": String(this.selection.id === d.id) },
      dataset: { tooltipTitle: d.name, tooltipDesc: d.description ?? d.id },
      on: { click: () => this.open(d.id) },
    }, [choiceCaption(d)])));
    this.list.style.cssText += ";overflow:auto;display:flex;flex-direction:column;gap:5px;padding:8px";
  }
  private renderForm(): void {
    this.form.replaceChildren(); this.form.style.padding = "8px";
    if (!this.editor.sharedStyles) { this.form.append(el("p", { text: text("Требуется общий хост рабочего пространства.", "A shared workspace host is required.") })); return; }
    this.form.append(el("div", { class: "extension-heading" }, [el("strong", { text: this.collection === "modifiers" ? text("Модификаторы", "Modifiers") : text("Правила стилей", "Style rules") }), extensionHelp(text("Как устроено редактирование", "How editing works"), text("Правки в форме — ваш локальный черновик. «Применить изменения» передаёт его общему хосту: результат сразу виден во всех редакторах, но ещё не записан на диск. «Отменить правки» удаляет ваши неприменённые изменения и заново загружает определение с хоста. Для записи на диск нажмите «Сохранить» в панели «Изменения». Оформление складывается по порядку: базовый стиль → автоматические модификаторы → ручные модификаторы → локальные переопределения.", "Form edits are a local draft. Apply changes sends them to the shared host and updates all editors without saving to disk. Discard edits drops your unapplied changes and reloads the host definition. Save in Changes writes to disk. Formatting order: base style → automatic modifiers → manual modifiers → local overrides."))]));
    const d = this.draft;
    if (!d) { this.form.append(el("p", { text: text("Выберите определение или создайте новое.", "Select a definition or create one.") }), this.status); if (this.collection === "modifiers") this.renderAssignment(); return; }
    this.draftStatus.textContent = this.dirty ? text("Есть неприменённые изменения", "Unapplied changes") : "";
    this.form.append(input("ID", d.id, v => { if (!this.original) { d.id = v; this.changed(); } }), input(text("Название", "Name"), d.name, v => { d.name = v; this.changed(); }), input(text("Описание", "Description"), d.description ?? "", v => { d.description = v || undefined; this.changed(); }, true));
    const idControl = this.form.querySelector<HTMLInputElement>("input"); if (idControl && this.original) idControl.readOnly = true;
    const apply = button(text("Применить изменения", "Apply changes"), () => this.run(() => this.apply()), text("Передать правки общему хосту и обновить все редакторы. Для записи на диск затем сохраните библиотеку в «Изменениях».", "Send edits to the shared host and update all editors. Then save the library in Changes to write to disk."));
    apply.classList.add("btn-primary");
    this.form.append(el("div", { class: "extension-actions" }, [apply, button(text("Отменить правки", "Discard edits"), () => { this.dirty = false; this.open(d.id); }, text("Удалить неприменённые правки этой формы и загрузить определение с общего хоста.", "Discard unapplied form edits and reload the shared host definition.")), extensionHelp(text("Применение и сохранение", "Apply and save"), text("Применение меняет общую библиотеку в памяти хоста. Сохранение в панели «Изменения» записывает её на диск. Отмена правок здесь затрагивает только форму; уже применённые изменения можно отменить в панели «Изменения».", "Apply changes updates the host library in memory. Save in Changes writes it to disk. Discard edits affects only this form; discard already applied library changes in Changes."))]), this.draftStatus, this.status);
    if (this.original) {
      const rename = el("input", { value: d.id, attrs: { "aria-label": text("Новый ID", "New ID") } });
      const replacement = choose([["", text("Без замены", "No replacement")], ...this.definitions().filter(x => x.id !== d.id).map(x => [x.id, x.name] as [string, string])], "", () => {});
      this.form.append(el("details", {}, [el("summary", { text: text("Переименование и удаление", "Rename and delete") }), rename, button(text("Переименовать", "Rename"), () => this.run(async () => {
        if (this.dirty) throw new Error(text("Сначала примените черновик.", "Apply your draft first."));
        await this.editor.editStyleExtension(this.collection, { action: "rename", id: d.id, newId: rename.value.trim(), expectedRevision: this.revision }); this.open(rename.value.trim());
      })), replacement, button(text("Удалить", "Delete"), () => this.run(async () => {
        await this.editor.editStyleExtension(this.collection, { action: "delete", id: d.id, replacement: replacement.value || undefined, expectedRevision: this.revision }); this.dirty = false; this.draft = null; this.original = null; this.renderList(); this.renderForm();
      }))]));
    }
    if (this.collection === "modifiers") this.modifierForm(d as StyleModifier); else this.ruleForm(d as StyleRule);
  }
  private modifierForm(d: StyleModifier): void {
    const target = choose([["block", text("Блок", "Block")], ["container", text("Контейнер", "Container")], ["edge", text("Связь", "Relation")]], this.family, v => { this.family = v as StyleTarget; this.renderForm(); }, text("Семейство модификатора", "Modifier family"));
    const enabled = el("input", { type: "checkbox", attrs: d.patches[this.family] !== undefined ? { checked: "" } : {}, on: { change: e => {
      if ((e.target as HTMLInputElement).checked) d.patches[this.family] = {}; else delete d.patches[this.family]; this.changed(); this.renderForm();
    } } });
    this.form.append(el("p", { text: text("Задайте только изменяемые свойства. Пустое поле сохраняет результат предыдущего слоя. Геометрия и шаблон берутся из базового стиля.", "Set only properties to change. Empty fields retain the previous layer. Geometry and template come from the base style.") }), target, el("label", { class: "extension-check-label" }, [enabled, text(" Форматировать это семейство", " Format this family")]));
    if (d.patches[this.family]) {
      const mount = el("div"); this.form.append(mount);
      const library = StyleLibrary.parse({ styles: [{ ...d.patches[this.family], id: "@modifier", name: d.name, appliesTo: this.family }] });
      const capture = (apply: () => void): void => {
        apply(); const style = { ...library.get("@modifier")! };
        for (const key of ["id", "name", "appliesTo", "forKinds", "basedOn"] as const) delete (style as Partial<WireStyle>)[key];
        d.patches[this.family] = style; this.changed();
      };
      const editor = this.editor;
      const form = new StyleEditor(mount, { canvas: editor.canvas, styles: library, editStyle: capture, commitStyle: capture, openStyle: () => {}, notify: v => editor.notify(v), applyKindAndStyle: () => {}, applyRelationTypeAndStyle: () => {} }, true);
      form.open("@modifier");
    }
    this.renderAssignment();
  }
  private assignmentMount: HTMLElement | null = null;
  private renderAssignment(): void {
    const expanded = this.assignmentMount?.hasAttribute("open") ?? false;
    this.assignmentMount?.remove();
    const selection = [...this.editor.canvas.selectedIds];
    const own = selection.map(id => this.editor.canvas.model?.element(id) ?? this.editor.canvas.model?.edge(id)).filter(v => v !== undefined);
    const common = (key: "modifiers" | "disabledModifiers"): string => own.length && own.every(v => JSON.stringify(v[key] ?? []) === JSON.stringify(own[0]![key] ?? [])) ? (own[0]![key] ?? []).join(", ") : "";
    let manual = idsFromText(common("modifiers")), disabled = idsFromText(common("disabledModifiers"));
    const choices = this.editor.styles.serialize().modifiers ?? [];
    const automatic = choose([["true", text("Автоматические включены", "Automatic enabled")], ["false", text("Автоматические выключены", "Automatic disabled")]], own.length && own.every(v => v.automaticModifiers === false) ? "false" : "true", () => {});
    const mount = el("details", { class: "style-section" }, [el("summary", { text: `${text("Назначения на выделение", "Selection assignments")} (${selection.length})` }), el("div", { class: "style-section-body" }, [
      extensionHelp(text("Назначения на выделенные объекты", "Assignments to selected objects"), text("Галочки задают полный список для всех выделенных объектов. Если назначения у объектов различаются, список показан пустым; применение заменит их выбранным списком. Стрелки задают порядок: нижний модификатор перекрывает верхний. Каждая кнопка меняет только свой раздел. Эти назначения сохраняются вместе с текущей схемой.", "Checkboxes set a complete list for every selected object. Mixed selections show an empty list; applying replaces their lists. Lower modifiers override higher ones. Each button changes only its section. Assignments are saved with the current view.")),
      choicePicker(text("Ручные модификаторы", "Manual modifiers"), choices, manual, ids => { manual = ids; }, { ordered: true }), button(text("Задать ручные", "Set manual"), () => this.run(() => this.editor.assignStyleModifiers({ modifiers: manual }))),
      choicePicker(text("Исключить автоматические модификаторы", "Exclude automatic modifiers"), choices, disabled, ids => { disabled = ids; }), button(text("Задать исключения", "Set exclusions"), () => this.run(() => this.editor.assignStyleModifiers({ disabledModifiers: disabled }))),
      automatic, button(text("Задать автоматику", "Set automatic"), () => this.run(() => this.editor.assignStyleModifiers({ automaticModifiers: automatic.value === "true" }))),
      ...selection.slice(0, 1).map(id => this.explanation(this.editor.appearances[id])),
    ])]);
    if (expanded) mount.setAttribute("open", "");
    this.assignmentMount = mount; this.form.append(mount);
  }
  private explanation(a: StyleAppearance | undefined): HTMLElement {
    if (!a) return el("p", { text: text("Выберите объект на схеме.", "Select an object on the diagram.") });
    const layers = a.applied.map(m => `${m.name} (${m.rule ? text("правило", "rule") + ": " + m.rule : text("вручную", "manual")})`);
    return el("div", {}, [el("p", { text: `${text("Базовый стиль", "Base style")}: ${a.baseStyleId}` }), el("p", { text: `${text("Применены", "Applied")}: ${layers.join(" → ") || "—"}` }), ...(a.skipped.length ? [el("p", { text: `${text("Исключены", "Excluded")}: ${a.skipped.map(m => m.name).join(", ")}` })] : []), el("details", {}, [el("summary", { text: text("Свойства и происхождение оформления", "Properties and formatting origins") }), el("pre", { attrs: { style: "white-space:pre-wrap;overflow-wrap:anywhere" }, text: JSON.stringify({ facts: a.facts, resolved: a.resolved, origins: a.origins }, null, 2) })])]);
  }
  private ruleForm(d: StyleRule): void {
    const enabled = el("input", { type: "checkbox", attrs: d.enabled !== false ? { checked: "" } : {}, on: { change: e => { d.enabled = (e.target as HTMLInputElement).checked; this.changed(); } } });
    const order = input(text("Порядок правила (большее число — позднее)", "Rule order (higher numbers apply later)"), String(d.order ?? 0), v => { d.order = Number(v); this.changed(); });
    order.querySelector("input")!.type = "number";
    const projects = this.editor.workspace.projects;
    const allScopes = text("Не выбрано — применяется ко всем", "Nothing selected — applies to all");
    const viewScopes = el("div");
    const renderViews = (): void => viewScopes.replaceChildren(choicePicker(text("Схемы", "Views"), [...new Map(projects.filter(p => !d.scope?.projects?.length || d.scope.projects.includes(p.id)).flatMap(p => p.views.map(v => [v.id, { id: v.id, name: `${p.title} / ${v.names?.[i18n.currentLanguage] ?? v.id}` }] as const))).values()], d.scope?.views ?? [], ids => { d.scope = { ...d.scope, views: ids }; this.changed(); }, { emptyHint: allScopes }));
    renderViews();
    this.form.append(el("label", { class: "extension-check-label" }, [enabled, text(" Правило включено", " Rule enabled")]), order,
      choicePicker(text("Модификаторы в порядке применения", "Modifiers in application order"), this.editor.styles.serialize().modifiers ?? [], d.modifierIds, ids => { d.modifierIds = ids; this.changed(); }, { ordered: true }),
      choicePicker(text("Проекты SeMaps", "SeMaps projects"), projects.map(p => ({ id: p.id, name: p.title })), d.scope?.projects ?? [], ids => { d.scope = { ...d.scope, projects: ids }; this.changed(); renderViews(); }, { emptyHint: allScopes }), viewScopes,
      extensionHelp(text("Область и условия правила", "Rule scope and conditions"), text("Проекты и схемы ограничивают область применения. Пустой список означает «все». В условиях «ВСЕ» требует совпадения каждого пункта, «ЛЮБОЕ» — хотя бы одного, «НЕ» исключает совпадение. Семейство выбирается галочками: блоки, контейнеры и связи. Доступные значения берутся из текущей схемы и реестра; при необходимости можно задать новое значение. Сборка определяется связями contains, а не положением на схеме. Проверяйте результат предпросмотром перед применением.", "Empty project or view selections mean all. ALL requires every condition, ANY at least one, NOT excludes matches. Family offers blocks, containers and relations. Available values come from the current view and registry; custom values are also supported. Assembly membership follows contains relations. Preview before applying.")));
    this.form.append(el("p", { text: text("Фильтруются сущности реестра и концы видимых связей. Сборка определяется структурными contains; визуальный parent не участвует. Отсутствующее поле не совпадает, даже под НЕ; используйте exists. Сравнения чувствительны к регистру.", "Filters registry entities and endpoints of visible relations. Assembly follows structural contains; visual parent is ignored. Missing facts do not match, even under NOT; use exists. Comparisons are case-sensitive.") }), this.selector(d.selector, next => { d.selector = next; this.changed(); this.renderForm(); }));
    this.form.append(button(text("Предпросмотр на текущем виде", "Preview on current view"), () => this.run(async () => {
      const result = await this.editor.previewStyleRule(d);
      replaceChildren(this.results, el("p", { text: `${text("Совпадений", "Matches")}: ${result.count}` }), ...result.matches.map(a => el("details", {}, [el("summary", { text: `${a.id} · ${a.family} · ${a.applied.some(m=>m.rule===d.id) ? text("применяется", "applies") : text("исключено", "excluded")}` }), this.explanation(a)])));
    })), this.results);
    let exclusions = (this.editor.canvas.model?.bundle?.view.disabledStyleRules as string[] | undefined) ?? [];
    this.form.append(el("details", {}, [el("summary", { text: text("Отключить правила на текущей схеме", "Disable rules on current view") }), choicePicker(text("Отключённые правила", "Disabled rules"), this.editor.styles.serialize().styleRules ?? [], exclusions, ids => { exclusions = ids; }, { emptyHint: text("Ничего не отключено", "No rules disabled") }), button(text("Задать список", "Set list"), () => this.run(() => this.editor.setStyleRuleExclusions(exclusions)))]));
  }
  private selector(value: StyleSelector, replace: (s: StyleSelector) => void): HTMLElement {
    const kind = "all" in value ? "all" : "any" in value ? "any" : "not" in value ? "not" : "field";
    const leaf = (): StyleSelector => ({ field: "kind", op: "eq", value: "class" });
    const frame = el("div", { class: "extension-condition" }, [choose([["field", text("Поле", "Field")], ["all", text("ВСЕ условия", "ALL conditions")], ["any", text("ЛЮБОЕ условие", "ANY condition")], ["not", text("НЕ совпадает", "NOT matching")]], kind, v => replace(v === "all" ? { all: [value] } : v === "any" ? { any: [value] } : v === "not" ? { not: value } : leaf()), text("Группа условия", "Condition group"))]);
    if ("all" in value || "any" in value) {
      const children = "all" in value ? value.all : value.any;
      const build = (items: StyleSelector[]): StyleSelector => "all" in value ? { all: items } : { any: items };
      children.forEach((child, n) => frame.append(this.selector(child, next => replace(build(children.map((x, i) => i === n ? next : x)))), button(text("Удалить условие", "Remove condition"), () => replace(build(children.filter((_, i) => i !== n))))));
      frame.append(button(text("+ Условие", "+ Condition"), () => replace(build([...children, leaf()]))));
    } else if ("not" in value) frame.append(this.selector(value.not, not => replace({ not })));
    else {
      const descriptor = this.fields.find(f => f.id === value.field);
      const opNames: Record<string, string> = { eq: text("Равно", "Equals"), neq: text("Не равно", "Not equal"), in: text("Одно из выбранных", "One of selected"), notIn: text("Ни одно из выбранных", "None of selected"), prefix: text("Начинается с", "Starts with"), contains: text("Содержит", "Contains"), exists: text("Поле есть / отсутствует", "Present / absent"), within: text("Namespace и его потомки", "Namespace and descendants") };
      frame.append(choose(this.fields.map(f => [f.id, `${f.name} · ${f.id}`]), value.field, field => replace({ field, op: "eq", value: "" }), text("Поле условия", "Condition field")), choose((descriptor?.operators ?? ["eq"]).map(op => [op, opNames[op] ?? op]), value.op, op => replace({ ...value, op, value: op === "exists" ? true : op === "in" || op === "notIn" ? [] : "" }), text("Операция сравнения", "Comparison operator")), el("p", { class: "muted", text: value.field === "family" ? text("Выберите виды объектов, которые должно оформлять правило.", "Choose the object families formatted by this rule.") : descriptor?.description ?? value.field }));
      if (value.op === "exists") frame.append(choose([["true", text("Есть", "Present")], ["false", text("Отсутствует", "Absent")]], String(value.value), v => { value.value = v === "true"; this.changed(); }));
      else if (value.op === "in" || value.op === "notIn") frame.append(choicePicker(text("Значения условия", "Condition values"), this.valueChoices(value.field), Array.isArray(value.value) ? value.value : [], ids => { value.value = ids; this.changed(); }, { custom: value.field !== "family" }));
      else {
        if (value.field === "family" && (value.op === "eq" || value.op === "neq")) {
          frame.append(choose([["", text("Выберите семейство…", "Choose family…")], ...this.valueChoices("family").map(v => [v.id, v.name] as [string, string])], String(value.value), v => { value.value = v; this.changed(); }, text("Семейство оформления", "Formatting family")));
          return frame;
        }
        const field = input(text("Значение", "Value"), String(value.value), v => { value.value = v; this.changed(); });
        const values = this.valueChoices(value.field);
        if (values.length) frame.append(choose([["", text("Выбрать доступное значение…", "Choose an available value…")], ...values.map(v => [v.id, v.name === v.id ? v.id : `${v.name} · ${v.id}`] as [string, string])], "", v => { if (!v) return; value.value = v; field.querySelector("input")!.value = v; this.changed(); }, text("Доступные значения условия", "Available condition values")));
        frame.append(field);
      }
    }
    return frame;
  }
  private valueChoices(field: string): NamedChoice[] {
    if (field === "family") return [{ id: "block", name: text("Блоки", "Blocks") }, { id: "container", name: text("Контейнеры", "Containers") }, { id: "edge", name: text("Связи", "Relations") }];
    if (field === "project") return this.editor.workspace.projects.map(p => ({ id: p.id, name: p.title }));
    if (field === "view") return this.editor.workspace.projects.flatMap(p => p.views.map(v => ({ id: v.id, name: v.names?.[i18n.currentLanguage] ?? v.id })));
    const values = new Set(Object.values(this.editor.appearances).flatMap(a => a.facts[field] ?? []));
    const entities = this.editor.canvas.model?.bundle?.entities.entities ?? [];
    const key = field.replace(/^(source|target)\./, "");
    for (const entity of entities) {
      if (["id", "kind", "name", "namespace", "origin", "status"].includes(key) && typeof entity[key] === "string") values.add(entity[key] as string);
      for (const code of entity.code ?? []) {
        const v = key === "library" ? code.library : key === "language" ? code.lang : key === "code.ref" ? code.ref : undefined;
        if (v) values.add(v);
      }
    }
    return [...values].sort().map(id => ({ id, name: key === "assembly" || key === "id" ? entities.find(e => e.id === id)?.name ?? id : id }));
  }
}

export class StyleRulesPanel implements IContentRenderer {
  readonly element: HTMLElement;
  constructor(editor: DiagramEditor) {
    const list = el("div", { attrs: { style: "flex:0 0 30%;min-width:0;overflow:auto" } });
    const form = el("div", { attrs: { style: "flex:1;min-width:0;overflow:auto" } });
    this.element = el("div", { attrs: { style: "display:flex;height:100%;background:var(--panel);overflow:hidden" } }, [list, form]);
    const rules = new StyleExtensionsEditor(list, form, editor, "styleRules");
    editor.libraryEvents.on("openExtension", ({collection,id}) => { if(collection === "styleRules") rules.open(id); });
  }
  init(_params: GroupPanelPartInitParameters): void {}
}
