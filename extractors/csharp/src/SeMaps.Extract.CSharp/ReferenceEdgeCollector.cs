using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Collects member-based edges: `holds` for field/property/event/indexer, `uses` for
/// method params/returns (including constructor params), plus `depends` for module->module
/// (ProjectReference) dependencies. Only to symbols present in this output; A→A is never
/// printed. Generates edges per docs/EXTRACTOR.md §2.2 with via information per §2.2a
/// and docs/extractors/csharp.md.
/// </summary>
internal static class ReferenceEdgeCollector
{
    private sealed class SymbolFeatures
    {
        public string? Cardinality { get; set; }
        public string? Mutability { get; set; }
        public bool Deferred { get; set; }
        public List<string> Path { get; set; } = [];
    }

    public static void Collect(
        string fromId,
        INamedTypeSymbol symbol,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges,
        HashSet<string>? requestedEdgeKinds)
    {
        foreach (var member in symbol.GetMembers())
        {
            if (member.IsImplicitlyDeclared) continue;

            switch (member)
            {
                case IFieldSymbol field:
                    AddForMember(fromId, field, field.Type, "holds", outputIds, edges, requestedEdgeKinds);
                    break;

                case IPropertySymbol { IsIndexer: false } property:
                    AddForMember(fromId, property, property.Type, "holds", outputIds, edges, requestedEdgeKinds);
                    break;

                case IEventSymbol @event when symbol.TypeKind == TypeKind.Interface:
                    AddForMember(fromId, @event, @event.Type, "holds", outputIds, edges, requestedEdgeKinds);
                    break;

                case IMethodSymbol method
                    when IsScannedMethod(method, symbol):
                    AddForMethodSignature(fromId, method, outputIds, edges, requestedEdgeKinds);
                    break;
            }
        }

        if (symbol.TypeKind == TypeKind.Delegate && symbol.DelegateInvokeMethod is { } invoke)
        {
            AddForMethodSignature(fromId, invoke, outputIds, edges, requestedEdgeKinds);
        }
    }

    private static bool IsScannedMethod(IMethodSymbol method, INamedTypeSymbol containingType)
    {
        if (containingType.TypeKind == TypeKind.Interface)
        {
            return method.MethodKind == MethodKind.Ordinary;
        }

        // Classes/structs/records: methods and constructors of any visibility (ADR-6).
        return method.MethodKind is MethodKind.Ordinary or MethodKind.Constructor;
    }

    private static void AddForMember(
        string fromId,
        ISymbol member,
        ITypeSymbol type,
        string edgeKind,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges,
        HashSet<string>? requestedEdgeKinds)
    {
        // Skip if edge kind not requested (holds/uses edges only when explicitly requested)
        if (requestedEdgeKinds is null || !requestedEdgeKinds.Contains(edgeKind))
        {
            return;
        }

        var memberName = member.Name;
        var memberKindStr = GetMemberKind(member);
        var modifiers = ExtractModifiers(member);
        var typeText = SymbolDisplayHelpers.TypeToDisplayString(type);

        // Get all referenced types and their features
        var typeAnalysis = AnalyzeType(type);

        foreach (var analyzed in typeAnalysis)
        {
            var toId = SymbolIds.TypeId(analyzed.ReferencedType);
            if (toId == fromId) continue;
            if (!outputIds.Contains(toId)) continue;

            var via = new ViaFact
            {
                Member = memberName,
                MemberKind = memberKindStr,
                Modifiers = modifiers.Count > 0 ? modifiers : null,
                Text = typeText,
                Path = analyzed.Features.Path.Count > 0 ? analyzed.Features.Path : null,
                Cardinality = analyzed.Features.Cardinality,
                Mutability = analyzed.Features.Mutability,
                Deferred = analyzed.Features.Deferred ? true : null,
            };

            edges.Add(new EdgeWithVia { From = fromId, To = toId, Kind = edgeKind, Via = via });
        }
    }

    private static void AddForMethodSignature(
        string fromId,
        IMethodSymbol method,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges,
        HashSet<string>? requestedEdgeKinds)
    {
        // Return type: uses
        AddForParameter(fromId, method.ReturnType, "return", "return", method, outputIds, edges, requestedEdgeKinds);

        // Parameters
        foreach (var param in method.Parameters)
        {
            var memberKindStr = method.MethodKind == MethodKind.Constructor ? "constructor" : "parameter";
            AddForParameter(fromId, param.Type, param.Name, memberKindStr, method, outputIds, edges, requestedEdgeKinds);
        }
    }

