import { paint } from "../model/StyleLibrary.js";
import type {
  Endpoint,
  Paint,
  ResolvedBlockStyle,
  ResolvedEdgeStyle,
  Stroke,
  TextStyle,
} from "../model/StyleLibrary.js";
import type {
  EndShape,
  StyleTarget,
  WireEndpoint,
  WireGradientStop,
  WirePaint,
  WireStroke,
  WireStyle,
  WireText,
} from "../model/style-types.js";
import { el, replaceChildren } from "../util/dom.js";
import {
  colorField,
  field,
  optionalBoolField,
  optionalColorField,
  optionalNumberField,
  optionalSelectField,
  optionalTextField,
  select,
  type Option,
} from "./fields.js";
import { blockPreview, edgePreview, paintPreview } from "./style-preview.js";
import type { StylePanelHost } from "./StyleList.js";
import { i18n } from "../workbench/i18n/I18nService.js";

/**
 * One form for both kinds of style.
 *
 * Blocks and edges share most of what a style *is* — a stroke, a text slot, an
 * inheritance parent, a name — and only diverge at the ends. Two forms would
 * mean two implementations of the sparse-field rule below, drifting apart the
 * first time one of them was fixed. So there is one vocabulary of editors
 * (`paintEditor`, `strokeEditor`, `textEditor`, `endpointEditor`) and a render
 * that shows whichever sections apply to `appliesTo`.
 *
 * The rule everything here obeys: a `WireStyle` is sparse, and an empty field
 * means *inherit*, not *empty*. Clearing a control removes the key entirely
 * (see `prune`), and the value that would take over is shown as the control's
 * placeholder — resolved from `basedOn`, not from the style itself, so the
 * placeholder answers "what happens if I clear this" rather than echoing back
 * the value already in the box.
 */
export class StyleEditor {
  private id: string | null = null;
  /**
   * Live previews to refresh after an edit.
   *
   * Rebuilt on every render and run instead of a re-render for edits typed into
   * a text field, because recreating the form would move the caret out of the
   * field mid-word.
   */
  private previews: Array<() => void> = [];

  constructor(
    private readonly mount: HTMLElement,
    private readonly host: StylePanelHost,
  ) {}

  get openId(): string | null {
    return this.id;
  }

  open(id: string | null): void {
    this.id = id;
    this.render();
  }

  render(): void {
    this.previews = [];
    const style = this.currentStyle();

    if (style === undefined) {
      replaceChildren(
        this.mount,
        el("div", { class: "inspector-empty" }, [
          el("p", { class: "inspector-empty-icon", text: "🎨" }),
          el("p", {
            text: i18n.d.panels.styles.emptyEditor,
          }),
        ]),
      );
      return;
    }

    const target: StyleTarget = style.appliesTo ?? "block";
    replaceChildren(
      this.mount,
      this.headerSection(style, target),
      target === "block" ? this.blockFillSection(style) : null,
      target === "block" ? this.blockBorderSection(style) : this.edgeLineSection(style),
      target === "block" ? this.blockTextSection(style) : this.edgeLabelSection(style),
      target === "block" ? this.iconSection(style) : null,
      target === "block" ? this.headerBandSection(style) : null,
      target === "edge" ? this.endsSection(style) : null,
      target === "edge" ? this.familySection(style) : null,
    );
  }

  // ---------------------------------------------------------------- plumbing

  private currentStyle(): WireStyle | undefined {
    return this.id === null ? undefined : this.host.styles.get(this.id);
  }

  /**
   * Apply one change to the open style.
   *
   * Every mutation reads and writes the *draft*, never the values captured when
   * the form was built: two controls edited between renders would otherwise
   * each overwrite the other's group with its own stale copy.
   */
  private patch(mutate: (draft: WireStyle) => void, options: { rerender?: boolean } = {}): void {
    const id = this.id;
    if (id === null) return;
    const current = this.host.styles.get(id);
    if (current === undefined) return;

    const draft = structuredClone(current);
    mutate(draft);
    prune(draft);
    this.host.editStyle(() => {
      this.host.styles.put(draft);
    });

    if (options.rerender === true) this.render();
    else for (const refresh of this.previews) refresh();
  }

  private inheritedBlock(style: WireStyle): ResolvedBlockStyle {
    return this.host.styles.resolveBlock(style.basedOn ?? null);
  }

