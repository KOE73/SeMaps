using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.Build.Locator;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.MSBuild;
using SeMaps.Extract.CSharp;

// MSBuildLocator must register before any Microsoft.Build.* assembly is loaded — first thing.
if (!MSBuildLocator.IsRegistered)
{
    MSBuildLocator.RegisterDefaults();
}

// The log goes to stderr of a pipe (semaps reads it as UTF-8); without this a localized
// MSBuild message arrives in the console's OEM code page and turns to mojibake.
Console.OutputEncoding = new System.Text.UTF8Encoding(false);

return await Run(args);

static async Task<int> Run(string[] args)
{
    var options = ArgsParser.Parse(args, out var argError);
    if (options is null)
    {
        Console.Error.WriteLine(argError);
        return 2;
    }

    var rootFullPath = Path.GetFullPath(options.Root);
    if (!Directory.Exists(rootFullPath))
    {
        Console.Error.WriteLine($"--root does not exist: {options.Root}");
        return 1;
    }

    // Design-time load only reads the code. A known vulnerability of a package (NuGet audit,
    // NU1901-NU1904) says nothing about the types, yet the workspace reports it as a load
    // failure on every project. The global NoWarn replaces the projects' own list; harmless
    // here, since warnings only reach this log.
    using var workspace = MSBuildWorkspace.Create(new Dictionary<string, string>
    {
        ["NoWarn"] = "NU1901;NU1902;NU1903;NU1904",
    });
    workspace.RegisterWorkspaceFailedHandler(e => Console.Error.WriteLine($"[msbuild] {e.Diagnostic.Kind}: {e.Diagnostic.Message}"));

    var searchRoots = ResolveSearchRoots(options, rootFullPath);

    var projects = await LoadProjects(workspace, searchRoots);
    if (projects is null)
    {
        Console.Error.WriteLine("no *.sln/*.slnx/*.csproj found under --root or --include");
        return 1;
    }

    var includeRelative = options.Includes
        .Select(i => PathFilter.NormalizeRelative(Path.GetRelativePath(rootFullPath, Path.GetFullPath(i, rootFullPath))))
        .ToList();
    var pathFilter = new PathFilter(includeRelative, options.Excludes);

    var extractor = new FactsExtractor(options.Root, rootFullPath, pathFilter);
    var loadedAny = false;

    foreach (var project in projects)
    {
        var compilation = await project.GetCompilationAsync();
        if (compilation is null)
        {
            continue;
        }

        loadedAny = true;
        extractor.ProcessProject(project, compilation);
    }

    if (!loadedAny)
    {
        Console.Error.WriteLine("no project produced a compilation");
        return 1;
    }

    var document = extractor.Build();

    var jsonOptions = new JsonSerializerOptions
    {
        WriteIndented = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };
    // Normalize to LF: WriteIndented can emit Environment.NewLine, which would make output
    // depend on the host OS and break cross-platform byte-for-byte determinism.
    var json = JsonSerializer.Serialize(document, jsonOptions).Replace("\r\n", "\n");

    var stdout = Console.OpenStandardOutput();
    await using var writer = new StreamWriter(stdout) { AutoFlush = true, NewLine = "\n" };
    await writer.WriteAsync(json);
    await writer.WriteAsync('\n');

    return 0;
}

static List<string> ResolveSearchRoots(Options options, string rootFullPath)
{
    if (options.Includes.Count == 0)
    {
        return [rootFullPath];
    }

    return options.Includes
        .Select(i => Path.GetFullPath(i, rootFullPath))
        .ToList();
}

static async Task<List<Project>?> LoadProjects(MSBuildWorkspace workspace, List<string> searchRoots)
{
    var solutionFiles = searchRoots
        .Where(Directory.Exists)
        .SelectMany(r => Directory.EnumerateFiles(r, "*.sln", SearchOption.AllDirectories)
            .Concat(Directory.EnumerateFiles(r, "*.slnx", SearchOption.AllDirectories)))
        .Distinct(StringComparer.OrdinalIgnoreCase)
        .OrderBy(x => x, StringComparer.Ordinal)
        .ToList();

    if (solutionFiles.Count == 1)
    {
        var solution = await workspace.OpenSolutionAsync(solutionFiles[0]);
        return solution.Projects.ToList();
    }

    var projectFiles = searchRoots
        .Where(Directory.Exists)
        .SelectMany(r => Directory.EnumerateFiles(r, "*.csproj", SearchOption.AllDirectories))
        .Distinct(StringComparer.OrdinalIgnoreCase)
        .OrderBy(x => x, StringComparer.Ordinal)
        .ToList();

    if (projectFiles.Count == 0)
    {
        return null;
    }

    foreach (var projectFile in projectFiles)
    {
        if (workspace.CurrentSolution.Projects.Any(p =>
                string.Equals(p.FilePath, projectFile, StringComparison.OrdinalIgnoreCase)))
        {
            continue;
        }

        await workspace.OpenProjectAsync(projectFile);
    }

    return workspace.CurrentSolution.Projects.ToList();
}
