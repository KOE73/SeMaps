using System.Text.Json.Serialization;

namespace SeMaps.Extract.CSharp;

// Property order below is the wire order (matches schemas/extractor-facts.schema.json);
// System.Text.Json serializes public properties in declaration order.

internal sealed class FactsDocument
{
    [JsonPropertyName("language")] public string Language { get; set; } = "csharp";
    [JsonPropertyName("root")] public string Root { get; set; } = ".";
    [JsonPropertyName("symbols")] public List<SymbolFact> Symbols { get; set; } = [];
    [JsonPropertyName("edges")] public List<EdgeFact> Edges { get; set; } = [];
}

internal sealed class SymbolFact
{
    [JsonPropertyName("id")] public string Id { get; set; } = "";
    [JsonPropertyName("kind")] public string Kind { get; set; } = "";
    [JsonPropertyName("nativeKind")] public string NativeKind { get; set; } = "";
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("namespace")] public string Namespace { get; set; } = "";
    [JsonPropertyName("file")] public string File { get; set; } = "";
    [JsonPropertyName("line")] public int? Line { get; set; }
    [JsonPropertyName("members")] public List<MemberFact>? Members { get; set; }
}

internal sealed class MemberFact
{
    [JsonPropertyName("kind")] public string? Kind { get; set; }
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("type")] public string? Type { get; set; }
    [JsonPropertyName("visibility")] public string? Visibility { get; set; }
    [JsonPropertyName("note")] public string? Note { get; set; }
}

internal sealed class EdgeFact
{
    [JsonPropertyName("from")] public string From { get; set; } = "";
    [JsonPropertyName("to")] public string To { get; set; } = "";
    [JsonPropertyName("kind")] public string Kind { get; set; } = "";
}
