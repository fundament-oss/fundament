// Demo-only stand-in for PluginResourceStoreService. The real one lists and gets
// the plugin's custom resources from the cluster with fetch() against
// kube-api-proxy; the static demo has no such origin and its CSP blocks the
// request, so the objects come from the fixtures instead.
import { Injectable } from '@angular/core';
import type PluginResourceStoreService from '../plugin-resources/plugin-resource-store.service';
import type { KubeResource, ParsedCrd } from '../plugin-resources/types';
import * as fx from './fixtures';

/**
 * Routes and nav address an *installation* ("system--cert-manager"), while the
 * fixtures index objects by catalog plugin name ("cert-manager") — the same
 * indirection FakePluginRegistryService resolves for CRDs. A name that matches no
 * installation is used as-is, so a fixture keyed on the plugin name still resolves.
 */
function fixtureName(installationName: string): string {
  return (
    Object.values(fx.pluginDefinitions).find((def) => def.installationName === installationName)
      ?.name ?? installationName
  );
}

function resourcesFor(pluginName: string, kind: string): KubeResource[] {
  return fx.pluginResources[`${fixtureName(pluginName)}/${kind}`] ?? [];
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
  ): Promise<KubeResource[]> {
    return resourcesFor(pluginName, crd.kind);
  }

  // eslint-disable-next-line class-methods-use-this
  async loadResource(
    crd: ParsedCrd,
    _clusterId: string,
    _kubeApiProxyUrl: string,
    pluginName: string,
    name: string,
    namespace: string | undefined,
  ): Promise<KubeResource | undefined> {
    return resourcesFor(pluginName, crd.kind).find(
      (r) => r.metadata.name === name && (!namespace || r.metadata.namespace === namespace),
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
