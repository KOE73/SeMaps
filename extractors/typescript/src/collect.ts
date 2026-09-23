import ts from "typescript";
import { declId, moduleId } from "./id.js";
import type { EdgeKind, EdgeRecord, MemberRecord, SymbolKind, SymbolRecord } from "./types.js";

export interface SourceFileEntry {
  sourceFile: ts.SourceFile;
  /** Path relative to --root, forward slashes, without extension. */
  relNoExt: string;
  /** Path relative to --root, forward slashes, with extension. */
  relWithExt: string;
}

interface DeclEntry {
  id: string;
  kind: "class" | "interface" | "enum" | "typeAlias" | "function" | "value";
  /** Declaration node; for class/interface/enum/typeAlias this is walked for members. */
  node: ts.Node;
  /** function/arrow-function only: parameters and return type of its own signature. */
  parameters?: readonly ts.ParameterDeclaration[];
  returnType?: ts.TypeNode;
  /** value only: its type annotation, if any. */
  typeNode?: ts.TypeNode;
}

function hasModifier(node: ts.Node, kind: ts.SyntaxKind): boolean {
  if (!ts.canHaveModifiers(node)) return false;
  const mods = ts.getModifiers(node);
  return !!mods?.some((m) => m.kind === kind);
}

function lineOf(node: ts.Node, sourceFile: ts.SourceFile): number {
  return sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1;
}

function basenameNoExt(relNoExt: string): string {
  const parts = relNoExt.split("/");
  return parts[parts.length - 1] ?? relNoExt;
}

function symbolOfName(checker: ts.TypeChecker, nameNode: ts.Node | undefined, fallback: ts.Node): ts.Symbol | undefined {
  const target = nameNode ?? fallback;
  return checker.getSymbolAtLocation(target);
}

function resolveAlias(checker: ts.TypeChecker, symbol: ts.Symbol): ts.Symbol {
  if (symbol.flags & ts.SymbolFlags.Alias) {
    try {
      return checker.getAliasedSymbol(symbol);
    } catch {
      return symbol;
    }
  }
  return symbol;
}

/** Walk a type node's AST and collect the ids (from idBySymbol) of every referenced type. */
function collectReferencedIds(
  typeNode: ts.TypeNode | undefined,
  checker: ts.TypeChecker,
  idBySymbol: Map<ts.Symbol, string>,
): Set<string> {
  const found = new Set<string>();
  if (!typeNode) return found;

  const visit = (node: ts.Node): void => {
    if (ts.isTypeReferenceNode(node)) {
      const nameNode = ts.isQualifiedName(node.typeName) ? node.typeName.right : node.typeName;
      const symbol = checker.getSymbolAtLocation(nameNode);
      if (symbol) {
        const resolved = resolveAlias(checker, symbol);
        const id = idBySymbol.get(resolved);
        if (id) found.add(id);
      }
    }
    node.forEachChild(visit);
  };

  visit(typeNode);
  return found;
}

function memberTypeString(checker: ts.TypeChecker, node: ts.Node): string {
  return checker.typeToString(checker.getTypeAtLocation(node));
}

function classMemberVisibility(member: ts.ClassElement): "public" | "protected" | "private" {
  const nameNode = (member as ts.PropertyDeclaration | ts.MethodDeclaration).name;
  if (nameNode && ts.isPrivateIdentifier(nameNode)) return "private";
  if (hasModifier(member, ts.SyntaxKind.PrivateKeyword)) return "private";
  if (hasModifier(member, ts.SyntaxKind.ProtectedKeyword)) return "protected";
  return "public";
}

function memberName(nameNode: ts.PropertyName | undefined): string | undefined {
  if (!nameNode) return undefined;
  if (ts.isIdentifier(nameNode) || ts.isPrivateIdentifier(nameNode) || ts.isStringLiteral(nameNode) || ts.isNumericLiteral(nameNode)) {
    return nameNode.text;
  }
  return undefined;
}

