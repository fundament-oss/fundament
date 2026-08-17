// Shared helpers for ceph-rook plugin templates. Plain ES module so templates
// can `import` from it.
//
// The plugin CSP has no 'unsafe-inline', which rules out onclick= and inline
// style alike: events go through addEventListener, styling through the
// .plugin-* classes in plugin-sdk.css.

// Loads the plugin SDK v1. The iframe runs on the plugin-proxy origin, which
// also serves the SDK, so these bare paths satisfy script-src 'self'. The /v1/
// segment tracks protocolVersion: a breaking change ships as /v2/.
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

export function emptyRow(colspan, message = 'No items.') {
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(message)}</td></tr>`;
}

export function errorRow(colspan, err) {
  const message = err?.message ?? String(err);
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(`Failed to load: ${message}`)}</td></tr>`;
}

// Posts a navigate message to the parent, which resolves it relative to the
// iframe's current route — so this sends only the resource identity, and works
// from a create view too. parentOrigin falls back to '*' before init.
export function navigateToDetail(name, namespace) {
  window.parent.postMessage(
    { type: 'plugin:navigate', name, namespace },
    window.fundament?.parentOrigin ?? '*',
  );
}

// Asks the host for this kind's create route. A custom list UI needs its own
// "Add": the console only renders its built-in Create button for kinds without a
// custom list component, so without this the create view is unreachable.
export function navigateToCreate() {
  window.parent.postMessage(
    { type: 'plugin:create' },
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

// Makes each data-name row a keyboard-reachable link. Anchors must be in the DOM.
export function wireRowLinks(root) {
  root.querySelectorAll('a.row-link').forEach((link) => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const row = link.closest('tr');
      navigateToDetail(row.dataset.name, row.dataset.namespace || undefined);
    });
  });
}

// Renders a key/value definition list from [key, value] pairs. Both halves are
// escaped here, so pass raw values — pre-escaped ones render as "&amp;".
export function renderDefList(pairs) {
  const rows = pairs
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v)}</dd>`)
    .join('');
  return `<dl class="plugin-deflist">${rows}</dl>`;
}

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
