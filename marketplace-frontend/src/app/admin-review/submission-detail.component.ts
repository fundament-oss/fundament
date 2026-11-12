import {
  Component,
  inject,
  signal,
  computed,
  effect,
  viewChild,
  ElementRef,
  DestroyRef,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Location } from '@angular/common';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
// The submission history timeline and the description's view switch; loaded
// with this page rather than up front.
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/step-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/cell';
import '@nldd/design-system/segmented-control';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PluginIconComponent } from '../icons';
import MarkdownComponent from '../markdown.component';
import opensElsewhere from '../utils/opens-elsewhere';
import AdminReviewService, {
  type PluginSubmission,
  type SubmissionHistoryEntry,
} from './admin-review.service';
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

// The description as the storefront will render it, or the markdown the
// publisher wrote: a reviewer has to be able to check both.
type DescriptionView = 'preview' | 'source';

// A history entry with what its row in the nldd-step-cell timeline needs.
interface HistoryRow extends SubmissionHistoryEntry {
  // A round under the newer round of the same version is a minor step of it.
  level: 'major' | 'minor';
  position: 'first' | 'between' | 'last' | 'only';
  // An open round is where the plugin stands now; every other one is behind it.
  stepStatus: 'current' | 'past';
  href: string;
}

// Admin-facing detail view for a single submission. Shows the submitted
// metadata and, while the submission is pending, lets the reviewer approve it,
// send it back with a note, or reject it with a reason and optional feedback.
@Component({
  selector: 'app-submission-detail',
  imports: [RouterLink, PluginIconComponent, MarkdownComponent],
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

  private location = inject(Location);

  private destroyRef = inject(DestroyRef);

  // Moving through the history reuses this component for another id, so a
  // slow response for the submission you left must not land on the next one.
  private loadSequence = 0;

  readonly rejectionReasons = REJECTION_REASONS;

  submission = signal<PluginSubmission | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  descriptionView = signal<DescriptionView>('preview');

  // Every review round of this plugin, newest first. Loaded after the
  // submission and on its own: a page that fails to list the history still
  // shows the submission under review.
  history = signal<SubmissionHistoryEntry[] | null>(null);

  historyError = signal<string | null>(null);

  historyRows = computed<HistoryRow[]>(() => {
    const entries = this.history() ?? [];
    const last = entries.length - 1;
    return entries.map((entry, index) => {
      let position: HistoryRow['position'] = 'between';
      if (last === 0) position = 'only';
      else if (index === 0) position = 'first';
      else if (index === last) position = 'last';
      return {
        ...entry,
        level:
          index > 0 && entries[index - 1].pluginVersionId === entry.pluginVersionId
            ? 'minor'
            : 'major',
        position,
        stepStatus: entry.status === 'pending' ? 'current' : 'past',
        href: this.location.prepareExternalUrl(`/admin/submissions/${entry.id}`),
      };
    });
  });

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

  ngOnInit() {
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      this.load(params.get('id'));
    });
  }

  private async load(id: string | null) {
    this.loadSequence += 1;
    const sequence = this.loadSequence;
    const isCurrent = () => sequence === this.loadSequence;

    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.submission.set(null);
    this.history.set(null);
    this.historyError.set(null);
    this.descriptionView.set('preview');
    this.showDecisionSheet.set(false);

    if (!id) {
      this.errorMessage.set('Submission id is missing');
      this.isLoading.set(false);
      return;
    }

    let submission: PluginSubmission | null = null;
    try {
      submission = await this.service.getSubmission(id);
      if (!isCurrent()) return;
      if (!submission) {
        this.errorMessage.set('Submission not found');
        return;
      }
      this.submission.set(submission);
      this.titleService.setTitle(`${submission.title} — Review queue`);
    } catch (error) {
      if (isCurrent()) this.errorMessage.set(connectErrorMessage(error));
      return;
    } finally {
      if (isCurrent()) this.isLoading.set(false);
    }

    if (!submission) return;
    try {
      const history = await this.service.listPluginHistory(submission.pluginId);
      if (isCurrent()) this.history.set(history);
    } catch (error) {
      if (isCurrent()) this.historyError.set(connectErrorMessage(error));
    }
  }

  // The rows are real links, so a new tab or a copied address works; a plain
  // click stays in the app instead of reloading it.
  openHistoryEntry(event: Event, id: string) {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.router.navigate(['/admin/submissions', id]);
  }

  onDescriptionViewChange(event: Event) {
    const { value } = event.target as HTMLInputElement;
    this.descriptionView.set(value === 'source' ? 'source' : 'preview');
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
