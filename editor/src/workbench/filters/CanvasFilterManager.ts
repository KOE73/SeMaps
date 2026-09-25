import { el } from "../../util/dom.js";
import { i18n } from "../i18n/I18nService.js";

export type TintMode = "none" | "warm" | "slate" | "cool";

export interface CanvasFilterSettings {
  brightnessEnabled: boolean;
  brightness: number; // 50 - 130 (%)
  contrastEnabled: boolean;
  contrast: number; // 70 - 140 (%)

  matteEnabled: boolean;
  matteStrength: number; // 0 - 50 (%)

  strokeBoostEnabled: boolean;
  strokeBoostRadius: number; // 0.5, 1.0, 1.5 (px)

  tintEnabled: boolean;
  tintMode: TintMode;
}

export function createDefaultFilterSettings(): CanvasFilterSettings {
  return {
    brightnessEnabled: false,
    brightness: 100,
    contrastEnabled: false,
    contrast: 100,
    matteEnabled: false,
    matteStrength: 18,
    strokeBoostEnabled: false,
    strokeBoostRadius: 0.6,
    tintEnabled: false,
    tintMode: "none",
  };
}

export class CanvasFilterManager {
  private currentTheme: string = "cream";
  private settings: CanvasFilterSettings = createDefaultFilterSettings();
  private matteLayerEl: HTMLElement | null = null;
  private popupEl: HTMLElement | null = null;
  private backdropEl: HTMLElement | null = null;
  /** The controls laid out in another panel (the gear's), kept to redraw them. */
  private inlineEl: HTMLElement | null = null;

  constructor(private readonly getCanvasHost: () => HTMLElement | null) {
    this.currentTheme = "cream";
    this.loadForTheme(this.currentTheme);
  }

  setTheme(theme: string): void {
    if (this.currentTheme === theme) return;
    this.saveForTheme(this.currentTheme);
    this.currentTheme = theme;
    this.loadForTheme(theme);
    this.apply();
    this.refreshInline();
  }

  private getStorageKey(theme: string): string {
    return `semaps.canvas-filters.${theme}`;
  }

  loadForTheme(theme: string): void {
    const raw = localStorage.getItem(this.getStorageKey(theme));
    if (raw) {
      try {
        this.settings = { ...createDefaultFilterSettings(), ...JSON.parse(raw) };
      } catch {
        this.settings = createDefaultFilterSettings();
      }
    } else {
      this.settings = createDefaultFilterSettings();
    }
  }

  saveForTheme(theme: string): void {
    try {
      localStorage.setItem(this.getStorageKey(theme), JSON.stringify(this.settings));
    } catch {
      // Ignore quota errors
    }
  }

  resetCurrentTheme(): void {
    this.settings = createDefaultFilterSettings();
    this.saveForTheme(this.currentTheme);
    this.apply();
    if (this.popupEl) {
      this.updatePopupControls();
    }
    this.refreshInline();
  }

  /** The controls as a block for another panel, instead of the flyout. */
  renderControls(): HTMLElement {
    this.inlineEl = el("div", { class: "canvas-filters-inline" });
    this.fillControls(this.inlineEl, i18n.currentLanguage === "en");
    return this.inlineEl;
  }

  private refreshInline(): void {
    if (this.inlineEl?.isConnected) this.fillControls(this.inlineEl, i18n.currentLanguage === "en");
  }

