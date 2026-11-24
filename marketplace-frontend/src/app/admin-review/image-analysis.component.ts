import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import '@nldd/design-system/checkbox-field';
import AdminReviewService, { type SubmissionHistoryEntry } from './admin-review.service';
import {
  type ImageAnalysisPage,
  type ImageFileEntry,
  changeLabel,
  changeTagColor,
  formatMode,
  formatSize,
  isInProgress,
  summaryText,
} from './image-analysis';
import { ToastService } from '../toast.service';
import connectErrorMessage from '../../connect/error';

// How often an analysis still on its way is checked again.
const POLL_INTERVAL_MS = 10_000;

// Waits for a pause in typing before filtering the files on the server.
const QUERY_DEBOUNCE_MS = 300;

// The image a version runs, as marketplace-analysis-worker found it (FUN-24):
// its packages from the SBOM, and its files, each as a diff against the base
// version the manifest is compared with.
@Component({
  selector: 'app-image-analysis',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './image-analysis.component.html',
})
export default class ImageAnalysisComponent {
  private service = inject(AdminReviewService);

  private toastService = inject(ToastService);

  pluginVersionId = input.required<string>();

  // Name and version of the plugin, for the SBOM's file name.
  pluginName = input('');

  version = input('');

  // The version to compare with: undefined until the history is in, null
  // when no earlier version was approved.
  base = input<SubmissionHistoryEntry | null | undefined>(undefined);

  page = signal<ImageAnalysisPage | null>(null);

  errorMessage = signal<string | null>(null);

  // Packages arrive whole and are filtered here.
  packagesChangesOnly = signal(true);

  packageQuery = signal('');

  // Files are filtered and paged by the server: a file list runs to 200k.
  filesChangesOnly = signal(true);

  filesQuery = signal('');

  files = signal<ImageFileEntry[]>([]);

  nextFilesPageToken = signal('');

  loadingMore = signal(false);

  compared = computed(() => this.page()?.compared ?? false);

  completed = computed(() => this.page()?.analysis.status === 'completed');

  // Neither side's change is known while one is in progress: keep asking.
  inProgress = computed(() => {
    const page = this.page();
    return isInProgress(page?.analysis.status) || isInProgress(page?.base?.status);
  });

  visiblePackages = computed(() => {
    const packages = this.page()?.packages ?? [];
    const changesOnly = this.compared() && this.packagesChangesOnly();
    const query = this.packageQuery().trim().toLocaleLowerCase();
    return packages.filter(
      (pkg) =>
        (!changesOnly || pkg.change !== 'unchanged') &&
        (query === '' ||
          pkg.name.toLocaleLowerCase().includes(query) ||
          pkg.purl.toLocaleLowerCase().includes(query)),
    );
  });

  // The platforms an index offers besides the one that was analyzed.
  otherPlatforms = computed(() => {
    const analysis = this.page()?.analysis;
    return (analysis?.platforms ?? []).filter((platform) => platform !== analysis?.platform);
  });

  baseVersionLabel = computed(() => this.base()?.version || 'the previous version');

  // What the lists are compared with, or why they are not.
  comparisonNote = computed(() => {
    const page = this.page();
    if (this.base() === null) {
      return 'No earlier version of this plugin was approved, so the full lists are shown.';
    }
    if (page?.compared) return `Compared with ${this.baseVersionLabel()}.`;
    if (page?.base?.status === 'failed') {
      return `${this.baseVersionLabel()} could not be analyzed (${page.base.statusInfo}), so the full lists are shown.`;
    }
    if (page?.base?.status === 'none') {
      return `${this.baseVersionLabel()} has not been analyzed yet, so the full lists are shown for now.`;
    }
    return `${this.baseVersionLabel()} is still being analyzed, so the full lists are shown for now.`;
  });

  // Moving to another submission reuses this component: a response for the
  // version you left must not land on the next one.
  private sequence = 0;

