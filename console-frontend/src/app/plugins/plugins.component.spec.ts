import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { vi } from 'vitest';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import PluginsComponent from './plugins.component';
import PluginInstallationService, {
  RetryReadError,
} from '../plugin-installation/plugin-installation.service';
import { ConfigService } from '../config.service';
import { OrganizationDataService } from '../organization-data.service';
import { NotificationService } from '../notification.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { CLUSTER, CATALOG, INSTALL } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import {
  PluginSummarySchema,
  CatalogService,
  type PluginSummary,
} from '../../generated/catalog/v1/catalog_pb';
import { InstallService } from '../../generated/install/v1/install_pb';
import {
  ListClustersResponse_ClusterSummarySchema,
  ClusterService,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { ClusterStatus } from '../../generated/v1/common_pb';
import type { PluginInstallationItem } from '../plugin-resources/types';

const cluster: ClusterSummary = create(ListClustersResponse_ClusterSummarySchema, {
  id: 'cl-1',
  name: 'production',
  status: ClusterStatus.RUNNING,
});

// Two organizations publishing a plugin under the identical catalog name — the
// scenario this branch exists to support, and the one unqualified matching would
// conflate.
// catalog.v1 carries the publisher as an id; the component resolves the name
// through ListPublishers, so the stub below answers both.
const acmeCertManager: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-acme--cert-manager',
  organizationId: 'org-acme',
  name: 'cert-manager',
  displayName: 'Acme Cert Manager',
});

const globexCertManager: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-globex-cert-manager',
  organizationId: 'org-globex',
  name: 'cert-manager',
  displayName: 'Globex Cert Manager',
});

const publishers = [
  { id: 'org-acme', name: 'acme', displayName: 'Acme' },
  { id: 'org-globex', name: 'globex', displayName: 'Globex' },
];

// Only acme's install exists on the cluster.
const acmeInstall: PluginInstallationItem = {
  metadata: { name: 'acme--cert-manager', uid: 'uid-1' },
  spec: {
    definitionRef: {
      organizationName: 'acme',
      pluginName: 'cert-manager',
      pluginVersion: 'v1.0.0',
      definitionHash: 'sha256:demo',
    },
  },
  status: { phase: 'Running', ready: true },
};

const staging: ClusterSummary = create(ListClustersResponse_ClusterSummarySchema, {
  id: 'cl-2',
  name: 'staging',
  status: ClusterStatus.RUNNING,
});

/** The route the component is opened on: its data, params and query. */
interface RouteSetup {
  data?: Record<string, unknown>;
  params?: Record<string, string>;
  query?: Record<string, string>;
}

function build(
  plugins: PluginSummary[],
  installs: PluginInstallationItem[],
  marketplaceUrl = '',
  installationService: Partial<PluginInstallationService> = {},
  route?: RouteSetup,
  clusters: ClusterSummary[] = [cluster],
  // Whether the shell's cluster list is in by the time the page loads.
  clustersLoaded = true,
) {
  TestBed.configureTestingModule({
    providers: [
      ...(route
        ? [
            {
              provide: ActivatedRoute,
              useValue: {
                snapshot: {
                  data: route.data ?? {},
                  paramMap: convertToParamMap(route.params ?? {}),
                  queryParamMap: convertToParamMap(route.query ?? {}),
                },
              },
            },
          ]
        : []),
      {
        provide: CATALOG,
        useValue: {
          listCategories: () => of({ categories: [] }),
          listPresets: () => of({ presets: [] }),
        } as unknown as ObservableClient<typeof CatalogService>,
      },
      {
        // Listings and versions come from install.v1 as the organization (FUN-22).
        provide: INSTALL,
        useValue: {
          listPlugins: () => of({ plugins }),
          // Latest first, as install.v1 returns them.
          listPluginVersions: () =>
            of({
              versions: [
                { version: '1.17.2', definitionHash: 'sha256:new' },
                { version: '1.16.0', definitionHash: 'sha256:old' },
              ],
            }),
          listPublishers: () => of({ publishers }),
        } as unknown as ObservableClient<typeof InstallService>,
      },
      {
        provide: CLUSTER,
        useValue: {
          listClusters: () => of({ clusters }),
        } as unknown as ObservableClient<typeof ClusterService>,
      },
      {
        provide: PluginInstallationService,
        useValue: {
          listInstallations: (clusterId: string) =>
            Promise.resolve(clusterId === cluster.id ? installs : []),
          ...installationService,
        } as unknown as PluginInstallationService,
      },
      {
        provide: OrganizationDataService,
        useValue: {
          clusterSummaries: () => (clustersLoaded ? clusters : []),
          clustersLoaded: () => clustersLoaded,
        } as unknown as OrganizationDataService,
      },
      {
        // marketplaceUrl decides whether the page is the catalog or lists only
        // what is installed; unconfigured unless a test says otherwise.
        provide: ConfigService,
        useValue: {
          getConfig: () => ({ marketplaceUrl }),
        } as unknown as ConfigService,
      },
      {
        provide: NotificationService,
        useValue: { success: () => {}, error: () => {} } as unknown as NotificationService,
      },
      { provide: TitleService, useValue: { setTitle: () => {} } },
      {
        provide: PageNavService,
        useValue: {
          backText: () => '',
          path: (within: string) => within,
          goTo: vi.fn(),
        } as unknown as PageNavService,
      },
    ],
  });
  return TestBed.createComponent(PluginsComponent).componentInstance;
}

