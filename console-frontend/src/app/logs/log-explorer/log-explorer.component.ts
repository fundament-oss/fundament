import {
  Component,
  ViewChild,
  AfterViewInit,
  ElementRef,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  signal,
  computed,
  effect,
  OnInit,
  OnDestroy,
} from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { Subscription } from 'rxjs';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import type { LogEntry, LogLevel, HistogramBucket } from '../log.types';
import { LogsApiService, type ClusterOption } from '../logs.service';
import { ShootPodsService, type ShootPod } from '../shoot-pods.service';
import { LogBackend, LogSource } from '../../../generated/v1/logs_pb';
import { TitleService } from '../../title.service';
import { NotificationService } from '../../notification.service';
import PageNavService from '../../page-nav.service';
import PluginInstallationService from '../../plugin-installation/plugin-installation.service';
import '@nldd/design-system/search-field';
import '@nldd/design-system/tab-bar';
import '@nldd/design-system/sidebar-section';
import '@nldd/design-system/combo-box';
import '@nldd/design-system/radio-button';
import '@nldd/design-system/code-viewer';
import '@nldd/design-system/token';
import '@nldd/design-system/pagination';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/checkbox';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/tag';
import '@nldd/design-system/text';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';

Chart.register(...registerables);

const ALL_LEVELS: LogLevel[] = ['ERROR', 'WARN', 'INFO', 'DEBUG'];
const PAGE_SIZE = 50;

const TIME_PRESETS: { label: string; value: string; minutes: number }[] = [
  { label: 'Last 15 min', value: '15m', minutes: 15 },
  { label: 'Last 1 hour', value: '1h', minutes: 60 },
  { label: 'Last 6 hours', value: '6h', minutes: 360 },
  { label: 'Last 24 hours', value: '24h', minutes: 1440 },
  { label: 'Last 7 days', value: '7d', minutes: 10080 },
];

/** ERROR is a failure, WARN is worth a look, INFO is the ordinary case and
 *  DEBUG is noise you asked for yourself. */
const LEVEL_LABEL: Record<LogLevel, string> = {
  ERROR: 'Error',
  WARN: 'Warning',
  INFO: 'Info',
  DEBUG: 'Debug',
};

const LEVEL_TAG_COLOR: Record<LogLevel, string> = {
  ERROR: 'critical',
  WARN: 'warning',
  INFO: 'accent',
  DEBUG: 'neutral',
};

/* The wire says ERROR and WARN, the screen does not shout. */
const levelLabel = (level: LogLevel): string => LEVEL_LABEL[level];

const levelTagColor = (level: LogLevel): string => LEVEL_TAG_COLOR[level];

const LEVEL_CHIP_ACTIVE: Record<LogLevel, string> = {
  ERROR:
    'border-danger-300 bg-danger-50 text-danger-700 dark:border-danger-700 dark:bg-danger-950 dark:text-danger-300',
  WARN: 'border-yellow-300 bg-yellow-50 text-yellow-700 dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-300',
  INFO: 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-950 dark:text-blue-300',
  DEBUG:
    'border-neutral-300 bg-neutral-50 text-neutral-600 dark:border-neutral-600 dark:bg-neutral-900 dark:text-neutral-400',
};

const HISTOGRAM_COLORS: Record<string, string> = {
  error: 'rgba(220, 38, 38, 0.75)',
  warn: 'rgba(217, 119, 6, 0.75)',
  info: 'rgba(37, 99, 235, 0.65)',
  debug: 'rgba(107, 114, 128, 0.5)',
};

function copyToClipboard(text: string): void {
  navigator.clipboard.writeText(text).catch(() => {});
}

function formattedJson(log: LogEntry): string {
  return JSON.stringify({ message: log.message, ...log.fields }, null, 2);
}

function formatTimestamp(date: Date): string {
  return date.toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

/** The moment a line arrived, to the millisecond, in the reader's own zone.
 *  Reading it back as UTC put the detail panel a whole offset away from the
 *  timestamp column next to it, on the same line of the same log. */
function formatTimestampFull(date: Date): string {
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
    hour12: false,
    timeZoneName: 'short',
  });
}

function fieldEntries(log: LogEntry): { key: string; value: string }[] {
  return Object.entries(log.fields).map(([key, value]) => ({
    key,
    value: typeof value === 'object' ? JSON.stringify(value) : String(value),
  }));
}

