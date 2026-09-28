using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Text;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Walks loaded compilations and builds the facts document per docs/EXTRACTOR.md §2 and
/// PLAN_20260923_extractors_csharp.md.
/// </summary>
internal sealed class FactsExtractor(string rootArgument, string rootFullPath, PathFilter pathFilter)
{
    private readonly Dictionary<string, TypeEntry> _types = new(StringComparer.Ordinal);
    private readonly Dictionary<string, NamespaceEntry> _namespaces = new(StringComparer.Ordinal);
    private readonly Dictionary<string, AssemblyEntry> _assemblies = new(StringComparer.Ordinal);

    // Method-or-property id -> the symbol behind it, printed only with --edges
    // calls (ADR_20260928-4). Kept for later passes (overrides/implements/calls)
    // that need to resolve a call target back to a symbol of this output.
    private readonly Dictionary<string, ISymbol> _methodSymbols = new(StringComparer.Ordinal);

    private sealed class AssemblyEntry
    {
        public required string Name;
        public required string File;
        public readonly HashSet<string> TopLevelTypes = new(StringComparer.Ordinal);
        public readonly HashSet<string> ReferencedAssemblies = new(StringComparer.Ordinal);
    }

    private sealed class TypeEntry
    {
        public required INamedTypeSymbol Symbol;
        public required string File;
        public required int Line;
        public int EndLine;
        // Every declaration (partial types have more than one); sorted by
        // (File, Line) once all syntax trees are processed.
        public readonly List<(string File, int Line, int EndLine)> Declarations = [];
    }

    private sealed class NamespaceEntry
    {
        public required INamespaceSymbol Symbol;
        public string? File;
        public int? Line;
    }

    private readonly HashSet<string>? _requestedEdgeKinds;

    /// <summary>
    /// Constructor that accepts options for edge kinds filtering.
    /// </summary>
    internal FactsExtractor(string rootArgument, string rootFullPath, PathFilter pathFilter, HashSet<string>? requestedEdgeKinds = null)
        : this(rootArgument, rootFullPath, pathFilter)
    {
        _requestedEdgeKinds = requestedEdgeKinds?.Count > 0 ? requestedEdgeKinds : null;
    }

    /// <summary>
    /// Processes one loaded project. The project itself becomes a <c>module</c>/<c>assembly</c>
    /// symbol (ADR_20260923-9) when its .csproj lies under the root and passes the path filter;
    /// multi-targeted projects (one Roslyn project per TFM) merge into one symbol by assembly name.
    /// </summary>
    public void ProcessProject(Project project, Compilation compilation)
    {
        var assembly = ConsiderAssembly(project);

        foreach (var tree in compilation.SyntaxTrees)
        {
            if (tree.FilePath.Length == 0)
            {
                continue;
            }

            var relPath = RelativePath(tree.FilePath);
            if (relPath is null || !pathFilter.IsIncluded(relPath))
            {
                continue;
            }

            var text = tree.GetText();
            var firstLines = FirstLines(text, 5);
            if (pathFilter.IsExcluded(relPath, firstLines))
            {
                continue;
            }

            var semanticModel = compilation.GetSemanticModel(tree);
            var root = tree.GetRoot();

            foreach (var node in root.DescendantNodes().Where(IsTypeLikeDeclaration))
            {
                if (semanticModel.GetDeclaredSymbol(node) is not INamedTypeSymbol symbol)
                {
                    continue;
                }

                var lineSpan = node.GetLocation().GetLineSpan();
                var line = lineSpan.StartLinePosition.Line + 1;
                var endLine = lineSpan.EndLinePosition.Line + 1;
                Consider(symbol, relPath, line, endLine);

                // A top-level type declared in this project's sources: the project contains it.
                // A linked file compiled into two projects gives "contains" from both.
                if (assembly is not null && symbol.ContainingType is null)
                {
                    var typeId = SymbolIds.TypeId(symbol);
                    if (_types.ContainsKey(typeId))
                    {
                        assembly.TopLevelTypes.Add(typeId);
                    }
                }
            }
        }
    }

