import type { WireStyle, StyleTarget } from "./style-types.js";

export interface StyleModifier {
  id: string; name: string; description?: string;
  patches: Partial<Record<StyleTarget, Partial<WireStyle>>>;
}
export type StyleSelector = { all: StyleSelector[] } | { any: StyleSelector[] } | { not: StyleSelector } | { field: string; op: string; value: string | string[] | boolean };
export interface StyleRule {
  id: string; name: string; description?: string; selectorVersion: 1;
  selector: StyleSelector; modifierIds: string[]; enabled?: boolean; order?: number;
  scope?: { projects?: string[]; views?: string[] };
}
export interface SelectorField { id: string; name: string; description: string; operators: string[] }
export interface AutomaticStyleLayer {
  modifierId: string; modifierName: string; ruleId: string; ruleName: string;
}
export interface StyleAppearance {
  id: string; family: StyleTarget; baseStyleId: string;
  applied: Array<{ id: string; name: string; stage: string; rule?: string }>;
  skipped: Array<{ id: string; name: string; stage: string; rule?: string }>;
  patch: Partial<WireStyle>; resolved: Partial<WireStyle>;
  origins: Record<string, string>; facts: Record<string, string[]>;
}
export type ExtensionCollection = "modifiers" | "styleRules";
export interface ExtensionEdit {
  action: "create" | "update" | "rename" | "delete"; id: string;
  definition?: StyleModifier | StyleRule; patch?: Record<string, unknown>;
  newId?: string; replacement?: string; expectedRevision?: number;
}
export const idsFromText = (value: string): string[] => value.split(",").map(v => v.trim()).filter(Boolean);
