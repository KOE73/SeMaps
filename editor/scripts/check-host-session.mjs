import { build } from 'esbuild';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

const bundle = await build({ entryPoints: [fileURLToPath(new URL('../src/util/hostKey.ts', import.meta.url))], bundle: true, write: false, format: 'esm', platform: 'neutral' });
const { hostFetch, rememberHostContext } = await import('data:text/javascript;base64,' + Buffer.from(bundle.outputFiles[0].text).toString('base64'));
const setup = { projectFile: 'demo.semaps', workspace: 'workspace', sourceRoot: 'sources' };
const origin = 'http://localhost:8777';
globalThis.window = { location: { href: origin + '/app/', origin } };
let meta;
globalThis.document = {
  querySelector: () => meta,
  createElement: () => ({ name: '', content: '' }),
  head: { appendChild: node => { meta = node; } },
};
globalThis.DOMParser = class {
  parseFromString(html) {
    const key = /<meta name="semaps-key" content="([^"]+)"/.exec(html)?.[1];
    return { querySelector: () => key ? { content: key } : null };
  }
};
const rejection = () => new Response('Missing or invalid host key', { status: 401 });
let count = 0;
async function check(name, action) { meta = { content: 'old' }; rememberHostContext(setup); await action(); count++; console.log('PASS: ' + name); }
function fixture({ nextSetup = setup, html = '<meta name="semaps-key" content="new" />', denied, failRefresh = false } = {}) {
  const calls = []; let writes = 0;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, method: init.method ?? 'GET', auth: new Headers(init.headers).get('Authorization'), contentType: new Headers(init.headers).get('Content-Type'), cache: init.cache, redirect: init.redirect, body: init.body });
    if (url === '/app/') {
      if (failRefresh) throw new TypeError('redirect refused');
      return new Response(html);
    }
    if (url === '/api/setup') return Response.json(nextSetup);
    if (denied) return new Response(denied.message, { status: denied.status });
    if (new Headers(init.headers).get('Authorization') !== 'Bearer new') return rejection();
    writes++; return Response.json({ ok: true });
  };
  return { calls, writes: () => writes };
}
const write = () => hostFetch('/api/runs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"extractor":"cs"}' });

await check('Current key sends one write with its body and content type', async () => {
  meta.content = 'new'; const f = fixture(); assert.equal((await write()).status, 200); assert.equal(f.calls.length, 1); assert.equal(f.writes(), 1);
  assert.equal(f.calls[0].body, '{"extractor":"cs"}'); assert.equal(f.calls[0].contentType, 'application/json');
});
await check('A stale key is refreshed on the same origin and the refused write executes once', async () => {
  const f = fixture(); assert.equal((await write()).status, 200); assert.equal(f.writes(), 1);
  assert.deepEqual(f.calls.filter(c => c.url === '/api/runs').map(c => c.auth), ['Bearer old', 'Bearer new']);
  assert.ok(f.calls.filter(c => c.url === '/api/runs').every(c => c.body === '{"extractor":"cs"}' && c.contentType === 'application/json'));
  assert.equal(meta.content, 'new');
  assert.ok(f.calls.filter(c => c.method === 'GET').every(c => c.cache === 'no-store' && c.redirect === 'error'));
});
await check('A missing meta key can be restored without discarding the form', async () => {
  meta = null; const f = fixture(); assert.equal((await write()).status, 200); assert.equal(f.writes(), 1); assert.equal(meta.content, 'new');
});
await check('Concurrent rejections share one refresh and each refused write executes once', async () => {
  const f = fixture(); const responses = await Promise.all([write(), write()]); assert.ok(responses.every(r => r.ok));
  assert.equal(f.calls.filter(c => c.url === '/app/').length, 1); assert.equal(f.writes(), 2);
});
await check('A different project on the same port never receives a retry', async () => {
  const f = fixture({ nextSetup: { ...setup, projectFile: 'another.semaps' } }); assert.equal((await write()).status, 401);
  assert.equal(f.calls.filter(c => c.url === '/api/runs').length, 1); assert.equal(f.writes(), 0); assert.equal(meta.content, 'old');
});
await check('A dev page without meta or a failed refresh does not repeat the write', async () => {
  for (const options of [{ html: '<html><head></head></html>' }, { failRefresh: true }]) {
    const f = fixture(options); assert.equal((await write()).status, 401); assert.equal(f.calls.filter(c => c.url === '/api/runs').length, 1);
  }
});
await check('Other 401/403 failures and an unknown original context are not retried', async () => {
  for (const denied of [{ status: 401, message: 'Another rejection' }, { status: 403, message: 'Cross-origin request refused' }]) {
    const f = fixture({ denied }); assert.equal((await write()).status, denied.status); assert.equal(f.calls.length, 1);
  }
  rememberHostContext(undefined); const f = fixture(); assert.equal((await write()).status, 401); assert.equal(f.calls.length, 1);
});
await check('Cross-origin URLs never receive the host key', async () => {
  const f = fixture(); await assert.rejects(hostFetch('http://another-host/api/runs', { method: 'POST' }), /Cross-origin/); assert.equal(f.calls.length, 0);
});
console.log(`All ${count} host session checks passed.`);
