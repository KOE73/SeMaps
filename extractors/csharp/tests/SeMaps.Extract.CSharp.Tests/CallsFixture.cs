using System.Diagnostics;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

/// <summary>
/// Restores testdata/CallsSample once before the extractor runs against it. A second,
/// separate fixture project (ADR_20260928-4 methods-and-calls work): its own solution keeps
/// the calls/overrides/implements scenarios out of testdata/Sample, whose golden files must
/// stay byte-identical to before this work (docs/plans/PLAN_20260928-6).
/// </summary>
public sealed class CallsFixture : IAsyncLifetime
{
    public async Task InitializeAsync()
    {
        var psi = new ProcessStartInfo("dotnet")
        {
            WorkingDirectory = TestPaths.CallsSampleRoot,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
        };
        psi.ArgumentList.Add("restore");
        psi.ArgumentList.Add("CallsSample.slnx");
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

[CollectionDefinition("CallsSample")]
public sealed class CallsCollection : ICollectionFixture<CallsFixture>;
