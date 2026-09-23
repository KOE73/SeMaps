using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Collects `references` edges: field/property types (any visibility) and params/return of
/// methods and constructors (any visibility, ADR-6), plus a delegate's own Invoke signature,
/// generic type arguments, array element types, Nullable&lt;T&gt; → T. Only to symbols present
/// in this output; A→A is never printed. See PLAN "Рёбра" / EXTRACTOR.md §2.2.
/// </summary>
internal static class ReferenceEdgeCollector
{
    public static void Collect(
        string fromId,
        INamedTypeSymbol symbol,
        HashSet<string> outputIds,
        HashSet<(string From, string To, string Kind)> edges)
    {
        foreach (var member in symbol.GetMembers())
        {
            if (member.IsImplicitlyDeclared) continue;

            switch (member)
            {
                case IFieldSymbol field:
                    AddFor(fromId, field.Type, outputIds, edges);
                    break;

                case IPropertySymbol { IsIndexer: false } property:
                    AddFor(fromId, property.Type, outputIds, edges);
                    break;

                case IEventSymbol @event when symbol.TypeKind == TypeKind.Interface:
                    AddFor(fromId, @event.Type, outputIds, edges);
                    break;

                case IMethodSymbol method
                    when IsScannedMethod(method, symbol):
                    AddFor(fromId, method.ReturnType, outputIds, edges);
                    foreach (var parameter in method.Parameters)
                    {
                        AddFor(fromId, parameter.Type, outputIds, edges);
                    }
                    break;
            }
        }

        if (symbol.TypeKind == TypeKind.Delegate && symbol.DelegateInvokeMethod is { } invoke)
        {
            AddFor(fromId, invoke.ReturnType, outputIds, edges);
            foreach (var parameter in invoke.Parameters)
            {
                AddFor(fromId, parameter.Type, outputIds, edges);
            }
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

    private static void AddFor(
        string fromId,
        ITypeSymbol type,
        HashSet<string> outputIds,
        HashSet<(string From, string To, string Kind)> edges)
    {
        foreach (var named in SymbolDisplayHelpers.ReferencedNamedTypes(type))
        {
            var toId = SymbolIds.TypeId(named);
            if (toId == fromId) continue;
            if (!outputIds.Contains(toId)) continue;

            edges.Add((fromId, toId, "references"));
        }
    }
}
