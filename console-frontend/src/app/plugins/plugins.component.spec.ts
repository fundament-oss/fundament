import { TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import PluginsComponent from './plugins.component';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { OrganizationDataService } from '../organization-data.service';
import { ToastService } from '../toast.service';
import { PLUGIN, CLUSTER } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import {
  PluginSummarySchema,
  PluginService,
  type PluginSummary,
} from '../../generated/v1/plugin_pb';
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
const acmeCertManager: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-acme--cert-manager',
  organizationName: 'acme',
  name: 'cert-manager',
  displayName: 'Acme Cert Manager',
});

const globexCertManager: PluginSummary = create(PluginSummarySchema, {
  id: 'pl-globex-cert-manager',
  organizationName: 'globex',
  name: 'cert-manager',
  displayName: 'Globex Cert Manager',
});

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

function build(plugins: PluginSummary[], installs: PluginInstallationItem[]) {
  TestBed.configureTestingModule({
    providers: [
      {
        provide: PLUGIN,
        useValue: {
          listPlugins: () => of({ plugins }),
          listPresets: () => of({ presets: [] }),
        } as unknown as ObservableClient<typeof PluginService>,
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
        } as unknown as PluginInstallationService,
      },
      {
        provide: OrganizationDataService,
        useValue: {
          clusterSummaries: () => [cluster],
        } as unknown as OrganizationDataService,
      },
      {
        provide: ToastService,
        useValue: { success: () => {}, error: () => {} } as unknown as ToastService,
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
