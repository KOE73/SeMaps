using System;
using System.IO;

namespace SeMaps.VisualStudio;

internal static class SourcePaths
{
    public static string Resolve(string root, string relative)
    {
        if (string.IsNullOrWhiteSpace(relative) || Path.IsPathRooted(relative) || relative.IndexOf(':') >= 0)
            throw new ArgumentException("Source path must be relative to source_root.");
        var prefix = Path.GetFullPath(root).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
        var path = Path.GetFullPath(Path.Combine(prefix, relative.Replace('/', Path.DirectorySeparatorChar)));
        if (!path.StartsWith(prefix, StringComparison.OrdinalIgnoreCase)) throw new ArgumentException("Source path leaves source_root.");
        return path;
    }

    public static string Relative(string root, string file)
    {
        var prefix = Path.GetFullPath(root).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
        var path = Path.GetFullPath(file);
        if (!path.StartsWith(prefix, StringComparison.OrdinalIgnoreCase)) throw new ArgumentException("Dropped file is outside source_root.");
        return path.Substring(prefix.Length).Replace('\\', '/');
    }
}
