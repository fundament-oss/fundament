import { Injectable, inject, computed } from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import type { PluginNavGroup } from './types';
import { crdRefToLabel, kindToLabel } from './crd-schema.utils';

@Injectable({ providedIn: 'root' })
export default class PluginNavService {
  private registry = inject(PluginRegistryService);

  clusterNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('organization'));

  projectNav = computed<PluginNavGroup[]>(() => this.buildNavGroups('project'));

  private buildNavGroups(section: 'organization' | 'project'): PluginNavGroup[] {
    const shown = this.registry
      .allPlugins()
      .map((plugin) => ({
        plugin,
        entries: (plugin.menu[section] ?? []).filter((entry) =>
          this.fitsSection(entry.crd, section),
        ),
      }))
      .filter(({ entries }) => entries.length > 0);
    // Two organizations may publish the same plugin, so a shared label would
    // give two indistinguishable nav groups. Qualify only the ambiguous ones —
    // adding the publisher everywhere is noise when there is nothing to resolve.
    const duplicated = new Set(
      shown.map(({ plugin }) => plugin.label).filter((label, i, all) => all.indexOf(label) !== i),
    );

    return shown.map(({ plugin, entries }) => ({
      installationName: plugin.installationName,
      label: duplicated.has(plugin.label)
        ? `${plugin.label} (${plugin.organizationName})`
        : plugin.label,
      items: entries.map((menuItem) => ({
        label: menuItem.label ?? this.labelFor(menuItem.crd),
        crdPlural: menuItem.crd,
        icon: menuItem.icon,
      })),
    }));
  }

  // Warned once per entry: the nav recomputes whenever the registry changes.
  private warned = new Set<string>();

  // An unread CRD is kept: its scope arrives with the registry's CRD read.
  private fitsSection(crdRef: string, section: 'organization' | 'project'): boolean {
    const scope = this.registry.crdScope(crdRef);
    const expected = section === 'organization' ? 'Cluster' : 'Namespaced';
    if (scope === undefined || scope === expected) return true;
    const key = `${crdRef}:${section}`;
    if (!this.warned.has(key)) {
      this.warned.add(key);
      // eslint-disable-next-line no-console
      console.warn(`[PluginNav] ${crdRef} is ${scope}-scoped; not shown under menu.${section}`);
    }
    return false;
  }

  /**
   * Names a menu entry's CRD. menuItem.crd is a CRD reference
   * ("dnsendpoints.externaldns.k8s.io"), not a kind, and its plural is lowercase
   * by Kubernetes' rules — derived from it alone the entry reads "Dnsendpoints".
   * So the CRD's own kind is used once the registry has read it ("DNSEndpoint" →
   * "DNS Endpoints"), and the reference is the stand-in until then.
   */
  private labelFor(crdRef: string): string {
    const kind = this.registry.crdKind(crdRef);
    return kind ? kindToLabel(kind) : crdRefToLabel(crdRef);
  }
}
