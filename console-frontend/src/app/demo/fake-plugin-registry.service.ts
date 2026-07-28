// Demo-only stand-in for PluginRegistryService. The real one reads the
// PluginInstallation CRs and the plugins' CRDs from the cluster with fetch()
// against kube-api-proxy — in the static demo there is no such origin, and the
// page's CSP (connect-src 'self') blocks the request outright. Here the
// definitions and CRDs are fixtures.
import { inject, Injectable, signal } from '@angular/core';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import type { ParsedCrd, PluginDefinition } from '../plugin-resources/types';
import { PLUGIN_INSTALLS_CHANGED_EVENT } from './fake-plugin-installation.service';
import * as fx from './fixtures';

@Injectable({ providedIn: 'root' })
export default class FakePluginRegistryService {
  // Resolves to FakePluginInstallationService in the demo injector, so the menu
  // reflects the install the walkthrough performs on its plugin slide.
  private readonly installations = inject(PluginInstallationService);

  private plugins = signal<PluginDefinition[]>([]);

  private loadedForClusterId: string | null = null;

  private pollTimer: ReturnType<typeof setTimeout> | null = null;

  /** How often to re-read while an install is still coming up. */
  private static readonly POLL_MS = 1000;

  constructor() {
    // The sidebar menu is built once, when a project is opened. In the real
    // console an install lands through a cluster the user then navigates to;
    // here it can happen while the menu is on screen, so re-read on every
    // change instead of leaving the plugin invisible until the next navigation.
    document.addEventListener(PLUGIN_INSTALLS_CHANGED_EVENT, () => {
      const clusterId = this.loadedForClusterId;
      if (clusterId) this.refresh(clusterId).catch(() => {});
    });
  }

  async loadPlugins(clusterId: string): Promise<void> {
    if (clusterId === this.loadedForClusterId) return;

    this.loadedForClusterId = clusterId;
    await this.refresh(clusterId);
  }

  private async refresh(clusterId: string): Promise<void> {
    const items = await this.installations.listInstallations(clusterId);
    // A cluster switch may have overtaken this read; its result is stale.
    if (clusterId !== this.loadedForClusterId) return;

    const definitionFor = (pluginName: string) => fx.pluginDefinitions[pluginName];

    this.plugins.set(
      items
        .filter((item) => item.status.phase === 'Running' && item.status.ready)
        .map((item) => definitionFor(item.spec.definitionRef.pluginName))
        // An installed plugin the fixtures give no definition for has no console
        // UI — the same outcome as a definition the catalog cannot resolve.
        .filter((definition): definition is PluginDefinition => definition !== undefined),
    );

    // The sidebar menu is built once per project, but a plugin installed moments
    // ago still reports Pending. Keep reading until it is up, so the walkthrough
    // can move straight from the install slide to the project without the menu
    // lagging a slide behind.
    this.clearPoll();
    const comingUp = items.some(
      (item) => !item.status.ready && definitionFor(item.spec.definitionRef.pluginName),
    );
    if (comingUp) {
      this.pollTimer = setTimeout(() => {
        this.refresh(clusterId).catch(() => {});
      }, FakePluginRegistryService.POLL_MS);
    }
  }

  private clearPoll(): void {
    if (this.pollTimer) clearTimeout(this.pollTimer);
    this.pollTimer = null;
  }

  // CRDs come from the fixtures with the definitions, so there is nothing to load.
  // Kept for signature parity with the real service.
  // eslint-disable-next-line class-methods-use-this
  async loadCrdsForPlugin(): Promise<void> {
    // Intentionally empty.
  }

  reset(): void {
    this.clearPoll();
    this.loadedForClusterId = null;
    this.plugins.set([]);
  }

  getPlugin(name: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.name === name);
  }

  /** `key` is a plural, a kind or a full `plural.group`, as in the real service. */
  // eslint-disable-next-line class-methods-use-this
  getCrd(pluginName: string, key: string, _clusterId: string): ParsedCrd | undefined {
    return (fx.pluginCrds[pluginName] ?? []).find(
      (crd) => key === crd.plural || key === crd.kind || key === `${crd.plural}.${crd.group}`,
    );
  }

  allPlugins = this.plugins.asReadonly();
}
