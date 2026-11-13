import { Injectable } from '@angular/core';
import type { KubeResource, ParsedCrd } from './types';
import buildResourceUrl from './kube-url.utils';
import { KubeApiError } from './kube-api-error';

/** Narrows a list to a project, and optionally to one of its namespaces. */
export interface ListScope {
  projectId: string;
  namespace?: string;
}

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  // Cache: key = "${pluginName}/${kind}/${clusterId}", plus "/${projectId}/${namespace}"
  // for a project listing
  private cache = new Map<string, KubeResource[]>();

  private static key(
    pluginName: string,
    kind: string,
    clusterId: string,
    scope?: ListScope,
  ): string {
    const base = `${pluginName}/${kind}/${clusterId}`;
    return scope ? `${base}/${scope.projectId}/${scope.namespace ?? ''}` : base;
  }

  async loadResources(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    pluginName: string,
    scope?: ListScope,
  ): Promise<KubeResource[]> {
    const base = kubeApiProxyUrl.replace(/\/$/, '');
    // Without a scope, list across all namespaces via the collection endpoint (the same call
    // custom plugin UIs use); the detail view disambiguates by name + namespace. A project
    // scope limits a namespaced CRD to the project's namespaces, or to one of them.
    const projectScope = crd.scope === 'Namespaced' ? scope : undefined;
    const url = buildResourceUrl(base, clusterId, {
      group: crd.group,
      version: crd.version,
      resource: crd.plural,
      namespace: projectScope?.namespace,
      projectId: projectScope?.namespace ? undefined : projectScope?.projectId,
    });

    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch resources for ${crd.kind}: ${response.status}`);
    }

    const data = (await response.json()) as { items?: KubeResource[] };
    const resources = data.items ?? [];
    this.cache.set(
      PluginResourceStoreService.key(pluginName, crd.kind, clusterId, projectScope),
      resources,
    );
    return resources;
  }

  async loadResource(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    pluginName: string,
    name: string,
    namespace: string | undefined,
    scope?: { projectId: string },
  ): Promise<KubeResource | undefined> {
    // A namespaced object cannot be fetched by name alone; without a namespace
    // (e.g. a deep link missing ?ns=) fall back to listing and matching by name,
    // within the project on a project route.
    if (crd.scope === 'Namespaced' && !namespace) {
      const all = await this.loadResources(crd, clusterId, kubeApiProxyUrl, pluginName, scope);
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

  async deleteResource(
    crd: ParsedCrd,
    clusterId: string,
    kubeApiProxyUrl: string,
    pluginName: string,
    name: string,
    namespace: string | undefined,
  ): Promise<void> {
    // Cluster-scoped resources have no namespace segment; drop a stray namespace.
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
      method: 'DELETE',
      credentials: 'include',
    });

    // A 404 means it's already gone — treat that as success (idempotent delete).
    if (!response.ok && response.status !== 404) {
      throw new KubeApiError(
        `Failed to delete ${crd.kind} ${name}: ${response.status}`,
        response.status,
      );
    }

    const prefix = PluginResourceStoreService.key(pluginName, crd.kind, clusterId);
    [...this.cache.keys()]
      .filter((k) => k === prefix || k.startsWith(`${prefix}/`))
      .forEach((k) => this.cache.delete(k));
  }

  getResource(
    pluginName: string,
    kind: string,
    resourceId: string,
    clusterId: string | null | undefined,
  ): KubeResource | undefined {
    if (!clusterId) return undefined;
    const prefix = PluginResourceStoreService.key(pluginName, kind, clusterId);
    return [...this.cache.entries()]
      .filter(([k]) => k === prefix || k.startsWith(`${prefix}/`))
      .flatMap(([, resources]) => resources)
      .find((r) => r.metadata.name === resourceId);
  }
}
