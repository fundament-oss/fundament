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
