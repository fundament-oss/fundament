import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import PlatformOverviewComponent, { matchesQuery, tilesFor } from './platform-overview.component';
import { PLATFORM_OVERVIEW } from '../../connect/tokens';
import { TitleService } from '../title.service';
import type { ObservableClient } from '../../connect/observable-client';
import {
  GetPlatformOverviewResponseSchema,
  OrganizationOverviewSchema,
  type OverviewService,
} from '../../generated/platform/v1/overview_pb';

const utrecht = create(OrganizationOverviewSchema, {
  name: 'gemeente-utrecht',
  alias: 'Gemeente Utrecht',
  clusters: 3,
  projects: 12,
  namespaces: 40,
});

const zwolle = create(OrganizationOverviewSchema, {
  name: 'gemeente-zwolle',
  alias: 'Gemeente Zwolle',
  clusters: 1,
  projects: 4,
  namespaces: 9,
});

const response = create(GetPlatformOverviewResponseSchema, {
  organizations: [utrecht, zwolle],
  totals: { organizations: 2, clusters: 4, projects: 16, namespaces: 49 },
  generatedAt: timestampFromDate(new Date('2026-10-14T14:02:00Z')),
});

async function render(client: Partial<ObservableClient<typeof OverviewService>>) {
  TestBed.configureTestingModule({
    imports: [PlatformOverviewComponent],
    providers: [
      { provide: PLATFORM_OVERVIEW, useValue: client },
      // The real one follows the organization, which this page has none of.
      { provide: TitleService, useValue: { setTitle: () => {} } },
    ],
  });
  const fixture = TestBed.createComponent(PlatformOverviewComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture;
}

describe('PlatformOverviewComponent', () => {
  it('renders the totals and a row per organization', async () => {
    const fixture = await render({ getPlatformOverview: () => of(response) });
    const element: HTMLElement = fixture.nativeElement;

    const tiles = [...element.querySelectorAll('nldd-card')].map(
      (card) => `${card.querySelector('h2')?.textContent}: ${card.querySelector('p')?.textContent}`,
    );
    expect(tiles).toEqual(['Organizations: 2', 'Clusters: 4', 'Projects: 16', 'Namespaces: 49']);

    const rows = element.querySelectorAll('nldd-table-row:not([slot="header"])');
    expect(rows).toHaveLength(2);
    const firstRow = [...rows[0].querySelectorAll('nldd-text-cell')].map((cell) =>
      cell.getAttribute('text'),
    );
    expect(firstRow).toEqual(['Gemeente Utrecht', '3', '12', '40']);
    expect(element.textContent).toContain('As of');
  });

  it('filters the table by name or alias', async () => {
    const fixture = await render({ getPlatformOverview: () => of(response) });

    fixture.componentInstance.query.set('zwolle');
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('nldd-table-row:not([slot="header"])');
    expect(rows).toHaveLength(1);
  });

  it('offers a way to sign in', async () => {
    const fixture = await render({ getPlatformOverview: () => of(response) });
    const signIn = fixture.nativeElement.querySelector('nldd-button[href="/login"]');
    expect(signIn?.getAttribute('text')).toBe('Sign in');
  });

  it('says so when the overview cannot be loaded', async () => {
    const fixture = await render({
      getPlatformOverview: () => throwError(() => new Error('unavailable')),
    });
    const banner = fixture.nativeElement.querySelector('nldd-banner');
    expect(banner?.getAttribute('text')).toBe('Overview not loaded');
    expect(fixture.nativeElement.querySelector('nldd-table')).toBeNull();
  });
});

describe('matchesQuery', () => {
  it('matches every organization on an empty query', () => {
    expect(matchesQuery(utrecht, '  ')).toBe(true);
  });

  it('matches name and alias case-insensitively', () => {
    expect(matchesQuery(utrecht, 'UTRECHT')).toBe(true);
    expect(matchesQuery(utrecht, 'gemeente-u')).toBe(true);
    expect(matchesQuery(utrecht, 'zwolle')).toBe(false);
  });
});

describe('tilesFor', () => {
  it('shows zeros before the totals are in', () => {
    expect(tilesFor(undefined).map((tile) => tile.value)).toEqual([0, 0, 0, 0]);
  });
});
