import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";

const read = (path) => JSON.parse(readFileSync(new URL(path, import.meta.url), "utf8"));
// Conditional branches refer to properties declared once on the common envelope.
const validate = new Ajv2020({ strict: true, strictRequired: false }).compile(read("../schemas/message.schema.json"));
test("request and response fixtures satisfy the common envelope schema", () => {
  for (const name of ["initialize", "initialized"]) {
    assert.equal(validate(read(`../examples/${name}.json`)), true, JSON.stringify(validate.errors));
  }
});
test("responses require exactly one result or error and cannot be requests", () => {
  const reply = read("../examples/initialized.json");
  assert.equal(validate({ ...reply, error: { code: "unsupported", message: "Unsupported" } }), false);
  assert.equal(validate({ ...reply, id: "unexpected" }), false);
  const { result, ...empty } = reply;
  assert.equal(validate(empty), false);
});
test("sessions, correlation, versions and message kinds are validated", () => {
  const request = read("../examples/initialize.json");
  for (const value of [{ ...request, version: 2 }, { ...request, sessionId: "" },
    { ...request, id: null }, { ...request, kind: "response" }, { ...request, extra: "unknown" }]) {
    assert.equal(validate(value), false);
  }
});