    private AssemblyEntry? ConsiderAssembly(Project project)
    {
        if (project.FilePath is null || project.AssemblyName.Length == 0)
        {
            return null;
        }

        var relPath = RelativePath(project.FilePath);
        if (relPath is null || !pathFilter.IsIncluded(relPath) || pathFilter.IsExcluded(relPath, null))
        {
            return null;
        }

        var id = SymbolIds.AssemblyId(project.AssemblyName);
        if (!_assemblies.TryGetValue(id, out var entry))
        {
            entry = new AssemblyEntry { Name = project.AssemblyName, File = relPath };
            _assemblies[id] = entry;
        }
        else if (string.CompareOrdinal(relPath, entry.File) < 0)
        {
            entry.File = relPath;
        }

        foreach (var reference in project.ProjectReferences)
        {
            var target = project.Solution.GetProject(reference.ProjectId);
            if (target is not null && target.AssemblyName.Length > 0)
            {
                entry.ReferencedAssemblies.Add(SymbolIds.AssemblyId(target.AssemblyName));
            }
        }

        return entry;
    }

    private void Consider(INamedTypeSymbol symbol, string relPath, int line, int endLine)
    {
        if (!IsEligibleKind(symbol.TypeKind))
        {
            return;
        }

        if (HasGeneratedAttribute(symbol))
        {
            return;
        }

        var id = SymbolIds.TypeId(symbol);
        if (_types.TryGetValue(id, out var existing))
        {
            // partial declarations: keep the first declaration by path/line ordinal sort,
            // but remember every declaration for `spans`.
            existing.Declarations.Add((relPath, line, endLine));
            if (string.CompareOrdinal(relPath, existing.File) < 0 ||
                (relPath == existing.File && line < existing.Line))
            {
                existing.File = relPath;
                existing.Line = line;
                existing.EndLine = endLine;
            }

            return;
        }

        var entry = new TypeEntry { Symbol = symbol, File = relPath, Line = line, EndLine = endLine };
        entry.Declarations.Add((relPath, line, endLine));
        _types[id] = entry;

        if (!symbol.ContainingNamespace.IsGlobalNamespace)
        {
            var nsId = SymbolIds.NamespaceId(symbol.ContainingNamespace);
            if (!_namespaces.ContainsKey(nsId))
            {
                _namespaces[nsId] = new NamespaceEntry { Symbol = symbol.ContainingNamespace };
            }
        }
    }

    private static bool IsEligibleKind(TypeKind kind) =>
        kind is TypeKind.Class or TypeKind.Struct or TypeKind.Enum or TypeKind.Interface or TypeKind.Delegate;

    private static bool HasGeneratedAttribute(ISymbol symbol)
    {
        foreach (var attr in symbol.GetAttributes())
        {
            var name = attr.AttributeClass?.Name;
            if (name is "GeneratedCodeAttribute" or "CompilerGeneratedAttribute")
            {
                return true;
            }
        }

        return false;
    }

    private static bool IsTypeLikeDeclaration(SyntaxNode node) => node is
        ClassDeclarationSyntax or
        StructDeclarationSyntax or
        RecordDeclarationSyntax or
        InterfaceDeclarationSyntax or
        EnumDeclarationSyntax or
        DelegateDeclarationSyntax;

    private static string? FirstLines(SourceText text, int count)
    {
        var lines = text.Lines;
        var take = Math.Min(count, lines.Count);
        if (take == 0)
        {
            return null;
        }

        return text.ToString(TextSpan.FromBounds(lines[0].Start, lines[take - 1].End));
    }

    private string? RelativePath(string fullPath)
    {
        var full = Path.GetFullPath(fullPath);
        if (!full.StartsWith(rootFullPath, StringComparison.OrdinalIgnoreCase))
        {
            return null;
        }

        var rel = Path.GetRelativePath(rootFullPath, full);
        return PathFilter.NormalizeRelative(rel);
    }

