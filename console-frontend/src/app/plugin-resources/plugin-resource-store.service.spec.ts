import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import PluginResourceStoreService from './plugin-resource-store.service';
import type { ParsedCrd } from './types';

const base = 'https://proxy.example';

function crd(scope: 'Namespaced' | 'Cluster'): ParsedCrd {
  return {
    group: 'db.example.io',
    version: 'v1',
    kind: 'Database',
    plural: 'databases',
    scope,
  } as ParsedCrd;
}

describe('PluginResourceStoreService.loadResources', () => {
  let store: PluginResourceStoreService;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [{ metadata: { name: 'a' } }] }),
    });
    vi.stubGlobal('fetch', fetchMock);
    TestBed.configureTestingModule({});
    store = TestBed.inject(PluginResourceStoreService);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const calledUrl = () => fetchMock.mock.calls[0][0] as string;

  it('lists a namespaced resource in one namespace', async () => {
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p', {
      projectId: 'pr1',
      namespace: 'tnt-p--web',
    });
    expect(calledUrl()).toBe(
      `${base}/clusters/c1/apis/db.example.io/v1/namespaces/tnt-p--web/databases`,
    );
  });

  it('filters a namespaced resource by project', async () => {
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p', { projectId: 'pr1' });
    expect(calledUrl()).toBe(
      `${base}/clusters/c1/apis/db.example.io/v1/databases?fundamentProjectId=pr1`,
    );
  });

  it('lists the whole collection for a cluster-scoped resource or without a scope', async () => {
    await store.loadResources(crd('Cluster'), 'c1', base, 'p', { projectId: 'pr1' });
    expect(calledUrl()).toBe(`${base}/clusters/c1/apis/db.example.io/v1/databases`);

    fetchMock.mockClear();
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p');
    expect(calledUrl()).toBe(`${base}/clusters/c1/apis/db.example.io/v1/databases`);
  });

  it('keeps project and namespace listings apart in the cache', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [{ metadata: { name: 'x' } }] }),
    });
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p', { projectId: 'pr1' });
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p', {
      projectId: 'pr1',
      namespace: 'n1',
    });
    expect(store.getResource('p', 'Database', 'x', 'c1')).toBeDefined();
    expect(store.getResource('p', 'Database', 'a', 'c1')).toBeDefined();

    fetchMock.mockClear();
    await store.loadResources(crd('Namespaced'), 'c1', base, 'p');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe('PluginResourceStoreService.loadResource', () => {
  let store: PluginResourceStoreService;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [{ metadata: { name: 'a' } }, { metadata: { name: 'b' } }] }),
    });
    vi.stubGlobal('fetch', fetchMock);
    TestBed.configureTestingModule({});
    store = TestBed.inject(PluginResourceStoreService);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('falls back to a project-filtered list when the namespace is missing', async () => {
    const found = await store.loadResource(crd('Namespaced'), 'c1', base, 'p', 'b', undefined, {
      projectId: 'p1',
    });
    expect(fetchMock.mock.calls[0][0]).toBe(
      `${base}/clusters/c1/apis/db.example.io/v1/databases?fundamentProjectId=p1`,
    );
    expect(found?.metadata.name).toBe('b');
  });

  it('falls back to an unfiltered list without a project scope', async () => {
    const found = await store.loadResource(crd('Namespaced'), 'c1', base, 'p', 'b', undefined);
    expect(fetchMock.mock.calls[0][0]).toBe(`${base}/clusters/c1/apis/db.example.io/v1/databases`);
    expect(found?.metadata.name).toBe('b');
  });
});