describe('PluginsComponent install-status matching', () => {
  it('reports a plugin installed under its qualified name as installed for the matching catalog entry', async () => {
    const component = build([acmeCertManager, globexCertManager], [acmeInstall]);
    await component.ngOnInit();

    expect(component.isPluginInstalledAnywhere('acme', 'cert-manager')).toBe(true);
    expect(component.installedClusterCount('acme', 'cert-manager')).toBe(1);
  });

  // The regression this guards: metadata.name is now the RFC-1123 slug of
  // (organizationName, pluginName), e.g. "acme--cert-manager". Matching on the
  // catalog's unqualified `name` alone would also mark globex's identically-named
  // "cert-manager" as installed, even though only acme's is.
  it('does not report the same plugin name published by a different organization as installed', async () => {
    const component = build([acmeCertManager, globexCertManager], [acmeInstall]);
    await component.ngOnInit();

    expect(component.isPluginInstalledAnywhere('globex', 'cert-manager')).toBe(false);
    expect(component.installedClusterCount('globex', 'cert-manager')).toBe(0);
  });

  it('counts a Degraded installation as installed, as the plugin page does', async () => {
    const degraded = { ...acmeInstall, status: { phase: 'Degraded', ready: false } };
    const component = build([acmeCertManager], [degraded]);
    await component.ngOnInit();

    expect(component.installedClusterCount('acme', 'cert-manager')).toBe(1);
    expect(component.pluginStatusLabel('acme', 'cert-manager')).toBe('Installed on 1 cluster');
  });

  it('says an installation still coming up is installing, not that nothing is installed', async () => {
    const pending = { ...acmeInstall, status: { phase: 'Pending', ready: false } };
    const component = build([acmeCertManager], [pending]);
    await component.ngOnInit();

    expect(component.installedClusterCount('acme', 'cert-manager')).toBe(0);
    expect(component.pluginStatusLabel('acme', 'cert-manager')).toBe('Installing…');
  });

  it('falls back to the plugin name, without throwing, for an install whose catalog entry is gone', () => {
    const component = build([], []);
    const withPrivateAccess = component as unknown as {
      pluginDisplayName(organizationName: string, pluginName: string): string;
    };

    expect(withPrivateAccess.pluginDisplayName('acme', 'cert-manager')).toBe('cert-manager');
  });
});

describe('PluginsComponent details sheet', () => {
  it("shows the publisher's display name and the latest published version", async () => {
    const component = build([acmeCertManager], []);
    await component.ngOnInit();
    const [plugin] = component.plugins;

    await component.openPluginDetails(plugin);

    expect(plugin.publisherDisplayName).toBe('Acme');
    expect(component.sheetPluginVersion()).toBe('1.17.2');
  });

  it('drops a version that resolves after the sheet was closed', async () => {
    const component = build([acmeCertManager], []);
    await component.ngOnInit();

    const opening = component.openPluginDetails(component.plugins[0]);
    component.closePluginDetails();
    await opening;

    expect(component.sheetPluginVersion()).toBe('');
  });
});

