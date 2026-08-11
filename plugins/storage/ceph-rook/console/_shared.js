// Shared helpers for ceph-rook plugin templates.
// Keep this file plain ES module so templates can `import` from it.
//
// The plugin CSP is `script-src 'self'; style-src 'self'` with no
// 'unsafe-inline' (see plugin-proxy/pkg/assets/handler.go buildCSP). That rules
// out inline event handlers (onclick=) and inline style attributes alike, so
// everything here wires events with addEventListener and styles with the
// .plugin-* classes from plugin-sdk.css.

// Loads the Fundament plugin SDK v1. Under FUN-17 the iframe runs on the
// dedicated plugin-proxy origin — the same origin that serves the SDK — so the
// bare-path URL below resolves on plugin-proxy, matching the plugin CSP
// (script-src 'self'). The /v1/ segment tracks fundament:init's protocolVersion:
// a future breaking protocol change ships as /plugins/sdk/v2/ and old plugins
// keep loading v1 unchanged.
export function loadSdk() {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = '/plugins/sdk/v1/plugin-sdk.css';
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = '/plugins/sdk/v1/plugin-sdk.js';
    script.onload = () => resolve(window.fundament);
    script.onerror = () => reject(new Error('failed to load plugin-sdk.js'));
    document.head.appendChild(script);
  });
}

export function escapeHtml(value) {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// Renders a "no rows" placeholder spanning the given number of columns.
export function emptyRow(colspan, message = 'No items.') {
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(message)}</td></tr>`;
}

export function errorRow(colspan, err) {
  const message = err?.message ?? String(err);
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(`Failed to load: ${message}`)}</td></tr>`;
}

// Posts a navigate message to the parent. The host resolves the destination
// relative to the iframe's current route, so the plugin only sends the resource
// identity. From a create view the host hops to the new resource's sibling
// detail route, which is what makes this work after a successful create too.
// The SDK's pinned parentOrigin scopes the message under FUN-17; falls back to
// '*' before init (or in a preview server that runs unframed).
export function navigateToDetail(name, namespace) {
  window.parent.postMessage(
    { type: 'plugin:navigate', name, namespace },
    window.fundament?.parentOrigin ?? '*',
  );
}

// Returns to the resource-kind list. Only meaningful from a create or detail
// view; the host ignores it on a list.
export function navigateBack() {
  window.parent.postMessage(
    { type: 'plugin:navigate-back' },
    window.fundament?.parentOrigin ?? '*',
  );
}

// Turns each row carrying data-name into a keyboard-reachable link to its
// detail page. The anchors must already be in the DOM.
export function wireRowLinks(root) {
  root.querySelectorAll('a.row-link').forEach((link) => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const row = link.closest('tr');
      navigateToDetail(row.dataset.name, row.dataset.namespace || undefined);
    });
  });
}

// Renders a key/value definition list. Values are already-escaped strings.
export function renderDefList(pairs) {
  const rows = pairs
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v)}</dd>`)
    .join('');
  return `<dl class="plugin-deflist">${rows}</dl>`;
}

// Humanizes a byte count to a human-readable size string (GiB or TiB).
export function humanizeBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const tib = bytes / 1024 ** 4;
  if (tib >= 1) return `${tib.toFixed(1)} TiB`;
  const gib = bytes / 1024 ** 3;
  if (gib >= 1) return `${gib.toFixed(1)} GiB`;
  const mib = bytes / 1024 ** 2;
  if (mib >= 1) return `${mib.toFixed(0)} MiB`;
  return `${bytes} B`;
}
