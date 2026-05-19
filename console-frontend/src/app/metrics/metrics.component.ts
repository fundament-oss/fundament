import {
  Component,
  inject,
  OnInit,
  OnDestroy,
  ElementRef,
  ViewChild,
  signal,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom, Subscription } from 'rxjs';
import { Chart, ChartConfiguration, ChartDataset, registerables } from 'chart.js';
import ZoomPlugin from 'chartjs-plugin-zoom';
import { type Timestamp, timestampFromDate, timestampDate } from '@bufbuild/protobuf/wkt';
import { TitleService } from '../title.service';
import DateRangePickerComponent from '../date-range-picker/date-range-picker.component';
import { CLUSTER, METRICS } from '../../connect/tokens';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary,
} from '../../generated/v1/cluster_pb';
import {
  StreamOrgWorkloadMetricsRequestSchema,
  StreamClusterWorkloadMetricsRequestSchema,
  StreamProjectWorkloadMetricsRequestSchema,
  type StreamWorkloadMetricsResponse,
  type NamespaceWorkloadMetrics,
} from '../../generated/v1/metrics_pb';

Chart.register(...registerables, ZoomPlugin);

interface ClusterOption {
  id: string;
  name: string;
}

interface ClusterUsageData {
  cpu: { used: number; total: number; unit: string };
  memory: { used: number; total: number; unit: string };
  pods: { used: number; total: number; unit: string };
}

interface NodeUsageData {
  name: string;
  cpu: { used: number; total: number };
  memory: { used: number; total: number };
  pods: { used: number; total: number };
}

interface NamespaceUsageData {
  name: string;
  cpu: number;
  memory: number;
  pods: number;
  cpuRequests: number;
  cpuLimits: number;
  memoryRequests: number;
  memoryLimits: number;
  networkReceiveMbs: number;
  networkTransmitMbs: number;
}

interface ClusterSummaryData {
  id: string;
  name: string;
  cpu: { used: number; total: number };
  memory: { used: number; total: number };
  pods: { used: number; total: number };
}

export type TimeRangePreset = '1h' | '6h' | '24h' | '7d' | '30d' | 'custom';

export const TIME_RANGE_PRESETS: { value: TimeRangePreset; label: string }[] = [
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
  { value: 'custom', label: 'Custom' },
];

const PRESET_WINDOW_SECONDS: Record<Exclude<TimeRangePreset, 'custom'>, number> = {
  '1h': 3600,
  '6h': 21600,
  '24h': 86400,
  '7d': 604800,
  '30d': 2592000,
};

const MAX_RECONNECT_DELAY_MS = 60_000;

function getUsagePercentage(used: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((used / total) * 100);
}

