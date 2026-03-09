import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup, PluginNavItem } from './types';
import { kindToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  organizationNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('organization'));

  projectNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('project'));

  private buildNavGroups(section: 'organization' | 'project'): PluginNavGroup[] {
    return this.registry
      .allPlugins()
      .filter((plugin) => (plugin.menu[section]?.length ?? 0) > 0)
      .reduce<PluginNavGroup[]>((groups, plugin) => {
        const items: PluginNavItem[] = (plugin.menu[section] ?? []).map((menuItem) => ({
          label: kindToLabel(menuItem.crd),
          crdKind: menuItem.crd,
          crdPlural: menuItem.plural,
          icon: menuItem.icon,
        }));

        if (items.length > 0) {
          groups.push({ pluginName: plugin.name, displayName: plugin.displayName, items });
        }
        return groups;
      }, []);
  }
}
