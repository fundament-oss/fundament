// Shared helpers for the OpenFSC console views.

import type { FundamentSdk } from './sdk.ts';
import type { Condition, FSCInstallation, GatewayStatus } from './types.ts';

// The Console origin, taken from the `?host=` param the host sets on the iframe
// src. Empty string when unframed (the dev preview), so asset URLs stay relative.
export function hostOrigin(): string {
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

// Loads the host's `/plugin-ui/<base>.{css,js}` pair. Injected at runtime because
// the sandboxed iframe only learns the host origin from `?host=`; classic
// <link>/<script> load cross-origin into the opaque-origin iframe without CORS.
function loadPluginAsset(base: string): Promise<void> {
  const host = hostOrigin();
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `${host}/plugin-ui/${base}.css`;
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = `${host}/plugin-ui/${base}.js`;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`failed to load ${base}.js`));
    document.head.appendChild(script);
  });
}

export function loadSdk(): Promise<FundamentSdk> {
  return loadPluginAsset('plugin-sdk').then(() => window.fundament);
}

// NLDS reads light/dark from `:root[data-scheme]`; mirror the SDK's body
// `.light`/`.dark` class (set on init and on every theme change) onto it.
function syncNldsTheme(): void {
  const dark = document.body.classList.contains('dark');
  document.documentElement.setAttribute('data-scheme', dark ? 'dark' : 'light');
}

// Loads the shared NLDS bundle from the host; every <nldd-*> element is registered
// once nldd.js has run. Views opt in by calling this alongside loadSdk().
export function loadNlds(): Promise<void> {
  syncNldsTheme();
  new MutationObserver(syncNldsTheme).observe(document.body, {
    attributes: true,
    attributeFilter: ['class'],
  });
  return loadPluginAsset('nldd');
}

export function escapeHtml(value: unknown): string {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export function formatAge(creationTimestamp: string | undefined): string {
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

export function phase(item: FSCInstallation): string {
  return item?.status?.phase ?? '—';
}

export function emptyRow(colspan: number, message = 'No items.'): string {
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(message)}</td></tr>`;
}

export function errorRow(colspan: number, err: unknown): string {
  const message = err instanceof Error ? err.message : String(err);
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(`Failed to load: ${message}`)}</td></tr>`;
}

// Posts a message to the host (Console). The ?host= origin scopes it; the
// console-preview stand-in runs unframed, hence the same-origin fallback.
function postToHost(message: unknown): void {
  window.parent.postMessage(message, hostOrigin() || window.location.origin);
}

// The host resolves the destination relative to the iframe's current route, so the
// view only sends the resource identity.
export function navigateToDetail(
  name: string | null | undefined,
  namespace: string | null | undefined,
): void {
  postToHost({ type: 'plugin:navigate', name, namespace });
}

export function navigateToCreate(): void {
  postToHost({ type: 'plugin:create' });
}

export function navigateBack(): void {
  postToHost({ type: 'plugin:navigate-back' });
}

function formatScalar(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'boolean') return v ? 'Yes' : 'No';
  if (typeof v === 'object') return JSON.stringify(v, null, 2);
  return String(v);
}

// Renders a key/value definition list for the given map. Returns HTML.
export function renderDefList(obj: Record<string, unknown> | null | undefined): string {
  if (!obj || typeof obj !== 'object') return '';
  const rows = Object.entries(obj).map(
    ([k, v]) => `<dt class="plugin-text">${escapeHtml(k)}</dt><dd>${escapeHtml(formatScalar(v))}</dd>`,
  );
  return `<dl class="plugin-deflist">${rows.join('')}</dl>`;
}

// Renders a `.plugin-table` with the given column headers, one row per entry;
// `cells` maps an entry to its column values (each escaped). Shows `empty` as a
// paragraph when there are no entries.
function renderTable<T>(
  entries: T[] | undefined,
  headers: string[],
  cells: (entry: T) => unknown[],
  empty: string,
): string {
  if (!Array.isArray(entries) || entries.length === 0) {
    return `<p class="plugin-text">${escapeHtml(empty)}</p>`;
  }
  const head = headers.map((h) => `<th>${escapeHtml(h)}</th>`).join('');
  const body = entries
    .map((e) => `<tr>${cells(e).map((c) => `<td>${escapeHtml(c)}</td>`).join('')}</tr>`)
    .join('');
  return `
    <table class="plugin-table">
      <thead><tr>${head}</tr></thead>
      <tbody>${body}</tbody>
    </table>`;
}

export function renderConditionsTable(item: FSCInstallation): string {
  return renderTable<Condition>(
    item?.status?.conditions,
    ['Type', 'Status', 'Reason', 'Message', 'Age'],
    (c) => [c.type, c.status, c.reason ?? '', c.message ?? '', formatAge(c.lastTransitionTime)],
    'No conditions reported.',
  );
}

// Renders the per-gateway entries of status.inways / status.outways: the declared
// gateways with their registration state.
export function renderGatewayTable(entries: GatewayStatus[] | undefined): string {
  return renderTable<GatewayStatus>(
    entries,
    ['Name', 'Phase', 'URL', 'Message'],
    (g) => [g.name, g.phase ?? '—', g.url ?? '', g.message ?? ''],
    'None declared.',
  );
}
