using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using SeMaps.Extract.CSharp;

namespace SeMaps.VisualStudio;

internal sealed class SourceSymbol
{
    public string SymbolId { get; set; } = "";
    public string Assembly { get; set; } = "";
    public string File { get; set; } = "";
    public int Line { get; set; }
    public bool Unsaved { get; set; }
    public override string ToString() => SymbolId + "  [" + Assembly + "]  " + File + ":" + Line + (Unsaved ? " (unsaved buffer)" : "");
}

internal static class SymbolResolver
{
    public static async Task<IReadOnlyList<SourceSymbol>> AtAsync(Solution solution, string sourceRoot, string file, int line, int column, CancellationToken token)
    {
        var relative = SourcePaths.Relative(sourceRoot, file);
        var candidates = new List<SourceSymbol>();
        foreach (var id in solution.GetDocumentIdsWithFilePath(Path.GetFullPath(file)))
        {
            var document = solution.GetDocument(id)!;
            if (document.Project.Language != LanguageNames.CSharp) continue;
            var text = await document.GetTextAsync(token).ConfigureAwait(false);
            if (line < 0 || line >= text.Lines.Count || column < 0 || column > text.Lines[line].Span.Length) continue;
            var root = await document.GetSyntaxRootAsync(token).ConfigureAwait(false);
            var model = await document.GetSemanticModelAsync(token).ConfigureAwait(false);
            if (root == null || model == null) continue;
            var position = text.Lines[line].Start + column;
            var declaration = root.FindToken(Math.Min(position, Math.Max(0, root.FullSpan.End - 1))).Parent?.AncestorsAndSelf()
                .FirstOrDefault(n => n is BaseTypeDeclarationSyntax || n is DelegateDeclarationSyntax);
            if (declaration == null || !(model.GetDeclaredSymbol(declaration, token) is INamedTypeSymbol symbol)) continue;
            var symbolId = SymbolIds.TypeId(symbol); var assembly = symbol.ContainingAssembly.Identity.Name;
            if (candidates.Any(c => c.SymbolId == symbolId && c.Assembly == assembly)) continue;
            candidates.Add(new SourceSymbol { SymbolId = symbolId, Assembly = assembly, File = relative,
                Line = declaration.GetLocation().GetLineSpan().StartLinePosition.Line + 1,
                Unsaved = File.Exists(file) && text.ToString() != File.ReadAllText(file) });
        }
        return candidates;
    }

    /// <summary>Real IDE compilations; no reparsing a file without project references.</summary>
    public static async Task<IReadOnlyList<SourceSymbol>> TypesAsync(Solution solution, string sourceRoot, string[] files, CancellationToken token)
    {
        var candidates = new List<SourceSymbol>();
        foreach (var file in files.Distinct(StringComparer.OrdinalIgnoreCase))
        {
            token.ThrowIfCancellationRequested();
            var relative = SourcePaths.Relative(sourceRoot, file);
            if (!string.Equals(Path.GetExtension(file), ".cs", StringComparison.OrdinalIgnoreCase))
                throw new ArgumentException("Only C# source files are supported in this stage: " + relative);
            var documents = solution.GetDocumentIdsWithFilePath(Path.GetFullPath(file));
            if (documents.IsEmpty) throw new ArgumentException("File is not in the loaded Roslyn solution: " + relative);
            foreach (var id in documents)
            {
                var document = solution.GetDocument(id)!;
                var root = await document.GetSyntaxRootAsync(token).ConfigureAwait(false);
                var model = await document.GetSemanticModelAsync(token).ConfigureAwait(false);
                var text = await document.GetTextAsync(token).ConfigureAwait(false);
                var unsaved = File.Exists(file) && text.ToString() != File.ReadAllText(file);
                if (root == null || model == null) continue;
                foreach (var declaration in root.DescendantNodes().Where(n => n is BaseTypeDeclarationSyntax || n is DelegateDeclarationSyntax))
                {
                    if (!(model.GetDeclaredSymbol(declaration, token) is INamedTypeSymbol symbol)) continue;
                    var symbolId = SymbolIds.TypeId(symbol);
                    var assembly = symbol.ContainingAssembly.Identity.Name;
                    if (candidates.Any(c => c.SymbolId == symbolId && c.Assembly == assembly)) continue;
                    candidates.Add(new SourceSymbol { SymbolId = symbolId, Assembly = assembly, File = relative,
                        Line = declaration.GetLocation().GetLineSpan().StartLinePosition.Line + 1, Unsaved = unsaved });
                }
            }
        }
        return candidates;
    }
}
