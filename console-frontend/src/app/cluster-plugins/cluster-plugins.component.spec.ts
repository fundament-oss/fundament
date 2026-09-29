import { TestBed } from '@angular/core/testing';
import { ActivatedRoute } from '@angular/router';
import { vi } from 'vitest';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import ClusterPluginsComponent from './cluster-plugins.component';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { CLUSTER, INSTALL, PLUGIN } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import { ClusterStatus } from '../../generated/v1/common_pb';
import {
  PluginSummarySchema,
  type PluginService,
  type PluginSummary,
} from '../../generated/v1/plugin_pb';
import type { ClusterService } from '../../generated/v1/cluster_pb';
import type { InstallService } from '../../generated/install/v1/install_pb';

const cephRook: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-1',
  name: 'ceph-rook',
  displayName: 'Ceph Rook',
  organizationName: 'acme',
  pluginVersion: 'v0.2.0',
  definitionHash: 'sha256:abc',
});

// This page's bulk form has no per-plugin config step, so a plugin whose
// definition declares required config (or whose schema the catalog cannot
// read) must be refused before anything is installed or uninstalled.
describe('ClusterPluginsComponent required-config guard', () => {
  function build(definitionResponse: {
    configSchema: { name: string; required?: boolean }[];
    configSchemaUnavailable?: boolean;
  }) {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      installPlugin: vi.fn().mockResolvedValue(undefined),
      uninstallPlugin: vi.fn().mockResolvedValue(undefined),
    };
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ActivatedRoute,
          useValue: { snapshot: { paramMap: new Map([['id', 'c1']]) } },
        },
        { provide: TitleService, useValue: { setTitle: () => {} } },
        { provide: PageNavService, useValue: { goTo: vi.fn() } },
        {
          provide: CLUSTER,
          useValue: {
            getCluster: () =>
              of({ cluster: { name: 'production', status: ClusterStatus.RUNNING } }),
          } as unknown as ObservableClient<typeof ClusterService>,
        },
        {
          provide: PLUGIN,
          useValue: {
            listPlugins: () => of({ plugins: [cephRook] }),
          } as unknown as ObservableClient<typeof PluginService>,
        },
        {
          provide: INSTALL,
          useValue: {
            getPluginDefinition: () => of(definitionResponse),
          } as unknown as ObservableClient<typeof InstallService>,
        },
        {
          provide: PluginInstallationService,
          useValue: installationService as unknown as PluginInstallationService,
        },
      ],
    });
    const component = TestBed.createComponent(ClusterPluginsComponent).componentInstance;
    return { component, installationService };
  }

  it('installs a plugin whose schema declares no required keys', async () => {
    const { component, installationService } = build({
      configSchema: [{ name: 'MON_COUNT' }],
    });
    await component.load();

    await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });

    expect(installationService.installPlugin).toHaveBeenCalledWith(
      'c1',
      'acme',
      'ceph-rook',
      'v0.2.0',
      'sha256:abc',
    );
    expect(component.errorMessage()).toBeNull();
  });

  it('refuses to install a plugin with a required config key', async () => {
    const { component, installationService } = build({
      configSchema: [{ name: 'FAILURE_DOMAIN', required: true }],
    });
    await component.load();

    await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });

    expect(installationService.installPlugin).not.toHaveBeenCalled();
    expect(component.errorMessage()).toContain('Ceph Rook');
  });

  it('fails closed when the catalog cannot read the schema', async () => {
    const { component, installationService } = build({
      configSchema: [],
      configSchemaUnavailable: true,
    });
    await component.load();

    await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });

    expect(installationService.installPlugin).not.toHaveBeenCalled();
    expect(component.errorMessage()).toContain('Ceph Rook');
  });
});
