import { Injectable } from '@angular/core';
import type { KubeResource, ParsedCrd } from './types';
import buildResourceUrl from './kube-url.utils';

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  // Cache: key = "${pluginName}/${kind}/${clusterId}"
  private cache = new Map<string, KubeResource[]>();

  async loadResources(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    pluginName: string,
  ): Promise<KubeResource[]> {
    const base = kubeApiProxyUrl.replace(/\/$/, '');
    // List across all namespaces via the collection endpoint (the same call custom plugin UIs
    // use). For namespaced CRDs this returns items from every namespace; the detail view
    // disambiguates by name + namespace.
    const url = buildResourceUrl(base, clusterId, {
      group: crd.group,
      version: crd.version,
      resource: crd.plural,
    });

    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch resources for ${crd.kind}: ${response.status}`);
    }

    const data = (await response.json()) as { items?: KubeResource[] };
    const resources = data.items ?? [];
    this.cache.set(`${pluginName}/${crd.kind}/${clusterId}`, resources);
    return resources;
  }

  async loadResource(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    pluginName: string,
    name: string,
    namespace: string | undefined,
  ): Promise<KubeResource | undefined> {
    // A namespaced object cannot be fetched by name alone; without a namespace
    // (e.g. a deep link missing ?ns=) fall back to listing and matching by name.
    if (crd.scope === 'Namespaced' && !namespace) {
      const all = await this.loadResources(crd, clusterId, kubeApiProxyUrl, pluginName);
      return all.find((r) => r.metadata.name === name);
    }

    // Cluster-scoped resources have no namespace segment; drop a stray ?ns=
    // (e.g. from a hand-crafted deep link) so we don't build an invalid URL.
    const ns = crd.scope === 'Namespaced' ? namespace : undefined;

    const base = kubeApiProxyUrl.replace(/\/$/, '');
    const url = buildResourceUrl(base, clusterId, {
      group: crd.group,
      version: crd.version,
      resource: crd.plural,
      namespace: ns,
      name,
    });

    const response = await fetch(url, {
      credentials: 'include',
    });

    if (response.status === 404) return undefined;
    if (!response.ok) {
      throw new Error(`Failed to fetch ${crd.kind} ${name}: ${response.status}`);
    }

    return (await response.json()) as KubeResource;
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
