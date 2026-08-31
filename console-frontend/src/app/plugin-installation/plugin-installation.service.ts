import { Injectable, inject } from '@angular/core';

import { ConfigService } from '../config.service';
import { PluginInstallationItem, PluginInstallationListResponse } from '../plugin-resources/types';

// Kubernetes resource names must be RFC-1123 (lowercase alphanumerics and '-'),
// but catalog entries carry display names like "Grafana Alloy".
function slug(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

// A plugin's identity is (organization, plugin), so the installation's
// metadata.name is both halves joined by a double dash — the pair the apiserver
// keeps unique. Each half is slugged separately: slugging the joined string
// would collapse the separator back to a single dash, and a single dash cannot
// tell ("system", "cert-manager") apart from ("system-cert", "manager").
export function pluginResourceName(organizationName: string, pluginName: string): string {
  return `${slug(organizationName)}--${slug(pluginName)}`;
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
    organizationName: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
  ): Promise<void> {
    // A plugin with no published definition has no version/hash to pin — the
    // install would reconcile to Failed. Refuse it here rather than create a
    // stuck CR.
    if (!pluginVersion || !definitionHash) {
      throw new Error(`${pluginName} has no published definition to install`);
    }
    const res = await fetch(this.url(clusterId), {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        apiVersion: 'plugins.fundament.io/v1',
        kind: 'PluginInstallation',
        metadata: { name: pluginResourceName(organizationName, pluginName) },
        spec: {
          definitionRef: {
            organizationName,
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
