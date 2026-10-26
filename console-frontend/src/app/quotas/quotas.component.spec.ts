import { TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import QuotasComponent, { quotaText } from './quotas.component';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { ORGANIZATION } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import {
  GetOrganizationQuotasResponseSchema,
  type OrganizationService,
} from '../../generated/v1/organization_pb';

const RESPONSE = create(GetOrganizationQuotasResponseSchema, {
  clusters: { quota: 3, inUse: 1 },
  nodes: [
    { region: 'local', machineType: 'local-medium', quota: 10, inUse: 6 },
    { region: 'local', machineType: 'local-small', quota: 2, inUse: 4 },
  ],
});

async function build(response = RESPONSE) {
  TestBed.configureTestingModule({
    providers: [
      { provide: TitleService, useValue: { setTitle: () => {} } },
      { provide: PageNavService, useValue: { backText: () => null } },
      {
        provide: ORGANIZATION,
        useValue: {
          getOrganizationQuotas: () => of(response),
        } as unknown as ObservableClient<typeof OrganizationService>,
      },
    ],
  });
  const fixture = TestBed.createComponent(QuotasComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture;
}

describe('quotaText', () => {
  it('reads the state off the numbers', () => {
    expect(quotaText({ label: 'Clusters', region: null, quota: 3, inUse: 1 })).toBe('1 of 3 clusters');
    expect(quotaText({ label: 'x', region: 'local', quota: 4, inUse: 4 })).toBe('4 of 4 nodes, all used');
    expect(quotaText({ label: 'x', region: 'local', quota: 2, inUse: 4 })).toBe('4 of 2 nodes, over the quota');
    expect(quotaText({ label: 'x', region: 'local', quota: 0, inUse: 0 })).toBe('0 in use, none allowed');
  });
});

/** The design system cells carry their words as attributes, not as text nodes. */
function cellTexts(fixture: Awaited<ReturnType<typeof build>>): string[] {
  return [...(fixture.nativeElement as HTMLElement).querySelectorAll('nldd-text-cell')].map(
    (cell) => `${cell.getAttribute('text')}|${cell.getAttribute('supporting-text') ?? ''}`,
  );
}

describe('QuotasComponent', () => {
  it('shows the cluster quota and one line per node quota, and says who to ask', async () => {
    const fixture = await build();
    const cells = cellTexts(fixture);

    expect(cells).toContain('Clusters|');
    expect(cells).toContain('1 of 3 clusters|');
    expect(cells).toContain('local-medium|local');
    expect(cells).toContain('6 of 10 nodes|');
    expect(cells).toContain('4 of 2 nodes, over the quota|');
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'contact a Fundament operator',
    );
  });

  it('warns when a quota sits below what runs', async () => {
    const fixture = await build();
    const banner = (fixture.nativeElement as HTMLElement).querySelector('nldd-banner[variant="warning"]');

    expect(banner).not.toBeNull();
    expect(fixture.componentInstance['anyOver']()).toBe(true);
  });

  it('says so too when the only node rows are pools from before quotas existed', async () => {
    const fixture = await build(
      create(GetOrganizationQuotasResponseSchema, {
        clusters: { quota: 1, inUse: 1 },
        nodes: [{ region: 'local', machineType: 'old-type', quota: 0, inUse: 2 }],
      }),
    );
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('no node pool can be added');
    expect(cellTexts(fixture)).toContain('2 in use, none allowed|');
  });

  it('says that no node pool can be made while no node quota is set', async () => {
    const fixture = await build(
      create(GetOrganizationQuotasResponseSchema, { clusters: { quota: 1, inUse: 0 }, nodes: [] }),
    );
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    // Both halves of the guidance: no pool can be added, and a cluster does not
    // start without one.
    expect(text).toContain('no node pool can be added');
    expect(text).toContain('a cluster starts only with its first node pool');
    expect(text).toContain('contact a Fundament operator');
    expect((fixture.nativeElement as HTMLElement).querySelector('nldd-banner[variant="warning"]')).toBeNull();
  });
});
