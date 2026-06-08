/**
 * Fundament Plugin SDK
 *
 * Compiled to public/plugin-ui/plugin-sdk.js. Plugin templates load this
 * script and use the `window.fundament` API to read host context, broker
 * Kubernetes API calls through the host, and react to theme changes.
 *
 * Host ↔ plugin postMessage protocol:
 *
 *   Host → plugin:
 *     - fundament:init           Initial context (theme, plugin, view, optional resource).
 *     - fundament:theme-changed  Theme switched by the user.
 *     - fundament:k8s:result     Reply for a previous plugin:k8s:* request.
 *
 *   Plugin → host (most sent automatically by this SDK):
 *     - plugin:ready             Plugin loaded.
 *     - plugin:resize            Reports content height for iframe sizing.
 *     - plugin:k8s:list          Request a Kubernetes list (brokered by host).
 *     - plugin:k8s:get           Request a Kubernetes get (brokered by host).
 */

type Theme = 'light' | 'dark';

interface ResourceContext {
  name: string;
  namespace?: string;
}

interface InitContext {
  theme: Theme;
  pluginName: string;
  crdKind: string;
  view: 'list' | 'detail';
  resource?: ResourceContext;
}

interface K8sListArgs {
  group: string;
  version: string;
  resource: string;
  namespace?: string;
}

interface K8sGetArgs extends K8sListArgs {
  name: string;
}

interface KubeListResult<T = unknown> {
  items: T[];
}

class SdkError extends Error {
  constructor(
    public readonly code: 'forbidden' | 'http' | 'timeout' | 'transport',
    message: string,
    public readonly status?: number,
  ) {
    super(message);
    this.name = 'SdkError';
  }
}

interface FundamentSdk {
  init: Promise<InitContext>;
  k8s: {
    list<T = unknown>(args: K8sListArgs): Promise<KubeListResult<T>>;
    get<T = unknown>(args: K8sGetArgs): Promise<T>;
  };
  onThemeChange(cb: (theme: Theme) => void): () => void;
}

declare global {
  interface Window {
    fundament: FundamentSdk;
  }
}

type HostMessage =
  | {
      type: 'fundament:init';
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail';
      resource?: ResourceContext;
    }
  | { type: 'fundament:theme-changed'; theme: Theme }
  | {
      type: 'fundament:k8s:result';
      requestId: string;
      ok: true;
      items?: unknown[];
      item?: unknown;
    }
  | {
      type: 'fundament:k8s:result';
      requestId: string;
      ok: false;
      error: string;
      status?: number;
    };

const REQUEST_TIMEOUT_MS = 10_000;

let parentOrigin: string | null = null;

const themeListeners = new Set<(theme: Theme) => void>();

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
      parentOrigin ?? '*',
    );
  }, 50);
}

async function waitForStylesheets(): Promise<void> {
  const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'));
  await Promise.all(
    links.map((link) =>
      link.sheet
        ? Promise.resolve()
        : new Promise<void>((resolve) => {
            link.addEventListener('load', () => resolve(), { once: true });
            link.addEventListener('error', () => resolve(), { once: true });
          }),
    ),
  );
}

interface Pending {
  resolve: (value: unknown) => void;
  reject: (err: SdkError) => void;
  timer: ReturnType<typeof setTimeout>;
  kind: 'list' | 'get';
}

const pendingRequests = new Map<string, Pending>();

let resolveInit!: (ctx: InitContext) => void;
const initPromise = new Promise<InitContext>((resolve) => {
  resolveInit = resolve;
});

let initResolved = false;

function generateRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `req-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function sendK8sRequest<T>(kind: 'list' | 'get', payload: Record<string, unknown>): Promise<T> {
  const requestId = generateRequestId();
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      pendingRequests.delete(requestId);
      reject(new SdkError('timeout', `request timed out after ${REQUEST_TIMEOUT_MS}ms`));
    }, REQUEST_TIMEOUT_MS);

    pendingRequests.set(requestId, {
      resolve: resolve as (v: unknown) => void,
      reject,
      timer,
      kind,
    });

    const message = {
      type: kind === 'list' ? 'plugin:k8s:list' : 'plugin:k8s:get',
      requestId,
      ...payload,
    };

    window.parent.postMessage(message, parentOrigin ?? '*');
  });
}

function handleHostMessage(data: HostMessage): void {
  if (data.type === 'fundament:init') {
    if (!initResolved) {
      initResolved = true;
      resolveInit({
        theme: data.theme,
        pluginName: data.pluginName,
        crdKind: data.crdKind,
        view: data.view,
        resource: data.resource,
      });
    }
    applyTheme(data.theme);
    return;
  }

  if (data.type === 'fundament:theme-changed') {
    applyTheme(data.theme);
    themeListeners.forEach((cb) => {
      try {
        cb(data.theme);
      } catch {
        // listeners must not crash the SDK
      }
    });
    return;
  }

  if (data.type === 'fundament:k8s:result') {
    const pending = pendingRequests.get(data.requestId);
    if (!pending) return;
    pendingRequests.delete(data.requestId);
    clearTimeout(pending.timer);

    if (!data.ok) {
      const code = data.error === 'forbidden' ? 'forbidden' : 'http';
      pending.reject(new SdkError(code, data.error, data.status));
      return;
    }

    if (pending.kind === 'list') {
      pending.resolve({ items: data.items ?? [] });
    } else {
      pending.resolve(data.item);
    }
  }
}

window.addEventListener('message', (event: MessageEvent) => {
  if (parentOrigin !== null && event.origin !== parentOrigin) return;

  const data = event.data as HostMessage;
  if (!data || typeof data.type !== 'string') return;

  if (parentOrigin === null && data.type === 'fundament:init') {
    parentOrigin = event.origin;
  }

  handleHostMessage(data);
});

const observer = new ResizeObserver(reportHeight);

waitForStylesheets().then(() => observer.observe(document.body));

const sdk: FundamentSdk = {
  init: initPromise,
  k8s: {
    list<T = unknown>(args: K8sListArgs): Promise<KubeListResult<T>> {
      return sendK8sRequest<KubeListResult<T>>('list', args as unknown as Record<string, unknown>);
    },
    get<T = unknown>(args: K8sGetArgs): Promise<T> {
      return sendK8sRequest<T>('get', args as unknown as Record<string, unknown>);
    },
  },
  onThemeChange(cb) {
    themeListeners.add(cb);
    return () => {
      themeListeners.delete(cb);
    };
  },
};

window.fundament = sdk;

// '*' is intentional: parentOrigin is not yet known at this point (fundament:init hasn't arrived),
// and this message carries no sensitive data.
window.parent.postMessage({ type: 'plugin:ready' }, '*');

export {};
