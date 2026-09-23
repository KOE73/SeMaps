import type { TemplateError } from "../content/template-types.js";
import type { AssetEntry } from "../assets/AssetRegistry.js";
import { el, replaceChildren } from "../util/dom.js";
import type { TemplatePanelHost } from "./TemplateList.js";
import { i18n } from "../workbench/i18n/I18nService.js";

/**
 * The live text editor for one content template (ADR_20260903 §2.3).
 *
 * The text is the source of truth, so every keystroke goes straight to
 * `TemplateLibrary.setText` — which compiles and caches the tree in the same
 * call — and then repaints the canvas. `compileTemplate` never throws
 * (`template-types.ts`), so the errors it returns are just data to show under
 * the box; the canvas keeps drawing the last tree that came out of the
 * compiler, broken text or not, which is the whole acceptance test for this
 * panel (ADR §2.3, PLAN_20260903 §"поток B" — "намеренно сломанный текст не
 * роняет холст").
 *
 * Saving is a separate, deliberate act (`TemplateLibrary.save`): the text
 * typed here is shared by every element wearing the style, and half-typed
 * text must never reach `templates.json`.
 */
export class TemplateEditor {
  private id: string | null = null;
  private errorsHost: HTMLElement | null = null;
  private textarea: HTMLTextAreaElement | null = null;
  private assetSelect: HTMLSelectElement | null = null;
  private assetsRequested = false;

  constructor(
    private readonly mount: HTMLElement,
    private readonly host: TemplatePanelHost,
  ) {}

  get openId(): string | null {
    return this.id;
  }

  open(id: string | null): void {
    this.id = id;
    this.assetsRequested = false;
    this.render();
  }

  render(): void {
    this.errorsHost = null;
    this.textarea = null;
    this.assetSelect = null;

    const id = this.id;
    if (id === null) {
      replaceChildren(
        this.mount,
        el("div", { class: "inspector-empty" }, [
          el("p", { class: "inspector-empty-icon", text: "📐" }),
          el("p", { text: i18n.d.panels.templates.emptyEditor }),
        ]),
      );
      return;
    }

    const templates = this.host.canvas.templates;
    const entry = templates.list().find((t) => t.id === id);
    if (entry === undefined) {
      replaceChildren(
        this.mount,
        el("div", { class: "inspector-empty" }, [el("p", { text: i18n.d.panels.templates.notFound })]),
      );
      return;
    }

    const errorsHost = el("div", { class: "template-errors" });
    this.errorsHost = errorsHost;

    const textarea = el("textarea", {
      class: "mono template-textarea",
      rows: 14,
      value: templates.text(id) ?? "",
      placeholder: i18n.d.panels.templates.textPlaceholder,
      on: {
        input: (e) => {
          this.applyText((e.target as HTMLTextAreaElement).value);
        },
      },
    });
    this.textarea = textarea;
    this.renderErrors(templates.get(id)?.errors ?? []);

    const assetSection = this.assetInsertSection();

    const saveBtn = el("button", {
      class: "btn btn-primary btn-small",
      text: i18n.d.panels.templates.saveBtn,
      title: i18n.d.panels.templates.saveHint,
      on: {
        click: () => {
          void this.save();
        },
      },
    });

    replaceChildren(
      this.mount,
      el("div", { class: "style-section-body template-editor-body" }, [
        el("div", { class: "field-row" }, [
          el("span", { class: "style-row-name", text: entry.name ?? entry.id }),
          el("span", { class: "mono muted style-row-id", text: entry.id }),
        ]),
        entry.description
          ? el("div", { class: "muted italic", text: entry.description })
          : null,
        el("div", { class: "field-label accent", text: i18n.d.panels.templates.textField }),
        textarea,
        errorsHost,
        assetSection,
        el("div", { class: "panel panel-info", text: i18n.d.panels.templates.saveHint }),
        el("div", { class: "field-row" }, [saveBtn]),
      ]),
    );
  }