  private pollTimer?: ReturnType<typeof setTimeout>;

  private queryTimer?: ReturnType<typeof setTimeout>;

  constructor() {
    effect(() => {
      this.pluginVersionId();
      const base = this.base();
      if (base === undefined) return;
      untracked(() => {
        this.page.set(null);
        this.errorMessage.set(null);
        this.packagesChangesOnly.set(true);
        this.packageQuery.set('');
        this.filesChangesOnly.set(true);
        this.filesQuery.set('');
        this.load();
      });
    });

    inject(DestroyRef).onDestroy(() => {
      this.sequence += 1;
      clearTimeout(this.pollTimer);
      clearTimeout(this.queryTimer);
    });
  }

  // Loads the first page of files with the current filters, and keeps polling
  // while an analysis is in progress.
  private async load() {
    this.sequence += 1;
    const sequence = this.sequence;
    clearTimeout(this.pollTimer);

    try {
      const page = await this.service.getImageAnalysis({
        pluginVersionId: this.pluginVersionId(),
        basePluginVersionId: this.base()?.pluginVersionId,
        filesChangesOnly: this.filesChangesOnly(),
        filesQuery: this.filesQuery(),
      });
      if (sequence !== this.sequence) return;
      this.errorMessage.set(null);
      this.page.set(page);
      this.files.set(page.files);
      this.nextFilesPageToken.set(page.nextFilesPageToken);
    } catch (error) {
      if (sequence !== this.sequence) return;
      this.errorMessage.set(connectErrorMessage(error));
      return;
    }

    if (this.inProgress()) {
      this.pollTimer = setTimeout(() => this.load(), POLL_INTERVAL_MS);
    }
  }

  async loadMoreFiles() {
    const token = this.nextFilesPageToken();
    if (!token || this.loadingMore()) return;
    const { sequence } = this;
    this.loadingMore.set(true);
    try {
      const page = await this.service.getImageAnalysis({
        pluginVersionId: this.pluginVersionId(),
        basePluginVersionId: this.base()?.pluginVersionId,
        filesChangesOnly: this.filesChangesOnly(),
        filesQuery: this.filesQuery(),
        filesPageToken: token,
      });
      if (sequence !== this.sequence) return;
      this.files.update((files) => [...files, ...page.files]);
      this.nextFilesPageToken.set(page.nextFilesPageToken);
    } catch (error) {
      if (sequence === this.sequence) this.toastService.error(connectErrorMessage(error));
    } finally {
      this.loadingMore.set(false);
    }
  }

  // nldd-checkbox-field reports its state in the event's detail.
  onPackagesChangesOnly(event: Event) {
    this.packagesChangesOnly.set((event as CustomEvent<{ checked: boolean }>).detail.checked);
  }

  onPackageQuery(event: Event) {
    this.packageQuery.set((event.target as HTMLInputElement).value ?? '');
  }

  onFilesChangesOnly(event: Event) {
    this.filesChangesOnly.set((event as CustomEvent<{ checked: boolean }>).detail.checked);
    this.load();
  }

  onFilesQuery(event: Event) {
    this.filesQuery.set((event.target as HTMLInputElement).value ?? '');
    clearTimeout(this.queryTimer);
    this.queryTimer = setTimeout(() => this.load(), QUERY_DEBOUNCE_MS);
  }

  async downloadSbom() {
    try {
      const sbom = await this.service.getSbom(this.pluginVersionId());
      const url = URL.createObjectURL(
        new Blob([sbom as BlobPart], { type: 'application/vnd.cyclonedx+json' }),
      );
      const link = document.createElement('a');
      link.href = url;
      link.download = `${this.pluginName() || 'plugin'}-${this.version() || 'version'}.cdx.json`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    }
  }

  summaryText = summaryText;

  changeLabel = changeLabel;

  changeTagColor = changeTagColor;

  formatMode = formatMode;

  formatSize = formatSize;
}
