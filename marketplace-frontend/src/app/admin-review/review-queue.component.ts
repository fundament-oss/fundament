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
import AdminReviewService, {
  type PluginSubmission,
  type SubmissionStatus,
} from './admin-review.service';
import { submissionStatusLabel, submissionStatusBadgeClass } from './submission-status';

interface StatusSummary {
  status: SubmissionStatus;
  label: string;
  count: number;
  dotColorVar: string;
}

const SUMMARY_STATUSES: { status: SubmissionStatus; dotColorVar: string }[] = [
  { status: 'pending', dotColorVar: 'var(--primitives-color-accent-650)' },
  { status: 'approved', dotColorVar: 'var(--primitives-color-success-600)' },
  { status: 'rejected', dotColorVar: 'var(--primitives-color-warning-600)' },
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

  // Pending submissions float to the top; within a status, newest first.
  sortedSubmissions = computed<PluginSubmission[]>(() => {
    const order: Record<SubmissionStatus, number> = { pending: 0, approved: 1, rejected: 1 };
    return [...this.submissions()].sort((a, b) => {
      if (order[a.status] !== order[b.status]) return order[a.status] - order[b.status];
      return b.submittedAt.localeCompare(a.submittedAt);
    });
  });

  statusCounts = computed<StatusSummary[]>(() => {
    const submissions = this.submissions();
    return SUMMARY_STATUSES.map(({ status, dotColorVar }) => ({
      status,
      label: submissionStatusLabel(status),
      count: submissions.filter((submission) => submission.status === status).length,
      dotColorVar,
    }));
  });

  constructor() {
    this.titleService.setTitle('Review queue');
  }

  async ngOnInit() {
    this.submissions.set(await this.service.listSubmissions());
    this.isLoading.set(false);
  }

  statusLabel = submissionStatusLabel;

  statusBadgeClass = submissionStatusBadgeClass;

  goToSubmission(id: string) {
    this.router.navigate(['/admin/submissions', id]);
  }
}
