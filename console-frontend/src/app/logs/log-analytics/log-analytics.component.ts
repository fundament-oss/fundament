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
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import type { LogLevel } from '../log.types';
import { generateMockLogs } from '../log-mock-data';
import { TitleService } from '../../title.service';

Chart.register(...registerables);

const ALL_LEVELS: LogLevel[] = ['ERROR', 'WARN', 'INFO', 'DEBUG'];

const TIME_PRESETS = [
  { label: 'Last 1 hour', value: '1h', minutes: 60, buckets: 12, bucketLabel: '5 min' },
  { label: 'Last 6 hours', value: '6h', minutes: 360, buckets: 12, bucketLabel: '30 min' },
  { label: 'Last 24 hours', value: '24h', minutes: 1440, buckets: 24, bucketLabel: '1 hour' },
  { label: 'Last 7 days', value: '7d', minutes: 10080, buckets: 14, bucketLabel: '12 hours' },
];

interface ErrorPattern {
  message: string;
  count: number;
}

@Component({
  selector: 'app-log-analytics',
  imports: [FormsModule, DecimalPipe, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './log-analytics.component.html',
})
export default class LogAnalyticsComponent implements AfterViewInit, OnDestroy {
  private readonly titleService = inject(TitleService);

  @ViewChild('volumeChart') private volumeCanvas!: ElementRef<HTMLCanvasElement>;
  @ViewChild('severityChart') private severityCanvas!: ElementRef<HTMLCanvasElement>;
  @ViewChild('namespaceChart') private namespaceCanvas!: ElementRef<HTMLCanvasElement>;

  private volumeChart: Chart | null = null;
  private severityChart: Chart | null = null;
  private namespaceChart: Chart | null = null;

  // ── filter state
  readonly selectedCluster = signal('');
  readonly selectedNamespace = signal('');
  readonly timePreset = signal('24h');

  readonly TIME_PRESETS = TIME_PRESETS;

  // ── raw data
  private readonly allLogs = signal(generateMockLogs(300));

  // ── derived filter options
  readonly clusters = computed(() =>
    [...new Set(this.allLogs().map((l) => l.cluster))].sort(),
  );
  readonly namespaces = computed(() => {
    const cl = this.selectedCluster();
    return [
      ...new Set(this.allLogs().filter((l) => !cl || l.cluster === cl).map((l) => l.namespace)),
    ].sort();
  });

  // ── time range
  private readonly timeRange = computed(() => {
    const now = new Date();
    const preset = TIME_PRESETS.find((p) => p.value === this.timePreset()) ?? TIME_PRESETS[2];
    return {
      from: new Date(now.getTime() - preset.minutes * 60 * 1000),
      to: now,
      buckets: preset.buckets,
    };
  });

  // ── filtered logs
  private readonly filteredLogs = computed(() => {
    const { from, to } = this.timeRange();
    const cl = this.selectedCluster();
    const ns = this.selectedNamespace();
    return this.allLogs().filter(
      (l) =>
        l.timestamp >= from &&
        l.timestamp <= to &&
        (!cl || l.cluster === cl) &&
        (!ns || l.namespace === ns),
    );
  });

  // ── KPI cards
  readonly totalLogs = computed(() => this.filteredLogs().length);
  readonly errorCount = computed(() => this.filteredLogs().filter((l) => l.level === 'ERROR').length);
  readonly errorRate = computed(() => {
    const total = this.totalLogs();
    return total > 0 ? ((this.errorCount() / total) * 100).toFixed(2) : '0.00';
  });
  readonly activePods = computed(() => new Set(this.filteredLogs().map((l) => l.pod)).size);

  // ── volume buckets (for volume chart)
  readonly volumeBuckets = computed(() => {
    const { from, to, buckets: count } = this.timeRange();
    const bucketMs = (to.getTime() - from.getTime()) / count;

    const buckets = Array.from({ length: count }, (_, i) => {
      const t = new Date(from.getTime() + i * bucketMs);
      const label = t.toLocaleString('en-US', {
        month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false,
      });
      return { label, error: 0, warn: 0, info: 0, debug: 0 };
    });

    for (const log of this.filteredLogs()) {
      const idx = Math.min(
        Math.floor((log.timestamp.getTime() - from.getTime()) / bucketMs),
        count - 1,
      );
      if (idx >= 0) {
        const key = log.level.toLowerCase() as 'error' | 'warn' | 'info' | 'debug';
        buckets[idx][key]++;
      }
    }

    return buckets;
  });

  // ── severity counts (for donut)
  readonly severityCounts = computed(() => {
    const logs = this.filteredLogs();
    return ALL_LEVELS.map((level) => ({
      level,
      count: logs.filter((l) => l.level === level).length,
    }));
  });

  // ── namespace breakdown (top 8)
  readonly namespaceCounts = computed(() => {
    const counts = new Map<string, number>();
    for (const log of this.filteredLogs()) {
      counts.set(log.namespace, (counts.get(log.namespace) ?? 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
      .map(([ns, count]) => ({ namespace: ns, count }));
  });

  // ── error patterns (top 6 error messages)
  readonly namespaceChartHeight = computed(() =>
    Math.max(180, this.namespaceCounts().length * 36 + 20),
  );

  readonly errorPatterns = computed((): ErrorPattern[] => {
    const counts = new Map<string, number>();
    for (const log of this.filteredLogs().filter((l) => l.level === 'ERROR')) {
      const pattern = log.message.replace(/\b\d{1,3}(?:\.\d{1,3}){3}:\d+\b/g, '*').replace(/[a-z0-9]{5,}\b/g, (m) => (m.length > 6 ? '*' : m));
      counts.set(pattern, (counts.get(pattern) ?? 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 6)
      .map(([message, count]) => ({ message, count }));
  });

  constructor() {
    this.titleService.setTitle('Log analytics');

    effect(() => {
      const buckets = this.volumeBuckets();
      if (this.volumeChart) {
        this.volumeChart.data.labels = buckets.map((b) => b.label);
        this.volumeChart.data.datasets[0].data = buckets.map((b) => b.error);
        this.volumeChart.data.datasets[1].data = buckets.map((b) => b.warn);
        this.volumeChart.data.datasets[2].data = buckets.map((b) => b.info);
        this.volumeChart.data.datasets[3].data = buckets.map((b) => b.debug);
        this.volumeChart.update('none');
      }
    });

    effect(() => {
      const counts = this.severityCounts();
      if (this.severityChart) {
        this.severityChart.data.datasets[0].data = counts.map((c) => c.count);
        this.severityChart.update('none');
      }
    });

    effect(() => {
      const ns = this.namespaceCounts();
      if (this.namespaceChart) {
        this.namespaceChart.data.labels = ns.map((n) => n.namespace);
        this.namespaceChart.data.datasets[0].data = ns.map((n) => n.count);
        this.namespaceChart.update('none');
      }
    });
  }

  ngAfterViewInit(): void {
    this.createVolumeChart();
    this.createSeverityChart();
    this.createNamespaceChart();
  }

  ngOnDestroy(): void {
    this.volumeChart?.destroy();
    this.severityChart?.destroy();
    this.namespaceChart?.destroy();
  }

  onClusterChange(value: string): void {
    this.selectedCluster.set(value);
    this.selectedNamespace.set('');
  }

  onNamespaceChange(value: string): void {
    this.selectedNamespace.set(value);
  }

  onTimePresetChange(value: string): void {
    this.timePreset.set(value);
  }

  private createVolumeChart(): void {
    const buckets = this.volumeBuckets();
    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: buckets.map((b) => b.label),
        datasets: [
          { label: 'Error', data: buckets.map((b) => b.error), backgroundColor: 'rgba(220,38,38,0.75)', stack: 'logs' },
          { label: 'Warn', data: buckets.map((b) => b.warn), backgroundColor: 'rgba(217,119,6,0.75)', stack: 'logs' },
          { label: 'Info', data: buckets.map((b) => b.info), backgroundColor: 'rgba(37,99,235,0.65)', stack: 'logs' },
          { label: 'Debug', data: buckets.map((b) => b.debug), backgroundColor: 'rgba(107,114,128,0.5)', stack: 'logs' },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { stacked: true, ticks: { maxTicksLimit: 8, maxRotation: 0, color: '#6b7280', font: { size: 11 } }, grid: { display: false } },
          y: { stacked: true, beginAtZero: true, ticks: { color: '#6b7280', font: { size: 11 } }, grid: { color: 'rgba(107,114,128,0.15)' } },
        },
      },
    };
    this.volumeChart = new Chart(this.volumeCanvas.nativeElement, config);
  }

  private createSeverityChart(): void {
    const counts = this.severityCounts();
    const config: ChartConfiguration<'doughnut'> = {
      type: 'doughnut',
      data: {
        labels: counts.map((c) => c.level),
        datasets: [{
          data: counts.map((c) => c.count),
          backgroundColor: ['rgba(220,38,38,0.8)', 'rgba(217,119,6,0.8)', 'rgba(37,99,235,0.75)', 'rgba(107,114,128,0.6)'],
          borderWidth: 2,
        }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        plugins: {
          legend: {
            position: 'bottom',
            labels: { color: '#6b7280', font: { size: 11 }, padding: 12, boxWidth: 10, boxHeight: 10 },
          },
        },
        cutout: '65%',
      },
    };
    this.severityChart = new Chart(this.severityCanvas.nativeElement, config);
  }

  private createNamespaceChart(): void {
    const ns = this.namespaceCounts();
    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: ns.map((n) => n.namespace),
        datasets: [{
          data: ns.map((n) => n.count),
          backgroundColor: 'rgba(37,99,235,0.65)',
          borderRadius: 3,
        }],
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { beginAtZero: true, ticks: { color: '#6b7280', font: { size: 11 } }, grid: { color: 'rgba(107,114,128,0.15)' } },
          y: { ticks: { color: '#6b7280', font: { size: 11 } }, grid: { display: false } },
        },
      },
    };
    this.namespaceChart = new Chart(this.namespaceCanvas.nativeElement, config);
  }
}
