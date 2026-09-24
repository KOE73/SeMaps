import * as fs from "node:fs";
import * as path from "node:path";
import ts from "typescript";
import { discoverFiles } from "./discovery.js";
import { collectFacts, type SourceFileEntry } from "./collect.js";
import { stripExtension } from "./id.js";
import type { EdgeKind, EdgeRecord, FactsOutput, SymbolRecord } from "./types.js";

export interface ExtractOptions {
  root: string;
  include: string[];
  exclude: string[];
  edges?: string[];
}

export class ExtractError extends Error {}

const DEFAULT_COMPILER_OPTIONS: ts.CompilerOptions = {
  target: ts.ScriptTarget.ES2022,
  module: ts.ModuleKind.ESNext,
  moduleResolution: ts.ModuleResolutionKind.Bundler,
  lib: ["lib.es2022.d.ts", "lib.dom.d.ts", "lib.dom.iterable.d.ts"],
  jsx: ts.JsxEmit.ReactJSX,
  esModuleInterop: true,
  allowJs: false,
  skipLibCheck: true,
  noEmit: true,
  strict: false,
};

function loadCompilerOptions(root: string): ts.CompilerOptions {
  const tsconfigPath = path.join(root, "tsconfig.json");
  if (!fs.existsSync(tsconfigPath)) {
    return DEFAULT_COMPILER_OPTIONS;
  }
  const read = ts.readConfigFile(tsconfigPath, (p) => fs.readFileSync(p, "utf8"));
  if (read.error) {
    return DEFAULT_COMPILER_OPTIONS;
  }
  const parsed = ts.parseJsonConfigFileContent(read.config, ts.sys, root);
  return { ...parsed.options, noEmit: true };
}

function symbolSortKey(a: SymbolRecord, b: SymbolRecord): number {
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

function edgeSortKey(a: EdgeRecord, b: EdgeRecord): number {
  if (a.from !== b.from) return a.from < b.from ? -1 : 1;
  if (a.to !== b.to) return a.to < b.to ? -1 : 1;
  if (a.kind !== b.kind) return a.kind < b.kind ? -1 : 1;
  const aMember = a.via?.member ?? "";
  const bMember = b.via?.member ?? "";
  if (aMember !== bMember) return aMember < bMember ? -1 : 1;
  const aPath = JSON.stringify(a.via?.path ?? []);
  const bPath = JSON.stringify(b.via?.path ?? []);
  if (aPath !== bPath) return aPath < bPath ? -1 : 1;
  return 0;
}

export function extract(options: ExtractOptions): FactsOutput {
  const root = path.resolve(options.root);
  let rootStat: fs.Stats;
  try {
    rootStat = fs.statSync(root);
  } catch {
    throw new ExtractError(`root does not exist: ${options.root}`);
  }
  if (!rootStat.isDirectory()) {
    throw new ExtractError(`root is not a directory: ${options.root}`);
  }

  const absFiles = discoverFiles({ root, include: options.include, exclude: options.exclude });
  const compilerOptions = loadCompilerOptions(root);

  const program = ts.createProgram({ rootNames: absFiles, options: compilerOptions });
  const checker = program.getTypeChecker();

  const files: SourceFileEntry[] = [];
  for (const abs of absFiles) {
    const sourceFile = program.getSourceFile(abs);
    if (!sourceFile) continue;
    const relWithExt = path.relative(root, abs).split(path.sep).join("/");
    files.push({ sourceFile, relNoExt: stripExtension(relWithExt), relWithExt });
  }

  const { symbols, edges } = collectFacts(checker, files, options.edges);
  symbols.sort(symbolSortKey);
  edges.sort(edgeSortKey);

  const result: FactsOutput = {
    language: "typescript",
    root: options.root,
    symbols,
    edges,
  };

  if (options.edges && options.edges.length > 0) {
    const baseKinds: EdgeKind[] = ["extends", "implements", "contains"];
    result.edgeKinds = [...new Set([...baseKinds, ...options.edges as EdgeKind[]])];
    result.edgeKinds.sort();
  }

  return result;
}
