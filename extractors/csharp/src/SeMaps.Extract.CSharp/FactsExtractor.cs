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

    private sealed class TypeEntry
    {
        public required INamedTypeSymbol Symbol;
        public required string File;
        public required int Line;
    }

    private sealed class NamespaceEntry
    {
        public required INamespaceSymbol Symbol;
        public string? File;
        public int? Line;
    }

    public void ProcessCompilation(Compilation compilation)
    {
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

                var line = node.GetLocation().GetLineSpan().StartLinePosition.Line + 1;
                Consider(symbol, relPath, line);
            }
        }
    }

    private void Consider(INamedTypeSymbol symbol, string relPath, int line)
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
            // partial declarations: keep the first declaration by path/line ordinal sort.
            if (string.CompareOrdinal(relPath, existing.File) < 0 ||
                (relPath == existing.File && line < existing.Line))
            {
                existing.File = relPath;
                existing.Line = line;
            }

            return;
        }

        _types[id] = new TypeEntry { Symbol = symbol, File = relPath, Line = line };

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
        var edges = new HashSet<(string From, string To, string Kind)>();

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

        // Namespace -> type, outer type -> nested type "contains" edges.
        foreach (var (id, entry) in _types)
        {
            var symbol = entry.Symbol;
            if (symbol.ContainingType is null && !symbol.ContainingNamespace.IsGlobalNamespace)
            {
                var nsId = SymbolIds.NamespaceId(symbol.ContainingNamespace);
                edges.Add((nsId, id, "contains"));
            }
            else if (symbol.ContainingType is not null)
            {
                var outerId = SymbolIds.TypeId(symbol.ContainingType);
                if (outputIds.Contains(outerId))
                {
                    edges.Add((outerId, id, "contains"));
                }
            }
        }

        foreach (var (id, entry) in _types)
        {
            var symbol = entry.Symbol;
            symbols.Add(BuildTypeSymbolFact(id, symbol, entry.File, entry.Line, outputIds, edges));
        }

        symbols.Sort((a, b) => string.CompareOrdinal(a.Id, b.Id));

        var edgeFacts = edges
            .Select(e => new EdgeFact { From = e.From, To = e.To, Kind = e.Kind })
            .OrderBy(e => e.From, StringComparer.Ordinal)
            .ThenBy(e => e.To, StringComparer.Ordinal)
            .ThenBy(e => e.Kind, StringComparer.Ordinal)
            .ToList();

        return new FactsDocument
        {
            Language = "csharp",
            Root = rootArgument,
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

    private static SymbolFact BuildTypeSymbolFact(
        string id,
        INamedTypeSymbol symbol,
        string file,
        int line,
        HashSet<string> outputIds,
        HashSet<(string From, string To, string Kind)> edges)
    {
        var (kind, nativeKind) = ClassifyType(symbol);
        var members = MembersBuilder.Build(symbol, kind);

        AddStructuralEdges(id, symbol, outputIds, edges);
        ReferenceEdgeCollector.Collect(id, symbol, outputIds, edges);

        return new SymbolFact
        {
            Id = id,
            Kind = kind,
            NativeKind = nativeKind,
            Name = symbol.Name,
            Namespace = SymbolIds.NamespaceDottedName(symbol.ContainingNamespace),
            File = file,
            Line = line,
            Visibility = SymbolDisplayHelpers.Visibility(symbol.DeclaredAccessibility),
            Members = members,
        };
    }

    private static void AddStructuralEdges(
        string id,
        INamedTypeSymbol symbol,
        HashSet<string> outputIds,
        HashSet<(string From, string To, string Kind)> edges)
    {
        if (symbol.TypeKind == TypeKind.Class && symbol.BaseType is { } baseType && IsExtendableBase(baseType))
        {
            var baseId = SymbolIds.TypeId(baseType);
            if (outputIds.Contains(baseId) && baseId != id)
            {
                edges.Add((id, baseId, "extends"));
            }
        }

        foreach (var iface in symbol.Interfaces)
        {
            var ifaceId = SymbolIds.TypeId(iface);
            if (outputIds.Contains(ifaceId) && ifaceId != id)
            {
                edges.Add((id, ifaceId, "implements"));
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
