import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { describe, expect, it } from 'vitest';
import PageNavService from './page-nav.service';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';

function navAt(url: string, clusterName?: string): PageNavService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      { provide: Router, useValue: { url, events: { pipe: () => ({ subscribe: () => {} }) } } },
      {
        provide: OrganizationDataService,
        useValue: {
          getClusterById: () => (clusterName ? { cluster: { name: clusterName } } : undefined),
          getProjectById: () => ({ project: { name: 'project-one', alias: 'Project one' } }),
          getOrganizationById: () => undefined,
        },
      },
      { provide: OrganizationContextService, useValue: { currentOrganizationId: () => null } },
    ],
  });
  return TestBed.inject(PageNavService);
}

describe('PageNavService.backText', () => {
  it('names the cluster on a cluster plugin route', () => {
    const nav = navAt('/clusters/c1/plugin-resources/p/databases', 'prod');
    expect(nav.backText()).toBe('prod');
  });

  it('falls back to "Cluster" when the cluster is unknown', () => {
    const nav = navAt('/clusters/c1/plugin-resources/p/databases');
    expect(nav.backText()).toBe('Cluster');
  });

  it('names the project on a project plugin route', () => {
    const nav = navAt('/projects/pr1/plugin-resources/p/databases', 'prod');
    expect(nav.backText()).toBe('Project one');
  });
});
