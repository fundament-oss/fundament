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
import { Chart, ChartConfiguration, registerables } from 'chart.js';
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

const PRESET_WINDOW_SECONDS: Record<string, number> = {
  '1h': 3600,
  '6h': 21600,
  '24h': 86400,
  '7d': 604800,
  '30d': 2592000,
};

function getUsagePercentage(used: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((used / total) * 100);
}

function getUsageColor(percentage: number): string {
  if (percentage >= 90) return 'bg-danger-500';
  if (percentage >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
}

function formatTimestamp(ts: Timestamp | undefined): string {
  if (!ts) return '';
  const d = timestampDate(ts);
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function presetToDateRange(preset: TimeRangePreset): { from: string; to: string } {
  const now = new Date();
  const to = now.toISOString().split('T')[0];

  if (preset === '1h' || preset === '6h' || preset === '24h') {
    const hoursMap: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24 };
    const hours = hoursMap[preset];
    const from = new Date(now.getTime() - hours * 60 * 60 * 1000);
    return { from: from.toISOString().split('T')[0], to };
  }
  if (preset === '7d') {
    const from = new Date(now);
    from.setDate(from.getDate() - 7);
    return { from: from.toISOString().split('T')[0], to };
  }
  if (preset === '30d') {
    const from = new Date(now);
    from.setDate(from.getDate() - 30);
    return { from: from.toISOString().split('T')[0], to };
  }
  return { from: to, to };
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

  private chartsReady = false;

  viewMode = signal<'org' | 'project'>('org');

  projectId = signal<string | null>(null);

  selectedClusterId = signal('');

  selectedNamespace = signal('');

  selectedPreset = signal<TimeRangePreset>('7d');

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
    if (this.chartDates[minIdx]) this.dateFrom = this.chartDates[minIdx];
    if (this.chartDates[maxIdx]) this.dateTo = this.chartDates[maxIdx];

    this.selectedPreset.set('custom');
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

  onPresetChange(preset: TimeRangePreset): void {
    this.selectedPreset.set(preset);
    if (preset !== 'custom') {
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

  private applyPreset(preset: TimeRangePreset): void {
    const { from, to } = presetToDateRange(preset);
    this.dateFrom = from;
    this.dateTo = to;
  }

  private cancelStream(): void {
    this.streamSub?.unsubscribe();
    this.streamSub = null;
  }

  private startStream(): void {
    this.cancelStream();
    this.isLoading.set(true);
    this.isLive.set(false);
    this.connectionError.set(false);
    this.errorMessage.set(null);

    // Destroy charts so they are recreated fresh for the new stream/filter.
    this.destroyCharts();
    this.chartsReady = false;

    const obs = this.buildStreamObservable();
    this.streamSub = obs.subscribe({
      next: (response) => {
        this.applyStreamResponse(response);
        this.isLoading.set(false);
        this.isLive.set(true);
        if (response.refreshedAt) {
          this.lastRefreshedAt.set(timestampDate(response.refreshedAt));
        }
        if (!this.chartsReady) {
          // Defer chart creation until Angular has rendered the canvases.
          setTimeout(() => {
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
      },
    });
  }

  private buildStreamObservable() {
    const preset = this.selectedPreset();
    const windowSeconds = PRESET_WINDOW_SECONDS[preset] ?? 0;

    if (this.viewMode() === 'project') {
      const pid = this.projectId();
      if (!pid) throw new Error('No project ID');
      const req =
        windowSeconds > 0
          ? create(StreamProjectWorkloadMetricsRequestSchema, { projectId: pid, windowSeconds })
          : create(StreamProjectWorkloadMetricsRequestSchema, {
              projectId: pid,
              start: timestampFromDate(new Date(this.dateFrom)),
              end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
            });
      return this.metricsClient.streamProjectWorkloadMetrics(req);
    }

    const clusterId = this.selectedClusterId();
    if (clusterId) {
      const req =
        windowSeconds > 0
          ? create(StreamClusterWorkloadMetricsRequestSchema, { clusterId, windowSeconds })
          : create(StreamClusterWorkloadMetricsRequestSchema, {
              clusterId,
              start: timestampFromDate(new Date(this.dateFrom)),
              end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
            });
      return this.metricsClient.streamClusterWorkloadMetrics(req);
    }

    const req =
      windowSeconds > 0
        ? create(StreamOrgWorkloadMetricsRequestSchema, { windowSeconds })
        : create(StreamOrgWorkloadMetricsRequestSchema, {
            start: timestampFromDate(new Date(this.dateFrom)),
            end: timestampFromDate(new Date(`${this.dateTo}T23:59:59`)),
          });
    return this.metricsClient.streamOrgWorkloadMetrics(req);
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
    } catch (err) {
      // Non-fatal — cluster dropdown will be empty but the rest of the page still works.
      // eslint-disable-next-line no-console
      console.error('Failed to load cluster list:', err);
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
    this.chartLabels = r.cpuCores.map((s) => formatTimestamp(s.timestamp));
    this.chartDates = r.cpuCores.map((s) =>
      s.timestamp ? timestampDate(s.timestamp).toISOString().split('T')[0] : '',
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

  private createCpuChart(labels: string[], data: number[]): void {
    if (!this.cpuChartCanvas) return;
    const ctx = this.cpuChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: labels.length ? labels : [''],
        datasets: [
          {
            label: 'CPU usage (cores)',
            data: data.length ? data : [0],
            borderColor: 'rgb(99, 102, 241)',
            backgroundColor: 'rgba(99, 102, 241, 0.1)',
            borderWidth: 1,
            tension: 0.4,
            fill: true,
            pointRadius: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          zoom: this.zoomOptions(),
        },
        scales: { x: { grid: { display: false } }, y: { beginAtZero: true } },
      },
    };

    this.cpuChart = new Chart(ctx, config);
  }

  private createMemoryChart(labels: string[], data: number[]): void {
    if (!this.memoryChartCanvas) return;
    const ctx = this.memoryChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: labels.length ? labels : [''],
        datasets: [
          {
            label: 'Memory usage (GiB)',
            data: data.length ? data : [0],
            borderColor: 'rgb(16, 185, 129)',
            backgroundColor: 'rgba(16, 185, 129, 0.1)',
            borderWidth: 1,
            tension: 0.4,
            fill: true,
            pointRadius: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          zoom: this.zoomOptions(),
        },
        scales: { x: { grid: { display: false } }, y: { beginAtZero: true } },
      },
    };

    this.memoryChart = new Chart(ctx, config);
  }

  private createPodChart(labels: string[], data: number[]): void {
    if (!this.podChartCanvas) return;
    const ctx = this.podChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: labels.length ? labels : [''],
        datasets: [
          {
            label: 'Pod count',
            data: data.length ? data : [0],
            borderColor: 'rgb(245, 158, 11)',
            backgroundColor: 'rgba(245, 158, 11, 0.1)',
            borderWidth: 1,
            tension: 0.4,
            fill: true,
            pointRadius: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          zoom: this.zoomOptions(),
        },
        scales: { x: { grid: { display: false } }, y: { beginAtZero: true } },
      },
    };

    this.podChart = new Chart(ctx, config);
  }

  private createNetworkChart(labels: string[], rx: number[], tx: number[]): void {
    if (!this.networkChartCanvas) return;
    const ctx = this.networkChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: labels.length ? labels : [''],
        datasets: [
          {
            label: 'Receive (MB/s)',
            data: rx.length ? rx : [0],
            borderColor: 'rgb(59, 130, 246)',
            backgroundColor: 'rgba(59, 130, 246, 0.1)',
            borderWidth: 1,
            tension: 0.4,
            fill: true,
            pointRadius: 0,
          },
          {
            label: 'Transmit (MB/s)',
            data: tx.length ? tx : [0],
            borderColor: 'rgb(168, 85, 247)',
            backgroundColor: 'rgba(168, 85, 247, 0.1)',
            borderWidth: 1,
            tension: 0.4,
            fill: true,
            pointRadius: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: true, position: 'top' },
          zoom: this.zoomOptions(),
        },
        scales: { x: { grid: { display: false } }, y: { beginAtZero: true } },
      },
    };

    this.networkChart = new Chart(ctx, config);
  }
}
