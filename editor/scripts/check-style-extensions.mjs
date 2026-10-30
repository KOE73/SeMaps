import { build } from 'esbuild';
import assert from 'node:assert/strict';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const root = fileURLToPath(new URL('../src/', import.meta.url));
const bundle = await build({ stdin: { contents: `export { StyleLibrary } from './model/StyleLibrary.js'; export { parseDocument, serializeDocument } from './model/wire.js'; export { HttpProjectStore } from './editor/io/ProjectStore.js'; export { HostModelStore } from './editor/io/HostModelStore.js'; export { diffModel } from './editor/io/ModelSync.js'; export { styleListFraction } from './workbench/panels/styleSplit.js'; export { HostStyleStore } from './editor/io/HostStyleStore.js';`, resolveDir: root }, bundle: true, write: false, format: 'esm', platform: 'neutral' });
const moduleFile = join(mkdtempSync(join(tmpdir(), 'semaps-modifiers-')), 'checks.mjs');
writeFileSync(moduleFile, bundle.outputFiles[0].text);
const { StyleLibrary, parseDocument, serializeDocument, HttpProjectStore, HostModelStore, diffModel, styleListFraction, HostStyleStore } = await import(pathToFileURL(moduleFile).href);
const controlsBundle = await build({ stdin: { contents: `export { changePickedIds, movePickedId } from './workbench/panels/StyleExtensionControls.js';`, resolveDir: root }, bundle: true, write: false, format: 'esm', platform: 'neutral' });
const controlsFile = join(mkdtempSync(join(tmpdir(), 'semaps-picker-')), 'controls.mjs');
writeFileSync(controlsFile, controlsBundle.outputFiles[0].text);
const { changePickedIds, movePickedId } = await import(pathToFileURL(controlsFile).href);
globalThis.location = { href: 'http://localhost:18979/app/' };
globalThis.window = { location: { ...location, origin: 'http://localhost:18979' } };
globalThis.document = { querySelector: () => ({ content: 'test-key' }) };
let count = 0;
async function check(name, action) { await action(); console.log('PASS: ' + name); count++; }

await check('Checkbox selection preserves stored references and application order', () => {
  const original = ['stored.unavailable', 'second'];
  assert.deepEqual(changePickedIds(original, 'third', true), ['stored.unavailable', 'second', 'third']);
  assert.deepEqual(changePickedIds(original, 'second', true), original);
  assert.deepEqual(changePickedIds(original, 'second', false), ['stored.unavailable']);
  assert.deepEqual(original, ['stored.unavailable', 'second']);
});
await check('Moving modifiers changes precedence without mutating the current selection', () => {
  const original = ['first', 'second', 'third'];
  assert.deepEqual(movePickedId(original, 'second', -1), ['second', 'first', 'third']);
  assert.deepEqual(movePickedId(original, 'second', 1), ['first', 'third', 'second']);
  assert.deepEqual(movePickedId(original, 'first', -1), original);
  assert.deepEqual(movePickedId(original, 'missing', 1), original);
  assert.deepEqual(original, ['first', 'second', 'third']);
});

