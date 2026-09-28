using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// `calls` and `constructs` edges (ADR_20260928-4 §1, §3), resolved through the semantic
/// model — never by text. Invocations; property/indexer reads and writes; object creation
/// (`constructs` to the type, plus `calls` to the constructor when it is a symbol of this
/// output); constructor initialisers (`base(...)`/`this(...)`); user-defined operators and
/// conversions; a method group converted to a delegate. A lambda or local function's calls
/// are attributed to the enclosing method/accessor, since neither is a symbol of its own
/// (EXTRACTOR.md §2.3 "Лямбды и локальные функции символами не становятся") — this falls out
/// for free by walking the whole body without treating either as a boundary.
///
/// The target is always the *original definition*: a generic method or a method of a generic
/// type resolves to its unbound form, and a call through an interface or a virtual method
/// resolves to the method as written at the call site (the interface's or the base's), because
/// that is exactly what <see cref="SemanticModel.GetSymbolInfo(SyntaxNode, System.Threading.CancellationToken)"/>
/// returns for such a call — no further override resolution is done here.
///
/// One edge per (from, to, kind); every call site's line is collected and sorted, `line`
/// (EdgeFact) is the first of them.
/// </summary>
internal sealed class CallsCollector
{
    private readonly Dictionary<(string From, string To, string Kind), SortedSet<int>> _lines = new();

    /// <summary>
    /// Call sites whose target resolved to a method-like symbol (or a type, for `constructs`)
    /// that is not printed in this output — outside `--include`, in a referenced assembly, or
    /// (for `calls`) a lambda/local function, which is never a symbol. Reported in the
    /// extractor's stderr summary, not in the facts themselves.
    /// </summary>
    public int OutsideOutput { get; private set; }

    public IEnumerable<EdgeWithVia> BuildEdges()
    {
        foreach (var ((from, to, kind), lines) in _lines)
        {
            var sorted = lines.ToList();
            yield return new EdgeWithVia
            {
                From = from,
                To = to,
                Kind = kind,
                Line = sorted[0],
                Lines = sorted.Count > 1 ? sorted : null,
            };
        }
    }

    /// <summary>Walks one declaration's body (a method, accessor, or constructor initialiser).</summary>
    public void Collect(
        string fromId,
        SemanticModel model,
        SyntaxNode root,
        HashSet<string> outputTypeIds,
        Dictionary<string, ISymbol> methodSymbols)
    {
        foreach (var node in root.DescendantNodesAndSelf())
        {
            switch (node)
            {
                case InvocationExpressionSyntax invocation:
                    HandleInvocation(fromId, model, invocation, methodSymbols);
                    break;

                case ObjectCreationExpressionSyntax or ImplicitObjectCreationExpressionSyntax:
                    HandleObjectCreation(fromId, model, node, outputTypeIds, methodSymbols);
                    break;

                case ConstructorInitializerSyntax initializer:
                    HandleTarget(fromId, model.GetSymbolInfo(initializer).Symbol, initializer.GetLocation(), methodSymbols);
                    break;

                case BinaryExpressionSyntax or PrefixUnaryExpressionSyntax or PostfixUnaryExpressionSyntax or CastExpressionSyntax:
                    HandleOperator(fromId, model, node, methodSymbols);
                    break;

                case MemberAccessExpressionSyntax or ElementAccessExpressionSyntax or IdentifierNameSyntax:
                    if (IsInvocationCallee(node))
                    {
                        break; // handled once, as the whole invocation, above
                    }

                    HandleTarget(fromId, model.GetSymbolInfo(node).Symbol, node.GetLocation(), methodSymbols);
                    break;
            }
        }
    }

    private static bool IsInvocationCallee(SyntaxNode node) =>
        node.Parent is InvocationExpressionSyntax invocation && invocation.Expression == node;

    private void HandleInvocation(string fromId, SemanticModel model, InvocationExpressionSyntax node, Dictionary<string, ISymbol> methodSymbols)
    {
        if (model.GetSymbolInfo(node).Symbol is not IMethodSymbol target)
        {
            return; // unresolved (ambiguous overload, dynamic, …): no single target to report
        }

        // A delegate variable's own Invoke, a local function, or a lambda are not symbols
        // of this output at all — not even candidates for "outside output" (EXTRACTOR.md §2.3).
        if (target.MethodKind is MethodKind.DelegateInvoke or MethodKind.LocalFunction or MethodKind.AnonymousFunction)
        {
            return;
        }

        Record(fromId, "calls", target.OriginalDefinition, node.GetLocation(), methodSymbols);
    }

    private void HandleObjectCreation(
        string fromId,
        SemanticModel model,
        SyntaxNode node,
        HashSet<string> outputTypeIds,
        Dictionary<string, ISymbol> methodSymbols)
    {
        var ctor = model.GetSymbolInfo(node).Symbol as IMethodSymbol;
        var type = ctor?.ContainingType ??
            (node is ObjectCreationExpressionSyntax oce ? model.GetTypeInfo(oce.Type).Type as INamedTypeSymbol : null);

        if (type is not null)
        {
            var typeId = SymbolIds.TypeId(type);
            if (outputTypeIds.Contains(typeId))
            {
                AddLine(fromId, typeId, "constructs", node.GetLocation());
            }
            else
            {
                OutsideOutput++;
            }
        }

        if (ctor is not null)
        {
            Record(fromId, "calls", ctor.OriginalDefinition, node.GetLocation(), methodSymbols);
        }
    }

    private void HandleOperator(string fromId, SemanticModel model, SyntaxNode node, Dictionary<string, ISymbol> methodSymbols)
    {
        if (model.GetSymbolInfo(node).Symbol is IMethodSymbol { MethodKind: MethodKind.UserDefinedOperator or MethodKind.Conversion } method)
        {
            Record(fromId, "calls", method.OriginalDefinition, node.GetLocation(), methodSymbols);
        }
    }

    /// <summary>
    /// A property/indexer read or write, or a plain method name converted to a delegate
    /// (a "method group"): both arrive here as a symbol referenced somewhere other than an
    /// invocation's callee position.
    /// </summary>
    private void HandleTarget(string fromId, ISymbol? symbol, Location location, Dictionary<string, ISymbol> methodSymbols)
    {
        switch (symbol)
        {
            case IPropertySymbol property:
                Record(fromId, "calls", property.OriginalDefinition, location, methodSymbols);
                break;

            case IMethodSymbol method when FactsExtractor.IsCallsEligibleMethod(method):
                Record(fromId, "calls", method.OriginalDefinition, location, methodSymbols);
                break;
        }
    }

    private void Record(string fromId, string kind, ISymbol targetOriginalDefinition, Location location, Dictionary<string, ISymbol> methodSymbols)
    {
        var toId = targetOriginalDefinition switch
        {
            IMethodSymbol m => SymbolIds.MethodId(m),
            IPropertySymbol { IsIndexer: true } p => SymbolIds.IndexerId(p),
            IPropertySymbol p => SymbolIds.PropertyId(p),
            _ => null,
        };

        if (toId is null || !methodSymbols.ContainsKey(toId))
        {
            OutsideOutput++;
            return;
        }

        AddLine(fromId, toId, kind, location);
    }

    private void AddLine(string fromId, string toId, string kind, Location location)
    {
        if (fromId == toId && kind != "calls")
        {
            return; // constructs/overrides/implements never self-loop; calls (recursion) may
        }

        var key = (fromId, toId, kind);
        if (!_lines.TryGetValue(key, out var set))
        {
            set = [];
            _lines[key] = set;
        }

        set.Add(location.GetLineSpan().StartLinePosition.Line + 1);
    }
}
