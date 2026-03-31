import { Injectable, inject } from '@angular/core';
import { ConfigService } from '../config.service';
import OrganizationContextService from '../organization-context.service';
import PluginRegistryService from './plugin-registry.service';
import PluginResourceStoreService from './plugin-resource-store.service';
import type { ParsedCrd, KubeResource } from './types';

@Injectable({ providedIn: 'root' })
export default class KubePluginLoaderService {
  private registry = inject(PluginRegistryService);

  private pluginStore = inject(PluginResourceStoreService);

  private configService = inject(ConfigService);

  private orgContext = inject(OrganizationContextService);

  async loadCrdAndResources(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
  ): Promise<{ crd: ParsedCrd | undefined; resources: KubeResource[] }> {
    const orgId = this.orgContext.currentOrganizationId();
    if (!orgId) return { crd: undefined, resources: [] };

    const kubeApiProxyUrl = this.configService.getConfig().kubeApiProxyUrl;
    await this.registry.loadCrdsForPlugin(pluginName, clusterId, kubeApiProxyUrl, orgId);
    const crd = this.registry.getCrd(pluginName, resourceKind, clusterId);
    if (!crd) return { crd: undefined, resources: [] };

    const resources = await this.pluginStore.loadResources(
      crd,
      clusterId,
      kubeApiProxyUrl,
      orgId,
      pluginName,
    );
    return { crd, resources };
  }
}
