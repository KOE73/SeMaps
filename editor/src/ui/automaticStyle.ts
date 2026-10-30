import type { AutomaticStyleLayer } from "../model/style-extensions.js";
import { i18n } from "../workbench/i18n/I18nService.js";

export const automaticStyleTitle = (): string => i18n.currentLanguage === "ru" ? "Оформление по правилам" : "Formatting by rules";
export const automaticStyleLines = (layers: readonly AutomaticStyleLayer[]): string[] =>
  layers.map(m => `${m.ruleName || m.ruleId} → ${m.modifierName}`);
export const automaticStyleHint = (): string => i18n.currentLanguage === "ru"
  ? "Нажмите, чтобы посмотреть применённые правила в свойствах."
  : "Click to see the applied rules in Properties.";
