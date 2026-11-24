import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { REVIEW_CLIENT } from '../../connect/admin';
import toIsoDate, { toEpochMs } from '../../connect/timestamp';
import { isCustomIcon } from '../utils/custom-icon';
import {
  type SubmissionStatus,
  type RejectionReasonValue,
  fromProtoStatus,
  fromProtoRejectionReason,
  toProtoRejectionReason,
} from '../status/submission-status';
import { type Submission, type Plugin as ReviewPlugin } from '../../generated/admin/v1/common_pb';
import {
  type ImageAnalysisPage,
  type ImageAnalysisQuery,
  fromProtoImageAnalysis,
} from './image-analysis';

// Fundament backoffice, backed by admin.v1.ReviewService. The service acts on
// submissions from every publishing organization, so it authorizes "is a
// Fundament reviewer" rather than "owns this plugin", and it serves its own
// read RPCs: the catalog holds only approved public listings, and the
// publication API authorizes ownership a reviewer does not have.

// A plugin version awaiting (or having gone through) review, joined with the
// listing it points at. The submission itself carries identifiers only.
export interface PluginSubmission {
  id: string; // submission UUID, used in URLs
  pluginId: string;
  pluginVersionId: string;
  title: string;
  // Plain text, one line: what the queue shows under the title.
  descriptionShort: string;
  // Markdown, as the storefront renders it.
  description: string;
  categories: string[];
  // data: URIs of the submitted version; empty when it declares none.
  logo: string;
  logoDark: string;
  // Resolved on the detail page only.
  menuIcons: string[];
  // The version under review. Resolved on the detail page only: the queue lists
  // submissions, and a version lookup per row would be an N+1.
  version?: string;
  // The image the version runs (digest-pinned), the definition's hash and the
  // definition.yaml itself. Detail page only, like version.
  image?: string;
  definitionHash?: string;
  manifest?: string;
  // The publishing organization. Submission carries submitter_user_id, and
  // nothing resolves a user identifier today (FUN-20), so the organization is
  // the closest thing to a name that is actually resolvable.
  submitterName: string;
  submitterUserId: string;
  submittedAt: string; // ISO date
  status: SubmissionStatus;
  // Set once a decision is made.
  reviewedAt?: string; // ISO date
  // The reviewer's name, resolved through ListReviewers; the raw id when the
  // lookup misses.
  reviewedBy?: string;
  // Raw reviewer_user_id: a dcim.users id, since reviewers authenticate against
  // the staff IDP (FUN-20). Not in the same namespace as submitterUserId.
  reviewerUserId?: string;
  // Present only when status === 'rejected'.
  rejectionReason?: RejectionReasonValue;
  feedback?: string;
}

// One review round of a plugin, as the submission page's history lists it. A
// version that was sent back and resubmitted has a round for each submission.
export interface SubmissionHistoryEntry {
  id: string; // submission UUID, used in URLs
  pluginVersionId: string;
  version: string;
  status: SubmissionStatus;
  submittedAt: string; // ISO date
  // The same instant for ordering: a day is too coarse a key.
  submittedMs: number;
  reviewedAt?: string; // ISO date
  // The reviewer's name, or the raw id when the lookup misses.
  reviewedBy?: string;
}

// The version a submission is compared with: the most recently approved other
// version of the plugin that was first submitted before this one. Not the
// newest approved version overall, which for an older round opened from the
// history would be a version that came after it.
export function baseVersionFor(
  history: SubmissionHistoryEntry[],
  pluginVersionId: string,
): SubmissionHistoryEntry | undefined {
  const firstSubmitted = new Map<string, number>();
  history.forEach((entry) => {
    const known = firstSubmitted.get(entry.pluginVersionId);
    if (known === undefined || entry.submittedMs < known) {
      firstSubmitted.set(entry.pluginVersionId, entry.submittedMs);
    }
  });
  const own = firstSubmitted.get(pluginVersionId);
  if (own === undefined) return undefined;

  let base: SubmissionHistoryEntry | undefined;
  let baseSubmitted = -Infinity;
  history.forEach((entry) => {
    const submitted = firstSubmitted.get(entry.pluginVersionId) ?? Infinity;
    if (
      entry.status === 'approved' &&
      entry.pluginVersionId !== pluginVersionId &&
      submitted < own &&
      submitted > baseSubmitted
    ) {
      base = entry;
      baseSubmitted = submitted;
    }
  });
  return base;
}

const manifestDecoder = new TextDecoder();

export interface RejectDecision {
  reason: RejectionReasonValue;
  feedback?: string;
}

@Injectable({ providedIn: 'root' })
export default class AdminReviewService {
  private readonly client = inject(REVIEW_CLIENT);

  // organization-api cannot serve publishers to a reviewer (its
  // GetOrganization is scoped to members), which is why ReviewService carries
  // its own. Both lookups are small and stable, so they are memoized.
  private publishers?: Promise<Map<string, string>>;