    public FactsDocument Build()
    {
        ResolveNamespaceLocations();

        var outputIds = new HashSet<string>(_types.Keys, StringComparer.Ordinal);
        var symbols = new List<SymbolFact>();
        var edges = new HashSet<EdgeWithVia>();

        foreach (var (nsId, entry) in _namespaces)
        {
            symbols.Add(new SymbolFact
            {
                Id = nsId,
                Kind = "module",
                NativeKind = "namespace",
                Name = entry.Symbol.Name,
                Namespace = "",
                File = entry.File ?? "",
                Line = entry.Line,
                Members = null,
            });
        }

        // Assembly (project) symbols: assembly -> top-level type "contains", assembly -> assembly
        // "depends" for a ProjectReference between two projects in this output.
        foreach (var (asmId, entry) in _assemblies)
        {
            symbols.Add(new SymbolFact
            {
                Id = asmId,
                Kind = "module",
                NativeKind = "assembly",
                Name = entry.Name,
                Namespace = "",
                File = entry.File,
                Members = null,
            });

            foreach (var typeId in entry.TopLevelTypes)
            {
                edges.Add(new EdgeWithVia { From = asmId, To = typeId, Kind = "contains" });
            }

            foreach (var targetId in entry.ReferencedAssemblies)
            {
                if (targetId != asmId && _assemblies.ContainsKey(targetId))
                {
                    edges.Add(new EdgeWithVia { From = asmId, To = targetId, Kind = "depends" });
                }
            }
        }

        // Namespace -> type, outer type -> nested type "contains" edges.
        foreach (var (id, entry) in _types)
        {
            var symbol = entry.Symbol;
            if (symbol.ContainingType is null && !symbol.ContainingNamespace.IsGlobalNamespace)
            {
                var nsId = SymbolIds.NamespaceId(symbol.ContainingNamespace);
                edges.Add(new EdgeWithVia { From = nsId, To = id, Kind = "contains" });
            }
            else if (symbol.ContainingType is not null)
            {
                var outerId = SymbolIds.TypeId(symbol.ContainingType);
                if (outputIds.Contains(outerId))
                {
                    edges.Add(new EdgeWithVia { From = outerId, To = id, Kind = "contains" });
                }
            }
        }

        foreach (var (id, entry) in _types)
        {
            var symbol = entry.Symbol;
            symbols.Add(BuildTypeSymbolFact(id, symbol, entry, outputIds, edges));
        }

        var printMethods = _requestedEdgeKinds?.Contains("calls") == true;
        if (printMethods)
        {
            // Two passes: every method symbol of every type first, then
            // overrides/implements, which need the *target* method (possibly
            // of a type visited later) already known.
            foreach (var (id, entry) in _types)
            {
                CollectMethods(id, entry.Symbol, entry.File, symbols, edges);
            }

            foreach (var (id, entry) in _types)
            {
                AddMethodLevelEdges(id, entry.Symbol, edges);
            }
        }

        symbols.Sort((a, b) => string.CompareOrdinal(a.Id, b.Id));

        var edgeFacts = edges
            .OrderBy(e => e.From, StringComparer.Ordinal)
            .ThenBy(e => e.To, StringComparer.Ordinal)
            .ThenBy(e => e.Kind, StringComparer.Ordinal)
            .ThenBy(e => e.Via?.Member ?? "", StringComparer.Ordinal)
            .ThenBy(e => string.Join("/", e.Via?.Path ?? []), StringComparer.Ordinal)
            .Select(e => new EdgeFact
            {
                From = e.From,
                To = e.To,
                Kind = e.Kind,
                Via = e.Via,
                Line = e.Line,
                File = e.File,
            })
            .ToList();

        // Always include base edge kinds plus requested member-based edges
        var edgeKinds = new List<string> { "contains", "depends", "extends", "implements" };
        if (_requestedEdgeKinds is not null && _requestedEdgeKinds.Count > 0)
        {
            if (_requestedEdgeKinds.Contains("holds"))
            {
                edgeKinds.Add("holds");
            }
            if (_requestedEdgeKinds.Contains("uses"))
            {
                edgeKinds.Add("uses");
            }
            if (_requestedEdgeKinds.Contains("injects"))
            {
                edgeKinds.Add("injects");
            }
            if (_requestedEdgeKinds.Contains("calls"))
            {
                edgeKinds.Add("calls");
            }
        }
        edgeKinds.Sort();

        return new FactsDocument
        {
            Language = "csharp",
            Root = rootArgument,
            EdgeKinds = edgeKinds,
            Symbols = symbols,
            Edges = edgeFacts,
        };
    }

