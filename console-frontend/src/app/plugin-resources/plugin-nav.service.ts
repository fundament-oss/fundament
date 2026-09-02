import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup } from './types';
import { crdRefToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  projectNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('project'));

  private buildNavGroups(section: 'project'): PluginNavGroup[] {
    const shown = this.registry.allPlugins().filter((plugin) => (plugin.menu[section]?.length ?? 0) > 0);
    // Two organizations may publish the same plugin, so a shared label would
    // give two indistinguishable nav groups. Qualify only the ambiguous ones —
    // adding the publisher everywhere is noise when there is nothing to resolve.
    const duplicated = new Set(
      shown
        .map((plugin) => plugin.label)
        .filter((label, i, all) => all.indexOf(label) !== i),
    );

    return shown.map((plugin) => ({
      installationName: plugin.installationName,
      label: duplicated.has(plugin.label)
        ? `${plugin.label} (${plugin.organizationName})`
        : plugin.label,
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
