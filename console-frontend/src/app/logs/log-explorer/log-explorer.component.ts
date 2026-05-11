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
  OnDestroy,
  AfterViewInit,
} from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import type { LogEntry, LogLevel, HistogramBucket } from '../log.types';
import { generateMockLogs, generateLiveTailEntry } from '../log-mock-data';
import { TitleService } from '../../title.service';
import { ToastService } from '../../toast.service';

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

const LEVEL_ROW_BORDER: Record<LogLevel, string> = {
  ERROR: 'border-l-[3px] border-l-danger-500',
  WARN: 'border-l-[3px] border-l-yellow-500',
  INFO: 'border-l-[3px] border-l-blue-500',
  DEBUG: 'border-l-[3px] border-l-neutral-300',
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

function levelRowBorderClass(level: LogLevel): string {
  return LEVEL_ROW_BORDER[level];
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
  imports: [FormsModule, DecimalPipe, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './log-explorer.component.html',
})
export default class LogExplorerComponent implements AfterViewInit, OnDestroy {
  private readonly titleService = inject(TitleService);

  private readonly toastService = inject(ToastService);

  @ViewChild('histogramChart') private histogramCanvas!: ElementRef<HTMLCanvasElement>;

  @ViewChild('detailSheet') private detailSheetRef?: ElementRef<HTMLElement>;

  private histogram: Chart | null = null;

  private liveTailInterval: ReturnType<typeof setInterval> | null = null;

  // ── static UI data
  readonly ALL_LEVELS = ALL_LEVELS;

  readonly TIME_PRESETS = TIME_PRESETS;

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

  // ── all log data
  private readonly allLogs = signal<LogEntry[]>(generateMockLogs(300));

  // ── derived: available filter options
  readonly clusters = computed(() => [...new Set(this.allLogs().map((l) => l.cluster))].sort());

  readonly namespaces = computed(() => {
    const cl = this.selectedCluster();
    return [
      ...new Set(
        this.allLogs()
          .filter((l) => !cl || l.cluster === cl)
          .map((l) => l.namespace),
      ),
    ].sort();
  });

  readonly pods = computed(() => {
    const cl = this.selectedCluster();
    const ns = this.selectedNamespace();
    return [
      ...new Set(
        this.allLogs()
          .filter((l) => !cl || l.cluster === cl)
          .filter((l) => !ns || l.namespace === ns)
          .map((l) => l.pod),
      ),
    ].sort();
  });

  readonly containers = computed(() => {
    const cl = this.selectedCluster();
    const ns = this.selectedNamespace();
    const pod = this.selectedPod();
    return [
      ...new Set(
        this.allLogs()
          .filter((l) => !cl || l.cluster === cl)
          .filter((l) => !ns || l.namespace === ns)
          .filter((l) => !pod || l.pod === pod)
          .map((l) => l.container),
      ),
    ].sort();
  });

  // ── time range
  private readonly timeRange = computed((): { from: Date; to: Date } => {
    const now = new Date();
    const preset = TIME_PRESETS.find((p) => p.value === this.timePreset());
    const minutes = preset?.minutes ?? 60;
    return { from: new Date(now.getTime() - minutes * 60 * 1000), to: now };
  });

  // ── filtered logs without level filter (used for counts + histogram)
  private readonly filteredLogsNoLevel = computed(() => {
    const { from, to } = this.timeRange();
    const cl = this.selectedCluster();
    const ns = this.selectedNamespace();
    const pod = this.selectedPod();
    const container = this.selectedContainer();
    const search = this.searchText().toLowerCase();
    return this.allLogs().filter(
      (l) =>
        l.timestamp >= from &&
        l.timestamp <= to &&
        (!cl || l.cluster === cl) &&
        (!ns || l.namespace === ns) &&
        (!pod || l.pod === pod) &&
        (!container || l.container === container) &&
        (!search ||
          l.message.toLowerCase().includes(search) ||
          l.pod.toLowerCase().includes(search)),
    );
  });

  // ── level counts for chips
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
    const parts: string[] = [];
    if (this.selectedNamespace()) parts.push(`namespace="${this.selectedNamespace()}"`);
    if (this.selectedPod()) parts.push(`pod=~"${this.selectedPod()}.*"`);
    if (this.selectedContainer()) parts.push(`container="${this.selectedContainer()}"`);
    const levels = this.selectedLevels();
    if (levels.size > 0 && levels.size < 4) {
      parts.push(`level=~"${[...levels].join('|')}"`);
    }
    const search = this.searchText();
    if (search) parts.push(`|= "${search}"`);
    if (parts.length === 0) return '{}';
    const scope = parts
      .slice(0, -1)
      .filter((p) => !p.startsWith('|='))
      .join(', ');
    const textParts = parts.filter((p) => p.startsWith('|='));
    const levelPart = parts.find((p) => p.startsWith('level'));
    const braceParts = [scope, levelPart && !scope.includes('level') ? levelPart : '']
      .filter(Boolean)
      .join(', ');
    return `{${braceParts}}${textParts.length ? ` ${textParts.join(' ')}` : ''}`;
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

  // ── all active filter chips (for display)
  readonly activeFilterChips = computed(() => {
    const chips: { label: string; key: string }[] = [];
    if (this.selectedCluster())
      chips.push({ label: `cluster: ${this.selectedCluster()}`, key: 'cluster' });
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

  ngAfterViewInit(): void {
    this.createHistogram();
  }

  ngOnDestroy(): void {
    this.stopLiveTailTimer();
    this.histogram?.destroy();
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
      this.stopLiveTailTimer();
      this.liveTailEnabled.set(false);
      this.liveTailPaused.set(false);
      this.liveTailRate.set(0);
    } else {
      this.liveTailEnabled.set(true);
      this.liveTailPaused.set(false);
      this.currentPage.set(0);
      this.startLiveTailTimer();
    }
  }

  pauseLiveTail(): void {
    this.toggleLiveTail();
  }

  resumeLiveTail(): void {
    this.liveTailPaused.set(false);
  }

  private startLiveTailTimer(): void {
    this.liveTailInterval = setInterval(() => {
      if (this.liveTailPaused()) return;
      const count = 1 + Math.floor(Math.random() * 3);
      const newEntries = Array.from({ length: count }, () =>
        generateLiveTailEntry(this.selectedCluster(), this.selectedNamespace()),
      );
      this.allLogs.update((logs) => [...newEntries, ...logs].slice(0, 2000));
      this.liveTailRate.set(count);
    }, 1000);
  }

  private stopLiveTailTimer(): void {
    if (this.liveTailInterval !== null) {
      clearInterval(this.liveTailInterval);
      this.liveTailInterval = null;
    }
  }

  // ── filter actions
  onClusterChange(value: string): void {
    this.selectedCluster.set(value);
    this.selectedNamespace.set('');
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.currentPage.set(0);
  }

  onNamespaceChange(value: string): void {
    this.selectedNamespace.set(value);
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.currentPage.set(0);
  }

  onPodChange(value: string): void {
    this.selectedPod.set(value);
    this.selectedContainer.set('');
    this.currentPage.set(0);
  }

  onContainerChange(value: string): void {
    this.selectedContainer.set(value);
    this.currentPage.set(0);
  }

  onTimePresetChange(value: string): void {
    this.timePreset.set(value);
    this.currentPage.set(0);
  }

  onSearchChange(value: string): void {
    this.searchText.set(value);
    this.currentPage.set(0);
  }

  clearAllFilters(): void {
    this.selectedCluster.set('');
    this.selectedNamespace.set('');
    this.selectedPod.set('');
    this.selectedContainer.set('');
    this.searchText.set('');
    this.selectedLevels.set(new Set(ALL_LEVELS));
    this.currentPage.set(0);
  }

  removeChip(key: string): void {
    if (key === 'cluster') {
      this.selectedCluster.set('');
      this.selectedNamespace.set('');
      this.selectedPod.set('');
      this.selectedContainer.set('');
    } else if (key === 'namespace') {
      this.selectedNamespace.set('');
      this.selectedPod.set('');
      this.selectedContainer.set('');
    } else if (key === 'pod') {
      this.selectedPod.set('');
      this.selectedContainer.set('');
    } else if (key === 'container') this.selectedContainer.set('');
    this.currentPage.set(0);
  }

  toggleAllLevels(): void {
    this.selectedLevels.set(new Set(ALL_LEVELS));
    this.currentPage.set(0);
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

  readonly levelRowBorderClass = levelRowBorderClass;

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
