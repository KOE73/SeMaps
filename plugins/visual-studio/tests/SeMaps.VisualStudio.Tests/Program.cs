using System;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using SeMaps.VisualStudio;

var root = Path.Combine(Path.GetTempPath(), "semaps-vsix-tests-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(root);
try
{
    var nested = Directory.CreateDirectory(Path.Combine(root, "src", "nested")).FullName;
    Assert(ProjectDiscovery.Find(nested).Count == 0, "No configuration must not select a project");
    var file = Path.Combine(root, "sample.semaps");
    File.WriteAllText(file, "version: 1\nworkspace: maps\nsource_root: src\n");
    Assert(ProjectDiscovery.Find(nested)[0] == file, "Find nearest ancestor config");
    File.WriteAllText(Path.Combine(root, "other.semaps"), "version: 1\n");
    Assert(ProjectDiscovery.Find(nested).Count == 2, "Keep ambiguous choices");
    File.WriteAllText(Path.Combine(nested, "local.semaps"), "version: 1\n");
    Assert(ProjectDiscovery.Find(nested).Count == 1, "Nearest directory wins");
    var project = new SeMapsProject(file);
    Assert(project.Workspace == Path.Combine(root, "maps") && project.SourceRoot == Path.Combine(root, "src"), "Resolve config paths against config root");
    var stateDirectory = Directory.CreateDirectory(Path.Combine(root, ".semaps")).FullName;
    var hostFile = Path.Combine(stateDirectory, "host.json");
    File.WriteAllText(hostFile, "{\"port\":12345}");
    var launches = 0;
    using var client = new HttpClient(new Handler(() => new JObject { ["workspace"] = project.Workspace, ["sourceRoot"] = project.SourceRoot }.ToString()));
    var connection = new HostConnection(client, (_, __) => launches++);
    var endpoint = await connection.EnsureAsync(project, CancellationToken.None);
    Assert(endpoint.AbsoluteUri == "http://localhost:12345/app/" && launches == 0, "Reuse correct host without launching");
    using var wrongClient = new HttpClient(new Handler(() => new JObject { ["workspace"] = root, ["sourceRoot"] = project.SourceRoot }.ToString()));
    Assert(await new HostConnection(wrongClient).TryReadAsync(project, CancellationToken.None) == null, "Reject unrelated host");
    using var wrongSource = new HttpClient(new Handler(() => new JObject { ["workspace"] = project.Workspace, ["sourceRoot"] = root }.ToString()));
    Assert(await new HostConnection(wrongSource).TryReadAsync(project, CancellationToken.None) == null, "Reject different source root");
    File.WriteAllText(hostFile, "{");
    Assert(await connection.TryReadAsync(project, CancellationToken.None) == null, "Tolerate transient host.json write");
    File.Delete(hostFile);
    var startingConnection = new HostConnection(client, (_, config) =>
    {
        Assert(config == file, "Start requested config");
        launches++;
        File.WriteAllText(hostFile, "{\"port\":12345}");
    }, () => "test-semaps.exe");
    await startingConnection.EnsureAsync(project, CancellationToken.None);
    Assert(launches == 1, "Start once when no host exists");
    using var cancelled = new CancellationTokenSource();
    cancelled.Cancel();
    try { await startingConnection.EnsureAsync(project, cancelled.Token); throw new Exception("Cancellation ignored"); }
    catch (OperationCanceledException) { Console.WriteLine("PASS: Cancel obsolete solution request"); }
    await IntegrationChecks.RunAsync(root);
    Console.WriteLine("All discovery/host/integration tests passed.");
}
finally
{
    var resolved = Path.GetFullPath(root);
    var expectedParent = Path.GetFullPath(Path.GetTempPath()).TrimEnd(Path.DirectorySeparatorChar);
    if (!string.Equals(Path.GetDirectoryName(resolved), expectedParent, StringComparison.OrdinalIgnoreCase)
        || !Path.GetFileName(resolved).StartsWith("semaps-vsix-tests-", StringComparison.Ordinal))
        throw new InvalidOperationException("Refusing to remove a directory outside the test's temporary location.");
    Directory.Delete(resolved, true);
}

static void Assert(bool condition, string label)
{
    if (!condition) throw new Exception(label);
    Console.WriteLine("PASS: " + label);
}

sealed class Handler(Func<string> response) : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent(response()) });
}
