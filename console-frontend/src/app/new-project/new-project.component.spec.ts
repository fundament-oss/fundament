import { TestBed } from '@angular/core/testing';
import { Subject, of } from 'rxjs';
import { vi } from 'vitest';
import NewProjectComponent from './new-project.component';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import { CLUSTER, PROJECT } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import type { ClusterService } from '../../generated/v1/cluster_pb';
import type { ProjectService } from '../../generated/v1/project_pb';

describe('NewProjectComponent submit', () => {
  it('sends one create while the first is still in flight', async () => {
    const response = new Subject<{ projectId: string }>();
    const createProject = vi.fn(() => response);
    TestBed.configureTestingModule({
      providers: [
        { provide: PageNavService, useValue: { goTo: vi.fn(), path: (p: string) => p } },
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
    await component.ngOnInit();
    component.projectForm.patchValue({ name: 'demo' });

    const first = component.onSubmit();
    const second = component.onSubmit();
    response.complete();
    await Promise.allSettled([first, second]);

    expect(createProject).toHaveBeenCalledTimes(1);
  });
});
