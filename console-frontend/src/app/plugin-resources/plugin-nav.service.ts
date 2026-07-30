import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup } from './types';
import { crdRefToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  projectNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('project'));

  private buildNavGroups(section: 'project'): PluginNavGroup[] {
    return this.registry
      .allPlugins()
      .filter((plugin) => (plugin.menu[section]?.length ?? 0) > 0)
      .map((plugin) => ({
        pluginName: plugin.name,
        label: plugin.label,
        items: (plugin.menu[section] ?? []).map((menuItem) => ({
          // menuItem.crd is a CRD reference ("certificates.cert-manager.io"),
          // not a kind — see crdRefToLabel.
          label: menuItem.label ?? crdRefToLabel(menuItem.crd),
          crdPlural: menuItem.crd,
          icon: menuItem.icon,
        })),
      }));
  }
}
