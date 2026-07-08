import { Injectable, inject } from '@angular/core';

import { ConfigService } from '../config.service';
import { PluginInstallationItem, PluginInstallationListResponse } from '../plugin-resources/types';

// Kubernetes resource names must be RFC-1123 (lowercase alphanumerics and '-'),
// but catalog plugins carry display names like "Grafana Alloy" or "ECK operator".
// Derive a stable slug for the PluginInstallation's metadata.name; the catalog
// name is still carried verbatim in spec.definitionRef.pluginName.
export function pluginResourceName(pluginName: string): string {
  return pluginName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

@Injectable({ providedIn: 'root' })
export default class PluginInstallationService {
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

  // Fetches a single installation by name; null means it does not exist yet
  // (e.g. still being created). Cheaper than listing the whole collection when
  // polling for one plugin's status.
  async getInstallation(clusterId: string, name: string): Promise<PluginInstallationItem | null> {
    const res = await fetch(this.url(clusterId, name), { credentials: 'include' });
    if (res.status === 404) return null;
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as PluginInstallationItem;
  }

  async installPlugin(
    clusterId: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
  ): Promise<void> {
    // TODO(FUN-11): once the marketplace returns the published pluginVersion
    // and definitionHash for each PluginSummary, surface them here. Until then
    // we send development placeholders; the consent record only becomes a real
    // pin when Plan B wires the mint endpoint to the marketplace artifact.
    const res = await fetch(this.url(clusterId), {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        apiVersion: 'plugins.fundament.io/v1',
        kind: 'PluginInstallation',
        metadata: { name: pluginResourceName(pluginName) },
        spec: {
          definitionRef: {
            pluginName,
            pluginVersion,
            definitionHash,
          },
        },
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
