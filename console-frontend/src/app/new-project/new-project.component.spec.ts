import { TestBed } from '@angular/core/testing';
import { Code, ConnectError, type CallOptions } from '@connectrpc/connect';
import { type Observable, Subject, of, throwError } from 'rxjs';
import { signal } from '@angular/core';
import { vi } from 'vitest';
import NewProjectComponent from './new-project.component';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import PermissionsService from '../permissions.service';
import OrganizationContextService from '../organization-context.service';
import { Action } from '../../generated/v1/actions_pb';
import { type PermissionCheck } from '../../generated/v1/authz_pb';
import { PROJECT } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import type { ProjectService } from '../../generated/v1/project_pb';

interface CreateResponse {
  projectId: string;
}

describe('NewProjectComponent submit', () => {
  function build(
    respond: () => Observable<CreateResponse>,
    can: (checks: PermissionCheck[]) => Promise<boolean[]> = (checks) =>
      Promise.resolve(checks.map(() => true)),
    clusters = [{ id: 'c1', name: 'test', region: 'local' }],
  ) {
    const createProject = vi.fn((_req: unknown, opts: CallOptions) => {
      opts.onHeader?.(new Headers({ 'Idempotency-Status': 'completed' }));
      return respond();
    });
    const goTo = vi.fn();
    const clusterSummaries = signal(clusters);
    TestBed.configureTestingModule({
      providers: [
        { provide: PageNavService, useValue: { goTo, path: (p: string) => p } },
        { provide: PermissionsService, useValue: { can } },
        { provide: NotificationService, useValue: { success: vi.fn(), error: vi.fn() } },
        {
          provide: OrganizationDataService,
          useValue: {
            reloadProjectsAndNamespaces: () => Promise.resolve(),
            clusterSummaries,
            clustersLoaded: signal(true),
            clustersLoadFailed: signal(false),
          },
        },
        {
          provide: PROJECT,
          useValue: { createProject } as unknown as ObservableClient<typeof ProjectService>,
        },
      ],
    });
    TestBed.inject(OrganizationContextService).setOrganizationId('org-1');
    const component = TestBed.createComponent(NewProjectComponent).componentInstance;
    return { component, createProject, goTo, clusterSummaries };
  }

  /** Lets the permission answers land. */
  async function ready() {
    TestBed.tick();
    await new Promise((resolve) => {
      setTimeout(resolve);
    });
    TestBed.tick();
  }

  it('ignores a submit while the first create is in flight, and lets that create finish', async () => {
    const response = new Subject<CreateResponse>();
    const { component, createProject, goTo } = build(() => response);
    await ready();
    component.projectForm.patchValue({ name: 'demo' });

    const first = component.onSubmit();
    await component.onSubmit();
    response.next({ projectId: 'p1' });
    response.complete();
    await first;

    expect(createProject).toHaveBeenCalledTimes(1);
    expect(goTo).toHaveBeenCalledWith('/projects/p1');
  });

  it('accepts a new submit after a failed create', async () => {
    const { component, createProject } = build(() =>
      throwError(() => new ConnectError('down', Code.Unavailable)),
    );
    await ready();
    component.projectForm.patchValue({ name: 'demo' });

    await component.onSubmit();
    await component.onSubmit();

    expect(createProject).toHaveBeenCalledTimes(2);
    expect(component.errorMessage()).toContain('down');
  });

  it('reports a taken name on the name field until the name changes', async () => {
    const { component } = build(() =>
      throwError(() => new ConnectError('taken', Code.AlreadyExists)),
    );
    await ready();
    component.projectForm.patchValue({ name: 'demo' });

    await component.onSubmit();

    expect(component.errorMessage()).toBeNull();
    expect(component.getNameError()).toBe(
      'A project with this name already exists in this cluster.',
    );

    component.onNameInput(new CustomEvent('input', { detail: { value: 'demo-2' } }));
    expect(component.getNameError()).toBe('');
  });

  it('offers only the clusters that allow CreateProject, and selects the first of them', async () => {
    const { component } = build(
      () => of({ projectId: 'p1' }),
      (checks) =>
        Promise.resolve(
          checks.map((c) => c.action === Action.CLUSTER_CAN_CREATE_PROJECT && c.objectId === 'c2'),
        ),
      [
        { id: 'c1', name: 'one', region: 'local' },
        { id: 'c2', name: 'two', region: 'local' },
      ],
    );
    await ready();

    expect(component.allowedClusters().map((c) => c.id)).toEqual(['c2']);
    expect(component.projectForm.value.clusterId).toBe('c2');
  });

  it('offers no cluster when the check fails', async () => {
    const { component } = build(
      () => of({ projectId: 'p1' }),
      (checks) => Promise.resolve(checks.map(() => false)),
    );
    await ready();

    expect(component.allowedClusters()).toEqual([]);
    expect(component.projectForm.value.clusterId).toBe('');
  });

  it('keeps the selected cluster when the cluster list changes behind the sheet', async () => {
    const two = [
      { id: 'c1', name: 'one', region: 'local' },
      { id: 'c2', name: 'two', region: 'local' },
    ];
    let secondAnswer: (allowed: boolean[]) => void = () => {};
    let calls = 0;
    const { component, clusterSummaries } = build(
      () => of({ projectId: 'p1' }),
      (checks) => {
        calls += 1;
        if (calls === 1) return Promise.resolve(checks.map(() => true));
        return new Promise((resolve) => {
          secondAnswer = (allowed) => resolve(allowed);
        });
      },
      two,
    );
    await ready();
    component.projectForm.patchValue({ clusterId: 'c2' });
    const seen: (string | null)[] = [];
    component.projectForm.controls.clusterId.valueChanges.subscribe((v) => seen.push(v));

    clusterSummaries.set([...two, { id: 'c3', name: 'three', region: 'local' }]);
    await ready();
    expect(calls).toBe(2);
    expect(component.projectForm.value.clusterId).toBe('c2');

    secondAnswer([true, true, true, true]);
    await ready();

    expect(component.projectForm.value.clusterId).toBe('c2');
    expect(seen).toEqual([]);
  });

  it("follows the organization's clusters when they change", async () => {
    const { component, clusterSummaries } = build(() => of({ projectId: 'p1' }));
    await ready();
    expect(component.allowedClusters().map((c) => c.id)).toEqual(['c1']);

    clusterSummaries.set([{ id: 'c9', name: 'other-org', region: 'local' }]);
    await ready();

    expect(component.allowedClusters().map((c) => c.id)).toEqual(['c9']);
    expect(component.projectForm.value.clusterId).toBe('c9');
  });
});
