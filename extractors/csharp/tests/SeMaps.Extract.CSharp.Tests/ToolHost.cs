using System.Diagnostics;

namespace SeMaps.Extract.CSharp.Tests;

/// <summary>Runs the built semaps-extract-csharp tool as a subprocess and captures its output.</summary>
internal static class ToolHost
{
    public sealed record Result(int ExitCode, string StdOut, string StdErr);

    private static readonly Lazy<string> ToolDllPath = new(FindToolDll);

    public static Result Run(string workingDirectory, params string[] args)
    {
        var psi = new ProcessStartInfo("dotnet")
        {
            WorkingDirectory = workingDirectory,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
        };
        psi.ArgumentList.Add(ToolDllPath.Value);
        foreach (var arg in args)
        {
            psi.ArgumentList.Add(arg);
        }

        using var process = Process.Start(psi) ?? throw new InvalidOperationException("failed to start tool process");

        // Read fully before waiting for exit to avoid deadlock on large output.
        var stdOutTask = process.StandardOutput.ReadToEndAsync();
        var stdErrTask = process.StandardError.ReadToEndAsync();
        process.WaitForExit();

        return new Result(process.ExitCode, stdOutTask.GetAwaiter().GetResult(), stdErrTask.GetAwaiter().GetResult());
    }

    /// <summary>Finds extractors/csharp by walking up from the test assembly's own output directory.</summary>
    public static string ExtractorsCSharpRoot()
    {
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir is not null &&
               !(Directory.Exists(Path.Combine(dir.FullName, "src")) &&
                 Directory.Exists(Path.Combine(dir.FullName, "tests")) &&
                 Directory.Exists(Path.Combine(dir.FullName, "testdata"))))
        {
            dir = dir.Parent;
        }

        return dir?.FullName ?? throw new InvalidOperationException("could not locate extractors/csharp root from " + AppContext.BaseDirectory);
    }

    private static string FindToolDll()
    {
        var root = ExtractorsCSharpRoot();
        var srcBin = Path.Combine(root, "src", "SeMaps.Extract.CSharp", "bin");
        var dll = Directory.EnumerateFiles(srcBin, "semaps-extract-csharp.dll", SearchOption.AllDirectories)
            .OrderByDescending(File.GetLastWriteTimeUtc)
            .FirstOrDefault();

        return dll ?? throw new InvalidOperationException(
            $"semaps-extract-csharp.dll not found under {srcBin}; build the tool project first");
    }
}
