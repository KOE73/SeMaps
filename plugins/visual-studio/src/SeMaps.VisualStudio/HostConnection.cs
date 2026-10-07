using System;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;

namespace SeMaps.VisualStudio;

internal sealed class HostConnection
{
    private readonly HttpClient client;
    private readonly Action<string, string> start;
    private readonly Func<string> executable;
    private readonly SemaphoreSlim gate = new SemaphoreSlim(1, 1);
    public HostConnection(HttpClient client, Action<string, string>? start = null, Func<string>? executable = null)
    {
        this.client = client;
        this.start = start ?? StartHost;
        this.executable = executable ?? FindExecutable;
    }

    public async Task<Uri> EnsureAsync(SeMapsProject project, CancellationToken cancellationToken)
    {
        await gate.WaitAsync(cancellationToken);
        try
        {
            var endpoint = await TryReadAsync(project, cancellationToken);
            if (endpoint != null) return endpoint;
            cancellationToken.ThrowIfCancellationRequested();
            start(executable(), project.File);
            var deadline = DateTime.UtcNow.AddSeconds(15);
            do
            {
                cancellationToken.ThrowIfCancellationRequested();
                endpoint = await TryReadAsync(project, cancellationToken);
                if (endpoint != null) return endpoint;
                await Task.Delay(250, cancellationToken);
            } while (DateTime.UtcNow < deadline);
            throw new TimeoutException("SeMaps did not start within 15 seconds. Check the .semaps file and the installed semaps executable.");
        }
        finally { gate.Release(); }
    }

    internal async Task<Uri?> TryReadAsync(SeMapsProject project, CancellationToken cancellationToken)
    {
        try
        {
            var stateFile = Path.Combine(project.Root, ".semaps", "host.json");
            if (!File.Exists(stateFile)) return null;
            var state = JObject.Parse(File.ReadAllText(stateFile));
            var port = (int?)state["port"] ?? 0;
            if (port < 1 || port > 65535) return null;
            var baseUri = new Uri("http://localhost:" + port + "/");
            using (var requestTimeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken))
            {
                requestTimeout.CancelAfter(TimeSpan.FromSeconds(1));
                using (var response = await client.GetAsync(new Uri(baseUri, "api/info"), requestTimeout.Token))
                {
                    if (!response.IsSuccessStatusCode) return null;
                    var info = JObject.Parse(await response.Content.ReadAsStringAsync());
                    if (!SamePath((string?)info["workspace"], project.Workspace)
                        || !SamePath((string?)info["sourceRoot"], project.SourceRoot)) return null;
                    return new Uri(baseUri, "app/");
                }
            }
        }
        catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested) { return null; }
        catch (IOException) { return null; }
        catch (Newtonsoft.Json.JsonException) { return null; }
        catch (HttpRequestException) { return null; }
        catch (ArgumentException) { return null; }
        catch (NotSupportedException) { return null; }
    }

    private static bool SamePath(string? actual, string expected) => !string.IsNullOrWhiteSpace(actual)
        && string.Equals(Path.GetFullPath(actual).TrimEnd(Path.DirectorySeparatorChar),
            expected.TrimEnd(Path.DirectorySeparatorChar), StringComparison.OrdinalIgnoreCase);

    private static string FindExecutable()
    {
        // Per-process development override; normal installations use the stable executable.
        var trial = Environment.GetEnvironmentVariable("SEMAPS_VS_HOST_EXE");
        if (!string.IsNullOrWhiteSpace(trial))
        {
            if (!Path.IsPathRooted(trial) || !File.Exists(trial)) throw new FileNotFoundException("SEMAPS_VS_HOST_EXE must name an existing absolute executable path.");
            return trial;
        }
        var installed = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "Programs", "SeMaps", "semaps.exe");
        if (File.Exists(installed)) return installed;
        foreach (var directory in (Environment.GetEnvironmentVariable("PATH") ?? "").Split(Path.PathSeparator))
        {
            if (string.IsNullOrWhiteSpace(directory)) continue;
            var candidate = Path.Combine(directory.Trim('"'), "semaps.exe");
            if (File.Exists(candidate)) return candidate;
        }
        throw new FileNotFoundException("Install SeMaps first: semaps.exe was not found in the user installation or PATH.");
    }

    private static void StartHost(string executable, string projectFile)
    {
        var process = Process.Start(new ProcessStartInfo(executable, "--here --no-browser \"" + projectFile + "\"")
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            WorkingDirectory = Path.GetDirectoryName(projectFile)!,
        });
        if (process == null) throw new InvalidOperationException("Could not launch SeMaps.");
        // This is a shared host. Closing the IDE must not kill it.
        process.Dispose();
    }
}
