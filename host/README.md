# host

Serves the editor, one workspace and a source tree. Go, no dependencies.

```
run.cmd                                   # opens examples/workspace
run.cmd --workspace <dir> --source-root <dir>
go run . --workspace <dir> [--source-root <dir>] [--port 8777]
```

- `app/` — built editor (`cd ../editor && npm run build:app`), committed so the host runs without Node.
- `defaults/` — `styles.json`, `templates.json`, `content/`; a workspace copy overrides them.

API: [docs/API.md](../docs/API.md). Why three roots: [ADR_20260923](../docs/adr/ADR_20260923_host_workspace-tool-source-roots.md).
