export type SymbolKind = "type" | "interface" | "function" | "module" | "value";
export type EdgeKind = "extends" | "implements" | "references" | "contains";

export interface MemberRecord {
  kind?: string;
  name: string;
  type?: string;
  visibility?: string;
  note?: string;
}

export interface SymbolRecord {
  id: string;
  kind: SymbolKind;
  nativeKind: string;
  name: string;
  namespace?: string;
  file: string;
  line?: number;
  visibility?: string;
  members?: MemberRecord[];
}

export interface EdgeRecord {
  from: string;
  to: string;
  kind: EdgeKind;
}

export interface FactsOutput {
  language: "typescript";
  root: string;
  symbols: SymbolRecord[];
  edges: EdgeRecord[];
}
