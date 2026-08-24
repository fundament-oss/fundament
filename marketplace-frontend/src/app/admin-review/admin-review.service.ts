import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { REVIEW_CLIENT } from '../../connect/tokens';
import toIsoDate from '../../connect/timestamp';
import {
  type SubmissionStatus,
  type RejectionReasonValue,
  fromProtoStatus,
  fromProtoRejectionReason,
  toProtoRejectionReason,
} from '../status/submission-status';
import { type Submission, type Plugin as ReviewPlugin } from '../../generated/admin/v1/common_pb';

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
  title: string;
  description: string;
  categories: string[];
  icon: string; // base name under /img/plugins/<icon>.svg
  // The version under review. Resolved on the detail page only: the queue lists
  // submissions, and a version lookup per row would be an N+1.
  version?: string;
  // The publishing organization. Submission carries submitter_user_id, and
  // nothing resolves a user identifier today (FUN-20), so the organization is
  // the closest thing to a name that is actually resolvable.
  submitterName: string;
  submitterUserId: string;
  submittedAt: string; // ISO date
  status: SubmissionStatus;
  // Set once a decision is made.
  reviewedAt?: string; // ISO date
  // Raw reviewer_user_id. Which store marketplace reviewers authenticate
  // against is undecided (FUN-20), so this cannot be resolved to a person and
  // is not even in the same namespace as submitterUserId.
  reviewedBy?: string;
  // Present only when status === 'rejected'.
  rejectionReason?: RejectionReasonValue;
  feedback?: string;
}

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

    const [plugin, version, publishers, categories] = await Promise.all([
      firstValueFrom(this.client.getPlugin({ pluginId: submission.pluginId })),
      firstValueFrom(this.client.getPluginVersion({ pluginVersionId: submission.pluginVersionId })),
      this.loadPublishers(),
      this.loadCategories(),
    ]);

    return {
      ...AdminReviewService.toSubmission(submission, plugin.plugin, publishers, categories),
      version: version.version?.version ?? '',
    };
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
    this.publishers ??= firstValueFrom(this.client.listPublishers({})).then(
      (response) =>
        new Map(response.publishers.map((publisher) => [publisher.id, publisher.displayName])),
    );
    return this.publishers;
  }

  private loadCategories(): Promise<Map<string, string>> {
    this.categories ??= firstValueFrom(this.client.listCategories({})).then(
      (response) => new Map(response.categories.map((category) => [category.id, category.name])),
    );
    return this.categories;
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
      title: plugin?.displayName ?? '',
      description: plugin?.description ?? '',
      categories: (plugin?.categoryIds ?? [])
        .map((categoryId) => categories.get(categoryId))
        .filter((name): name is string => name !== undefined),
      icon: plugin?.name ?? '',
      submitterName: publishers.get(submission.organizationId) ?? submission.organizationId,
      submitterUserId: submission.submitterUserId,
      submittedAt: toIsoDate(submission.submitted),
      status,
      reviewedAt: toIsoDate(submission.reviewed) || undefined,
      reviewedBy: submission.reviewerUserId || undefined,
      rejectionReason:
        status === 'rejected' ? fromProtoRejectionReason(submission.rejectionReason) : undefined,
      feedback: submission.feedback || undefined,
    };
  }
}
