namespace Sample.Core;

// Interface extending an interface: "extends" with native "interface" (csharp.md, Сопоставление рёбер).
public interface IReadStore
{
}

public interface IWriteStore : IReadStore
{
}