  private inheritedEdge(style: WireStyle): ResolvedEdgeStyle {
    return this.host.styles.resolveEdge(style.basedOn ?? null);
  }

  private section(
    title: string,
    open: boolean,
    children: readonly (Node | null)[],
  ): HTMLElement {
    const details = el("details", { class: "style-section" }, [
      el("summary", { text: title }),
      el("div", { class: "style-section-body" }, children),
    ]);
    details.open = open;
    return details;
  }

  /** A thumbnail that re-reads the library whenever something changes. */
  private livePreview(target: StyleTarget): HTMLElement {
    const host = el("div", { class: "style-preview-host" });
    const refresh = (): void => {
      const id = this.id;
      if (id === null) return;
      replaceChildren(
        host,
        target === "block"
          ? blockPreview(this.host.styles.resolveBlock(id))
          : edgePreview(this.host.styles.resolveEdge(id)),
      );
    };
    refresh();
    this.previews.push(refresh);
    return host;
  }

  // ------------------------------------------------------------------ header

  private headerSection(style: WireStyle, target: StyleTarget): HTMLElement {
    const idInput = el("input", { class: "mono", type: "text", value: style.id });
    const error = el("div", { class: "style-error", hidden: true });

    const parents: Option[] = [
      ["", i18n.d.panels.styles.noneInherit],
      ...this.host.styles
        .list(target)
        // A style cannot be its own parent: the library survives the cycle, but
        // the result is a style that silently stops inheriting anything.
        .filter((entry) => entry.id !== style.id)
        .map((entry) => [entry.id, `${entry.name} · ${entry.id}`] as Option),
    ];

    return this.section(i18n.d.panels.styles.headerSection, true, [
      this.livePreview(target),
      field(
        i18n.d.panels.styles.nameField,
        el("input", {
          class: "input-strong",
          type: "text",
          value: style.name ?? "",
          placeholder: style.id,
          on: {
            input: (e) => {
              const next = (e.target as HTMLInputElement).value;
              this.patch((d) => {
                d.name = next.trim() === "" ? undefined : next;
              });
            },
          },
        }),
      ),
      el("div", { class: "field" }, [
        el("span", { class: "field-label", text: i18n.d.panels.styles.idField }),
        el("div", { class: "field-row gap" }, [
          idInput,
          el("button", {
            class: "btn btn-small",
            text: i18n.d.panels.styles.renameBtn,
            on: {
              click: () => {
                error.hidden = true;
                this.renameStyle(style, idInput.value.trim(), error);
              },
            },
          }),
        ]),
        error,
      ]),
      field(
        i18n.d.panels.styles.descriptionField,
        el("textarea", {
          rows: 2,
          value: style.description ?? "",
          placeholder: i18n.d.panels.styles.descriptionPlaceholder,
          on: {
            input: (e) => {
              const next = (e.target as HTMLTextAreaElement).value;
              this.patch((d) => {
                d.description = next.trim() === "" ? undefined : next;
              });
            },
          },
        }),
      ),
      field(
        i18n.d.panels.styles.tagsField,
        el("input", {
          type: "text",
          value: (style.tags ?? []).join(", "),
          placeholder: i18n.d.panels.styles.tagsPlaceholder,
          on: {
            input: (e) => {
              const raw = (e.target as HTMLInputElement).value;
              const tags = raw
                .split(",")
                .map((t) => t.trim())
                .filter((t) => t !== "");
              this.patch((d) => {
                d.tags = tags;
              });
            },
          },
        }),
      ),
      field(
        i18n.d.panels.styles.basedOnField,
        select(parents, style.basedOn ?? "", (next) => {
          this.patch(
            (d) => {
              d.basedOn = next === "" ? undefined : next;
            },
            // Every placeholder in the form comes from the parent, so the whole
            // form is stale the moment the parent changes.
            { rerender: true },
          );
        }),
      ),
    ]);
  }

