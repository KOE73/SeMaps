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

    /// <summary>
    /// Id of a project/assembly symbol per ADR_20260923-9: the assembly name in square brackets,
    /// as IL writes an assembly reference (<c>[Sample.Core]</c>). "[" cannot occur in a C#
    /// namespace or type metadata name, so it never collides with <see cref="NamespaceId"/> or
    /// <see cref="TypeId"/> even when the assembly and its root namespace share a name.
    /// </summary>
    public static string AssemblyId(string assemblyName) => "[" + assemblyName + "]";

    /// <summary>
    /// Id of a method-like symbol per ADR_20260928-4 §2:
    /// <c>&lt;id типа&gt;.&lt;имя&gt;(&lt;типы параметров&gt;)</c>. A constructor's name is
    /// <c>.ctor</c>, a static constructor's is <c>.cctor</c>; a generic method's arity follows
    /// the name as <c>`N</c>. Always used for a method-kind symbol other than a non-indexer
    /// property (see <see cref="PropertyId"/>): ordinary methods, constructors, operators,
    /// conversions, and indexers (docs/extractors/csharp.md: an indexer keeps its parameter
    /// list — indexers can be overloaded — under the name <c>this</c>).
    /// </summary>
    public static string MethodId(IMethodSymbol method)
    {
        var name = method.MethodKind switch
        {
            MethodKind.Constructor => ".ctor",
            MethodKind.StaticConstructor => ".cctor",
            _ => method.Arity > 0 ? $"{MethodShortName(method)}`{method.Arity}" : MethodShortName(method),
        };
        return $"{TypeId(method.ContainingType)}.{name}({ParameterTypeList(method.Parameters)})";
    }

    /// <summary>
    /// A method's own short name: for an ordinary method, <c>Name</c>; for an explicit
    /// interface implementation (<c>double IShape.Area()</c>), <c>Name</c> is the fully
    /// qualified <c>N.IShape.Area</c> as written — this returns the interface member's
    /// simple name instead (<c>Area</c>), so the id and `name` field read like any other
    /// method. Two explicit implementations of same-named members of different interfaces
    /// on the same type are the one case this can still collide on; not seen in practice
    /// (docs/extractors/csharp.md).
    /// </summary>
    private static string MethodShortName(IMethodSymbol method) =>
        method.ExplicitInterfaceImplementations.Length > 0
            ? method.ExplicitInterfaceImplementations[0].Name
            : method.Name;

    /// <summary>Id of an indexer: same shape as <see cref="MethodId"/>, name <c>this</c>.</summary>
    public static string IndexerId(IPropertySymbol indexer) =>
        $"{TypeId(indexer.ContainingType)}.this({ParameterTypeList(indexer.Parameters)})";

    /// <summary>Id of a non-indexer property: no parameter list (ADR_20260928-4 §2).</summary>
    public static string PropertyId(IPropertySymbol property) =>
        $"{TypeId(property.ContainingType)}.{property.Name}";

    private static string ParameterTypeList(System.Collections.Immutable.ImmutableArray<IParameterSymbol> parameters) =>
        string.Join(",", parameters.Select(p => SymbolDisplayHelpers.TypeToDisplayString(p.Type)));
}
