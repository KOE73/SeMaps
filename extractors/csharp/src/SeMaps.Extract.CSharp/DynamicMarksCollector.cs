using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace SeMaps.Extract.CSharp;

/// <summary>
/// Blind-spot marks on methods (ADR_20260928-5 §4, PLAN_20260928-7 step 6): a method that
/// creates or invokes something through the platform's base library reflection, or operates on
/// an expression of the C# type <c>dynamic</c>, gets a mark — kind and line, never a target.
/// Resolved through the semantic model, never by text; a lambda or local function's marks are
/// attributed to the enclosing method, exactly like calls (EXTRACTOR.md §2.3), because walking
/// here never stops at that boundary either.
/// </summary>
internal sealed class DynamicMarksCollector
{
    private readonly Dictionary<string, SortedSet<(int Line, string Kind)>> _marks = new(StringComparer.Ordinal);

    /// <summary>
    /// The closed list of base-library members the extractor knows (ADR_20260928-5 §3): one
    /// place, as data. No other library, and no target, is ever recorded. Kept in the order
    /// docs/extractors/csharp.md lists them.
    /// </summary>
    internal static readonly (string Namespace, string Type, string Method, string Kind)[] KnownCalls =
    [
        ("System", "Activator", "CreateInstance", "create"),
        ("System.Reflection", "ConstructorInfo", "Invoke", "create"),
        ("System.Reflection", "MethodBase", "Invoke", "invoke"),
        ("System", "Delegate", "DynamicInvoke", "invoke"),
        ("System", "Type", "MakeGenericType", "make-type"),
        ("System", "Type", "GetType", "make-type"),
        ("System.Reflection", "Assembly", "GetType", "make-type"),
        ("System.Reflection", "MethodInfo", "MakeGenericMethod", "make-type"),
    ];

    public IReadOnlyDictionary<string, SortedSet<(int Line, string Kind)>> Marks => _marks;

    /// <summary>Walks one declaration's body, alongside <see cref="CallsCollector"/>.</summary>
    public void Collect(string fromId, SemanticModel model, SyntaxNode root)
    {
        foreach (var node in root.DescendantNodesAndSelf())
        {
            switch (node)
            {
                case InvocationExpressionSyntax invocation:
                    CheckKnownCall(fromId, model, invocation);
                    CheckDynamicOperation(fromId, model, invocation);
                    break;

                case MemberAccessExpressionSyntax or ElementAccessExpressionSyntax or BinaryExpressionSyntax:
                    CheckDynamicOperation(fromId, model, node);
                    break;
            }
        }
    }

    private void CheckKnownCall(string fromId, SemanticModel model, InvocationExpressionSyntax invocation)
    {
        if (model.GetSymbolInfo(invocation).Symbol is not IMethodSymbol target)
        {
            return; // unresolved (including a dynamic call: handled separately, below)
        }

        var original = target.OriginalDefinition;
        var type = original.ContainingType;
        if (type is null)
        {
            return;
        }

        var ns = type.ContainingNamespace is { IsGlobalNamespace: false } containing
            ? containing.ToDisplayString()
            : "";

        foreach (var (knownNs, knownType, knownMethod, kind) in KnownCalls)
        {
            if (ns == knownNs && type.Name == knownType && original.Name == knownMethod)
            {
                Add(fromId, kind, invocation.GetLocation());
                return;
            }
        }
    }

    /// <summary>
    /// An operation whose own result is the C# type <c>dynamic</c> is, by construction, an
    /// operation on a dynamic expression: a member access, element access, invocation or binary
    /// operator resolved at run time because one side is <c>dynamic</c>. No text pattern, no
    /// `typeof` nearby — the semantic model reports the type directly (ADR_20260928-5 §1).
    /// </summary>
    private void CheckDynamicOperation(string fromId, SemanticModel model, SyntaxNode node)
    {
        var type = model.GetTypeInfo(node).Type;
        if (type?.TypeKind == TypeKind.Dynamic)
        {
            Add(fromId, "dynamic", node.GetLocation());
        }
    }

    private void Add(string fromId, string kind, Location location)
    {
        if (!_marks.TryGetValue(fromId, out var set))
        {
            set = new SortedSet<(int Line, string Kind)>(MarkComparer.Instance);
            _marks[fromId] = set;
        }

        var line = location.GetLineSpan().StartLinePosition.Line + 1;
        set.Add((line, kind));
    }

    /// <summary>Sorts marks by (line, kind), ordinal — deterministic regardless of locale.</summary>
    private sealed class MarkComparer : IComparer<(int Line, string Kind)>
    {
        public static readonly MarkComparer Instance = new();

        public int Compare((int Line, string Kind) a, (int Line, string Kind) b)
        {
            var byLine = a.Line.CompareTo(b.Line);
            return byLine != 0 ? byLine : string.CompareOrdinal(a.Kind, b.Kind);
        }
    }
}
