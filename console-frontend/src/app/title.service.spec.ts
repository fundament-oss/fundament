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
function opzetten(
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

const EEN = [{ id: 'org-1', name: 'Gemeente Fundament', alias: 'fundament' }];
const TWEE = [...EEN, { id: 'org-2', name: 'Gemeente Delft', alias: 'delft' }];

describe('TitleService', () => {
  it('noemt de pagina en het product, gescheiden door een middot', () => {
    const { service, title } = opzetten(EEN, 'org-1');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).toBe('Namespaces · Fundament');
  });

  it('laat de organisatie weg als er maar één is', () => {
    const { service, title } = opzetten(EEN, 'org-1');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).not.toContain('fundament ·');
  });

  it('zet de organisatie ertussen zodra er meer zijn om uit elkaar te houden', () => {
    const { service, title } = opzetten(TWEE, 'org-2');
    service.setTitle('Namespaces');
    TestBed.tick();
    expect(title.getTitle()).toBe('Namespaces · delft · Fundament');
  });

  it('zet het project tussen de pagina en het product', () => {
    const { service, title } = opzetten(EEN, 'org-1', '/projects/pr-burgerzaken/limits');
    service.setTitle('Limits');
    TestBed.tick();
    expect(title.getTitle()).toBe('Limits · burgerzaken · Fundament');
  });

  it('zet het project én de organisatie ertussen als er meer organisaties zijn', () => {
    const { service, title } = opzetten(TWEE, 'org-2', '/projects/pr-burgerzaken/limits');
    service.setTitle('Limits');
    TestBed.tick();
    expect(title.getTitle()).toBe('Limits · burgerzaken · delft · Fundament');
  });

  it('noemt het project ook zonder pagina, zoals op de projectroute zelf', () => {
    const { service, title } = opzetten(EEN, 'org-1', '/projects/pr-burgerzaken');
    service.setTitle();
    TestBed.tick();
    expect(title.getTitle()).toBe('burgerzaken · Fundament');
  });

  it('noemt het cluster en de clusterlijst waar het onder hangt', () => {
    const { service, title } = opzetten(EEN, 'org-1', '/clusters/cl-production/nodes');
    service.setTitle('Node pools');
    TestBed.tick();
    expect(title.getTitle()).toBe('Node pools · production · Clusters · Fundament');
  });

  it('noemt de catalogus waar een plugin uit komt', () => {
    const { service, title } = opzetten(EEN, 'org-1', '/plugins/pl-cert-manager');
    service.setTitle('Cert Manager');
    TestBed.tick();
    expect(title.getTitle()).toBe('Cert Manager · Plugins · Fundament');
  });

  it('valt terug op de console zelf voor een pagina zonder eigen naam', () => {
    const { service, title } = opzetten(EEN, 'org-1');
    service.setTitle();
    TestBed.tick();
    expect(title.getTitle()).toBe('Fundament Console');
  });
});
