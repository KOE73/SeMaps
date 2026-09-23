# host

`semaps` — one binary: the editor (`app/`) and the defaults (`defaults/`) are embedded into it.

## Everyday use

Once per machine:

```
host\install.cmd
```

It puts `semaps.exe` into `%USERPROFILE%\go\bin` (on PATH) and associates `*.semaps` with it.

Once per project — a project file in its root, e.g. `myproject.semaps`:

```yaml
version: 1
name: My project
workspace: docs/diagrams   # default
source_root: .             # default: the folder of this file
port: 8777                 # default; the next free one is taken if busy
```

Then either press Enter / double-click the `.semaps` file, or type `semaps` anywhere inside the
project — it walks up to the first `*.semaps`. Without a project file it falls back to the first
`catalog.json` or `docs/diagrams/catalog.json` upward, with the repository root as source root.

```
semaps [dir | file.semaps]
semaps --workspace <dir> --source-root <dir> --port 9000 --no-browser
semaps --here                                  # keep the server in this console
```

## Development

`app/` is the built editor (`cd ../editor && npm run build:app`), committed so the host builds
without Node. After rebuilding it or changing `defaults/`, run `install.cmd` again. `run.cmd` runs
from source on `examples/example.semaps`.

API and the project file format: [docs/API.md](../docs/API.md).
