using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Builds the "id" of a symbol per ADR_20260923-5_extractors_symbol-ids.md: metadata name
/// with namespace, "+" for nested types, backtick arity.
/// </summary>
internal static class SymbolIds
{
    public static string NamespaceDottedName(INamespaceSymbol? ns)
    {
        if (ns is null || ns.IsGlobalNamespace)
        {
            return "";
        }

        var parts = new List<string>();
        var current = ns;
        while (current is not null && !current.IsGlobalNamespace)
        {
            parts.Add(current.Name);
            current = current.ContainingNamespace;
        }

        parts.Reverse();
        return string.Join(".", parts);
    }

    public static string TypeId(INamedTypeSymbol type)
    {
        var typeParts = new List<string>();
        INamedTypeSymbol? current = type;
        while (current is not null)
        {
            typeParts.Add(current.MetadataName);
            current = current.ContainingType;
        }

        typeParts.Reverse();
        var typePath = string.Join("+", typeParts);

        var ns = NamespaceDottedName(type.ContainingNamespace);
        return ns.Length == 0 ? typePath : ns + "." + typePath;
    }

    public static string NamespaceId(INamespaceSymbol ns) => NamespaceDottedName(ns);
}
