// Demo-only stand-in for PluginInstallationService. The real one talks to the
// kube API proxy over fetch(); here installs live in memory so the walkthrough can
// actually install a plugin and watch it come up.
import { Injectable } from '@angular/core';
import type PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { PluginInstallationItem } from '../plugin-resources/types';
import {
  PLUGIN_INSTALLS_ENSURE_EVENT,
  PLUGIN_INSTALLS_RESET_EVENT,
} from '../presentation/presentation.tokens';
import * as fx from './fixtures';

/**
 * Dispatched on `document` whenever the in-memory installs change, so the parts
 * of the console that read them once — the sidebar menu, built per project — can
 * follow along instead of waiting for a navigation. Stands in for the watch the
 * real console would have on the cluster.
 */
export const PLUGIN_INSTALLS_CHANGED_EVENT = 'demo:plugin-installs-changed';

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
export default class FakePluginInstallationService implements Pick<
  PluginInstallationService,
  'listInstallations' | 'getInstallation' | 'installPlugin' | 'uninstallPlugin'
> {
  private readonly byCluster = new Map<string, DemoInstall[]>();

  constructor() {
    this.seed();
    // Let the walkthrough reset installs so its install slide can be replayed.
    document.addEventListener(PLUGIN_INSTALLS_RESET_EVENT, () => this.seed());
    // ...and let the slides after it put their own subject in place.
    document.addEventListener(PLUGIN_INSTALLS_ENSURE_EVENT, () => this.ensureUiPlugins());
  }

  private static notifyChanged(): void {
    document.dispatchEvent(new CustomEvent(PLUGIN_INSTALLS_CHANGED_EVENT));
  }

  /** An install of a catalog plugin, already running (nothing to wait for). */
  private static seededInstall(pluginName: string): DemoInstall {
    const plugin = fx.plugins.find((p) => p.name === pluginName);
    return {
      pluginName,
      pluginVersion: plugin?.pluginVersion || 'demo',
      definitionHash: plugin?.definitionHash || 'sha256:demo',
      startedAt: null,
    };
  }

  /** (Re)seed the in-memory installs to the fixture baseline, dropping any added live. */
  private seed(): void {
    this.byCluster.clear();
    Object.entries(fx.seededInstalls).forEach(([clusterId, pluginNames]) => {
      this.byCluster.set(clusterId, pluginNames.map(FakePluginInstallationService.seededInstall));
    });
    FakePluginInstallationService.notifyChanged();
  }

  /**
   * Install every plugin the console has a UI for, on every cluster, as already
   * running. Idempotent: an install that is there stays as it is, so arriving
   * from the install slide keeps that slide's freshly installed (and briefly
   * Pending) plugin instead of skipping its status change.
   */
  private ensureUiPlugins(): void {
    // Every cluster the demo currently knows about, not just the ones with a
    // seeded baseline: the add-cluster wizard appends to fx.clusterSummaries, and
    // a cluster created during the walkthrough must get the UI plugins too.
    const clusterIds = new Set([
      ...fx.clusterSummaries.map((cluster) => cluster.id),
      ...Object.keys(fx.seededInstalls),
      ...this.byCluster.keys(),
    ]);
    clusterIds.forEach((clusterId) => {
      const current = this.byCluster.get(clusterId) ?? [];
      const missing = Object.keys(fx.pluginDefinitions)
        .filter((pluginName) => !current.some((i) => i.pluginName === pluginName))
        .map(FakePluginInstallationService.seededInstall);
      if (missing.length > 0) this.byCluster.set(clusterId, [...current, ...missing]);
    });
    FakePluginInstallationService.notifyChanged();
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
    FakePluginInstallationService.notifyChanged();
  }

  async uninstallPlugin(clusterId: string, pluginName: string): Promise<void> {
    const current = this.byCluster.get(clusterId) ?? [];
    this.byCluster.set(
      clusterId,
      current.filter((i) => i.pluginName !== pluginName),
    );
    FakePluginInstallationService.notifyChanged();
  }
}
