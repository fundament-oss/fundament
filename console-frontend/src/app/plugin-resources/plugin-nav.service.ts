import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup, PluginNavItem } from './types';
import { kindToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  organizationNav = computed<PluginNavGroup[]>(() =>
    this.buildNavGroups('organization', (plugin, crd) => [
      '/plugin-resources',
      plugin.name,
      crd.plural,
    ]),
  );

  projectNav = computed<PluginNavGroup[]>(() =>
    this.buildNavGroups('project', (_plugin, crd) => [crd.plural]),
  );

  private buildNavGroups(
    section: 'organization' | 'project',
    routerLink: (plugin: { name: string }, crd: { kind: string; plural: string }) => string[],
  ): PluginNavGroup[] {
    return this.registry
      .allPlugins()
      .filter((plugin) => plugin.menu[section] && plugin.menu[section]!.length > 0)
      .reduce<PluginNavGroup[]>((groups, plugin) => {
        const items: PluginNavItem[] = (plugin.menu[section] ?? [])
          .filter((menuItem) => plugin.crds.find((c) => c.kind === menuItem.crd))
          .map((menuItem) => {
            const crd = plugin.crds.find((c) => c.kind === menuItem.crd)!;
            return {
              label: kindToLabel(crd.kind),
              crdKind: crd.kind,
              crdPlural: crd.plural,
              routerLink: routerLink(plugin, crd),
              icon: menuItem.icon,
            };
          });

        if (items.length > 0) {
          groups.push({ pluginName: plugin.name, displayName: plugin.displayName, items });
        }
        return groups;
      }, []);
  }
}
