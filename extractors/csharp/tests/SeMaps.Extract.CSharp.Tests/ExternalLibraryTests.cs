using System.Diagnostics;
using System.Text.Json.Nodes;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Json.Schema;
using Xunit;

namespace SeMaps.Extract.CSharp.Tests;

public sealed class ExternalLibraryFixture : IAsyncLifetime
{
    public string Root { get; } = Path.Combine(Path.GetTempPath(),"semaps-external-tests-"+Guid.NewGuid().ToString("N"));
    public async Task InitializeAsync()
    {
        Directory.CreateDirectory(Path.Combine(Root,"Consumer"));
        var refs = ((string)AppContext.GetData("TRUSTED_PLATFORM_ASSEMBLIES")!).Split(Path.PathSeparator)
            .Select(p=>MetadataReference.CreateFromFile(p)).ToList();
        Emit("LibraryB","namespace LibB { public struct Frame { public long Id; } public class Leaf {} } namespace Shared { public class Same {} }",refs);
        refs.Add(MetadataReference.CreateFromFile(Path.Combine(Root,"LibraryB.dll")));
        Emit("LibraryA","""
            namespace LibA {
                public interface IContract {}
                public class Base : IContract {}
                public class A : Base { public LibB.Frame Value; public Sibling Child; private Hidden hidden; public LibB.Leaf Make(LibB.Leaf input) => input; }
                public class Sibling { public A Parent; }
                internal class Hidden {}
                public class Unused { public LibB.Leaf Child; }
                public class Generic<T> { public T Value; }
                public class Outer { public class Nested {} }
            }
            namespace Shared { public class Same {} }
            """,refs);
        await File.WriteAllTextAsync(Path.Combine(Root,"Consumer","Consumer.csproj"),"""
            <Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><AllowUnsafeBlocks>true</AllowUnsafeBlocks></PropertyGroup>
            <ItemGroup><Reference Include="LibraryA"><HintPath>../LibraryA.dll</HintPath></Reference><Reference Include="LibraryB"><HintPath>../LibraryB.dll</HintPath></Reference></ItemGroup></Project>
            """);
        await File.WriteAllTextAsync(Path.Combine(Root,"Consumer","Source.cs"),"""
            public unsafe class Local { public LibA.A Value; public System.Collections.Generic.List<LibA.A> Items; public LibB.Frame* frame; }
            """);
        var psi = new ProcessStartInfo("dotnet") { WorkingDirectory=Path.Combine(Root,"Consumer"),RedirectStandardOutput=true,RedirectStandardError=true,UseShellExecute=false };
        foreach (var arg in new[]{"restore","-nodeReuse:false"}) psi.ArgumentList.Add(arg);
        psi.Environment["MSBUILDDISABLENODEREUSE"]="1";
        using var process=Process.Start(psi)!;
        var stdout=process.StandardOutput.ReadToEndAsync();var stderr=process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();
        if (process.ExitCode!=0) throw new InvalidOperationException(await stdout+await stderr);
    }
    private void Emit(string name,string source,List<PortableExecutableReference> refs)
    {
        var compilation=CSharpCompilation.Create(name,[CSharpSyntaxTree.ParseText(source)],refs,new CSharpCompilationOptions(OutputKind.DynamicallyLinkedLibrary));
        var result=compilation.Emit(Path.Combine(Root,name+".dll"));
        if(!result.Success) throw new InvalidOperationException(string.Join("\n",result.Diagnostics));
    }
    public Task DisposeAsync()
    {
        // Only this fixture's unique directory under the system temporary root.
        var full=Path.GetFullPath(Root);
        if (!full.StartsWith(Path.GetFullPath(Path.GetTempPath()),StringComparison.OrdinalIgnoreCase) || !Path.GetFileName(full).StartsWith("semaps-external-tests-",StringComparison.Ordinal)) throw new InvalidOperationException("unsafe fixture path");
        Directory.Delete(full,true);return Task.CompletedTask;
    }
}

[CollectionDefinition("ExternalLibraries")]
public sealed class ExternalLibraryCollection : ICollectionFixture<ExternalLibraryFixture>;

[Collection("ExternalLibraries")]
public sealed class ExternalLibraryTests(ExternalLibraryFixture fixture)
{
    private JsonObject Run(params string[] extra)
    {
        var result=ToolHost.Run(fixture.Root,new[]{"--root",".","--include","Consumer","--edges","holds,uses"}.Concat(extra).ToArray());
        Assert.True(result.ExitCode==0,result.StdErr);
        var json=JsonNode.Parse(result.StdOut)!.AsObject();
        var evaluation=JsonSchema.FromFile(TestPaths.SchemaPath, new BuildOptions { SchemaRegistry = new SchemaRegistry() }).Evaluate(System.Text.Json.JsonSerializer.SerializeToElement(json),new EvaluationOptions {OutputFormat=OutputFormat.List});
        Assert.True(evaluation.IsValid,System.Text.Json.JsonSerializer.Serialize(evaluation));
        return json;
    }
    private static bool Symbol(JsonObject json,string id)=>json["symbols"]!.AsArray().Any(s=>s!["id"]!.GetValue<string>()==id);
    private static bool Edge(JsonObject json,string from,string to,string kind)=>json["edges"]!.AsArray().Any(e=>e!["from"]!.GetValue<string>()==from&&e["to"]!.GetValue<string>()==to&&e["kind"]!.GetValue<string>()==kind);