  private categories?: Promise<Map<string, string>>;

  private reviewers?: Promise<Map<string, string>>;

  async listSubmissions(): Promise<PluginSubmission[]> {
    // ReviewService.ListPlugins resolves every listing in the queue in one
    // call, which is what it exists for: a GetPlugin per submission would be an
    // N+1 over the whole queue.
    const [submissions, plugins, publishers, categories] = await Promise.all([
      firstValueFrom(this.client.listSubmissions({})),
      firstValueFrom(this.client.listPlugins({})),
      this.loadPublishers(),
      this.loadCategories(),
    ]);
    const byId = new Map(plugins.plugins.map((plugin) => [plugin.id, plugin]));
    return submissions.submissions.map((submission) =>
      AdminReviewService.toSubmission(
        submission,
        byId.get(submission.pluginId),
        publishers,
        categories,
      ),
    );
  }

  async getSubmission(id: string): Promise<PluginSubmission | null> {
    const response = await firstValueFrom(this.client.getSubmission({ submissionId: id }));
    const submission = response.submission;
    if (!submission) return null;

    const [plugin, version, publishers, categories, reviewers] = await Promise.all([
      firstValueFrom(this.client.getPlugin({ pluginId: submission.pluginId })),
      firstValueFrom(this.client.getPluginVersion({ pluginVersionId: submission.pluginVersionId })),
      this.loadPublishers(),
      this.loadCategories(),
      this.resolveReviewers([submission.reviewerUserId]),
    ]);

    return {
      ...AdminReviewService.toSubmission(submission, plugin.plugin, publishers, categories),
      reviewedBy: AdminReviewService.reviewerName(submission.reviewerUserId, reviewers),
      version: version.version?.version ?? '',
      menuIcons: (version.version?.menuIcons ?? []).filter(isCustomIcon),
      image: version.version?.image ?? '',
      definitionHash: version.version?.definitionHash ?? '',
      manifest: manifestDecoder.decode(version.version?.manifest),
    };
  }

  // The definition.yaml of another version, for the manifest diff.
  async getVersionManifest(pluginVersionId: string): Promise<string> {
    const response = await firstValueFrom(this.client.getPluginVersion({ pluginVersionId }));
    return manifestDecoder.decode(response.version?.manifest);
  }

  // Every review round of a plugin, newest first.
  //
  // ReviewService has no per-plugin query and Submission carries no version
  // string, so this lists the whole queue, keeps the plugin's rounds and looks
  // up each distinct version once. The queue inlines every row's logos, so this
  // costs as much as opening the queue; a plugin_id filter and a version field
  // on Submission would make it one small call.
  async listPluginHistory(pluginId: string): Promise<SubmissionHistoryEntry[]> {
    const response = await firstValueFrom(this.client.listSubmissions({}));
    const rounds = response.submissions
      .filter((submission) => submission.pluginId === pluginId)
      .sort((a, b) => toEpochMs(b.submitted) - toEpochMs(a.submitted));

    // Every round is known from the queue already, so a version that fails
    // to load only loses its label (the page shows "Unknown version"), not
    // the whole timeline.
    const versionIds = [...new Set(rounds.map((round) => round.pluginVersionId))];
    const [lookups, reviewers] = await Promise.all([
      Promise.allSettled(
        versionIds.map((pluginVersionId) =>
          firstValueFrom(this.client.getPluginVersion({ pluginVersionId })),
        ),
      ),
      this.resolveReviewers(rounds.map((round) => round.reviewerUserId)),
    ]);
    const versions = new Map(
      versionIds.map((pluginVersionId, index) => {
        const lookup = lookups[index];
        return [
          pluginVersionId,
          lookup.status === 'fulfilled' ? (lookup.value.version?.version ?? '') : '',
        ] as const;
      }),
    );

    return rounds.map((round) => ({
      id: round.id,
      pluginVersionId: round.pluginVersionId,
      version: versions.get(round.pluginVersionId) ?? '',
      status: fromProtoStatus(round.status),
      submittedAt: toIsoDate(round.submitted),
      submittedMs: toEpochMs(round.submitted),
      reviewedAt: toIsoDate(round.reviewed) || undefined,
      reviewedBy: AdminReviewService.reviewerName(round.reviewerUserId, reviewers),
    }));
  }

  // The version's image analysis against a base, one page of files at a time.
  async getImageAnalysis(query: ImageAnalysisQuery): Promise<ImageAnalysisPage> {
    const response = await firstValueFrom(
      this.client.getImageAnalysis({
        pluginVersionId: query.pluginVersionId,
        basePluginVersionId: query.basePluginVersionId ?? '',
        filesChangesOnly: query.filesChangesOnly,
        filesQuery: query.filesQuery.trim(),
        filesPageToken: query.filesPageToken ?? '',
      }),
    );
    return fromProtoImageAnalysis(response, query.pluginVersionId);
  }