  // ------------------------------------------------------------------ edits

  /**
   * Compile and repaint on every keystroke, without touching the DOM node the
   * caret is sitting in.
   *
   * Only `errorsHost` is replaced — rebuilding the whole pane (as a full
   * `render()` would) recreates the `<textarea>` and throws the caret to the
   * end of the field on every character, which makes the box unusable.
   */
  private applyText(text: string): void {
    const id = this.id;
    if (id === null) return;
    const result = this.host.canvas.templates.setText(id, text);
    this.renderErrors(result.errors);
    // The compiled tree lives only in memory (ADR_20260903 §2.3) — repainting
    // is how "type and see it happen" actually happens.
    this.host.canvas.render();
  }

  private renderErrors(errors: readonly TemplateError[]): void {
    if (this.errorsHost === null) return;
    if (errors.length === 0) {
      replaceChildren(this.errorsHost);
      return;
    }
    replaceChildren(
      this.errorsHost,
      ...errors.map((err) =>
        el("div", {
          class: "style-error template-error",
          text: `${err.line}:${err.column} — ${err.message}`,
        }),
      ),
    );
  }

  private async save(): Promise<void> {
    try {
      await this.host.canvas.templates.save();
      this.host.notify(i18n.d.panels.templates.saveOk);
    } catch (e) {
      this.host.notify(
        i18n.format(i18n.d.panels.templates.saveFailed, {
          error: e instanceof Error ? e.message : String(e),
        }),
      );
    }
  }

  // ----------------------------------------------------------------- assets

  /**
   * A picker for `@Asset(name)`, listing what `AssetRegistry` can hand out.
   *
   * Fetched once per open rather than eagerly for every template: most
   * templates never touch `@Asset`, and the manifest read is cheap to defer
   * until this pane is actually looked at.
   */
  private assetInsertSection(): HTMLElement {
    const select = el("select", { class: "template-asset-select" }, [
      el("option", { value: "", text: i18n.d.panels.templates.assetLoading }),
    ]);
    this.assetSelect = select;

    const insertBtn = el("button", {
      class: "btn btn-small",
      text: i18n.d.panels.templates.assetInsertBtn,
      disabled: true,
      on: {
        click: () => {
          const id = select.value;
          if (id !== "") this.insertAtCursor(`@Asset(${id})`);
        },
      },
    });

    if (!this.assetsRequested) {
      this.assetsRequested = true;
      void this.host.canvas.assets.list().then((entries) => {
        this.populateAssetSelect(select, insertBtn, entries);
      });
    }

    return el("div", { class: "field-row gap template-asset-row" }, [select, insertBtn]);
  }

  private populateAssetSelect(
    select: HTMLSelectElement,
    insertBtn: HTMLButtonElement,
    entries: readonly AssetEntry[],
  ): void {
    // The pane may have moved on to a different template (or closed) by the
    // time the manifest arrives; only touch the DOM if this select is still
    // the one on screen.
    if (this.assetSelect !== select) return;

    if (entries.length === 0) {
      replaceChildren(select, el("option", { value: "", text: i18n.d.panels.templates.assetEmpty }));
      return;
    }

    replaceChildren(
      select,
      el("option", { value: "", text: i18n.d.panels.templates.assetChoose }),
      ...entries.map((entry) =>
        el("option", { value: entry.id, text: entry.name ? `${entry.name} (${entry.id})` : entry.id }),
      ),
    );
    insertBtn.disabled = false;
  }

  private insertAtCursor(snippet: string): void {
    const textarea = this.textarea;
    if (textarea === null) return;
    const start = textarea.selectionStart ?? textarea.value.length;
    const end = textarea.selectionEnd ?? textarea.value.length;
    const next = textarea.value.slice(0, start) + snippet + textarea.value.slice(end);
    textarea.value = next;
    const caret = start + snippet.length;
    textarea.focus();
    textarea.setSelectionRange(caret, caret);
    this.applyText(next);
  }
}
