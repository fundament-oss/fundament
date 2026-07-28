// Demo-only stand-in for PluginInstallationService. The real one talks to the
// kube API proxy over fetch(); here installs live in memory so the walkthrough can
// actually install a plugin and watch it come up.
import { Injectable } from '@angular/core';
import { PluginInstallationItem } from '../plugin-resources/types';
import { PLUGIN_INSTALLS_RESET_EVENT } from '../presentation/presentation.tokens';
import * as fx from './fixtures';

// How long a fresh install stays Pending before it reports Running. The plugins
// page polls every 5s, so this is short enough to land within one poll while the
// slide is still on screen.
const INSTALL_MS = 3000;

interface DemoInstall {
  pluginName: string;
  pluginVersion: string;
  definitionHash: string;
  /** Wall-clock time the install was requested; null for seeded installs. */
  startedAt: number | null;
}

function toItem(install: DemoInstall): PluginInstallationItem {
  const running = install.startedAt === null || Date.now() - install.startedAt > INSTALL_MS;
  return {
    // The plugins page matches installs to catalog entries on metadata.name, so
    // keep the catalog name here rather than the RFC-1123 slug the real API uses.
    metadata: { name: install.pluginName, uid: `demo-${install.pluginName}` },
    spec: {
      definitionRef: {
        pluginName: install.pluginName,
        pluginVersion: install.pluginVersion,
        definitionHash: install.definitionHash,
      },
    },
    status: { phase: running ? 'Running' : 'Pending', ready: running },
  };
}

@Injectable({ providedIn: 'root' })
export default class FakePluginInstallationService {
  private readonly byCluster = new Map<string, DemoInstall[]>();

  constructor() {
    this.seed();
    // Let the walkthrough reset installs so its install slide can be replayed.
    document.addEventListener(PLUGIN_INSTALLS_RESET_EVENT, () => this.seed());
  }

  /** (Re)seed the in-memory installs to the fixture baseline, dropping any added live. */
  private seed(): void {
    this.byCluster.clear();
    Object.entries(fx.seededInstalls).forEach(([clusterId, pluginNames]) => {
      this.byCluster.set(
        clusterId,
        pluginNames.map((pluginName) => {
          const plugin = fx.plugins.find((p) => p.name === pluginName);
          return {
            pluginName,
            pluginVersion: plugin?.pluginVersion || 'demo',
            definitionHash: plugin?.definitionHash || 'sha256:demo',
            startedAt: null,
          };
        }),
      );
    });
  }

  async listInstallations(clusterId: string): Promise<PluginInstallationItem[]> {
    return (this.byCluster.get(clusterId) ?? []).map(toItem);
  }

  async getInstallation(clusterId: string, name: string): Promise<PluginInstallationItem | null> {
    const install = (this.byCluster.get(clusterId) ?? []).find((i) => i.pluginName === name);
    return install ? toItem(install) : null;
  }

  // Mirrors the real signature. Unlike the real service this does not reject an
  // unpinned definition: the fixture catalog carries no pluginVersion/definitionHash,
  // and the walkthrough's install slide must succeed regardless.
  async installPlugin(
    clusterId: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
  ): Promise<void> {
    const current = this.byCluster.get(clusterId) ?? [];
    if (current.some((i) => i.pluginName === pluginName)) return;
    this.byCluster.set(clusterId, [
      ...current,
      {
        pluginName,
        pluginVersion: pluginVersion || 'demo',
        definitionHash: definitionHash || 'sha256:demo',
        startedAt: Date.now(),
      },
    ]);
  }

  async uninstallPlugin(clusterId: string, pluginName: string): Promise<void> {
    const current = this.byCluster.get(clusterId) ?? [];
    this.byCluster.set(
      clusterId,
      current.filter((i) => i.pluginName !== pluginName),
    );
  }
}
