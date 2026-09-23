import type { UiLanguage, UiDictionary } from "./types.js";
import { ruDictionary } from "./locales/ru.js";
import { enDictionary } from "./locales/en.js";

const STORAGE_KEY = "semaps:ui-lang";

export class I18nService {
  private static instance?: I18nService;
  private lang: UiLanguage = "ru";
  private readonly listeners = new Set<(lang: UiLanguage) => void>();
  private readonly dictionaries: Record<UiLanguage, UiDictionary> = {
    ru: ruDictionary,
    en: enDictionary,
  };

  static get(): I18nService {
    if (!I18nService.instance) {
      I18nService.instance = new I18nService();
    }
    return I18nService.instance;
  }

  constructor() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY) as UiLanguage | null;
      if (saved === "ru" || saved === "en") {
        this.lang = saved;
      } else {
        const nav = typeof navigator !== "undefined" ? navigator.language.toLowerCase() : "ru";
        this.lang = nav.startsWith("ru") ? "ru" : "en";
      }
    } catch {
      this.lang = "ru";
    }
  }

  get currentLanguage(): UiLanguage {
    return this.lang;
  }

  get d(): UiDictionary {
    return this.dictionaries[this.lang] ?? this.dictionaries.ru;
  }

  setLanguage(lang: UiLanguage): void {
    if (this.lang === lang) return;
    this.lang = lang;
    try {
      localStorage.setItem(STORAGE_KEY, lang);
    } catch {
      // ignore storage errors
    }
    for (const listener of this.listeners) {
      listener(lang);
    }
  }

  onLanguageChange(listener: (lang: UiLanguage) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  format(template: string, params: Record<string, string | number>): string {
    let res = template;
    for (const [k, v] of Object.entries(params)) {
      res = res.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
    }
    return res;
  }
}

export const i18n = I18nService.get();
