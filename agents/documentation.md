# Documentation layout

Documents are separated by **how
long they stay true**, not by topic.

## Folders and genres

| Path | Prefix | Contents | Lifecycle |
|---|---|---|---|
| `agents/` | — | agent-facing rules. **English only.** | must match the code; a mismatch is a bug |
| `docs/` | — | normative contracts: `CONTRACT.md`, `API.md`, `EXTRACTOR.md` | edited together with the code they govern |
| `docs/adr/` | `ADR_` | decisions: what was chosen and why, including what was rejected | **never edited** |
| `docs/plans/` | `PLAN_` | work plans, with a status line | edited as work proceeds, closed when done |
| `docs/ideas/` | `IDEA_` | ideas, "would be nice" notes | become a plan or die |
| `docs/reviews/` | date | outside-eye audits of the code (see below) | **never edited** |

File name: `GENRE_YYYYMMDD_zone_short-name.md`. Two documents on one day → `YYYYMMDD-2`.

Both the folder and the prefix carry the genre: the folder keeps the store tidy, the prefix keeps
the meaning attached to the file when the path is not visible (search results, diffs, links).

**Zone** is a closed list, one per top-level part of the repository: `editor`, `host`, `core`,
`extractors` (refined by language: `extractors_csharp`), `schemas`, `contract`, `docs`, `build`.
`diagrams` is the historical zone of the first ADRs; new ADRs use the list above.
A new zone is added by editing this file in its own commit, not on impulse.

## Rules that matter

- **Never edit an `ADR_`.** A changed decision gets a *new* record that says "supersedes
  ADR_…"; the old one gets at most a single line pointing at its replacement.
- **Status** appears only in `ADR_` (accepted / superseded by …) and `PLAN_` (in progress /
  closed). Genre and date are already in the file name.
- **The word `DESIGN` is not used.** It meant intent, description and plan at once.
- **How something works now belongs in `agents/` or in the normative `docs/*.md`**, never in an
  ADR. Those are the only documents obliged to track the code.
- A change to the workspace file format, the host API or the extractor output updates
  `CONTRACT.md` / `API.md` / `EXTRACTOR.md` **in the same commit**.
- Move documents with `git mv` so history survives.
- Before picking a `-N` slot for a same-day document, check `git log --all`, not just the current
  branch: two branches can both land on `-2` and git merges that silently.

## Reviews: the outside eye

A file in `docs/reviews/` is produced by pointing a strong model at **the code and nothing else** —
no `agents/`, no ADRs, no plans. The missing context is the point: only someone who does not know
that something was "on purpose" will notice that the purpose was bad.

A review is a list of observations, never a verdict. A second pass, with documentation at hand,
sorts each finding into: **real defect** (fix, or file a `PLAN_`/`IDEA_`); **deliberate** (point at
the ADR — and if there is none, write it); **rethink** (new ADR superseding the old one). Never edit
a review. If the same finding keeps landing in "deliberate", the code fails to communicate its
intent — fix the opacity, not the finding.

## Language

Rules in `agents/` — English. ADRs, plans, ideas and the normative docs may be Russian, as the
imported ones are. Commit subjects, branch and PR titles — English.