function getUsageColor(percentage: number): string {
  if (percentage >= 90) return 'bg-danger-500';
  if (percentage >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
}

function formatTimestamp(ts: Timestamp | undefined, includeTime: boolean): string {
  if (!ts) return '';
  const d = timestampDate(ts);
  if (includeTime) {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function toLocalDateString(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

function presetToDateRange(preset: Exclude<TimeRangePreset, 'custom'>): {
  from: string;
  to: string;
} {
  const now = new Date();
  const to = toLocalDateString(now);

  if (preset === '1h' || preset === '6h' || preset === '24h') {
    const hoursMap: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24 };
    const hours = hoursMap[preset];
    const from = new Date(now.getTime() - hours * 60 * 60 * 1000);
    return { from: toLocalDateString(from), to };
  }
  if (preset === '7d') {
    const from = new Date(now);
    from.setDate(from.getDate() - 7);
    return { from: toLocalDateString(from), to };
  }
  if (preset === '30d') {
    const from = new Date(now);
    from.setDate(from.getDate() - 30);
    return { from: toLocalDateString(from), to };
  }
  const _: never = preset;
  throw new Error(`Unhandled preset: ${_}`);
}

function computeStepSeconds(rangeSeconds: number): number {
  if (rangeSeconds <= 3_600) return 60;
  if (rangeSeconds <= 86_400) return 300;
  if (rangeSeconds <= 604_800) return 1_800;
  return 3_600;
}

function lineDataset(
  label: string,
  borderColor: string,
  backgroundColor: string,
  data: number[],
): ChartDataset<'line'> {
  return {
    label,
    data: data.length ? data : [0],
    borderColor,
    backgroundColor,
    borderWidth: 1,
    tension: 0.4,
    fill: true,
    pointRadius: 0,
  };
}

@Component({
  selector: 'app-metrics',
  imports: [FormsModule, DateRangePickerComponent, DecimalPipe],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './metrics.component.html',
})
export default class MetricsComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private clusterClient = inject(CLUSTER);

  private metricsClient = inject(METRICS);

  @ViewChild('cpuChart') cpuChartCanvas!: ElementRef<HTMLCanvasElement>;

  @ViewChild('memoryChart') memoryChartCanvas!: ElementRef<HTMLCanvasElement>;

  @ViewChild('podChart') podChartCanvas!: ElementRef<HTMLCanvasElement>;

  @ViewChild('networkChart') networkChartCanvas!: ElementRef<HTMLCanvasElement>;

  private cpuChart?: Chart;

  private memoryChart?: Chart;

  private podChart?: Chart;

  private networkChart?: Chart;

  private streamSub: Subscription | null = null;

  private refreshChartTimeoutId: ReturnType<typeof setTimeout> | null = null;

  private reconnectTimeoutId: ReturnType<typeof setTimeout> | null = null;

  private reconnectAttempt = 0;

  private chartsReady = false;

  viewMode = signal<'org' | 'project'>('org');

  projectId = signal<string | null>(null);

  selectedClusterId = signal('');

  selectedNamespace = signal('');

  selectedPreset = signal<TimeRangePreset>('7d');

  // dateFrom, dateTo, and the chart series arrays are plain fields rather than
  // signals. Chart updates are imperative (Chart.js update()), so they don't
  // need to trigger Angular's change detection. Keeping them as plain fields
  // avoids unnecessary signal overhead and makes the intent clear.
  dateFrom = '';

  dateTo = '';

  showCustomRange = computed(() => this.selectedPreset() === 'custom');

  readonly presets = TIME_RANGE_PRESETS;

  clusters = signal<ClusterOption[]>([]);

  isLoading = signal(false);

  isLive = signal(false);

  connectionError = signal(false);

  lastRefreshedAt = signal<Date | null>(null);

  errorMessage = signal<string | null>(null);

  orgTotals = signal<ClusterUsageData | null>(null);

  projectTotals = signal<ClusterUsageData | null>(null);

  clusterSummaries = signal<ClusterSummaryData[]>([]);

  clusterTotals = signal<ClusterUsageData | null>(null);

  nodeUsage = signal<NodeUsageData[]>([]);

  namespaceUsage = signal<NamespaceUsageData[]>([]);

  private cpuSeriesData: number[] = [];

  private memorySeriesData: number[] = [];

  private podSeriesData: number[] = [];

  private networkRxSeriesData: number[] = [];

  private networkTxSeriesData: number[] = [];

  private chartLabels: string[] = [];

  private chartDates: string[] = [];

  constructor() {
    this.titleService.setTitle('Metrics');
    this.applyPreset('7d');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    if (projectId) {
      this.viewMode.set('project');
      this.projectId.set(projectId);
    } else {
      this.viewMode.set('org');
      this.loadClusters();
    }
    this.startStream();
  }

  ngOnDestroy() {
    this.cancelStream();
  }

  currentTotals = computed<ClusterUsageData | null>(() => {
    if (this.viewMode() === 'project') return this.projectTotals();
    return this.selectedClusterId() ? this.clusterTotals() : this.orgTotals();
  });

  get hasCpuData(): boolean {
    return this.cpuSeriesData.length > 0;
  }

  get hasMemoryData(): boolean {
    return this.memorySeriesData.length > 0;
  }

  get hasPodData(): boolean {
    return this.podSeriesData.length > 0;
  }

  get hasNetworkData(): boolean {
    return this.networkRxSeriesData.length > 0 || this.networkTxSeriesData.length > 0;
  }

  filteredNamespaceUsage = computed<NamespaceUsageData[]>(() => {
    if (!this.selectedNamespace()) return this.namespaceUsage();
    return this.namespaceUsage().filter((ns) => ns.name === this.selectedNamespace());
  });

  getUsagePercentage = getUsagePercentage;

  getUsageColor = getUsageColor;

  private onChartZoom(source: Chart): void {
    const { min, max } = source.scales['x'];
    [this.cpuChart, this.memoryChart, this.podChart, this.networkChart]
      .filter((chart): chart is Chart => !!chart && chart !== source)
      .forEach((chart) => chart.zoomScale('x', { min, max }, 'none'));

    const minIdx = Math.max(0, Math.round(min));
    const maxIdx = Math.min(this.chartDates.length - 1, Math.round(max));
    if (this.chartDates[minIdx]) this.dateFrom = this.chartDates[minIdx].split('T')[0];
    if (this.chartDates[maxIdx]) this.dateTo = this.chartDates[maxIdx].split('T')[0];

    if (this.selectedPreset() !== 'custom') {
      this.selectedPreset.set('custom');
      this.cancelStream();
      this.isLive.set(false);
    }
  }

  private zoomOptions() {
    return {
      zoom: {
        drag: { enabled: true },
        mode: 'x' as const,
        onZoomComplete: ({ chart }: { chart: Chart }) => this.onChartZoom(chart),
      },
      pan: { enabled: false },
    };
  }

  onPresetChangeEvent(event: Event): void {
    this.onPresetChange((event as CustomEvent<{ value: TimeRangePreset }>).detail.value);
  }

  onPresetChange(preset: TimeRangePreset): void {
    this.selectedPreset.set(preset);
    if (preset === 'custom') {
      this.cancelStream();
      this.isLive.set(false);
    } else {
      this.applyPreset(preset);
      this.startStream();
    }
  }

  onClusterChange(): void {
    if (!this.selectedClusterId()) {
      this.clusterTotals.set(null);
      this.nodeUsage.set([]);
    }
    this.startStream();
  }

  onDateChange(): void {
    this.startStream();
  }

  private applyPreset(preset: Exclude<TimeRangePreset, 'custom'>): void {
    const { from, to } = presetToDateRange(preset);
    this.dateFrom = from;
    this.dateTo = to;
  }

  private cancelStream(): void {
    this.streamSub?.unsubscribe();
    this.streamSub = null;
    if (this.refreshChartTimeoutId !== null) {
      clearTimeout(this.refreshChartTimeoutId);
      this.refreshChartTimeoutId = null;
    }
    if (this.reconnectTimeoutId !== null) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }
  }

  private startStream(fromReconnect = false): void {
    if (!fromReconnect) this.reconnectAttempt = 0;
    this.cancelStream();
    this.isLoading.set(true);
    this.isLive.set(false);
    this.connectionError.set(false);
    this.errorMessage.set(null);

    // Destroy charts so they are recreated fresh for the new stream/filter.
    this.destroyCharts();
    this.chartsReady = false;

    let obs;
    try {
      obs = this.buildStreamObservable();
    } catch (err) {
      this.isLoading.set(false);
      this.errorMessage.set(err instanceof Error ? err.message : 'Failed to start stream');
      return;
    }

    this.streamSub = obs.subscribe({
      next: (response) => {
        this.applyStreamResponse(response);
        this.isLoading.set(false);
        this.isLive.set(true);
        this.reconnectAttempt = 0;
        if (response.refreshedAt) {
          this.lastRefreshedAt.set(timestampDate(response.refreshedAt));
        }
        if (!this.chartsReady) {
          // Defer chart creation until Angular has rendered the canvases.
          this.refreshChartTimeoutId = setTimeout(() => {
            this.refreshChartTimeoutId = null;
            this.refreshCharts();
            this.chartsReady = true;
          });
        } else {
          this.updateChartsInPlace();
        }
      },
      error: () => {
        this.isLoading.set(false);
        this.isLive.set(false);
        this.connectionError.set(true);
        const delay = Math.min(5_000 * 2 ** this.reconnectAttempt, MAX_RECONNECT_DELAY_MS);
        this.reconnectAttempt += 1;
        this.reconnectTimeoutId = setTimeout(() => {
          this.reconnectTimeoutId = null;
          this.startStream(true);
        }, delay);
      },
    });
  }

  private buildStreamObservable() {
    const preset = this.selectedPreset();
    const windowSeconds = preset !== 'custom' ? PRESET_WINDOW_SECONDS[preset] : 0;

    if (this.viewMode() === 'project') {
      const pid = this.projectId();
      if (!pid) throw new Error('No project ID');
      const req =
        windowSeconds > 0
          ? create(StreamProjectWorkloadMetricsRequestSchema, {
              projectId: pid,
              windowSeconds,
              stepSeconds: computeStepSeconds(windowSeconds),
            })
          : create(StreamProjectWorkloadMetricsRequestSchema, {
              projectId: pid,
              start: timestampFromDate(new Date(this.dateFrom)),
              end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
              stepSeconds: computeStepSeconds(this.customRangeSeconds()),
            });
      return this.metricsClient.streamProjectWorkloadMetrics(req);
    }

    const clusterId = this.selectedClusterId();
    if (clusterId) {
      const req =
        windowSeconds > 0
          ? create(StreamClusterWorkloadMetricsRequestSchema, {
              clusterId,
              windowSeconds,
              stepSeconds: computeStepSeconds(windowSeconds),
            })
          : create(StreamClusterWorkloadMetricsRequestSchema, {
              clusterId,
              start: timestampFromDate(new Date(this.dateFrom)),
              end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
              stepSeconds: computeStepSeconds(this.customRangeSeconds()),
            });
      return this.metricsClient.streamClusterWorkloadMetrics(req);
    }

    const req =
      windowSeconds > 0
        ? create(StreamOrgWorkloadMetricsRequestSchema, {
            windowSeconds,
            stepSeconds: computeStepSeconds(windowSeconds),
          })
        : create(StreamOrgWorkloadMetricsRequestSchema, {
            start: timestampFromDate(new Date(this.dateFrom)),
            end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
            stepSeconds: computeStepSeconds(this.customRangeSeconds()),
          });
    return this.metricsClient.streamOrgWorkloadMetrics(req);
  }

  private customRangeSeconds(): number {
    const from = new Date(this.dateFrom).getTime();
    const to = new Date(`${this.dateTo}T23:59:59`).getTime();
    return Math.max(0, Math.round((to - from) / 1000));
  }

  private applyStreamResponse(r: StreamWorkloadMetricsResponse): void {
    const t = r.totals;
    const totals: ClusterUsageData | null = t
      ? {
          cpu: { used: t.cpu?.used ?? 0, total: t.cpu?.total ?? 0, unit: t.cpu?.unit ?? 'cores' },
          memory: {
            used: t.memory?.used ?? 0,
            total: t.memory?.total ?? 0,
            unit: t.memory?.unit ?? 'GiB',
          },
          pods: {
            used: t.pods?.used ?? 0,
            total: t.pods?.total ?? 0,
            unit: t.pods?.unit ?? 'pods',
          },
        }
      : null;

    if (this.viewMode() === 'project') {
      this.projectTotals.set(totals);
    } else if (this.selectedClusterId()) {
      this.clusterTotals.set(totals);
      this.nodeUsage.set(
        r.nodes.map((n) => ({
          name: n.node,
          cpu: { used: n.cpu?.used ?? 0, total: n.cpu?.total ?? 0 },
          memory: { used: n.memory?.used ?? 0, total: n.memory?.total ?? 0 },
          pods: { used: n.pods?.used ?? 0, total: n.pods?.total ?? 0 },
        })),
      );
    } else {
      this.orgTotals.set(totals);
      this.clusterSummaries.set(
        r.clusters.map((c) => ({
          id: c.clusterId,
          name: c.clusterName,
          cpu: { used: c.cpu?.used ?? 0, total: c.cpu?.total ?? 0 },
          memory: { used: c.memory?.used ?? 0, total: c.memory?.total ?? 0 },
          pods: { used: c.pods?.used ?? 0, total: c.pods?.total ?? 0 },
        })),
      );
    }

    this.namespaceUsage.set(MetricsComponent.mapNamespaceUsage(r.namespaces));

    if (r.timeSeries) {
      this.applyTimeSeries(r.timeSeries);
    }
  }

  private async loadClusters(): Promise<void> {
    try {
      const response = await firstValueFrom(
        this.clusterClient.listClusters(create(ListClustersRequestSchema, {})),
      );
      this.clusters.set(
        response.clusters.map((c: ListClustersResponse_ClusterSummary) => ({
          id: c.id,
          name: c.name,
        })),
      );
    } catch {
      // Non-fatal — cluster dropdown will be empty but the rest of the page still works.
    }
  }

  private static mapNamespaceUsage(namespaces: NamespaceWorkloadMetrics[]): NamespaceUsageData[] {
    return namespaces.map((n) => ({
      name: n.namespace,
      cpu: n.cpuCores,
      memory: n.memoryGib,
      pods: n.pods,
      cpuRequests: n.cpuRequests,
      cpuLimits: n.cpuLimits,
      memoryRequests: n.memoryRequestsGib,
      memoryLimits: n.memoryLimitsGib,
      networkReceiveMbs: n.networkReceiveMbS,
      networkTransmitMbs: n.networkTransmitMbS,
    }));
  }

  private applyTimeSeries(r: {
    cpuCores: { timestamp?: Timestamp; value: number }[];
    memoryGib: { timestamp?: Timestamp; value: number }[];
    podCount: { timestamp?: Timestamp; value: number }[];
    networkReceiveMbS: { timestamp?: Timestamp; value: number }[];
    networkTransmitMbS: { timestamp?: Timestamp; value: number }[];
  }): void {
    const windowSeconds =
      PRESET_WINDOW_SECONDS[this.selectedPreset() as Exclude<TimeRangePreset, 'custom'>] ?? 0;
    const includeTime = windowSeconds > 0 && windowSeconds <= 86400;
    this.chartLabels = r.cpuCores.map((s) => formatTimestamp(s.timestamp, includeTime));
    this.chartDates = r.cpuCores.map((s) =>
      s.timestamp ? timestampDate(s.timestamp).toISOString() : '',
    );
    this.cpuSeriesData = r.cpuCores.map((s) => s.value);
    this.memorySeriesData = r.memoryGib.map((s) => s.value);
    this.podSeriesData = r.podCount.map((s) => s.value);
    this.networkRxSeriesData = r.networkReceiveMbS.map((s) => s.value);
    this.networkTxSeriesData = r.networkTransmitMbS.map((s) => s.value);
  }

  private destroyCharts(): void {
    this.cpuChart?.destroy();
    this.memoryChart?.destroy();
    this.podChart?.destroy();
    this.networkChart?.destroy();
    this.cpuChart = undefined;
    this.memoryChart = undefined;
    this.podChart = undefined;
    this.networkChart = undefined;
  }

  private refreshCharts(): void {
    this.destroyCharts();
    this.initializeCharts(
      this.chartLabels,
      this.cpuSeriesData,
      this.memorySeriesData,
      this.podSeriesData,
      this.networkRxSeriesData,
      this.networkTxSeriesData,
    );
  }

  private updateChartsInPlace(): void {
    if (this.cpuChart) {
      this.cpuChart.data.labels = this.chartLabels;
      this.cpuChart.data.datasets[0].data = this.cpuSeriesData;
      this.cpuChart.update('none');
    }
    if (this.memoryChart) {
      this.memoryChart.data.labels = this.chartLabels;
      this.memoryChart.data.datasets[0].data = this.memorySeriesData;
      this.memoryChart.update('none');
    }
    if (this.podChart) {
      this.podChart.data.labels = this.chartLabels;
      this.podChart.data.datasets[0].data = this.podSeriesData;
      this.podChart.update('none');
    }
    if (this.networkChart) {
      this.networkChart.data.labels = this.chartLabels;
      this.networkChart.data.datasets[0].data = this.networkRxSeriesData;
      this.networkChart.data.datasets[1].data = this.networkTxSeriesData;
      this.networkChart.update('none');
    }
  }

  private initializeCharts(
    labels: string[],
    cpu: number[],
    memory: number[],
    pods: number[],
    networkRx: number[],
    networkTx: number[],
  ): void {
    this.createCpuChart(labels, cpu);
    this.createMemoryChart(labels, memory);
    this.createPodChart(labels, pods);
    this.createNetworkChart(labels, networkRx, networkTx);
  }

  private lineChartConfig(
    labels: string[],
    datasets: ChartDataset<'line'>[],
    showLegend = false,
  ): ChartConfiguration<'line'> {
    return {
      type: 'line',
      data: { labels: labels.length ? labels : [''], datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: showLegend, position: 'top' },
          zoom: this.zoomOptions(),
        },
        scales: { x: { grid: { display: false } }, y: { beginAtZero: true } },
      },
    };
  }

  private createCpuChart(labels: string[], data: number[]): void {
    if (!this.cpuChartCanvas) return;
    const ctx = this.cpuChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;
    this.cpuChart = new Chart(
      ctx,
      this.lineChartConfig(labels, [
        lineDataset('CPU usage (cores)', 'rgb(99, 102, 241)', 'rgba(99, 102, 241, 0.1)', data),
      ]),
    );
  }

  private createMemoryChart(labels: string[], data: number[]): void {
    if (!this.memoryChartCanvas) return;
    const ctx = this.memoryChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;
    this.memoryChart = new Chart(
      ctx,
      this.lineChartConfig(labels, [
        lineDataset('Memory usage (GiB)', 'rgb(16, 185, 129)', 'rgba(16, 185, 129, 0.1)', data),
      ]),
    );
  }

  private createPodChart(labels: string[], data: number[]): void {
    if (!this.podChartCanvas) return;
    const ctx = this.podChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;
    this.podChart = new Chart(
      ctx,
      this.lineChartConfig(labels, [
        lineDataset('Pod count', 'rgb(245, 158, 11)', 'rgba(245, 158, 11, 0.1)', data),
      ]),
    );
  }

  private createNetworkChart(labels: string[], rx: number[], tx: number[]): void {
    if (!this.networkChartCanvas) return;
    const ctx = this.networkChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;
    this.networkChart = new Chart(
      ctx,
      this.lineChartConfig(
        labels,
        [
          lineDataset('Receive (MB/s)', 'rgb(59, 130, 246)', 'rgba(59, 130, 246, 0.1)', rx),
          lineDataset('Transmit (MB/s)', 'rgb(168, 85, 247)', 'rgba(168, 85, 247, 0.1)', tx),
        ],
        true,
      ),
    );
  }
}
