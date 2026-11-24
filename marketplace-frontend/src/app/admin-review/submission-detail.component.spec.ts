import { TestBed } from '@angular/core/testing';
import {
  ActivatedRoute,
  Router,
  convertToParamMap,
  provideRouter,
  type Params,
} from '@angular/router';
import { of } from 'rxjs';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import type SubmissionDetailComponent from './submission-detail.component';
import AdminReviewService, {
  type PluginSubmission,
  type SubmissionHistoryEntry,
} from './admin-review.service';
import ReviewQueueFiltersService from './review-queue-filters.service';

const LOGO = 'data:image/svg+xml;base64,AA';
const LOGO_DARK = 'data:image/svg+xml;base64,BB';
const MENU_ICONS = ['data:image/png;base64,CC', 'data:image/svg+xml;base64,DD'];

const submission: PluginSubmission = {
  id: 's1',
  pluginId: 'p1',
  pluginVersionId: 'v3',
  title: 'Grafana',
  descriptionShort: '',
  description: '',
  categories: [],
  logo: LOGO,
  logoDark: LOGO_DARK,
  menuIcons: MENU_ICONS,
  version: '1.0.0',
  submitterName: 'Acme',
  submitterUserId: 'u1',
  submittedAt: '2026-11-04',
  status: 'pending',
};

// Newest first, as AdminReviewService.listPluginHistory returns it: s1 is the
// round under review, s2 and s3 two rounds of the version before it.
const history: SubmissionHistoryEntry[] = [
  {
    id: 's1',
    pluginVersionId: 'v3',
    version: '1.0.0',
    status: 'pending',
    submittedAt: '2026-11-04',
    submittedMs: Date.parse('2026-11-04'),
  },
  {
    id: 's2',
    pluginVersionId: 'v2',
    version: '0.9.0',
    status: 'approved',
    submittedAt: '2026-10-02',
    submittedMs: Date.parse('2026-10-02'),
    reviewedAt: '2026-10-03',
  },
  {
    id: 's3',
    pluginVersionId: 'v2',
    version: '0.9.0',
    status: 'changes_requested',
    submittedAt: '2026-09-28',
    submittedMs: Date.parse('2026-09-28'),
    reviewedAt: '2026-09-29',
  },
];

// Imported in beforeAll because the design system's tooltip, which the
// segmented control pulls in, calls matchMedia as it loads, and jsdom has none.
// jsdom's ElementInternals has no form API either, which the segmented control
// calls on every update, and no ResizeObserver, which a list item measures with.
let DetailComponent: typeof SubmissionDetailComponent;

