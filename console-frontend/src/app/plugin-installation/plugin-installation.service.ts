import { Injectable, inject } from '@angular/core';

import { ConfigService } from '../config.service';
import {
  PluginInstallationItem,
  PluginInstallationListResponse,
} from '../plugin-resources/types';

@Injectable({ providedIn: 'root' })
export class PluginInstallationService {
  private configService = inject(ConfigService);

  private url(clusterId: string, name?: string): string {
    const { kubeApiProxyUrl } = this.configService.getConfig();
    const base = `${kubeApiProxyUrl}/clusters/${clusterId}/apis/plugins.fundament.io/v1/plugininstallations`;
    return name ? `${base}/${name}` : base;
  }

  async listInstallations(clusterId: string): Promise<PluginInstallationItem[]> {
    const res = await fetch(this.url(clusterId), { credentials: 'include' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const body: PluginInstallationListResponse = await res.json();
    return body.items ?? [];
  }

  async installPlugin(clusterId: string, pluginName: string, image: string): Promise<void> {
    const res = await fetch(this.url(clusterId), {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        apiVersion: 'plugins.fundament.io/v1',
        kind: 'PluginInstallation',
        metadata: { name: pluginName },
        spec: { pluginName, image },
      }),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
  }

  async uninstallPlugin(clusterId: string, pluginName: string): Promise<void> {
    const res = await fetch(this.url(clusterId, pluginName), {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
  }
}
