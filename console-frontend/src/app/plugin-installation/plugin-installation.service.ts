import { Injectable, inject } from '@angular/core';

import { ConfigService } from '../config.service';
import { PluginInstallationItem, PluginInstallationListResponse } from '../plugin-resources/types';
import { RetryConfigVersionError, RetryReadError } from './retry-errors';

export { RetryAbortedError, RetryConfigVersionError, RetryReadError } from './retry-errors';

/** Placeholder spec.definitionRef.pluginVersion for installs that never
 *  resolved a real published version (the terraform provider's default until
 *  the marketplace supplies real pins). Mirrors plugin-sdk's
 *  pluginruntime.UnknownVersion — the one spelling every codebase keys off. */
export const UNKNOWN_PLUGIN_VERSION = 'unknown';

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
    config: Record<string, string> = {},
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
          ...(Object.keys(config).length > 0 ? { config } : {}),
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

  // Re-creates a failed installation, preserving its spec.config.
  //
  // The failed install may carry config; it is read before deleting the CR so
  // the retry re-creates the installation as it was, not with defaults. A
  // read failure (network blip, RBAC hiccup) is not the same as "CR already
  // gone" (404 resolves null) — only the latter is safe to proceed past.
  // Uninstalling on a failed read would delete the only copy of the config,
  // so that aborts with RetryReadError instead.
  async retryInstall(
    clusterId: string,
    organizationName: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
  ): Promise<void> {
    const resourceName = pluginResourceName(organizationName, pluginName);
    let existing: PluginInstallationItem | null;
    try {
      existing = await this.getInstallation(clusterId, resourceName);
    } catch {
      throw new RetryReadError(`failed to read installation ${resourceName}`);
    }
    const config = existing?.spec.config ?? {};

    // Preserved config was written against the recorded version's schema; a
    // retry that switches versions (the fallback for an unrecorded pin) must
    // not replay it against a different schema. Abort before deleting — the
    // failed CR still holds the only copy of that config.
    if (
      existing &&
      Object.keys(config).length > 0 &&
      existing.spec.definitionRef.pluginVersion !== pluginVersion
    ) {
      throw new RetryConfigVersionError(
        `installation ${resourceName} carries config for version ${existing.spec.definitionRef.pluginVersion}`,
      );
    }

    // The CR from the failed install still exists, so remove it and wait for
    // it to be gone before re-creating (a plain re-POST would 409).
    await this.uninstallPlugin(clusterId, resourceName).catch(() => {});
    await this.waitForUninstall(clusterId, resourceName);
    await this.installPlugin(
      clusterId,
      organizationName,
      pluginName,
      pluginVersion,
      definitionHash,
      config,
    );
  }

  private async waitForUninstall(
    clusterId: string,
    resourceName: string,
    // Wait up to ~30s for finalizers to clear the old CR before re-creating it;
    // re-POSTing while it is still terminating would 409.
    attempts = 30,
  ): Promise<void> {
    if (attempts <= 0) return;
    // A poll error keeps waiting (non-null sentinel): only a definite 404
    // proves the CR is gone.
    const item = await this.getInstallation(clusterId, resourceName).catch(() => ({}));
    if (item === null) return;
    await new Promise((resolve) => {
      setTimeout(resolve, 1000);
    });
    await this.waitForUninstall(clusterId, resourceName, attempts - 1);
  }
}
