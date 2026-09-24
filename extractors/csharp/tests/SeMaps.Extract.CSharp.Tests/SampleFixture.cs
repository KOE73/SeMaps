using System.Diagnostics;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

/// <summary>
/// Restores testdata/Sample once before the extractor runs against it: an SDK-style project
/// needs its obj/*.nuget.g.* files even with zero PackageReferences.
/// </summary>
public sealed class SampleFixture : IAsyncLifetime
{
    public async Task InitializeAsync()
    {
        var psi = new ProcessStartInfo("dotnet")
        {
            WorkingDirectory = TestPaths.SampleRoot,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
        };
        psi.ArgumentList.Add("restore");
        psi.ArgumentList.Add("Sample.slnx");
        // A reused MSBuild node outlives restore and inherits the redirected stdout, so
        // ReadToEndAsync below would never see EOF.
        psi.ArgumentList.Add("-nodeReuse:false");
        psi.Environment["MSBUILDDISABLENODEREUSE"] = "1";

        using var process = Process.Start(psi)!;
        var stdOut = await process.StandardOutput.ReadToEndAsync();
        var stdErr = await process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();

        if (process.ExitCode != 0)
        {
            throw new InvalidOperationException($"dotnet restore failed ({process.ExitCode}):\n{stdOut}\n{stdErr}");
        }
    }

    public Task DisposeAsync() => Task.CompletedTask;
}

[CollectionDefinition("Sample")]
public sealed class SampleCollection : ICollectionFixture<SampleFixture>;
