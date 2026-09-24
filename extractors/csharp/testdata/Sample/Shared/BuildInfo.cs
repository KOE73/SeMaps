namespace Sample.Shared;

// Linked into both projects (<Compile Include="..\Shared\BuildInfo.cs" />): one symbol,
// "contains" from each assembly.
internal static class BuildInfo
{
    public const string Product = "Sample";
}
