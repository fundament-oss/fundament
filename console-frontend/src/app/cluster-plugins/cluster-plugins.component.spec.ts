import { TestBed } from '@angular/core/testing';
import { ActivatedRoute } from '@angular/router';
import { vi } from 'vitest';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import ClusterPluginsComponent from './cluster-plugins.component';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
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

const certManager: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-2',
  name: 'cert-manager',
  displayName: 'Cert Manager',
  organizationName: 'acme',
  pluginVersion: 'v1.0.0',
  definitionHash: 'sha256:def',
});

function build(
  definitionResponse: {
    configSchema: { name: string; required?: boolean }[];
    configSchemaUnavailable?: boolean;
  },
  opts: {
    plugins?: PluginSummary[];
    installations?: {
      metadata: { name: string };
      spec: { definitionRef: { pluginName: string } };
    }[];
    status?: ClusterStatus;
  } = {},
) {
  const installationService = {
    listInstallations: () => Promise.resolve(opts.installations ?? []),
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
      { provide: PageNavService, useValue: { goTo: vi.fn(), backText: () => 'production' } },
      { provide: NotificationService, useValue: { success: vi.fn() } },
      {
        provide: CLUSTER,
        useValue: {
          getCluster: () =>
            of({ cluster: { name: 'production', status: opts.status ?? ClusterStatus.RUNNING } }),
        } as unknown as ObservableClient<typeof ClusterService>,
      },
      {
        provide: PLUGIN,
        useValue: {
          listPlugins: () => of({ plugins: opts.plugins ?? [cephRook] }),
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
  return {
    component,
    installationService,
    pageNav: TestBed.inject(PageNavService) as unknown as { goTo: ReturnType<typeof vi.fn> },
  };
}

// A page in the cluster's menu, not a sheet over the cluster page: saving
// leaves you where you are, with the form reloaded from what is now installed.
describe('ClusterPluginsComponent as a page', () => {
  it('stays on the page and reloads after a save', async () => {
    const { component, pageNav } = build({ configSchema: [] });
    await component.load();
    const reload = vi.spyOn(component, 'load');

    await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });

    expect(pageNav.goTo).not.toHaveBeenCalled();
    expect(reload).toHaveBeenCalled();
    expect(component.errorMessage()).toBeNull();
  });
});

// This page's bulk form has no per-plugin config step, so a plugin whose
// definition declares required config (or whose schema the catalog cannot
// read) must be refused before anything is installed or uninstalled.
describe('ClusterPluginsComponent required-config guard', () => {
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

  // A blocked install must not swallow the rest of the submit: the uninstall
  // the user also requested still goes through, and the banner says so.
  it('still uninstalls when another plugin is refused for required config', async () => {
    const { component, installationService } = build(
      { configSchema: [{ name: 'FAILURE_DOMAIN', required: true }] },
      {
        plugins: [cephRook, certManager],
        installations: [
          {
            metadata: { name: 'acme--cert-manager' },
            spec: { definitionRef: { pluginName: 'cert-manager' } },
          },
        ],
      },
    );
    await component.load();

    // cert-manager unchecked (uninstall), ceph-rook checked (blocked install).
    await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });

    expect(installationService.installPlugin).not.toHaveBeenCalled();
    expect(installationService.uninstallPlugin).toHaveBeenCalledWith('c1', 'acme--cert-manager');
    expect(component.errorMessage()).toContain('Ceph Rook');
  });
});

// Plugin management talks to the cluster's API server, which answers in every
// reachable status, not only RUNNING: starting, upgrading and unhealthy
// clusters serve, while a stopped one does not.
describe('ClusterPluginsComponent reachable gate', () => {
  it.each([ClusterStatus.STARTING, ClusterStatus.UPGRADING, ClusterStatus.UNHEALTHY])(
    'offers the form and submits on reachable status %i',
    async (status) => {
      const { component, installationService } = build({ configSchema: [] }, { status });
      await component.load();

      expect(component['notRunning']()).toBe(false);

      await component.onFormSubmit({ preset: '', plugins: ['pl-1'] });
      expect(installationService.installPlugin).toHaveBeenCalled();
    },
  );

  it('shows the not-running notice for a stopped cluster', async () => {
    const { component } = build({ configSchema: [] }, { status: ClusterStatus.STOPPED });
    await component.load();

    expect(component['notRunning']()).toBe(true);
  });
});