    [Fact] public void UsedProducesBoundaryAndPointerButDoesNotExpandLibraries()
    {
        var json=Run("--external","used");
        Assert.True(Symbol(json,"[LibraryA]::LibA.A"));
        Assert.True(Symbol(json,"[LibraryB]::LibB.Frame"));
        Assert.True(Edge(json,"Local","[LibraryA]::LibA.A","holds"));
        Assert.True(Edge(json,"Local","[LibraryB]::LibB.Frame","holds"));
        Assert.Contains(json["edges"]!.AsArray(),e=>e!["to"]!.GetValue<string>()=="[LibraryB]::LibB.Frame" && e["via"]?["path"]?.ToJsonString()=="[\"pointee\"]");
        Assert.False(Symbol(json,"[LibraryA]::LibA.Unused"));
        Assert.False(Edge(json,"[LibraryA]::LibA.A","[LibraryB]::LibB.Frame","holds"));
        Assert.All(json["symbols"]!.AsArray().Where(s=>s!["library"]!=null),s=>Assert.Null(s!["file"]));
    }
    [Fact] public void ExpandTraversesExternalEdgesWithBoundedDepthAndCycles()
    {
        var one=Run("--external","expand","--external-type","[LibraryA]::LibA.A","--external-depth","1");
        Assert.True(Edge(one,"[LibraryA]::LibA.A","[LibraryB]::LibB.Frame","holds"));
        Assert.True(Edge(one,"[LibraryA]::LibA.A","[LibraryA]::LibA.Hidden","holds"));
        Assert.True(Edge(one,"[LibraryA]::LibA.A","[LibraryB]::LibB.Leaf","uses"));
        Assert.False(Edge(one,"[LibraryA]::LibA.Sibling","[LibraryA]::LibA.A","holds"));
        var two=Run("--external","expand","--external-type","LibA.A","--external-depth","2");
        Assert.True(Edge(two,"[LibraryA]::LibA.Sibling","[LibraryA]::LibA.A","holds"));
        Assert.True(Edge(two,"[LibraryA]::LibA.Base","[LibraryA]::LibA.IContract","implements"));
        Assert.False(Symbol(two,"[LibraryA]::LibA.Unused"));
    }
    [Fact] public void FullExtractsUnusedNestedAndPrivateTypesAndDisambiguatesAssemblies()
    {
        var json=Run("--external","full","--library","LibraryA","--library","LibraryB");
        Assert.True(Symbol(json,"[LibraryA]::LibA.Unused"));
        Assert.True(Symbol(json,"[LibraryA]::LibA.Outer+Nested"));
        Assert.True(Symbol(json,"[LibraryA]::Shared.Same"));
        Assert.True(Symbol(json,"[LibraryB]::Shared.Same"));
        Assert.True(Edge(json,"[LibraryA]::LibA.Unused","[LibraryB]::LibB.Leaf","holds"));
        Assert.All(json["libraries"]!.AsArray().Where(l=>l!["id"]!.GetValue<string>() is "LibraryA" or "LibraryB"),l=>Assert.True(l!["complete"]!.GetValue<bool>()));
        Assert.DoesNotContain(json["edges"]!.AsArray(),e=>e!["kind"]!.GetValue<string>()=="calls");
        Assert.Equal(json.ToJsonString(),Run("--external","full","--library","LibraryA","--library","LibraryB").ToJsonString());
    }
    [Fact] public void UsedLibraryFilterAndMissingSelectionAreExplicit()
    {
        var json=Run("--external","used","--library","LibraryA");
        Assert.True(Symbol(json,"[LibraryA]::LibA.A"));
        Assert.False(Symbol(json,"[LibraryB]::LibB.Frame"));
        var result=ToolHost.Run(fixture.Root,["--root",".","--include","Consumer","--external","used","--library","NotReferenced"]);
        Assert.Equal(1,result.ExitCode);Assert.Empty(result.StdOut);Assert.Contains("NotReferenced",result.StdErr);
    }
    [Theory]
    [InlineData("--external","full")]
    [InlineData("--external","expand")]
    [InlineData("--external-depth","-1")]
    [InlineData("--external","unknown")]
    public void InvalidSelectionsFailWithoutFacts(params string[] args)
    {
        var result=ToolHost.Run(fixture.Root,args);Assert.Equal(2,result.ExitCode);Assert.Empty(result.StdOut);
    }
}