  apply(): void {
    const host = this.getCanvasHost();
    if (!host) return;

    // 1. Ensure Matte layer exists
    if (!this.matteLayerEl || !host.contains(this.matteLayerEl)) {
      this.matteLayerEl = host.querySelector<HTMLElement>(".semaps-canvas-matte-layer");
      if (!this.matteLayerEl) {
        this.matteLayerEl = el("div", { class: "semaps-canvas-matte-layer" });
        host.appendChild(this.matteLayerEl);
      }
    }

    // 2. Brightness & Contrast
    const brightnessVal = this.settings.brightnessEnabled ? `${this.settings.brightness}%` : "100%";
    const contrastVal = this.settings.contrastEnabled ? `${this.settings.contrast}%` : "100%";

    // 3. Matte strength
    const matteOpacity = this.settings.matteEnabled ? (this.settings.matteStrength / 100).toFixed(2) : "0";

    // 4. Stroke boost
    const boostPx = this.settings.strokeBoostEnabled ? `${this.settings.strokeBoostRadius}px` : "0px";
    host.classList.toggle("with-stroke-boost", this.settings.strokeBoostEnabled && this.settings.strokeBoostRadius > 0);

    // 5. Tint mode
    let sepia = "0%";
    let hue = "0deg";
    let sat = "100%";

    if (this.settings.tintEnabled && this.settings.tintMode !== "none") {
      switch (this.settings.tintMode) {
        case "warm":
          sepia = "25%";
          sat = "115%";
          break;
        case "slate":
          sat = "65%";
          break;
        case "cool":
          hue = "18deg";
          sat = "90%";
          break;
      }
    }

    // Apply to canvas host CSS variables
    host.style.setProperty("--semaps-filter-brightness", brightnessVal);
    host.style.setProperty("--semaps-filter-contrast", contrastVal);
    host.style.setProperty("--semaps-filter-sepia", sepia);
    host.style.setProperty("--semaps-filter-hue-rotate", hue);
    host.style.setProperty("--semaps-filter-saturate", sat);
    host.style.setProperty("--semaps-matte-overlay-opacity", matteOpacity);
    host.style.setProperty("--semaps-edge-stroke-boost", boostPx);

    // Save state
    this.saveForTheme(this.currentTheme);
  }

  toggleFlyout(anchorBtn?: HTMLElement): void {
    if (this.popupEl) {
      this.closeFlyout();
    } else {
      this.openFlyout(anchorBtn);
    }
  }

  closeFlyout(): void {
    if (this.popupEl) {
      this.popupEl.remove();
      this.popupEl = null;
    }
    if (this.backdropEl) {
      this.backdropEl.remove();
      this.backdropEl = null;
    }
  }

  openFlyout(anchorBtn?: HTMLElement): void {
    this.closeFlyout();

    // 1. Create backdrop for click-outside
    this.backdropEl = el("div", {
      class: "canvas-filters-backdrop",
      on: {
        click: () => this.closeFlyout(),
      },
    });
    document.body.appendChild(this.backdropEl);

    // 2. Create popup container
    const isEn = i18n.currentLanguage === "en";

    this.popupEl = el("div", { class: "canvas-filters-popup" });

    // Position near anchor button
    if (anchorBtn) {
      const rect = anchorBtn.getBoundingClientRect();
      const left = Math.min(window.innerWidth - 330, Math.max(10, rect.left));
      const top = rect.bottom + 4;
      this.popupEl.style.left = `${left}px`;
      this.popupEl.style.top = `${top}px`;
    } else {
      this.popupEl.style.right = "20px";
      this.popupEl.style.top = "100px";
    }

    this.renderPopupContent(isEn);
    document.body.appendChild(this.popupEl);
  }

  private renderPopupContent(isEn: boolean): void {
    if (this.popupEl) this.fillControls(this.popupEl, isEn);
  }