    private static void AddForParameter(
        string fromId,
        ITypeSymbol type,
        string paramName,
        string memberKindStr,
        IMethodSymbol method,
        HashSet<string> outputIds,
        HashSet<EdgeWithVia> edges,
        HashSet<string>? requestedEdgeKinds)
    {
        var edgeKind = "uses";

        // Filter by requested edge kinds (uses/injects edges only when explicitly requested)
        if (requestedEdgeKinds is null)
        {
            return;
        }

        // injects filters only constructor uses edges
        if (memberKindStr == "constructor" && requestedEdgeKinds.Contains("injects"))
        {
            // Include this edge
        }
        else if (!requestedEdgeKinds.Contains(edgeKind))
        {
            return;
        }

        var typeText = SymbolDisplayHelpers.TypeToDisplayString(type);

        var typeAnalysis = AnalyzeType(type);
        foreach (var analyzed in typeAnalysis)
        {
            var toId = SymbolIds.TypeId(analyzed.ReferencedType);
            if (toId == fromId) continue;
            if (!outputIds.Contains(toId)) continue;

            var via = new ViaFact
            {
                Member = paramName,
                MemberKind = memberKindStr,
                Text = typeText,
                Path = analyzed.Features.Path.Count > 0 ? analyzed.Features.Path : null,
                Cardinality = analyzed.Features.Cardinality,
                Mutability = analyzed.Features.Mutability,
                Deferred = analyzed.Features.Deferred ? true : null,
            };

            edges.Add(new EdgeWithVia { From = fromId, To = toId, Kind = edgeKind, Via = via });
        }
    }

    private sealed class AnalyzedSymbol
    {
        public required INamedTypeSymbol ReferencedType { get; set; }
        public required SymbolFeatures Features { get; set; }
    }

    private static List<AnalyzedSymbol> AnalyzeType(ITypeSymbol type)
    {
        var results = new List<AnalyzedSymbol>();
        var visited = new HashSet<INamedTypeSymbol>();
        AnalyzeTypeRecursive(type, [], results, visited);
        return results;
    }

    private static void AnalyzeTypeRecursive(
        ITypeSymbol type,
        List<string> path,
        List<AnalyzedSymbol> results,
        HashSet<INamedTypeSymbol> visited)
    {
        // Unwrap deferred types
        var unwrapped = type;
        var isDeferred = false;
        if (type is INamedTypeSymbol namedType && TryUnwrapDeferred(namedType, out var unwrappedDeferred))
        {
            unwrapped = unwrappedDeferred;
            isDeferred = true;
        }

        // Handle collections
        if (TryClassifyCollection(unwrapped, out var cardinality, out var mutability, out var items))
        {
            foreach (var item in items)
            {
                var itemType = item.Type;
                var slot = item.Slot;
                var newPath = new List<string>(path) { slot };

                // If itemType is a named type, add it
                if (itemType is INamedTypeSymbol itemNamed && visited.Add(itemNamed))
                {
                    results.Add(new AnalyzedSymbol
                    {
                        ReferencedType = itemNamed,
                        Features = new SymbolFeatures
                        {
                            Cardinality = cardinality,
                            Mutability = mutability,
                            Deferred = isDeferred,
                            Path = newPath,
                        }
                    });
                }

                // Recursively analyze
                AnalyzeTypeRecursive(itemType, newPath, results, visited);
            }
            return;
        }

        // Handle Nullable<T>
        if (TryUnwrapNullable(unwrapped, out var innerType))
        {
            if (innerType is INamedTypeSymbol innerNamed && visited.Add(innerNamed))
            {
                results.Add(new AnalyzedSymbol
                {
                    ReferencedType = innerNamed,
                    Features = new SymbolFeatures
                    {
                        Cardinality = "optional",
                        Deferred = isDeferred,
                        Path = new List<string>(path),
                    }
                });
            }
            AnalyzeTypeRecursive(innerType, path, results, visited);
            return;
        }

        // Plain type
        if (unwrapped is INamedTypeSymbol named && visited.Add(named))
        {
            results.Add(new AnalyzedSymbol
            {
                ReferencedType = named,
                Features = new SymbolFeatures
                {
                    Cardinality = "one",
                    Deferred = isDeferred,
                    Path = new List<string>(path),
                }
            });
        }
    }

    private static bool TryUnwrapDeferred(INamedTypeSymbol type, out ITypeSymbol result)
    {
        result = type;
        var name = type.Name;

        // Task<T>, ValueTask<T>, Lazy<T>, Func<T>, IObservable<T>
        if ((name == "Task" || name == "ValueTask" || name == "Lazy" || name == "Func" || name == "IObservable")
            && type.TypeArguments.Length > 0)
        {
            result = type.TypeArguments[^1];
            return true;
        }

        // Task without result
        if (name == "Task" && type.TypeArguments.Length == 0)
        {
            return true;
        }

        return false;
    }

    private sealed class CollectionItem
    {
        public required ITypeSymbol Type { get; set; }
        public required string Slot { get; set; }
    }

