using Json.Schema;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

[Collection("Sample")]
public sealed class ExtractorTests
{
    [Fact]
    public void Output_MatchesGoldenFile()
    {
        var result = ToolHost.Run(TestPaths.SampleRoot, "--root", ".");

        Assert.Equal(0, result.ExitCode);
        var expected = File.ReadAllText(TestPaths.ExpectedJsonPath);
        Assert.Equal(expected, result.StdOut);
    }

    [Fact]
    public void Output_IsValidPerSchema()
    {
        var result = ToolHost.Run(TestPaths.SampleRoot, "--root", ".");
        Assert.Equal(0, result.ExitCode);

        var schema = JsonSchema.FromFile(TestPaths.SchemaPath);
        var instance = System.Text.Json.Nodes.JsonNode.Parse(result.StdOut);
        var evaluation = schema.Evaluate(instance, new EvaluationOptions { OutputFormat = OutputFormat.List });

        Assert.True(evaluation.IsValid, DescribeErrors(evaluation));
    }

    [Fact]
    public void Output_IsDeterministicAcrossRuns()
    {
        var first = ToolHost.Run(TestPaths.SampleRoot, "--root", ".");
        var second = ToolHost.Run(TestPaths.SampleRoot, "--root", ".");

        Assert.Equal(0, first.ExitCode);
        Assert.Equal(0, second.ExitCode);
        Assert.Equal(first.StdOut, second.StdOut);
    }

    [Fact]
    public void StdOut_ContainsOnlyJson()
    {
        var result = ToolHost.Run(TestPaths.SampleRoot, "--root", ".");

        Assert.Equal(0, result.ExitCode);
        // The whole of stdout must parse as one JSON document (no stray diagnostics interleaved).
        System.Text.Json.Nodes.JsonNode.Parse(result.StdOut);
    }

    private static string DescribeErrors(EvaluationResults evaluation)
    {
        var details = evaluation.Details
            .Where(d => !d.IsValid && d.HasErrors)
            .SelectMany(d => d.Errors!.Select(e => $"{d.InstanceLocation}: {e.Key} - {e.Value}"));
        return string.Join("\n", details);
    }
}