  /**
   * Rename, carrying the references along.
   *
   * `StyleLibrary.rename` repoints `basedOn` but knows nothing about the
   * document, so elements that named the old id explicitly are repointed here.
   * Elements that matched it by `type` cannot be: pinning them with a `styleId`
   * would turn "this is a record" into "this box is painted like that", which
   * is the coupling styles exist to remove — so those are reported instead.
   */
  private renameStyle(style: WireStyle, next: string, error: HTMLElement): void {
    if (next === "" || next === style.id) return;

    const lib = this.host.styles;
    if (lib.has(next)) {
      error.hidden = false;
      error.textContent = `Идентификатор «${next}» уже занят другим стилем.`;
      return;
    }

    const doc = this.host.canvas.model;
    let byType = 0;
    if (doc !== null) {
      for (const element of doc.elements()) {
        if (element.styleId === undefined && element.type === style.id) byType += 1;
      }
      for (const edge of doc.edges) {
        if (edge.styleId === undefined && edge.type === style.id) byType += 1;
      }
    }
    if (byType > 0) {
      const ok = window.confirm(
        `Элементов, которые сейчас берут этот стиль по совпадению type = «${style.id}»: ${byType}.\n` +
          `После переименования они вернутся к стилю по умолчанию.\n\nПереименовать всё равно?`,
      );
      if (!ok) return;
    }

    this.host.commitStyle(() => {
      lib.rename(style.id, next);
      if (doc === null) return;
      for (const element of doc.elements()) {
        if (element.styleId === style.id) element.styleId = next;
      }
      for (const edge of doc.edges) {
        if (edge.styleId === style.id) edge.styleId = next;
      }
    });
    this.host.openStyle(next);
  }

  // ------------------------------------------------------------------ blocks

