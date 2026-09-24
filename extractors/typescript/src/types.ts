export type SymbolKind = "type" | "interface" | "function" | "module" | "value";
export type EdgeKind = "extends" | "implements" | "contains" | "depends" | "holds" | "uses";

export interface MemberRecord {
  kind?: string;
  name: string;
  type?: string;
  visibility?: string;
  note?: string;
}

export interface ViaRecord {
  member?: string;
  memberKind?: string;
  modifiers?: string[];
  text?: string;
  path?: string[];
  cardinality?: "one" | "optional" | "many" | "keyed";
  mutability?: "mutable" | "readonly";
  deferred?: boolean;
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
  via?: ViaRecord;
}

export interface FactsOutput {
  language: "typescript";
  root: string;
  edgeKinds?: EdgeKind[];
  symbols: SymbolRecord[];
  edges: EdgeRecord[];
}
