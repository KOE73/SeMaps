using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

internal static class SymbolDisplayHelpers
{
    public static string TypeToDisplayString(ITypeSymbol type) =>
        type.ToDisplayString(SymbolDisplayFormat.MinimallyQualifiedFormat);

    public static string Visibility(Accessibility accessibility) => accessibility switch
    {
        Accessibility.Public => "public",
        Accessibility.Internal => "internal",
        Accessibility.Protected => "protected",
        Accessibility.Private => "private",
        Accessibility.ProtectedOrInternal => "protected internal",
        Accessibility.ProtectedAndInternal => "private protected",
        _ => "private",
    };

    public static string MethodSignature(IMethodSymbol method)
    {
        var returnType = method.ReturnsVoid ? "void" : TypeToDisplayString(method.ReturnType);
        var parameters = string.Join(", ", method.Parameters.Select(p => $"{TypeToDisplayString(p.Type)} {p.Name}"));
        return $"{returnType} {method.Name}({parameters})";
    }

    /// <summary>
    /// Unwraps generic type arguments, array element types and Nullable&lt;T&gt; to find every
    /// named type mentioned by <paramref name="type"/>, per PLAN §"Рёбра" / EXTRACTOR.md §2.2.
    /// </summary>
    public static IEnumerable<INamedTypeSymbol> ReferencedNamedTypes(ITypeSymbol? type)
    {
        switch (type)
        {
            case null:
                yield break;

            case INamedTypeSymbol { OriginalDefinition.SpecialType: SpecialType.System_Nullable_T } nullable
                when nullable.TypeArguments.Length == 1:
                foreach (var t in ReferencedNamedTypes(nullable.TypeArguments[0]))
                {
                    yield return t;
                }
                yield break;

            case INamedTypeSymbol named:
                yield return named;
                foreach (var arg in named.TypeArguments)
                {
                    foreach (var t in ReferencedNamedTypes(arg))
                    {
                        yield return t;
                    }
                }
                yield break;

            case IArrayTypeSymbol array:
                foreach (var t in ReferencedNamedTypes(array.ElementType))
                {
                    yield return t;
                }
                yield break;

            default:
                yield break;
        }
    }
}
