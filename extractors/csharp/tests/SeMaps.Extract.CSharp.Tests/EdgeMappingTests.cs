using System.Text.Json.Nodes;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

/// <summary>
/// One row per line of the normative table "Сопоставление рёбер" in docs/extractors/csharp.md
/// (ADR_20260927 §4). A row changed in the table without the code, or the other way round,
/// fails here. The method layer (--edges calls) is covered by <see cref="MethodLayerCarriesNoNative"/>.
/// </summary>
[Collection("Sample")]
public sealed class EdgeMappingTests
{
    public static TheoryData<string, string, string, string, string?> Rows => new()
    {
        // construct: from, to, kind, native
        { "class : Base", "Sample.App.AdvancedRepetitionGuard", "Sample.Core.Guards.RepetitionGuard", "extends", "class" },
        { "interface IA : IB", "Sample.Core.IWriteStore", "Sample.Core.IReadStore", "extends", "interface" },
        { "class : IA", "Sample.App.AdvancedRepetitionGuard", "Sample.Core.IRepository", "implements", "interface" },
        { "namespace -> type", "Sample.Core", "Sample.Core.IReadStore", "contains", "namespace" },
        { "assembly -> type", "[Sample.App]", "Sample.App.AdvancedRepetitionGuard", "contains", "assembly" },
        { "nested type", "Sample.Core.Container", "Sample.Core.Container+Inner", "contains", "nested" },
        { "ProjectReference", "[Sample.App]", "[Sample.Core]", "depends", null },
        { "field", "Sample.Core.Container+Secret", "Sample.Core.Status", "holds", "field" },
        { "property", "Sample.App.AdvancedRepetitionGuard", "Sample.Core.Status", "holds", "property" },
        { "interface event", "Sample.Core.IRepository", "Sample.Core.Notify", "holds", "event" },
        { "method parameter", "Sample.Core.Notify", "Sample.Core.Status", "uses", "parameter" },
        { "return type", "Sample.App.AdvancedRepetitionGuard", "Sample.Core.Repo`1", "uses", "return" },
        { "constructor parameter", "Sample.Core.Features", "Sample.Core.Status", "uses", "constructor" },
    };

    [Theory]
    [MemberData(nameof(Rows))]
    public void Edge_HasKindAndNative(string construct, string from, string to, string kind, string? native)
    {
        var result = ToolHost.Run(TestPaths.SampleRoot, "--root", ".", "--edges", "holds,uses");
        Assert.Equal(0, result.ExitCode);

        var edges = JsonNode.Parse(result.StdOut)!["edges"]!.AsArray()
            .Where(e => (string?)e!["from"] == from && (string?)e["to"] == to && (string?)e["kind"] == kind)
            .ToList();

        Assert.True(edges.Count > 0, $"{construct}: no {kind} edge {from} -> {to}");
        Assert.Contains(edges, e => (string?)e!["native"] == native);
    }

    /// <summary>
    /// Last rows of the table: the method layer writes no `native` — calls, constructs, overrides,
    /// and contains/implements with a method end. The language has one obvious way for each.
    /// </summary>
    [Fact]
    public void MethodLayerCarriesNoNative()
    {
        var result = ToolHost.Run(TestPaths.SampleRoot, "--root", ".", "--edges", "calls");
        Assert.Equal(0, result.ExitCode);

        var root = JsonNode.Parse(result.StdOut)!;
        var methods = root["symbols"]!.AsArray()
            .Where(s => (string?)s!["kind"] == "method")
            .Select(s => (string)s!["id"]!)
            .ToHashSet();
        var layer = root["edges"]!.AsArray()
            .Where(e => methods.Contains((string)e!["from"]!) || methods.Contains((string)e["to"]!))
            .ToList();

        Assert.NotEmpty(layer);
        Assert.All(layer, e => Assert.Null(e!["native"]));
    }
}
