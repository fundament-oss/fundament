import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { describe, expect, it } from 'vitest';
import PluginNavService from './plugin-nav.service';
import PluginRegistryService from './plugin-registry.service';
import type { PluginDefinition } from './types';

function definition(organizationName: string, name: string, label: string): PluginDefinition {
  return {
    name,
    label,
    version: 'v1',
    description: '',
    menu: { project: [{ crd: 'certificates.cert-manager.io' }] },
    crds: ['certificates.cert-manager.io'],
    allowedResources: [],
    installationId: `${organizationName}-${name}-uid`,
    installationName: `${organizationName}--${name}`,
    installationVersion: 'v1',
    organizationName,
  };
}

function navFor(plugins: PluginDefinition[]): PluginNavService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [{ provide: PluginRegistryService, useValue: { allPlugins: signal(plugins) } }],
  });
  return TestBed.inject(PluginNavService);
}

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