    private void ResolveNamespaceLocations()
    {
        foreach (var entry in _namespaces.Values)
        {
            string? bestFile = null;
            var bestLine = int.MaxValue;

            foreach (var location in entry.Symbol.Locations)
            {
                if (location.SourceTree is null)
                {
                    continue;
                }

                var relPath = RelativePath(location.SourceTree.FilePath);
                if (relPath is null || !pathFilter.IsIncluded(relPath))
                {
                    continue;
                }

                var text = location.SourceTree.GetText();
                var firstLines = FirstLines(text, 5);
                if (pathFilter.IsExcluded(relPath, firstLines))
                {
                    continue;
                }

                var line = location.GetLineSpan().StartLinePosition.Line + 1;
                if (bestFile is null || string.CompareOrdinal(relPath, bestFile) < 0 ||
                    (relPath == bestFile && line < bestLine))
                {
                    bestFile = relPath;
                    bestLine = line;
                }
            }

            if (bestFile is not null)
            {
                entry.File = bestFile;
                entry.Line = bestLine;
                continue;
            }

            // Fallback: earliest declaration among output types owned by this namespace.
            foreach (var typeEntry in _types.Values)
            {
                if (typeEntry.Symbol.ContainingType is not null) continue;
                if (!SymbolEqualityComparer.Default.Equals(typeEntry.Symbol.ContainingNamespace, entry.Symbol)) continue;

                if (entry.File is null || string.CompareOrdinal(typeEntry.File, entry.File) < 0 ||
                    (typeEntry.File == entry.File && typeEntry.Line < entry.Line))
                {
                    entry.File = typeEntry.File;
                    entry.Line = typeEntry.Line;
                }
            }
        }
    }

    private SymbolFact BuildTypeSymbolFact(
        string id,
        INamedTypeSymbol symbol,
        TypeEntry entry,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges)
    {
        var (kind, nativeKind) = ClassifyType(symbol);
        var (members, memberLines) = MembersBuilder.Build(symbol, kind, entry.File, m => SourceLocation(m));

        var baseListLocation = BaseListLocation(symbol);
        AddStructuralEdges(id, symbol, outputIds, edges, entry.File, baseListLocation);
        ReferenceEdgeCollector.Collect(id, symbol, outputIds, edges, _requestedEdgeKinds, entry.File, m => SourceLocation(m));

        // A file linked into two projects yields one Consider() call per project for the
        // same physical declaration: dedupe by (file, line) before deciding whether there
        // is more than one real declaration.
        var distinctDeclarations = entry.Declarations
            .GroupBy(d => (d.File, d.Line))
            .Select(g => g.First())
            .ToList();

        List<SpanFact>? spans = null;
        if (distinctDeclarations.Count > 1)
        {
            spans = distinctDeclarations
                .OrderBy(d => d.File, StringComparer.Ordinal)
                .ThenBy(d => d.Line)
                .Select(d => new SpanFact { File = d.File, Line = d.Line, EndLine = d.EndLine })
                .ToList();
        }

        return new SymbolFact
        {
            Id = id,
            Kind = kind,
            NativeKind = nativeKind,
            Name = symbol.Name,
            Namespace = SymbolIds.NamespaceDottedName(symbol.ContainingNamespace),
            File = entry.File,
            Line = entry.Line,
            EndLine = entry.EndLine,
            Spans = spans,
            Visibility = SymbolDisplayHelpers.Visibility(symbol.DeclaredAccessibility),
            Members = members,
            MemberLines = memberLines,
        };
    }