describe('PluginsComponent listing', () => {
  it('lists the whole catalog when no marketplace is deployed', async () => {
    const component = build([acmeCertManager, globexCertManager], [acmeInstall]);
    await component.ngOnInit();

    expect(component.listedPlugins.map((p) => p.id)).toEqual([
      acmeCertManager.id,
      globexCertManager.id,
    ]);
    expect(component.summaryText).toBe('2 of 2 plugins');
  });

  it('lists only the installed plugins when the marketplace does the browsing', async () => {
    const component = build(
      [acmeCertManager, globexCertManager],
      [acmeInstall],
      'https://marketplace.example.test/',
    );
    await component.ngOnInit();

    expect(component.listedPlugins.map((p) => p.id)).toEqual([acmeCertManager.id]);
    expect(component.filteredPlugins.map((p) => p.id)).toEqual([acmeCertManager.id]);
    expect(component.summaryText).toBe('1 of 1 plugins');
  });

  it('keeps an installation that is still coming up in the installed list', async () => {
    const pending: PluginInstallationItem = {
      ...acmeInstall,
      status: undefined as unknown as PluginInstallationItem['status'],
    };
    const component = build([acmeCertManager, globexCertManager], [pending], 'https://m.test');
    await component.ngOnInit();

    expect(component.listedPlugins.map((p) => p.id)).toEqual([acmeCertManager.id]);
  });

  // The catalog only returns published listings, so a plugin unpublished after
  // it was installed has no entry, yet still runs.
  it('keeps an installed plugin that is no longer in the catalog', async () => {
    const component = build([globexCertManager], [acmeInstall], 'https://m.test');
    await component.ngOnInit();

    const [plugin] = component.listedPlugins;
    expect(component.listedPlugins).toHaveLength(1);
    expect(plugin).toMatchObject({
      organizationName: 'acme',
      name: 'cert-manager',
      unlisted: true,
    });
    expect(component.marketplacePluginUrl(plugin)).toBe('');

    await component.onInstallPlugin(plugin);
    expect(component.installVersions()).toEqual([]);
    expect(component.installVersionsError()).toBe(false);
  });
});

