import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';
import { vi } from 'vitest';
import ClusterDetailsComponent from './cluster-details.component';
import PermissionsService from '../permissions.service';
import PageNavService from '../page-nav.service';
import { ConfigService } from '../config.service';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { CLUSTER, METRICS, PLUGIN, PROJECT } from '../../connect/tokens';
import { Action } from '../../generated/v1/actions_pb';
import { type PermissionCheck } from '../../generated/v1/authz_pb';
import { ClusterDetailsSchema } from '../../generated/v1/cluster_pb';

const CLUSTER_ID = '11111111-1111-4111-8111-111111111111';

function build(can: (checks: PermissionCheck[]) => Promise<boolean[]>) {
  TestBed.configureTestingModule({
    providers: [
      { provide: PermissionsService, useValue: { can: vi.fn(can) } },
      {
        provide: ActivatedRoute,
        useValue: {
          snapshot: {
            params: { id: CLUSTER_ID },
            paramMap: convertToParamMap({ id: CLUSTER_ID }),
          },
          paramMap: of(convertToParamMap({ id: CLUSTER_ID })),
        },
      },
      { provide: PageNavService, useValue: { path: (p: string) => p, goTo: vi.fn() } },
      { provide: ConfigService, useValue: { getConfig: () => ({}) } },
      { provide: NotificationService, useValue: { success: vi.fn(), error: vi.fn() } },
      {
        provide: OrganizationDataService,
        useValue: { nodePoolsChanged: signal(0), organizations: signal([]) },
      },
      {
        provide: PluginInstallationService,
        useValue: { listInstallations: () => Promise.resolve([]) },
      },
      {
        provide: CLUSTER,
        useValue: {
          getCluster: () =>
            of({
              cluster: create(ClusterDetailsSchema, { id: CLUSTER_ID, name: 'prod', region: 'eu' }),
            }),
          listNodePools: () => of({ nodePools: [] }),
          getClusterActivity: () => of({ events: [] }),
          getClusterDefaults: () => of({}),
        },
      },
      { provide: METRICS, useValue: { getClusterWorkloadMetrics: () => of({}) } },
      { provide: PROJECT, useValue: { listProjects: () => of({ projects: [] }) } },
      { provide: PLUGIN, useValue: { listPlugins: () => of({ plugins: [] }) } },
    ],
  });
  return TestBed.createComponent(ClusterDetailsComponent);
}

async function settle(fixture: ReturnType<typeof build>) {
  for (let i = 0; i < 3; i += 1) {
    fixture.detectChanges();
    // eslint-disable-next-line no-await-in-loop
    await fixture.whenStable();
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => {
      setTimeout(resolve);
    });
  }
}

const deleteButton = (fixture: ReturnType<typeof build>) =>
  (fixture.nativeElement as HTMLElement).querySelector(
    'nldd-box nldd-button[text="Delete cluster"]',
  );

describe('ClusterDetailsComponent delete permission', () => {
  it('hides Delete until the answer arrives, then shows it on true', async () => {
    // The defaults block on the page asks too, so keep every pending question.
    const pending: (() => void)[] = [];
    const fixture = build(
      (checks) =>
        new Promise((resolve) => {
          pending.push(() => resolve(checks.map(() => true)));
        }),
    );
    await settle(fixture);
    expect(deleteButton(fixture)).toBeNull();

    pending.forEach((resolve) => resolve());
    await settle(fixture);
    expect(deleteButton(fixture)).not.toBeNull();
  });

  it('keeps Delete hidden on false', async () => {
    const fixture = build((checks) => Promise.resolve(checks.map(() => false)));
    await settle(fixture);
    expect(deleteButton(fixture)).toBeNull();
  });

  it('asks for DeleteCluster on the routed cluster', async () => {
    const fixture = build((checks) => Promise.resolve(checks.map(() => true)));
    await settle(fixture);
    const can = TestBed.inject(PermissionsService).can as ReturnType<typeof vi.fn>;
    const asked = can.mock.calls.flatMap(([checks]) => checks as PermissionCheck[]);
    expect(asked).toContainEqual(
      expect.objectContaining({ action: Action.CLUSTER_CAN_DELETE, objectId: CLUSTER_ID }),
    );
  });
});
