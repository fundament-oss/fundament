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

  async loadCrdAndResources(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
  ): Promise<{ crd: ParsedCrd | undefined; resources: KubeResource[] }> {
    const kubeApiProxyUrl = this.configService.getConfig().kubeApiProxyUrl;
    await this.registry.loadPlugins(clusterId);
    await this.registry.loadCrdsForPlugin(pluginName, clusterId, kubeApiProxyUrl);
    const crd = this.registry.getCrd(pluginName, resourceKind, clusterId);
    if (!crd) return { crd: undefined, resources: [] };

    const resources = await this.pluginStore.loadResources(
      crd,
      clusterId,
      kubeApiProxyUrl,
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
    const kubeApiProxyUrl = this.configService.getConfig().kubeApiProxyUrl;
    await this.registry.loadPlugins(clusterId);
    await this.registry.loadCrdsForPlugin(pluginName, clusterId, kubeApiProxyUrl);
    const crd = this.registry.getCrd(pluginName, resourceKind, clusterId);
    if (!crd) return { crd: undefined, resource: undefined };

    const resource = await this.pluginStore.loadResource(
      crd,
      clusterId,
      kubeApiProxyUrl,
      pluginName,
      name,
      namespace,
    );
    return { crd, resource };
  }
}
