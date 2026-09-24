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

                var line = node.GetLocation().GetLineSpan().StartLinePosition.Line + 1;
                Consider(symbol, relPath, line);

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
            symbols.Add(BuildTypeSymbolFact(id, symbol, entry.File, entry.Line, outputIds, edges));
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
            })
            .ToList();

        // Determine edge kinds to report
        List<string>? edgeKinds = null;
        if (_requestedEdgeKinds is not null && _requestedEdgeKinds.Count > 0)
        {
            edgeKinds = new List<string> { "extends", "implements", "contains", "depends" };
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
            edgeKinds.Sort();
        }

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
        string file,
        int line,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges)
    {
        var (kind, nativeKind) = ClassifyType(symbol);
        var members = MembersBuilder.Build(symbol, kind);

        AddStructuralEdges(id, symbol, outputIds, edges);
        ReferenceEdgeCollector.Collect(id, symbol, outputIds, edges, _requestedEdgeKinds);

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
        HashSet<EdgeWithVia> edges)
    {
        if (symbol.TypeKind == TypeKind.Class && symbol.BaseType is { } baseType && IsExtendableBase(baseType))
        {
            var baseId = SymbolIds.TypeId(baseType);
            if (outputIds.Contains(baseId) && baseId != id)
            {
                edges.Add(new EdgeWithVia { From = id, To = baseId, Kind = "extends" });
            }
        }

        foreach (var iface in symbol.Interfaces)
        {
            var ifaceId = SymbolIds.TypeId(iface);
            if (outputIds.Contains(ifaceId) && ifaceId != id)
            {
                edges.Add(new EdgeWithVia { From = id, To = ifaceId, Kind = "implements" });
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
