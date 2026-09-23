# SeMaps — agent rules

Layout and purpose: [README.md](README.md).

## Never optional

- **Git: do not commit, amend, push, merge, tag or reset unless the user asked for it in the
  message you are answering.** Stage explicit paths, never `git add -A` / `git add .`.
- **Commit subjects, branch and PR titles are English.**
- **An `ADR_` is never edited**; a changed decision gets a new ADR. → [agents/documentation.md](agents/documentation.md)
- **A consuming project keeps only its workspace.** Nothing in `editor/`, `host/` or `core/` may
  assume a particular workspace or source tree; both arrive as arguments.

## Read before you act

| When you are about to… | Read |
|---|---|
| create or move a file under `docs/`, write an ADR/plan/idea | [agents/documentation.md](agents/documentation.md) |
| change a workspace file's shape | [docs/CONTRACT.md](docs/CONTRACT.md) |
| change what the host serves or accepts | [docs/API.md](docs/API.md) |
| write or change an extractor | [docs/EXTRACTOR.md](docs/EXTRACTOR.md) |
| change installing, the `.semaps` file or `semaps check` | also update [docs/ADOPTING.md](docs/ADOPTING.md) |
