import type { DiagramEditor } from "../DiagramEditor.js";
import { layoutFreeKey } from "../../util/keys.js";
import { i18n } from "../../workbench/i18n/I18nService.js";
import { renderMarkdown } from "./MarkdownRenderer.js";
import { isContainer } from "../../model/types.js";

export type DocTargetKind = "node" | "zone" | "edge";

export class DocEditorDialog {
  private readonly modalEl: HTMLElement;
  private readonly cardEl: HTMLElement;
  private readonly headEl: HTMLElement;
  private readonly titleEl: HTMLElement;
  private readonly kindBadgeEl: HTMLElement;
  private readonly langRuBtn: HTMLButtonElement;
  private readonly langEnBtn: HTMLButtonElement;
  private readonly descInput: HTMLTextAreaElement;
  private readonly docTextarea: HTMLTextAreaElement;
  private readonly previewPane: HTMLElement;
  private readonly contentAreaEl: HTMLElement;
  private readonly tabEditBtn: HTMLButtonElement;
  private readonly tabSplitBtn: HTMLButtonElement;
  private readonly tabPreviewBtn: HTMLButtonElement;

  private currentTargetId: string | null = null;
  private currentKind: DocTargetKind = "node";
  private currentLang = "ru";
  private currentMode: "edit" | "split" | "preview" = "preview";

  constructor(private readonly editor: DiagramEditor) {
    // 1. Root Modal Container
    this.modalEl = document.createElement("div");
    this.modalEl.className = "semaps-doc-modal";
    this.modalEl.hidden = true;

    // 2. Card Container
    this.cardEl = document.createElement("div");
    this.cardEl.className = "semaps-doc-card";

    // 3. Header
    this.headEl = document.createElement("div");
    this.headEl.className = "semaps-doc-head";

    const titleGroup = document.createElement("div");
    titleGroup.className = "semaps-doc-title-group";

    const icon = document.createElement("span");
    icon.className = "semaps-doc-title-icon";
    icon.textContent = "📄";

    this.titleEl = document.createElement("span");
    this.titleEl.className = "semaps-doc-title-text";

    this.kindBadgeEl = document.createElement("span");
    this.kindBadgeEl.className = "semaps-doc-kind-badge";

    titleGroup.append(icon, this.titleEl, this.kindBadgeEl);

    const headActions = document.createElement("div");
    headActions.className = "semaps-doc-head-actions";

    const langSelector = document.createElement("div");
    langSelector.className = "semaps-doc-lang-selector";

    this.langRuBtn = document.createElement("button");
    this.langRuBtn.className = "semaps-doc-lang-btn is-active";
    this.langRuBtn.textContent = "RU";
    this.langRuBtn.onclick = () => this.setLanguage("ru");

    this.langEnBtn = document.createElement("button");
    this.langEnBtn.className = "semaps-doc-lang-btn";
    this.langEnBtn.textContent = "EN";
    this.langEnBtn.onclick = () => this.setLanguage("en");

    langSelector.append(this.langRuBtn, this.langEnBtn);

    const closeBtn = document.createElement("button");
    closeBtn.className = "semaps-doc-close-btn";
    closeBtn.textContent = "✕";
    closeBtn.title = i18n.d.common.close;
    closeBtn.onclick = () => this.close();

    headActions.append(langSelector, closeBtn);
    this.headEl.append(titleGroup, headActions);

    // 4. Body
    const bodyEl = document.createElement("div");
    bodyEl.className = "semaps-doc-body";

    // Section 1: Description
    const descSection = document.createElement("div");
    descSection.className = "semaps-doc-section-desc";

    const descLabel = document.createElement("label");
    descLabel.className = "semaps-doc-label";
    descLabel.textContent = i18n.d.dialogs.docEditor.descriptionLabel;

    this.descInput = document.createElement("textarea");
    this.descInput.className = "semaps-doc-desc-input";
    this.descInput.rows = 2;
    this.descInput.placeholder = i18n.d.dialogs.docEditor.descriptionPlaceholder;

    descSection.append(descLabel, this.descInput);

    // Section 2: Main Doc (Markdown)
    const mainSection = document.createElement("div");
    mainSection.className = "semaps-doc-section-main";

    // Toolbar Bar
    const toolbarBar = document.createElement("div");
    toolbarBar.className = "semaps-doc-toolbar-bar";

    const toolbar = document.createElement("div");
    toolbar.className = "semaps-doc-toolbar";

    const createTbBtn = (label: string, title: string, action: () => void) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "semaps-doc-tb-btn";
      btn.innerHTML = label;
      btn.title = title;
      btn.onclick = (e) => {
        e.preventDefault();
        action();
      };
      return btn;
    };

