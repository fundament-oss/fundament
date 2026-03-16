import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup, PluginNavItem, NavSectionDefinition } from './types';
import { kindToLabel } from './crd-schema.utils';

export interface PluginNavSection {
  pluginName: string;
  displayName: string;
  section: NavSectionDefinition;
}

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  organizationNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('organization'));

  projectNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('project'));

  /**
   * Custom nav sections declared by plugins via `navSections` in their manifest.
   * Each section renders a registered component at a sub-path under /plugin-resources/:pluginName/.
   */
  navSections = computed<PluginNavSection[]>(() => {
    const sections: PluginNavSection[] = [];
    for (const plugin of this.registry.allPlugins()) {
      if (plugin.navSections) {
        for (const section of plugin.navSections) {
          sections.push({
            pluginName: plugin.name,
            displayName: plugin.displayName,
            section,
          });
        }
      }
    }
    return sections;
  });

  private buildNavGroups(section: 'organization' | 'project'): PluginNavGroup[] {
    return this.registry
      .allPlugins()
      .filter((plugin) => (plugin.menu[section]?.length ?? 0) > 0)
      .reduce<PluginNavGroup[]>((groups, plugin) => {
        const items: PluginNavItem[] = (plugin.menu[section] ?? []).map((menuItem) => ({
          label: kindToLabel(menuItem.crd),
          crdKind: menuItem.crd,
          icon: menuItem.icon,
        }));

        if (items.length > 0) {
          groups.push({ pluginName: plugin.name, displayName: plugin.displayName, items });
        }
        return groups;
      }, []);
  }
}