function makeMember(kind: string, name: string, type: string | undefined, visibility: string | undefined): MemberRecord {
  const m: MemberRecord = { kind, name };
  if (type !== undefined) m.type = type;
  if (visibility !== undefined) m.visibility = visibility;
  return m;
}

export function collectFacts(
  checker: ts.TypeChecker,
  files: SourceFileEntry[],
): { symbols: SymbolRecord[]; edges: EdgeRecord[] } {
  const symbols: SymbolRecord[] = [];
  const edgeMap = new Map<string, EdgeRecord>();
  const idBySymbol = new Map<ts.Symbol, string>();
  const declEntries: DeclEntry[] = [];

  const addEdge = (from: string, to: string, kind: EdgeKind): void => {
    if (from === to) return;
    edgeMap.set(`${from}\u0000${to}\u0000${kind}`, { from, to, kind });
  };

  const pushSymbol = (
    id: string,
    kind: SymbolKind,
    nativeKind: string,
    name: string,
    namespacePath: readonly string[],
    file: string,
    line: number | undefined,
    visibility: string | undefined,
  ): SymbolRecord => {
    const record: SymbolRecord = {
      id,
      kind,
      nativeKind,
      name,
      namespace: namespacePath.join("."),
      file,
      ...(line !== undefined ? { line } : {}),
      ...(visibility !== undefined ? { visibility } : {}),
    };
    symbols.push(record);
    return record;
  };

  function collectLateExportedNames(statements: readonly ts.Statement[]): Set<string> {
    const names = new Set<string>();
    for (const stmt of statements) {
      if (ts.isExportDeclaration(stmt) && !stmt.moduleSpecifier && stmt.exportClause && ts.isNamedExports(stmt.exportClause)) {
        for (const spec of stmt.exportClause.elements) {
          names.add((spec.propertyName ?? spec.name).text);
        }
      }
    }
    return names;
  }

  /** Identifier target of a bare `export default <identifier>;`, if any. */
  function collectDefaultExportedIdentifierName(statements: readonly ts.Statement[]): string | undefined {
    for (const stmt of statements) {
      if (ts.isExportAssignment(stmt) && !stmt.isExportEquals && ts.isIdentifier(stmt.expression)) {
        return stmt.expression.text;
      }
    }
    return undefined;
  }

  function processContainer(
    statements: readonly ts.Statement[],
    nsPath: readonly string[],
    containerId: string,
    relNoExt: string,
    relWithExt: string,
  ): void {
    const lateExported = collectLateExportedNames(statements);
    const defaultExportedName = collectDefaultExportedIdentifierName(statements);

    for (const stmt of statements) {
      if (ts.isClassDeclaration(stmt)) {
        const localName = stmt.name?.text;
        const isDefault = hasModifier(stmt, ts.SyntaxKind.DefaultKeyword) || (localName !== undefined && localName === defaultExportedName);
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || isDefault || (localName !== undefined && lateExported.has(localName));
        const exportName = isDefault ? "default" : (localName ?? "default");
        const symbol = symbolOfName(checker, stmt.name, stmt);
        if (symbol && idBySymbol.has(symbol)) {
          addEdge(containerId, idBySymbol.get(symbol)!, "contains");
          continue;
        }
        const id = declId(relNoExt, nsPath, exportName);
        const nativeKind = hasModifier(stmt, ts.SyntaxKind.AbstractKeyword) ? "abstract-class" : "class";
        pushSymbol(id, "type", nativeKind, localName ?? "default", nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
        if (symbol) idBySymbol.set(symbol, id);
        declEntries.push({ id, node: stmt, kind: "class" });
        addEdge(containerId, id, "contains");
        continue;
      }

      if (ts.isInterfaceDeclaration(stmt)) {
        const localName = stmt.name.text;
        const isDefault = localName === defaultExportedName && !hasModifier(stmt, ts.SyntaxKind.ExportKeyword);
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || lateExported.has(localName) || isDefault;
        const exportName = isDefault ? "default" : localName;
        const symbol = symbolOfName(checker, stmt.name, stmt);
        if (symbol && idBySymbol.has(symbol)) {
          addEdge(containerId, idBySymbol.get(symbol)!, "contains");
          continue;
        }
        const id = declId(relNoExt, nsPath, exportName);
        pushSymbol(id, "interface", "interface", localName, nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
        if (symbol) idBySymbol.set(symbol, id);
        declEntries.push({ id, node: stmt, kind: "interface" });
        addEdge(containerId, id, "contains");
        continue;
      }

      if (ts.isTypeAliasDeclaration(stmt)) {
        const localName = stmt.name.text;
        const isDefault = localName === defaultExportedName && !hasModifier(stmt, ts.SyntaxKind.ExportKeyword);
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || lateExported.has(localName) || isDefault;
        const exportName = isDefault ? "default" : localName;
        const symbol = symbolOfName(checker, stmt.name, stmt);
        if (symbol && idBySymbol.has(symbol)) continue;
        const id = declId(relNoExt, nsPath, exportName);
        pushSymbol(id, "type", "type-alias", localName, nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
        if (symbol) idBySymbol.set(symbol, id);
        declEntries.push({ id, node: stmt, kind: "typeAlias" });
        addEdge(containerId, id, "contains");
        continue;
      }

      if (ts.isEnumDeclaration(stmt)) {
        const localName = stmt.name.text;
        const isDefault = localName === defaultExportedName && !hasModifier(stmt, ts.SyntaxKind.ExportKeyword);
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || lateExported.has(localName) || isDefault;
        const exportName = isDefault ? "default" : localName;
        const symbol = symbolOfName(checker, stmt.name, stmt);
        if (symbol && idBySymbol.has(symbol)) continue;
        const id = declId(relNoExt, nsPath, exportName);
        pushSymbol(id, "type", "enum", localName, nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
        if (symbol) idBySymbol.set(symbol, id);
        declEntries.push({ id, node: stmt, kind: "enum" });
        addEdge(containerId, id, "contains");
        continue;
      }

      if (ts.isFunctionDeclaration(stmt)) {
        const localName = stmt.name?.text;
        const isDefault = hasModifier(stmt, ts.SyntaxKind.DefaultKeyword) || (localName !== undefined && localName === defaultExportedName);
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || isDefault || (localName !== undefined && lateExported.has(localName));
        const exportName = isDefault ? "default" : (localName ?? "default");
        const symbol = symbolOfName(checker, stmt.name, stmt);
        if (symbol && idBySymbol.has(symbol)) {
          addEdge(containerId, idBySymbol.get(symbol)!, "contains");
          continue;
        }
        const id = declId(relNoExt, nsPath, exportName);
        pushSymbol(id, "function", "function", localName ?? "default", nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
        if (symbol) idBySymbol.set(symbol, id);
        declEntries.push({ id, kind: "function", node: stmt, parameters: stmt.parameters, returnType: stmt.type });
        addEdge(containerId, id, "contains");
        continue;
      }

      if (ts.isModuleDeclaration(stmt) && ts.isIdentifier(stmt.name) && stmt.body && ts.isModuleBlock(stmt.body)) {
        const localName = stmt.name.text;
        const isExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword) || lateExported.has(localName);
        const symbol = symbolOfName(checker, stmt.name, stmt);
        let id: string;
        if (symbol && idBySymbol.has(symbol)) {
          id = idBySymbol.get(symbol)!;
          addEdge(containerId, id, "contains");
        } else {
          id = declId(relNoExt, nsPath, localName);
          pushSymbol(id, "module", "namespace", localName, nsPath, relWithExt, lineOf(stmt, stmt.getSourceFile()), isExported ? "exported" : "file");
          if (symbol) idBySymbol.set(symbol, id);
          addEdge(containerId, id, "contains");
        }
        processContainer(stmt.body.statements, [...nsPath, localName], id, relNoExt, relWithExt);
        continue;
      }

      if (ts.isVariableStatement(stmt)) {
        const isStmtExported = hasModifier(stmt, ts.SyntaxKind.ExportKeyword);
        const isConst = (stmt.declarationList.flags & ts.NodeFlags.Const) !== 0;
        const isLet = (stmt.declarationList.flags & ts.NodeFlags.Let) !== 0;
        const declKeyword = isConst ? "const" : isLet ? "let" : "var";
        for (const decl of stmt.declarationList.declarations) {
          if (!ts.isIdentifier(decl.name)) continue; // destructuring patterns: narrow, not handled
          const localName = decl.name.text;
          const isDefault = localName === defaultExportedName && !isStmtExported;
          const isExported = isStmtExported || isDefault || lateExported.has(localName);
          const exportName = isDefault ? "default" : localName;
          const symbol = checker.getSymbolAtLocation(decl.name);
          if (symbol && idBySymbol.has(symbol)) {
            addEdge(containerId, idBySymbol.get(symbol)!, "contains");
            continue;
          }
          const id = declId(relNoExt, nsPath, exportName);
          const init = decl.initializer;
          const isFunctionValued = init !== undefined && (ts.isArrowFunction(init) || ts.isFunctionExpression(init));
          const visibility = isExported ? "exported" : "file";
          if (isFunctionValued) {
            const fn = init as ts.ArrowFunction | ts.FunctionExpression;
            const nativeKind = ts.isArrowFunction(fn) ? "arrow-function" : "function-expression";
            pushSymbol(id, "function", nativeKind, localName, nsPath, relWithExt, lineOf(decl, stmt.getSourceFile()), visibility);
            declEntries.push({ id, kind: "function", node: decl, parameters: fn.parameters, returnType: fn.type });
          } else {
            pushSymbol(id, "value", declKeyword, localName, nsPath, relWithExt, lineOf(decl, stmt.getSourceFile()), visibility);
            declEntries.push({ id, kind: "value", node: decl, typeNode: decl.type });
          }
          if (symbol) idBySymbol.set(symbol, id);
          addEdge(containerId, id, "contains");
        }
        continue;
      }
    }
  }

  for (const file of files) {
    const fileId = moduleId(file.relNoExt);
    pushSymbol(fileId, "module", "file", basenameNoExt(file.relNoExt), [], file.relWithExt, undefined, undefined);
    processContainer(file.sourceFile.statements, [], fileId, file.relNoExt, file.relWithExt);
  }

  const validIds = idBySymbol;

  for (const entry of declEntries) {
    if (entry.kind === "class") {
      const classDecl = entry.node as ts.ClassDeclaration;
      const members: MemberRecord[] = [];
      for (const heritage of classDecl.heritageClauses ?? []) {
        const edgeKind: EdgeKind = heritage.token === ts.SyntaxKind.ExtendsKeyword ? "extends" : "implements";
        for (const t of heritage.types) {
          const symbol = checker.getSymbolAtLocation(t.expression);
          if (!symbol) continue;
          const resolved = resolveAlias(checker, symbol);
          const targetId = validIds.get(resolved);
          if (targetId) addEdge(entry.id, targetId, edgeKind);
        }
      }
      for (const member of classDecl.members) {
        if (ts.isConstructorDeclaration(member)) continue;
        const name = memberName((member as ts.PropertyDeclaration | ts.MethodDeclaration).name);
        if (!name) continue;
        const visibility = classMemberVisibility(member);
        let kind: string | undefined;
        let typeNode: ts.TypeNode | undefined;
        if (ts.isPropertyDeclaration(member)) {
          kind = "field";
          typeNode = member.type;
        } else if (ts.isGetAccessor(member) || ts.isSetAccessor(member)) {
          kind = "property";
          typeNode = ts.isGetAccessor(member) ? member.type : undefined;
        } else if (ts.isMethodDeclaration(member)) {
          kind = "method";
        }
        if (!kind) continue;
        const typeStr = memberTypeString(checker, member);
        members.push(makeMember(kind, name, typeStr, visibility));
        if (typeNode) {
          for (const id of collectReferencedIds(typeNode, checker, validIds)) addEdge(entry.id, id, "references");
        }
        if (ts.isMethodDeclaration(member) || ts.isGetAccessor(member) || ts.isSetAccessor(member)) {
          for (const param of member.parameters) {
            for (const id of collectReferencedIds(param.type, checker, validIds)) addEdge(entry.id, id, "references");
          }
          if (ts.isMethodDeclaration(member) && member.type) {
            for (const id of collectReferencedIds(member.type, checker, validIds)) addEdge(entry.id, id, "references");
          }
        }
      }
      const symbolRecord = symbols.find((s) => s.id === entry.id)!;
      symbolRecord.members = members;
    } else if (entry.kind === "interface") {
      const ifaceDecl = entry.node as ts.InterfaceDeclaration;
      const members: MemberRecord[] = [];
      for (const heritage of ifaceDecl.heritageClauses ?? []) {
        if (heritage.token !== ts.SyntaxKind.ExtendsKeyword) continue;
        for (const t of heritage.types) {
          const symbol = checker.getSymbolAtLocation(t.expression);
          if (!symbol) continue;
          const resolved = resolveAlias(checker, symbol);
          const targetId = validIds.get(resolved);
          if (targetId) addEdge(entry.id, targetId, "extends");
        }
      }
      for (const member of ifaceDecl.members) {
        if (ts.isPropertySignature(member)) {
          const name = memberName(member.name);
          if (!name) continue;
          const typeStr = memberTypeString(checker, member);
          members.push(makeMember("property", name, typeStr, undefined));
          if (member.type) {
            for (const id of collectReferencedIds(member.type, checker, validIds)) addEdge(entry.id, id, "references");
          }
        } else if (ts.isMethodSignature(member)) {
          const name = memberName(member.name);
          if (!name) continue;
          const typeStr = memberTypeString(checker, member);
          members.push(makeMember("method", name, typeStr, undefined));
          for (const param of member.parameters) {
            for (const id of collectReferencedIds(param.type, checker, validIds)) addEdge(entry.id, id, "references");
          }
          if (member.type) {
            for (const id of collectReferencedIds(member.type, checker, validIds)) addEdge(entry.id, id, "references");
          }
        }
      }
      const symbolRecord = symbols.find((s) => s.id === entry.id)!;
      symbolRecord.members = members;
    } else if (entry.kind === "enum") {
      const enumDecl = entry.node as ts.EnumDeclaration;
      const members: MemberRecord[] = [];
      for (const member of enumDecl.members) {
        const name = memberName(member.name);
        if (!name) continue;
        const value = checker.getConstantValue(member);
        const typeStr = value === undefined ? undefined : String(value);
        members.push(makeMember("value", name, typeStr, undefined));
      }
      const symbolRecord = symbols.find((s) => s.id === entry.id)!;
      symbolRecord.members = members;
    } else if (entry.kind === "typeAlias") {
      const aliasDecl = entry.node as ts.TypeAliasDeclaration;
      for (const id of collectReferencedIds(aliasDecl.type, checker, validIds)) addEdge(entry.id, id, "references");
    } else if (entry.kind === "function") {
      for (const param of entry.parameters ?? []) {
        for (const id of collectReferencedIds(param.type, checker, validIds)) addEdge(entry.id, id, "references");
      }
      for (const id of collectReferencedIds(entry.returnType, checker, validIds)) addEdge(entry.id, id, "references");
    } else if (entry.kind === "value") {
      for (const id of collectReferencedIds(entry.typeNode, checker, validIds)) addEdge(entry.id, id, "references");
    }
  }

  return { symbols, edges: Array.from(edgeMap.values()) };
}
