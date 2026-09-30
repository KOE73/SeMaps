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

export interface SpanRecord {
  file: string;
  line: number;
  endLine?: number;
}

export interface SymbolRecord {
  id: string;
  kind: SymbolKind;
  nativeKind: string;
  name: string;
  namespace?: string;
  file: string;
  line?: number;
  endLine?: number;
  /** Every declaration of the symbol; printed only when there is more than
   * one (merged interfaces/namespaces). `file`/`line` above stay the first
   * declaration, as without this field. */
  spans?: SpanRecord[];
  visibility?: string;
  members?: MemberRecord[];
  /** member name -> line, beside `members` (docs/EXTRACTOR.md §2.1): sync
   * copies `members` verbatim into entities.json, so a line inside it would
   * make the registry change on every code edit above it. */
  memberLines?: Record<string, number>;
}

export interface EdgeRecord {
  from: string;
  to: string;
  kind: EdgeKind;
  /** How TypeScript expressed the edge (docs/extractors/typescript.md, "Сопоставление рёбер"). */
  native?: string;
  via?: ViaRecord;
  /** Where the edge comes from: the member for holds/uses, the heritage
   * clause for extends/implements. Not for contains/depends. `file` only
   * when it differs from the `from` symbol's file. */
  line?: number;
  file?: string;
}

export interface FactsOutput {
  language: "typescript";
  root: string;
  edgeKinds?: EdgeKind[];
  symbols: SymbolRecord[];
  edges: EdgeRecord[];
}
