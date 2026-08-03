// Shared helpers for ceph-rook plugin templates.
// Keep this file plain ES module so templates can `import` from it.

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

export function formatAge(creationTimestamp) {
  if (!creationTimestamp) return '';
  const created = new Date(creationTimestamp).getTime();
  if (Number.isNaN(created)) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - created) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

// Renders a "no rows" placeholder spanning the given number of columns.
export function emptyRow(colspan, message = 'No items.') {
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(message)}</td></tr>`;
}

export function errorRow(colspan, err) {
  const message = err?.message ?? String(err);
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(`Failed to load: ${message}`)}</td></tr>`;
}

// Humanizes a byte count to a human-readable size string (GiB or TiB).
export function humanizeBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const tib = bytes / (1024 ** 4);
  if (tib >= 1) return `${tib.toFixed(1)} TiB`;
  const gib = bytes / (1024 ** 3);
  if (gib >= 1) return `${gib.toFixed(1)} GiB`;
  const mib = bytes / (1024 ** 2);
  if (mib >= 1) return `${mib.toFixed(0)} MiB`;
  return `${bytes} B`;
}
