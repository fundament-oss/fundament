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
import AdminReviewService, { type PluginSubmission } from './admin-review.service';
import {
  type RejectionReasonValue,
  REJECTION_REASONS,
  statusLabel,
  statusTagColor,
  rejectionReasonLabel,
} from '../status/submission-status';
import connectErrorMessage from '../../connect/error';

// Which decision the sheet is collecting. Rejecting ends the submission and
// needs a reason for backoffice reporting; requesting changes leaves it open
// and needs a note saying what to fix, which is the only thing the developer
// ever sees of either decision.
type DecisionMode = 'reject' | 'changes';

// Admin-facing detail view for a single submission. Shows the submitted
// metadata and, while the submission is pending, lets the reviewer approve it,
// send it back with a note, or reject it with a reason and optional feedback.
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

  // Decision sheet state.
  showDecisionSheet = signal(false);

  decisionMode = signal<DecisionMode>('reject');

  selectedReason = signal<RejectionReasonValue | ''>('');

  feedback = signal('');

  isSubmitting = signal(false);

  // Rejecting requires a reason; requesting changes requires the note itself
  // (admin.v1.RequestChangesRequest.feedback is min_len: 1), because asking for
  // changes without saying which is a dead end.
  canSubmitDecision = computed(() =>
    this.decisionMode() === 'reject'
      ? this.selectedReason() !== ''
      : this.feedback().trim().length > 0,
  );

  private readonly decisionSheetEl = viewChild<ElementRef>('decisionSheet');

  constructor() {
    effect(() => {
      const el = this.decisionSheetEl()?.nativeElement as {
        show?: () => void;
        hide?: () => void;
      };
      if (this.showDecisionSheet()) el?.show?.();
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

    try {
      const submission = await this.service.getSubmission(id);
      if (!submission) {
        this.errorMessage.set('Submission not found');
        return;
      }
      this.submission.set(submission);
      this.titleService.setTitle(`${submission.title} — Review queue`);
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  async approve() {
    const submission = this.submission();
    if (!submission) return;
    try {
      await this.service.approve(submission.id);
      this.toastService.success(`${submission.title} approved and published to the marketplace`);
      this.router.navigate(['/admin']);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    }
  }

  openReject() {
    this.openDecision('reject');
  }

  openRequestChanges() {
    this.openDecision('changes');
  }

  // Each decision starts from a blank sheet: a note or reason the reviewer
  // typed and then abandoned must never ride along with the next decision.
  private openDecision(mode: DecisionMode) {
    this.decisionMode.set(mode);
    this.selectedReason.set('');
    this.feedback.set('');
    this.showDecisionSheet.set(true);
  }

  closeDecision() {
    this.showDecisionSheet.set(false);
  }

  onReasonChange(event: Event) {
    this.selectedReason.set((event.target as HTMLSelectElement).value as RejectionReasonValue | '');
  }

  onFeedbackInput(event: Event) {
    this.feedback.set((event.target as HTMLTextAreaElement).value);
  }

  async submitDecision() {
    const submission = this.submission();
    if (!submission || !this.canSubmitDecision()) return;

    this.isSubmitting.set(true);
    try {
      if (this.decisionMode() === 'changes') {
        await this.service.requestChanges(submission.id, this.feedback().trim());
        this.toastService.info(`Changes requested on ${submission.title}`);
      } else {
        const reason = this.selectedReason();
        if (reason === '') return;
        await this.service.reject(submission.id, { reason, feedback: this.feedback() });
        this.toastService.info(`${submission.title} rejected`);
      }
      this.closeDecision();
      this.router.navigate(['/admin']);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    } finally {
      this.isSubmitting.set(false);
    }
  }

  statusLabel = statusLabel;

  statusTagColor = statusTagColor;

  rejectionReasonLabel = rejectionReasonLabel;
}
