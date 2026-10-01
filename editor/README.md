# editor — `@semaps/editor`

Canvas and editor, TypeScript. Diagrams have fixed coordinates, no auto-layout; the separate
code graph page (`/app/#graph`) is laid out by algorithm on purpose
(`docs/adr/ADR_20260928-2_editor_graph-is-a-separate-form.md`).

```
npm ci
npm run build:app                              # → ../host/app
npm run build                                  # library → dist/
npm run dev                                    # SEMAPS_WORKSPACE=<dir> to override examples/workspace
semaps check <dir or file.semaps>              # model check lives in core/, see ../host
```
