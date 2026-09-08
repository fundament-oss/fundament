import { TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { signal } from '@angular/core';
import { Router } from '@angular/router';
import { NEVER } from 'rxjs';
import { TitleService } from './title.service';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';

const PROJECT = { id: 'pr-burgerzaken', alias: 'burgerzaken', name: 'burgerzaken-prod' };

/** Only the handful of things the title reads, so the test needs no backend. */
function setUp(
  organizations: { id: string; name: string; alias: string }[],
  currentId: string,
  url = '/clusters',
) {
  TestBed.configureTestingModule({
    providers: [
      // A router that never navigates: the title is read once, from the address
      // the test starts at.
      { provide: Router, useValue: { events: NEVER, url } },
      {
        provide: OrganizationDataService,
        useValue: {
          organizations: signal(organizations),
          clusterSummaries: signal([{ id: 'cl-production', name: 'production' }]),
          getOrganizationById: (id: string) => organizations.find((org) => org.id === id),
          getProjectById: (id: string) => (id === PROJECT.id ? { project: PROJECT } : undefined),
          getClusterById: () => undefined,
        },
      },
      {
        provide: OrganizationContextService,
        useValue: { currentOrganizationId: signal(currentId) },
      },
    ],
  });
  return {
    service: TestBed.inject(TitleService),
    title: TestBed.inject(Title),
  };
}

const ONE = [{ id: 'org-1', name: 'gemeente-fundament', alias: 'fundament' }];
const TWO = [...ONE, { id: 'org-2', name: 'gemeente-delft', alias: 'delft' }];

describe('TitleService', () => {
  it('names the page and the product, with a middot between them', () => {
    const { service, title } = setUp(ONE, 'org-1');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).toBe('Namespaces · Fundament');
  });

  it('leaves the organization out while there is only one', () => {
    const { service, title } = setUp(ONE, 'org-1');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).not.toContain('fundament ·');
  });

  it('puts the organization in between as soon as there are more to tell apart', () => {
    const { service, title } = setUp(TWO, 'org-2');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).toBe('Namespaces · delft · Fundament');
  });

  it('puts the project between the page and the product', () => {
    const { service, title } = setUp(ONE, 'org-1', '/projects/pr-burgerzaken/limits');
    service.setTitle('Limits');
    TestBed.tick();
    expect(title.getTitle()).toBe('Limits · burgerzaken · Fundament');
  });

  it('reads the project from an address with the organization in it too', () => {
    const { service, title } = setUp(
      ONE,
      'org-1',
      '/organizations/gemeente-fundament/projects/pr-burgerzaken/limits',
    );
    service.setTitle('Limits');
    TestBed.tick();
    expect(title.getTitle()).toBe('Limits · burgerzaken · Fundament');
  });

  it('puts both the project and the organization in between when there are more organizations', () => {
    const { service, title } = setUp(TWO, 'org-2', '/projects/pr-burgerzaken/limits');
    service.setTitle('Limits');
    TestBed.tick();
    expect(title.getTitle()).toBe('Limits · burgerzaken · delft · Fundament');
  });

  it('names the project without a page as well, as on the project route itself', () => {
    const { service, title } = setUp(ONE, 'org-1', '/projects/pr-burgerzaken');
    service.setTitle();
    TestBed.tick();
    expect(title.getTitle()).toBe('burgerzaken · Fundament');
  });

  it('names the cluster and the list of clusters it hangs under', () => {
    const { service, title } = setUp(ONE, 'org-1', '/clusters/cl-production/nodes');
    service.setTitle('Node pools');
    TestBed.tick();
    expect(title.getTitle()).toBe('Node pools · production · Clusters · Fundament');
  });

  it('names the catalogue a plugin comes from', () => {
    const { service, title } = setUp(ONE, 'org-1', '/plugins/pl-cert-manager');
    service.setTitle('Cert Manager');
    TestBed.tick();
    expect(title.getTitle()).toBe('Cert Manager · Plugins · Fundament');
  });

  it('falls back to the console itself for a page with no name of its own', () => {
    const { service, title } = setUp(ONE, 'org-1');
    service.setTitle();
    TestBed.tick();
    expect(title.getTitle()).toBe('Fundament Console');
  });
});
