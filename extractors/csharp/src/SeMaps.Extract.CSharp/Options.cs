namespace SeMaps.Extract.CSharp;

/// <summary>Parsed command-line arguments. See docs/EXTRACTOR.md §1.</summary>
internal sealed class Options
{
    public string Root { get; init; } = ".";
    public List<string> Includes { get; init; } = [];
    public List<string> Excludes { get; init; } = [];
    public HashSet<string> EdgeKinds { get; init; } = [];  // holds, uses, injects (subset of holds+uses)
    public string External { get; init; } = "off"; // off, used, expand, full
    public List<string> Libraries { get; init; } = [];
    public List<string> ExternalTypes { get; init; } = [];
    public int ExternalDepth { get; init; } = 1;
    public bool ListLibraries { get; init; }
}

internal static class ArgsParser
{
    /// <summary>
    /// Returns null and sets <paramref name="error"/> when the arguments are invalid
    /// (exit code 2 per EXTRACTOR.md §1).
    /// </summary>
    public static Options? Parse(string[] args, out string? error)
    {
        var root = ".";
        var includes = new List<string>();
        var excludes = new List<string>();
        var edgeKinds = new HashSet<string>();
        var external = "off";
        var libraries = new List<string>();
        var externalTypes = new List<string>();
        var externalDepth = 1;
        var listLibraries = false;

        for (var i = 0; i < args.Length; i++)
        {
            var arg = args[i];
            switch (arg)
            {
                case "--list-libraries":
                    listLibraries = true; break;
                case "--external":
                    if (!TryTakeValue(args, ref i, out external) || external is not ("off" or "used" or "expand" or "full"))
                    { error = "--external requires off, used, expand or full"; return null; }
                    break;
                case "--library":
                    if (!TryTakeValue(args, ref i, out var library) || string.IsNullOrWhiteSpace(library))
                    { error = "--library requires an assembly name or library id"; return null; }
                    libraries.Add(library); break;
                case "--external-type":
                    if (!TryTakeValue(args, ref i, out var externalType) || string.IsNullOrWhiteSpace(externalType))
                    { error = "--external-type requires a metadata type id"; return null; }
                    externalTypes.Add(externalType); break;
                case "--external-depth":
                    if (!TryTakeValue(args, ref i, out var depth) || !int.TryParse(depth,out externalDepth) || externalDepth < 0)
                    { error = "--external-depth requires a nonnegative integer"; return null; }
                    break;
                case "--root":
                    if (!TryTakeValue(args, ref i, out var rootValue))
                    {
                        error = "--root requires a value";
                        return null;
                    }
                    root = rootValue;
                    break;

                case "--include":
                    if (!TryTakeValue(args, ref i, out var includeValue))
                    {
                        error = "--include requires a value";
                        return null;
                    }
                    includes.Add(includeValue);
                    break;

                case "--exclude":
                    if (!TryTakeValue(args, ref i, out var excludeValue))
                    {
                        error = "--exclude requires a value";
                        return null;
                    }
                    excludes.Add(excludeValue);
                    break;

                case "--edges":
                    if (!TryTakeValue(args, ref i, out var edgesValue))
                    {
                        error = "--edges requires a value";
                        return null;
                    }
                    foreach (var kind in edgesValue.Split(','))
                    {
                        var trimmed = kind.Trim();
                        if (trimmed.Length == 0) continue;
                        if (!IsValidEdgeKind(trimmed))
                        {
                            error = $"unknown edge kind: {trimmed}";
                            return null;
                        }
                        edgeKinds.Add(trimmed);
                    }
                    break;

                default:
                    error = $"unknown argument: {arg}";
                    return null;
            }
        }

        error = null;
        if (external == "full" && libraries.Count == 0) { error = "--external full requires --library"; return null; }
        if (external == "expand" && externalTypes.Count == 0) { error = "--external expand requires --external-type"; return null; }
        if (external == "off" && (libraries.Count > 0 || externalTypes.Count > 0)) { error = "library/type selection requires --external"; return null; }
        return new Options { Root = root, Includes = includes, Excludes = excludes, EdgeKinds = edgeKinds, External=external,Libraries=libraries,ExternalTypes=externalTypes,ExternalDepth=externalDepth,ListLibraries=listLibraries };
    }

    private static bool IsValidEdgeKind(string kind) => kind is "holds" or "uses" or "injects" or "calls";

    private static bool TryTakeValue(string[] args, ref int i, out string value)
    {
        if (i + 1 >= args.Length)
        {
            value = "";
            return false;
        }

        i++;
        value = args[i];
        return true;
    }
}
