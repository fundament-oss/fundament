import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import PluginsComponent from './plugins.component';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { ConfigService } from '../config.service';
import { OrganizationDataService } from '../organization-data.service';
import { NotificationService } from '../notification.service';
import { CLUSTER, CATALOG } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import {
  PluginSummarySchema,
  CatalogService,
  type PluginSummary,
} from '../../generated/catalog/v1/catalog_pb';
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

function build(
  plugins: PluginSummary[],
  installs: PluginInstallationItem[],
  installationService: Partial<PluginInstallationService> = {},
) {
  TestBed.configureTestingModule({
    providers: [
      {
        provide: CATALOG,
        useValue: {
          listPlugins: () => of({ plugins }),
          listCategories: () => of({ categories: [] }),
          listPublishers: () => of({ publishers }),
          listPresets: () => of({ presets: [] }),
        } as unknown as ObservableClient<typeof CatalogService>,
      },
      {
        provide: CLUSTER,
        useValue: {} as unknown as ObservableClient<typeof ClusterService>,
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
          clusterSummaries: () => [cluster],
        } as unknown as OrganizationDataService,
      },
      {
        // The details sheet reads marketplaceUrl to decide between the
        // marketplace listing and the console's own plugin page. Nothing here
        // exercises that link, so an unconfigured marketplace is enough.
        provide: ConfigService,
        useValue: {
          getConfig: () => ({ marketplaceUrl: '' }),
        } as unknown as ConfigService,
      },
      {
        provide: NotificationService,
        useValue: { success: () => {}, error: () => {} } as unknown as NotificationService,
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
    expect(component.runningInstallCount('acme', 'cert-manager')).toBe(1);
  });

  // The regression this guards: metadata.name is now the RFC-1123 slug of
  // (organizationName, pluginName), e.g. "acme--cert-manager". Matching on the
  // catalog's unqualified `name` alone would also mark globex's identically-named
  // "cert-manager" as installed, even though only acme's is.
  it('does not report the same plugin name published by a different organization as installed', async () => {
    const component = build([acmeCertManager, globexCertManager], [acmeInstall]);
    await component.ngOnInit();

    expect(component.isPluginInstalledAnywhere('globex', 'cert-manager')).toBe(false);
    expect(component.runningInstallCount('globex', 'cert-manager')).toBe(0);
  });

  it('falls back to the plugin name, without throwing, for an install whose catalog entry is gone', () => {
    const component = build([], []);
    const withPrivateAccess = component as unknown as {
      pluginDisplayName(organizationName: string, pluginName: string): string;
    };

    expect(withPrivateAccess.pluginDisplayName('acme', 'cert-manager')).toBe('cert-manager');
  });
});

describe('PluginsComponent retry install', () => {
  it('retry re-creates the installation with its previous config', async () => {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      getInstallation: vi.fn().mockResolvedValue({
        metadata: { name: 'acme--ceph-rook', uid: 'uid-1' },
        spec: {
          definitionRef: {
            organizationName: 'acme',
            pluginName: 'ceph-rook',
            pluginVersion: 'v0.2.0',
            definitionHash: 'sha256:abc',
          },
          config: { MON_COUNT: '1', DEV_LOOP_DEVICES: 'true' },
        },
        status: { phase: 'Failed', ready: false },
      } as PluginInstallationItem),
      uninstallPlugin: vi.fn().mockResolvedValue(undefined),
      installPlugin: vi.fn().mockResolvedValue(undefined),
    };

    const component = build([], [], installationService);
    // Selected as it would be from opening the modal off the plugin row.
    component.selectedPlugin = {
      id: 'pl-1',
      name: 'ceph-rook',
      displayName: 'Ceph Rook',
      descriptionShort: '',
      image: '',
      organizationName: 'acme',
      categories: [],
      tags: [],
    };

    await component.onRetryInstall({ clusterId: 'c1', version: 'v0.2.0', hash: 'sha256:abc' });

    expect(installationService.installPlugin).toHaveBeenCalledWith(
      'c1',
      'acme',
      'ceph-rook',
      'v0.2.0',
      'sha256:abc',
      { MON_COUNT: '1', DEV_LOOP_DEVICES: 'true' },
    );
  });

  // A read failure (network blip, RBAC hiccup) is not "the CR is already
  // gone": proceeding would uninstall the plugin's only copy of its config.
  it('aborts without uninstalling when reading the existing installation fails', async () => {
    const installationService = {
      listInstallations: () => Promise.resolve([]),
      getInstallation: vi.fn().mockRejectedValue(new Error('HTTP 500')),
      uninstallPlugin: vi.fn().mockResolvedValue(undefined),
      installPlugin: vi.fn().mockResolvedValue(undefined),
    };

    const component = build([], [], installationService);
    component.selectedPlugin = {
      id: 'pl-1',
      name: 'ceph-rook',
      displayName: 'Ceph Rook',
      descriptionShort: '',
      image: '',
      organizationName: 'acme',
      categories: [],
      tags: [],
    };
    const notificationService = TestBed.inject(NotificationService);
    const errorNotification = vi.spyOn(notificationService, 'error');

    await component.onRetryInstall({ clusterId: 'c1', version: 'v0.2.0', hash: 'sha256:abc' });

    expect(installationService.uninstallPlugin).not.toHaveBeenCalled();
    expect(installationService.installPlugin).not.toHaveBeenCalled();
    expect(errorNotification).toHaveBeenCalledTimes(1);
  });
});