  private blockFillSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedBlock(style);
    return this.section(i18n.d.panels.styles.fillSection, true, [
      this.paintEditor(
        i18n.d.panels.styles.fillSection,
        (s) => s.fill,
        (s, next) => {
          s.fill = next;
        },
        inherited.fill,
      ),
    ]);
  }

  private blockBorderSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedBlock(style);
    return this.section(i18n.d.panels.styles.borderSection, true, [
      this.strokeEditor(
        (s) => s.border,
        (s, next) => {
          s.border = next;
        },
        inherited.border,
      ),
      el("div", { class: "grid-2" }, [
        optionalNumberField(i18n.d.panels.styles.radiusField, style.radius, inherited.radius, (v) => {
          this.patch((d) => {
            d.radius = v;
          });
        }, { min: 0, step: 1 }),
        optionalBoolField(i18n.d.panels.styles.shadowField, style.shadow, inherited.shadow, (v) => {
          this.patch((d) => {
            d.shadow = v;
          });
        }),
      ]),
    ]);
  }

  private blockTextSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedBlock(style);
    return this.section(i18n.d.panels.styles.textSection, false, [
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.titleField }),
      this.textEditor(
        (s) => s.title,
        (s, next) => {
          s.title = next;
        },
        inherited.title,
      ),
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.labelField }),
      this.textEditor(
        (s) => s.subtitle,
        (s, next) => {
          s.subtitle = next;
        },
        inherited.subtitle,
      ),
    ]);
  }

  private iconSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedBlock(style);
    return this.section(i18n.d.panels.styles.iconSection, false, [
      el("div", { class: "grid-2" }, [
        optionalTextField(i18n.d.panels.styles.glyphField, style.icon?.glyph, inherited.icon.glyph, (v) => {
          this.patch((d) => {
            d.icon = { ...d.icon, glyph: v };
          });
        }),
        optionalBoolField(i18n.d.panels.styles.showField, style.icon?.show, inherited.icon.show, (v) => {
          this.patch((d) => {
            d.icon = { ...d.icon, show: v };
          });
        }),
      ]),
    ]);
  }

  private headerBandSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedBlock(style);
    return this.section(i18n.d.panels.styles.headerBandSection, false, [
      el("div", {
        class: "panel panel-info",
        text: i18n.d.panels.styles.headerBandHint,
      }),
      this.paintEditor(
        i18n.d.panels.styles.headerFillField,
        (s) => s.header?.fill,
        (s, next) => {
          s.header = { ...s.header, fill: next };
        },
        inherited.header.fill,
      ),
      optionalNumberField(i18n.d.panels.styles.headerHeightField, style.header?.height, inherited.header.height, (v) => {
        this.patch((d) => {
          d.header = { ...d.header, height: v };
        });
      }, { min: 0, step: 1 }),
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.headerTextField }),
      this.textEditor(
        (s) => s.header?.text,
        (s, next) => {
          s.header = { ...s.header, text: next };
        },
        inherited.header.text,
      ),
    ]);
  }

  // ------------------------------------------------------------------- edges

  private edgeLineSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedEdge(style);
    return this.section(i18n.d.panels.styles.lineSection, true, [
      this.strokeEditor(
        (s) => s.line,
        (s, next) => {
          s.line = next;
        },
        inherited.line,
      ),
    ]);
  }

  private edgeLabelSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedEdge(style);
    return this.section(i18n.d.panels.styles.edgeLabelSection, false, [
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.labelField }),
      this.textEditor(
        (s) => s.label,
        (s, next) => {
          s.label = next;
        },
        inherited.label,
      ),
    ]);
  }

  private endsSection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedEdge(style);
    return this.section(i18n.d.panels.styles.endsSection, true, [
      this.livePreview("edge"),
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.startEndpoint }),
      this.endpointEditor(
        (s) => s.source,
        (s, next) => {
          s.source = next;
        },
        inherited.source,
      ),
      el("div", { class: "field-label accent", text: i18n.d.panels.styles.endEndpoint }),
      this.endpointEditor(
        (s) => s.target,
        (s, next) => {
          s.target = next;
        },
        inherited.target,
      ),
    ]);
  }

  private familySection(style: WireStyle): HTMLElement {
    const inherited = this.inheritedEdge(style);
    return this.section(i18n.d.panels.styles.familySection, false, [
      el("div", {
        class: "panel panel-info",
        text: i18n.d.panels.styles.familyHint,
      }),
      optionalSelectField(
        i18n.d.panels.styles.familyField,
        style.family,
        [
          ["structure", i18n.d.panels.styles.familyStructure],
          ["flow", i18n.d.panels.styles.familyFlow],
        ],
        inherited.family === "structure" ? i18n.d.panels.styles.familyStructure : i18n.d.panels.styles.familyFlow,
        (v) => {
          this.patch((d) => {
            d.family = v;
          });
        },
      ),
      optionalBoolField(
        i18n.d.panels.styles.overviewField,
        style.overview,
        inherited.overview,
        (v) => {
          this.patch((d) => {
            d.overview = v;
          });
        },
      ),
    ]);
  }

  // -------------------------------------------------------- shared editors

  /**
   * Fill: solid, linear gradient or radial gradient.
   *
   * Shared by the block fill and the container header band, because they are
   * the same `WirePaint` and a header that could only be a flat colour would be
   * an arbitrary restriction of the contract.
   */
  private paintEditor(
    label: string,
    read: (style: WireStyle) => WirePaint | undefined,
    write: (style: WireStyle, next: WirePaint | undefined) => void,
    inherited: Paint,
  ): HTMLElement {
    const style = this.currentStyle();
    if (style === undefined) return el("div");
    const value = read(style);
    const kind = value === undefined ? "" : typeof value === "string" ? "solid" : value.kind;

    const rows: Array<Node | null> = [
      field(
        label,
        select(
          [
            ["", `— как в основе (${describePaint(inherited)}) —`],
            ["solid", "сплошная"],
            ["linear", "линейный градиент"],
            ["radial", "радиальный градиент"],
          ],
          kind,
          (next) => {
            this.patch(
              (d) => {
                write(d, switchPaintKind(next, read(d), inherited));
              },
              { rerender: true },
            );
          },
        ),
      ),
    ];

    if (kind === "solid") {
      rows.push(
        colorField("Цвет", typeof value === "string" ? value : "", solidOf(inherited), (next) => {
          this.patch((d) => {
            write(d, next.trim() === "" ? undefined : next);
          });
        }),
      );
    } else if (value !== undefined && typeof value !== "string") {
      const gradient = value;

      const previewHost = el("div", { class: "paint-preview-host" });
      const refresh = (): void => {
        const current = this.currentStyle();
        if (current === undefined) return;
        replaceChildren(previewHost, paintPreview(paint(read(current), inherited)));
      };
      refresh();
      this.previews.push(refresh);
      rows.push(previewHost);

      if (gradient.kind === "linear") {
        rows.push(
          optionalNumberField(
            "Угол (° по часовой, 90 — сверху вниз)",
            gradient.angle,
            90,
            (v) => {
              this.patch((d) => {
                const current = read(d);
                if (current === undefined || typeof current === "string" || current.kind !== "linear") return;
                write(d, { ...current, angle: v });
              });
            },
            { min: 0, max: 360, step: 5 },
          ),
        );
      }

      rows.push(
        el(
          "div",
          { class: "stop-list" },
          gradient.stops.map((stop, index) =>
            this.stopRow(stop, index, gradient.stops.length, read, write),
          ),
        ),
        el("button", {
          class: "btn btn-small full",
          text: "＋ Добавить стоп",
          on: {
            click: () => {
              this.patch(
                (d) => {
                  const current = read(d);
                  if (current === undefined || typeof current === "string") return;
                  const last = current.stops.at(-1);
                  write(d, {
                    ...current,
                    stops: [
                      ...current.stops,
                      { offset: 1, color: last?.color ?? "#ffffff", opacity: 1 },
                    ],
                  });
                },
                { rerender: true },
              );
            },
          },
        }),
      );
    }

    return el("div", { class: "sub-group" }, rows);
  }

  private stopRow(
    stop: WireGradientStop,
    index: number,
    total: number,
    read: (style: WireStyle) => WirePaint | undefined,
    write: (style: WireStyle, next: WirePaint | undefined) => void,
  ): HTMLElement {
    const editStop = (mutate: (s: WireGradientStop) => WireGradientStop): void => {
      this.patch((d) => {
        const current = read(d);
        if (current === undefined || typeof current === "string") return;
        write(d, {
          ...current,
          stops: current.stops.map((s, i) => (i === index ? mutate(s) : s)),
        });
      });
    };

    return el("div", { class: "stop-row" }, [
      el("input", {
        class: "stop-offset",
        type: "number",
        title: "Позиция 0…1",
        value: String(stop.offset),
        attrs: { min: "0", max: "1", step: "0.05" },
        on: {
          input: (e) => {
            const v = Number((e.target as HTMLInputElement).value);
            if (!Number.isFinite(v)) return;
            editStop((s) => ({ ...s, offset: Math.min(1, Math.max(0, v)) }));
          },
        },
      }),
      el("input", {
        class: "color-swatch",
        type: "color",
        title: "Цвет стопа",
        value: stop.color,
        on: {
          input: (e) => {
            const v = (e.target as HTMLInputElement).value;
            editStop((s) => ({ ...s, color: v }));
          },
        },
      }),
      el("input", {
        class: "mono stop-color",
        type: "text",
        value: stop.color,
        on: {
          input: (e) => {
            const v = (e.target as HTMLInputElement).value;
            editStop((s) => ({ ...s, color: v }));
          },
        },
      }),
      el("input", {
        class: "stop-opacity",
        type: "number",
        title: "Прозрачность 0…1",
        value: String(stop.opacity ?? 1),
        attrs: { min: "0", max: "1", step: "0.1" },
        on: {
          input: (e) => {
            const v = Number((e.target as HTMLInputElement).value);
            if (!Number.isFinite(v)) return;
            editStop((s) => ({ ...s, opacity: Math.min(1, Math.max(0, v)) }));
          },
        },
      }),
      el("button", {
        class: "btn-icon danger",
        text: "✕",
        title: "Удалить стоп",
        // Two stops is the floor: with fewer the library ignores the gradient
        // entirely and falls back, so removing one more would blank the fill
        // with no visible reason.
        disabled: total <= 2,
        on: {
          click: () => {
            this.patch(
              (d) => {
                const current = read(d);
                if (current === undefined || typeof current === "string") return;
                write(d, { ...current, stops: current.stops.filter((_, i) => i !== index) });
              },
              { rerender: true },
            );
          },
        },
      }),
    ]);
  }

  /** Border of a block or line of an edge — the same four fields. */
  private strokeEditor(
    read: (style: WireStyle) => WireStroke | undefined,
    write: (style: WireStyle, next: WireStroke) => void,
    inherited: Stroke,
  ): HTMLElement {
    const style = this.currentStyle();
    if (style === undefined) return el("div");
    const value = read(style) ?? {};

    const isPreset = DASH_PRESETS.some(([v]) => v === value.dash);
    const dashChoice = value.dash === undefined ? "" : isPreset ? value.dash : "custom";

    return el("div", { class: "sub-group" }, [
      optionalColorField("Цвет", value.color, inherited.color, (v) => {
        this.patch((d) => {
          write(d, { ...read(d), color: v });
        });
      }),
      el("div", { class: "grid-2" }, [
        optionalNumberField("Толщина", value.width, inherited.width, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), width: v });
          });
        }, { min: 0, step: 0.1 }),
        optionalNumberField("Прозрачность", value.opacity, inherited.opacity, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), opacity: v });
          });
        }, { min: 0, max: 1, step: 0.1 }),
      ]),
      field(
        "Штрих",
        select(
          [
            ["", `— как в основе (${inherited.dash}) —`],
            ...DASH_PRESETS,
            ["custom", "произвольный…"],
          ],
          dashChoice,
          (next) => {
            this.patch(
              (d) => {
                const dash =
                  next === "" ? undefined : next === "custom" ? value.dash ?? "4,2" : next;
                write(d, { ...read(d), dash });
              },
              { rerender: true },
            );
          },
        ),
      ),
      dashChoice === "custom"
        ? field(
            "Свой шаблон (SVG dash-array)",
            el("input", {
              class: "mono",
              type: "text",
              value: value.dash ?? "",
              placeholder: "например 10,3,2,3",
              on: {
                input: (e) => {
                  const v = (e.target as HTMLInputElement).value;
                  this.patch((d) => {
                    write(d, { ...read(d), dash: v.trim() === "" ? undefined : v });
                  });
                },
              },
            }),
          )
        : null,
    ]);
  }

  /** One text slot: title, subtitle, header caption or edge label. */
  private textEditor(
    read: (style: WireStyle) => WireText | undefined,
    write: (style: WireStyle, next: WireText) => void,
    inherited: TextStyle,
  ): HTMLElement {
    const style = this.currentStyle();
    if (style === undefined) return el("div");
    const value = read(style) ?? {};

    return el("div", { class: "sub-group" }, [
      el("div", { class: "grid-2" }, [
        optionalBoolField("Показывать", value.show, inherited.show, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), show: v });
          });
        }),
        optionalBoolField("Курсив", value.italic, inherited.italic, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), italic: v });
          });
        }),
      ]),
      optionalTextField("Шрифт", value.family, inherited.family, (v) => {
        this.patch((d) => {
          write(d, { ...read(d), family: v });
        });
      }),
      el("div", { class: "grid-2" }, [
        optionalNumberField("Размер", value.size, inherited.size, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), size: v });
          });
        }, { min: 1, step: 0.5 }),
        optionalNumberField("Насыщенность", value.weight, inherited.weight, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), weight: v });
          });
        }, { min: 100, max: 900, step: 100 }),
      ]),
      optionalColorField("Цвет", value.color, inherited.color, (v) => {
        this.patch((d) => {
          write(d, { ...read(d), color: v });
        });
      }),
      el("div", { class: "grid-2" }, [
        optionalSelectField(
          "Выравнивание",
          value.align,
          ALIGN_OPTIONS,
          alignLabel(inherited.align),
          (v) => {
            this.patch((d) => {
              write(d, { ...read(d), align: v });
            });
          },
        ),
        optionalNumberField("Прозрачность", value.opacity, inherited.opacity, (v) => {
          this.patch((d) => {
            write(d, { ...read(d), opacity: v });
          });
        }, { min: 0, max: 1, step: 0.1 }),
      ]),
    ]);
  }

  /** One end of an edge. Both ends are the same three fields, independently. */
  private endpointEditor(
    read: (style: WireStyle) => WireEndpoint | undefined,
    write: (style: WireStyle, next: WireEndpoint) => void,
    inherited: Endpoint,
  ): HTMLElement {
    const style = this.currentStyle();
    if (style === undefined) return el("div");
    const value = read(style) ?? {};

    return el("div", { class: "sub-group" }, [
      optionalSelectField(
        "Форма",
        value.shape,
        SHAPE_OPTIONS,
        SHAPE_LABELS[inherited.shape],
        (v) => {
          this.patch((d) => {
            write(d, { ...read(d), shape: v });
          });
        },
      ),
      optionalNumberField("Размер", value.size, inherited.size, (v) => {
        this.patch((d) => {
          write(d, { ...read(d), size: v });
        });
      }, { min: 1, step: 1 }),
      optionalColorField(
        "Цвет (пусто — по линии)",
        value.color,
        inherited.color ?? "по цвету линии",
        (v) => {
          this.patch((d) => {
            write(d, { ...read(d), color: v });
          });
        },
      ),
    ]);
  }
}

