// One row per line of the normative table "Сопоставление рёбер" in docs/extractors/typescript.md
// (ADR_20260927 §4). A row changed in the table without the code, or the other way round, fails here.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import { extract } from "../extract.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const testdataRoot = path.resolve(here, "..", "..", "testdata", "project");

// [construct, from, to, kind, native]
const rows: [string, string, string, string, string | undefined][] = [
  ["class extends class", "a#Widget", "a#Base", "extends", "class"],
  ["interface extends interface", "mapping#Stream", "mapping#Readable", "extends", "interface"],
  ["class implements interface", "a#Widget", "a#Greeter", "implements", "implements"],
  ["file -> declaration", "a", "a#Base", "contains", "file"],
  ["namespace -> declaration", "a#Registry", "a#Registry.Entry", "contains", "namespace"],
  ["class field", "a#Registry.Store", "a#Registry.Entry", "holds", "field"],
  ["interface property", "a#Registry.Entry", "a#Box", "holds", "property"],
  ["constructor parameter property", "comprehensive#ParameterProperty", "comprehensive#Container", "holds", "parameterProperty"],
  ["function parameter", "a#helper", "a#Box", "uses", "parameter"],
  ["return type", "a#build", "a#Widget", "uses", "return"],
  ["constructor parameter", "comprehensive#Constructor", "comprehensive#Container", "uses", "constructor"],
  ["type alias", "comprehensive#ContainerBox", "comprehensive#Box", "uses", "alias"],
  ["annotated value", "mapping#defaultStream", "mapping#Stream", "uses", "annotation"],
];

const facts = extract({ root: testdataRoot, include: [], exclude: [], edges: ["holds", "uses"] });

for (const [construct, from, to, kind, native] of rows) {
  test(`edge mapping: ${construct} -> ${kind}/${native ?? "-"}`, () => {
    const edges = facts.edges.filter((e) => e.from === from && e.to === to && e.kind === kind);
    assert.ok(edges.length > 0, `no ${kind} edge ${from} -> ${to}`);
    assert.ok(edges.some((e) => e.native === native), `natives: ${edges.map((e) => e.native).join(",")}`);
  });
}