    const sep = () => {
      const s = document.createElement("div");
      s.className = "semaps-doc-tb-sep";
      return s;
    };

    const d = i18n.d.dialogs.docEditor;
    toolbar.append(
      createTbBtn("<b>B</b>", d.tbBold, () => this.insertMarkdown("**", "**", "текст")),
      createTbBtn("<i>I</i>", d.tbItalic, () => this.insertMarkdown("*", "*", "текст")),
      createTbBtn("<b>H</b>", d.tbHeading, () => this.insertMarkdown("### ", "", "Заголовок")),
      sep(),
      createTbBtn("🔗", d.tbLink, () => this.insertMarkdown("[", "](https://...)", "ссылка")),
      createTbBtn("<code>`</code>", d.tbCode, () => this.insertMarkdown("`", "`", "code")),
      createTbBtn("<code>{ }</code>", d.tbCodeBlock, () => this.insertMarkdown("```\n", "\n```", "code")),
      sep(),
      createTbBtn("• list", d.tbList, () => this.insertMarkdown("- ", "", "пункт списка")),
      createTbBtn("1. list", d.tbNumList, () => this.insertMarkdown("1. ", "", "пункт списка")),
      createTbBtn("❝", d.tbQuote, () => this.insertMarkdown("> ", "", "цитата")),
      createTbBtn("📊", d.tbTable, () =>
        this.insertMarkdown(
          "| Параметр | Описание |\n|---|---|\n| Поле 1 | Значение 1 |\n",
          "",
          "",
        ),
      ),
    );

    // View Tabs
    const viewTabs = document.createElement("div");
    viewTabs.className = "semaps-doc-view-tabs";

    this.tabEditBtn = document.createElement("button");
    this.tabEditBtn.className = "semaps-doc-tab-btn";
    this.tabEditBtn.textContent = d.tabEdit;
    this.tabEditBtn.onclick = () => this.setMode("edit");

    this.tabSplitBtn = document.createElement("button");
    this.tabSplitBtn.className = "semaps-doc-tab-btn";
    this.tabSplitBtn.textContent = d.tabSplit;
    this.tabSplitBtn.onclick = () => this.setMode("split");

    this.tabPreviewBtn = document.createElement("button");
    this.tabPreviewBtn.className = "semaps-doc-tab-btn is-active";
    this.tabPreviewBtn.textContent = d.tabPreview;
    this.tabPreviewBtn.onclick = () => this.setMode("preview");

    viewTabs.append(this.tabEditBtn, this.tabSplitBtn, this.tabPreviewBtn);
    toolbarBar.append(toolbar, viewTabs);

    // Content Area (Editor + Preview)
    this.contentAreaEl = document.createElement("div");
    this.contentAreaEl.className = "semaps-doc-content-area mode-preview";

    const editorPane = document.createElement("div");
    editorPane.className = "semaps-doc-editor-pane";

    this.docTextarea = document.createElement("textarea");
    this.docTextarea.className = "semaps-doc-textarea";
    this.docTextarea.placeholder = d.docPlaceholder;
    this.docTextarea.addEventListener("input", () => this.updatePreview());
    this.docTextarea.addEventListener("keydown", (e) => this.handleTextareaKeydown(e));

    editorPane.appendChild(this.docTextarea);

    this.previewPane = document.createElement("div");
    this.previewPane.className = "semaps-doc-preview-pane semaps-markdown-body";

    this.contentAreaEl.append(editorPane, this.previewPane);
    mainSection.append(toolbarBar, this.contentAreaEl);
    bodyEl.append(descSection, mainSection);

    // 5. Footer
    const footEl = document.createElement("div");
    footEl.className = "semaps-doc-foot";

