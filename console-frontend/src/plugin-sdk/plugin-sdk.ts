/**
 * Fundament Plugin SDK
 *
 * Compiled to public/plugin-ui/plugin-sdk.js. Plugin HTML templates load this
 * script via <script src> and use the `window.fundament` API to read host
 * context, call the Kubernetes API through kube-api-proxy with an auto-attached
 * PluginToken, and react to theme / token / auth-failure events.
 *
 * Host ↔ plugin postMessage protocol:
 *
 *   Host → plugin:
 *     - fundament:init            Initial context + first PluginToken (protocolVersion 1).
 *     - fundament:theme-changed   Theme switched by the user.
 *     - fundament:token-refreshed New PluginToken (mint refresh).
 *     - fundament:auth-failed     Persistent mint failure; reject pending getToken() calls.
 *
 *   Plugin → host (mostly sent automatically by this SDK):
 *     - plugin:ready              Plugin loaded.
 *     - plugin:resize             Reports content height for iframe sizing.
 *     - plugin:navigate           Router hop to a sibling resource.
 *     - plugin:create             Router hop to the create route.
 *     - plugin:navigate-back      Router hop to the parent list.
 *     - plugin:request-token-refresh  Ask the host to mint a fresh token
 *                                     (sent when a plugin fetch returns 401).
 */

// GET_TOKEN_TIMEOUT_MS bounds any getToken() waiter — no plugin fetch should
// hang forever if the host never signals a refresh. Chosen to cover the
// host's exponential-backoff retry window (2s+4s+8s ≈ 14s) plus slack.
const GET_TOKEN_TIMEOUT_MS = 20_000;

// REQUEST_TIMEOUT_MS bounds each kube-api-proxy request. Without it a proxy
// that accepts the connection but never responds would leave k8s.list()/get()
// pending forever, hanging the plugin view on a spinner with no error. On
// expiry the fetch is aborted and surfaces as SdkError('timeout').
const REQUEST_TIMEOUT_MS = 30_000;

type Theme = 'light' | 'dark';

type AuthFailReason = 'mint_failed' | 'unauthorized' | 'revoked';

interface ResourceContext {
  name: string;
  namespace?: string;
}

