import { TestBed } from '@angular/core/testing';
import { Code, ConnectError, type CallOptions } from '@connectrpc/connect';
import { type Observable, Subject, of, throwError } from 'rxjs';
import { vi } from 'vitest';
import NewProjectComponent from './new-project.component';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import { CLUSTER, PROJECT } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import type { ClusterService } from '../../generated/v1/cluster_pb';
import type { ProjectService } from '../../generated/v1/project_pb';

interface CreateResponse {
  projectId: string;
}

describe('NewProjectComponent submit', () => {
  function build(respond: () => Observable<CreateResponse>) {
    const createProject = vi.fn((_req: unknown, opts: CallOptions) => {
      opts.onHeader?.(new Headers({ 'Idempotency-Status': 'completed' }));
      return respond();
    });
    const goTo = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: PageNavService, useValue: { goTo, path: (p: string) => p } },
        { provide: NotificationService, useValue: { success: vi.fn(), error: vi.fn() } },
        {
          provide: OrganizationDataService,
          useValue: { reloadProjectsAndNamespaces: () => Promise.resolve() },
        },
        {
          provide: CLUSTER,
          useValue: {
            listClusters: () => of({ clusters: [{ id: 'c1', name: 'test', region: 'local' }] }),
          } as unknown as ObservableClient<typeof ClusterService>,
        },
        {
          provide: PROJECT,
          useValue: { createProject } as unknown as ObservableClient<typeof ProjectService>,
        },
      ],
    });
    const component = TestBed.createComponent(NewProjectComponent).componentInstance;
    return { component, createProject, goTo };
  }

  it('ignores a submit while the first create is in flight, and lets that create finish', async () => {
    const response = new Subject<CreateResponse>();
    const { component, createProject, goTo } = build(() => response);
    await component.ngOnInit();
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
    await component.ngOnInit();
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
    await component.ngOnInit();
    component.projectForm.patchValue({ name: 'demo' });

    await component.onSubmit();

    expect(component.errorMessage()).toBeNull();
    expect(component.getNameError()).toBe(
      'A project with this name already exists in this cluster.',
    );

    component.onNameInput(new CustomEvent('input', { detail: { value: 'demo-2' } }));
    expect(component.getNameError()).toBe('');
  });
});