    const hintEl = document.createElement("div");
    hintEl.className = "semaps-doc-foot-hint";
    hintEl.textContent = d.hotkeyHint;

    const footActions = document.createElement("div");
    footActions.className = "semaps-doc-foot-actions";

    const cancelBtn = document.createElement("button");
    cancelBtn.className = "semaps-doc-btn";
    cancelBtn.textContent = i18n.d.common.cancel;
    cancelBtn.onclick = () => this.close();

    const saveBtn = document.createElement("button");
    saveBtn.className = "semaps-doc-btn semaps-doc-btn-primary";
    saveBtn.textContent = i18n.d.common.save;
    saveBtn.onclick = () => this.save();

    footActions.append(cancelBtn, saveBtn);
    footEl.append(hintEl, footActions);

    // Assemble Card
    this.cardEl.append(this.headEl, bodyEl, footEl);
    this.modalEl.appendChild(this.cardEl);
    document.body.appendChild(this.modalEl);

    // Backdrop click close
    this.modalEl.addEventListener("mousedown", (e) => {
      if (e.target === this.modalEl) {
        this.close();
      }
    });

    // Make Draggable
    this.makeDraggable(this.headEl, this.cardEl);

    // Global Key Listener
    window.addEventListener("keydown", (e) => {
      if (!this.modalEl.hidden) {
        if (e.key === "Escape") {
          e.preventDefault();
          this.close();
        } else if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
          e.preventDefault();
          this.save();
        }
      }
    });
  }

  /**
   * Open the editor for a given entity, zone, or edge.
   */
  open(targetId: string, kind?: DocTargetKind, preferredLang?: string): void {
    this.currentTargetId = targetId;
    const doc = this.editor.canvas.model;
    const dataLang = preferredLang || this.editor.dataLang || "ru";
    this.currentLang = dataLang;

    // Detect kind if not specified
    if (!kind) {
      if (doc?.edge(targetId)) {
        this.currentKind = "edge";
      } else {
        const el = doc?.element(targetId);
        this.currentKind = el && isContainer(el) ? "zone" : "node";
      }
    } else {
      this.currentKind = kind;
    }

    this.updateLanguageUI();
    this.loadValues();

    // Reset card position if centered
    this.cardEl.style.transform = "";

    this.modalEl.hidden = false;
    setTimeout(() => {
      if (this.currentMode !== "preview") {
        this.docTextarea.focus();
      }
    }, 50);
  }

  close(): void {
    this.modalEl.hidden = true;
    this.currentTargetId = null;
  }

  setLanguage(lang: string): void {
    if (this.currentLang === lang) return;
    // Auto-save draft or load next language
    this.currentLang = lang;
    this.updateLanguageUI();
    this.loadValues();
  }

  private updateLanguageUI(): void {
    this.langRuBtn.classList.toggle("is-active", this.currentLang === "ru");
    this.langEnBtn.classList.toggle("is-active", this.currentLang === "en");
  }

  private loadValues(): void {
    if (!this.currentTargetId) return;
    const doc = this.editor.canvas.model;
    if (!doc) return;

    let titleText = this.currentTargetId;
    let kindText = this.currentKind.toUpperCase();

    if (this.currentKind === "edge") {
      const edge = doc.edge(this.currentTargetId);
      if (edge) {
        const fromEl = doc.element(edge.from);
        const toEl = doc.element(edge.to);
        const fromName = doc.getText(edge.from, this.currentLang)?.name || fromEl?.label || edge.from;
        const toName = doc.getText(edge.to, this.currentLang)?.name || toEl?.label || edge.to;
        titleText = `${fromName} ➔ ${toName}`;
        kindText = edge.type || "RELATION";
      }
    } else {
      const el = doc.element(this.currentTargetId);
      if (el) {
        titleText = el.label || el.id;
        kindText = el.type || (isContainer(el) ? "ZONE" : "NODE");
      }
    }

    this.titleEl.textContent = titleText;
    this.kindBadgeEl.textContent = kindText;

    const textEntry = doc.getText(this.currentTargetId, this.currentLang);
    const description = textEntry?.description ?? "";
    const docText = textEntry?.doc ?? "";

    this.descInput.value = description;
    this.docTextarea.value = docText;
    this.updatePreview();
  }

  private updatePreview(): void {
    const raw = this.docTextarea.value;
    if (!raw.trim()) {
      this.previewPane.innerHTML = `<div class="semaps-doc-empty-preview">${i18n.d.dialogs.docEditor.previewEmpty}</div>`;
      return;
    }
    this.previewPane.innerHTML = renderMarkdown(raw);
  }

  setMode(mode: "edit" | "split" | "preview"): void {
    this.currentMode = mode;
    this.contentAreaEl.className = `semaps-doc-content-area mode-${mode}`;
    this.tabEditBtn.classList.toggle("is-active", mode === "edit");
    this.tabSplitBtn.classList.toggle("is-active", mode === "split");
    this.tabPreviewBtn.classList.toggle("is-active", mode === "preview");
    if (mode === "preview" || mode === "split") {
      this.updatePreview();
    }
  }

  private insertMarkdown(prefix: string, suffix = "", defaultText = ""): void {
    const ta = this.docTextarea;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const val = ta.value;

    const selected = val.substring(start, end) || defaultText;
    const replacement = prefix + selected + suffix;

    ta.value = val.substring(0, start) + replacement + val.substring(end);
    ta.focus();
    ta.selectionStart = start + prefix.length;
    ta.selectionEnd = start + prefix.length + selected.length;

    this.updatePreview();
  }

  private handleTextareaKeydown(e: KeyboardEvent): void {
    if (e.key === "Tab") {
      e.preventDefault();
      const ta = this.docTextarea;
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      ta.value = ta.value.substring(0, start) + "  " + ta.value.substring(end);
      ta.selectionStart = ta.selectionEnd = start + 2;
      this.updatePreview();
    } else if (layoutFreeKey(e) === "b" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      this.insertMarkdown("**", "**", "жирный");
    } else if (layoutFreeKey(e) === "i" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      this.insertMarkdown("*", "*", "курсив");
    } else if (layoutFreeKey(e) === "k" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      this.insertMarkdown("[", "](https://...)", "ссылка");
    }
  }

  save(): void {
    if (!this.currentTargetId) {
      this.close();
      return;
    }

    const doc = this.editor.canvas.model;
    if (!doc) {
      this.close();
      return;
    }

    const nextDesc = this.descInput.value.trim();
    const nextDoc = this.docTextarea.value.trim();
    const lang = this.currentLang;
    const targetId = this.currentTargetId;

    this.editor.editField(() => {
      // 1. Update centralized text registry
      doc.setText(targetId, { description: nextDesc, doc: nextDoc }, lang);

      // 2. If element, also update element.metadata.description
      const el = doc.element(targetId);
      if (el) {
        el.metadata = {
          ...el.metadata,
          description: nextDesc,
        };
      }

      // 3. If edge, update edge description if needed
      const edge = doc.edge(targetId);
      if (edge) {
        // Edge label remains concise; text registry holds doc and description
      }
    }, { rerender: true });

    this.close();
  }

  private makeDraggable(handle: HTMLElement, target: HTMLElement): void {
    let startX = 0;
    let startY = 0;
    let initialLeft = 0;
    let initialTop = 0;
    let isDragging = false;

    const onMouseDown = (e: MouseEvent) => {
      // Ignore clicks on buttons/inputs inside header
      if ((e.target as HTMLElement).closest("button, input, select")) return;

      isDragging = true;
      startX = e.clientX;
      startY = e.clientY;

      const rect = target.getBoundingClientRect();
      initialLeft = rect.left;
      initialTop = rect.top;

      target.style.position = "fixed";
      target.style.margin = "0";
      target.style.left = `${initialLeft}px`;
      target.style.top = `${initialTop}px`;
      target.style.transform = "none";

      window.addEventListener("mousemove", onMouseMove);
      window.addEventListener("mouseup", onMouseUp);
    };

    const onMouseMove = (e: MouseEvent) => {
      if (!isDragging) return;
      const dx = e.clientX - startX;
      const dy = e.clientY - startY;
      target.style.left = `${Math.max(10, initialLeft + dx)}px`;
      target.style.top = `${Math.max(10, initialTop + dy)}px`;
    };

    const onMouseUp = () => {
      isDragging = false;
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };

    handle.addEventListener("mousedown", onMouseDown);
  }
}