    /// <summary>
    /// Resolves a symbol's declaration to (file, line) within the root, for
    /// `memberLines` and edge `line`/`file`. The first location that lies
    /// under the root wins; a symbol with none (e.g. from metadata) yields
    /// null.
    /// </summary>
    private (string File, int Line)? SourceLocation(ISymbol symbol)
    {
        foreach (var location in symbol.Locations)
        {
            if (location.SourceTree is null)
            {
                continue;
            }

            var relPath = RelativePath(location.SourceTree.FilePath);
            if (relPath is null)
            {
                continue;
            }

            var line = location.GetLineSpan().StartLinePosition.Line + 1;
            return (relPath, line);
        }

        return null;
    }

    /// <summary>
    /// Finds where the base list (`: Base, IFoo`) is written, across every
    /// partial declaration; the first one that has a base list wins.
    /// </summary>
    private (string File, int Line)? BaseListLocation(INamedTypeSymbol symbol)
    {
        foreach (var syntaxRef in symbol.DeclaringSyntaxReferences)
        {
            var baseList = syntaxRef.GetSyntax() switch
            {
                ClassDeclarationSyntax c => c.BaseList,
                StructDeclarationSyntax s => s.BaseList,
                RecordDeclarationSyntax r => r.BaseList,
                InterfaceDeclarationSyntax i => i.BaseList,
                _ => null,
            };
            if (baseList is null)
            {
                continue;
            }

            var relPath = RelativePath(baseList.SyntaxTree.FilePath);
            if (relPath is null)
            {
                continue;
            }

            var line = baseList.GetLocation().GetLineSpan().StartLinePosition.Line + 1;
            return (relPath, line);
        }

        return null;
    }

    private static void AddStructuralEdges(
        string id,
        INamedTypeSymbol symbol,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges,
        string fromFile,
        (string File, int Line)? baseListLocation)
    {
        int? line = baseListLocation?.Line;
        string? file = baseListLocation is { } loc && loc.File != fromFile ? loc.File : null;

        if (symbol.TypeKind == TypeKind.Class && symbol.BaseType is { } baseType && IsExtendableBase(baseType))
        {
            var baseId = SymbolIds.TypeId(baseType);
            if (outputIds.Contains(baseId) && baseId != id)
            {
                edges.Add(new EdgeWithVia { From = id, To = baseId, Kind = "extends", Line = line, File = file });
            }
        }

        foreach (var iface in symbol.Interfaces)
        {
            var ifaceId = SymbolIds.TypeId(iface);
            if (outputIds.Contains(ifaceId) && ifaceId != id)
            {
                edges.Add(new EdgeWithVia { From = id, To = ifaceId, Kind = "implements", Line = line, File = file });
            }
        }
    }

    /// <summary>
    /// Methods, constructors, properties, indexers and operators of <paramref name="symbol"/>
    /// (ADR_20260928-4 §1), plus the `contains` edge from the type. Property and indexer
    /// accessors are not separate symbols: get/set is one symbol (EXTRACTOR.md §3). A
    /// `partial` method's declaration and implementation are one symbol keyed by the
    /// implementation part, with `spans` when there is more than one declaration under
    /// the root.
    /// </summary>
    private void CollectMethods(
        string typeId,
        INamedTypeSymbol symbol,
        string typeFile,
        List<SymbolFact> symbols,
        HashSet<EdgeWithVia> edges)
    {
        var seen = new HashSet<string>(StringComparer.Ordinal);

        foreach (var member in symbol.GetMembers())
        {
            switch (member)
            {
                case IMethodSymbol method when IsCallsEligibleMethod(method):
                {
                    var canonical = method.PartialImplementationPart ?? method;
                    var id = SymbolIds.MethodId(canonical);
                    if (!seen.Add(id))
                    {
                        break;
                    }

                    var name = canonical.ExplicitInterfaceImplementations.Length > 0
                        ? canonical.ExplicitInterfaceImplementations[0].Name
                        : canonical.Name;
                    AddMethodSymbol(id, typeId, canonical, name, NativeKindOf(canonical),
                        symbols, edges, PartialSyntaxRefs(canonical));
                    break;
                }

                case IPropertySymbol property when !property.IsImplicitlyDeclared:
                {
                    var id = property.IsIndexer ? SymbolIds.IndexerId(property) : SymbolIds.PropertyId(property);
                    if (!seen.Add(id))
                    {
                        break;
                    }

                    var name = property.IsIndexer ? "this" : property.Name;
                    AddMethodSymbol(id, typeId, property, name, property.IsIndexer ? "indexer" : "property",
                        symbols, edges, property.DeclaringSyntaxReferences.ToList());
                    break;
                }
            }
        }
    }

