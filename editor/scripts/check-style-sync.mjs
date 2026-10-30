import { build } from 'esbuild';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

const bundle = await build({ entryPoints: [fileURLToPath(new URL('../src/editor/io/HostStyleStore.ts', import.meta.url))], bundle: true, write: false, format: 'esm', platform: 'neutral' });
const { HostStyleStore, stylePatch } = await import('data:text/javascript;base64,' + Buffer.from(bundle.outputFiles[0].text).toString('base64'));
const selectionBundle = await build({ entryPoints: [fileURLToPath(new URL('../src/editor/StyleSelection.ts', import.meta.url))], bundle: true, write: false, format: 'esm', platform: 'neutral' });
const { StyleSelection } = await import('data:text/javascript;base64,' + Buffer.from(selectionBundle.outputFiles[0].text).toString('base64'));
globalThis.window = { location: { href: 'http://localhost:8777/app/', origin: 'http://localhost:8777' } };
globalThis.document = { querySelector: () => ({ content: 'test-key' }) };
const base = { id: 'debug', name: 'Debug', appliesTo: 'block', forKinds: ['class'], fill: '#ffe4e6', border: { color: '#dc2626', width: 3 } };
const initial = () => ({ sheet: { styles: [structuredClone(base)] }, revision: 7, dirty: [], kinds: {} });
let count = 0;
async function check(name, action) { await action(); count++; console.log('PASS: ' + name); }

await check('Nested own values are cleared with null while unrelated values survive', async () => {
  assert.deepEqual(stylePatch(base, { ...base, border: { width: 4 } }), { border: { color: null, width: 4 } });
});

await check('Rapid edits use incremental patches and acknowledge revisions in order', async () => {
  const store = new HostStyleStore('editor-1'); store.accept(initial());
  const calls = [];
  let release;
  const blocked = new Promise(resolve => { release = resolve; });
  globalThis.fetch = async (_url, init) => {
    const body = JSON.parse(init.body); calls.push(body);
    if (calls.length === 1) await blocked;
    return Response.json({ state: { ...initial(), revision: 7 + calls.length } });
  };
  const first = store.sync({ styles: [{ ...base, name: 'First' }] });
  const second = store.sync({ styles: [{ ...base, name: 'Second', border: { width: 4 } }] });
  await Promise.resolve(); assert.equal(calls.length, 1); release();
  await Promise.all([first, second]);
  assert.deepEqual(calls.map(c => c.patch), [{ name: 'First' }, { name: 'Second', border: { color: null, width: 4 } }]);
  assert.deepEqual(calls.map(c => c.expectedRevision), [7, 8]);
  assert.ok(calls.every(c => c.client === 'editor-1' && c.action === 'update'));
});

await check('Creating a style must finish before flush permits its assignment', async () => {
  const store = new HostStyleStore('editor-1'); store.accept(initial());
  let release; const blocked = new Promise(resolve => { release = resolve; });
  globalThis.fetch = async (_url, init) => {
    assert.equal(JSON.parse(init.body).action, 'create'); await blocked;
    return Response.json({ state: { ...initial(), revision: 8 } });
  };
  const edit = store.sync({ styles: [base, { ...base, id: 'new' }] });
  let assigned = false; const assignment = store.flush().then(() => { assigned = true; });
  await Promise.resolve(); assert.equal(assigned, false); release();
  await Promise.all([edit, assignment]); assert.equal(assigned, true);
});

await check('A conflicting revision is reported and a reload recovers accepted host state', async () => {
  const store = new HostStyleStore('editor-1'); store.accept(initial());
  globalThis.fetch = async (_url, init) => init?.method === 'POST'
    ? new Response('revision conflict: reload styles', { status: 422 })
    : Response.json({ ...initial(), revision: 8 });
  await assert.rejects(store.sync({ styles: [{ ...base, name: 'Rejected' }] }), /revision conflict/);
  assert.deepEqual(await store.load(), initial().sheet); assert.equal(store.state.revision, 8);
});
await check('A latest snapshot arriving before queued rename events preserves the selected form', async () => {
  const selection = new StyleSelection(); selection.open('old');
  const has = id => id === 'latest';
  selection.reconcile(has); assert.equal(selection.id, null);
  assert.equal(selection.rename('old', 'middle'), true);
  selection.reconcile(has); assert.equal(selection.id, null);
  assert.equal(selection.rename('middle', 'latest'), true);
  selection.reconcile(has); assert.equal(selection.id, 'latest');
});

await check('Deletion or a new human selection prevents a delayed rename from reopening a form', async () => {
  const selection = new StyleSelection(); selection.open('old'); selection.reconcile(() => false);
  selection.remove('old'); assert.equal(selection.rename('old', 'new'), false);
  selection.open('old'); selection.reconcile(() => false); selection.open('chosen');
  assert.equal(selection.rename('old', 'new'), false); assert.equal(selection.id, 'chosen');
});

await check('Discard restoring a temporarily missing definition also restores its selection', async () => {
  const selection = new StyleSelection(); selection.open('saved'); selection.reconcile(() => false);
  selection.reconcile(id => id === 'saved'); assert.equal(selection.id, 'saved');
});
console.log(`All ${count} style sync checks passed.`);
