import ts from "typescript";
import { declId, moduleId } from "./id.js";
import type { EdgeKind, EdgeRecord, MemberRecord, SymbolKind, SymbolRecord, ViaRecord } from "./types.js";

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

interface TypePath {
  symbol?: ts.Symbol;
  path: string[];
  cardinality?: "one" | "optional" | "many" | "keyed";
  mutability?: "mutable" | "readonly";
  deferred?: boolean;
}

/** Walk a type node's AST and collect all referenced types with their paths and features. */
function collectTypePaths(
  typeNode: ts.TypeNode | undefined,
  checker: ts.TypeChecker,
  idBySymbol: Map<ts.Symbol, string>,
): TypePath[] {
  const found: TypePath[] = [];
  if (!typeNode) return found;

  const visit = (node: ts.Node, currentPath: string[], currentCardinal?: string, currentMutability?: string, currentDeferred?: boolean): void => {
    // Array types: T[]
    if (ts.isArrayTypeNode(node)) {
      const elemType = node.elementType;
      const hasReadonly = hasModifier(node, ts.SyntaxKind.ReadonlyKeyword);
      visit(elemType, [...currentPath, "item"], "many", hasReadonly ? "readonly" : "mutable", currentDeferred);
      return;
    }

    // Type references like Array<T>, Set<T>, Map<K,V>, etc.
    if (ts.isTypeReferenceNode(node)) {
      const nameNode = ts.isQualifiedName(node.typeName) ? node.typeName.right : node.typeName;
      const symbol = checker.getSymbolAtLocation(nameNode);
      const typeName = nameNode.text;

      // Handle generic collections
      const typeArgs = node.typeArguments ?? [];

      if (typeName === "Array" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "item"], "many", "mutable", currentDeferred);
        return;
      } else if (typeName === "ReadonlyArray" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "item"], "many", "readonly", currentDeferred);
        return;
      } else if (typeName === "Set" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "item"], "many", "mutable", currentDeferred);
        return;
      } else if (typeName === "ReadonlySet" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "item"], "many", "readonly", currentDeferred);
        return;
      } else if (typeName === "Map" && typeArgs[0] && typeArgs[1]) {
        visit(typeArgs[0], [...currentPath, "key"], "keyed", "mutable", currentDeferred);
        visit(typeArgs[1], [...currentPath, "value"], "keyed", "mutable", currentDeferred);
        return;
      } else if (typeName === "ReadonlyMap" && typeArgs[0] && typeArgs[1]) {
        visit(typeArgs[0], [...currentPath, "key"], "keyed", "readonly", currentDeferred);
        visit(typeArgs[1], [...currentPath, "value"], "keyed", "readonly", currentDeferred);
        return;
      } else if (typeName === "Record" && typeArgs[0] && typeArgs[1]) {
        visit(typeArgs[1], [...currentPath, "value"], "keyed", undefined, currentDeferred);
        return;
      } else if (typeName === "Promise" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "result"], currentCardinal, currentMutability, true);
        return;
      } else if (typeName === "Awaited" && typeArgs[0]) {
        visit(typeArgs[0], [...currentPath, "result"], currentCardinal, currentMutability, true);
        return;
      }

      // Regular type reference
      if (symbol) {
        const resolved = resolveAlias(checker, symbol);
        const id = idBySymbol.get(resolved);
        if (id) {
          found.push({
            symbol: resolved,
            path: currentPath,
            cardinality: (currentCardinal as any) || "one",
            ...(currentMutability ? { mutability: currentMutability as "mutable" | "readonly" } : {}),
            ...(currentDeferred ? { deferred: true } : {}),
          });
        }
      }
      return;
    }

    // Tuple types: [T1, T2, ...]
    if (ts.isTupleTypeNode(node)) {
      for (let i = 0; i < node.elements.length; i++) {
        const elem = node.elements[i];
        if (ts.isNamedTupleMember(elem)) {
          const name = (elem.name as any).text ?? `element:${i}`;
          visit(elem.type, [...currentPath, `element:${name}`], "one", undefined, currentDeferred);
        } else {
          visit(elem, [...currentPath, `element:${i}`], "one", undefined, currentDeferred);
        }
      }
      return;
    }

    // Union types: T1 | T2 | ...
    if (ts.isUnionTypeNode(node)) {
      for (const t of node.types) {
        visit(t, currentPath, "one", undefined, currentDeferred);
      }
      return;
    }

    // Index signature type
    if (ts.isIndexSignatureDeclaration(node) && node.type) {
      const hasReadonly = hasModifier(node, ts.SyntaxKind.ReadonlyKeyword);
      visit(node.type, [...currentPath, "value"], "keyed", hasReadonly ? "readonly" : undefined, currentDeferred);
      return;
    }

    // Parenthesized type: (T)
    if (ts.isParenthesizedTypeNode(node)) {
      visit(node.type, currentPath, currentCardinal, currentMutability, currentDeferred);
      return;
    }

    // Unknown/generic parameters: collect as-is without features
    if (ts.isTypeParameterDeclaration(node)) {
      // Skip type parameters
      return;
    }

    // Base case: direct type reference
    if (ts.isTypeReferenceNode(node)) {
      // Already handled above
      return;
    }
  };

  visit(typeNode, []);
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