interface InitContext {
  theme: Theme;
  pluginName: string;
  crdKind: string;
  view: 'list' | 'detail' | 'create';
  resource?: ResourceContext;
  namespaces?: string[];
  // FUN-17: plugin JS builds fetch URLs against kube-api-proxy from these.
  // fundament.fetch() automatically attaches the bearer PluginToken.
  kubeApiProxyUrl: string;
  clusterId: string;
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

type K8sCreateArgs = K8sListArgs;

interface KubeListResult<T = unknown> {
  items: T[];
}

class SdkError extends Error {
  constructor(
    public readonly code: 'unauthorized' | 'forbidden' | 'http' | 'transport' | 'timeout',
    message: string,
    public readonly status?: number,
  ) {
    super(message);
    this.name = 'SdkError';
  }
}

interface FundamentSdk {
  init: Promise<InitContext>;
  /**
   * The pinned console (parent frame) origin, captured from fundament:init's
   * event.origin. `null` before init has arrived. Use it as the targetOrigin
   * on any raw `window.parent.postMessage` calls the plugin makes — falling
   * back to `'*'` only for messages that carry no secrets.
   */
  readonly parentOrigin: string | null;
  getToken(): Promise<string>;
  fetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response>;
  k8s: {
    list<T = unknown>(args: K8sListArgs): Promise<KubeListResult<T>>;
    get<T = unknown>(args: K8sGetArgs): Promise<T>;
    create<T = unknown>(args: K8sCreateArgs, body: unknown): Promise<T>;
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
      protocolVersion: 1;
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail' | 'create';
      resource?: ResourceContext;
      namespaces?: string[];
      kubeApiProxyUrl: string;
      clusterId: string;
      token: string;
      tokenExpiresAt: number;
    }
  | { type: 'fundament:theme-changed'; theme: Theme }
  | { type: 'fundament:token-refreshed'; token: string; tokenExpiresAt: number }
  | { type: 'fundament:auth-failed'; reason: AuthFailReason };

let parentOrigin: string | null = null;

const themeListeners = new Set<(theme: Theme) => void>();

interface AuthState {
  token: string | null;
  expiresAt: number;
  waiters: Array<{ resolve: (t: string) => void; reject: (e: Error) => void }>;
  failed: { reason: AuthFailReason } | null;
}

const auth: AuthState = { token: null, expiresAt: 0, waiters: [], failed: null };

function deliverTokenToWaiters(): void {
  if (auth.token === null) return;
  const t = auth.token;
  const pending = auth.waiters.splice(0);
  for (const w of pending) w.resolve(t);
}

function failAllWaiters(reason: AuthFailReason): void {
  const pending = auth.waiters.splice(0);
  for (const w of pending) w.reject(new Error(`plugin auth failed: ${reason}`));
}

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

let resolveInit!: (ctx: InitContext) => void;
const initPromise = new Promise<InitContext>((resolve) => {
  resolveInit = resolve;
});

let initResolved = false;

function handleHostMessage(data: HostMessage): void {
  if (data.type === 'fundament:init') {
    if (data.protocolVersion !== 1) return; // unknown protocol — ignore
    if (!initResolved) {
      initResolved = true;
      resolveInit({
        theme: data.theme,
        pluginName: data.pluginName,
        crdKind: data.crdKind,
        view: data.view,
        resource: data.resource,
        namespaces: data.namespaces,
        kubeApiProxyUrl: data.kubeApiProxyUrl,
        clusterId: data.clusterId,
      });
    }
    applyTheme(data.theme);
    auth.token = data.token;
    auth.expiresAt = data.tokenExpiresAt;
    auth.failed = null;
    deliverTokenToWaiters();
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

  if (data.type === 'fundament:token-refreshed') {
    auth.token = data.token;
    auth.expiresAt = data.tokenExpiresAt;
    auth.failed = null;
    deliverTokenToWaiters();
    return;
  }

  if (data.type === 'fundament:auth-failed') {
    auth.token = null;
    auth.failed = { reason: data.reason };
    failAllWaiters(data.reason);
  }
}

window.addEventListener('message', (event: MessageEvent) => {
  // Only accept messages that came from the direct parent window. Without
  // this, a popup opened by the plugin, an ancestor other than the console,
  // or another same-origin plugin iframe on plugin-proxy could postMessage
  // us. Combined with the origin check below, this pins the sender to the
  // one window we trust to deliver init.
  if (event.source !== window.parent) return;

  // Once init has arrived, pin origin. Before init we still enforce that
  // the sender is window.parent — the console iframe was navigated to a
  // known origin by the host, and if that origin ever mismatches the one
  // that eventually delivers init we drop everything.
  if (parentOrigin !== null && event.origin !== parentOrigin) return;

  const data = event.data as HostMessage;
  if (!data || typeof data.type !== 'string') return;

  if (parentOrigin === null) {
    // Only the init message is allowed to pin parentOrigin. Any other
    // message before init is out-of-order — drop it.
    if (data.type !== 'fundament:init') return;
    parentOrigin = event.origin;
  }

  handleHostMessage(data);
});

const observer = new ResizeObserver(reportHeight);

waitForStylesheets().then(() => observer.observe(document.body));

function getTokenImpl(): Promise<string> {
  if (auth.failed) {
    return Promise.reject(new Error(`plugin auth failed: ${auth.failed.reason}`));
  }
  if (auth.token !== null) return Promise.resolve(auth.token);
  return new Promise<string>((resolve, reject) => {
    // Timer bounds the wait so a lost refresh signal surfaces as an error
    // instead of an indefinite spinner. Waiter identity is preserved so
    // handleHostMessage's fan-out can still resolve us if the host does mint.
    let settled = false;
    let timerId: ReturnType<typeof setTimeout> | undefined;
    const waiter = {
      resolve: (t: string): void => {
        if (settled) return;
        settled = true;
        if (timerId !== undefined) clearTimeout(timerId);
        const idx = auth.waiters.indexOf(waiter);
        if (idx >= 0) auth.waiters.splice(idx, 1);
        resolve(t);
      },
      reject: (e: Error): void => {
        if (settled) return;
        settled = true;
        if (timerId !== undefined) clearTimeout(timerId);
        const idx = auth.waiters.indexOf(waiter);
        if (idx >= 0) auth.waiters.splice(idx, 1);
        reject(e);
      },
    };
    auth.waiters.push(waiter);
    timerId = setTimeout(() => {
      waiter.reject(new Error('plugin auth: getToken timed out waiting for host mint'));
    }, GET_TOKEN_TIMEOUT_MS);
  });
}

// requestHostRefresh asks the host to mint a fresh token. The response
// arrives asynchronously via fundament:token-refreshed (or
// fundament:auth-failed if the host gives up). Guarded by parentOrigin so we
// never post to '*' — the token in play is sensitive.
function requestHostRefresh(): void {
  if (parentOrigin === null) return;
  window.parent.postMessage({ type: 'plugin:request-token-refresh' }, parentOrigin);
}

async function fetchImpl(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const call = async (): Promise<Response> => {
    const token = await getTokenImpl();
    const headers = new Headers(init?.headers);
    headers.set('Authorization', `Bearer ${token}`);

    // Bound the request so a stalled proxy can't hang the plugin forever.
    // Chain any caller-supplied signal so their abort still works.
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
    const callerSignal = init?.signal;
    if (callerSignal) {
      if (callerSignal.aborted) controller.abort();
      else callerSignal.addEventListener('abort', () => controller.abort(), { once: true });
    }
    try {
      return await fetch(input, { ...init, headers, signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
  };

  const res = await call();
  if (res.status !== 401) return res;

  // Token was rejected upstream. Clear the cached token AND ask the host to
  // mint a new one — without the explicit request, the host's next refresh
  // wouldn't fire until the timer scheduled off the old (now-invalid) token
  // expires, which could be several minutes away or never if the token was
  // revoked. The getToken() waiter is bounded by GET_TOKEN_TIMEOUT_MS so we
  // still surface an error if the host never signals back.
  auth.token = null;
  requestHostRefresh();
  return call();
}

// Builds a URL against kube-api-proxy from the init context. Mirrors the shape
// the previous host-brokered path used:
//   ${kubeApiProxyUrl}/clusters/${clusterId}/apis/${group}/${version}/[namespaces/${ns}/]${resource}[/${name}]
// Empty (core) group swaps /apis/${group}/${version} for /api/${version}.
function buildKubeUrl(ctx: InitContext, args: K8sListArgs & { name?: string }): string {
  const base = ctx.kubeApiProxyUrl.replace(/\/$/, '');
  const cluster = `/clusters/${encodeURIComponent(ctx.clusterId)}`;
  const groupPart = args.group ? `/apis/${args.group}/${args.version}` : `/api/${args.version}`;
  const scope = args.namespace ? `/namespaces/${encodeURIComponent(args.namespace)}` : '';
  const nameSuffix = args.name ? `/${encodeURIComponent(args.name)}` : '';
  return `${base}${cluster}${groupPart}${scope}/${args.resource}${nameSuffix}`;
}

// Kubernetes reports write rejections (409 conflict, 422 validation/CEL) in the
// Status body's `message` — surface it so the plugin form can show the real reason.
async function readK8sError(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { message?: unknown };
    if (typeof body.message === 'string' && body.message) return body.message;
  } catch {
    // body wasn't JSON — fall through
  }
  return response.statusText || `HTTP ${response.status}`;
}

async function k8sRequest<T>(
  method: 'GET' | 'POST',
  args: K8sListArgs & { name?: string },
  body?: unknown,
): Promise<T> {
  const ctx = await initPromise;
  const url = buildKubeUrl(ctx, args);
  let res: Response;
  try {
    res = await fetchImpl(url, {
      method,
      ...(body !== undefined
        ? { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
        : {}),
    });
  } catch (err) {
    // k8sRequest never passes a caller signal, so an AbortError here can only
    // be our REQUEST_TIMEOUT_MS firing.
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new SdkError('timeout', `request timed out after ${REQUEST_TIMEOUT_MS}ms`);
    }
    throw new SdkError('transport', err instanceof Error ? err.message : 'transport error');
  }

  if (!res.ok) {
    const message = await readK8sError(res);
    // A 401 that survives fetchImpl's refresh-and-retry means the freshly
    // minted token was itself rejected. Surface it as a distinct 'unauthorized'
    // code (not a generic 'http') so the plugin can prompt re-auth instead of
    // showing an opaque HTTP error.
    const code = res.status === 401 ? 'unauthorized' : res.status === 403 ? 'forbidden' : 'http';
    throw new SdkError(code, message, res.status);
  }
  return (await res.json()) as T;
}

const sdk: FundamentSdk = {
  init: initPromise,
  get parentOrigin(): string | null {
    return parentOrigin;
  },
  getToken: getTokenImpl,
  fetch: fetchImpl,
  k8s: {
    async list<T = unknown>(args: K8sListArgs): Promise<KubeListResult<T>> {
      const data = await k8sRequest<{ items?: T[] }>('GET', args);
      return { items: data.items ?? [] };
    },
    get<T = unknown>(args: K8sGetArgs): Promise<T> {
      return k8sRequest<T>('GET', args);
    },
    create<T = unknown>(args: K8sCreateArgs, body: unknown): Promise<T> {
      return k8sRequest<T>('POST', args, body);
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
