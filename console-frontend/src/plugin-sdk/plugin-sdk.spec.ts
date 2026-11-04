import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

/** The host handshake the SDK waits for before any k8s call resolves. */
function sendInit(): void {
  window.dispatchEvent(
    new MessageEvent('message', {
      source: window,
      origin: window.location.origin,
      data: {
        type: 'fundament:init',
        protocolVersion: 1,
        theme: 'light',
        pluginName: 'ceph-rook',
        crdKind: 'DiskPool',
        view: 'detail',
        kubeApiProxyUrl: 'https://proxy.example/kube',
        clusterId: 'cluster-1',
        token: 'test-token',
        tokenExpiresAt: Date.now() + 600_000,
      },
    }),
  );
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const POOL = {
  group: 'ceph.fundament.io',
  version: 'v1alpha1',
  resource: 'diskpools',
  name: 'test-pool',
};

describe('plugin-sdk k8s write verbs', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    fetchMock = vi.fn(() => Promise.resolve(jsonResponse({ ok: true })));
    vi.stubGlobal('fetch', fetchMock);
    await import('./plugin-sdk');
    sendInit();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('patch sends a merge-patch to the named resource URL', async () => {
    await window.fundament.k8s.patch(POOL, { spec: { replicas: 2 } });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];

    expect(url).toBe(
      'https://proxy.example/kube/clusters/cluster-1' +
        '/apis/ceph.fundament.io/v1alpha1/diskpools/test-pool',
    );
    expect(init.method).toBe('PATCH');
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/merge-patch+json');
    expect(init.body).toBe(JSON.stringify({ spec: { replicas: 2 } }));
  });

  it('delete sends DELETE with no body', async () => {
    await window.fundament.k8s.delete(POOL);

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/diskpools/test-pool');
    expect(init.method).toBe('DELETE');
    expect(init.body).toBeUndefined();
  });

  it('create still sends application/json', async () => {
    await window.fundament.k8s.create(
      { group: POOL.group, version: POOL.version, resource: POOL.resource },
      { metadata: { name: 'p' } },
    );

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe('POST');
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
  });

  it('surfaces the Kubernetes Status message on a rejected write', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ kind: 'Status', message: 'spec.disks: must not be empty' }, 422),
    );

    await expect(window.fundament.k8s.patch(POOL, { spec: { disks: [] } })).rejects.toThrow(
      /must not be empty/,
    );
  });
});

describe('plugin-sdk sheets', () => {
  let postMessage: { mock: { calls: unknown[][] }; mockRestore(): void };

  beforeEach(async () => {
    await import('./plugin-sdk');
    sendInit();
    await window.fundament.init;
    postMessage = vi.spyOn(window.parent, 'postMessage') as unknown as typeof postMessage;
  });

  afterEach(() => {
    postMessage.mockRestore();
  });

  /** The messages the SDK posted to the console, once init's promise ran. */
  async function posted(): Promise<unknown[]> {
    await window.fundament.init;
    await Promise.resolve();
    return postMessage.mock.calls.map((call: unknown[]) => call[0]);
  }

  it('asks the console for a sheet, to its pinned origin only', async () => {
    window.fundament.openSheet({ view: 'edit', resource: { name: 'test-pool' } });

    expect(await posted()).toContainEqual({
      type: 'plugin:open-sheet',
      view: 'edit',
      resource: { name: 'test-pool' },
    });
    expect(postMessage.mock.calls.at(-1)?.[1]).toBe(window.location.origin);
  });

  it('cannot be talked into posting another message type', async () => {
    window.fundament.openSheet({ view: 'create', type: 'plugin:navigate' } as never);

    expect(await posted()).toContainEqual({ type: 'plugin:open-sheet', view: 'create' });
  });

  // sendInit opens the page on a route, which a page in a console from before
  // sheets is too: it has no presentation to say otherwise.
  it('leaves a page on a route by going back, not by closing a sheet', async () => {
    window.fundament.closeSheet({ changed: true });

    await Promise.resolve();
    expect(await posted()).toContainEqual({ type: 'plugin:navigate-back' });
  });

  it('tells its listeners when a sheet over the page saved something', () => {
    const listener = vi.fn();
    const unsubscribe = window.fundament.onResourcesChanged(listener);

    window.dispatchEvent(
      new MessageEvent('message', {
        source: window,
        origin: window.location.origin,
        data: { type: 'fundament:resources-changed' },
      }),
    );
    unsubscribe();

    expect(listener).toHaveBeenCalledTimes(1);
  });
});
