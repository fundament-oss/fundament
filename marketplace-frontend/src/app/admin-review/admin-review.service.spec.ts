import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { describe, expect, it } from 'vitest';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { REVIEW_CLIENT } from '../../connect/admin';
import AdminReviewService, {
  baseVersionFor,
  type SubmissionHistoryEntry,
} from './admin-review.service';

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

describe('AdminReviewService reviewer names', () => {
  function serviceWith(listReviewers: () => unknown) {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: REVIEW_CLIENT,
          useValue: {
            getSubmission: ({ submissionId }: { submissionId: string }) =>
              of({
                submission: {
                  id: submissionId,
                  pluginId: 'p1',
                  pluginVersionId: 'v1',
                  reviewerUserId: submissionId === 's-known' ? 'r1' : 'r-unknown',
                },
              }),
            listSubmissions: () =>
              of({
                submissions: [
                  { id: 's-known', pluginId: 'p1', pluginVersionId: 'v1', reviewerUserId: 'r1' },
                ],
              }),
            listPlugins: () => of({ plugins: [] }),
            getPlugin: () => of({ plugin: { displayName: 'Grafana' } }),
            getPluginVersion: () => of({ version: { version: '1.0.0' } }),
            listPublishers: () => of({ publishers: [] }),
            listCategories: () => of({ categories: [] }),
            listReviewers,
          },
        },
      ],
    });
    return TestBed.inject(AdminReviewService);
  }

  it('shows a known reviewer by name and keeps the id', async () => {
    const service = serviceWith(() => of({ reviewers: [{ id: 'r1', name: 'Alice' }] }));

    const submission = await service.getSubmission('s-known');

    expect(submission?.reviewedBy).toBe('Alice');
    expect(submission?.reviewerUserId).toBe('r1');
  });

  it('leaves the name unset in the queue, which does not resolve it', async () => {
    const service = serviceWith(() => of({ reviewers: [{ id: 'r1', name: 'Alice' }] }));

    const [submission] = await service.listSubmissions();

    expect(submission.reviewedBy).toBeUndefined();
    expect(submission.reviewerUserId).toBe('r1');
  });

  it('falls back to the id when the lookup misses', async () => {
    const service = serviceWith(() => of({ reviewers: [{ id: 'r1', name: 'Alice' }] }));

    const submission = await service.getSubmission('s-unknown');

    expect(submission?.reviewedBy).toBe('r-unknown');
  });

  it('falls back to the id when the lookup fails', async () => {
    const service = serviceWith(() => throwError(() => new Error('unavailable')));

    const submission = await service.getSubmission('s-known');

    expect(submission?.reviewedBy).toBe('r1');
  });

  it('refetches once when a reviewer decided after the list was loaded', async () => {
    let calls = 0;
    const service = serviceWith(() => {
      calls += 1;
      return of({ reviewers: calls === 1 ? [] : [{ id: 'r1', name: 'Alice' }] });
    });

    const submission = await service.getSubmission('s-known');

    expect(submission?.reviewedBy).toBe('Alice');
    expect(calls).toBe(2);
  });
});

describe('baseVersionFor', () => {
  function entry(
    id: string,
    pluginVersionId: string,
    status: SubmissionHistoryEntry['status'],
    submitted: string,
  ): SubmissionHistoryEntry {
    return {
      id,
      pluginVersionId,
      version: pluginVersionId,
      status,
      submittedAt: submitted,
      submittedMs: Date.parse(submitted),
    };
  }

  // Newest first, as listPluginHistory returns it.
  const history = [
    entry('s5', 'v4', 'pending', '2026-11-20'),
    entry('s4', 'v3', 'approved', '2026-11-01'),
    entry('s3', 'v2', 'rejected', '2026-10-15'),
    // v1 was sent back once and approved on its second round.
    entry('s2', 'v1', 'approved', '2026-10-02'),
    entry('s1', 'v1', 'approved', '2026-09-28'),
  ];

  it('picks the latest approved version submitted before this one', () => {
    expect(baseVersionFor(history, 'v4')?.pluginVersionId).toBe('v3');
  });

  it('ignores an approved version that came after an older round', () => {
    expect(baseVersionFor(history, 'v2')?.pluginVersionId).toBe('v1');
  });

  it('skips versions that were not approved', () => {
    expect(baseVersionFor(history, 'v3')?.pluginVersionId).toBe('v1');
  });

  it('has no base for the first approved version', () => {
    expect(baseVersionFor(history, 'v1')).toBeUndefined();
  });

  it('has no base for a version missing from the history', () => {
    expect(baseVersionFor(history, 'unknown')).toBeUndefined();
  });
});