  // The CycloneDX SBOM, verbatim.
  async getSbom(pluginVersionId: string): Promise<Uint8Array> {
    const response = await firstValueFrom(this.client.getSbom({ pluginVersionId }));
    return response.sbom;
  }

  async approve(id: string): Promise<void> {
    await firstValueFrom(this.client.approveSubmission({ submissionId: id }));
  }

  // Sends the version back with a note. Unlike reject this leaves the
  // submission open: the developer can fix and resubmit.
  async requestChanges(id: string, feedback: string): Promise<void> {
    await firstValueFrom(this.client.requestChanges({ submissionId: id, feedback }));
  }

  async reject(id: string, decision: RejectDecision): Promise<void> {
    await firstValueFrom(
      this.client.rejectSubmission({
        submissionId: id,
        reason: toProtoRejectionReason(decision.reason),
        feedback: decision.feedback?.trim() ?? '',
      }),
    );
  }

  private loadPublishers(): Promise<Map<string, string>> {
    this.publishers ??= firstValueFrom(this.client.listPublishers({}))
      .then(
        (response) =>
          new Map(response.publishers.map((publisher) => [publisher.id, publisher.displayName])),
      )
      .catch((err: unknown) => {
        // The promise is memoized to coalesce concurrent callers, but a failure
        // must not be: keeping a rejected promise here would fail every later
        // caller for the life of the page. Clearing the slot lets the next one
        // retry.
        this.publishers = undefined;
        throw err;
      });
    return this.publishers;
  }

  private loadCategories(): Promise<Map<string, string>> {
    this.categories ??= firstValueFrom(this.client.listCategories({}))
      .then(
        (response) => new Map(response.categories.map((category) => [category.id, category.name])),
      )
      .catch((err: unknown) => {
        // The promise is memoized to coalesce concurrent callers, but a failure
        // must not be: keeping a rejected promise here would fail every later
        // caller for the life of the page. Clearing the slot lets the next one
        // retry.
        this.categories = undefined;
        throw err;
      });
    return this.categories;
  }

  // Names for the given reviewer ids. A reviewer who decided after the list was
  // first loaded (often the person on the page) is missing from the memoized
  // list, so a miss refetches once. A failed lookup is no reason to fail the
  // page: the caller falls back to the raw id.
  private async resolveReviewers(ids: string[]): Promise<Map<string, string>> {
    const wanted = ids.filter((id) => id);
    if (wanted.length === 0) return new Map();
    try {
      let reviewers = await this.loadReviewers();
      if (wanted.some((id) => !reviewers.has(id))) {
        this.reviewers = undefined;
        reviewers = await this.loadReviewers();
      }
      return reviewers;
    } catch {
      return new Map();
    }
  }

  private loadReviewers(): Promise<Map<string, string>> {
    this.reviewers ??= firstValueFrom(this.client.listReviewers({}))
      .then(
        (response) => new Map(response.reviewers.map((reviewer) => [reviewer.id, reviewer.name])),
      )
      .catch((err: unknown) => {
        // Memoized to coalesce concurrent callers, but a failure must not be,
        // or every later caller fails for the life of the page.
        this.reviewers = undefined;
        throw err;
      });
    return this.reviewers;
  }

  private static reviewerName(
    reviewerUserId: string,
    reviewers: Map<string, string>,
  ): string | undefined {
    if (!reviewerUserId) return undefined;
    return reviewers.get(reviewerUserId) || reviewerUserId;
  }

  private static toSubmission(
    submission: Submission,
    plugin: ReviewPlugin | undefined,
    publishers: Map<string, string>,
    categories: Map<string, string>,
  ): PluginSubmission {
    const status = fromProtoStatus(submission.status);
    return {
      id: submission.id,
      pluginId: submission.pluginId,
      pluginVersionId: submission.pluginVersionId,
      title: plugin?.displayName ?? '',
      descriptionShort: plugin?.descriptionShort ?? '',
      description: plugin?.description ?? '',
      categories: (plugin?.categoryIds ?? [])
        .map((categoryId) => categories.get(categoryId))
        .filter((name): name is string => name !== undefined),
      logo: submission.logo,
      logoDark: submission.logoDark,
      menuIcons: [],
      submitterName: publishers.get(submission.organizationId) ?? submission.organizationId,
      submitterUserId: submission.submitterUserId,
      submittedAt: toIsoDate(submission.submitted),
      status,
      reviewedAt: toIsoDate(submission.reviewed) || undefined,
      reviewedBy: submission.reviewerUserId || undefined,
      reviewerUserId: submission.reviewerUserId || undefined,
      rejectionReason:
        status === 'rejected' ? fromProtoRejectionReason(submission.rejectionReason) : undefined,
      feedback: submission.feedback || undefined,
    };
  }
}
