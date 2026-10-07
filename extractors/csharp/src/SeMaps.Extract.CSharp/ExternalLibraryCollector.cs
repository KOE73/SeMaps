using Microsoft.CodeAnalysis;

namespace SeMaps.Extract.CSharp;

/// <summary>Metadata-only structural extraction. Never loads a DLL for execution.</summary>
internal sealed class ExternalLibraryCollector(Options options, HashSet<string> outputIds,
    IEnumerable<IAssemblySymbol> assemblies)
{
    private readonly Dictionary<string, IAssemblySymbol> available = AssembliesById(assemblies);
    private readonly Dictionary<string, INamedTypeSymbol> types = new(StringComparer.Ordinal);
    private readonly Dictionary<string, SymbolFact> facts = new(StringComparer.Ordinal);
    private readonly HashSet<string> expanded = new(StringComparer.Ordinal);
    private readonly HashSet<string> complete = new(StringComparer.Ordinal);

    private static Dictionary<string,IAssemblySymbol> AssembliesById(IEnumerable<IAssemblySymbol> input)
    {
        var result = new Dictionary<string,IAssemblySymbol>(StringComparer.Ordinal);
        var queue = new Queue<IAssemblySymbol>(input);
        while (queue.TryDequeue(out var assembly))
        {
            // Unresolved transitive references are Roslyn placeholders, often version 0.0.0.0.
            // They are not another loaded DLL and cannot supply metadata definitions.
            if (!assembly.Locations.Any(location => location.IsInMetadata)) continue;
            var id = SymbolIds.LibraryId(assembly);
            if (result.TryGetValue(id,out var old))
            {
                if (old.Identity.Version != assembly.Identity.Version)
                    throw new InvalidOperationException($"Library {id} resolves to multiple versions: {old.Identity.Version} ({string.Join(", ",old.Locations)}) and {assembly.Identity.Version} ({string.Join(", ",assembly.Locations)}); choose one target framework/project.");
                continue;
            }
            result[id] = assembly;
            foreach (var module in assembly.Modules)
                foreach (var reference in module.ReferencedAssemblySymbols) queue.Enqueue(reference);
        }
        return result;
    }

    public static List<LibraryFact> ListLibraries(IEnumerable<IAssemblySymbol> assemblies) =>
        AssembliesById(assemblies).OrderBy(p => p.Value.Identity.Name, StringComparer.Ordinal)
        .ThenBy(p => p.Key, StringComparer.Ordinal)
        .Select(p => new LibraryFact { Id=p.Key,Name=p.Value.Identity.Name,Version=p.Value.Identity.Version.ToString() }).ToList();

    public string? Resolve(INamedTypeSymbol type)
    {
        type = type.OriginalDefinition;
        var local = SymbolIds.TypeId(type);
        if (type.Locations.Any(l => l.IsInSource)) return outputIds.Contains(local) ? local : null;
        if (type.TypeKind is TypeKind.Error or TypeKind.TypeParameter || (type.SpecialType != SpecialType.None && !complete.Contains(SymbolIds.LibraryId(type.ContainingAssembly))) ||
            !type.Locations.Any(l => l.IsInMetadata)) return null;
        var id = SymbolIds.ExternalTypeId(type);
        if (!facts.ContainsKey(id))
        {
            var library = SymbolIds.LibraryId(type.ContainingAssembly);
            if (options.External=="used" && options.Libraries.Count>0 && !options.Libraries.Contains(library) && !options.Libraries.Contains(type.ContainingAssembly.Identity.Name)) return null;
            available.TryAdd(library,type.ContainingAssembly);
            types[id] = type;
            var (kind,nativeKind) = FactsExtractor.ClassifyType(type);
            facts[id] = new SymbolFact { Id=id,Kind=kind,NativeKind=nativeKind,Name=type.Name,
                Namespace=SymbolIds.NamespaceDottedName(type.ContainingNamespace),Library=library,File=null,
                Visibility=SymbolDisplayHelpers.Visibility(type.DeclaredAccessibility) };
            outputIds.Add(id);
        }
        return id;
    }

    public void Build(List<SymbolFact> symbols, HashSet<EdgeWithVia> edges)
    {
        // Validate explicit selections even in `used`, where an unused library
        // legitimately contributes no nodes. A typo must not look like an empty graph.
        foreach (var selection in options.Libraries)
        {
            var count = available.Count(x=>x.Key==selection || x.Value.Identity.Name==selection);
            if (count != 1) throw new InvalidOperationException($"Library {selection}: expected one referenced assembly, found {count}.");
        }
        var seeds = new List<string>();
        if (options.External == "full")
        {
            foreach (var selection in options.Libraries)
            {
                var matches = available.Where(x=>x.Key==selection || x.Value.Identity.Name==selection).ToList();
                if (matches.Count != 1) throw new InvalidOperationException($"Library {selection}: expected one referenced assembly, found {matches.Count}.");
                complete.Add(matches[0].Key);
                foreach (var type in AllTypes(matches[0].Value.GlobalNamespace))
                    if (Resolve(type) is { } id) seeds.Add(id);
            }
        }
        else if (options.External == "expand")
        {
            foreach (var selection in options.ExternalTypes)
            {
                var matches = new List<INamedTypeSymbol>();
                foreach (var (lib,assembly) in available)
                {
                    if (options.Libraries.Count > 0 && !options.Libraries.Contains(lib) && !options.Libraries.Contains(assembly.Identity.Name)) continue;
                    var prefix = "["+lib+"]::";
                    if (selection.StartsWith("[",StringComparison.Ordinal) && !selection.StartsWith(prefix,StringComparison.Ordinal)) continue;
                    var name = selection.StartsWith(prefix,StringComparison.Ordinal) ? selection[prefix.Length..] : selection;
                    if (assembly.GetTypeByMetadataName(name) is { } type) matches.Add(type);
                }
                if (matches.Count != 1) throw new InvalidOperationException($"External type {selection}: expected one metadata definition, found {matches.Count}.");
                if (Resolve(matches[0]) is { } id) seeds.Add(id);
            }
        }
        var queue = new Queue<(string Id,int Depth)>();
        foreach (var id in seeds.Distinct()) queue.Enqueue((id,0));
        while (queue.TryDequeue(out var item))
        {
            if (options.External != "full" && item.Depth >= options.ExternalDepth) continue;
            if (!expanded.Add(item.Id)) continue;
            var type = types[item.Id];
            var (members,_) = MembersBuilder.Build(type,facts[item.Id].Kind,"",_=>null);
            facts[item.Id].Members = members ?? [];
            var targets = new HashSet<string>(StringComparer.Ordinal);
            string? ResolveTarget(INamedTypeSymbol target)
            {
                var result = Resolve(target);
                if (result is not null && types.ContainsKey(result)) targets.Add(result);
                return result;
            }
            FactsExtractor.AddStructuralEdges(item.Id,type,outputIds,edges,"",null,ResolveTarget);
            ReferenceEdgeCollector.Collect(item.Id,type,outputIds,edges,options.EdgeKinds,"",_=>null,ResolveTarget);
            foreach (var nested in type.GetTypeMembers())
            {
                if (ResolveTarget(nested) is not { } id) continue;
                edges.Add(new EdgeWithVia { From=item.Id,To=id,Kind="contains",Native="nested" });
            }
            // Full is bounded to selected assemblies; dependencies remain endpoints.
            foreach (var id in targets.Order(StringComparer.Ordinal))
                if (options.External != "full" || complete.Contains(facts[id].Library!)) queue.Enqueue((id,item.Depth+1));
        }
        // A library is also an ordinary assembly container; all its known types belong to it.
        foreach (var group in facts.Values.GroupBy(f=>f.Library).ToList())
        {
            var lib = group.Key!; var assemblyId = "["+lib+"]";
            symbols.Add(new SymbolFact { Id=assemblyId,Kind="module",NativeKind="assembly",Name=available[lib].Identity.Name,Library=lib,File=null });
            foreach (var type in group)
                edges.Add(new EdgeWithVia { From=assemblyId,To=type.Id,Kind="contains",Native="assembly" });
        }
        symbols.AddRange(facts.Values);
    }

    public List<LibraryFact>? Libraries => facts.Count == 0 && complete.Count == 0 ? null :
        facts.Values.Select(f=>f.Library!).Concat(complete).Distinct().Order(StringComparer.Ordinal)
        .Select(id=>new LibraryFact { Id=id,Name=available[id].Identity.Name,Version=available[id].Identity.Version.ToString(),
            Complete=complete.Contains(id) ? true : null,
            Expanded=expanded.Where(s=>facts[s].Library==id).Order(StringComparer.Ordinal).ToList() is {Count:>0} list ? list : null }).ToList();

    private static IEnumerable<INamedTypeSymbol> AllTypes(INamespaceSymbol ns)
    {
        foreach (var member in ns.GetMembers())
            if (member is INamespaceSymbol child) { foreach (var type in AllTypes(child)) yield return type; }
            else if (member is INamedTypeSymbol type) { yield return type; foreach (var nested in NestedTypes(type)) yield return nested; }
    }
    private static IEnumerable<INamedTypeSymbol> NestedTypes(INamedTypeSymbol type)
    {
        foreach (var nested in type.GetTypeMembers()) { yield return nested; foreach (var child in NestedTypes(nested)) yield return child; }
    }
}
