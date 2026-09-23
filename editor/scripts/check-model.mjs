#!/usr/bin/env node
/**
 * Model check for diagram projects (contract v3).
 *
 * Reports, per project:
 *   1. stale translations   — `fromHash` no longer matches its source
 *   2. divergences          — two languages both `authored`, neither derived
 *                             from the other: not translations at all
 *   3. missing text         — an id used by the structure has no text
 *   4. views without an axis
 *   5. containment contradictions — one node placed in different containers by
 *                             two views declaring the *same* axis
 *   6. broken codeRef       — a file that is no longer there
 *
 * Plus two cheap ones that catch the same rot earlier: placeholder names (a
 * name equal to the id says nothing) and relation types used but not declared.
 *
 * Exit code is 1 when anything is found, so CI fails on it. This check is the
 * only reason the provenance fields are worth writing: unchecked, they decay
 * into optional fields nobody fills in.
 *
 * See ADR_20260831_diagrams_text-provenance-and-view-axes.
 */

import { readFileSync, existsSync, readdirSync } from "node:fs";
import { join } from "node:path";

// Usage: check-model.mjs <workspace> [source-root]
// <workspace> holds `projects/`; [source-root] is what `codeRef` resolves
// against (defaults to the current directory).
if (!process.argv[2]) {
  console.error("usage: check-model.mjs <workspace> [source-root]");
  process.exit(2);
}
const ROOT = join(process.argv[2], "projects");
const REPO = process.argv[3] ?? ".";
// Kept in step with `TEXT_FIELDS` in src/model/text-provenance.ts by hand:
// this script runs standalone, before any build step, so it cannot import
// the TS module directly. `fromLabel`/`toLabel` are the end-label captions on
// a relation (ADR_20260903 §2.6) — same shape, same provenance mechanism as
// every other field here, so the checks below apply to them without change.
const TEXT_FIELDS = ["name", "title", "description", "doc", "fromLabel", "toLabel"];

const read = (p) => JSON.parse(readFileSync(p, "utf8"));

/**
 * Normalise before hashing, so that a change in how the text is stored is not
 * mistaken for a change in what it says. Must stay in step with whatever writes
 * `fromHash`.
 *
 * Line endings, trailing spaces and runs of blank lines are noise. Indentation
 * is not: descriptions may hold markdown, where leading spaces carry meaning —
 * nesting in a list, lines inside a fenced block. Collapsing those would make a
 * real edit invisible to the staleness check, which is the one thing this
 * function exists to prevent.
 */
const normalise = (s) =>
  s
    .replace(/\r\n/g, "\n")
    .replace(/[ \t]+$/gm, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();

/** FNV-1a, 32 bit. Change detection, not security. */
function hash(text) {
  let h = 0x811c9dc5;
  const s = normalise(text);
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16).padStart(8, "0");
}

const findings = [];
const report = (project, kind, message) => findings.push({ project, kind, message });

/** node id -> { axis -> { container, view } }, gathered across every project. */
const byAxis = new Map();

