import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { describe, expect, it } from 'vitest';
import PluginNavService from './plugin-nav.service';
import PluginRegistryService from './plugin-registry.service';
import type { PluginDefinition } from './types';

function definition(
  organizationName: string,
  name: string,
  label: string,
  crd = 'certificates.cert-manager.io',
  menuLabel?: string,
): PluginDefinition {
  return {
    name,
    label,
    version: 'v1',
    description: '',
    menu: { project: [{ crd, label: menuLabel }] },
    crds: [crd],
    allowedResources: [],
    installationId: `${organizationName}-${name}-uid`,
    installationName: `${organizationName}--${name}`,
    installationVersion: 'v1',
    organizationName,
  };
}

function navFor(plugins: PluginDefinition[], kinds: Record<string, string> = {}): PluginNavService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      {
        provide: PluginRegistryService,
        useValue: {
          allPlugins: signal(plugins),
          crdKind: (crdRef: string) => kinds[crdRef],
        },
      },
    ],
  });
  return TestBed.inject(PluginNavService);
}

const CRD = 'dnsendpoints.externaldns.k8s.io';

describe('PluginNavService', () => {
  it('routes on the installation name, not the plugin name', () => {
    const nav = navFor([definition('system', 'cert-manager', 'Cert Manager')]);

    expect(nav.projectNav().map((g) => g.installationName)).toEqual(['system--cert-manager']);
  });

  it('leaves the label alone when nothing is ambiguous', () => {
    const nav = navFor([definition('system', 'cert-manager', 'Cert Manager')]);

    expect(nav.projectNav().map((g) => g.label)).toEqual(['Cert Manager']);
  });

  it('names the publisher when two organizations share a label', () => {
    const nav = navFor([
      definition('system', 'cert-manager', 'Cert Manager'),
      definition('acme-corp', 'cert-manager', 'Cert Manager'),
    ]);

    expect(nav.projectNav().map((g) => g.label)).toEqual([
      'Cert Manager (system)',
      'Cert Manager (acme-corp)',
    ]);
    // Distinct route segments: the two groups address different installations.
    expect(nav.projectNav().map((g) => g.installationName)).toEqual([
      'system--cert-manager',
      'acme-corp--cert-manager',
    ]);
  });

  it("names an item after the CRD's kind once the registry has read it", () => {
    const nav = navFor([definition('system', 'external-dns', 'External DNS', CRD)], {
      [CRD]: 'DNSEndpoint',
    });

    expect(nav.projectNav()[0].items.map((i) => i.label)).toEqual(['DNS Endpoints']);
  });

  it('falls back to the CRD reference until the kind is known', () => {
    // All a reference can give: a plural is lowercase by Kubernetes' rules.
    const nav = navFor([definition('system', 'external-dns', 'External DNS', CRD)]);

    expect(nav.projectNav()[0].items.map((i) => i.label)).toEqual(['Dnsendpoints']);
  });

  it('prefers the label the manifest states over the kind', () => {
    const nav = navFor([definition('system', 'external-dns', 'External DNS', CRD, 'DNS records')], {
      [CRD]: 'DNSEndpoint',
    });

    expect(nav.projectNav()[0].items.map((i) => i.label)).toEqual(['DNS records']);
  });

  it('qualifies only the ambiguous group', () => {
    const nav = navFor([
      definition('system', 'cert-manager', 'Cert Manager'),
      definition('acme-corp', 'cert-manager', 'Cert Manager'),
      definition('system', 'grafana', 'Grafana'),
    ]);

    expect(nav.projectNav().map((g) => g.label)).toEqual([
      'Cert Manager (system)',
      'Cert Manager (acme-corp)',
      'Grafana',
    ]);
  });
});
