import { test } from "node:test";
import assert from "node:assert/strict";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const here = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(here, "..", "..");
const cliPath = path.join(packageRoot, "dist", "cli.js");
const testdataRoot = path.join(packageRoot, "testdata", "project");

function runCli(args: string[]) {
  return spawnSync(process.execPath, [cliPath, ...args], { encoding: "utf8" });
}

test("prints valid JSON and exits 0 on a good run", () => {
  const result = runCli(["--root", testdataRoot]);
  assert.equal(result.status, 0);
  const parsed = JSON.parse(result.stdout);
  assert.equal(parsed.language, "typescript");
  assert.ok(Array.isArray(parsed.symbols));
});

test("exits 1 with empty stdout when --root does not exist", () => {
  const result = runCli(["--root", path.join(testdataRoot, "does-not-exist")]);
  assert.equal(result.status, 1);
  assert.equal(result.stdout, "");
  assert.ok(result.stderr.length > 0);
});

test("exits 2 with empty stdout on an unknown flag", () => {
  const result = runCli(["--root", testdataRoot, "--bogus"]);
  assert.equal(result.status, 2);
  assert.equal(result.stdout, "");
});

test("exits 2 when a flag is missing its value", () => {
  const result = runCli(["--root"]);
  assert.equal(result.status, 2);
  assert.equal(result.stdout, "");
});