// ------------------------------------------------------------------ vocabulary

const DASH_PRESETS: readonly Option[] = [
  ["none", "сплошная"],
  ["6,4", "6,4 — пунктир"],
  ["4,4", "4,4 — средний"],
  ["3,3", "3,3 — мелкий"],
  ["2,3", "2,3 — точки"],
  ["8,4", "8,4 — длинный"],
];

const ALIGN_OPTIONS: readonly Option[] = [
  ["start", "слева"],
  ["middle", "по центру"],
  ["end", "справа"],
];

function alignLabel(align: TextStyle["align"]): string {
  return align === "start" ? "слева" : align === "middle" ? "по центру" : "справа";
}

const SHAPE_LABELS: Readonly<Record<EndShape, string>> = {
  none: "нет",
  arrow: "стрелка",
  "arrow-open": "стрелка (открытая)",
  triangle: "треугольник",
  "triangle-hollow": "треугольник (полый)",
  diamond: "ромб",
  "diamond-hollow": "ромб (полый)",
  circle: "круг",
  "circle-hollow": "круг (полый)",
  bar: "черта",
};

const SHAPE_OPTIONS: readonly Option[] = (
  Object.keys(SHAPE_LABELS) as EndShape[]
).map((shape) => [shape, SHAPE_LABELS[shape]] as Option);

