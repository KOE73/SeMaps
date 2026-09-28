using Json.Schema;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

/// <summary>
/// The overrides/implements/calls/constructs scenarios of ADR_20260928-4 §1, §3: an override
/// chain, an interface with two implementers plus an explicit implementation, a call through
/// an interface, and a static factory that constructs and returns (docs/plans/PLAN_20260928-6).
/// </summary>
[Collection("CallsSample")]
public sealed class CallsExtractorTests
{
    [Fact]
    public void Output_MatchesGoldenFile()
    {
        var result = ToolHost.Run(TestPaths.CallsSampleRoot, "--root", ".", "--edges", "calls");

        Assert.Equal(0, result.ExitCode);
        var expected = File.ReadAllText(TestPaths.CallsSampleExpectedPath);
        Assert.Equal(expected, result.StdOut);
    }

    [Fact]
    public void Output_IsValidPerSchema()
    {
        var result = ToolHost.Run(TestPaths.CallsSampleRoot, "--root", ".", "--edges", "calls");
        Assert.Equal(0, result.ExitCode);

        var schema = JsonSchema.FromFile(TestPaths.SchemaPath);
        var instance = System.Text.Json.Nodes.JsonNode.Parse(result.StdOut);
        var evaluation = schema.Evaluate(instance, new EvaluationOptions { OutputFormat = OutputFormat.List });

        Assert.True(evaluation.IsValid, DescribeErrors(evaluation));
    }

    [Fact]
    public void Output_IsDeterministicAcrossRuns()
    {
        var first = ToolHost.Run(TestPaths.CallsSampleRoot, "--root", ".", "--edges", "calls");
        var second = ToolHost.Run(TestPaths.CallsSampleRoot, "--root", ".", "--edges", "calls");

        Assert.Equal(0, first.ExitCode);
        Assert.Equal(0, second.ExitCode);
        Assert.Equal(first.StdOut, second.StdOut);
    }

    private static string DescribeErrors(EvaluationResults evaluation)
    {
        var details = evaluation.Details
            .Where(d => !d.IsValid && d.HasErrors)
            .SelectMany(d => d.Errors!.Select(e => $"{d.InstanceLocation}: {e.Key} - {e.Value}"));
        return string.Join("\n", details);
    }
}
