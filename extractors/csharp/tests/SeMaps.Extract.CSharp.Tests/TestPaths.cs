namespace SeMaps.Extract.CSharp.Tests;

internal static class TestPaths
{
    public static string SampleRoot => Path.Combine(ToolHost.ExtractorsCSharpRoot(), "testdata", "Sample");

    public static string ExpectedJsonPath => Path.Combine(SampleRoot, "expected.json");

    public static string SchemaPath => Path.Combine(RepoRoot(), "schemas", "extractor-facts.schema.json");

    private static string RepoRoot() =>
        Path.GetFullPath(Path.Combine(ToolHost.ExtractorsCSharpRoot(), "..", ".."));
}