function describePaint(p: Paint): string {
  if (p.kind === "solid") return p.color;
  return p.kind === "linear" ? "линейный градиент" : "радиальный градиент";
}

/** A representative flat colour for a paint, for swatches and conversions. */
function solidOf(p: Paint): string {
  return p.kind === "solid" ? p.color : p.stops[0]?.color ?? "#ffffff";
}

/**
 * Move a fill between kinds without losing what the user already chose.
 *
 * Switching solid → gradient seeds the first stop from the colour that was
 * there, and gradient → solid keeps the first stop's colour, so flipping the
 * selector twice by accident does not reset the field to white.
 */
function switchPaintKind(
  choice: string,
  current: WirePaint | undefined,
  inherited: Paint,
): WirePaint | undefined {
  if (choice === "") return undefined;

  const base =
    typeof current === "string"
      ? current
      : current !== undefined
        ? current.stops[0]?.color ?? solidOf(inherited)
        : solidOf(inherited);

  if (choice === "solid") return base;

  const stops: WireGradientStop[] =
    current !== undefined && typeof current !== "string" && current.stops.length >= 2
      ? current.stops
      : [
          { offset: 0, color: base, opacity: 1 },
          { offset: 1, color: "#ffffff", opacity: 1 },
        ];

  if (choice === "radial") return { kind: "radial", stops };
  const angle = current !== undefined && typeof current !== "string" && current.kind === "linear"
    ? current.angle ?? 90
    : 90;
  return { kind: "linear", angle, stops };
}

