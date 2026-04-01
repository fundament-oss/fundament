/**
 * Fundament Plugin SDK
 *
 * This script is compiled to public/plugin-ui/plugin-sdk.js and served alongside plugin UIs.
 * Include it in every plugin HTML page to handle the standard host↔plugin message protocol.
 *
 * Host → plugin messages:
 *   - fundament:init          Initial context (theme, pluginName, crdKind, view)
 *   - fundament:theme-changed Theme switched by the user
 *
 * Plugin → host messages (sent automatically by this SDK):
 *   - plugin:ready            Signals the plugin has loaded
 *   - plugin:resize           Reports the content height so the host can resize the iframe
 */

type Theme = 'light' | 'dark';

type HostMessage =
  | {
      type: 'fundament:init';
      version: number;
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail';
    }
  | {
      type: 'fundament:theme-changed';
      theme: Theme;
    };

const PLUGIN_SDK_VERSION = 1;

function applyTheme(theme: Theme): void {
  document.body.classList.remove('light', 'dark');
  document.body.classList.add(theme);
}

let resizeTimer: ReturnType<typeof setTimeout> | undefined;

function reportHeight(): void {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    window.parent.postMessage(
      { type: 'plugin:resize', height: document.documentElement.scrollHeight },
      '*',
    );
  }, 50);
}

/** Resolves once all <link rel="stylesheet"> elements in the document have loaded. */
async function waitForStylesheets(): Promise<void> {
  const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'));
  await Promise.all(
    links.map((link) =>
      link.sheet
        ? Promise.resolve()
        : new Promise<void>((resolve) => {
            link.addEventListener(
              'load',
              () => {
                resolve();
              },
              { once: true },
            );
          }),
    ),
  );
}

window.addEventListener('message', (event: MessageEvent) => {
  const data = event.data as HostMessage;
  if (!data || typeof data.type !== 'string') return;

  if (data.type === 'fundament:init') {
    applyTheme(data.theme);
  } else if (data.type === 'fundament:theme-changed') {
    applyTheme(data.theme);
  }
});

const observer = new ResizeObserver(reportHeight);

waitForStylesheets().then(() => observer.observe(document.body));

window.parent.postMessage({ type: 'plugin:ready', version: PLUGIN_SDK_VERSION }, '*');
