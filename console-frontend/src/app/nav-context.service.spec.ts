import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { describe, expect, it } from 'vitest';
import NavContextService from './nav-context.service';
import { OrganizationDataService } from './organization-data.service';

const ORG = '/organizations/acme';

function contextAt(
  url: string,
  {
    projectsLoaded = true,
    projectsSettled = projectsLoaded,
  }: { projectsLoaded?: boolean; projectsSettled?: boolean } = {},
): NavContextService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      { provide: Router, useValue: { url, events: { pipe: () => ({ subscribe: () => {} }) } } },
      {
        provide: OrganizationDataService,
        useValue: {
          currentOrganizationDisplayName: () => 'Acme',
          getClusterById: (id: string) =>
            id === 'c1' ? { cluster: { id: 'c1', name: 'production' } } : undefined,
          getProjectById: (id: string) =>
            projectsLoaded && id === 'p1'
              ? {
                  project: { name: 'burgerzaken', alias: 'Burgerzaken' },
                  cluster: { id: 'c1', name: 'production' },
                }
              : undefined,
          clusterSummaries: () => [{ id: 'c2', name: 'staging' }],
          projectsSettled: () => projectsSettled,
        },
      },
    ],
  });
  return TestBed.inject(NavContextService);
}

describe('NavContextService', () => {
  it('names the organization and has nothing above it', () => {
    const nav = contextAt(`${ORG}/members`);
    expect(nav.current()).toEqual({ kind: 'organization', name: 'Acme', path: '/' });
    expect(nav.parent()).toBeNull();
    expect(nav.clusterId()).toBeNull();
  });

  it('returns from a cluster to the organization’s clusters', () => {
    const nav = contextAt(`${ORG}/clusters/c1/general`);
    expect(nav.current()).toEqual({ kind: 'cluster', name: 'production', path: '/clusters/c1' });
    expect(nav.parent()).toEqual({ kind: 'organization', name: 'Acme', path: '/clusters' });
    expect(nav.clusterId()).toBe('c1');
  });

  it('names a cluster from the cluster list before its projects are in', () => {
    expect(contextAt(`${ORG}/clusters/c2`).current().name).toBe('staging');
  });

  it('returns from a project to its cluster’s projects', () => {
    const nav = contextAt(`${ORG}/projects/p1/namespaces`);
    expect(nav.current()).toEqual({ kind: 'project', name: 'Burgerzaken', path: '/projects/p1' });
    expect(nav.parent()).toEqual({
      kind: 'cluster',
      name: 'production',
      path: '/clusters/c1/projects',
    });
    expect(nav.clusterId()).toBe('c1');
  });

  it('has no way up from a project until its cluster is known', () => {
    const nav = contextAt(`${ORG}/projects/p1`, { projectsLoaded: false });
    expect(nav.current().name).toBe('Project');
    expect(nav.parent()).toBeNull();
  });

  it('leads up to the organization when a project’s cluster stays unknown', () => {
    const nav = contextAt(`${ORG}/projects/p9`, { projectsSettled: true });
    expect(nav.parent()).toEqual({ kind: 'organization', name: 'Acme', path: '/clusters' });
  });
});
