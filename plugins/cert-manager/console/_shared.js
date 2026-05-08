// Shared helpers for cert-manager plugin templates.
// Keep this file plain ES module so templates can `import` from it.

export function hostOrigin() {
  const raw = new URLSearchParams(location.search).get('host');
  if (!raw) return '';
  try {
    const url = new URL(raw);
    if (url.protocol !== 'https:' && url.protocol !== 'http:') return '';
    return url.origin;
  } catch {
    return '';
  }
}

// Loads the Fundament plugin SDK from the host (Console) origin and resolves
// once `window.fundament` is available. Templates must call this before any
// other SDK use because the iframe is sandboxed `allow-scripts` and the
// host origin is not known at parse time.
export function loadSdk() {
  const host = hostOrigin();
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `${host}/plugin-ui/plugin-sdk.css`;
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = `${host}/plugin-ui/plugin-sdk.js`;
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

export function findCondition(item, type) {
  const conditions = item?.status?.conditions;
  if (!Array.isArray(conditions)) return null;
  return conditions.find((c) => c?.type === type) ?? null;
}

export function readyText(item) {
  const c = findCondition(item, 'Ready');
  if (!c) return '—';
  if (c.status === 'True') return 'Ready';
  if (c.status === 'False') return 'Not ready';
  return c.status ?? '—';
}

export function approvedText(item) {
  const approved = findCondition(item, 'Approved');
  const denied = findCondition(item, 'Denied');
  if (denied?.status === 'True') return 'Denied';
  if (approved?.status === 'True') return 'Approved';
  return 'Pending';
}

// Cert-manager Issuer / ClusterIssuer types live as the first key under spec.
export function issuerType(item) {
  const spec = item?.spec ?? {};
  for (const key of ['acme', 'ca', 'selfSigned', 'vault', 'venafi']) {
    if (spec[key]) return key;
  }
  const keys = Object.keys(spec);
  return keys.length > 0 ? keys[0] : '—';
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
// relative to the iframe's current route, so the plugin only sends the
// resource identity (name + optional namespace).
export function navigateToDetail(name, namespace) {
  window.parent.postMessage({ type: 'plugin:navigate', name, namespace }, '*');
}

// Renders a key/value definition list for the given map. Returns HTML.
export function renderDefList(obj) {
  if (!obj || typeof obj !== 'object') return '';
  const rows = Object.entries(obj).map(
    ([k, v]) =>
      `<dt class="plugin-text">${escapeHtml(k)}</dt><dd>${escapeHtml(formatScalar(v))}</dd>`,
  );
  return `<dl class="plugin-deflist">${rows.join('')}</dl>`;
}

function formatScalar(v) {
  if (v === null || v === undefined) return '';
  if (typeof v === 'object') return JSON.stringify(v, null, 2);
  return String(v);
}

export function renderConditionsTable(item) {
  const conditions = item?.status?.conditions;
  if (!Array.isArray(conditions) || conditions.length === 0) {
    return `<p class="plugin-text">No conditions reported.</p>`;
  }
  const rows = conditions
    .map(
      (c) => `
        <tr>
          <td>${escapeHtml(c.type)}</td>
          <td>${escapeHtml(c.status)}</td>
          <td>${escapeHtml(c.reason ?? '')}</td>
          <td>${escapeHtml(c.message ?? '')}</td>
          <td>${escapeHtml(formatAge(c.lastTransitionTime))}</td>
        </tr>`,
    )
    .join('');
  return `
    <table class="plugin-table">
      <thead>
        <tr><th>Type</th><th>Status</th><th>Reason</th><th>Message</th><th>Age</th></tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}
