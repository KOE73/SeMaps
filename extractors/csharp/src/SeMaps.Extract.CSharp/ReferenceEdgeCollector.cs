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
        var methodName = method.Name;

        // Return type: uses with method name as member
        AddForMethodParameter(fromId, method.ReturnType, methodName, "return", method, outputIds, edges, requestedEdgeKinds);

        // Parameters: uses with parameter name as member
        foreach (var param in method.Parameters)
        {
            var memberKindStr = method.MethodKind == MethodKind.Constructor ? "constructor" : "parameter";
            AddForMethodParameter(fromId, param.Type, param.Name, memberKindStr, method, outputIds, edges, requestedEdgeKinds);
        }
    }

    private static void AddForMethodParameter(
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
        var visited = new HashSet<INamedTypeSymbol>(SymbolEqualityComparer.Default);
        AnalyzeTypeRecursive(type, [], results, visited);
        return results;
    }

    private static void AnalyzeTypeRecursive(
        ITypeSymbol type,
        List<string> path,
        List<AnalyzedSymbol> results,
        HashSet<INamedTypeSymbol> visited)
    {
        // Handle arrays: T[] -> item path
        if (type is IArrayTypeSymbol array)
        {
            var itemPath = new List<string>(path) { "item" };
            AnalyzeTypeRecursive(array.ElementType, itemPath, results, visited);
            return;
        }

        if (type is not INamedTypeSymbol named)
        {
            return;
        }

        var name = named.Name;

        // Unwrap deferred types: Task<T>, ValueTask<T>, Lazy<T>, Func<T>, IObservable<T>
        // These have path ["result"] and deferred=true
        if ((name == "Task" || name == "ValueTask" || name == "Lazy" || name == "Func" || name == "IObservable")
            && named.TypeArguments.Length > 0)
        {
            var resultType = named.TypeArguments[^1];  // Last type arg is result
            var resultPath = new List<string>(path) { "result" };
            var features = new SymbolFeatures
            {
                Deferred = true,
                Path = resultPath,
            };

            // Add the result type with features
            if (resultType is INamedTypeSymbol resultNamed && visited.Add(resultNamed))
            {
                features.Cardinality = GetCardinality(resultNamed);
                results.Add(new AnalyzedSymbol { ReferencedType = resultNamed, Features = features });
            }

            // Recurse into result type to find nested symbols
            AnalyzeTypeRecursive(resultType, resultPath, results, visited);
            return;
        }

        // Handle Task without result type
        if (name == "Task" && named.TypeArguments.Length == 0)
        {
            // Task with no result type - no symbol dependency
            return;
        }

        // Handle Nullable<T>
        if (named.OriginalDefinition.SpecialType == SpecialType.System_Nullable_T && named.TypeArguments.Length == 1)
        {
            var innerType = named.TypeArguments[0];
            var features = new SymbolFeatures { Cardinality = "optional", Path = new List<string>(path) };

            if (innerType is INamedTypeSymbol innerNamed && visited.Add(innerNamed))
            {
                results.Add(new AnalyzedSymbol { ReferencedType = innerNamed, Features = features });
            }
            AnalyzeTypeRecursive(innerType, path, results, visited);
            return;
        }

        // Handle collections: List<T>, IReadOnlyList<T>, Dictionary<K,V>, etc.
        if (TryClassifyCollection(named, out var cardinality, out var mutability, out var items))
        {
            foreach (var item in items)
            {
                var itemType = item.Type;
                var slot = item.Slot;
                var slotPath = new List<string>(path) { slot };

                if (itemType is INamedTypeSymbol itemNamed && visited.Add(itemNamed))
                {
                    var features = new SymbolFeatures
                    {
                        Cardinality = cardinality,
                        Mutability = mutability,
                        Path = slotPath,
                    };
                    results.Add(new AnalyzedSymbol { ReferencedType = itemNamed, Features = features });
                }

                // Recurse to handle generic type arguments
                AnalyzeTypeRecursive(itemType, slotPath, results, visited);
            }
            return;
        }

        // Handle generic type arguments for symbol that is in output
        if (named.TypeArguments.Length > 0)
        {
            // Add the symbol itself if in output
            if (visited.Add(named))
            {
                var features = new SymbolFeatures { Path = new List<string>(path) };
                results.Add(new AnalyzedSymbol { ReferencedType = named, Features = features });
            }

            // Add generic type arguments
            for (int i = 0; i < named.TypeArguments.Length; i++)
            {
                var argType = named.TypeArguments[i];
                var argPath = new List<string>(path) { $"arg:{i}" };

                if (argType is INamedTypeSymbol argNamed)
                {
                    if (visited.Add(argNamed))
                    {
                        // If argument is in output, add it
                        var features = new SymbolFeatures { Path = argPath };
                        results.Add(new AnalyzedSymbol { ReferencedType = argNamed, Features = features });
                    }
                    else
                    {
                        // Already visited - add with arg path
                        var features = new SymbolFeatures { Path = argPath };
                        results.Add(new AnalyzedSymbol { ReferencedType = argNamed, Features = features });
                    }
                }

                // Recurse to find nested symbols
                AnalyzeTypeRecursive(argType, argPath, results, visited);
            }
            return;
        }

        // Plain type reference
        if (visited.Add(named))
        {
            var features = new SymbolFeatures { Path = new List<string>(path) };
            results.Add(new AnalyzedSymbol { ReferencedType = named, Features = features });
        }
    }

    private static string? GetCardinality(INamedTypeSymbol type)
    {
        // For types inside deferred wrappers, return appropriate cardinality
        // This is mainly "one" for simple types
        return "one";
    }

    private sealed class CollectionItem
    {
        public required ITypeSymbol Type { get; set; }
        public required string Slot { get; set; }
    }

    private static bool TryClassifyCollection(INamedTypeSymbol type, out string? cardinality, out string? mutability, out List<CollectionItem> items)
    {
        cardinality = null;
        mutability = null;
        items = [];

        var name = type.Name;
        var tyargCount = type.TypeArguments.Length;

        // Collections with single type parameter
        if (tyargCount == 1)
        {
            var itemType = type.TypeArguments[0];

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

        // Dictionary with two type parameters
        if (tyargCount == 2)
        {
            var keyType = type.TypeArguments[0];
            var valueType = type.TypeArguments[1];

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