@Component({
  selector: 'app-log-explorer',
  imports: [FormsModule, DecimalPipe],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './log-explorer.component.html',
  styles: `
    #filter-trigger {
      display: none;
    }

    nldd-sidebar-section[collapsed] #filter-trigger:not([hidden]) {
      display: inline-flex;
    }
  `,
})
export default class LogExplorerComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly titleService = inject(TitleService);

  private readonly notificationService = inject(NotificationService);

  private readonly logsApi = inject(LogsApiService);

  private readonly pluginInstallations = inject(PluginInstallationService);

  private readonly shootPods = inject(ShootPodsService);

  /* The chart block is only rendered while there are entries, so the canvas is
     created and destroyed as you filter. A setter rather than a plain @ViewChild:
     it is the moment the element appears, which is the only moment Chart.js can
     be pointed at it. */
  @ViewChild('histogramChart')
  private set histogramCanvasRef(ref: ElementRef<HTMLCanvasElement> | undefined) {
    this.histogramCanvas = ref;
    if (ref) this.createHistogram();
    else {
      this.histogram?.destroy();
      this.histogram = null;
    }
  }

  private histogramCanvas?: ElementRef<HTMLCanvasElement>;

  @ViewChild('detailSheet') private detailSheetRef?: ElementRef<HTMLElement>;

  @ViewChild('filterSection') private filterSectionRef?: ElementRef<HTMLElement>;

  private histogram: Chart | null = null;

  private liveTailSub: Subscription | null = null;

  private liveTailRateInterval: ReturnType<typeof setInterval> | null = null;

  private liveTailReceived = 0;

  private searchDebounce: ReturnType<typeof setTimeout> | null = null;

  private readonly LOG_LIMIT = 2000;

  // ── static UI data
  readonly ALL_LEVELS = ALL_LEVELS;

  readonly TIME_PRESETS = TIME_PRESETS;

  // ── log source mode
  //
  // 'vali' reads the shoot's system logs from the Gardener logging stack
  // (history, cross-pod search); 'live' reads a single plugin pod through the
  // kube-api-proxy (live only, pod required, per-user access). The two behave
  // differently by design — the tab bar makes that explicit up front, and the
  // page text under it says what the picked source can and cannot answer.
  readonly sourceMode = signal<'vali' | 'live'>('vali');

  readonly isLiveMode = computed(() => this.sourceMode() === 'live');

  /** The source mode as the backend expresses it. */
  readonly requestedSource = computed(() =>
    this.isLiveMode() ? LogSource.PLUGIN : LogSource.CLUSTER,
  );

  /**
   * The severity selection to send. The backend applies it before the entry
   * limit, which is the only place it can be applied correctly — filtering a
   * page that was already truncated to the newest N lines reported "no errors"
   * on any namespace that logs mostly INFO.
   */
  readonly requestedLevels = computed(() => [...this.selectedLevels()]);

  // Pods (with their containers) of the selected plugin namespace, listed
  // through the kube-api-proxy in live mode.
  private readonly livePods = signal<ShootPod[]>([]);

  // ── filter state
  readonly selectedCluster = signal('');

  readonly selectedNamespace = signal('');

  readonly selectedPod = signal('');

  readonly selectedContainer = signal('');

  readonly selectedLevels = signal<Set<LogLevel>>(new Set(ALL_LEVELS));

  readonly searchText = signal('');

  readonly timePreset = signal('1h');

  readonly sortNewestFirst = signal(true);

  readonly currentPage = signal(0);

  // ── live tail
  readonly liveTailEnabled = signal(false);

  readonly liveTailPaused = signal(false);

  readonly liveTailRate = signal(0);

  // ── detail panel
  readonly selectedLog = signal<LogEntry | null>(null);

  readonly showRawJson = signal(false);

  // ── all log data (loaded from the backend for the selected cluster)
  private readonly allLogs = signal<LogEntry[]>([]);

  /** Nothing came back at all, which is a different nothing from a search that
   *  matched none of what did: only the second one is about the filters. */
  readonly hasNoLogs = computed(() => this.allLogs().length === 0);

  // ── request state
  readonly isLoading = signal(false);

  readonly loadError = signal(false);

  readonly backend = signal<LogBackend>(LogBackend.UNSPECIFIED);

  readonly isFallback = computed(() => this.backend() === LogBackend.KUBERNETES);

  readonly isNoBackend = computed(() => this.backend() === LogBackend.NONE);

  // ── filter options (sourced from the backend label values, not the loaded logs)
  protected pageNav = inject(PageNavService);

  /* True while the filter column is a sheet. The required choices then ride
     along as tokens above the list, because a sheet you have to open first is a
     poor place for the one field that decides whether there is anything to
     read at all. The section announces the flip; the first state is read off it
     once the view is up, since a section that starts wide never flips. */
  readonly filtersCollapsed = signal(false);

  readonly levelLabel = levelLabel;

  readonly levelTagColor = levelTagColor;

  /* A filter menu is shut most of the time, so each button says what it is
     filtering on. Without that the toolbar is five words that never change and
     you have to open all five to see where you are. */
  readonly clusterLabel = computed(
    () => this.clusters().find((c) => c.id === this.selectedCluster())?.name ?? 'Cluster',
  );

  readonly scopeLabel = computed(
    () => this.selectedNamespace() || (this.isLiveMode() ? 'Select a plugin' : 'All namespaces'),
  );

  readonly podLabel = computed(
    () => this.selectedPod() || (this.isLiveMode() ? 'Select a pod' : 'All pods'),
  );

  readonly containerLabel = computed(() => this.selectedContainer() || 'All containers');

  /**
   * The total beside "All levels", and only while the query asked for every
   * level: with a subset selected the other levels are absent from the result
   * set entirely, so a sum over what came back would count a part and call it
   * the whole. Same reason a single level's count goes blank.
   */
  readonly allLevelsCountLabel = computed(() => {
    if (this.selectedLevels().size !== ALL_LEVELS.length) return '';
    const counts = this.levelCounts();
    return ALL_LEVELS.reduce((total, level) => total + counts[level], 0).toLocaleString();
  });

  readonly levelsLabel = computed(() => {
    const chosen = ALL_LEVELS.filter((level) => this.selectedLevels().has(level));
    if (chosen.length === ALL_LEVELS.length) return 'All levels';
    if (chosen.length === 0) return 'No levels';
    if (chosen.length <= 2) return chosen.map((level) => LEVEL_LABEL[level]).join(', ');
    return `${chosen.length} levels`;
  });

  readonly timeLabel = computed(
    () => TIME_PRESETS.find((p) => p.value === this.timePreset())?.label ?? 'Time range',
  );

  readonly clusters = signal<ClusterOption[]>([]);

  readonly namespaces = signal<string[]>([]);

  readonly pods = signal<string[]>([]);

  readonly containers = signal<string[]>([]);

  // ── time range
  // `new Date()` is not reactive, so the window has to hang off a signal:
  // without this the range would freeze at first evaluation and every entry
  // arriving later (tail, or a page left open) would fall outside it.
  private readonly windowAnchor = signal(Date.now());

  private readonly timeRange = computed((): { from: Date; to: Date } => {
    const now = new Date(this.windowAnchor());
    const preset = TIME_PRESETS.find((p) => p.value === this.timePreset());
    const minutes = preset?.minutes ?? 60;
    return { from: new Date(now.getTime() - minutes * 60 * 1000), to: now };
  });

  // ── filtered logs without level filter (the base for the list, and the
  //    fallback the chip counts use when the histogram did not come back)
  private readonly filteredLogsNoLevel = computed(() => {
    const { from, to } = this.timeRange();
    // Tailed entries carry a server timestamp of "now", which is at or past
    // the anchor (and past it outright under clock skew), so an upper bound
    // would filter out exactly what the tail delivers.
    const upper = this.liveTailEnabled() ? null : to;
    const cl = this.selectedCluster();
    const ns = this.selectedNamespace();
    const pod = this.selectedPod();
    const container = this.selectedContainer();
    const search = this.searchText().toLowerCase();
    return this.allLogs().filter(
      (l) =>
        l.timestamp >= from &&
        (upper === null || l.timestamp <= upper) &&
        (!cl || l.cluster === cl) &&
        (!ns || l.namespace === ns) &&
        (!pod || l.pod === pod) &&
        (!container || l.container === container) &&
        (!search ||
          l.message.toLowerCase().includes(search) ||
          l.pod.toLowerCase().includes(search)),
    );
  });

  /**
   * The count to show on a severity chip, or "" when it is not known.
   *
   * The backend applies the severity filter, so an unselected level is absent
   * from the result set entirely — its count would render as a confident zero
   * that says nothing about the cluster. A number is only shown for levels the
   * current query actually asked for.
   */
  levelCountLabel(level: LogLevel): string {
    if (!this.selectedLevels().has(level)) {
      return '';
    }
    return this.levelCounts()[level].toLocaleString();
  }

  /**
   * Counts for the severity chips.
   *
   * Taken from the histogram whenever there is one, rather than from the
   * fetched page: the page stops at LOG_LIMIT while the chart directly above
   * counts the whole window, and a chip reading "2,000" under bars that sum to
   * forty thousand is worse than either number on its own. The page is only
   * the fallback for when the counts did not come back — which is also when
   * the chart is hidden, so the two are never on screen disagreeing.
   */
  readonly levelCounts = computed(() => {
    const buckets = this.histogramBuckets();
    if (buckets.length > 0) {
      return buckets.reduce(
        (total, b) => ({
          ERROR: total.ERROR + b.error,
          WARN: total.WARN + b.warn,
          INFO: total.INFO + b.info,
          DEBUG: total.DEBUG + b.debug,
        }),
        { ERROR: 0, WARN: 0, INFO: 0, DEBUG: 0 },
      );
    }
    const logs = this.filteredLogsNoLevel();
    return {
      ERROR: logs.filter((l) => l.level === 'ERROR').length,
      WARN: logs.filter((l) => l.level === 'WARN').length,
      INFO: logs.filter((l) => l.level === 'INFO').length,
      DEBUG: logs.filter((l) => l.level === 'DEBUG').length,
    };
  });

  // ── fully filtered logs (with level filter)
  readonly filteredLogs = computed(() => {
    const levels = this.selectedLevels();
    const logs =
      levels.size === 0
        ? this.filteredLogsNoLevel()
        : this.filteredLogsNoLevel().filter((l) => levels.has(l.level));
    return this.sortNewestFirst()
      ? [...logs].sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime())
      : [...logs].sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime());
  });

  readonly pagedLogs = computed(() => {
    const start = this.currentPage() * PAGE_SIZE;
    return this.filteredLogs().slice(start, start + PAGE_SIZE);
  });

  readonly totalPages = computed(() =>
    Math.max(1, Math.ceil(this.filteredLogs().length / PAGE_SIZE)),
  );

  // ── generated query string for the query bar
  readonly generatedQuery = computed(() => {
    // Two lists from the start, rather than one flat list taken apart again
    // afterwards: the old reassembly sliced off the final element before
    // filtering, so with a namespace and a pod selected it silently dropped the
    // pod matcher from the query it displayed.
    const matchers: string[] = [];
    if (this.selectedNamespace()) matchers.push(`namespace="${this.selectedNamespace()}"`);
    // Exact, matching the matcher the backend actually sends.
    if (this.selectedPod()) matchers.push(`pod="${this.selectedPod()}"`);
    if (this.selectedContainer()) matchers.push(`container="${this.selectedContainer()}"`);
    const levels = this.selectedLevels();
    if (levels.size > 0 && levels.size < ALL_LEVELS.length) {
      matchers.push(`level=~"${[...levels].join('|')}"`);
    }

    const pipeline: string[] = [];
    const search = this.searchText();
    if (search) pipeline.push(`|~ "(?i)${search}"`);

    return `{${matchers.join(', ')}}${pipeline.length ? ` ${pipeline.join(' ')}` : ''}`;
  });

  // ── histogram
  //
  // Counted by the backend over the whole window, not reduced from the entry
  // page: the page is the newest LOG_LIMIT lines, so counting it here pinned
  // every total at the limit and drew a cliff wherever the page happened to
  // start — a shape indistinguishable from traffic actually falling off.
  readonly histogramBuckets = signal<HistogramBucket[]>([]);

  /** False when the counts came from a bounded page because the backend
   *  cannot aggregate (Kubernetes pod logs, plugin logs). */
  readonly histogramExact = signal(true);

  private readonly HISTOGRAM_BUCKETS = 30;

  /**
   * How many one-second tail ticks pass between histogram refreshes. Every
   * refresh is a backend round trip, and thirty buckets of a sliding window
   * cannot show a one-second change anyway.
   */
  private readonly LIVE_HISTOGRAM_REFRESH_TICKS = 5;

  /**
   * Which load the answers on screen belong to.
   *
   * A load is two independent requests, the entries and the counts, and a
   * filter change can start a new pair before the last one has landed. Without
   * a ticket a slow response overwrites a fast newer one — and worse, a late
   * entry list can pair with an early histogram, leaving the chart describing
   * a different filter than the lines underneath it.
   */
  private loadSeq = 0;

  /**
   * Axis labels for the buckets. A window wider than a day needs the date:
   * now that the chart really does span the whole range, twelve identical
   * "08:00" ticks would be seven different days.
   */
  readonly histogramLabels = computed((): string[] => {
    const buckets = this.histogramBuckets();
    if (buckets.length === 0) return [];
    const spanMs = buckets[buckets.length - 1].start.getTime() - buckets[0].start.getTime();
    const multiDay = spanMs > 24 * 60 * 60 * 1000;
    return buckets.map((b) =>
      multiDay
        ? b.start.toLocaleString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
            hour12: false,
          })
        : b.start.toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            hour12: false,
          }),
    );
  });

  // ── selected log index for prev/next navigation

  // ── all active filter chips (for display). Cluster is always selected, so it
  // is shown in the dropdown rather than as a removable chip.
  readonly activeFilterChips = computed(() => {
    const chips: { label: string; key: string }[] = [];
    if (this.selectedNamespace())
      chips.push({ label: `Namespace: ${this.selectedNamespace()}`, key: 'namespace' });
    if (this.selectedPod()) chips.push({ label: `Pod: ${this.selectedPod()}`, key: 'pod' });
    if (this.selectedContainer())
      chips.push({ label: `Container: ${this.selectedContainer()}`, key: 'container' });
    // A level narrows the query as much as a namespace does, so it belongs in
    // the same row. All levels is the absence of a filter rather than one, and
    // each chosen level gets its own token: you drop them one at a time.
    if (this.selectedLevels().size !== ALL_LEVELS.length) {
      chips.push(
        ...ALL_LEVELS.filter((level) => this.selectedLevels().has(level)).map((level) => ({
          label: `Level: ${LEVEL_LABEL[level]}`,
          key: `level:${level}`,
        })),
      );
    }
    return chips;
  });

  readonly isAllLevelsSelected = computed(() => this.selectedLevels().size === ALL_LEVELS.length);

  constructor() {
    this.titleService.setTitle('Log explorer');

    effect(() => {
      const buckets = this.histogramBuckets();
      if (this.histogram) {
        this.histogram.data.labels = this.histogramLabels();
        this.histogram.data.datasets[0].data = buckets.map((b) => b.error);
        this.histogram.data.datasets[1].data = buckets.map((b) => b.warn);
        this.histogram.data.datasets[2].data = buckets.map((b) => b.info);
        this.histogram.data.datasets[3].data = buckets.map((b) => b.debug);
        this.histogram.update('none');
      }
    });
  }

  async ngAfterViewInit(): Promise<void> {
    // The section seeds its collapsed state on its own first render, which is
    // after ours, and it stays quiet about that first value: the event only
    // carries later flips. So wait for the element to render, then read it once.
    await customElements.whenDefined('nldd-sidebar-section');
    const section = this.filterSectionRef?.nativeElement as
      (HTMLElement & { updateComplete?: Promise<unknown> }) | undefined;
    await section?.updateComplete;
    this.filtersCollapsed.set(section?.hasAttribute('collapsed') ?? false);
  }

  async ngOnInit(): Promise<void> {
    try {
      const clusters = await this.logsApi.listClusters();
      this.clusters.set(clusters);
      if (clusters.length > 0) {
        this.selectedCluster.set(clusters[0].id);
        await this.onClusterSelected();
      }
    } catch {
      this.loadError.set(true);
    }
  }

  ngOnDestroy(): void {
    this.stopLiveTail();
    if (this.searchDebounce !== null) clearTimeout(this.searchDebounce);
    this.histogram?.destroy();
  }

  // ── data loading
  clusterName(id: string): string {
    return this.clusters().find((c) => c.id === id)?.name ?? id;
  }

  private async onClusterSelected(): Promise<void> {
    const clusterId = this.selectedCluster();
    if (!clusterId) return;
    if (this.isLiveMode()) {
      await this.loadPluginNamespaces();
      await this.loadLogs();
      return;
    }
    try {
      const { from, to } = this.timeRange();
      const labels = await this.logsApi.labels(clusterId, undefined, from, to);
      this.backend.set(labels.backend);
      this.namespaces.set(labels.namespaces);
      this.pods.set(labels.pods);
      this.containers.set(labels.containers);
    } catch {
      // Labels are best-effort; a failure should not block log loading.
    }
    await this.loadLogs();
  }

  // ── live mode (plugin pods through the kube-api-proxy)

  onSourceModeChange(mode: 'vali' | 'live'): void {
    if (mode === this.sourceMode()) return;
    this.sourceMode.set(mode);
    this.stopLiveTail();
    this.selectedNamespace.set('');
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.currentPage.set(0);
    this.allLogs.set([]);
    this.namespaces.set([]);
    this.pods.set([]);
    this.containers.set([]);
    this.livePods.set([]);
    this.onClusterSelected();
  }

  // Plugin namespaces are derived, not listed: every PluginInstallation named
  // <name> runs in namespace "plugin-<name>" (plugin-controller convention).
  // Deriving from the installations the caller can see avoids listing all
  // shoot namespaces, which would drag user workloads into the dropdown.
  private async loadPluginNamespaces(): Promise<void> {
    const clusterId = this.selectedCluster();
    if (!clusterId) return;
    try {
      const installations = await this.pluginInstallations.listInstallations(clusterId);
      this.namespaces.set(installations.map((i) => `plugin-${i.metadata.name}`));
    } catch {
      this.namespaces.set([]);
    }
  }

  private async loadLivePods(): Promise<void> {
    const clusterId = this.selectedCluster();
    const namespace = this.selectedNamespace();
    if (!clusterId || !namespace) {
      this.livePods.set([]);
      this.pods.set([]);
      this.containers.set([]);
      return;
    }
    try {
      const pods = await this.shootPods.listPods(clusterId, namespace);
      this.livePods.set(pods);
      this.pods.set(pods.map((p) => p.name));
      this.containers.set([]);
    } catch {
      this.livePods.set([]);
      this.pods.set([]);
      this.containers.set([]);
    }
  }

  private async refineLabels(): Promise<void> {
    const clusterId = this.selectedCluster();
    if (!clusterId) return;
    try {
      const { from, to } = this.timeRange();
      const labels = await this.logsApi.labels(
        clusterId,
        this.selectedNamespace() || undefined,
        from,
        to,
      );
      // The namespace list is unscoped in the response, so it can refresh
      // here too (it is time-scoped, like all label values).
      this.namespaces.set(labels.namespaces);
      this.pods.set(labels.pods);
      this.containers.set(labels.containers);
    } catch {
      // best-effort
    }
  }

  private async loadLogs(): Promise<void> {
    this.loadSeq += 1;
    const seq = this.loadSeq;
    const clusterId = this.selectedCluster();
    if (!clusterId) {
      this.clearLogs();
      return;
    }
    // Live mode reads one pod at a time (Kubernetes pod-log semantics), so a
    // namespace + pod selection is a hard requirement before querying.
    if (this.isLiveMode() && (!this.selectedNamespace() || !this.selectedPod())) {
      this.clearLogs();
      return;
    }
    // The Kubernetes fallback can only read a single pod, so require one.
    if (this.isFallback() && (!this.selectedNamespace() || !this.selectedPod())) {
      this.clearLogs();
      return;
    }

    this.isLoading.set(true);
    this.loadError.set(false);
    try {
      // Re-anchor so a page left open queries "the last hour" from now, not
      // from when it was opened.
      this.windowAnchor.set(Date.now());
      const { from, to } = this.timeRange();
      const result = await this.logsApi.query({
        clusterId,
        namespace: this.selectedNamespace() || undefined,
        pod: this.selectedPod() || undefined,
        container: this.selectedContainer() || undefined,
        search: this.searchText() || undefined,
        levels: this.requestedLevels(),
        source: this.requestedSource(),
        from,
        to,
        limit: this.LOG_LIMIT,
      });
      if (seq !== this.loadSeq) return;
      this.backend.set(result.backend);
      this.allLogs.set(result.entries);
      this.currentPage.set(0);
      // The chart is context for the lines, so it is not allowed to hold them
      // up: the page is done here, and the counts land on their own ticket.
      this.isLoading.set(false);
      this.refreshHistogram(from, to, seq);
    } catch {
      if (seq !== this.loadSeq) return;
      this.loadError.set(true);
      // Voids the histogram still in flight for this load: a chart above an
      // empty, errored list would be describing logs the page cannot show.
      this.clearLogs();
      this.isLoading.set(false);
    }
  }

  /**
   * Puts counts under the chart for the window the entry list just queried.
   *
   * A backend that can aggregate is asked; one that cannot would answer by
   * re-reading the pod's log and counting the page it got back — the same page
   * this component is already holding — so that page is counted here instead.
   * It saves a second read of the same log per filter change, and it makes the
   * chart and the list provably the same lines rather than two reads that
   * happened to be taken a moment apart.
   */
  private refreshHistogram(from: Date, to: Date, seq: number): void {
    if (!this.isFallback()) {
      this.loadHistogram(from, to, seq).catch(() => {
        // loadHistogram reports its own failures by dropping the chart.
      });
      return;
    }
    this.histogramBuckets.set(this.bucketFetchedEntries(from, to));
    this.histogramExact.set(false);
  }

  /**
   * Buckets the entries already on the page, laid out exactly as the backend
   * lays its own out: HISTOGRAM_BUCKETS equal divisions of the window, empty
   * ones included, so a quiet stretch draws as a zero and not as a gap.
   *
   * Only ever used for counts that are a page rather than a window, which is
   * what `histogramExact` is false for and what the caption says out loud.
   */
  private bucketFetchedEntries(from: Date, to: Date): HistogramBucket[] {
    const width = (to.getTime() - from.getTime()) / this.HISTOGRAM_BUCKETS;
    const buckets: HistogramBucket[] = Array.from({ length: this.HISTOGRAM_BUCKETS }, (_, i) => ({
      start: new Date(from.getTime() + i * width),
      error: 0,
      warn: 0,
      info: 0,
      debug: 0,
    }));
    if (width <= 0) return buckets;
    this.filteredLogs().forEach((log) => {
      // Clamped rather than dropped: a live tail carries server timestamps a
      // moment past the anchor, and those lines are in the list, so leaving
      // them out of the chart would understate exactly the edge being watched.
      const idx = Math.min(
        Math.max(Math.floor((log.timestamp.getTime() - from.getTime()) / width), 0),
        this.HISTOGRAM_BUCKETS - 1,
      );
      buckets[idx][log.level.toLowerCase() as 'error' | 'warn' | 'info' | 'debug'] += 1;
    });
    return buckets;
  }

  /**
   * Fetches the bucketed counts for the window the entry list is querying.
   *
   * Kept separate from the entry query and never allowed to fail the page: the
   * chart is context for the lines, so a backend that can serve lines but not
   * aggregates should still show you your logs. `seq` is the load it belongs
   * to; an answer for a load that has been superseded is dropped rather than
   * painted over the current one.
   */
  private async loadHistogram(from: Date, to: Date, seq: number): Promise<void> {
    const clusterId = this.selectedCluster();
    if (!clusterId) return;
    try {
      const result = await this.logsApi.histogram({
        clusterId,
        namespace: this.selectedNamespace() || undefined,
        pod: this.selectedPod() || undefined,
        container: this.selectedContainer() || undefined,
        search: this.searchText() || undefined,
        levels: this.requestedLevels(),
        source: this.requestedSource(),
        from,
        to,
        buckets: this.HISTOGRAM_BUCKETS,
        // A backend that cannot aggregate counts a page instead. Handing it
        // the entry query's limit keeps that page the one the list is
        // showing, rather than a second, larger read of the same pod log.
        limit: this.LOG_LIMIT,
      });
      if (seq !== this.loadSeq) return;
      this.histogramBuckets.set(result.buckets);
      this.histogramExact.set(result.exact);
    } catch {
      if (seq !== this.loadSeq) return;
      // An empty chart above a full list reads as "no traffic", which is the
      // opposite of what happened, so drop the chart instead.
      this.histogramBuckets.set([]);
      this.histogramExact.set(true);
    }
  }

  /**
   * Drop what is on screen and void what is still in flight: the answers to a
   * load we have given up on must not arrive later and repopulate half a page.
   */
  private clearLogs(): void {
    this.loadSeq += 1;
    this.allLogs.set([]);
    this.histogramBuckets.set([]);
    this.histogramExact.set(true);
  }

  // ── chart
  private createHistogram(): void {
    const buckets = this.histogramBuckets();
    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: this.histogramLabels(),
        datasets: [
          {
            label: 'Error',
            data: buckets.map((b) => b.error),
            backgroundColor: HISTOGRAM_COLORS['error'],
            stack: 'logs',
          },
          {
            label: 'Warn',
            data: buckets.map((b) => b.warn),
            backgroundColor: HISTOGRAM_COLORS['warn'],
            stack: 'logs',
          },
          {
            label: 'Info',
            data: buckets.map((b) => b.info),
            backgroundColor: HISTOGRAM_COLORS['info'],
            stack: 'logs',
          },
          {
            label: 'Debug',
            data: buckets.map((b) => b.debug),
            backgroundColor: HISTOGRAM_COLORS['debug'],
            stack: 'logs',
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              title: (items) => `Time: ${items[0].label}`,
            },
          },
        },
        scales: {
          x: {
            stacked: true,
            ticks: {
              maxTicksLimit: 8,
              maxRotation: 0,
              color: '#6b7280',
              font: { size: 11 },
            },
            grid: { display: false },
          },
          y: {
            stacked: true,
            beginAtZero: true,
            ticks: { color: '#6b7280', font: { size: 11 } },
            grid: { color: 'rgba(107,114,128,0.15)' },
          },
        },
      },
    };
    const canvas = this.histogramCanvas?.nativeElement;
    if (!canvas) return;
    this.histogram?.destroy();
    this.histogram = new Chart(canvas, config);
  }

  // ── live tail
  /* The stream is one control with three states, so the menu sets a state
     instead of firing three unrelated verbs at it, and the button label reports
     which one you are in. */
  readonly streamState = computed<'off' | 'streaming' | 'paused'>(() => {
    if (!this.liveTailEnabled()) return 'off';
    return this.liveTailPaused() ? 'paused' : 'streaming';
  });

  readonly streamLabel = computed(() => {
    const state = this.streamState();
    if (state === 'off') return 'Not streaming';
    if (state === 'paused') return 'Stream paused';
    return `Streaming ${this.liveTailRate()}/s`;
  });

  setStreamState(state: 'off' | 'streaming' | 'paused'): void {
    if (state === 'off') {
      if (this.liveTailEnabled()) this.stopLiveTail();
      return;
    }
    if (!this.liveTailEnabled()) this.startLiveTail();
    this.liveTailPaused.set(state === 'paused');
  }

  toggleLiveTail(): void {
    if (this.liveTailEnabled()) {
      this.stopLiveTail();
    } else {
      this.startLiveTail();
    }
  }

  pauseLiveTail(): void {
    this.liveTailPaused.set(true);
  }

  resumeLiveTail(): void {
    this.liveTailPaused.set(false);
  }

  private startLiveTail(): void {
    const clusterId = this.selectedCluster();
    if (!clusterId) return;
    this.liveTailEnabled.set(true);
    this.liveTailPaused.set(false);
    this.liveTailReceived = 0;
    this.liveTailRate.set(0);
    this.currentPage.set(0);

    let ticksSinceHistogram = 0;
    this.liveTailRateInterval = setInterval(() => {
      this.liveTailRate.set(this.liveTailReceived);
      this.liveTailReceived = 0;
      // Slide the window with the clock so the lower bound keeps up with a
      // long-running tail.
      this.windowAnchor.set(Date.now());
      // The counts do not slide with it — they are a server answer, fetched
      // once per load — so without this the chart would keep describing the
      // window it was loaded with while the list moved out from under it.
      ticksSinceHistogram += 1;
      // Counting the page costs nothing, so it keeps pace with the tail; a
      // fetch is a round trip, so it goes at the slower rate.
      if (this.isFallback() || ticksSinceHistogram >= this.LIVE_HISTOGRAM_REFRESH_TICKS) {
        ticksSinceHistogram = 0;
        const { from, to } = this.timeRange();
        this.refreshHistogram(from, to, this.loadSeq);
      }
    }, 1000);

    this.liveTailSub = this.logsApi
      .tail({
        clusterId,
        namespace: this.selectedNamespace() || undefined,
        pod: this.selectedPod() || undefined,
        container: this.selectedContainer() || undefined,
        search: this.searchText() || undefined,
        levels: this.requestedLevels(),
        source: this.requestedSource(),
      })
      .subscribe({
        next: (entry) => {
          if (this.liveTailPaused()) return;
          this.liveTailReceived += 1;
          this.allLogs.update((logs) => [entry, ...logs].slice(0, this.LOG_LIMIT));
        },
        error: () => {
          this.notificationService.error('Live tail disconnected');
          this.stopLiveTail();
        },
        // A server-side stream that ends normally (pod gone, backend closed the
        // follow) would otherwise leave the UI claiming it is still streaming.
        complete: () => {
          if (this.liveTailEnabled()) {
            this.notificationService.info('Live tail ended');
            this.stopLiveTail();
          }
        },
      });
  }

  /**
   * Reopen the tail so its server-side filters match the current selection.
   *
   * The stream carries namespace, pod, container, search, levels and source, so
   * leaving it untouched after a filter change means the server keeps applying
   * the *previous* query: entries matching the old filter keep arriving and
   * evict the rows just fetched for the new one, while the status bar still
   * reads "Streaming".
   */
  private restartLiveTailIfRunning(): void {
    if (!this.liveTailEnabled()) return;
    this.stopLiveTail();
    this.startLiveTail();
  }

  private stopLiveTail(): void {
    this.liveTailSub?.unsubscribe();
    this.liveTailSub = null;
    if (this.liveTailRateInterval !== null) {
      clearInterval(this.liveTailRateInterval);
      this.liveTailRateInterval = null;
    }
    this.liveTailEnabled.set(false);
    this.liveTailPaused.set(false);
    this.liveTailRate.set(0);
  }

  /** Re-query and realign a running tail after a filter change. */
  private reloadForFilterChange(): void {
    this.loadLogs();
    this.restartLiveTailIfRunning();
  }

  // ── filter actions
  onClusterChange(value: string): void {
    this.selectedCluster.set(value);
    this.selectedNamespace.set('');
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.currentPage.set(0);
    this.stopLiveTail();
    this.onClusterSelected();
  }

  onNamespaceChange(value: string): void {
    this.selectedNamespace.set(value);
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.currentPage.set(0);
    if (this.isLiveMode()) {
      this.loadLivePods();
    } else {
      this.refineLabels();
    }
    this.reloadForFilterChange();
  }

  onPodChange(value: string): void {
    this.selectedPod.set(value);
    this.selectedContainer.set('');
    this.currentPage.set(0);
    if (this.isLiveMode()) {
      const pod = this.livePods().find((p) => p.name === value);
      this.containers.set(pod?.containers ?? []);
    }
    this.reloadForFilterChange();
  }

  onContainerChange(value: string): void {
    this.selectedContainer.set(value);
    this.currentPage.set(0);
    this.reloadForFilterChange();
  }

  onTimePresetChange(value: string): void {
    this.timePreset.set(value);
    this.currentPage.set(0);
    // Label values are time-scoped in the backend, so the filter dropdowns
    // must follow the active window. Live-mode pods come from the cluster
    // itself and are not time-scoped.
    if (!this.isLiveMode()) {
      this.refineLabels();
    }
    this.reloadForFilterChange();
  }

  onSearchChange(value: string): void {
    this.searchText.set(value);
    this.currentPage.set(0);
    if (this.searchDebounce !== null) clearTimeout(this.searchDebounce);
    this.searchDebounce = setTimeout(() => {
      this.reloadForFilterChange();
    }, 400);
  }

  clearAllFilters(): void {
    this.selectedNamespace.set('');
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.searchText.set('');
    this.selectedLevels.set(new Set(ALL_LEVELS));
    this.currentPage.set(0);
    if (this.isLiveMode()) {
      this.pods.set([]);
      this.containers.set([]);
    } else {
      this.refineLabels();
    }
    this.reloadForFilterChange();
  }

  removeChip(key: string): void {
    if (key === 'namespace') {
      this.selectedNamespace.set('');
      this.selectedPod.set('');
      this.selectedContainer.set('');
      if (this.isLiveMode()) {
        this.pods.set([]);
        this.containers.set([]);
      } else {
        this.refineLabels();
      }
    } else if (key === 'pod') {
      this.selectedPod.set('');
      this.selectedContainer.set('');
    } else if (key === 'container') {
      this.selectedContainer.set('');
    } else if (key.startsWith('level:')) {
      const next = new Set(this.selectedLevels());
      next.delete(key.slice('level:'.length) as LogLevel);
      // Nothing left selected is a question with no possible answer, so dropping
      // the last level asks for all of them again.
      this.selectedLevels.set(next.size === 0 ? new Set(ALL_LEVELS) : next);
    }
    this.currentPage.set(0);
    this.reloadForFilterChange();
  }

  toggleAllLevels(): void {
    this.selectedLevels.set(new Set(ALL_LEVELS));
    this.currentPage.set(0);
    // The severity filter is applied by the backend, before the entry limit, so
    // changing it has to re-query. Re-filtering the page already in memory was
    // the whole bug: on an INFO-heavy namespace the first page fills with INFO
    // and selecting ERROR emptied the table while the backend held matches it
    // was never asked for.
    this.reloadForFilterChange();
  }

  /** The menu is a choice, so a click replaces the selection instead of adding
   *  to it. Same reload as the other level moves: the backend applies the
   *  filter, before the entry limit. */
  selectLevel(level: LogLevel): void {
    this.selectedLevels.set(new Set([level]));
    this.currentPage.set(0);
    this.reloadForFilterChange();
  }

  onLevelChoice(value: string): void {
    if (value === 'all') {
      this.toggleAllLevels();
      return;
    }
    this.selectLevel(value as LogLevel);
  }

  toggleLevel(level: LogLevel): void {
    this.selectedLevels.update((levels) => {
      if (levels.size === ALL_LEVELS.length) {
        return new Set([level]);
      }
      const next = new Set(levels);
      if (next.has(level)) {
        next.delete(level);
      } else {
        next.add(level);
      }
      return next;
    });
    this.currentPage.set(0);
    this.reloadForFilterChange();
  }

  isLevelSelected(level: LogLevel): boolean {
    return this.selectedLevels().has(level);
  }

  // ── log detail
  selectLog(log: LogEntry): void {
    this.selectedLog.set(log);
    this.showRawJson.set(false);
    (this.detailSheetRef?.nativeElement as (HTMLElement & { show(): void }) | undefined)?.show();
  }

  closeDetail(): void {
    (this.detailSheetRef?.nativeElement as (HTMLElement & { hide(): void }) | undefined)?.hide();
  }

  onDetailSheetClose(): void {
    this.selectedLog.set(null);
  }

  copyToClipboard(text: string): void {
    copyToClipboard(text);
    this.notificationService.success('Copied to clipboard');
  }

  readonly formattedJson = formattedJson;

  // ── pagination
  onPageChange(event: Event): void {
    this.currentPage.set((event as CustomEvent<{ page: number }>).detail.page - 1);
  }

  onSearchInput(event: Event): void {
    this.onSearchChange((event.target as HTMLInputElement).value);
  }

  // ── style helpers

  levelChipClass(level: LogLevel): string {
    const base =
      'flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium transition-colors cursor-pointer';
    if (this.isLevelSelected(level)) return `${base} ${LEVEL_CHIP_ACTIVE[level]}`;
    return `${base} border-neutral-200 bg-white text-neutral-500 hover:bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-500`;
  }

  allChipClass(): string {
    const base =
      'flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium transition-colors cursor-pointer';
    if (this.isAllLevelsSelected())
      return `${base} border-accent-300 bg-accent-50 text-accent-700 dark:border-accent-700 dark:bg-accent-950 dark:text-accent-300`;
    return `${base} border-neutral-200 bg-white text-neutral-500 hover:bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900`;
  }

  readonly formatTimestamp = formatTimestamp;

  readonly formatTimestampFull = formatTimestampFull;

  readonly fieldEntries = fieldEntries;
}