for (const entry of readdirSync(ROOT, { withFileTypes: true }).filter((d) => d.isDirectory())) {
  const project = entry.name;
  const dir = join(ROOT, project);
  if (!existsSync(join(dir, "project.json"))) continue;

  const manifest = read(join(dir, "project.json"));
  const languages = manifest.languages ?? ["ru"];
  const entities = existsSync(join(dir, "entities.json")) ? read(join(dir, "entities.json")).entities ?? [] : [];
  const relations = existsSync(join(dir, "relations.json")) ? read(join(dir, "relations.json")).relations ?? [] : [];
  const declaredTypes = existsSync(join(dir, "relation-types.json"))
    ? new Set((read(join(dir, "relation-types.json")).relationTypes ?? []).map((t) => t.id))
    : new Set();

  const catalogues = {};
  for (const lang of languages) {
    const path = join(dir, `text.${lang}.json`);
    catalogues[lang] = existsSync(path) ? read(path).entries ?? {} : {};
  }

  // ------------------------------------------------- 1 & 2: provenance health
  for (const lang of languages) {
    for (const [id, record] of Object.entries(catalogues[lang])) {
      for (const field of TEXT_FIELDS) {
        const value = record[field];
        if (!value || typeof value !== "object") continue;

        if (value.origin === "translated") {
          const source = catalogues[value.from]?.[id]?.[field];
          if (!source) {
            report(project, "протух", `${id}.${field}@${lang}: источник ${value.from} исчез`);
          } else if (hash(source.v ?? "") !== value.fromHash) {
            report(project, "протух", `${id}.${field}@${lang}: источник ${value.from} изменился`);
          }
        }

        if (value.origin === "authored" && languages.length > 1) {
          const others = languages.filter((l) => l !== lang);
          for (const other of others) {
            const rival = catalogues[other]?.[id]?.[field];
            if (rival?.origin === "authored" && lang < other) {
              report(
                project,
                "расхождение",
                `${id}.${field}: ${lang} и ${other} написаны независимо, связи между ними нет`,
              );
            }
          }
        }
      }

      const name = record.name?.v;
      if (name && name === id) {
        report(project, "имя-заглушка", `${id}@${lang}: имя совпадает с идентификатором`);
      }
    }
  }

  // ------------------------------------------------------------ 3: недостача
  //
  // Only what is actually authored is expected to carry text. An entity's name
  // is canonical and lives in `entities.json` — it is not translated, and
  // demanding a catalogue entry for each of a thousand generated classes would
  // bury every other finding. Containers, views and relation types are named by
  // a human, so silence there is a real gap.
  const mustBeNamed = new Set(declaredTypes);
  const viewsPath = join(dir, "views");
  const viewFiles = existsSync(viewsPath) ? readdirSync(viewsPath).filter((f) => f.endsWith(".json")) : [];
  for (const file of viewFiles) {
    const view = read(join(viewsPath, file));
    mustBeNamed.add(view.id ?? file.replace(/\.view\.json$/, ""));
    // A zone id is `z_<x>` for the container `c_<x>` it renders; its text is
    // keyed to the container. See the same shim in ProjectStore.
    for (const zone of view.zones ?? []) {
      mustBeNamed.add(zone.id.startsWith("z_") ? "c_" + zone.id.slice(2) : zone.id);
    }
  }

  for (const lang of languages) {
    for (const id of mustBeNamed) {
      const key = declaredTypes.has(id) ? `rt_${id}` : id;
      const record = catalogues[lang][key];
      if (!record || (!record.name && !record.title)) {
        report(project, "недостача", `${key}: нет имени в ${lang}`);
      }
    }
  }

  // ---------------------------------------------------- типы связей объявлены
  for (const relation of relations) {
    const type = relation.type ?? relation.relation;
    if (type && declaredTypes.size > 0 && !declaredTypes.has(type)) {
      report(project, "тип связи", `${relation.id}: тип "${type}" не объявлен в relation-types.json`);
    }
  }

  // ------------------------------------------------------- 4 & 5: виды и оси
  const viewsDir = join(dir, "views");
  if (existsSync(viewsDir)) {
    for (const file of readdirSync(viewsDir).filter((f) => f.endsWith(".json"))) {
      const view = read(join(viewsDir, file));
      // A view says what its containers classify, or the project says what to
      // assume for the views that do not — which is what keeps regenerated
      // views readable without every generator learning about axes.
      const axis = view.axis ?? manifest.defaultAxis;
      if (!axis) {
        report(project, "вид без оси", `${file}: и у проекта нет defaultAxis`);
        continue;
      }
      for (const placement of view.nodes ?? view.placements ?? []) {
        const id = placement.entity ?? placement.id;
        const container = placement.container ?? placement.zone ?? null;
        if (!id || !container) continue;
        if (!byAxis.has(axis)) byAxis.set(axis, new Map());
        const perNode = byAxis.get(axis);
        const seen = perNode.get(id);
        if (seen && seen.container !== container) {
          report(
            project,
            "противоречие",
            `${id} лежит в ${seen.container} (${seen.where}) и в ${container} (${project}/${file}) — одна ось ${axis}`,
          );
        } else if (!seen) {
          perNode.set(id, { container, where: `${project}/${file}` });
        }
      }
    }
  }

  // --------------------------------------------------------------- 6: codeRef
  for (const item of entities) {
    const ref = item.codeRef;
    if (!ref) continue;
    const path = join(REPO, ref.split("#")[0].split(":")[0]);
    if (!existsSync(path)) {
      report(project, "битый codeRef", `${item.id}: ${ref}`);
    }
  }
}

const byKind = new Map();
for (const f of findings) {
  if (!byKind.has(f.kind)) byKind.set(f.kind, []);
  byKind.get(f.kind).push(f);
}

if (findings.length === 0) {
  console.log("Модель схем: замечаний нет.");
  process.exit(0);
}

console.log(`Модель схем: ${findings.length} замечаний\n`);
for (const [kind, items] of byKind) {
  console.log(`${kind} (${items.length})`);
  for (const item of items.slice(0, 15)) console.log(`  ${item.project}: ${item.message}`);
  if (items.length > 15) console.log(`  ... и ещё ${items.length - 15}`);
  console.log();
}
process.exit(1);
