import { build } from 'esbuild';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
const base = new URL('../', import.meta.url);
async function module(path) {
  const result = await build({ entryPoints: [fileURLToPath(new URL(path, base))], bundle: true, write: false, format: 'esm', platform: 'neutral', target: 'es2022' });
  return import('data:text/javascript;base64,' + Buffer.from(result.outputFiles[0].text).toString('base64'));
}
const { IdeBridge, validMessage } = await module('src/integration/bridge.ts');
const { EditorIdeAdapter, sourceAddress } = await module('src/integration/editorAdapter.ts');
const { frameTransport, webViewTransport } = await module('src/integration/transport.ts');
const { DiagramDocument } = await module('src/model/document.ts');
let count = 0;
async function check(name, run) { await run(); count++; console.log('PASS: ' + name); }
const tick = () => new Promise(resolve => setTimeout(resolve, 0));
const request = (id, method, params = {}, sessionId = 'session') => ({ protocol: 'semaps.ide', version: 1, sessionId, kind: 'request', id, method, params });
function channel() {
  const sent = []; let receive;
  return { sent, inject: value => receive(value), transport: { send: value => sent.push(value), subscribe: callback => { receive = callback; return () => { receive = () => {}; }; } } };
}
await check('Envelope validator agrees with handshake fixtures and rejects malformed messages', () => {
  for (const name of ['initialize', 'initialized', 'editor-initialized', 'place', 'selection-changed']) assert.equal(validMessage(JSON.parse(readFileSync(new URL('../plugins/protocol/examples/' + name + '.json', base)))), true);
  for (const value of [request('', 'context.get'), { ...request('x', 'context.get'), version: 2 }, { ...request('x', 'context.get'), params: [] }, { ...request('x', 'context.get'), extra: 1 }, { ...request('x', 'context.get'), result: {} }, { protocol: 'semaps.ide', version: 1, sessionId: 's', kind: 'response', requestId: 'r', result: {}, error: {} }]) assert.equal(validMessage(value), false);
});
await check('Commands require initialization; duplicate placement requests execute once', async () => {
  const c = channel(); let executions = 0;
  const bridge = new IdeBridge(c.transport, { capabilities: ['entities.place'], context: () => ({}), execute: () => { executions++; return { placed: true }; } });
  c.inject(request('early', 'entities.place')); await tick(); assert.equal(executions, 0);
  c.inject(request('init', 'bridge.initialize')); await tick();
  c.inject(request('place', 'entities.place')); c.inject(request('place', 'entities.place')); await tick();
  c.inject(request('place', 'entities.place')); await tick(); assert.equal(executions, 1);
  assert.equal(c.sent.filter(m => m.requestId === 'place').length, 2);
  c.inject(request('place', 'entities.place', { different: true })); await tick(); assert.equal(c.sent.at(-1).error.code, 'duplicateId'); bridge.dispose();
});
await check('Old sessions cannot affect a reconnected bridge', async () => {
  const c = channel(); let executions = 0;
  const bridge = new IdeBridge(c.transport, { capabilities: ['context.get'], context: () => ({}), execute: () => { executions++; return {}; } });
  c.inject(request('init', 'bridge.initialize')); await tick(); c.inject(request('init2', 'bridge.initialize', {}, 'new')); await tick();
  c.inject(request('foreign', 'context.get')); await tick(); assert.equal(executions, 0); bridge.dispose();
});
await check('Cancellation stops an asynchronous request before mutation', async () => {
  const c = channel(); let finish; let mutated = false;
  const bridge = new IdeBridge(c.transport, { capabilities: ['entities.place'], context: () => ({}), execute: async (_m, _p, signal) => { await new Promise(resolve => { finish = resolve; }); if (!signal.aborted) mutated = true; return {}; } });
  c.inject(request('init', 'bridge.initialize')); await tick(); c.inject(request('long', 'entities.place')); await tick();
  c.inject(request('cancel', 'bridge.cancel', { requestId: 'long' })); await tick(); finish(); await tick();
  assert.equal(mutated, false); assert.equal(c.sent.find(m => m.requestId === 'long').error.code, 'cancelled'); bridge.dispose();
});
await check('Outgoing source requests are correlated and time out', async () => {
  const c = channel(); const bridge = new IdeBridge(c.transport, { capabilities: [], context: () => ({}), execute: () => ({}) });
  c.inject(request('init', 'bridge.initialize', { capabilities: ['source.open'] })); await tick();
  const opening = bridge.request('source.open', { file: 'src/A.cs' }); const message = c.sent.at(-1);
  c.inject({ protocol: 'semaps.ide', version: 1, sessionId: 'session', kind: 'response', requestId: message.id, result: { opened: true } });
  assert.equal((await opening).opened, true); await assert.rejects(bridge.request('source.open', {}, 5), error => error.code === 'timeout'); bridge.dispose();
});
await check('WebView and iframe adapters carry the same envelope; frame origin and peer are checked', () => {
  const message = request('init', 'bridge.initialize'); const sent = []; let listener; const peer = { postMessage: (...args) => sent.push(args) };
  const receiver = { addEventListener: (_name, callback) => { listener = callback; }, removeEventListener: () => {} };
  const frame = frameTransport(peer, 'https://ide.example', receiver); const received = []; frame.subscribe(value => received.push(value)); frame.send(message);
  listener({ source: peer, origin: 'https://evil.example', data: message }); listener({ source: {}, origin: 'https://ide.example', data: message });
  listener({ source: peer, origin: 'https://ide.example', data: message }); assert.deepEqual(received, [message]); assert.deepEqual(sent[0], [message, 'https://ide.example']);
  assert.throws(() => frameTransport(peer, '*', receiver));
  const web = webViewTransport({ postMessage: value => sent.push(value), addEventListener: (_type, callback) => { listener = callback; }, removeEventListener: () => {} });
  web.send(message); web.subscribe(value => received.push(value)); listener({ data: message }); assert.deepEqual(sent.at(-1), message); assert.equal(received.length, 2);
});
function editor() {
  const entities = [{ id: 'e_a', kind: 'class', code: [{ lang: 'csharp', symbol: 'N.A', ref: 'src/A.cs#L7' }] }, { id: 'e_b', kind: 'class', code: [{ lang: 'csharp', symbol: 'N.B' }] }];
  const doc = new DiagramDocument({ metadata: {}, views: [], roots: [], edges: [], raw: { bundle: { entities, relations: [], project: {} } } });
  const commits = []; const canvas = { model: doc, selectedIds: new Set(), selected: null, viewCenter: () => ({ x: 0, y: 0 }), selectMany(ids) { this.selectedIds = new Set(ids); this.selected = ids.length ? { id: ids.at(-1) } : null; }, reveal(id) { return doc.element(id) !== undefined; }, hostElement: { getBoundingClientRect: () => ({ left: 10, right: 510, top: 20, bottom: 520 }) }, viewport: { toModel: p => ({ x: p.x / 2, y: p.y / 2 }) } };
  return { canvas, currentView: { id: 'v_one' }, projectOf: () => ({ id: 'p_one' }), commit: value => commits.push(value), commits };
}
await check('Placement uses the real document, skips duplicates and commits one Undo step', async () => {
  const e = editor(); const a = new EditorIdeAdapter(e, () => ''); const context = a.context(); const signal = new AbortController().signal;
  const result = await a.execute('entities.place', { context, entityIds: ['e_a', 'e_b'], at: { space: 'model', x: 100, y: 200 } }, signal, 'drop');
  assert.equal([...e.canvas.model.elements()].length, 2); assert.deepEqual(e.commits, ['place-entity']); assert.equal(result.results[0].status, 'placed');
  await a.execute('entities.place', { context, entityIds: ['e_a'] }, signal, 'again'); assert.equal([...e.canvas.model.elements()].length, 2); assert.equal(e.commits.length, 1);
});
await check('Stale view/document/mode and invalid points do not write', async () => {
  const e = editor(); let mode = ''; const a = new EditorIdeAdapter(e, () => mode); const context = a.context(); const signal = new AbortController().signal;
  e.currentView = { id: 'v_other' }; await assert.rejects(a.execute('entities.place', { context, entityIds: ['e_a'] }, signal, 'stale'), error => error.code === 'staleContext');
  const beforeReload = a.context(); e.canvas.model = editor().canvas.model;
  await assert.rejects(a.execute('entities.place', { context: beforeReload, entityIds: ['e_a'] }, signal, 'reload'), error => error.code === 'staleContext');
  const newContext = a.context(); mode = 'graph'; await assert.rejects(a.execute('entities.place', { context: newContext, entityIds: ['e_a'] }, signal, 'mode'), error => error.code === 'staleContext');
  mode = ''; const valid = a.context(); await assert.rejects(a.execute('entities.place', { context: valid, entityIds: ['e_a'], at: { space: 'screen', x: 1, y: 2 } }, signal, 'point'), error => error.code === 'invalidParams');
  assert.equal(e.commits.length, 0);
});
await check('Client CSS coordinates are converted through the canvas viewport', async () => {
  const e = editor(); const a = new EditorIdeAdapter(e, () => '');
  await a.execute('entities.place', { context: a.context(), entityIds: ['e_a'], at: { space: 'client', x: 210, y: 420 } }, new AbortController().signal, 'css');
  assert.equal(e.canvas.model.element('e_a').x, 100); assert.equal(e.canvas.model.element('e_a').y, 200);
});
await check('Resolve distinguishes missing facts, absent entities and existing placements', async () => {
  const e = editor(); const a = new EditorIdeAdapter(e, () => ''); const signal = new AbortController().signal;
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async () => ({ ok: true, json: async () => ({ nodes: [{ id: 'n_c', symbol: 'N.C', language: 'csharp' }] }) });
  try {
    const result = await a.execute('symbols.resolve', { context: a.context(), symbols: ['N.A', 'N.C', 'N.D'].map(symbolId => ({ symbolId, language: 'csharp' })) }, signal, 'resolve');
    assert.deepEqual(result.results.map(r => r.status), ['resolved', 'notInModel', 'notFound']);
  } finally { globalThis.fetch = previousFetch; }
  assert.deepEqual(sourceAddress('src\\A.cs#L7-L12'), { file: 'src/A.cs', line: 7 });
});
await check('An asynchronous resolve cannot finish in a different view', async () => {
  const e = editor(); const a = new EditorIdeAdapter(e, () => ''); const context = a.context();
  const previousFetch = globalThis.fetch; let complete;
  globalThis.fetch = () => new Promise(resolve => { complete = resolve; });
  try {
    const pending = a.execute('symbols.resolve', { context, symbols: [{ language: 'csharp', symbolId: 'N.A' }] }, new AbortController().signal, 'slow');
    e.currentView = { id: 'v_other' };
    complete({ ok: true, json: async () => ({ nodes: [] }) });
    await assert.rejects(pending, error => error.code === 'staleContext'); assert.equal(e.commits.length, 0);
  } finally { globalThis.fetch = previousFetch; }
});
await check('Ambiguous bindings remain candidates; an IDE selection carries causation and primary', async () => {
  const e = editor(); e.canvas.model.entities[1].code = [{ lang: 'csharp', symbol: 'N.A' }];
  const a = new EditorIdeAdapter(e, () => ''); const signal = new AbortController().signal;
  const previousFetch = globalThis.fetch; globalThis.fetch = async () => ({ ok: true, json: async () => ({ nodes: [] }) });
  try {
    const resolved = await a.execute('symbols.resolve', { context: a.context(), symbols: [{ language: 'csharp', symbolId: 'N.A' }] }, signal, 'ambiguous');
    assert.equal(resolved.results[0].status, 'ambiguous'); assert.equal(resolved.results[0].candidates.length, 2);
    await a.execute('entities.place', { context: a.context(), entityIds: ['e_a', 'e_b'] }, signal, 'drop');
    const events = []; a.bridge = { event: (...args) => events.push(args) };
    const selectMany = e.canvas.selectMany.bind(e.canvas); e.canvas.selectMany = ids => { selectMany(ids); a.selectionChanged(); };
    await a.execute('selection.set', { context: a.context(), entityIds: ['e_a', 'e_b'], primaryId: 'e_a' }, signal, 'selection');
    assert.equal(events.at(-1)[1].primaryId, 'e_a'); assert.equal(events.at(-1)[2], 'selection');
  } finally { globalThis.fetch = previousFetch; }
});
console.log(`All ${count} IDE bridge checks passed.`);
