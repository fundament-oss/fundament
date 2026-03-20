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
    if (this.resourceCache.has(cacheKey)) return;

    const existing = this.inFlight.get(cacheKey);
    if (existing) {
      await existing;
      return;
    }

    const base = orgApiUrl.replace(/\/$/, '');
    const url = `${base}/k8sproxy/apis/${crd.group}/${crd.version}/${crd.plural}`;

    const promise = fetch(url, {
      credentials: 'include',
      headers: { 'Fun-Organization': orgId, 'Fun-Cluster': clusterId },
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
    await promise;
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

  clearResourceCache(pluginName: string, kind: string, clusterId: string): void {
    this.resourceCache.delete(`${pluginName}/${kind}/${clusterId}`);
  }
}
