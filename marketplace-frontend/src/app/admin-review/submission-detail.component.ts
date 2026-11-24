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
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/cell';
import '@nldd/design-system/segmented-control';
import '@nldd/design-system/code-viewer';
import { createPatch, OMIT_HEADERS } from 'diff';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PluginIconComponent } from '../icons';
import MarkdownComponent from '../markdown.component';
import ImageAnalysisComponent from './image-analysis.component';
import opensElsewhere from '../utils/opens-elsewhere';
import AdminReviewService, {
  type PluginSubmission,
  type SubmissionHistoryEntry,
  baseVersionFor,
} from './admin-review.service';
import ReviewQueueFiltersService from './review-queue-filters.service';
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

// The manifest's changes against the base version, or the whole file.
type ManifestView = 'changes' | 'full';

// What the manifest is compared with. Known only once the history is in, since
// the base is picked from it.
type ManifestBase =
  | { state: 'loading' }
  // No version of this plugin was approved before this one.
  | { state: 'none' }
  | { state: 'unavailable'; reason: string }
  | { state: 'loaded'; version: string; manifest: string };

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
  imports: [RouterLink, PluginIconComponent, MarkdownComponent, ImageAnalysisComponent],
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

  // The filters of the queue this reviewer came from, for every way back to it.
  protected queueParams = inject(ReviewQueueFiltersService).queryParams;

  // Moving through the history reuses this component for another id, so a
  // slow response for the submission you left must not land on the next one.
  private loadSequence = 0;

  readonly rejectionReasons = REJECTION_REASONS;

  submission = signal<PluginSubmission | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  descriptionView = signal<DescriptionView>('preview');

  // The version this one is compared with; undefined until the history is in,
  // null when there is none. The image analysis compares against it too.
  baseVersion = signal<SubmissionHistoryEntry | null | undefined>(undefined);

  manifestBase = signal<ManifestBase>({ state: 'loading' });

  manifestView = signal<ManifestView>('changes');

  manifestNote = computed(() => {
    const base = this.manifestBase();
    switch (base.state) {
      case 'loading':
        return 'Looking up the previous approved version...';
      case 'none':
        return 'No earlier version of this plugin was approved, so the full manifest is shown.';
      case 'unavailable':
        return base.reason;
      case 'loaded': {
        const version = base.version || 'the previous version';
        // The card says so itself, in place of the manifest.
        if (!this.submission()?.manifest) return '';
        return this.manifestDiff()
          ? `Compared with ${version}, the most recent approved version before this one.`
          : `Identical to the manifest of ${version}.`;
      }
      default:
        throw new Error(`unhandled manifest base state: ${JSON.stringify(base satisfies never)}`);
    }
  });

  // A unified diff of the manifest against the base, without file headers
  // (nldd-code-viewer leaves those out anyway). Empty when there is no base or
  // nothing changed, in which case the page shows the full manifest only.
  manifestDiff = computed(() => {
    const base = this.manifestBase();
    const current = this.submission()?.manifest ?? '';
    if (base.state !== 'loaded' || !current || base.manifest === current) return '';
    return createPatch('definition.yaml', base.manifest, current, '', '', {
      headerOptions: OMIT_HEADERS,
    });
  });

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

    // Until this submission is in, the tab must not keep the previous one's name.
    this.titleService.setTitle('Review queue');
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.submission.set(null);
    this.history.set(null);
    this.historyError.set(null);
    this.descriptionView.set('preview');
    this.baseVersion.set(undefined);
    this.manifestBase.set({ state: 'loading' });
    this.manifestView.set('changes');
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
    let history: SubmissionHistoryEntry[];
    try {
      history = await this.service.listPluginHistory(submission.pluginId);
      if (!isCurrent()) return;
      this.history.set(history);
    } catch (error) {
      if (!isCurrent()) return;
      this.historyError.set(connectErrorMessage(error));
      this.baseVersion.set(null);
      this.manifestBase.set({
        state: 'unavailable',
        reason:
          'The submission history could not be loaded, so there is no version to compare with.',
      });
      return;
    }

    const base = baseVersionFor(history, submission.pluginVersionId) ?? null;
    this.baseVersion.set(base);
    if (!base) {
      this.manifestBase.set({ state: 'none' });
      return;
    }
    try {
      const manifest = await this.service.getVersionManifest(base.pluginVersionId);
      if (isCurrent()) this.manifestBase.set({ state: 'loaded', version: base.version, manifest });
    } catch (error) {
      if (isCurrent()) {
        this.manifestBase.set({
          state: 'unavailable',
          reason: `The manifest of ${base.version || 'the previous version'} could not be loaded: ${connectErrorMessage(error)}`,
        });
      }
    }
  }

  // The rows are real links, so a new tab or a copied address works; a plain
  // click stays in the app instead of reloading it.
  openHistoryEntry(event: Event, id: string) {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.router.navigate(['/admin/submissions', id]);
  }

  // nldd-segmented-control reports its selection in the event's detail.
  onDescriptionViewChange(event: Event) {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.descriptionView.set(value === 'source' ? 'source' : 'preview');
  }

  onManifestViewChange(event: Event) {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.manifestView.set(value === 'full' ? 'full' : 'changes');
  }

  async approve() {
    const submission = this.submission();
    if (!submission) return;
    try {
      await this.service.approve(submission.id);
      this.toastService.success(`${submission.title} approved and published to the marketplace`);
      this.router.navigate(['/admin'], { queryParams: this.queueParams() });
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
      this.router.navigate(['/admin'], { queryParams: this.queueParams() });
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
