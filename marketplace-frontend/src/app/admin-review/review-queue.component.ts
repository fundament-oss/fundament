import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
// Pads the empty state's box; loaded with this page rather than up front.
import '@nldd/design-system/container';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { TitleService } from '../title.service';
import { ConfigService } from '../config.service';
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
// Doubling as the status filter: clicking a card shows only that status.
const SUMMARY_STATUSES: { status: SubmissionStatus; dotColorVar: string }[] = [
  { status: 'pending', dotColorVar: 'var(--primitives-color-accent-650)' },
  { status: 'changes_requested', dotColorVar: 'var(--primitives-color-warning-600)' },
  { status: 'approved', dotColorVar: 'var(--primitives-color-success-600)' },
  { status: 'rejected', dotColorVar: 'var(--primitives-color-critical-600)' },
];

// A status the queue can be filtered on. Withdrawn has no card, but a link with
// ?status=withdrawn is still honoured; draft never reaches the queue.
const FILTERABLE_STATUSES: readonly SubmissionStatus[] = [
  'pending',
  'changes_requested',
  'approved',
  'rejected',
  'withdrawn',
];

function parseStatus(value: string | null): SubmissionStatus | null {
  return FILTERABLE_STATUSES.find((status) => status === value) ?? null;
}

// Admin-facing review queue: lists every plugin submission, pending ones first,
// so a reviewer can pick one to approve or reject. The filters live in the
// address (?status=, ?q=), so Back from a submission returns to the same view.
@Component({
  selector: 'app-review-queue',
  imports: [RouterLink, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './review-queue.component.html',
})
export default class ReviewQueueComponent implements OnInit {
  private titleService = inject(TitleService);

  // The storefront is its own deployable (FUN-20); empty hides the link.
  protected storefrontUrl = inject(ConfigService).getConfig().storefrontUrl ?? '';

  private service = inject(AdminReviewService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private queryParams = toSignal(this.route.queryParamMap);

  statusFilter = computed(() => parseStatus(this.queryParams()?.get('status') ?? null));

  searchQuery = computed(() => this.queryParams()?.get('q') ?? '');

  hasFilters = computed(() => this.statusFilter() !== null || this.searchQuery().trim() !== '');

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

  filteredSubmissions = computed<PluginSubmission[]>(() => {
    const status = this.statusFilter();
    const query = this.searchQuery().trim().toLocaleLowerCase();
    return this.sortedSubmissions().filter(
      (submission) =>
        (status === null || submission.status === status) &&
        (query === '' ||
          submission.title.toLocaleLowerCase().includes(query) ||
          submission.submitterName.toLocaleLowerCase().includes(query)),
    );
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

  // Clicking the card of the status already shown clears the filter again.
  toggleStatus(status: SubmissionStatus) {
    this.setFilters({ status: this.statusFilter() === status ? null : status });
  }

  onSearchInput(event: Event) {
    this.setFilters({ q: (event.target as HTMLInputElement).value || null });
  }

  clearFilters() {
    this.setFilters({ status: null, q: null });
  }

  // replaceUrl: each keystroke and card click is a refinement of the same
  // view, not a page of its own to step back through.
  private setFilters(params: { status?: SubmissionStatus | null; q?: string | null }) {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: params,
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
  }
}