    private static bool IsCallsEligibleMethod(IMethodSymbol method)
    {
        // The implicit parameterless constructor is handled where field/property
        // initialisers are attributed to it (docs/extractors/csharp.md); every
        // other implicitly declared method (accessors, event add/remove, the
        // default Equals/GetHashCode of a record, …) is not a symbol of its own.
        if (method.IsImplicitlyDeclared)
        {
            return false;
        }

        return method.MethodKind is MethodKind.Ordinary or MethodKind.Constructor or
            MethodKind.StaticConstructor or MethodKind.UserDefinedOperator or MethodKind.Conversion or
            MethodKind.ExplicitInterfaceImplementation;
    }

    private static string NativeKindOf(IMethodSymbol method) => method.MethodKind switch
    {
        MethodKind.Constructor or MethodKind.StaticConstructor => "constructor",
        MethodKind.UserDefinedOperator or MethodKind.Conversion => "operator",
        _ => "method",
    };

    /// <summary>
    /// Every declaring syntax reference of a partial method's canonical (implementation)
    /// part, plus its definition part's — so `spans` sees both halves of a `partial` pair.
    /// </summary>
    private static List<SyntaxReference> PartialSyntaxRefs(IMethodSymbol canonical)
    {
        var refs = canonical.DeclaringSyntaxReferences.ToList();
        if (canonical.PartialDefinitionPart is { } definition)
        {
            refs.AddRange(definition.DeclaringSyntaxReferences);
        }

        return refs;
    }

    private void AddMethodSymbol(
        string id,
        string typeId,
        ISymbol symbol,
        string name,
        string nativeKind,
        List<SymbolFact> symbols,
        HashSet<EdgeWithVia> edges,
        List<SyntaxReference> syntaxRefs)
    {
        var declarations = syntaxRefs
            .Select(r => r.GetSyntax())
            .Select(node => (RelativePath(node.SyntaxTree.FilePath), node.GetLocation().GetLineSpan()))
            .Where(t => t.Item1 is not null)
            .Select(t => (File: t.Item1!, Line: t.Item2.StartLinePosition.Line + 1, EndLine: t.Item2.EndLinePosition.Line + 1))
            .Distinct()
            .OrderBy(t => t.File, StringComparer.Ordinal)
            .ThenBy(t => t.Line)
            .ToList();

        if (declarations.Count == 0)
        {
            // No location under root (e.g. from metadata): not this output's to print.
            return;
        }

        var first = declarations[0];
        List<SpanFact>? spans = declarations.Count > 1
            ? declarations.Select(d => new SpanFact { File = d.File, Line = d.Line, EndLine = d.EndLine }).ToList()
            : null;

        symbols.Add(new SymbolFact
        {
            Id = id,
            Kind = "method",
            NativeKind = nativeKind,
            Name = name,
            Namespace = "",
            File = first.File,
            Line = first.Line,
            EndLine = first.EndLine,
            Spans = spans,
            Visibility = SymbolDisplayHelpers.Visibility(symbol.DeclaredAccessibility),
        });

        edges.Add(new EdgeWithVia { From = typeId, To = id, Kind = "contains" });
        _methodSymbols[id] = symbol;
    }

