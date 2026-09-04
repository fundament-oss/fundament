// Demo-only stand-in for PluginInstallationService. The real one talks to the
// kube API proxy over fetch(); here installs live in memory so the walkthrough can
// actually install a plugin and watch it come up.
import { Injectable } from '@angular/core';
import type PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { pluginResourceName } from '../plugin-installation/plugin-installation.service';
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
  organizationName: string;
  pluginName: string;
  pluginVersion: string;
  definitionHash: string;
  /** Wall-clock time the install was requested; null for seeded installs. */
  startedAt: number | null;
}

function toItem(install: DemoInstall): PluginInstallationItem {
  const running = install.startedAt === null || Date.now() - install.startedAt > INSTALL_MS;
  // Qualify metadata.name the same way the real API does: plugin-details reads it
  // back through pluginResourceName(organizationName, pluginName) to find this row.
  const resourceName = pluginResourceName(install.organizationName, install.pluginName);
  return {
    metadata: { name: resourceName, uid: `demo-${install.pluginName}` },
    spec: {
      definitionRef: {
        organizationName: install.organizationName,
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
      organizationName: plugin?.organizationName || 'system',
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
    // seeded baseline: the new-cluster form appends to fx.clusterSummaries, and
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

  // `name` is the installation (resource) name, e.g. "system--cert-manager" — the
  // same identity uninstallPlugin below and the real API take, not the catalog name.
  async getInstallation(clusterId: string, name: string): Promise<PluginInstallationItem | null> {
    const install = (this.byCluster.get(clusterId) ?? []).find(
      (i) => pluginResourceName(i.organizationName, i.pluginName) === name,
    );
    return install ? toItem(install) : null;
  }

  // Mirrors the real signature. Unlike the real service this does not reject an
  // unpinned definition: the fixture catalog carries no pluginVersion/definitionHash,
  // and the walkthrough's install slide must succeed regardless.
  async installPlugin(
    clusterId: string,
    organizationName: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
  ): Promise<void> {
    const current = this.byCluster.get(clusterId) ?? [];
    // Match on the pair: two organizations may publish the same pluginName.
    if (current.some((i) => i.organizationName === organizationName && i.pluginName === pluginName))
      return;
    this.byCluster.set(clusterId, [
      ...current,
      {
        organizationName,
        pluginName,
        pluginVersion: pluginVersion || 'demo',
        definitionHash: definitionHash || 'sha256:demo',
        startedAt: Date.now(),
      },
    ]);
    FakePluginInstallationService.notifyChanged();
  }

  // Real callers pass the installation (resource) name — e.g.
  // pluginResourceName(plugin.organizationName, plugin.name) — not the catalog name.
  async uninstallPlugin(clusterId: string, resourceName: string): Promise<void> {
    const current = this.byCluster.get(clusterId) ?? [];
    this.byCluster.set(
      clusterId,
      current.filter((i) => pluginResourceName(i.organizationName, i.pluginName) !== resourceName),
    );
    FakePluginInstallationService.notifyChanged();
  }
}
