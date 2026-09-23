namespace SeMaps.Extract.CSharp;

/// <summary>Parsed command-line arguments. See docs/EXTRACTOR.md §1.</summary>
internal sealed class Options
{
    public string Root { get; init; } = ".";
    public List<string> Includes { get; init; } = [];
    public List<string> Excludes { get; init; } = [];
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

        for (var i = 0; i < args.Length; i++)
        {
            var arg = args[i];
            switch (arg)
            {
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

                default:
                    error = $"unknown argument: {arg}";
                    return null;
            }
        }

        error = null;
        return new Options { Root = root, Includes = includes, Excludes = excludes };
    }

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
