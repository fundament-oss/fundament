import { TestBed } from '@angular/core/testing';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import type ImageAnalysisComponent from './image-analysis.component';
import AdminReviewService, { type SubmissionHistoryEntry } from './admin-review.service';
import {
  type AnalysisInfo,
  type ImageAnalysisPage,
  type ImageAnalysisQuery,
} from './image-analysis';

const base: SubmissionHistoryEntry = {
  id: 's2',
  pluginVersionId: 'v1',
  version: '1.0.0',
  status: 'approved',
  submittedAt: '2026-10-02',
  submittedMs: Date.parse('2026-10-02'),
};

function info(status: AnalysisInfo['status'], overrides: Partial<AnalysisInfo> = {}): AnalysisInfo {
  return {
    pluginVersionId: 'v2',
    status,
    statusInfo: '',
    analyzed: status === 'completed' ? '2026-11-24' : '',
    imageDigest: 'sha256:abc',
    platform: 'linux/amd64',
    platforms: [],
    filesTruncated: false,
    ...overrides,
  };
}

const comparedPage: ImageAnalysisPage = {
  analysis: info('completed'),
  base: info('completed', { pluginVersionId: 'v1' }),
  compared: true,
  packages: [
    {
      name: 'openssl',
      version: '3.0.14',
      type: 'deb',
      purl: 'pkg:deb/debian/openssl@3.0.14',
      licenses: ['Apache-2.0'],
      change: 'changed',
      baseVersion: '3.0.13',
    },
    {
      name: 'zlib',
      version: '1.3',
      type: 'deb',
      purl: 'pkg:deb/debian/zlib@1.3',
      licenses: [],
      change: 'unchanged',
      baseVersion: '',
    },
  ],
  files: [{ path: '/etc/config', size: 2048, mode: 0o644, link: '', change: 'changed' }],
  nextFilesPageToken: '1',
  packageSummary: { unchanged: 1, added: 0, removed: 0, changed: 1 },
  fileSummary: { unchanged: 10, added: 2, removed: 1, changed: 1 },
  filesMatching: 2,
};

// Imported in beforeAll: the checkbox field calls ElementInternals' form API,
// which jsdom lacks, as it loads.
let AnalysisComponent: typeof ImageAnalysisComponent;

async function settle() {
  await new Promise((resolve) => {
    setTimeout(resolve);
  });
}

async function render(
  getImageAnalysis: (query: ImageAnalysisQuery) => Promise<ImageAnalysisPage>,
  baseVersion: SubmissionHistoryEntry | null = base,
) {
  TestBed.configureTestingModule({
    providers: [{ provide: AdminReviewService, useValue: { getImageAnalysis } }],
  });
  const fixture = TestBed.createComponent(AnalysisComponent);
  fixture.componentRef.setInput('pluginVersionId', 'v2');
  fixture.componentRef.setInput('base', baseVersion);
  fixture.detectChanges();
  await settle();
  fixture.detectChanges();
  return { fixture, root: fixture.nativeElement as HTMLElement };
}

function rows(root: HTMLElement, table: string): string[] {
  return Array.from(root.querySelectorAll(`[data-testid="${table}"] tbody tr`)).map((row) =>
    Array.from(row.querySelectorAll('td'))
      .map((cell) => cell.textContent?.replace(/\s+/g, ' ').trim() ?? '')
      .filter((text) => text !== '')
      .join(' | '),
  );
}

