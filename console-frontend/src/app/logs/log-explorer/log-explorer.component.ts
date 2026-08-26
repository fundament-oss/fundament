import {
  Component,
  ViewChild,
  ElementRef,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  signal,
  computed,
  effect,
  OnInit,
  OnDestroy,
  AfterViewInit,
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
import { ToastService } from '../../toast.service';
import PluginInstallationService from '../../plugin-installation/plugin-installation.service';

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

const LEVEL_BADGE: Record<LogLevel, string> = {
  ERROR: 'badge badge-sm badge-rose',
  WARN: 'badge badge-sm badge-yellow',
  INFO: 'badge badge-sm badge-blue',
  DEBUG: 'badge badge-sm badge-gray',
};

const LEVEL_CHIP_ACTIVE: Record<LogLevel, string> = {
  ERROR:
    'border-danger-300 bg-danger-50 text-danger-700 dark:border-danger-700 dark:bg-danger-950 dark:text-danger-300',
  WARN: 'border-yellow-300 bg-yellow-50 text-yellow-700 dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-300',
  INFO: 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-950 dark:text-blue-300',
  DEBUG:
    'border-neutral-300 bg-neutral-50 text-neutral-600 dark:border-neutral-600 dark:bg-neutral-900 dark:text-neutral-400',
};

const LEVEL_DOT: Record<LogLevel, string> = {
  ERROR: 'bg-danger-500',
  WARN: 'bg-yellow-500',
  INFO: 'bg-blue-500',
  DEBUG: 'bg-neutral-400',
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

function levelBadgeClass(level: LogLevel): string {
  return LEVEL_BADGE[level];
}

function levelDotClass(level: LogLevel): string {
  return `inline-block h-2 w-2 rounded-full ${LEVEL_DOT[level]}`;
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

function formatTimestampFull(date: Date): string {
  return date.toISOString().replace('T', ' ').replace('Z', ' UTC');
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
})
export default class LogExplorerComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly titleService = inject(TitleService);

  private readonly toastService = inject(ToastService);

  private readonly logsApi = inject(LogsApiService);

  private readonly pluginInstallations = inject(PluginInstallationService);

  private readonly shootPods = inject(ShootPodsService);

  @ViewChild('histogramChart') private histogramCanvas!: ElementRef<HTMLCanvasElement>;

  @ViewChild('detailSheet') private detailSheetRef?: ElementRef<HTMLElement>;

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
  // differently by design — the mode switch makes that explicit up front.
  //
  // TODO(#978 follow-up): the switch and the live-mode hints are provisional
  // (deliberately loud orange) — a proper UI design pass is still pending.
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

  // ── request state
  readonly isLoading = signal(false);

  readonly loadError = signal(false);

  readonly backend = signal<LogBackend>(LogBackend.UNSPECIFIED);

  readonly isFallback = computed(() => this.backend() === LogBackend.KUBERNETES);

  readonly isNoBackend = computed(() => this.backend() === LogBackend.NONE);

  // ── filter options (sourced from the backend label values, not the loaded logs)
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

  // ── filtered logs without level filter (used for counts + histogram)
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

  // ── level counts for chips (within the fetched, already level-filtered set)
  readonly levelCounts = computed(() => {
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

  // ── histogram buckets
  readonly histogramBuckets = computed((): HistogramBucket[] => {
    const { from, to } = this.timeRange();
    const BUCKET_COUNT = 30;
    const bucketMs = (to.getTime() - from.getTime()) / BUCKET_COUNT;

    const buckets: HistogramBucket[] = Array.from({ length: BUCKET_COUNT }, (_, i) => {
      const bucketTime = new Date(from.getTime() + i * bucketMs);
      return {
        label: bucketTime.toLocaleTimeString('en-US', {
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        }),
        error: 0,
        warn: 0,
        info: 0,
        debug: 0,
      };
    });

    this.filteredLogs().forEach((log) => {
      const idx = Math.min(
        Math.floor((log.timestamp.getTime() - from.getTime()) / bucketMs),
        BUCKET_COUNT - 1,
      );
      if (idx >= 0) {
        const key = log.level.toLowerCase() as 'error' | 'warn' | 'info' | 'debug';
        buckets[idx][key] += 1;
      }
    });

    return buckets;
  });

  // ── selected log index for prev/next navigation
  readonly selectedLogIndex = computed(() => {
    const log = this.selectedLog();
    if (!log) return -1;
    return this.pagedLogs().findIndex((l) => l.id === log.id);
  });

  // ── all active filter chips (for display). Cluster is always selected, so it
  // is shown in the dropdown rather than as a removable chip.
  readonly activeFilterChips = computed(() => {
    const chips: { label: string; key: string }[] = [];
    if (this.selectedNamespace())
      chips.push({ label: `namespace: ${this.selectedNamespace()}`, key: 'namespace' });
    if (this.selectedPod()) chips.push({ label: `pod: ${this.selectedPod()}`, key: 'pod' });
    if (this.selectedContainer())
      chips.push({ label: `container: ${this.selectedContainer()}`, key: 'container' });
    return chips;
  });

  readonly isAllLevelsSelected = computed(() => this.selectedLevels().size === ALL_LEVELS.length);

  constructor() {
    this.titleService.setTitle('Log explorer');

    effect(() => {
      const buckets = this.histogramBuckets();
      if (this.histogram) {
        this.histogram.data.labels = buckets.map((b) => b.label);
        this.histogram.data.datasets[0].data = buckets.map((b) => b.error);
        this.histogram.data.datasets[1].data = buckets.map((b) => b.warn);
        this.histogram.data.datasets[2].data = buckets.map((b) => b.info);
        this.histogram.data.datasets[3].data = buckets.map((b) => b.debug);
        this.histogram.update('none');
      }
    });
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

  ngAfterViewInit(): void {
    this.createHistogram();
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
    const clusterId = this.selectedCluster();
    if (!clusterId) {
      this.allLogs.set([]);
      return;
    }
    // Live mode reads one pod at a time (Kubernetes pod-log semantics), so a
    // namespace + pod selection is a hard requirement before querying.
    if (this.isLiveMode() && (!this.selectedNamespace() || !this.selectedPod())) {
      this.allLogs.set([]);
      return;
    }
    // The Kubernetes fallback can only read a single pod, so require one.
    if (this.isFallback() && (!this.selectedNamespace() || !this.selectedPod())) {
      this.allLogs.set([]);
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
      this.backend.set(result.backend);
      this.allLogs.set(result.entries);
      this.currentPage.set(0);
    } catch {
      this.loadError.set(true);
      this.allLogs.set([]);
    } finally {
      this.isLoading.set(false);
    }
  }

  // ── chart
  private createHistogram(): void {
    const buckets = this.histogramBuckets();
    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: buckets.map((b) => b.label),
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
    this.histogram = new Chart(this.histogramCanvas.nativeElement, config);
  }

  // ── live tail
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

    this.liveTailRateInterval = setInterval(() => {
      this.liveTailRate.set(this.liveTailReceived);
      this.liveTailReceived = 0;
      // Slide the window with the clock so the histogram and the lower bound
      // keep up with a long-running tail.
      this.windowAnchor.set(Date.now());
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
          this.toastService.error('Live tail disconnected');
          this.stopLiveTail();
        },
        // A server-side stream that ends normally (pod gone, backend closed the
        // follow) would otherwise leave the UI claiming it is still streaming.
        complete: () => {
          if (this.liveTailEnabled()) {
            this.toastService.info('Live tail ended');
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

  navigateDetail(direction: -1 | 1): void {
    const idx = this.selectedLogIndex();
    const logs = this.pagedLogs();
    const next = logs[idx + direction];
    if (next) {
      this.selectedLog.set(next);
      this.showRawJson.set(false);
    }
  }

  copyToClipboard(text: string): void {
    copyToClipboard(text);
    this.toastService.success('Copied to clipboard');
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
  readonly levelBadgeClass = levelBadgeClass;

  levelChipClass(level: LogLevel): string {
    const base =
      'flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium transition-colors cursor-pointer';
    if (this.isLevelSelected(level)) return `${base} ${LEVEL_CHIP_ACTIVE[level]}`;
    return `${base} border-neutral-200 bg-white text-neutral-500 hover:bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-500`;
  }

  readonly levelDotClass = levelDotClass;

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
