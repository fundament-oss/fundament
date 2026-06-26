import { Injectable, inject } from '@angular/core';
import { ConfigService } from '../config.service';
import PluginRegistryService from './plugin-registry.service';
import PluginResourceStoreService from './plugin-resource-store.service';
import type { ParsedCrd, KubeResource } from './types';

@Injectable({ providedIn: 'root' })
export default class KubePluginLoaderService {
  private registry = inject(PluginRegistryService);

  private pluginStore = inject(PluginResourceStoreService);

  private configService = inject(ConfigService);

  // Resolves the CRD definition for a plugin's resource kind, loading the plugin
  // registry and its CRDs first. The shared prologue of the loaders below; the
  // create flow uses it directly since it needs only the schema, not objects.
  async loadCrd(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
  ): Promise<ParsedCrd | undefined> {
    const kubeApiProxyUrl = this.configService.getConfig().kubeApiProxyUrl;
    await this.registry.loadPlugins(clusterId);
    await this.registry.loadCrdsForPlugin(pluginName, clusterId, kubeApiProxyUrl);
    return this.registry.getCrd(pluginName, resourceKind, clusterId);
  }

  async loadCrdAndResources(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
  ): Promise<{ crd: ParsedCrd | undefined; resources: KubeResource[] }> {
    const crd = await this.loadCrd(pluginName, resourceKind, clusterId);
    if (!crd) return { crd: undefined, resources: [] };

    const resources = await this.pluginStore.loadResources(
      crd,
      clusterId,
      this.configService.getConfig().kubeApiProxyUrl,
      pluginName,
    );
    return { crd, resources };
  }

  async loadCrdAndResource(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
    name: string,
    namespace: string | undefined,
  ): Promise<{ crd: ParsedCrd | undefined; resource: KubeResource | undefined }> {
    const crd = await this.loadCrd(pluginName, resourceKind, clusterId);
    if (!crd) return { crd: undefined, resource: undefined };

    const resource = await this.pluginStore.loadResource(
      crd,
      clusterId,
      this.configService.getConfig().kubeApiProxyUrl,
      pluginName,
      name,
      namespace,
    );
    return { crd, resource };
  }
}
