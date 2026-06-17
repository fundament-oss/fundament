#!/usr/bin/env bun
// Dev-only static server for previewing the OpenFSC console templates in a
// browser against the RUNNING cluster (no Console host needed).
//
//   just openfsc console-preview   (or: bun plugins/openfsc/scripts/console-preview/serve.js)
//
// It serves plugins/openfsc/console/ as the web root, maps the SDK paths the
// templates request (/plugin-ui/plugin-sdk.{js,css}) to the local stand-in JS
// and the real Console CSS, and backs that stand-in's k8s.list/get with
// `kubectl` via /api/list and /api/get, so the views render the same CRs the
// real Console would.
import { join, normalize, sep } from 'node:path';

const HERE = new URL('.', import.meta.url).pathname;
const CONSOLE_DIR = normalize(join(HERE, '..', '..', 'console'));
// Real plugin SDK CSS lives in console-frontend; reuse it (no copy = no drift).
const SDK_CSS = normalize(
  join(HERE, '..', '..', '..', '..', 'console-frontend', 'public', 'plugin-ui', 'plugin-sdk.css'),
);
const PORT = Number(process.env.PORT ?? Bun.argv[2] ?? 4319);
const CTX = process.env.kube_context ?? 'k3d-fundament-plugin';

// The openfsc.fundament.io CRDs the templates address, with their scope.
// Cluster vs namespaced controls whether list spans all namespaces (-A) and
// whether get needs -n.
const RESOURCES = {
  fscinstallations: { kind: 'fscinstallations.openfsc.fundament.io', namespaced: true },
};

async function kubectl(args) {
  const proc = Bun.spawn(['kubectl', '--context', CTX, ...args], { stdout: 'pipe', stderr: 'pipe' });
  const [out, err] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
  ]);
  return { code: await proc.exited, out, err };
}

const jsonResponse = (obj, status = 200) =>
  new Response(JSON.stringify(obj), { status, headers: { 'content-type': 'application/json' } });

// `kubectl get ... -o json` returns a List ({items:[...]}) for the list form and
// a single object for the get form -- both match what the SDK callers expect.
async function kubectlJson(args) {
  const { code, out, err } = await kubectl([...args, '-o', 'json']);
  if (code !== 0) return jsonResponse({ error: (err || out).trim() || `kubectl exited ${code}` }, 502);
  try {
    return jsonResponse(JSON.parse(out));
  } catch {
    return jsonResponse({ error: 'kubectl returned invalid JSON' }, 502);
  }
}

async function handleList(q) {
  const r = RESOURCES[q.get('resource')];
  if (!r) return jsonResponse({ error: `unknown resource ${q.get('resource')}` }, 400);
  return kubectlJson(['get', r.kind, ...(r.namespaced ? ['-A'] : [])]);
}

async function handleGet(q) {
  const r = RESOURCES[q.get('resource')];
  if (!r) return jsonResponse({ error: `unknown resource ${q.get('resource')}` }, 400);
  const name = q.get('name');
  if (!name) return jsonResponse({ error: 'missing name' }, 400);
  const ns = q.get('namespace');
  // `--` terminates flags so a name like `--all` is read as a resource name.
  return kubectlJson(['get', r.kind, ...(r.namespaced && ns ? ['-n', ns] : []), '--', name]);
}

const pages = [...new Bun.Glob('*.html').scanSync(CONSOLE_DIR)].sort();

const indexHtml = () => `<!doctype html><meta charset="utf-8">
<title>OpenFSC console preview</title>
<style>body{font-family:system-ui,sans-serif;margin:2rem;color:#1f2933}
a{color:#2563eb} li{margin:.25rem 0} code{background:#eef2f7;padding:0 .25rem;border-radius:3px}</style>
<h1>OpenFSC console preview</h1>
<p>Templates from <code>console/</code> rendered against context <code>${CTX}</code> (live cluster data).</p>
<ul>${pages.map((p) => `<li><a href="/${p}">${p}</a></li>`).join('')}</ul>`;

const server = Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url);
    const path = decodeURIComponent(url.pathname);

    if (path === '/' || path === '/index.html') {
      return new Response(indexHtml(), { headers: { 'content-type': 'text/html; charset=utf-8' } });
    }
    if (path === '/api/list') return handleList(url.searchParams);
    if (path === '/api/get') return handleGet(url.searchParams);

    // SDK paths the templates load from the (stubbed) Console host: the JS is
    // the local stand-in, the CSS is the real Console stylesheet.
    if (path === '/plugin-ui/plugin-sdk.js') return new Response(Bun.file(join(HERE, 'plugin-sdk.js')));
    if (path === '/plugin-ui/plugin-sdk.css') return new Response(Bun.file(SDK_CSS));

    // Everything else from the real console/ dir. normalize() + the prefix
    // check keep requests from escaping it via ../.
    const file = normalize(join(CONSOLE_DIR, path));
    if (file !== CONSOLE_DIR && !file.startsWith(CONSOLE_DIR + sep)) {
      return new Response('forbidden', { status: 403 });
    }
    const f = Bun.file(file);
    return (await f.exists()) ? new Response(f) : new Response('not found', { status: 404 });
  },
});

console.log(`OpenFSC console preview: http://localhost:${server.port}  (context: ${CTX})`);
console.log(`Templates: ${pages.join(', ')}`);
