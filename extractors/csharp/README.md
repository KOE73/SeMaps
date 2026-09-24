# extractors/csharp

`semaps-extract-csharp`: prints code facts for a C# workspace per
[`docs/EXTRACTOR.md`](../../docs/EXTRACTOR.md) (output contract),
[`ADR_20260923-5`](../../docs/adr/ADR_20260923-5_extractors_symbol-ids.md) (`id` form),
[`ADR_20260923-10`](../../docs/adr/ADR_20260923-10_extractors_csharp-assembly-symbol.md) (project
symbol) and
[`schemas/extractor-facts.schema.json`](../../schemas/extractor-facts.schema.json) (schema).
Spec and steps: [`PLAN_20260923_extractors_csharp`](../../docs/plans/PLAN_20260923_extractors_csharp.md).

Built with Roslyn (`Microsoft.CodeAnalysis.Workspaces.MSBuild` + `Microsoft.Build.Locator`):
resolved symbols, not text/regex matching. Target framework: `net10.0`.

## Install

It ships with `semaps` in `extractors/csharp/` and is run by `semaps sync` / `semaps extract`
([ADR_20260924-3](../../docs/adr/ADR_20260924-3_host_tools-beside-the-binary-and-setup.md)):
`install.cmd` in the repository root publishes it there, the release archives carry it. Nothing to install by hand.

It stays a program of its own — facts on stdout — and can be run alone:

```sh
dotnet publish src/SeMaps.Extract.CSharp -c Release -o <folder>
<folder>/semaps-extract-csharp --root <dir> --include src > facts.json
```

As a global dotnet tool, if you want the command on PATH (`semaps` finds it there too):

```sh
dotnet pack src/SeMaps.Extract.CSharp -c Release -o ./nupkg
dotnet tool install -g --add-source ./nupkg SeMaps.Extract.CSharp
```

## Run without installing

```sh
dotnet run --project src/SeMaps.Extract.CSharp -- --root <dir> [--include <path>]... [--exclude <glob>]...
```

## Build, test, pack

```sh
dotnet build SeMaps.Extract.CSharp.slnx
dotnet test SeMaps.Extract.CSharp.slnx
dotnet pack src/SeMaps.Extract.CSharp -c Release -o ./nupkg
```

`tests/SeMaps.Extract.CSharp.Tests` builds the tool, then runs it as a subprocess against
`testdata/Sample` (two projects joined by a `ProjectReference`, plus `Shared/BuildInfo.cs`
linked into both) and checks: output equals
`testdata/Sample/expected.json` byte for byte, output validates against
`schemas/extractor-facts.schema.json`, two runs produce byte-identical output, and the exit
codes from `docs/EXTRACTOR.md` §1 (`1` for a missing `--root`, `2` for an unknown flag).
`testdata/Sample` is a real SDK-style solution: the first test run restores it (`dotnet restore
Sample.slnx`), which needs network access once; after that, `dotnet test` needs none.

## Measured on a real solution

Run against a real consumer solution (7 projects, ~390 types): 394 symbols, edges — 548
`references`, 383 `contains`, 118 `extends`, 107 `implements`; exit `0` in ~7 s. `references`
stays in the hundreds, not thousands, for a solution this size (see the plan's "Открытое" §1).

## What it prints

- **Project** (`.csproj` under `--root` that passes `--include`/`--exclude`): one symbol,
  `kind: "module"`, `nativeKind: "assembly"`, `id` = `[AssemblyName]` (brackets as in IL, so it
  never collides with a namespace of the same name), `name` = `AssemblyName`, `file` = the
  `.csproj` path. Edges: `contains` → every top-level type declared in its sources (a file
  linked into two projects gives `contains` from both), `references` → every
  `ProjectReference` target that is also in the output. Multi-targeted projects merge into
  one symbol.
- **Namespace**: `module`/`namespace`, `contains` → its top-level types.
- **Types** of every visibility (`type`/`interface`/`function`), `contains` outer → nested,
  `extends`, `implements`, `references`.

```json
{ "id": "[Sample.Core]", "kind": "module", "nativeKind": "assembly", "name": "Sample.Core",
  "namespace": "", "file": "Sample.Core/Sample.Core.csproj" }
{ "from": "[Sample.Core]", "to": "Sample.Core.Container", "kind": "contains" }
{ "from": "[Sample.App]", "to": "[Sample.Core]", "kind": "references" }
```

## Scope notes

- Only `--root`, `--include`, `--exclude` per `docs/EXTRACTOR.md` §1. What gets opened: a
  single `*.sln`/`*.slnx` found under the search roots (`--include` paths, or `--root` if none
  were given), else every `*.csproj` found there.
- Default exclusions and the symbol/member/edge rules are in
  `PLAN_20260923_extractors_csharp.md`; open questions the plan left for this implementation are
  in that file's "Открытое" section.
