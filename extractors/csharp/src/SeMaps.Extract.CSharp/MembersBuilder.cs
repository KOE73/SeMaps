using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Builds the `members` array of a symbol per PLAN_20260923_extractors_csharp.md "Символы":
/// enum values, interface members, and public/internal fields/properties of class-like types.
/// Methods of classes/structs/records are never printed (EXTRACTOR.md §6.2 scope limit).
/// </summary>
internal static class MembersBuilder
{
    public static List<MemberFact>? Build(INamedTypeSymbol symbol, string kind)
    {
        return symbol.TypeKind switch
        {
            TypeKind.Enum => BuildEnumMembers(symbol),
            TypeKind.Interface => BuildInterfaceMembers(symbol),
            TypeKind.Class or TypeKind.Struct => BuildDataMembers(symbol),
            _ => null, // delegate: no members concept in the contract
        };
    }

    private static List<MemberFact> BuildEnumMembers(INamedTypeSymbol symbol)
    {
        var result = new List<MemberFact>();
        foreach (var member in symbol.GetMembers())
        {
            if (member is not IFieldSymbol field) continue;
            if (!SymbolEqualityComparer.Default.Equals(field.Type, symbol)) continue;
            if (!field.HasConstantValue) continue;

            result.Add(new MemberFact
            {
                Kind = "value",
                Name = field.Name,
                Type = Convert.ToString(field.ConstantValue, System.Globalization.CultureInfo.InvariantCulture),
            });
        }

        return result;
    }

    private static List<MemberFact> BuildInterfaceMembers(INamedTypeSymbol symbol)
    {
        var result = new List<MemberFact>();
        foreach (var member in symbol.GetMembers())
        {
            if (member.IsImplicitlyDeclared) continue;

            switch (member)
            {
                case IPropertySymbol property when !property.IsIndexer:
                    result.Add(new MemberFact
                    {
                        Kind = "property",
                        Name = property.Name,
                        Type = SymbolDisplayHelpers.TypeToDisplayString(property.Type),
                        Visibility = "public",
                    });
                    break;

                case IEventSymbol @event:
                    result.Add(new MemberFact
                    {
                        Kind = "event",
                        Name = @event.Name,
                        Type = SymbolDisplayHelpers.TypeToDisplayString(@event.Type),
                        Visibility = "public",
                    });
                    break;

                case IMethodSymbol { MethodKind: MethodKind.Ordinary } method:
                    result.Add(new MemberFact
                    {
                        Kind = "method",
                        Name = method.Name,
                        Type = SymbolDisplayHelpers.MethodSignature(method),
                        Visibility = "public",
                    });
                    break;
            }
        }

        return result;
    }

    private static List<MemberFact> BuildDataMembers(INamedTypeSymbol symbol)
    {
        var result = new List<MemberFact>();
        foreach (var member in symbol.GetMembers())
        {
            if (member.IsImplicitlyDeclared) continue;
            if (member.DeclaredAccessibility is not (Accessibility.Public or Accessibility.Internal)) continue;

            switch (member)
            {
                case IFieldSymbol field:
                    result.Add(new MemberFact
                    {
                        Kind = "field",
                        Name = field.Name,
                        Type = SymbolDisplayHelpers.TypeToDisplayString(field.Type),
                        Visibility = SymbolDisplayHelpers.Visibility(field.DeclaredAccessibility),
                    });
                    break;

                case IPropertySymbol { IsIndexer: false } property:
                    result.Add(new MemberFact
                    {
                        Kind = "property",
                        Name = property.Name,
                        Type = SymbolDisplayHelpers.TypeToDisplayString(property.Type),
                        Visibility = SymbolDisplayHelpers.Visibility(property.DeclaredAccessibility),
                    });
                    break;
            }
        }

        return result;
    }
}