// The read-config/uninstall/re-create sequence itself is the service's;
// see plugin-installation.service.spec.ts. These pin the delegation and the
// component's handling of an aborted retry.
describe('PluginsComponent retry install', () => {
  // Selected as it would be from opening the modal off the plugin row.
  const selectedPlugin = {
    id: 'pl-1',
    name: 'ceph-rook',
    displayName: 'Ceph Rook',
    descriptionShort: '',
    image: '',
    logo: '',
    logoDark: '',
    organizationName: 'acme',
    publisherDisplayName: 'Acme',
    categories: [],
    tags: [],
  };

  it('retries through the service at the chosen version and hash', async () => {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      retryInstall: vi.fn().mockResolvedValue(undefined),
    };

    const component = build([], [], '', installationService);
    component.selectedPlugin = { ...selectedPlugin };

    await component.onRetryInstall({ clusterId: 'c1', version: 'v0.2.0', hash: 'sha256:abc' });

    expect(installationService.retryInstall).toHaveBeenCalledWith(
      'c1',
      'acme',
      'ceph-rook',
      'v0.2.0',
      'sha256:abc',
    );
  });

  // A poll read that was out when the user acted must not put the row back to
  // the phase it had before.
  it('drops a poll read that started before an uninstall', async () => {
    const failed: PluginInstallationItem = {
      ...acmeInstall,
      metadata: { name: 'acme--ceph-rook', uid: 'uid-2' },
      spec: { definitionRef: { ...acmeInstall.spec.definitionRef, pluginName: 'ceph-rook' } },
      status: { phase: 'Failed', ready: false },
    };
    let resolveRead: (items: PluginInstallationItem[]) => void = () => {};
    const installationService = {
      listInstallations: vi.fn().mockResolvedValue([failed]),
      uninstallPlugin: vi.fn().mockResolvedValue(undefined),
    };

    const component = build([], [], '', installationService);
    await component.ngOnInit();
    component.selectedPlugin = { ...selectedPlugin };

    installationService.listInstallations.mockReturnValueOnce(
      new Promise<PluginInstallationItem[]>((resolve) => {
        resolveRead = resolve;
      }),
    );
    const read = component['refreshInstalls']();
    await component.onUninstallFromCluster(cluster.id);
    resolveRead([failed]);
    await read;

    expect(component.installs().find((i) => i.pluginName === 'ceph-rook')?.phase).toBe(
      'Terminating',
    );
  });

  // The message belongs to the phase the poll read; an optimistic phase change
  // must not carry it under a phase it no longer describes.
  it.each([
    ['a retry', 'Failed', 'ImagePullBackOff: image not found', 'Pending'],
    ['an uninstall', 'Running', 'Open the dashboard at https://example.test', 'Terminating'],
  ])(
    'clears the status message when %s changes the phase before the next poll',
    async (action, phase, message, next) => {
      const installed: PluginInstallationItem = {
        ...acmeInstall,
        metadata: { name: 'acme--ceph-rook', uid: 'uid-3' },
        spec: { definitionRef: { ...acmeInstall.spec.definitionRef, pluginName: 'ceph-rook' } },
        status: { phase, ready: phase === 'Running', message },
      };
      const installationService = {
        listInstallations: vi.fn().mockResolvedValue([installed]),
        retryInstall: vi.fn().mockResolvedValue(undefined),
        uninstallPlugin: vi.fn().mockResolvedValue(undefined),
      };

      const component = build([], [], '', installationService);
      await component.ngOnInit();
      component.selectedPlugin = { ...selectedPlugin };
      const row = () => component.installs().find((i) => i.pluginName === 'ceph-rook');
      expect(row()?.message).toBe(message);

      const acting =
        action === 'a retry'
          ? component.onRetryInstall({
              clusterId: cluster.id,
              version: 'v1.0.0',
              hash: 'sha256:demo',
            })
          : component.onUninstallFromCluster(cluster.id);

      expect(row()?.phase).toBe(next);
      expect(row()?.message).toBe('');
      await acting;
    },
  );

  it('notifies when the service aborts the retry on a failed read', async () => {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      retryInstall: vi.fn().mockRejectedValue(new RetryReadError('read failed')),
    };

    const component = build([], [], '', installationService);
    component.selectedPlugin = { ...selectedPlugin };
    const notificationService = TestBed.inject(NotificationService);
    const errorNotification = vi.spyOn(notificationService, 'error');

    await component.onRetryInstall({ clusterId: 'c1', version: 'v0.2.0', hash: 'sha256:abc' });

    expect(errorNotification).toHaveBeenCalledTimes(1);
  });
});

describe('PluginsComponent on a cluster', () => {
  const onProduction: RouteSetup = { data: { pluginsView: 'cluster' }, params: { id: cluster.id } };

  it('acts on the cluster it is about, not on every cluster', async () => {
    const component = build([acmeCertManager], [acmeInstall], '', {}, onProduction, [
      cluster,
      staging,
    ]);
    await component.ngOnInit();

    expect(component.clusters().map((c) => c.id)).toEqual([cluster.id]);
    component.selectedPlugin = component.plugins[0];
    expect(component.clustersForModal.map((c) => c.id)).toEqual([cluster.id]);
    expect(component.pluginStatusLabel('acme', 'cert-manager')).toBe('Installed');
  });

  it("shows the installation's phase, version and what it reports", async () => {
    const degraded: PluginInstallationItem = {
      ...acmeInstall,
      status: { phase: 'Degraded', ready: false, message: 'webhook not reachable' },
    };
    const component = build([acmeCertManager], [degraded], '', {}, onProduction);
    await component.ngOnInit();

    expect(component.installStatusRows('acme', 'cert-manager')).toEqual([
      {
        clusterId: cluster.id,
        clusterName: 'production',
        label: 'Degraded',
        badgeColor: 'warning',
        version: 'v1.0.0',
        message: 'The installation reports: “webhook not reachable”',
        configInvalid: false,
      },
    ]);
  });

  it('says a running installation is installed', async () => {
    const component = build([acmeCertManager], [acmeInstall], '', {}, onProduction);
    await component.ngOnInit();

    expect(component.installStatusRows('acme', 'cert-manager')).toMatchObject([
      { label: 'Installed', badgeColor: 'success', message: '' },
    ]);
  });

  it('opens the details of the plugin named in the address', async () => {
    const component = build(
      [acmeCertManager],
      [acmeInstall],
      '',
      {},
      {
        ...onProduction,
        query: { plugin: acmeCertManager.id },
      },
    );
    await component.ngOnInit();

    expect(component.sheetPlugin()?.id).toBe(acmeCertManager.id);
  });
});

