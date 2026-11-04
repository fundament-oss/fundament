import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';
import { vi } from 'vitest';
import ClusterNamespacesComponent from './cluster-namespaces.component';
import PermissionsService from '../permissions.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { CLUSTER, NAMESPACE, PROJECT } from '../../connect/tokens';
import { Action, type PermissionCheck } from '../../generated/v1/authz_pb';
import { NamespaceSchema } from '../../generated/v1/namespace_pb';

const CLUSTER_ID = '11111111-1111-4111-8111-111111111111';
const NS_A = '22222222-2222-4222-8222-222222222222';
const NS_B = '33333333-3333-4333-8333-333333333333';

function build(can: (checks: PermissionCheck[]) => Promise<boolean[]>) {
  TestBed.configureTestingModule({
    providers: [
      { provide: PermissionsService, useValue: { can: vi.fn(can) } },
      {
        provide: ActivatedRoute,
        useValue: {
          snapshot: { paramMap: convertToParamMap({ id: CLUSTER_ID }) },
          paramMap: of(convertToParamMap({ id: CLUSTER_ID })),
        },
      },
      { provide: PageNavService, useValue: { path: (p: string) => p, goTo: vi.fn() } },
      { provide: TitleService, useValue: { setTitle: vi.fn() } },
      { provide: NotificationService, useValue: { success: vi.fn(), error: vi.fn() } },
      { provide: OrganizationDataService, useValue: {} },
      { provide: CLUSTER, useValue: { getCluster: () => of({}) } },
      { provide: PROJECT, useValue: { listProjects: () => of({ projects: [] }) } },
      {
        provide: NAMESPACE,
        useValue: {
          listClusterNamespaces: () =>
            of({
              namespaces: [
                create(NamespaceSchema, { id: NS_A, name: 'alpha' }),
                create(NamespaceSchema, { id: NS_B, name: 'beta' }),
              ],
            }),
        },
      },
    ],
  });
  return TestBed.createComponent(ClusterNamespacesComponent);
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

// The sheet is portaled to document.body.
const rowDeletes = () =>
  Array.from(document.body.querySelectorAll('nldd-list-item nldd-button[text="Delete"]'));

const bulkDelete = () =>
  document.body.querySelector('nldd-box nldd-button[text="Delete namespaces"]');

describe('ClusterNamespacesComponent delete permission', () => {
  it('hides every Delete until the answer arrives, then shows them on true', async () => {
    let answer!: (allowed: boolean[]) => void;
    const fixture = build(
      () =>
        new Promise((resolve) => {
          answer = resolve;
        }),
    );
    await settle(fixture);
    expect(rowDeletes()).toHaveLength(0);
    expect(bulkDelete()).toBeNull();

    answer([true, true]);
    await settle(fixture);
    expect(rowDeletes()).toHaveLength(2);
    expect(bulkDelete()).not.toBeNull();
  });

  it('hides everything on false', async () => {
    const fixture = build((checks) => Promise.resolve(checks.map(() => false)));
    await settle(fixture);
    expect(rowDeletes()).toHaveLength(0);
    expect(bulkDelete()).toBeNull();
  });

  it('shows Delete per row and no bulk delete when the rows differ', async () => {
    const fixture = build((checks) => Promise.resolve(checks.map((c) => c.objectId === NS_A)));
    await settle(fixture);
    expect(rowDeletes()).toHaveLength(1);
    expect(bulkDelete()).toBeNull();
  });

  it('checks DeleteNamespace on each row in one call', async () => {
    const fixture = build((checks) => Promise.resolve(checks.map(() => true)));
    await settle(fixture);
    const can = TestBed.inject(PermissionsService).can as ReturnType<typeof vi.fn>;
    const calls = can.mock.calls.map(([checks]) => checks as PermissionCheck[]);
    expect(calls).toHaveLength(1);
    expect(calls[0]).toEqual([
      expect.objectContaining({ action: Action.DELETE_NAMESPACE, objectId: NS_A }),
      expect.objectContaining({ action: Action.DELETE_NAMESPACE, objectId: NS_B }),
    ]);
  });
});