    /// <summary>
    /// `overrides` (method -> the base virtual method it overrides) and `implements`
    /// (method -> the interface method it implements, implicit and explicit alike)
    /// for the methods of <paramref name="symbol"/> already collected by
    /// <see cref="CollectMethods"/> (ADR_20260928-4 §1, §3). Both ends must be
    /// symbols of this output; an edge is only added from the type that actually
    /// declares the overriding/implementing method, not from a subclass that
    /// merely inherits it.
    /// </summary>
    private void AddMethodLevelEdges(string typeId, INamedTypeSymbol symbol, HashSet<EdgeWithVia> edges)
    {
        foreach (var member in symbol.GetMembers().OfType<IMethodSymbol>())
        {
            if (member.OverriddenMethod is not { } overridden || !IsCallsEligibleMethod(member))
            {
                continue;
            }

            var fromId = SymbolIds.MethodId(member.PartialImplementationPart ?? member);
            var toId = SymbolIds.MethodId(overridden.OriginalDefinition);
            if (fromId != toId && _methodSymbols.ContainsKey(fromId) && _methodSymbols.ContainsKey(toId))
            {
                edges.Add(new EdgeWithVia { From = fromId, To = toId, Kind = "overrides" });
            }
        }

        foreach (var iface in symbol.Interfaces)
        {
            foreach (var ifaceMethod in iface.GetMembers().OfType<IMethodSymbol>().Where(m => m.MethodKind == MethodKind.Ordinary))
            {
                if (symbol.FindImplementationForInterfaceMember(ifaceMethod) is not IMethodSymbol impl ||
                    !SymbolEqualityComparer.Default.Equals(impl.ContainingType, symbol))
                {
                    continue; // not declared on this type: no edge from here
                }

                var fromId = SymbolIds.MethodId(impl.PartialImplementationPart ?? impl);
                var toId = SymbolIds.MethodId(ifaceMethod);
                if (fromId != toId && _methodSymbols.ContainsKey(fromId) && _methodSymbols.ContainsKey(toId))
                {
                    edges.Add(new EdgeWithVia { From = fromId, To = toId, Kind = "implements" });
                }
            }

            foreach (var ifaceProperty in iface.GetMembers().OfType<IPropertySymbol>())
            {
                if (symbol.FindImplementationForInterfaceMember(ifaceProperty) is not IPropertySymbol impl ||
                    !SymbolEqualityComparer.Default.Equals(impl.ContainingType, symbol))
                {
                    continue;
                }

                var fromId = impl.IsIndexer ? SymbolIds.IndexerId(impl) : SymbolIds.PropertyId(impl);
                var toId = ifaceProperty.IsIndexer ? SymbolIds.IndexerId(ifaceProperty) : SymbolIds.PropertyId(ifaceProperty);
                if (fromId != toId && _methodSymbols.ContainsKey(fromId) && _methodSymbols.ContainsKey(toId))
                {
                    edges.Add(new EdgeWithVia { From = fromId, To = toId, Kind = "implements" });
                }
            }
        }
    }

    private static bool IsExtendableBase(INamedTypeSymbol baseType) => baseType.SpecialType is not (
        SpecialType.System_Object or
        SpecialType.System_ValueType or
        SpecialType.System_Enum or
        SpecialType.System_Delegate or
        SpecialType.System_MulticastDelegate);

    private static (string Kind, string NativeKind) ClassifyType(INamedTypeSymbol symbol) => symbol.TypeKind switch
    {
        TypeKind.Interface => ("interface", "interface"),
        TypeKind.Delegate => ("function", "delegate"),
        TypeKind.Enum => ("type", "enum"),
        TypeKind.Struct => ("type", symbol.IsRecord ? "record-struct" : "struct"),
        TypeKind.Class => ("type", ClassifyClass(symbol)),
        _ => ("type", symbol.TypeKind.ToString().ToLowerInvariant()),
    };

    private static string ClassifyClass(INamedTypeSymbol symbol)
    {
        if (symbol.IsRecord) return "record";
        if (symbol.IsStatic) return "static-class";
        if (symbol.IsAbstract) return "abstract-class";
        return "class";
    }
}
