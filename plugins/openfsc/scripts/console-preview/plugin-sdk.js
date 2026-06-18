// Dev-only stand-in for the Fundament plugin SDK.
//
// The console templates (plugins/openfsc/console/*.html) load the real SDK from
// the Console host at /plugin-ui/plugin-sdk.js and then call fundament.k8s.* to
// fetch CRs. The local preview server (serve.js) maps that path here and backs
// k8s.list/get with `kubectl` against the running cluster, so the views render
// the SAME data the real Console would show -- just without the Console.
//
// This file is NOT embedded in the plugin (it lives outside console/); it only
// exists to view/iterate on the templates in a browser.

async function api(path) {
  const res = await fetch(path);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.message || body.error || `${res.status} ${res.statusText}`);
  return body;
}

async function apiPost(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.message || body.error || `${res.status} ${res.statusText}`);
  return body;
}

async function fetchNamespaces() {
  try {
    const body = await api('/api/namespaces');
    return Array.isArray(body.namespaces) ? body.namespaces : [];
  } catch {
    return [];
  }
}

const params = new URLSearchParams(location.search);

window.fundament = {
  // Templates read fundament.init -> { resource: { name, namespace }, namespaces }.
  // resource.name comes from ?name=&namespace= (e.g. set by a row click in the
  // list page). namespaces feeds the create form's dropdown.
  init: (async () => ({
    resource: { name: params.get('name'), namespace: params.get('namespace') },
    namespaces: await fetchNamespaces(),
  }))(),
  k8s: {
    list: ({ resource }) => api(`/api/list?resource=${encodeURIComponent(resource)}`),
    get: ({ resource, name, namespace }) =>
      api(
        `/api/get?resource=${encodeURIComponent(resource)}&name=${encodeURIComponent(name ?? '')}` +
          `&namespace=${encodeURIComponent(namespace ?? '')}`,
      ),
    create: (_args, body) => apiPost('/api/create', body),
  },
};

// The real Console handles plugin:navigate from the iframe. Here we approximate
// it by routing to the matching detail page so row clicks are clickable too.
window.addEventListener('message', (e) => {
  if (e.data?.type !== 'plugin:navigate') return;
  const base = (location.pathname.match(/([a-z]+)-list\.html$/) || [])[1];
  if (!base) return;
  const q = new URLSearchParams();
  if (e.data.name) q.set('name', e.data.name);
  if (e.data.namespace) q.set('namespace', e.data.namespace);
  location.href = `${base}-detail.html?${q.toString()}`;
});