// ----------------------------------------------------------------- pruning

const GROUP_KEYS = [
  "border",
  "line",
  "title",
  "subtitle",
  "label",
  "icon",
  "source",
  "target",
] as const;

/**
 * Remove everything that means nothing.
 *
 * The form writes `undefined` when a control is cleared, and a group that has
 * lost its last field is not "a border with no properties" — it is no border
 * override at all. Left in place, `{ "border": {} }` would litter the saved
 * file and, worse, read as an intentional statement about the border when the
 * user had simply emptied the boxes.
 */
function prune(style: WireStyle): void {
  const bag = style as unknown as Record<string, unknown>;
  stripUndefined(bag);

  for (const key of GROUP_KEYS) {
    const group = bag[key];
    if (!isPlainObject(group)) continue;
    stripUndefined(group);
    if (Object.keys(group).length === 0) delete bag[key];
  }

  const header = style.header as unknown as Record<string, unknown> | undefined;
  if (header !== undefined) {
    const text = header.text;
    if (isPlainObject(text)) {
      stripUndefined(text);
      if (Object.keys(text).length === 0) delete header.text;
    }
    stripUndefined(header);
    if (Object.keys(header).length === 0) delete style.header;
  }

  if (style.tags !== undefined && style.tags.length === 0) delete style.tags;
}

function stripUndefined(bag: Record<string, unknown>): void {
  for (const key of Object.keys(bag)) {
    if (bag[key] === undefined) delete bag[key];
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