async function render(
  value: PluginSubmission,
  rounds: SubmissionHistoryEntry[] = history,
  queueParams: Params = {},
  manifests: Record<string, string> = {},
): Promise<HTMLElement> {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: ActivatedRoute, useValue: { paramMap: of(convertToParamMap({ id: 's1' })) } },
      {
        provide: AdminReviewService,
        useValue: {
          getSubmission: async () => value,
          listPluginHistory: async () => rounds,
          getVersionManifest: async (pluginVersionId: string) => manifests[pluginVersionId] ?? '',
          // The image analysis has its own spec; here it only has to not fail.
          getImageAnalysis: () => new Promise(() => {}),
          approve: async () => {},
        },
      },
    ],
  });
  TestBed.inject(ReviewQueueFiltersService).queryParams.set(queueParams);
  const fixture = TestBed.createComponent(DetailComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  // The history is fetched once the submission is in, and the base manifest
  // once the history is: let all three settle.
  await new Promise((resolve) => {
    setTimeout(resolve);
  });
  await new Promise((resolve) => {
    setTimeout(resolve);
  });
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

function logoPreviews(root: HTMLElement): HTMLImageElement[] {
  return Array.from(root.querySelectorAll<HTMLImageElement>('figure img'));
}

// Called with `new`, which an arrow function cannot be.
function ResizeObserverStub() {
  return { observe: () => {}, unobserve: () => {}, disconnect: () => {} };
}

describe('SubmissionDetailComponent', () => {
  beforeAll(async () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    vi.stubGlobal('ResizeObserver', ResizeObserverStub);
    ['setFormValue', 'setValidity'].forEach((method) => {
      if (!(method in ElementInternals.prototype)) {
        Object.defineProperty(ElementInternals.prototype, method, { value: () => {} });
      }
    });
    DetailComponent = (await import('./submission-detail.component')).default;
  });

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

  it('renders the description as markdown', async () => {
    const root = await render({ ...submission, description: '## Setup\n\nRun **helm**.' });

    expect(root.querySelector('app-markdown h3')?.textContent).toBe('Setup');
    expect(root.querySelector('app-markdown strong')?.textContent).toBe('helm');
  });

  it('shows the short description, not the markdown, in the metadata', async () => {
    const root = await render({
      ...submission,
      descriptionShort: 'Dashboards for your clusters',
      description: '## Setup',
    });

    const metadata = Array.from(root.querySelectorAll('h2')).find(
      (heading) => heading.textContent?.trim() === 'Submitted metadata',
    )?.parentElement;
    expect(metadata?.textContent).toContain('Dashboards for your clusters');
    expect(metadata?.textContent).not.toContain('Setup');
  });

  it('switches the description between the rendered preview and its markdown', async () => {
    const root = await render({ ...submission, description: '## Setup' });
    const control = root.querySelector('nldd-segmented-control');

    control?.dispatchEvent(new CustomEvent('change', { detail: { value: 'source' } }));
    await new Promise((resolve) => {
      setTimeout(resolve);
    });

    expect(root.querySelector('pre')?.textContent).toBe('## Setup');
    expect(root.querySelector('app-markdown')).toBeNull();
  });

  it('goes back to the queue with the filters the reviewer left it with', async () => {
    const root = await render(submission, history, { status: 'pending', q: 'acme' });

    expect(root.querySelector('a[href^="/admin"]')?.getAttribute('href')).toBe(
      '/admin?status=pending&q=acme',
    );
  });

  it('returns to the filtered queue after a decision', async () => {
    const root = await render(submission, history, { status: 'pending' });
    const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);

    root.querySelector('nldd-button[text="Approve"]')?.dispatchEvent(new Event('click'));
    await new Promise((resolve) => {
      setTimeout(resolve);
    });

    expect(navigate).toHaveBeenCalledWith(['/admin'], { queryParams: { status: 'pending' } });
  });

  it('lists every review round, linking to all but the one on screen', async () => {
    const root = await render(submission);

    const rows = Array.from(root.querySelectorAll('nldd-list-item'));
    expect(rows).toHaveLength(3);
    expect(rows[0].hasAttribute('current')).toBe(true);
    expect(rows[0].getAttribute('href')).toBeNull();
    expect(rows[1].getAttribute('href')).toBe('/admin/submissions/s2');
    expect(rows[2].getAttribute('href')).toBe('/admin/submissions/s3');
  });

  it('draws an earlier round of the same version as a minor step', async () => {
    const root = await render(submission);

    // Read as properties: the step cell drops an attribute set to its default.
    const steps = Array.from(root.querySelectorAll('nldd-step-cell')) as unknown as {
      level: string;
      position: string;
      status: string;
    }[];
    expect(steps.map((step) => step.level)).toEqual(['major', 'major', 'minor']);
    expect(steps.map((step) => step.position)).toEqual(['first', 'between', 'last']);
    expect(steps.map((step) => step.status)).toEqual(['current', 'past', 'past']);
  });

  describe('manifest', () => {
    const OLD = 'spec:\n  image: example@sha256:aaa\n';
    const NEW = 'spec:\n  image: example@sha256:bbb\n';

    function viewer(root: HTMLElement) {
      return root.querySelector('nldd-code-viewer');
    }

    function note(root: HTMLElement) {
      return root.querySelector('[data-testid="manifest-note"]')?.textContent?.trim();
    }

    it('shows the changes against the version approved before it', async () => {
      const root = await render({ ...submission, manifest: NEW }, history, {}, { v2: OLD });

      expect(note(root)).toBe(
        'Compared with 0.9.0, the most recent approved version before this one.',
      );
      expect(viewer(root)?.hasAttribute('diff')).toBe(true);
      expect(viewer(root)?.textContent).toContain('-  image: example@sha256:aaa');
      expect(viewer(root)?.textContent).toContain('+  image: example@sha256:bbb');
    });

    it('switches to the full manifest', async () => {
      const root = await render({ ...submission, manifest: NEW }, history, {}, { v2: OLD });
      const control = Array.from(root.querySelectorAll('nldd-segmented-control')).find(
        (element) => element.getAttribute('accessible-label') === 'Show the manifest as',
      );

      control?.dispatchEvent(new CustomEvent('change', { detail: { value: 'full' } }));
      await new Promise((resolve) => {
        setTimeout(resolve);
      });

      expect(viewer(root)?.hasAttribute('diff')).toBe(false);
      expect(viewer(root)?.textContent).toBe(NEW);
    });

    it('shows the full manifest when no earlier version was approved', async () => {
      const firstOnly = history.filter((entry) => entry.pluginVersionId === 'v3');
      const root = await render({ ...submission, manifest: NEW }, firstOnly);

      expect(note(root)).toBe(
        'No earlier version of this plugin was approved, so the full manifest is shown.',
      );
      expect(viewer(root)?.hasAttribute('diff')).toBe(false);
      expect(viewer(root)?.textContent).toBe(NEW);
    });

    it('says so when the manifest did not change', async () => {
      const root = await render({ ...submission, manifest: OLD }, history, {}, { v2: OLD });

      expect(note(root)).toBe('Identical to the manifest of 0.9.0.');
      expect(viewer(root)?.hasAttribute('diff')).toBe(false);
    });
  });
});