describe('ImageAnalysisComponent', () => {
  beforeAll(async () => {
    ['setFormValue', 'setValidity'].forEach((method) => {
      if (!(method in ElementInternals.prototype)) {
        Object.defineProperty(ElementInternals.prototype, method, { value: () => {} });
      }
    });
    AnalysisComponent = (await import('./image-analysis.component')).default;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows the changes against the base, changed packages only by default', async () => {
    const { root } = await render(async () => comparedPage);

    expect(root.querySelector('[data-testid="comparison-note"]')?.textContent?.trim()).toBe(
      'Compared with 1.0.0.',
    );
    expect(root.querySelector('[data-testid="package-summary"]')?.textContent?.trim()).toBe(
      '1 changed',
    );
    expect(root.querySelector('[data-testid="file-summary"]')?.textContent?.trim()).toBe(
      '2 added, 1 changed, 1 removed',
    );
    expect(rows(root, 'packages')).toEqual(['openssl | 3.0.13 → 3.0.14 | deb | Apache-2.0']);
    expect(rows(root, 'files')).toEqual(['/etc/config | 2.0 KiB | 0644']);
  });

  it('names the platforms that were not analyzed', async () => {
    const { root } = await render(async () => ({
      ...comparedPage,
      analysis: info('completed', { platforms: ['linux/amd64', 'linux/arm64'] }),
    }));

    expect(root.textContent).toContain(
      'The image also offers linux/arm64, which was not analyzed.',
    );
  });

  it('lists unchanged packages too once changes only is off', async () => {
    const { fixture, root } = await render(async () => comparedPage);

    root
      .querySelector('nldd-checkbox-field')
      ?.dispatchEvent(new CustomEvent('change', { detail: { checked: false } }));
    fixture.detectChanges();

    expect(rows(root, 'packages')).toHaveLength(2);
  });

  it('asks the server for the next page of files and appends it', async () => {
    const queries: ImageAnalysisQuery[] = [];
    const { fixture, root } = await render(async (query) => {
      queries.push(query);
      return query.filesPageToken
        ? {
            ...comparedPage,
            files: [{ path: '/etc/new', size: 1, mode: 0o644, link: '', change: 'added' }],
            nextFilesPageToken: '',
          }
        : comparedPage;
    });

    root.querySelector('nldd-button[text="Load more"]')?.dispatchEvent(new Event('click'));
    await settle();
    fixture.detectChanges();

    expect(queries[1]).toMatchObject({ basePluginVersionId: 'v1', filesPageToken: '1' });
    expect(rows(root, 'files')).toHaveLength(2);
    expect(root.querySelector('nldd-button[text="Load more"]')).toBeNull();
  });

  it('filters files on the server after a pause in typing', async () => {
    const queries: ImageAnalysisQuery[] = [];
    const { root } = await render(async (query) => {
      queries.push(query);
      return comparedPage;
    });
    vi.useFakeTimers();

    const search = root.querySelector('nldd-search-field[accessible-label="Search files by path"]');
    Object.defineProperty(search, 'value', { value: 'etc', configurable: true });
    search?.dispatchEvent(new Event('input'));
    await vi.advanceTimersByTimeAsync(300);

    expect(queries.at(-1)).toMatchObject({ filesQuery: 'etc', filesChangesOnly: true });
  });

  it('shows the full lists, without changes, when no earlier version was approved', async () => {
    const { root } = await render(
      async () => ({ ...comparedPage, base: undefined, compared: false }),
      null,
    );

    expect(root.querySelector('[data-testid="comparison-note"]')?.textContent?.trim()).toBe(
      'No earlier version of this plugin was approved, so the full lists are shown.',
    );
    expect(root.querySelector('[data-testid="package-summary"]')).toBeNull();
    expect(root.querySelectorAll('nldd-tag')).toHaveLength(0);
  });

  it('polls while the analysis is running', async () => {
    vi.useFakeTimers();
    let calls = 0;
    TestBed.configureTestingModule({
      providers: [
        {
          provide: AdminReviewService,
          useValue: {
            getImageAnalysis: async () => {
              calls += 1;
              return calls === 1 ? { ...comparedPage, analysis: info('running') } : comparedPage;
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(AnalysisComponent);
    fixture.componentRef.setInput('pluginVersionId', 'v2');
    fixture.componentRef.setInput('base', base);
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(0);
    fixture.detectChanges();
    const root = fixture.nativeElement as HTMLElement;

    expect(root.querySelector('[data-testid="analysis-state"]')?.textContent).toContain(
      'Analyzing the image.',
    );

    await vi.advanceTimersByTimeAsync(10_000);
    fixture.detectChanges();

    expect(calls).toBe(2);
    expect(rows(root, 'packages')).toHaveLength(1);
  });

  it('says why the image could not be analyzed', async () => {
    const { root } = await render(async () => ({
      ...comparedPage,
      analysis: info('failed', {
        statusInfo: 'Image is private or does not exist. Only public images can be analyzed.',
      }),
    }));

    const state = root.querySelector('[data-testid="analysis-state"]')?.textContent;
    expect(state).toContain('The image could not be analyzed.');
    expect(state).toContain('Image is private or does not exist.');
  });
});