    private static bool TryClassifyCollection(ITypeSymbol type, out string? cardinality, out string? mutability, out List<CollectionItem> items)
    {
        cardinality = null;
        mutability = null;
        items = [];

        if (type is not INamedTypeSymbol named) return false;

        var name = named.Name;
        var tyargCount = named.TypeArguments.Length;

        // Collections with single type parameter
        if (tyargCount == 1)
        {
            var itemType = named.TypeArguments[0];

            switch (name)
            {
                case "List":
                    cardinality = "many";
                    mutability = "mutable";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "HashSet":
                case "SortedSet":
                    cardinality = "many";
                    mutability = "mutable";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "ICollection":
                case "IList":
                    cardinality = "many";
                    mutability = "mutable";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "IReadOnlyList":
                case "IReadOnlyCollection":
                case "IEnumerable":
                    cardinality = "many";
                    mutability = "readonly";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "ImmutableArray":
                case "ImmutableList":
                    cardinality = "many";
                    mutability = "readonly";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "Span":
                    cardinality = "many";
                    mutability = "readonly";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;

                case "ReadOnlySpan":
                    cardinality = "many";
                    mutability = "readonly";
                    items.Add(new CollectionItem { Type = itemType, Slot = "item" });
                    return true;
            }
        }

        // T[] array
        if (type is IArrayTypeSymbol array)
        {
            cardinality = "many";
            mutability = "mutable";
            items.Add(new CollectionItem { Type = array.ElementType, Slot = "item" });
            return true;
        }

        // Dictionary with two type parameters
        if (tyargCount == 2)
        {
            var keyType = named.TypeArguments[0];
            var valueType = named.TypeArguments[1];

            switch (name)
            {
                case "Dictionary":
                case "SortedDictionary":
                case "SortedList":
                case "IDictionary":
                case "ConcurrentDictionary":
                    cardinality = "keyed";
                    mutability = "mutable";
                    items.Add(new CollectionItem { Type = keyType, Slot = "key" });
                    items.Add(new CollectionItem { Type = valueType, Slot = "value" });
                    return true;

                case "IReadOnlyDictionary":
                case "ImmutableDictionary":
                    cardinality = "keyed";
                    mutability = "readonly";
                    items.Add(new CollectionItem { Type = keyType, Slot = "key" });
                    items.Add(new CollectionItem { Type = valueType, Slot = "value" });
                    return true;
            }
        }

        return false;
    }

    private static bool TryUnwrapNullable(ITypeSymbol type, out ITypeSymbol result)
    {
        result = null!;

        if (type is not INamedTypeSymbol named) return false;

        if (named.OriginalDefinition.SpecialType == SpecialType.System_Nullable_T && named.TypeArguments.Length == 1)
        {
            result = named.TypeArguments[0];
            return true;
        }

        return false;
    }

    private static string GetMemberKind(ISymbol member) => member switch
    {
        IFieldSymbol => "field",
        IPropertySymbol => "property",
        IEventSymbol => "event",
        _ => "unknown",
    };

    private static List<string> ExtractModifiers(ISymbol member)
    {
        var modifiers = new List<string>();

        // Accessibility
        switch (member.DeclaredAccessibility)
        {
            case Accessibility.Public:
                modifiers.Add("public");
                break;
            case Accessibility.Internal:
                modifiers.Add("internal");
                break;
            case Accessibility.Protected:
                modifiers.Add("protected");
                break;
            case Accessibility.Private:
                modifiers.Add("private");
                break;
        }

        // Other modifiers
        if (member is IFieldSymbol field)
        {
            if (field.IsReadOnly) modifiers.Add("readonly");
            if (field.IsStatic) modifiers.Add("static");
        }
        else if (member is IPropertySymbol prop)
        {
            if (prop.IsReadOnly) modifiers.Add("readonly");
            if (prop.IsStatic) modifiers.Add("static");
        }

        return modifiers;
    }
}

internal sealed class EdgeWithVia
{
    public required string From { get; set; }
    public required string To { get; set; }
    public required string Kind { get; set; }
    public ViaFact? Via { get; set; }

    public override bool Equals(object? obj)
    {
        if (obj is not EdgeWithVia other) return false;
        return From == other.From && To == other.To && Kind == other.Kind
            && ViaEquals(Via, other.Via);
    }

    public override int GetHashCode()
    {
        return HashCode.Combine(From, To, Kind, ViaHashCode(Via));
    }

    private static bool ViaEquals(ViaFact? a, ViaFact? b)
    {
        if (a is null && b is null) return true;
        if (a is null || b is null) return false;

        return a.Member == b.Member && a.MemberKind == b.MemberKind
            && ListEquals(a.Modifiers, b.Modifiers)
            && a.Text == b.Text
            && ListEquals(a.Path, b.Path)
            && a.Cardinality == b.Cardinality
            && a.Mutability == b.Mutability
            && a.Deferred == b.Deferred;
    }

    private static int ViaHashCode(ViaFact? via)
    {
        if (via is null) return 0;
        return HashCode.Combine(
            via.Member, via.MemberKind, ListHashCode(via.Modifiers),
            via.Text, ListHashCode(via.Path), via.Cardinality, via.Mutability, via.Deferred);
    }

    private static bool ListEquals<T>(List<T>? a, List<T>? b)
    {
        if (a is null && b is null) return true;
        if (a is null || b is null) return false;
        return a.SequenceEqual(b);
    }

    private static int ListHashCode<T>(List<T>? list)
    {
        if (list is null) return 0;
        var hash = new HashCode();
        foreach (var item in list)
        {
            hash.Add(item);
        }
        return hash.ToHashCode();
    }
}
