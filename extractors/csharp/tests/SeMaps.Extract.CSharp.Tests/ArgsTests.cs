using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

public sealed class ArgsTests
{
    [Fact]
    public void NonexistentRoot_ExitsOne_WithEmptyStdOut()
    {
        var missing = Path.Combine(Path.GetTempPath(), "semaps-extract-csharp-does-not-exist-" + Guid.NewGuid());

        var result = ToolHost.Run(Path.GetTempPath(), "--root", missing);

        Assert.Equal(1, result.ExitCode);
        Assert.Equal("", result.StdOut);
        Assert.NotEqual("", result.StdErr);
    }

    [Fact]
    public void UnknownFlag_ExitsTwo_WithEmptyStdOut()
    {
        var result = ToolHost.Run(Path.GetTempPath(), "--bogus-flag");

        Assert.Equal(2, result.ExitCode);
        Assert.Equal("", result.StdOut);
        Assert.NotEqual("", result.StdErr);
    }

    [Fact]
    public void MissingValueForRoot_ExitsTwo()
    {
        var result = ToolHost.Run(Path.GetTempPath(), "--root");

        Assert.Equal(2, result.ExitCode);
        Assert.Equal("", result.StdOut);
    }
}