await check('A modifier changes only supplied formatting before the local override', () => {
  const lib = StyleLibrary.parse({ styles: [{ id: 'class', fill: '#fff', title: { family: 'Consolas', size: 19, color: '#111' }, border: { width: 3, color: '#222' } }, { id: 'interface', icon: { glyph: 'contract' } }] });
  lib.setAppearances({ a: { patch: { fill: '#aabbcc80', title: { color: '#333', opacity: 0.6 }, border: { color: '#444' } } }, b: { patch: { fill: '#aabbcc80' } } });
  const a = lib.blockStyle({ id: 'a', type: 'class', styleId: 'class', container: false, override: { border: { color: '#555' } } });
  assert.equal(a.fill.color, '#aabbcc80'); assert.equal(a.border.color, '#555'); assert.equal(a.border.width, 3);
  assert.equal(a.title.family, 'Consolas'); assert.equal(a.title.size, 19); assert.equal(a.title.color, '#333'); assert.equal(a.title.opacity, 0.6);
  assert.equal(lib.blockStyle({ id: 'b', type: 'interface', styleId: 'interface', container: false }).icon.glyph, 'contract');
  lib.setAppearances({}); assert.equal(lib.blockStyle({ id: 'a', type: 'class', styleId: 'class', container: false }).fill.color, '#fff');
});
await check('Edge patches retain arrow shapes and respect the line override', () => {
  const lib = StyleLibrary.parse({ styles: [{ id: 'line', appliesTo: 'edge', line: { color: '#111', width: 2 }, target: { shape: 'triangle', size: 12 } }] });
  lib.setAppearances({ r: { patch: { line: { color: '#222', opacity: 0.5 }, target: { color: '#333' } } } });
  const edge = lib.edgeStyle({ id: 'r', type: 'uses', styleId: 'line', override: { line: { color: '#444' } } });
  assert.equal(edge.line.color, '#444'); assert.equal(edge.line.opacity, 0.5); assert.equal(edge.line.width, 2); assert.equal(edge.target.shape, 'triangle'); assert.equal(edge.target.size, 12);
});
await check('Definitions survive ordinary stylesheet edits and serialization', () => {
  const sheet = { styles: [{ id: 'base' }], modifiers: [{ id: 'palette', name: 'Palette', patches: { block: { fill: '#abc' } } }], styleRules: [{ id: 'r', selectorVersion: 1 }] };
  const lib = StyleLibrary.parse(sheet); lib.put({ id: 'base', fill: '#def' });
  assert.deepEqual(lib.serialize().modifiers, sheet.modifiers); assert.deepEqual(lib.serialize().styleRules, sheet.styleRules);
});
await check('Host bundle loading and wire roundtrip preserve manual references and exclusions', async () => {
  const placement = { entity: 'a', parent: null, x: 12, y: 23, width: 210, height: 60, modifiers: ['one', 'two'], disabledModifiers: ['auto'], automaticModifiers: false };
  const overlay = { id: 'r', modifiers: ['two'], disabledModifiers: ['auto'], automaticModifiers: false };
  const view = { id: 'v', project: 'p', axis: 'layer', placements: [placement, { entity: 'b', parent: null, x: 250, y: 0, width: 170, height: 50 }], edges: [overlay], disabledStyleRules: ['r1'] };
  const wire = await new HttpProjectStore().loadProjectBundle('projects/p/views/v.view.json', view, { project: { id: 'p', contractVersion: 5 }, entities: { entities: [{ id: 'a', kind: 'class', origin: 'code', name: 'A' }, { id: 'b', kind: 'class', origin: 'code', name: 'B' }] }, relations: { relations: [{ id: 'r', from: 'a', to: 'b', type: 'uses', origin: 'code' }] }, texts: {} });
  const output = serializeDocument(parseDocument(wire));
  for (const field of ['modifiers', 'disabledModifiers', 'automaticModifiers']) { assert.deepEqual(output.placements[0][field], placement[field]); assert.deepEqual(output.edges[0][field], overlay[field]); }
  assert.deepEqual(output.bundle.view.disabledStyleRules, ['r1']);
  assert.deepEqual(diffModel(wire, output, view, wire.bundle.entities.entities, {}, wire.bundle.relations.relations, { entity: new Set(), relation: new Set() }).filter(op => op.kind === 'view' || op.kind === 'placement'), []);
  const edited = structuredClone(output); edited.placements[0].x = 88;
  const moved = diffModel(output, edited, view, wire.bundle.entities.entities, {}, wire.bundle.relations.relations, { entity: new Set(), relation: new Set() }).find(op => op.kind === 'placement');
  assert.deepEqual(moved.value.modifiers, ['one', 'two']); assert.equal(moved.value.automaticModifiers, false);
});
await check('Panel split keeps its preferred proportion through resizing and temporary limits', () => {
  assert.equal(styleListFraction(0.4, 800), 0.4); assert.equal(styleListFraction(0.4, 600), 0.4); assert.equal(styleListFraction(0.4, 320), 0.4);
  assert.equal(styleListFraction(0.25, 500), 180 / 500); assert.equal(styleListFraction(0.25, 1000), 0.25);
});
await check('Modifier edits use the shared revision queue and retain all collections', async () => {
  const store = new HostStyleStore('editor'); store.accept({ sheet: { styles: [] }, revision: 4, kinds: {}, dirty: [] });
  const seen = [];
  globalThis.fetch = async (url, init) => { const body = JSON.parse(init.body); seen.push([String(url), body]); return Response.json({ state: { sheet: { styles: [], modifiers: [{ id: body.id }] }, revision: 5, kinds: {}, dirty: [{ kind: 'modifiers', id: body.id }] } }); };
  await store.extensionEdit('modifiers', { action: 'create', id: 'palette', definition: { name: 'Palette', patches: { block: {} } } });
  assert.match(seen[0][0], /style-modifiers/); assert.equal(seen[0][1].expectedRevision, 4); assert.equal(store.state.sheet.modifiers[0].id, 'palette'); assert.equal(store.state.revision, 5);
});
await check('Live appearance refresh uses the view ID when its file has another name', async () => {
  const store = new HostModelStore();
  const seen = [];
  globalThis.fetch = async url => {
    seen.push(String(url));
    if (String(url).endsWith('/api/model/p')) return Response.json({ project: { id: 'p', contractVersion: 5 }, registry: {}, texts: {}, views: {}, viewFiles: { v_main: 'projects/p/views/main.view.json' } });
    assert.match(String(url), /views\/v_main\/styled$/);
    return Response.json({ view: { id: 'v_main', project: 'p', placements: [] }, styles: { sheet: {}, kinds: {}, revision: 1, dirty: [] }, appearances: { a: { patch: { fill: '#def' } } } });
  };
  let latest;
  store.stylesLoaded = (_state, appearances) => { latest = appearances; };
  await store.load('projects/p/views/main.view.json');
  store.invalidate('p'); // The SSE listener invalidates contents before notifying the editor.
  await store.refreshAppearance('projects/p/views/main.view.json');
  assert.equal(latest.a.patch.fill, '#def');
  assert.equal(seen.filter(url => url.endsWith('/views/v_main/styled')).length, 2);
});
await check('Automatic indicators use applied layers, preserve order, and disappear after refresh', () => {
  const lib = StyleLibrary.parse({ styles: [], styleRules: [{ id: 'r1', name: 'Library rule' }, { id: 'r2', name: 'Namespace rule' }] });
  lib.setAppearances({ a: { applied: [
    { id: 'palette', name: 'Library palette', stage: 'automatic', rule: 'r1' },
    { id: 'accent', name: 'Namespace accent', stage: 'automatic', rule: 'r2' },
    { id: 'manual', name: 'Manual accent', stage: 'manual' },
  ], skipped: [{ id: 'excluded', stage: 'automatic', rule: 'r1' }] }, b: { applied: [{ id: 'manual', stage: 'manual' }] } });
  assert.deepEqual(lib.automaticModifiersFor('a'), [
    { modifierId: 'palette', modifierName: 'Library palette', ruleId: 'r1', ruleName: 'Library rule' },
    { modifierId: 'accent', modifierName: 'Namespace accent', ruleId: 'r2', ruleName: 'Namespace rule' },
  ]);
  assert.deepEqual(lib.automaticModifiersFor('b'), []);
  lib.setAppearances({});
  assert.deepEqual(lib.automaticModifiersFor('a'), []);
});
console.log(`All ${count} style extension checks passed.`);
