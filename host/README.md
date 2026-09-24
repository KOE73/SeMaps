# host

`semaps` — one binary: the editor (`app/`) and the defaults (`defaults/`) are embedded into it.
The check and the sync with code in [`../core`](../core) are part of the same binary
(`semaps check`, `semaps sync`).

## Everyday use

Once per machine:

```
host\install.cmd
```

It builds the editor bundle if it is missing (needs npm), builds `semaps.exe` and runs
`semaps install`: the exe copies itself to `%LOCALAPPDATA%\Programs\SeMaps`, adds that folder to
the user `PATH` and associates `*.semaps` with itself. A downloaded exe from GitHub Releases does
the same when double-clicked (it asks first) or with `semaps.exe install`.

Once per project — a project file in its root, e.g. `myproject.semaps`:

```yaml
version: 1
name: My project
workspace: docs/diagrams   # default
source_root: .             # default: the folder of this file
port: 8777                 # default; the next free one is taken if busy
```

Then either press Enter / double-click the `.semaps` file, or type `semaps` anywhere inside the
project — it walks up to the first `*.semaps`. Without a project file there is nothing to open
(pass `--workspace` explicitly); the host no longer guesses a workspace from marker files.

```
semaps [dir | file.semaps]
semaps --workspace <dir> --source-root <dir> --port 9000 --no-browser
semaps --here                                  # keep the server in this console
semaps install                                 # Windows: copy to a stable folder, PATH, *.semaps
semaps check [dir | file.semaps]               # model check, exit 1 on findings
semaps doctor [dir | file.semaps]              # extractors and runtimes found for the .semaps file
semaps extract [--extractor <id>] [dir | file.semaps]
                                               # run the extractors, keep the runs
semaps sync [--extractor <id>] [--run <id>] [--dry-run] [--no-renames] [dir | file.semaps]
                                               # extract and reconcile the registry (docs/EXTRACTOR.md §5)
semaps sync --facts facts.json [--project <id>] [--dry-run] [--no-renames] [dir | file.semaps]
                                               # the same from a facts file made by hand
```

Flags go before the project argument; `--facts -` reads stdin. Extractors are found beside the
binary in `extractors/<language>/` (ADR_20260924-3); `install.cmd` builds them there. The pages
`/extract` and `/setup` do the same from the browser (docs/API.md §5).

## Development

`app/` is the built editor (`cd ../editor && npm run build:app`). It is **not in git**
([`ADR_20260923-3`](../docs/adr/ADR_20260923-3_build_bundle-from-ci-not-git.md)): build it once, and
again after changing the editor. `go.mod` sits in the repository root, so build from there
(`go build ./host`) or from here (`go build .`). `run.cmd` runs from source on
`examples/example.semaps`.

API and the project file format: [docs/API.md](../docs/API.md).
