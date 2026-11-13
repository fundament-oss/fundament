// Demo-only stand-in for PluginResourceStoreService. The real one lists and gets
// the plugin's custom resources from the cluster with fetch() against
// kube-api-proxy; the static demo has no such origin and its CSP blocks the
// request, so the objects come from the fixtures instead.
import { Injectable } from '@angular/core';
import type PluginResourceStoreService from '../plugin-resources/plugin-resource-store.service';
import type { ListScope } from '../plugin-resources/plugin-resource-store.service';
import type { KubeResource, ParsedCrd } from '../plugin-resources/types';
import * as fx from './fixtures';

/**
 * Objects of a kind, for the plugin a route names.
 *
 * Callers address an installation ("system--cert-manager"), as the real store's
 * callers do; the fixtures are keyed by catalog name, so resolve one to the
 * other first. An installation no definition claims has no objects here — the
 * same empty answer a cluster gives for a CRD it does not serve.
 */
function resourcesFor(installationName: string, kind: string): KubeResource[] {
  const pluginName = fx.catalogPluginName(installationName);
  if (!pluginName) return [];
  // The fixtures use console names; a cluster answers with its own.
  return (fx.pluginResources[`${pluginName}/${kind}`] ?? []).map((r) => {
    const ns = fx.namespaces.find((n) => n.name === r.metadata.namespace);
    return ns ? { ...r, metadata: { ...r.metadata, namespace: ns.clusterSideName } } : r;
  });
}

/** The project's namespaces, as the cluster names them. */
function projectNamespaces(projectId: string): Set<string> {
  return new Set(
    fx.namespaces.filter((n) => n.projectId === projectId).map((n) => n.clusterSideName),
  );
}

@Injectable({ providedIn: 'root' })
export default class FakePluginResourceStoreService implements Pick<
  PluginResourceStoreService,
  'loadResources' | 'loadResource' | 'getResource'
> {
  // Signatures mirror the real service (including the unused kubeApiProxyUrl),
  // so the callers it is swapped in for need no demo-specific branch.
  // eslint-disable-next-line class-methods-use-this
  async loadResources(
    crd: ParsedCrd,
    _clusterId: string,
    _kubeApiProxyUrl: string,
    pluginName: string,
    scope?: ListScope,
  ): Promise<KubeResource[]> {
    const all = resourcesFor(pluginName, crd.kind);
    if (crd.scope !== 'Namespaced' || !scope) return all;
    const allowed = projectNamespaces(scope.projectId);
    return all.filter((r) => {
      const ns = r.metadata.namespace ?? '';
      return allowed.has(ns) && (!scope.namespace || ns === scope.namespace);
    });
  }

  // eslint-disable-next-line class-methods-use-this
  async loadResource(
    crd: ParsedCrd,
    _clusterId: string,
    _kubeApiProxyUrl: string,
    pluginName: string,
    name: string,
    namespace: string | undefined,
    scope?: { projectId: string },
  ): Promise<KubeResource | undefined> {
    const allowed =
      crd.scope === 'Namespaced' && scope ? projectNamespaces(scope.projectId) : undefined;
    return resourcesFor(pluginName, crd.kind).find(
      (r) =>
        r.metadata.name === name &&
        (!namespace || r.metadata.namespace === namespace) &&
        (!allowed || allowed.has(r.metadata.namespace ?? '')),
    );
  }

  // eslint-disable-next-line class-methods-use-this
  getResource(
    pluginName: string,
    kind: string,
    resourceId: string,
    clusterId: string | null | undefined,
  ): KubeResource | undefined {
    if (!clusterId) return undefined;
    return resourcesFor(pluginName, kind).find((r) => r.metadata.name === resourceId);
  }
}
