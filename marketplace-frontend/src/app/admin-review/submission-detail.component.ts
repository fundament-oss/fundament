import {
  Component,
  inject,
  signal,
  computed,
  effect,
  viewChild,
  ElementRef,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PluginIconComponent } from '../icons';
import AdminReviewService, {
  type PluginSubmission,
  type RejectionReasonValue,
  REJECTION_REASONS,
} from './admin-review.service';
import {
  submissionStatusLabel,
  submissionStatusTagColor,
  rejectionReasonLabel,
} from './submission-status';

// Admin-facing detail view for a single submission. Shows the submitted
// metadata and, while the submission is pending, lets the reviewer approve it
// or reject it with a reason (from a fixed dropdown) and optional feedback.
@Component({
  selector: 'app-submission-detail',
  imports: [RouterLink, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './submission-detail.component.html',
})
export default class SubmissionDetailComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private service = inject(AdminReviewService);

  readonly rejectionReasons = REJECTION_REASONS;

  submission = signal<PluginSubmission | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  // Reject dialog state.
  showRejectSheet = signal(false);

  selectedReason = signal<RejectionReasonValue | ''>('');

  feedback = signal('');

  // The reject action requires a reason to be chosen.
  canReject = computed(() => this.selectedReason() !== '');

  private readonly rejectSheetEl = viewChild<ElementRef>('rejectSheet');

  constructor() {
    effect(() => {
      const el = this.rejectSheetEl()?.nativeElement as {
        show?: () => void;
        hide?: () => void;
      };
      if (this.showRejectSheet()) el?.show?.();
      else el?.hide?.();
    });
  }

  async ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id');
    if (!id) {
      this.errorMessage.set('Submission id is missing');
      this.isLoading.set(false);
      return;
    }

    const submission = await this.service.getSubmission(id);
    if (!submission) {
      this.errorMessage.set('Submission not found');
      this.isLoading.set(false);
      return;
    }

    this.submission.set(submission);
    this.titleService.setTitle(`${submission.title} — Review queue`);
    this.isLoading.set(false);
  }

  async approve() {
    const submission = this.submission();
    if (!submission) return;
    await this.service.approve(submission.id);
    this.toastService.success(`${submission.title} approved and published to the marketplace`);
    this.router.navigate(['/admin']);
  }

  openReject() {
    this.showRejectSheet.set(true);
  }

  closeReject() {
    this.showRejectSheet.set(false);
  }

  onReasonChange(event: Event) {
    this.selectedReason.set((event.target as HTMLSelectElement).value as RejectionReasonValue | '');
  }

  onFeedbackInput(event: Event) {
    this.feedback.set((event.target as HTMLTextAreaElement).value);
  }

  async submitReject() {
    const submission = this.submission();
    const reason = this.selectedReason();
    if (!submission || reason === '') return;

    await this.service.reject(submission.id, { reason, feedback: this.feedback() });
    this.toastService.info(`${submission.title} rejected`);
    this.closeReject();
    this.router.navigate(['/admin']);
  }

  statusLabel = submissionStatusLabel;

  statusTagColor = submissionStatusTagColor;

  rejectionReasonLabel = rejectionReasonLabel;
}
