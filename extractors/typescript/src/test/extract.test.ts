import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import { Ajv2020 } from "ajv/dist/2020.js";
import { extract } from "../extract.js";

const here = path.dirname(fileURLToPath(import.meta.url));
// dist/test -> ../.. is the extractor package root
const packageRoot = path.resolve(here, "..", "..");
const testdataRootAbs = path.join(packageRoot, "testdata", "project");
// Passed to extract() the same way the CLI would receive it from the package root,
// so the output's `root` field matches testdata/expected.json byte for byte.
const testdataRoot = path.relative(process.cwd(), testdataRootAbs).split(path.sep).join("/") || ".";
const expectedPath = path.join(packageRoot, "testdata", "expected.json");
const schemaPath = path.resolve(packageRoot, "..", "..", "schemas", "extractor-facts.schema.json");

test("matches testdata/expected.json", () => {
  const facts = extract({ root: testdataRoot, include: [], exclude: [] });
  const expected = JSON.parse(fs.readFileSync(expectedPath, "utf8"));
  assert.deepStrictEqual(facts, expected);
});

test("output validates against extractor-facts.schema.json", () => {
  const facts = extract({ root: testdataRoot, include: [], exclude: [] });
  const schema = JSON.parse(fs.readFileSync(schemaPath, "utf8"));
  const ajv = new Ajv2020({ strict: false });
  const validate = ajv.compile(schema);
  const ok = validate(facts);
  assert.equal(ok, true, JSON.stringify(validate.errors));
});

test("two runs produce byte-identical output", () => {
  const a = extract({ root: testdataRoot, include: [], exclude: [] });
  const b = extract({ root: testdataRoot, include: [], exclude: [] });
  assert.equal(JSON.stringify(a, null, 2), JSON.stringify(b, null, 2));
});

test("symbols are sorted by id and edges by (from, to, kind)", () => {
  const facts = extract({ root: testdataRoot, include: [], exclude: [] });
  const ids = facts.symbols.map((s) => s.id);
  assert.deepStrictEqual(ids, [...ids].sort());
  const edgeKeys = facts.edges.map((e) => `${e.from}\u0000${e.to}\u0000${e.kind}`);
  assert.deepStrictEqual(edgeKeys, [...edgeKeys].sort());
});
