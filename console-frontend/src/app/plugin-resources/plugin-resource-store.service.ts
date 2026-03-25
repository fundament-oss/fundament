import { Injectable } from '@angular/core';
import type { KubeResource, ParsedCrd } from './types';

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  // Cache: key = "${pluginName}/${kind}/${clusterId}"
  private cache = new Map<string, KubeResource[]>();

  async loadResources(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    orgId: string,
    pluginName: string,
  ): Promise<KubeResource[]> {
    const base = kubeApiProxyUrl.replace(/\/$/, '');
    // TODO: Support namespaced resources by adding /namespaces/{ns}/ when crd.scope === 'Namespaced'.
    // Currently fetches cluster-scoped list; real mode will return 404 for namespaced CRDs.
    const url = `${base}/k8s-api/apis/${crd.group}/${crd.version}/${crd.plural}`;

    const response = await fetch(url, {
      credentials: 'include',
      headers: { 'Fun-Organization': orgId, 'Fun-Cluster': clusterId },
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch resources for ${crd.kind}: ${response.status}`);
    }

    const data = (await response.json()) as { items?: KubeResource[] };
    const resources = data.items ?? [];
    this.cache.set(`${pluginName}/${crd.kind}/${clusterId}`, resources);
    return resources;
  }

  getResource(
    pluginName: string,
    kind: string,
    resourceId: string,
    clusterId: string | null | undefined,
  ): KubeResource | undefined {
    if (!clusterId) return undefined;
    const key = `${pluginName}/${kind}/${clusterId}`;
    return this.cache.get(key)?.find((r) => r.metadata.name === resourceId);
  }
}