describe('PluginsComponent install page', () => {
  const installPage = (pluginId: string): RouteSetup => ({
    data: { pluginsView: 'install' },
    params: { pluginId },
  });

  it('offers the plugin on every cluster of the organization', async () => {
    const component = build([acmeCertManager], [], '', {}, installPage(acmeCertManager.id), [
      cluster,
      staging,
    ]);
    await component.ngOnInit();
    await new Promise((resolve) => {
      setTimeout(resolve);
    });

    expect(component.selectedPlugin?.id).toBe(acmeCertManager.id);
    expect(component.showInstallModal()).toBe(true);
    expect(component.installVersions().map((v) => v.version)).toEqual(['1.17.2', '1.16.0']);
    expect(component.clustersForModal.map((c) => c.id)).toEqual([cluster.id, staging.id]);
  });

  // The marketplace's "Install plugin" opens this page in a new tab, where the
  // catalog can come back before the shell has the organization's clusters.
  it('reads the clusters itself when the shell has not got them yet', async () => {
    // In creation order, as the API lists them; the page shows them by name.
    const component = build(
      [acmeCertManager],
      [],
      '',
      {},
      installPage(acmeCertManager.id),
      [staging, cluster],
      false,
    );
    await component.ngOnInit();

    expect(component.clustersForModal.map((c) => c.name)).toEqual(['production', 'staging']);
  });

  // A page left while it loads (to another cluster's plugins, which mounts a
  // new one) must not start polling once its loads come back.
  it('starts no polling when left before its loads finish', async () => {
    const provisioning = { ...staging, status: ClusterStatus.PROVISIONING };
    const setIntervalSpy = vi.spyOn(globalThis, 'setInterval');
    const component = build([acmeCertManager], [], '', {}, installPage(acmeCertManager.id), [
      provisioning,
    ]);

    const loading = component.ngOnInit();
    component.ngOnDestroy();
    await loading;

    expect(setIntervalSpy).not.toHaveBeenCalled();
    setIntervalSpy.mockRestore();
  });

  it('says so when the plugin is not in the catalog', async () => {
    const component = build([acmeCertManager], [], '', {}, installPage('pl-gone'));
    await component.ngOnInit();

    expect(component.installPluginMissing()).toBe(true);
    expect(component.showInstallModal()).toBe(false);
  });

  it('goes on to the cluster the plugin was installed on', async () => {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      installPlugin: vi.fn().mockResolvedValue(undefined),
    };
    const component = build(
      [acmeCertManager],
      [],
      '',
      installationService,
      installPage(acmeCertManager.id),
      [cluster, staging],
    );
    const { goTo } = TestBed.inject(PageNavService);
    await component.ngOnInit();
    component.selectedPlugin = component.plugins[0];

    await component.onInstallOnClusters({
      clusterIds: [staging.id],
      version: '1.17.2',
      hash: 'sha256:new',
      config: {},
    });

    expect(installationService.installPlugin).toHaveBeenCalledWith(
      staging.id,
      'acme',
      'cert-manager',
      '1.17.2',
      'sha256:new',
      {},
    );
    expect(goTo).toHaveBeenCalledWith(
      `/clusters/${staging.id}/plugins?plugin=${acmeCertManager.id}`,
    );
  });
});
