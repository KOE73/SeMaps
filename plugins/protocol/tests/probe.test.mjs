import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const code = readFileSync(new URL("../probe.js", import.meta.url), "utf8");
const initialize = JSON.parse(readFileSync(new URL("../examples/initialize.json", import.meta.url)));
const initialized = JSON.parse(readFileSync(new URL("../examples/initialized.json", import.meta.url)));

function harness() {
  const replies = [];
  const listeners = [];
  const context = vm.createContext({ chrome: { webview: {
    addEventListener: (_, listener) => listeners.push(listener),
    postMessage: (reply) => replies.push(JSON.parse(JSON.stringify(reply))),
  } } });
  vm.runInContext(code, context);
  return { context, replies, listeners, send: (request) => listeners[0]({ data: request }) };
}

test("round-trip matches the common example and advertises no editor commands", () => {
  const h = harness();
  h.send(initialize);
  assert.deepEqual(h.replies, [initialized]);
});
test("duplicate attachment does not duplicate handlers or replies", () => {
  const h = harness();
  vm.runInContext(code, h.context);
  assert.equal(h.listeners.length, 1);
  h.send(initialize);
  assert.equal(h.replies.length, 1);
});
test("unrelated, malformed, future-version and unsupported messages are ignored", () => {
  const h = harness();
  for (const request of [null, {}, { ...initialize, protocol: "other" }, { ...initialize, version: 2 },
    { ...initialize, method: "entities.place" }, { ...initialize, id: 5 }, { ...initialize, sessionId: null },
    { ...initialize, id: "" }, { ...initialize, sessionId: "" }]) h.send(request);
  assert.equal(h.replies.length, 0);
});
test("fresh sessions are reflected in their own response", () => {
  const h = harness();
  h.send({ ...initialize, sessionId: "new-session", id: "new-id" });
  assert.equal(h.replies[0].sessionId, "new-session");
  assert.equal(h.replies[0].requestId, "new-id");
});
test("ordinary browser remains untouched", () => {
  const context = vm.createContext({});
  vm.runInContext(code, context);
  assert.equal(context.__semapsIdeProbe, undefined);
});
