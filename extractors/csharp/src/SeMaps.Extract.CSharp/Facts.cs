using System.Text.Json.Serialization;

namespace SeMaps.Extract.CSharp;

// Property order below is the wire order (matches schemas/extractor-facts.schema.json);
// System.Text.Json serializes public properties in declaration order.

internal sealed class FactsDocument
{
    [JsonPropertyName("language")] public string Language { get; set; } = "csharp";
    [JsonPropertyName("root")] public string Root { get; set; } = ".";
    [JsonPropertyName("edgeKinds")] public List<string>? EdgeKinds { get; set; }
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
    [JsonPropertyName("endLine")] public int? EndLine { get; set; }
    [JsonPropertyName("spans")] public List<SpanFact>? Spans { get; set; }
    [JsonPropertyName("visibility")] public string? Visibility { get; set; }
    [JsonPropertyName("members")] public List<MemberFact>? Members { get; set; }
    [JsonPropertyName("memberLines")] public Dictionary<string, int>? MemberLines { get; set; }
    // Blind-spot marks (ADR_20260928-5 §4, PLAN_20260928-7 step 6): only for kind
    // "method", only printed with --edges calls, only when not empty.
    [JsonPropertyName("dynamic")] public List<DynamicMarkFact>? Dynamic { get; set; }
}

internal sealed class DynamicMarkFact
{
    [JsonPropertyName("kind")] public string Kind { get; set; } = "";
    [JsonPropertyName("line")] public int Line { get; set; }
}

internal sealed class SpanFact
{
    [JsonPropertyName("file")] public string File { get; set; } = "";
    [JsonPropertyName("line")] public int Line { get; set; }
    [JsonPropertyName("endLine")] public int? EndLine { get; set; }
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
    [JsonPropertyName("native")] public string? Native { get; set; }
    [JsonPropertyName("via")] public ViaFact? Via { get; set; }
    [JsonPropertyName("line")] public int? Line { get; set; }
    [JsonPropertyName("file")] public string? File { get; set; }
    [JsonPropertyName("lines")] public List<int>? Lines { get; set; }
}

internal sealed class ViaFact
{
    [JsonPropertyName("member")] public string? Member { get; set; }
    [JsonPropertyName("memberKind")] public string? MemberKind { get; set; }
    [JsonPropertyName("modifiers")] public List<string>? Modifiers { get; set; }
    [JsonPropertyName("text")] public string? Text { get; set; }
    [JsonPropertyName("path")] public List<string>? Path { get; set; }
    [JsonPropertyName("cardinality")] public string? Cardinality { get; set; }
    [JsonPropertyName("mutability")] public string? Mutability { get; set; }
    [JsonPropertyName("deferred")] public bool? Deferred { get; set; }
}
