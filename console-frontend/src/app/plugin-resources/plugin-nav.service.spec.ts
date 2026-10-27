import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { describe, expect, it, vi } from 'vitest';
import PluginNavService from './plugin-nav.service';
import PluginRegistryService from './plugin-registry.service';
import type { PluginDefinition } from './types';

function definition(
  organizationName: string,
  name: string,
  label: string,
  crd = 'certificates.cert-manager.io',
  menuLabel?: string,
  section: 'organization' | 'project' = 'project',
): PluginDefinition {
  return {
    name,
    label,
    version: 'v1',
    description: '',
    menu: { [section]: [{ crd, label: menuLabel }] },
    crds: [crd],
    allowedResources: [],
    installationId: `${organizationName}-${name}-uid`,
    installationName: `${organizationName}--${name}`,
    installationVersion: 'v1',
    organizationName,
  };
}

function navFor(
  plugins: PluginDefinition[],
  kinds: Record<string, string> = {},
  scopes: Record<string, 'Namespaced' | 'Cluster'> = {},
): PluginNavService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      {
        provide: PluginRegistryService,
        useValue: {
          allPlugins: signal(plugins),
          crdKind: (crdRef: string) => kinds[crdRef],
          crdScope: (crdRef: string) => scopes[crdRef],
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

  it('clusterNav lists menu.organization entries', () => {
    const nav = navFor([
      definition('system', 'external-dns', 'External DNS', CRD, undefined, 'organization'),
    ]);

    expect(nav.clusterNav().map((g) => g.installationName)).toEqual(['system--external-dns']);
    expect(nav.clusterNav()[0].items.map((i) => i.crdPlural)).toEqual([CRD]);
    expect(nav.projectNav()).toEqual([]);
  });

  it('drops a Namespaced CRD under organization', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const nav = navFor(
      [definition('system', 'external-dns', 'External DNS', CRD, undefined, 'organization')],
      {},
      { [CRD]: 'Namespaced' },
    );

    expect(nav.clusterNav()).toEqual([]);
    expect(warn).toHaveBeenCalledWith(
      `[PluginNav] ${CRD} is Namespaced-scoped; not shown under menu.organization`,
    );
    warn.mockRestore();
  });

  it('warns once about a mismatched entry across recomputes', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const plugins = signal([
      definition('system', 'external-dns', 'External DNS', CRD, undefined, 'organization'),
    ]);
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        {
          provide: PluginRegistryService,
          useValue: {
            allPlugins: plugins,
            crdKind: () => undefined,
            crdScope: () => 'Namespaced',
          },
        },
      ],
    });
    const nav = TestBed.inject(PluginNavService);

    expect(nav.clusterNav()).toEqual([]);
    plugins.set([...plugins()]);
    expect(nav.clusterNav()).toEqual([]);

    expect(warn).toHaveBeenCalledTimes(1);
    warn.mockRestore();
  });

  it('drops a Cluster CRD under project', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const nav = navFor(
      [definition('system', 'external-dns', 'External DNS', CRD)],
      {},
      { [CRD]: 'Cluster' },
    );

    expect(nav.projectNav()).toEqual([]);
    expect(warn).toHaveBeenCalledWith(
      `[PluginNav] ${CRD} is Cluster-scoped; not shown under menu.project`,
    );
    warn.mockRestore();
  });

  it('keeps an entry whose scope is not known yet', () => {
    const nav = navFor([definition('system', 'external-dns', 'External DNS', CRD)]);

    expect(nav.projectNav()[0].items).toHaveLength(1);
  });
});
