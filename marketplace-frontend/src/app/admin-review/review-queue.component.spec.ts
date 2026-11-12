import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { describe, expect, it } from 'vitest';
import ReviewQueueComponent from './review-queue.component';
import AdminReviewService, { type PluginSubmission } from './admin-review.service';
import { ConfigService } from '../config.service';

function submission(
  id: string,
  title: string,
  submitterName: string,
  status: PluginSubmission['status'],
): PluginSubmission {
  return {
    id,
    pluginId: `plugin-${id}`,
    pluginVersionId: `version-${id}`,
    title,
    descriptionShort: `${title} in one line`,
    description: `## ${title}`,
    categories: [],
    logo: '',
    logoDark: '',
    menuIcons: [],
    submitterName,
    submitterUserId: 'u1',
    submittedAt: '2026-11-04',
    status,
  };
}

const queue = [
  submission('s1', 'Grafana', 'Acme', 'pending'),
  submission('s2', 'Loki', 'Acme', 'approved'),
  submission('s3', 'Tempo', 'Globex', 'pending'),
];

async function render(url: string) {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([{ path: 'admin', component: ReviewQueueComponent }]),
      { provide: AdminReviewService, useValue: { listSubmissions: async () => queue } },
      { provide: ConfigService, useValue: { getConfig: () => ({}) } },
    ],
  });
  const harness = await RouterTestingHarness.create();
  await harness.navigateByUrl(url);
  await harness.fixture.whenStable();
  harness.detectChanges();
  return harness;
}

function titles(root: HTMLElement): string[] {
  return Array.from(root.querySelectorAll('tbody tr p.text-lg')).map(
    (title) => title.textContent?.trim() ?? '',
  );
}

describe('ReviewQueueComponent', () => {
  it('shows every submission without filters', async () => {
    const harness = await render('/admin');

    expect(titles(harness.routeNativeElement as HTMLElement)).toEqual(['Grafana', 'Tempo', 'Loki']);
  });

  it('shows only the status in the address', async () => {
    const harness = await render('/admin?status=approved');

    expect(titles(harness.routeNativeElement as HTMLElement)).toEqual(['Loki']);
  });

  it('matches the search on plugin and publisher', async () => {
    const byPlugin = await render('/admin?q=temp');
    expect(titles(byPlugin.routeNativeElement as HTMLElement)).toEqual(['Tempo']);

    await byPlugin.navigateByUrl('/admin?q=acme');
    byPlugin.detectChanges();
    expect(titles(byPlugin.routeNativeElement as HTMLElement)).toEqual(['Grafana', 'Loki']);
  });

  it('combines the status and the search', async () => {
    const harness = await render('/admin?status=pending&q=acme');

    expect(titles(harness.routeNativeElement as HTMLElement)).toEqual(['Grafana']);
  });

  it('ignores a status the queue cannot filter on', async () => {
    const harness = await render('/admin?status=draft');

    expect(titles(harness.routeNativeElement as HTMLElement)).toHaveLength(3);
  });

  it('filters on a card and clears it on a second click', async () => {
    const harness = await render('/admin');
    const router = TestBed.inject(Router);
    const card = () =>
      (harness.routeNativeElement as HTMLElement).querySelectorAll<HTMLElement>(
        'nldd-card[button]',
      )[2];

    card().click();
    await harness.fixture.whenStable();
    expect(router.url).toBe('/admin?status=approved');

    card().click();
    await harness.fixture.whenStable();
    expect(router.url).toBe('/admin');
  });

  it('shows the short description, not the markdown', async () => {
    const harness = await render('/admin');
    const root = harness.routeNativeElement as HTMLElement;

    expect(root.textContent).toContain('Grafana in one line');
    expect(root.textContent).not.toContain('## Grafana');
  });
});
