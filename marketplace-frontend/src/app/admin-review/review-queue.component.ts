import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { PluginIconComponent } from '../icons';
import AdminReviewService, { type PluginSubmission } from './admin-review.service';
import { type SubmissionStatus, statusLabel, statusBadgeClass } from '../status/submission-status';
import connectErrorMessage from '../../connect/error';

interface StatusSummary {
  status: SubmissionStatus;
  label: string;
  count: number;
  dotColorVar: string;
}

// The four states a reviewer can act on or has acted on. `draft` never reaches
// the queue (no submission exists for it) and `withdrawn` is the developer
// taking a version back, so neither gets a counter.
const SUMMARY_STATUSES: { status: SubmissionStatus; dotColorVar: string }[] = [
  { status: 'pending', dotColorVar: 'var(--primitives-color-accent-650)' },
  { status: 'changes_requested', dotColorVar: 'var(--primitives-color-warning-600)' },
  { status: 'approved', dotColorVar: 'var(--primitives-color-success-600)' },
  { status: 'rejected', dotColorVar: 'var(--primitives-color-critical-600)' },
];

// Admin-facing review queue: lists every plugin submission, pending ones first,
// so a reviewer can pick one to approve or reject.
@Component({
  selector: 'app-review-queue',
  imports: [RouterLink, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './review-queue.component.html',
})
export default class ReviewQueueComponent implements OnInit {
  private titleService = inject(TitleService);

  private service = inject(AdminReviewService);

  private router = inject(Router);

  submissions = signal<PluginSubmission[]>([]);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  // Submissions needing a decision float to the top; within a group, newest
  // first. draft never appears in the queue, but the map covers it so the
  // vocabulary stays exhaustive.
  sortedSubmissions = computed<PluginSubmission[]>(() => {
    const order: Record<SubmissionStatus, number> = {
      pending: 0,
      changes_requested: 1,
      withdrawn: 2,
      approved: 3,
      rejected: 3,
      draft: 4,
    };
    return [...this.submissions()].sort((a, b) => {
      if (order[a.status] !== order[b.status]) return order[a.status] - order[b.status];
      return b.submittedAt.localeCompare(a.submittedAt);
    });
  });

  statusCounts = computed<StatusSummary[]>(() => {
    const submissions = this.submissions();
    return SUMMARY_STATUSES.map(({ status, dotColorVar }) => ({
      status,
      label: statusLabel(status),
      count: submissions.filter((submission) => submission.status === status).length,
      dotColorVar,
    }));
  });

  constructor() {
    this.titleService.setTitle('Review queue');
  }

  async ngOnInit() {
    try {
      this.submissions.set(await this.service.listSubmissions());
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  statusLabel = statusLabel;

  statusBadgeClass = statusBadgeClass;

  goToSubmission(id: string) {
    this.router.navigate(['/admin/submissions', id]);
  }
}
