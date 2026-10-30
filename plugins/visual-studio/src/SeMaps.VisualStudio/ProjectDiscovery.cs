using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using YamlDotNet.RepresentationModel;

namespace SeMaps.VisualStudio;

internal sealed class SeMapsProject
{
    public string File { get; }
    public string Root => Path.GetDirectoryName(File)!;
    public string Workspace { get; }
    public string SourceRoot { get; }

    public SeMapsProject(string file)
    {
        File = Path.GetFullPath(file);
        var yaml = new YamlStream();
        using (var reader = System.IO.File.OpenText(File)) yaml.Load(reader);
        if (yaml.Documents.Count != 1 || !(yaml.Documents[0].RootNode is YamlMappingNode map))
            throw new InvalidDataException("The .semaps file must contain one YAML mapping.");
        string Setting(string key, string fallback) =>
            map.Children.TryGetValue(new YamlScalarNode(key), out var node)
                ? (node as YamlScalarNode)?.Value ?? throw new InvalidDataException(key + " must be a string.")
                : fallback;
        Workspace = Path.GetFullPath(Path.Combine(Root, Setting("workspace", "docs/diagrams")));
        SourceRoot = Path.GetFullPath(Path.Combine(Root, Setting("source_root", ".")));
    }
}

internal static class ProjectDiscovery
{
    // Match SeMaps' nearest-directory rule; do not recursively search unrelated projects.
    public static IReadOnlyList<string> Find(string solutionDirectory)
    {
        for (var directory = new DirectoryInfo(solutionDirectory); directory != null; directory = directory.Parent)
        {
            var files = directory.EnumerateFiles("*", SearchOption.TopDirectoryOnly)
                .Where(f => string.Equals(f.Extension, ".semaps", StringComparison.OrdinalIgnoreCase))
                .Select(f => f.FullName).OrderBy(f => f, StringComparer.OrdinalIgnoreCase).ToArray();
            if (files.Length != 0) return files;
        }
        return Array.Empty<string>();
    }
}
