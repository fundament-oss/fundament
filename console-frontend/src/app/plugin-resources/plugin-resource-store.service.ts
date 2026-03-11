import { Injectable } from '@angular/core';
import type { KubeResource, ParsedCrd } from './types';

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  // Keyed by "${pluginName}/${kind}/${clusterId}"
  private resourceCache = new Map<string, KubeResource[]>();

  async loadResources(
    pluginName: string,
    crd: ParsedCrd,
    clusterId: string,
    orgApiUrl: string,
    orgId: string,
  ): Promise<void> {
    const cacheKey = `${pluginName}/${crd.kind}/${clusterId}`;
    if (this.resourceCache.has(cacheKey)) return;

    const base = orgApiUrl.replace(/\/$/, '');
    const url = `${base}/k8s/${clusterId}/apis/${crd.group}/${crd.version}/${crd.plural}`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: { 'Fun-Organization': orgId },
    });
    if (!response.ok) {
      throw new Error(`Failed to fetch resources for ${crd.kind}: ${response.status}`);
    }

    const data = (await response.json()) as { items?: KubeResource[] };
    this.resourceCache.set(cacheKey, data.items ?? []);
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
}
