# extractors/typescript

`semaps-extract-typescript`: prints [`EXTRACTOR.md`](../../docs/EXTRACTOR.md) facts for a
TypeScript source tree. Spec and steps:
[`PLAN_20260923_extractors_typescript`](../../docs/plans/PLAN_20260923_extractors_typescript.md).
Symbol `id` form: [`ADR_20260923-5`](../../docs/adr/ADR_20260923-5_extractors_symbol-ids.md).

Uses the TypeScript compiler API (`ts.createProgram` + `TypeChecker`) for resolved symbols,
not text/regex matching. Only runtime dependency: `typescript`.

## Build

```sh
npm install
npm run build
```

Produces `dist/cli.js` (and the rest of `dist/`).

## Usage

```sh
node dist/cli.js [--root <dir>] [--include <path>]... [--exclude <glob>]...
```

- `--root` — source root; all `file` paths in the output are relative to it. Default: cwd.
- `--include` — paths under `--root` to read (files or directories). Omitted → whole root.
- `--exclude` — glob (`*`, `**`, `?`) to skip on top of the language defaults
  (`node_modules/`, `dist/`, `build/`, `*.d.ts`, `*.test.ts`, `*.spec.ts`, `vite-env.d.ts`,
  anything outside `.ts`/`.tsx`/`.mts`).

stdout is the JSON document from [`EXTRACTOR.md`](../../docs/EXTRACTOR.md) §2, nothing else.
Diagnostics go to stderr. Exit codes: `0` printed, `1` extraction failed (stdout empty), `2`
bad arguments.

If `<root>/tsconfig.json` exists, its `compilerOptions` are used to build the program;
otherwise a fixed default (`ES2022`, DOM+ES2022 libs, `react-jsx`, `bundler` resolution).

Installed as a package, the same binary is available as `semaps-extract-typescript` (see
`bin` in `package.json`).

## Test

```sh
npm test
```

Runs `node --test` over `dist/test/`: output equals `testdata/expected.json`, output validates
against `schemas/extractor-facts.schema.json` (via `ajv`), two runs are byte-identical, and the
CLI's exit codes are checked directly (bad `--root` → 1 with empty stdout; unknown flag → 2).

`testdata/project/` is a two-file sample exercising: interface, class with `extends` +
`implements`, a non-exported class (must not appear), `enum`, generic `type` alias whose
reference resolves through the type argument, a plain function, a `const` value, an arrow
function value, a `namespace` with nested exported declarations, a late `export { … }` list,
and a re-export (`export { Box } from "./a"`, which must not create its own symbol) together
with a cross-file `references` edge.

## `npm run self`

Runs the extractor on the real SeMaps editor sources: `--root ../..`
`--include editor/src`. Exits `0` with no stderr output. Numbers from the current run:

| | |
|---|---|
| symbols | 432 (module 105, interface 125, type 93, function 84, value 25) |
| edges | 656 (contains 327, references 306, implements 17, extends 6) |
| output size | 353 347 bytes |

## Open questions

Tracked in the plan's [«Открытое»](../../docs/plans/PLAN_20260923_extractors_typescript.md#открытое)
section, not here.
