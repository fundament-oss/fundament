import { Injectable } from '@angular/core';
import type { KubeResource, ParsedCrd } from './types';

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  // Keyed by "${pluginName}/${kind}/${clusterId}"
  private resourceCache = new Map<string, KubeResource[]>();

  // In-flight promises to prevent duplicate concurrent requests for the same key
  private inFlight = new Map<string, Promise<void>>();

  async loadResources(
    pluginName: string,
    crd: ParsedCrd,
    clusterId: string,
    orgApiUrl: string,
    orgId: string,
  ): Promise<void> {
    const cacheKey = `${pluginName}/${crd.kind}/${clusterId}`;
    if (this.resourceCache.has(cacheKey)) return undefined;

    const existing = this.inFlight.get(cacheKey);
    if (existing) {
      await existing;
      return undefined;
    }

    const base = orgApiUrl.replace(/\/$/, '');
    const url = `${base}/k8s/${clusterId}/apis/${crd.group}/${crd.version}/${crd.plural}`;

    const promise = fetch(url, {
      credentials: 'include',
      headers: { 'Fun-Organization': orgId },
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`Failed to fetch resources for ${crd.kind}: ${response.status}`);
        }
        const data = (await response.json()) as { items?: KubeResource[] };
        this.resourceCache.set(cacheKey, data.items ?? []);
      })
      .finally(() => {
        this.inFlight.delete(cacheKey);
      });

    this.inFlight.set(cacheKey, promise);
    return promise;
  }

  listResources(pluginName: string, kind: string, clusterId: string): KubeResource[] {
    return this.resourceCache.get(`${pluginName}/${kind}/${clusterId}`) ?? [];
  }

  getResource(
    pluginName: string,
    kind: string,
    name: string,
    clusterId: string,
  ): KubeResource | undefined {
    return this.listResources(pluginName, kind, clusterId).find((r) => r.metadata.name === name);
  }

  /**
   * PATCH an existing resource. Sends a strategic-merge-patch with the updated spec.
   * Invalidates the in-memory cache so the list is refreshed on next load.
   */
  async patchResource(
    pluginName: string,
    crd: ParsedCrd,
    name: string,
    namespace: string | undefined,
    spec: Record<string, unknown>,
    clusterId: string,
    orgApiUrl: string,
    orgId: string,
  ): Promise<KubeResource> {
    const base = orgApiUrl.replace(/\/$/, '');
    const namespacePart = namespace ? `namespaces/${namespace}/` : '';
    const url = `${base}/k8s/${clusterId}/apis/${crd.group}/${crd.version}/${namespacePart}${crd.plural}/${name}`;

    const response = await fetch(url, {
      method: 'PATCH',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/strategic-merge-patch+json',
        'Fun-Organization': orgId,
      },
      body: JSON.stringify({ spec }),
    });

    if (!response.ok) {
      const text = await response.text().catch(() => response.statusText);
      throw new Error(`Failed to patch ${crd.kind}/${name}: ${response.status} ${text}`);
    }

    const updated = (await response.json()) as KubeResource;
    // Invalidate cache so next load fetches fresh data
    this.resourceCache.delete(`${pluginName}/${crd.kind}/${clusterId}`);
    return updated;
  }

  /**
   * POST a new resource. Invalidates the in-memory cache so the list is refreshed on next load.
   */
  async createResource(
    pluginName: string,
    crd: ParsedCrd,
    namespace: string | undefined,
    resource: Omit<KubeResource, 'metadata'> & { metadata: { name: string; namespace?: string } },
    clusterId: string,
    orgApiUrl: string,
    orgId: string,
  ): Promise<KubeResource> {
    const base = orgApiUrl.replace(/\/$/, '');
    const namespacePart = namespace ? `namespaces/${namespace}/` : '';
    const url = `${base}/k8s/${clusterId}/apis/${crd.group}/${crd.version}/${namespacePart}${crd.plural}`;

    const response = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'Fun-Organization': orgId,
      },
      body: JSON.stringify(resource),
    });

    if (!response.ok) {
      const text = await response.text().catch(() => response.statusText);
      throw new Error(`Failed to create ${crd.kind}: ${response.status} ${text}`);
    }

    const created = (await response.json()) as KubeResource;
    // Invalidate cache so next load fetches fresh data
    this.resourceCache.delete(`${pluginName}/${crd.kind}/${clusterId}`);
    return created;
  }
}
