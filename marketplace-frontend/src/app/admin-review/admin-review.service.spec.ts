import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { describe, expect, it } from 'vitest';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { REVIEW_CLIENT } from '../../connect/admin';
import AdminReviewService from './admin-review.service';

function round(id: string, pluginVersionId: string, submitted: string) {
  return {
    id,
    pluginId: 'p1',
    pluginVersionId,
    submitted: timestampFromDate(new Date(submitted)),
  };
}

describe('AdminReviewService.listPluginHistory', () => {
  it('keeps every round when one version fails to load, without its label', async () => {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: REVIEW_CLIENT,
          useValue: {
            listSubmissions: () =>
              of({
                submissions: [
                  round('s1', 'v2', '2026-11-04'),
                  round('s2', 'v1', '2026-10-02'),
                  { ...round('s3', 'other', '2026-10-01'), pluginId: 'p2' },
                ],
              }),
            getPluginVersion: ({ pluginVersionId }: { pluginVersionId: string }) =>
              pluginVersionId === 'v1'
                ? throwError(() => new Error('not found'))
                : of({ version: { version: '2.0.0' } }),
          },
        },
      ],
    });

    const history = await TestBed.inject(AdminReviewService).listPluginHistory('p1');

    expect(history.map(({ id, version }) => ({ id, version }))).toEqual([
      { id: 's1', version: '2.0.0' },
      { id: 's2', version: '' },
    ]);
  });
});

describe('AdminReviewService.getSubmission', () => {
  it('shows the listing as approving the submitted edits leaves it', async () => {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: REVIEW_CLIENT,
          useValue: {
            getSubmission: () =>
              of({
                submission: {
                  ...round('s1', 'v1', '2026-11-04'),
                  organizationId: 'o1',
                  listingChanges: {
                    description: '## Rewritten',
                    categoryIds: ['c1'],
                    authorName: 'Jetstack',
                    authorUrl: '',
                    image: '',
                    logo: '',
                    logoDark: '',
                    features: [{ id: '', title: 'Issuers', body: 'Many.' }],
                    documentationLinks: [],
                  },
                },
              }),
            getPlugin: () =>
              of({
                plugin: { displayName: 'Cert Manager', description: '## Old', categoryIds: ['c1'] },
              }),
            getPluginVersion: () => of({ version: { version: '1.0.0', menuIcons: [] } }),
            listPublishers: () => of({ publishers: [] }),
            listCategories: () => of({ categories: [{ id: 'c1', name: 'Security' }] }),
          },
        },
      ],
    });

    const submission = await TestBed.inject(AdminReviewService).getSubmission('s1');

    expect(submission?.description).toBe('## Rewritten');
    expect(submission?.categories).toEqual(['Security']);
    expect(submission?.listingChanges).toEqual(
      expect.objectContaining({
        descriptionChanged: true,
        categoriesChanged: false,
        authorName: 'Jetstack',
        features: [{ title: 'Issuers', body: 'Many.' }],
      }),
    );
  });
});
