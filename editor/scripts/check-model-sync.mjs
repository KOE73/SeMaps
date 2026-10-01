#!/usr/bin/env node
/**
 * Self-check for what `diffModel` (`src/editor/io/ModelSync.ts`) writes about a
 * view's lines (ADR_20260930-7): the `edges` value is an overlay of own fields
 * only, hiding a line is a change of `relations.except`, a drawn line brings its
 * relation. Same esbuild-bundle-then-run harness as `check-nudge.mjs`.
 */

import { build } from "esbuild";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const ROOT = new URL("..", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

async function loadModule() {
  const result = await build({
    entryPoints: [join(ROOT, "src/editor/io/ModelSync.ts")],
    bundle: true,
    write: false,
    format: "esm",
    platform: "neutral",
    target: "es2022",
  });
  const dir = mkdtempSync(join(tmpdir(), "semaps-modelsync-"));
  const file = join(dir, "ModelSync.mjs");
  writeFileSync(file, result.outputFiles[0].text, "utf8");
  return import(pathToFileURL(file).href);
}

const { diffModel } = await loadModule();

let failures = 0;
const fail = (name, detail) => { failures++; console.log(`[FAIL] ${name} :: ${detail}`); };
const ok = (name) => console.log(`[ok] ${name}`);
const clone = (v) => JSON.parse(JSON.stringify(v));

// A view of 6 blocks and 30 registry relations between them, all drawn.
const ids = ["e_a", "e_b", "e_c", "e_d", "e_e", "e_f"];
const placements = ids.map((id, i) => ({ id, container: false, label: id, type: "class", parent: null, x: i * 200, y: 0, width: 170, height: 50 }));
const relations = [];
for (let i = 0; i < 30; i++) {
  relations.push({ id: `r_${i}`, from: ids[i % 6], to: ids[(i + 1 + (i % 4)) % 6], type: "uses", origin: "code" });
}
const wireEdge = (r) => ({ id: r.id, from: r.from, to: r.to, type: r.type, label: "", origin: r.origin });
const doc = (edges, extra = {}) => ({
  metadata: {},
  placements: clone(placements),
  edges,
  views: [],
  bundle: { relations: { relations: clone(relations) }, view: { id: "v" } },
  ...extra,
});
const viewFile = (extra = {}) => ({ id: "v", project: "p", placements: [], ...extra });
const nothingUnsaved = { entity: new Set(), relation: new Set() };
const run = (before, after, view, raw = relations, unsaved = nothingUnsaved, rawEntities = []) =>
  diffModel(before, after, view, rawEntities, {}, raw, unsaved);
const viewOp = (ops) => ops.find((o) => o.kind === "view")?.value;

// (a) routing of one line of many: exactly one entry, no from/to/type, no relations.
{
  const before = doc(relations.map(wireEdge));
  const after = clone(before);
  after.edges[3].routing = "straight";
  const value = viewOp(run(before, after, viewFile({ relations: { default: "visible" } })));
  const edges = value?.edges;
  if (!Array.isArray(edges) || edges.length !== 1) fail("routing on one line", `edges = ${JSON.stringify(edges)}`);
  else if (JSON.stringify(edges[0]) !== JSON.stringify({ id: "r_3", routing: "straight" })) fail("routing on one line", JSON.stringify(edges[0]));
  else if ("relations" in value) fail("routing on one line", "relations written");
  else ok("routing on one line: one overlay entry, no from/to/type");
}

// (a2) a second look on another line keeps the first entry of the file, in its place.
{
  const file = viewFile({ edges: [{ id: "r_7", styleId: "relation.fancy" }] });
  const before = doc(relations.map((r) => (r.id === "r_7" ? { ...wireEdge(r), styleId: "relation.fancy" } : wireEdge(r))));
  const after = clone(before);
  after.edges[2].routing = "curved";
  const edges = viewOp(run(before, after, file))?.edges;
  const got = JSON.stringify(edges);
  if (got !== JSON.stringify([{ id: "r_7", styleId: "relation.fancy" }, { id: "r_2", routing: "curved" }])) fail("second look", got);
  else ok("a second look is added beside the file's entry");
}

// (b) nothing own left: the key is removed (`null`, as the host applies it).
{
  const file = viewFile({ edges: [{ id: "r_3", routing: "straight" }] });
  const before = doc(relations.map((r) => (r.id === "r_3" ? { ...wireEdge(r), routing: "straight" } : wireEdge(r))));
  const after = clone(before);
  delete after.edges[3].routing;
  const value = viewOp(run(before, after, file));
  if (!value || value.edges !== null) fail("nothing own left", `edges = ${JSON.stringify(value?.edges)}`);
  else ok("nothing own left: edges: null removes the key");
}

// (c) a hidden line: except changes, no edges written.
{
  const before = doc(relations.map(wireEdge));
  const after = clone(before);
  after.edges.splice(5, 1);
  const value = viewOp(run(before, after, viewFile({ relations: { default: "visible" } })));
  if (!value) fail("hide a line", "no view op");
  else if ("edges" in value) fail("hide a line", "edges written");
  else if (JSON.stringify(value.relations) !== JSON.stringify({ default: "visible", except: ["r_5"] })) fail("hide a line", JSON.stringify(value.relations));
  else ok("hide a line: relations.except, no edges");
}

// (c2) show it again: except loses the entry.
{
  const before = doc(relations.filter((r) => r.id !== "r_5").map(wireEdge));
  const after = doc(relations.map(wireEdge));
  const value = viewOp(run(before, after, viewFile({ relations: { default: "visible", except: ["r_5"] } })));
  if (!value || JSON.stringify(value.relations) !== JSON.stringify({ default: "visible", except: [] }) || "edges" in value) fail("show a line", JSON.stringify(value));
  else ok("show a line: except emptied");
}

// (c3) a line whose end left the view is not the view's to decide: nothing written.
{
  const before = doc(relations.map(wireEdge));
  const after = clone(before);
  after.placements = after.placements.filter((p) => p.id !== "e_a");
  after.edges = after.edges.filter((e) => e.from !== "e_a" && e.to !== "e_a");
  const ops = run(before, after, viewFile({ relations: { default: "visible" } }));
  if (viewOp(ops)) fail("end removed", JSON.stringify(viewOp(ops)));
  else ok("an end removed from the view writes no view op");
}

// (d) a line drawn by hand: a relation op plus, on a `hidden` view, its except entry; the overlay only if it has a look.
{
  const before = doc(relations.map(wireEdge));
  const after = clone(before);
  const drawn = { id: "r_a_b_calls", from: "e_a", to: "e_b", type: "calls", origin: "authored", status: "present" };
  after.bundle.relations.relations.push(drawn);
  after.edges.push({ id: drawn.id, from: "e_a", to: "e_b", type: "calls", label: "", origin: "authored", styleId: "relation.fancy" });
  const ops = run(before, after, viewFile({ relations: { default: "visible" } }));
  const rel = ops.find((o) => o.kind === "relation");
  const value = viewOp(ops);
  if (!rel || JSON.stringify(rel.value) !== JSON.stringify({ id: "r_a_b_calls", from: "e_a", to: "e_b", type: "calls", origin: "authored", status: "present" })) fail("drawn line", `relation op ${JSON.stringify(rel)}`);
  else if (JSON.stringify(value?.edges) !== JSON.stringify([{ id: "r_a_b_calls", styleId: "relation.fancy" }])) fail("drawn line", JSON.stringify(value));
  else if ("relations" in value) fail("drawn line", "except written for a visible-by-default line");
  else ok("drawn line: relation op + own-look entry, no except on a visible view");

  const hiddenView = viewFile({ relations: { default: "hidden" } });
  const beforeH = doc([]);
  const afterH = clone(beforeH);
  afterH.bundle.relations.relations.push(drawn);
  afterH.edges.push({ id: drawn.id, from: "e_a", to: "e_b", type: "calls", label: "", origin: "authored" });
  const opsH = run(beforeH, afterH, hiddenView);
  const v = viewOp(opsH);
  if (JSON.stringify(v?.relations) !== JSON.stringify({ default: "hidden", except: ["r_a_b_calls"] }) || "edges" in v) fail("drawn line on a hidden view", JSON.stringify(v));
  else ok("drawn line on a view hidden by default: except entry, no overlay");
}

// (e) a drawn line undone: the relation stays in the registry, hidden on this view.
{
  const drawn = { id: "r_a_b_calls", from: "e_a", to: "e_b", type: "calls", origin: "authored", status: "present" };
  const before = doc(relations.map(wireEdge));
  before.bundle.relations.relations.push(drawn);
  before.edges.push({ id: drawn.id, from: "e_a", to: "e_b", type: "calls", label: "", origin: "authored" });
  const after = doc(relations.map(wireEdge));
  const ops = run(before, after, viewFile({ relations: { default: "visible" } }), [...relations, drawn]);
  if (ops.some((o) => o.kind === "relation")) fail("undone line", "relation op sent");
  else if (JSON.stringify(viewOp(ops)?.relations) !== JSON.stringify({ default: "visible", except: ["r_a_b_calls"] })) fail("undone line", JSON.stringify(viewOp(ops)));
  else ok("undone drawn line: hidden by except, relation not deleted");
}

// (e2) draw -> undo -> redo, each step a separate commit against the baseline the previous one left
// (ADR_20260930-8). The line is an unsaved creation: undo withdraws the relation (`value: null`) and
// leaves no `except` / `edges` residue on the view; redo creates the same id again.
{
  const drawn = { id: "r_a_b_calls", from: "e_a", to: "e_b", type: "calls", origin: "authored", status: "present" };
  const withLine = () => {
    const d = doc(relations.map(wireEdge));
    d.bundle.relations.relations.push(clone(drawn));
    d.edges.push({ id: drawn.id, from: "e_a", to: "e_b", type: "calls", label: "", origin: "authored" });
    return d;
  };
  const clean = doc(relations.map(wireEdge));
  const file0 = viewFile({ relations: { default: "visible" } });
  const unsavedDrawn = { entity: new Set(), relation: new Set([drawn.id]) };
  // Draw and undo inside one batch (nothing was sent between): nothing at all.
  const empty = run(clean, clone(clean), file0);
  if (empty.length !== 0) fail("draw+undo in one batch", JSON.stringify(empty));
  else ok("draw and undo before any send: no op");
  const drawOps = run(clean, withLine(), file0);
  const undoOps = run(withLine(), clean, file0, [...relations, drawn], unsavedDrawn);
  const rel = undoOps.filter((o) => o.kind === "relation");
  if (!drawOps.some((o) => o.kind === "relation" && o.id === drawn.id)) fail("draw", "no relation op");
  else if (rel.length !== 1 || rel[0].id !== drawn.id || rel[0].value !== null) fail("undo withdraws", JSON.stringify(undoOps));
  else if (viewOp(undoOps) !== undefined) fail("undo leaves no residue", JSON.stringify(viewOp(undoOps)));
  else ok("draw -> undo: the relation is withdrawn, the view is not written");

  // The view names the line in `except` and `edges` (a look, a shown exception); the undo takes both away
  // and leaves what is not the line's (an entry of another relation, an except of a relation this view cannot decide).
  const marked = viewFile({ relations: { default: "visible", except: [drawn.id, "r_x"] }, edges: [{ id: drawn.id, styleId: "relation.fancy" }, { id: "r_0", routing: "straight" }] });
  const cleanLook = doc(relations.map((r) => (r.id === "r_0" ? { ...wireEdge(r), routing: "straight" } : wireEdge(r))));
  const withLook = withLine();
  withLook.edges.find((e) => e.id === "r_0").routing = "straight";
  const undoMarked = run(withLook, cleanLook, marked, [...relations, drawn], unsavedDrawn);
  const v = viewOp(undoMarked);
  if (JSON.stringify(v?.relations) !== JSON.stringify({ default: "visible", except: ["r_x"] })) fail("undo clears except", JSON.stringify(v));
  else if (JSON.stringify(v?.edges) !== JSON.stringify([{ id: "r_0", routing: "straight" }])) fail("undo clears edges", JSON.stringify(v));
  else if (!undoMarked.some((o) => o.kind === "relation" && o.id === drawn.id && o.value === null)) fail("undo clears except", "no withdrawal");
  else ok("draw -> undo on a hidden view: withdrawn, its except and edges mentions removed, others kept");

  // The look was its only edges entry: the key goes away.
  const only = viewFile({ relations: { default: "visible" }, edges: [{ id: drawn.id, routing: "curved" }] });
  const undoOnly = run(withLine(), clean, only, [...relations, drawn], unsavedDrawn);
  if (viewOp(undoOnly)?.edges !== null) fail("undo, last edges entry", JSON.stringify(viewOp(undoOnly)));
  else ok("draw -> undo: the last edges entry goes with the key");

  // Redo: from the baseline the undo left, the same id is created again.
  const redoOps = run(clean, withLine(), file0, relations);
  const again = redoOps.find((o) => o.kind === "relation");
  if (again?.id !== drawn.id || again.value === null) fail("redo", `relation op ${JSON.stringify(again)}`);
  else ok("draw -> undo -> redo: the same id is created again");
}

// (e3) draw -> save -> delete: after the save the line is a saved record — hidden through except, never withdrawn.
{
  const drawn = { id: "r_a_b_calls", from: "e_a", to: "e_b", type: "calls", origin: "authored", status: "present" };
  const before = doc(relations.map(wireEdge));
  before.bundle.relations.relations.push(drawn);
  before.edges.push({ id: drawn.id, from: "e_a", to: "e_b", type: "calls", label: "", origin: "authored" });
  const after = doc(relations.map(wireEdge)); // the line deleted; the relation is still in the registry
  after.bundle.relations.relations.push(drawn);
  const ops = run(before, after, viewFile({ relations: { default: "visible" } }), [...relations, drawn]); // not unsaved
  if (ops.some((o) => o.value === null && o.kind !== "placement")) fail("delete a saved line", JSON.stringify(ops));
  else if (JSON.stringify(viewOp(ops)?.relations) !== JSON.stringify({ default: "visible", except: [drawn.id] })) fail("delete a saved line", JSON.stringify(ops));
  else ok("draw -> save -> delete: hidden through except, no withdrawal");
  // Even if the registry lost it in the editor's model (an undo across a save), a saved record is not in `unsaved`: no null.
  const gone = run(before, doc(relations.map(wireEdge)), viewFile({ relations: { default: "visible" } }), [...relations, drawn]);
  if (gone.some((o) => o.kind === "relation")) fail("saved record, undone", JSON.stringify(gone));
  else ok("a saved relation is never withdrawn, even when the model lost it");
}

// (e4) blocks: an entity drawn in the editor and taken off the view is withdrawn with its unsaved lines;
// a saved one only leaves the view.
{
  const block = { id: "e_new", container: false, label: "New", type: "class", parent: null, x: 0, y: 200, width: 170, height: 50 };
  const withBlock = () => { const d = doc(relations.map(wireEdge)); d.placements.push(clone(block)); return d; };
  const line = { id: "r_e_new_e_a_uses", from: "e_new", to: "e_a", type: "uses", origin: "authored", status: "present" };
  const clean = doc(relations.map(wireEdge));
  const withBoth = withBlock();
  withBoth.bundle.relations.relations.push(clone(line));
  withBoth.edges.push({ id: line.id, from: "e_new", to: "e_a", type: "uses", label: "", origin: "authored" });
  const rawEntities = [{ id: "e_new", kind: "class", origin: "authored", status: "present" }];
  const unsaved = { entity: new Set(["e_new"]), relation: new Set([line.id]) };
  const ops = run(withBoth, clean, viewFile({ relations: { default: "visible" } }), [...relations, line], unsaved, rawEntities);
  const nulls = ops.filter((o) => o.value === null).map((o) => `${o.kind}:${o.id}`).sort();
  if (JSON.stringify(nulls) !== JSON.stringify(["entity:e_new", "placement:e_new", `relation:${line.id}`])) fail("drawn block undone", JSON.stringify(ops));
  else ok("draw block + line -> undo: entity, its line and its placement are withdrawn together");

  const saved = run(withBlock(), clean, viewFile({ relations: { default: "visible" } }), relations, nothingUnsaved, rawEntities);
  if (saved.some((o) => o.kind === "entity" || o.kind === "relation") || !saved.some((o) => o.kind === "placement" && o.value === null)) fail("saved block removed", JSON.stringify(saved));
  else ok("a saved block taken off the view stays in the registry");

  const redo = run(clean, withBlock(), viewFile({ relations: { default: "visible" } }), relations, nothingUnsaved, []);
  const e = redo.find((o) => o.kind === "entity");
  if (e?.id !== "e_new" || e.value === null) fail("redo block", JSON.stringify(redo));
  else ok("draw block -> undo -> redo: the same entity id is created again");
}

// (f) nothing changed: no view op.
{
  const before = doc(relations.map(wireEdge));
  if (viewOp(run(before, clone(before), viewFile()))) fail("no change", "view op sent");
  else ok("no change, no view op");
}

console.log(failures === 0 ? "\nAll cases clean." : `\n${failures} FAILURES`);
process.exit(failures === 0 ? 0 : 1);
