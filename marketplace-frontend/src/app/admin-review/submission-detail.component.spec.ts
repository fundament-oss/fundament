import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, provideRouter } from '@angular/router';
import { describe, expect, it } from 'vitest';
import SubmissionDetailComponent from './submission-detail.component';
import AdminReviewService, { type PluginSubmission } from './admin-review.service';

const LOGO = 'data:image/svg+xml;base64,AA';
const LOGO_DARK = 'data:image/svg+xml;base64,BB';
const MENU_ICONS = ['data:image/png;base64,CC', 'data:image/svg+xml;base64,DD'];

const submission: PluginSubmission = {
  id: 's1',
  pluginId: 'p1',
  title: 'Grafana',
  description: '',
  categories: [],
  icon: 'grafana',
  logo: LOGO,
  logoDark: LOGO_DARK,
  menuIcons: MENU_ICONS,
  version: '1.0.0',
  submitterName: 'Acme',
  submitterUserId: 'u1',
  submittedAt: '2026-11-04',
  status: 'pending',
};

async function render(value: PluginSubmission): Promise<HTMLElement> {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: ActivatedRoute, useValue: { snapshot: { paramMap: { get: () => 's1' } } } },
      { provide: AdminReviewService, useValue: { getSubmission: async () => value } },
    ],
  });
  const fixture = TestBed.createComponent(SubmissionDetailComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

function logoPreviews(root: HTMLElement): HTMLImageElement[] {
  return Array.from(root.querySelectorAll<HTMLImageElement>('figure img'));
}

describe('SubmissionDetailComponent', () => {
  it('renders the logo, dark logo and each menu icon of the version as images', async () => {
    const root = await render(submission);

    const sources = Array.from(root.querySelectorAll('img')).map((img) => img.getAttribute('src'));
    expect(sources).toEqual(expect.arrayContaining([LOGO, LOGO_DARK, ...MENU_ICONS]));
  });

  it('shows both logo variants whatever the reviewer theme is', async () => {
    const root = await render(submission);

    const previews = logoPreviews(root);
    expect(previews.map((img) => img.getAttribute('src'))).toEqual([LOGO, LOGO_DARK]);
    previews.forEach((img) => {
      expect(img.className).not.toMatch(/(^|\s)(dark:)?hidden(\s|$)/);
    });
  });

  it('previews the light logo in dark mode when there is no dark logo', async () => {
    const root = await render({ ...submission, logoDark: '' });

    const previews = logoPreviews(root);
    expect(previews.map((img) => img.getAttribute('src'))).toEqual([LOGO, LOGO]);
    expect(previews[1].className).toContain('bg-gray-100');
    expect(root.textContent).toContain('No dark logo');
  });

  it('shows no logo previews when the version has no logo', async () => {
    const root = await render({ ...submission, logo: '', logoDark: '' });

    expect(logoPreviews(root)).toEqual([]);
  });
});
