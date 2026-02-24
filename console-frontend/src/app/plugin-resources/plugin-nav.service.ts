import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup, PluginNavItem } from './types';
import { kindToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  organizationNav = computed<PluginNavGroup[]>(() => {
    const plugins = this.registry.allPlugins();
    return plugins
      .filter((plugin) => plugin.menu.organization && plugin.menu.organization.length > 0)
      .reduce<PluginNavGroup[]>((groups, plugin) => {
        const items: PluginNavItem[] = (plugin.menu.organization ?? [])
          .filter((menuItem) => {
            const crd = plugin.crds.find((c) => c.kind === menuItem.crd);
            return crd && menuItem.list;
          })
          .map((menuItem) => {
            const crd = plugin.crds.find((c) => c.kind === menuItem.crd)!;
            return {
              label: kindToLabel(crd.kind),
              crdKind: crd.kind,
              crdPlural: crd.plural,
              routerLink: ['/plugin-resources', plugin.metadata.name, crd.plural],
            };
          });

        if (items.length > 0) {
          groups.push({
            pluginName: plugin.metadata.name,
            displayName: plugin.metadata.displayName,
            icon: plugin.metadata.icon,
            items,
          });
        }
        return groups;
      }, []);
  });

  projectNav = computed<PluginNavGroup[]>(() => {
    const plugins = this.registry.allPlugins();
    return plugins
      .filter((plugin) => plugin.menu.project && plugin.menu.project.length > 0)
      .reduce<PluginNavGroup[]>((groups, plugin) => {
        const items: PluginNavItem[] = (plugin.menu.project ?? [])
          .filter((menuItem) => {
            const crd = plugin.crds.find((c) => c.kind === menuItem.crd);
            return crd && menuItem.list;
          })
          .map((menuItem) => {
            const crd = plugin.crds.find((c) => c.kind === menuItem.crd)!;
            return {
              label: kindToLabel(crd.kind),
              crdKind: crd.kind,
              crdPlural: crd.plural,
              routerLink: [crd.plural],
            };
          });

        if (items.length > 0) {
          groups.push({
            pluginName: plugin.metadata.name,
            displayName: plugin.metadata.displayName,
            icon: plugin.metadata.icon,
            items,
          });
        }
        return groups;
      }, []);
  });
}