  private fillControls(target: HTMLElement, isEn: boolean): void {
    target.innerHTML = "";

    // Header
    const header = el("div", { class: "canvas-filters-header" }, [
      el("div", { class: "canvas-filters-title" }, [
        el("span", { text: "✨" }),
        el("span", { text: isEn ? "Canvas Visual Effects" : "Эффекты и фильтры холста" }),
      ]),
      el("div", { class: "canvas-filters-theme-badge", text: this.currentTheme.toUpperCase() }),
    ]);

    // Body
    const body = el("div", { class: "canvas-filters-body" });

    // --- Section 1: Brightness & Contrast
    const brightnessSwitch = this.createSwitch(this.settings.brightnessEnabled, (v) => {
      this.settings.brightnessEnabled = v;
      brightnessSlider.disabled = !v;
      this.apply();
    });

    const brightnessValLabel = el("span", { class: "canvas-filters-value", text: `${this.settings.brightness}%` });
    const brightnessSlider = this.createSlider(50, 130, this.settings.brightness, !this.settings.brightnessEnabled, (val) => {
      this.settings.brightness = val;
      brightnessValLabel.textContent = `${val}%`;
      this.apply();
    });

    const contrastSwitch = this.createSwitch(this.settings.contrastEnabled, (v) => {
      this.settings.contrastEnabled = v;
      contrastSlider.disabled = !v;
      this.apply();
    });

    const contrastValLabel = el("span", { class: "canvas-filters-value", text: `${this.settings.contrast}%` });
    const contrastSlider = this.createSlider(70, 140, this.settings.contrast, !this.settings.contrastEnabled, (val) => {
      this.settings.contrast = val;
      contrastValLabel.textContent = `${val}%`;
      this.apply();
    });

    const sec1 = el("div", { class: "canvas-filters-section" }, [
      el("div", { class: "canvas-filters-section-header" }, [
        el("span", { class: "canvas-filters-section-label", text: isEn ? "☀️ Brightness & Contrast" : "☀️ Яркость и контраст" }),
      ]),
      el("div", { class: "canvas-filters-control-row" }, [
        brightnessSwitch,
        el("span", { class: "canvas-filters-control-name", text: isEn ? "Brightness" : "Яркость" }),
        el("div", { class: "canvas-filters-slider-wrapper" }, [brightnessSlider, brightnessValLabel]),
      ]),
      el("div", { class: "canvas-filters-control-row" }, [
        contrastSwitch,
        el("span", { class: "canvas-filters-control-name", text: isEn ? "Contrast" : "Контраст" }),
        el("div", { class: "canvas-filters-slider-wrapper" }, [contrastSlider, contrastValLabel]),
      ]),
    ]);

    // --- Section 2: Matte & White Dimmer (soften extreme whites)
    const matteSwitch = this.createSwitch(this.settings.matteEnabled, (v) => {
      this.settings.matteEnabled = v;
      matteSlider.disabled = !v;
      this.apply();
    });

    const matteValLabel = el("span", { class: "canvas-filters-value", text: `${this.settings.matteStrength}%` });
    const matteSlider = this.createSlider(0, 45, this.settings.matteStrength, !this.settings.matteEnabled, (val) => {
      this.settings.matteStrength = val;
      matteValLabel.textContent = `${val}%`;
      this.apply();
    });

    const sec2 = el("div", { class: "canvas-filters-section" }, [
      el("div", { class: "canvas-filters-section-header" }, [
        el("span", { class: "canvas-filters-section-label", text: isEn ? "🕶️ Matte & White Dimmer" : "🕶️ Приглушение белизны" }),
        matteSwitch,
      ]),
      el("div", { class: "canvas-filters-control-row" }, [
        el("span", { class: "canvas-filters-control-name", text: isEn ? "Dimming" : "Степень" }),
        el("div", { class: "canvas-filters-slider-wrapper" }, [matteSlider, matteValLabel]),
      ]),
    ]);

    // --- Section 3: Stroke & Edge Boost
    const strokeSwitch = this.createSwitch(this.settings.strokeBoostEnabled, (v) => {
      this.settings.strokeBoostEnabled = v;
      strokePillGroup.querySelectorAll<HTMLButtonElement>("button").forEach((b) => (b.disabled = !v));
      this.apply();
    });

    const strokeRadii = [
      { label: "+0.5px", val: 0.5 },
      { label: "+1.0px", val: 1.0 },
      { label: "+1.5px", val: 1.5 },
    ];

    const strokePillGroup = el("div", { class: "canvas-filters-pill-group" });
    for (const item of strokeRadii) {
      const btn = el("button", {
        class: `canvas-filters-pill ${this.settings.strokeBoostRadius === item.val ? "is-active" : ""}`,
        text: item.label,
        attrs: !this.settings.strokeBoostEnabled ? { disabled: "true" } : {},
        on: {
          click: () => {
            this.settings.strokeBoostRadius = item.val;
            strokePillGroup.querySelectorAll(".canvas-filters-pill").forEach((b) => b.classList.remove("is-active"));
            btn.classList.add("is-active");
            this.apply();
          },
        },
      });
      strokePillGroup.appendChild(btn);
    }

    const sec3 = el("div", { class: "canvas-filters-section" }, [
      el("div", { class: "canvas-filters-section-header" }, [
        el("span", { class: "canvas-filters-section-label", text: isEn ? "✏️ Edge & Stroke Boost" : "✏️ Усиление толщины линий" }),
        strokeSwitch,
      ]),
      el("div", { class: "canvas-filters-control-row" }, [
        el("span", { class: "canvas-filters-control-name", text: isEn ? "Extra width" : "Прибавка" }),
        strokePillGroup,
      ]),
    ]);

    // --- Section 4: Color Tint / Tone
    const tintSwitch = this.createSwitch(this.settings.tintEnabled, (v) => {
      this.settings.tintEnabled = v;
      tintPillGroup.querySelectorAll<HTMLButtonElement>("button").forEach((b) => (b.disabled = !v));
      this.apply();
    });

    const tintModes: { id: TintMode; label: string }[] = [
      { id: "none", label: isEn ? "None" : "Нет" },
      { id: "warm", label: isEn ? "Warm" : "Тёплый" },
      { id: "slate", label: isEn ? "Slate" : "Сланец" },
      { id: "cool", label: isEn ? "Cool" : "Холодный" },
    ];

    const tintPillGroup = el("div", { class: "canvas-filters-pill-group" });
    for (const item of tintModes) {
      const btn = el("button", {
        class: `canvas-filters-pill ${this.settings.tintMode === item.id ? "is-active" : ""}`,
        text: item.label,
        attrs: !this.settings.tintEnabled ? { disabled: "true" } : {},
        on: {
          click: () => {
            this.settings.tintMode = item.id;
            tintPillGroup.querySelectorAll(".canvas-filters-pill").forEach((b) => b.classList.remove("is-active"));
            btn.classList.add("is-active");
            this.apply();
          },
        },
      });
      tintPillGroup.appendChild(btn);
    }

    const sec4 = el("div", { class: "canvas-filters-section" }, [
      el("div", { class: "canvas-filters-section-header" }, [
        el("span", { class: "canvas-filters-section-label", text: isEn ? "🎨 Color Tint & Tone" : "🎨 Цветовой тон и оттенок" }),
        tintSwitch,
      ]),
      el("div", { class: "canvas-filters-control-row" }, [
        el("span", { class: "canvas-filters-control-name", text: isEn ? "Preset" : "Пресет" }),
        tintPillGroup,
      ]),
    ]);

    body.appendChild(sec1);
    body.appendChild(sec2);
    body.appendChild(sec3);
    body.appendChild(sec4);

    // Footer
    const footer = el("div", { class: "canvas-filters-footer" }, [
      el(
        "button",
        {
          class: "canvas-filters-reset-btn",
          on: {
            click: () => this.resetCurrentTheme(),
          },
        },
        [el("span", { text: "↺" }), el("span", { text: isEn ? "Reset defaults" : "Сбросить настройки" })],
      ),
      el("span", {
        attrs: { style: "font-size: 10px; color: var(--muted); font-style: italic;" },
        text: isEn ? "Auto-saved per theme" : "Автосохранение для темы",
      }),
    ]);

    target.appendChild(header);
    target.appendChild(body);
    target.appendChild(footer);
  }

  private updatePopupControls(): void {
    const isEn = i18n.currentLanguage === "en";
    this.renderPopupContent(isEn);
  }

  private createSwitch(checked: boolean, onChange: (checked: boolean) => void): HTMLElement {
    const input = el("input", {
      attrs: checked ? { type: "checkbox", checked: "true" } : { type: "checkbox" },
      on: {
        change: (e) => {
          onChange((e.target as HTMLInputElement).checked);
        },
      },
    }) as HTMLInputElement;

    const slider = el("span", { class: "canvas-filters-switch-slider" });
    return el("label", { class: "canvas-filters-switch" }, [input, slider]);
  }

  private createSlider(
    min: number,
    max: number,
    val: number,
    disabled: boolean,
    onChange: (val: number) => void,
  ): HTMLInputElement {
    const slider = el("input", {
      class: "canvas-filters-slider",
      attrs: {
        type: "range",
        min: String(min),
        max: String(max),
        value: String(val),
        ...(disabled ? { disabled: "true" } : {}),
      },
      on: {
        input: (e) => {
          onChange(Number((e.target as HTMLInputElement).value));
        },
      },
    }) as HTMLInputElement;

    return slider;
  }
}