function parameterName(bindingName: ts.BindingName | undefined): string | undefined {
  if (!bindingName) return undefined;
  if (ts.isIdentifier(bindingName)) {
    return bindingName.text;
  }
  // For destructuring patterns, we skip them (not supported)
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
  edgeFilter?: string[],
): { symbols: SymbolRecord[]; edges: EdgeRecord[] } {
  const symbols: SymbolRecord[] = [];
  const edgeMap = new Map<string, EdgeRecord>();
  const idBySymbol = new Map<ts.Symbol, string>();
  const declEntries: DeclEntry[] = [];

  const wantEdge = (kind: string): boolean => {
    if (!edgeFilter) return false;
    return edgeFilter.includes(kind);
  };

  const addEdge = (from: string, to: string, kind: EdgeKind, via?: ViaRecord): void => {
    if (from === to) return;
    const key = `${from}\u0000${to}\u0000${kind}\u0000${via?.member ?? ""}\u0000${JSON.stringify(via?.path ?? [])}`;
    edgeMap.set(key, { from, to, kind, ...(via ? { via } : {}) });
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

      // Handle constructor parameters (holds/uses with memberKind: constructor)
      for (const member of classDecl.members) {
        if (ts.isConstructorDeclaration(member)) {
          for (const param of member.parameters) {
            const paramType = param.type;
            if (paramType) {
              const paths = collectTypePaths(paramType, checker, validIds);
              for (const tp of paths) {
                if (tp.symbol) {
                  const targetId = validIds.get(tp.symbol);
                  if (targetId && wantEdge("uses")) {
                    const paramName = parameterName(param.name);
                    const modifiers = [];
                    if (ts.getModifiers(param)?.some(m => m.kind === ts.SyntaxKind.ReadonlyKeyword)) {
                      modifiers.push("readonly");
                    }
                    const via: ViaRecord = {
                      ...(paramName ? { member: paramName } : {}),
                      memberKind: "constructor",
                      ...(modifiers.length > 0 ? { modifiers } : {}),
                      text: checker.typeToString(checker.getTypeAtLocation(param)),
                      ...(tp.path.length > 0 ? { path: tp.path } : {}),
                      ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                      ...(tp.mutability ? { mutability: tp.mutability } : {}),
                      ...(tp.deferred ? { deferred: true } : {}),
                    };
                    addEdge(entry.id, targetId, "uses", via);
                  }
                }
              }
            }
          }
        }
      }

      for (const member of classDecl.members) {
        if (ts.isConstructorDeclaration(member)) continue;
        const name = memberName((member as ts.PropertyDeclaration | ts.MethodDeclaration).name);
        if (!name) continue;
        const visibility = classMemberVisibility(member);
        let kind: string | undefined;
        let typeNode: ts.TypeNode | undefined;
        const modifiers: string[] = [];
        if (hasModifier(member, ts.SyntaxKind.ReadonlyKeyword)) modifiers.push("readonly");
        if (hasModifier(member, ts.SyntaxKind.StaticKeyword)) modifiers.push("static");
        if (visibility === "public") modifiers.push("public");
        if (visibility === "protected") modifiers.push("protected");
        if (visibility === "private") modifiers.push("private");

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

        if (typeNode && wantEdge("holds")) {
          const paths = collectTypePaths(typeNode, checker, validIds);
          for (const tp of paths) {
            if (tp.symbol) {
              const targetId = validIds.get(tp.symbol);
              if (targetId) {
                const via: ViaRecord = {
                  member: name,
                  memberKind: kind as "field" | "property",
                  ...(modifiers.length > 0 ? { modifiers } : {}),
                  text: typeStr,
                  ...(tp.path.length > 0 ? { path: tp.path } : {}),
                  ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                  ...(tp.mutability ? { mutability: tp.mutability } : {}),
                  ...(tp.deferred ? { deferred: true } : {}),
                };
                addEdge(entry.id, targetId, "holds", via);
              }
            }
          }
        }

        if ((ts.isMethodDeclaration(member) || ts.isGetAccessor(member) || ts.isSetAccessor(member)) && wantEdge("uses")) {
          for (const param of member.parameters) {
            if (param.type) {
              const paths = collectTypePaths(param.type, checker, validIds);
              for (const tp of paths) {
                if (tp.symbol) {
                  const targetId = validIds.get(tp.symbol);
                  if (targetId) {
                    const paramName = parameterName(param.name);
                    const via: ViaRecord = {
                      ...(paramName ? { member: paramName } : {}),
                      memberKind: "parameter",
                      text: checker.typeToString(checker.getTypeAtLocation(param)),
                      ...(tp.path.length > 0 ? { path: tp.path } : {}),
                      ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                      ...(tp.mutability ? { mutability: tp.mutability } : {}),
                      ...(tp.deferred ? { deferred: true } : {}),
                    };
                    addEdge(entry.id, targetId, "uses", via);
                  }
                }
              }
            }
          }
          if (ts.isMethodDeclaration(member) && member.type) {
            const paths = collectTypePaths(member.type, checker, validIds);
            for (const tp of paths) {
              if (tp.symbol) {
                const targetId = validIds.get(tp.symbol);
                if (targetId) {
                  const via: ViaRecord = {
                    member: name,
                    memberKind: "return",
                    text: checker.typeToString(checker.getTypeAtLocation(member)),
                    ...(tp.path.length > 0 ? { path: tp.path } : {}),
                    ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                    ...(tp.mutability ? { mutability: tp.mutability } : {}),
                    ...(tp.deferred ? { deferred: true } : {}),
                  };
                  addEdge(entry.id, targetId, "uses", via);
                }
              }
            }
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
          if (member.type && wantEdge("holds")) {
            const paths = collectTypePaths(member.type, checker, validIds);
            for (const tp of paths) {
              if (tp.symbol) {
                const targetId = validIds.get(tp.symbol);
                if (targetId) {
                  const via: ViaRecord = {
                    member: name,
                    memberKind: "property",
                    text: typeStr,
                    ...(tp.path.length > 0 ? { path: tp.path } : {}),
                    ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                    ...(tp.mutability ? { mutability: tp.mutability } : {}),
                    ...(tp.deferred ? { deferred: true } : {}),
                  };
                  addEdge(entry.id, targetId, "holds", via);
                }
              }
            }
          }
        } else if (ts.isMethodSignature(member)) {
          const name = memberName(member.name);
          if (!name) continue;
          const typeStr = memberTypeString(checker, member);
          members.push(makeMember("method", name, typeStr, undefined));
          if (wantEdge("uses")) {
            for (const param of member.parameters) {
              if (param.type) {
                const paths = collectTypePaths(param.type, checker, validIds);
                for (const tp of paths) {
                  if (tp.symbol) {
                    const targetId = validIds.get(tp.symbol);
                    if (targetId) {
                      const paramName = parameterName(param.name);
                      const via: ViaRecord = {
                        ...(paramName ? { member: paramName } : {}),
                        memberKind: "parameter",
                        text: checker.typeToString(checker.getTypeAtLocation(param)),
                        ...(tp.path.length > 0 ? { path: tp.path } : {}),
                        ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                        ...(tp.mutability ? { mutability: tp.mutability } : {}),
                        ...(tp.deferred ? { deferred: true } : {}),
                      };
                      addEdge(entry.id, targetId, "uses", via);
                    }
                  }
                }
              }
            }
            if (member.type) {
              const paths = collectTypePaths(member.type, checker, validIds);
              for (const tp of paths) {
                if (tp.symbol) {
                  const targetId = validIds.get(tp.symbol);
                  if (targetId) {
                    const via: ViaRecord = {
                      member: name,
                      memberKind: "return",
                      text: checker.typeToString(checker.getTypeAtLocation(member)),
                      ...(tp.path.length > 0 ? { path: tp.path } : {}),
                      ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                      ...(tp.mutability ? { mutability: tp.mutability } : {}),
                      ...(tp.deferred ? { deferred: true } : {}),
                    };
                    addEdge(entry.id, targetId, "uses", via);
                  }
                }
              }
            }
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
      if (wantEdge("uses")) {
        const paths = collectTypePaths(aliasDecl.type, checker, validIds);
        for (const tp of paths) {
          if (tp.symbol) {
            const targetId = validIds.get(tp.symbol);
            if (targetId) {
              const typeStr = checker.typeToString(checker.getTypeAtLocation(aliasDecl));
              const via: ViaRecord = {
                member: aliasDecl.name.text,
                memberKind: "self",
                text: typeStr,
                ...(tp.path.length > 0 ? { path: tp.path } : {}),
                ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                ...(tp.mutability ? { mutability: tp.mutability } : {}),
                ...(tp.deferred ? { deferred: true } : {}),
              };
              addEdge(entry.id, targetId, "uses", via);
            }
          }
        }
      }
    } else if (entry.kind === "function") {
      if (wantEdge("uses")) {
        for (const param of entry.parameters ?? []) {
          if (param.type) {
            const paths = collectTypePaths(param.type, checker, validIds);
            for (const tp of paths) {
              if (tp.symbol) {
                const targetId = validIds.get(tp.symbol);
                if (targetId) {
                  const paramName = parameterName(param.name);
                  const via: ViaRecord = {
                    ...(paramName ? { member: paramName } : {}),
                    memberKind: "parameter",
                    text: checker.typeToString(checker.getTypeAtLocation(param)),
                    ...(tp.path.length > 0 ? { path: tp.path } : {}),
                    ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                    ...(tp.mutability ? { mutability: tp.mutability } : {}),
                    ...(tp.deferred ? { deferred: true } : {}),
                  };
                  addEdge(entry.id, targetId, "uses", via);
                }
              }
            }
          }
        }
        const paths = collectTypePaths(entry.returnType, checker, validIds);
        for (const tp of paths) {
          if (tp.symbol) {
            const targetId = validIds.get(tp.symbol);
            if (targetId) {
              const typeStr = entry.returnType ? checker.typeToString(checker.getTypeAtLocation(entry.returnType)) : "unknown";
              const via: ViaRecord = {
                memberKind: "return",
                text: typeStr,
                ...(tp.path.length > 0 ? { path: tp.path } : {}),
                ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                ...(tp.mutability ? { mutability: tp.mutability } : {}),
                ...(tp.deferred ? { deferred: true } : {}),
              };
              addEdge(entry.id, targetId, "uses", via);
            }
          }
        }
      }
    } else if (entry.kind === "value") {
      if (wantEdge("uses")) {
        const paths = collectTypePaths(entry.typeNode, checker, validIds);
        for (const tp of paths) {
          if (tp.symbol) {
            const targetId = validIds.get(tp.symbol);
            if (targetId) {
              const typeStr = entry.typeNode ? checker.typeToString(checker.getTypeAtLocation(entry.typeNode)) : "unknown";
              const via: ViaRecord = {
                memberKind: "self",
                text: typeStr,
                ...(tp.path.length > 0 ? { path: tp.path } : {}),
                ...(tp.cardinality ? { cardinality: tp.cardinality } : {}),
                ...(tp.mutability ? { mutability: tp.mutability } : {}),
                ...(tp.deferred ? { deferred: true } : {}),
              };
              addEdge(entry.id, targetId, "uses", via);
            }
          }
        }
      }
    }
  }

  return { symbols, edges: Array.from(edgeMap.values()) };
}
