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
        crdKind: 'StoragePool',
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
  group: 'storage.fundament.io',
  version: 'v1alpha1',
  resource: 'storagepools',
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
    await window.fundament.k8s.patch(POOL, { spec: { replication: '2' } });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];

    expect(url).toBe(
      'https://proxy.example/kube/clusters/cluster-1' +
        '/apis/storage.fundament.io/v1alpha1/storagepools/test-pool',
    );
    expect(init.method).toBe('PATCH');
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/merge-patch+json');
    expect(init.body).toBe(JSON.stringify({ spec: { replication: '2' } }));
  });

  it('delete sends DELETE with no body', async () => {
    await window.fundament.k8s.delete(POOL);

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/storagepools/test-pool');
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
