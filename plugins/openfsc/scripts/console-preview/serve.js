#!/usr/bin/env bun
// Dev-only static server for previewing the OpenFSC console UI against the running
// cluster (no Console host needed). Serves console/ as the web root, the stand-in
// SDK at /plugin-ui/*, and backs k8s.list/get with `kubectl` via /api/*.
//
//   just openfsc console-preview
import { join, normalize, sep } from 'node:path';

const HERE = new URL('.', import.meta.url).pathname;
const CONSOLE_DIR = normalize(join(HERE, '..', '..', 'console'));
// Real plugin-ui assets live in console-frontend/public; reuse them so the
// preview serves the exact built files the Console serves (no copy = no drift).
const PLUGIN_UI_DIR = normalize(
  join(HERE, '..', '..', '..', '..', 'console-frontend', 'public', 'plugin-ui'),
);
const SDK_CSS = join(PLUGIN_UI_DIR, 'plugin-sdk.css');
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

// Runs kubectl and parses stdout as JSON. Callers include `-o json` themselves
// so it can be placed before any `--` separator (see handleGet). `kubectl get`
// returns a List ({items:[...]}) for the list form and a single object for the
// get form -- both match what the SDK callers expect.
async function kubectlJson(args) {
  const { code, out, err } = await kubectl(args);
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
  return kubectlJson(['get', r.kind, ...(r.namespaced ? ['-A'] : []), '-o', 'json']);
}

async function handleGet(q) {
  const r = RESOURCES[q.get('resource')];
  if (!r) return jsonResponse({ error: `unknown resource ${q.get('resource')}` }, 400);
  const name = q.get('name');
  if (!name) return jsonResponse({ error: 'missing name' }, 400);
  const ns = q.get('namespace');
  // `-o json` must come before `--`, which terminates flag parsing so a name
  // like `--all` is still read as a resource name (not an option).
  return kubectlJson(['get', r.kind, ...(r.namespaced && ns ? ['-n', ns] : []), '-o', 'json', '--', name]);
}

// Backs the stand-in SDK's namespace dropdown: the names of the cluster's
// namespaces. The real Console scopes this to the project; here we list all.
async function handleNamespaces() {
  const r = await kubectl(['get', 'namespaces', '-o', 'json']);
  if (r.code !== 0) return jsonResponse({ namespaces: [] });
  try {
    const list = JSON.parse(r.out);
    const names = (list.items ?? []).map((n) => n.metadata?.name).filter(Boolean);
    return jsonResponse({ namespaces: names });
  } catch {
    return jsonResponse({ namespaces: [] });
  }
}

// Backs the stand-in SDK's k8s.create: applies the posted object with
// `kubectl apply` and returns it, mirroring the host's create broker. k8s
// validation errors come back on stderr and surface as { message } so the
// create form shows the real reason.
async function handleCreate(req) {
  const text = await req.text();
  const proc = Bun.spawn(['kubectl', '--context', CTX, 'apply', '-f', '-', '-o', 'json'], {
    stdin: new TextEncoder().encode(text),
    stdout: 'pipe',
    stderr: 'pipe',
  });
  const [out, err] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
  ]);
  const code = await proc.exited;
  if (code !== 0) {
    return jsonResponse({ message: (err || out).trim() || `kubectl exited ${code}` }, 422);
  }
  try {
    return jsonResponse(JSON.parse(out));
  } catch {
    return jsonResponse({ message: 'kubectl returned invalid JSON' }, 502);
  }
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
    if (path === '/api/namespaces') return handleNamespaces();
    if (path === '/api/create' && req.method === 'POST') return handleCreate(req);

    // SDK paths the templates load from the (stubbed) Console host: the JS is
    // the local stand-in, the CSS is the real Console stylesheet.
    if (path === '/plugin-ui/plugin-sdk.js') return new Response(Bun.file(join(HERE, 'plugin-sdk.js')));
    if (path === '/plugin-ui/plugin-sdk.css') return new Response(Bun.file(SDK_CSS));

    // The opt-in NLDD Design System bundle (real built files; fonts are inlined as data: URIs,
    // so there are no separate font assets to serve). Bun.file sets content-type.
    if (path === '/plugin-ui/nldd.js') return new Response(Bun.file(join(PLUGIN_UI_DIR, 'nldd.js')));
    if (path === '/plugin-ui/nldd.css') return new Response(Bun.file(join(PLUGIN_UI_DIR, 'nldd.css')));

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
