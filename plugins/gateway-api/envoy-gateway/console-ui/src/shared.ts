// Shared helpers for the Envoy Gateway console views. The plugin ships only a
// Gateway create form, so this trims the openfsc shared.ts to loading, escaping
// and host navigation.

import type { FundamentSdk } from './sdk.ts';

function whenSettled(el: HTMLLinkElement | HTMLScriptElement, what: string): Promise<void> {
  return new Promise((resolve, reject) => {
    el.addEventListener('load', () => resolve(), { once: true });
    el.addEventListener('error', () => reject(new Error(`failed to load ${what}`)), { once: true });
  });
}

// Loads the plugin-proxy's /plugins/sdk/v1/<base>.{css,js} pair (same origin as
// the iframe under FUN-17). See docs/funs/FUN-18.adoc.
function loadPluginAsset(base: string): Promise<void> {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `/plugins/sdk/v1/${base}.css`;
  const css = whenSettled(link, `${base}.css`);
  document.head.appendChild(link);

  const script = document.createElement('script');
  script.src = `/plugins/sdk/v1/${base}.js`;
  const js = whenSettled(script, `${base}.js`);
  document.head.appendChild(script);

  return Promise.all([css, js]).then(() => undefined);
}

export function loadSdk(): Promise<FundamentSdk> {
  return loadPluginAsset('plugin-sdk').then(() => window.fundament);
}

function syncNlddDesignSystemTheme(): void {
  const dark = document.body.classList.contains('dark');
  document.documentElement.setAttribute('data-scheme', dark ? 'dark' : 'light');
}

export function loadNlddDesignSystem(): Promise<void> {
  syncNlddDesignSystemTheme();
  new MutationObserver(syncNlddDesignSystemTheme).observe(document.body, {
    attributes: true,
    attributeFilter: ['class'],
  });
  return loadPluginAsset('nldd-design-system');
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

function postToHost(message: unknown): void {
  window.parent.postMessage(message, window.fundament?.parentOrigin ?? '*');
}

export function navigateToDetail(
  name: string | null | undefined,
  namespace: string | null | undefined,
): void {
  postToHost({ type: 'plugin:navigate', name, namespace });
}

export function navigateBack(): void {
  postToHost({ type: 'plugin:navigate-back' });
}
