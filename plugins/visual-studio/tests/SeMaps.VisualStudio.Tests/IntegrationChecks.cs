using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.Text;
using Newtonsoft.Json.Linq;
using SeMaps.VisualStudio;

internal static class IntegrationChecks
{
    public static async Task RunAsync(string root)
    {
        Assert(SourcePaths.Relative(root, Path.Combine(root, "src", "A.cs")) == "src/A.cs", "Normalize native paths to protocol slashes");
        Assert(SourcePaths.Resolve(root, "src/A.cs") == Path.Combine(root, "src", "A.cs"), "Resolve protocol source path");
        foreach (var path in new[] { "../outside.cs", "C:/outside.cs", "/outside.cs", "src/A.cs:stream" })
        {
            try { SourcePaths.Resolve(root, path); throw new Exception("Unsafe path accepted: " + path); }
            catch (ArgumentException) { }
        }
        Assert(true, "Reject traversal, absolute and alternate stream paths");
        try { SourcePaths.Relative(root, root + "-other/A.cs"); throw new Exception("Sibling prefix accepted"); }
        catch (ArgumentException) { Assert(true, "Reject sibling directory with same prefix"); }

        var sent = new List<JObject>(); var client = new IdeBridgeClient(sent.Add);
        var pending = client.RequestAsync("context.get", new JObject(), CancellationToken.None);
        var request = sent.Last();
        await client.ReceiveAsync(new JObject { ["protocol"] = "semaps.ide", ["version"] = 1, ["sessionId"] = "foreign", ["kind"] = "response", ["requestId"] = request["id"], ["result"] = new JObject() });
        Assert(!pending.IsCompleted, "Ignore a response from a foreign IDE session");
        await client.ReceiveAsync(new JObject { ["protocol"] = "semaps.ide", ["version"] = 1, ["sessionId"] = client.Session, ["kind"] = "response", ["requestId"] = request["id"], ["result"] = new JObject { ["ok"] = true } });
        Assert((bool)(await pending)["ok"]!, "Correlate editor response with pending request");
        var oldSession = client.Session; var cancelled = client.RequestAsync("context.get", new JObject(), CancellationToken.None);
        client.Reset();
        try { await cancelled; throw new Exception("Reset did not cancel"); }
        catch (OperationCanceledException) { Assert(client.Session != oldSession, "Reset cancels pending requests and changes session"); }
        try { await client.RequestAsync("context.get", new JObject(), CancellationToken.None, 5); throw new Exception("Timeout ignored"); }
        catch (OperationCanceledException) { Assert(true, "IDE requests have a bounded timeout"); }
        var opened = 0; client.RequestReceived = (_, __) => { opened++; return Task.FromResult(new JObject { ["opened"] = true }); };
        var open = new JObject { ["protocol"] = "semaps.ide", ["version"] = 1, ["sessionId"] = client.Session,
            ["kind"] = "request", ["id"] = "open", ["method"] = "source.open", ["params"] = new JObject { ["file"] = "src/A.cs" } };
        await client.ReceiveAsync(open); await client.ReceiveAsync(open);
        Assert(opened == 1, "Repeated source request opens the file once");
        Assert((bool)sent.Last()["result"]!["opened"]!, "Replay returns the cached response");
        var conflict = (JObject)open.DeepClone(); conflict["params"] = new JObject { ["file"] = "src/B.cs" };
        await client.ReceiveAsync(conflict);
        Assert((string?)sent.Last()["error"]?["code"] == "duplicateId" && opened == 1, "Reject request id reused with different parameters");
        foreach (var field in new[] { "protocol", "version", "sessionId", "kind", "id", "method", "params" })
        {
            var invalid = (JObject)open.DeepClone(); invalid[field] = new JArray();
            Assert(!IdeBridgeClient.ValidMessage(invalid), "Reject malformed " + field + " without throwing");
        }
        var hugeVersion = (JObject)open.DeepClone(); hugeVersion["version"] = long.MaxValue;
        Assert(!IdeBridgeClient.ValidMessage(hugeVersion), "Reject out of range protocol version without throwing");
        var firstOpen = new TaskCompletionSource<JObject>(); var concurrentCount = 0;
        client.Reset();
        client.RequestReceived = (_, p) => { if ((string?)p["file"] == "slow.cs") { concurrentCount++; return firstOpen.Task; } return Task.FromResult(new JObject { ["opened"] = true }); };
        var slow = (JObject)open.DeepClone(); slow["sessionId"] = client.Session; slow["id"] = "slow"; slow["params"] = new JObject { ["file"] = "slow.cs" };
        var slowRequest = client.ReceiveAsync(slow);
        for (var index = 0; index < 201; index++)
        {
            var other = (JObject)open.DeepClone(); other["sessionId"] = client.Session; other["id"] = "other-" + index;
            await client.ReceiveAsync(other);
        }
        await client.ReceiveAsync(slow);
        Assert(concurrentCount == 1, "Completed replay eviction preserves running requests");
        firstOpen.SetResult(new JObject { ["opened"] = true }); await slowRequest;

        using var workspace = new AdhocWorkspace();
        var project = workspace.AddProject("Symbols", LanguageNames.CSharp).WithCompilationOptions(new CSharpCompilationOptions(OutputKind.DynamicallyLinkedLibrary))
            .AddMetadataReference(MetadataReference.CreateFromFile(typeof(object).Assembly.Location));
        var file1 = Path.Combine(root, "Symbols.cs"); var file2 = Path.Combine(root, "Partial.cs");
        const string source = "namespace N { public partial class Outer<T> { public class Inner {} } public delegate void Handler(); public enum Choice { A } }";
        File.WriteAllText(file1, source); File.WriteAllText(file2, "namespace N { public partial class Outer<T> {} }");
        var first = project.AddDocument("Symbols.cs", SourceText.From(source), filePath: file1);
        var second = first.Project.AddDocument("Partial.cs", SourceText.From(File.ReadAllText(file2)), filePath: file2);
        var symbols = await SymbolResolver.TypesAsync(second.Project.Solution, root, new[] { file1, file2 }, CancellationToken.None);
        Assert(symbols.Select(s => s.SymbolId).OrderBy(s => s).SequenceEqual(new[] { "N.Choice", "N.Handler", "N.Outer`1", "N.Outer`1+Inner" }), "Roslyn ids match extractor for generics, nested types, delegates, enums and partials");
        var at = await SymbolResolver.AtAsync(second.Project.Solution, root, file1, 0, source.IndexOf("Inner", StringComparison.Ordinal), CancellationToken.None);
        Assert(at.Single().SymbolId == "N.Outer`1+Inner", "Caret resolves the nearest containing type with extractor identity");
        var edited = second.Project.Solution.WithDocumentText(first.Id, SourceText.From(source + " // unsaved"));
        var unsaved = await SymbolResolver.TypesAsync(edited, root, new[] { file1 }, CancellationToken.None);
        Assert(unsaved.All(s => s.Unsaved), "Resolve the unsaved Roslyn buffer and report its divergence from disk");
        try { await SymbolResolver.TypesAsync(edited, root, new[] { Path.Combine(root, "Unknown.cs") }, CancellationToken.None); throw new Exception("Unknown file accepted"); }
        catch (ArgumentException) { Assert(true, "Reject dropped source files outside the loaded solution"); }
    }

    private static void Assert(bool condition, string name)
    {
        if (!condition) throw new Exception(name);
        Console.WriteLine("PASS: " + name);
    }
}
