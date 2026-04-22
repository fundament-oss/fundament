import {
  Component,
  inject,
  OnInit,
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
import { firstValueFrom } from 'rxjs';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import { type Timestamp, timestampFromDate, timestampDate } from '@bufbuild/protobuf/wkt';
import { TitleService } from '../title.service';
import DateRangePickerComponent from '../date-range-picker/date-range-picker.component';
import { CLUSTER, METRICS } from '../../connect/tokens';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary,
} from '../../generated/v1/cluster_pb';
import {
  GetClusterWorkloadMetricsRequestSchema,
  GetClusterWorkloadTimeSeriesRequestSchema,
  GetOrgWorkloadMetricsRequestSchema,
  GetOrgWorkloadTimeSeriesRequestSchema,
  GetProjectWorkloadMetricsRequestSchema,
  GetProjectWorkloadTimeSeriesRequestSchema,
  type GetClusterWorkloadMetricsResponse,
  type GetOrgWorkloadMetricsResponse,
  type GetProjectWorkloadMetricsResponse,
  type GetWorkloadTimeSeriesResponse,
  type NamespaceWorkloadMetrics,
} from '../../generated/v1/metrics_pb';

Chart.register(...registerables);

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

@Component({
  selector: 'app-usage',
  imports: [FormsModule, DateRangePickerComponent, DecimalPipe],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './usage.component.html',
})
export default class UsageComponent implements OnInit {
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

  // View mode derived from route
  viewMode = signal<'org' | 'project'>('org');

  projectId = signal<string | null>(null);

  // Filter state
  selectedClusterId = signal('');

  selectedNamespace = signal('');

  dateFrom = '';

  dateTo = '';

  // Data signals
  clusters = signal<ClusterOption[]>([]);

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  // Org-level: totals + per-cluster breakdown
  orgTotals = signal<ClusterUsageData | null>(null);

  // Project-level totals (separate from orgTotals to avoid stale data on view switch)
  projectTotals = signal<ClusterUsageData | null>(null);

  clusterSummaries = signal<ClusterSummaryData[]>([]);

  // Cluster-level: totals + per-node breakdown
  clusterTotals = signal<ClusterUsageData | null>(null);

  nodeUsage = signal<NodeUsageData[]>([]);

  // Shared: namespace breakdown
  namespaceUsage = signal<NamespaceUsageData[]>([]);

  // Chart data (plain arrays — updated before chart re-creation)
  private cpuSeriesData: number[] = [];

  private memorySeriesData: number[] = [];

  private podSeriesData: number[] = [];

  private networkRxSeriesData: number[] = [];

  private networkTxSeriesData: number[] = [];

  private chartLabels: string[] = [];

  constructor() {
    this.titleService.setTitle('Usage');

    const today = new Date();
    const weekAgo = new Date(today);
    weekAgo.setDate(weekAgo.getDate() - 7);
    this.dateTo = today.toISOString().split('T')[0];
    this.dateFrom = weekAgo.toISOString().split('T')[0];
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    if (projectId) {
      this.viewMode.set('project');
      this.projectId.set(projectId);
      this.loadProjectMetrics();
    } else {
      this.viewMode.set('org');
      this.loadClusters();
      this.loadOrgMetrics();
    }
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

  onClusterChange(): void {
    if (this.selectedClusterId()) {
      this.loadClusterMetrics(this.selectedClusterId());
    } else {
      this.clusterTotals.set(null);
      this.nodeUsage.set([]);
      this.loadOrgMetrics();
    }
  }

  onDateChange(): void {
    if (this.viewMode() === 'project') {
      this.loadProjectMetrics();
    } else if (this.selectedClusterId()) {
      this.loadClusterMetrics(this.selectedClusterId());
    } else {
      this.loadOrgMetrics();
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

  private async loadOrgMetrics(): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    const { start, end } = this.dateRange();

    try {
      const [workload, timeSeries] = await Promise.all([
        firstValueFrom(
          this.metricsClient.getOrgWorkloadMetrics(create(GetOrgWorkloadMetricsRequestSchema, {})),
        ),
        firstValueFrom(
          this.metricsClient.getOrgWorkloadTimeSeries(
            create(GetOrgWorkloadTimeSeriesRequestSchema, { start, end }),
          ),
        ),
      ]);

      this.applyOrgWorkload(workload);
      this.applyTimeSeries(timeSeries);
    } catch (err) {
      this.errorMessage.set(String(err));
    } finally {
      this.isLoading.set(false);
      // setTimeout defers chart creation until the next macrotask, giving Angular
      // a chance to render the canvas elements before Chart.js tries to access them.
      setTimeout(() => this.refreshCharts());
    }
  }

  private async loadClusterMetrics(clusterId: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    const { start, end } = this.dateRange();

    try {
      const [workload, timeSeries] = await Promise.all([
        firstValueFrom(
          this.metricsClient.getClusterWorkloadMetrics(
            create(GetClusterWorkloadMetricsRequestSchema, { clusterId }),
          ),
        ),
        firstValueFrom(
          this.metricsClient.getClusterWorkloadTimeSeries(
            create(GetClusterWorkloadTimeSeriesRequestSchema, {
              clusterId,
              start,
              end,
            }),
          ),
        ),
      ]);

      this.applyClusterWorkload(workload);
      this.applyTimeSeries(timeSeries);
    } catch (err) {
      this.errorMessage.set(String(err));
    } finally {
      this.isLoading.set(false);
      // setTimeout defers chart creation until the next macrotask, giving Angular
      // a chance to render the canvas elements before Chart.js tries to access them.
      setTimeout(() => this.refreshCharts());
    }
  }

  private async loadProjectMetrics(): Promise<void> {
    const pid = this.projectId();
    if (!pid) return;

    this.isLoading.set(true);
    this.errorMessage.set(null);

    const { start, end } = this.dateRange();

    try {
      const [workload, timeSeries] = await Promise.all([
        firstValueFrom(
          this.metricsClient.getProjectWorkloadMetrics(
            create(GetProjectWorkloadMetricsRequestSchema, { projectId: pid }),
          ),
        ),
        firstValueFrom(
          this.metricsClient.getProjectWorkloadTimeSeries(
            create(GetProjectWorkloadTimeSeriesRequestSchema, {
              projectId: pid,
              start,
              end,
            }),
          ),
        ),
      ]);

      this.applyProjectWorkload(workload);
      this.applyTimeSeries(timeSeries);
    } catch (err) {
      this.errorMessage.set(String(err));
    } finally {
      this.isLoading.set(false);
      // setTimeout defers chart creation until the next macrotask, giving Angular
      // a chance to render the canvas elements before Chart.js tries to access them.
      setTimeout(() => this.refreshCharts());
    }
  }

  // -- Response mappers --

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

  private applyOrgWorkload(r: GetOrgWorkloadMetricsResponse): void {
    const t = r.totals;
    this.orgTotals.set(
      t
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
        : null,
    );
    this.clusterSummaries.set(
      r.clusters.map((c) => ({
        id: c.clusterId,
        name: c.clusterName,
        cpu: { used: c.cpu?.used ?? 0, total: c.cpu?.total ?? 0 },
        memory: { used: c.memory?.used ?? 0, total: c.memory?.total ?? 0 },
        pods: { used: c.pods?.used ?? 0, total: c.pods?.total ?? 0 },
      })),
    );
    this.namespaceUsage.set(UsageComponent.mapNamespaceUsage(r.namespaces));
  }

  private applyClusterWorkload(r: GetClusterWorkloadMetricsResponse): void {
    const t = r.totals;
    this.clusterTotals.set(
      t
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
        : null,
    );
    this.nodeUsage.set(
      r.nodes.map((n) => ({
        name: n.node,
        cpu: { used: n.cpu?.used ?? 0, total: n.cpu?.total ?? 0 },
        memory: { used: n.memory?.used ?? 0, total: n.memory?.total ?? 0 },
        pods: { used: n.pods?.used ?? 0, total: n.pods?.total ?? 0 },
      })),
    );
    this.namespaceUsage.set(UsageComponent.mapNamespaceUsage(r.namespaces));
  }

  private applyProjectWorkload(r: GetProjectWorkloadMetricsResponse): void {
    const t = r.totals;
    this.projectTotals.set(
      t
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
        : null,
    );
    this.namespaceUsage.set(UsageComponent.mapNamespaceUsage(r.namespaces));
  }

  private applyTimeSeries(r: GetWorkloadTimeSeriesResponse): void {
    this.chartLabels = r.cpuCores.map((s) => formatTimestamp(s.timestamp));
    this.cpuSeriesData = r.cpuCores.map((s) => s.value);
    this.memorySeriesData = r.memoryGib.map((s) => s.value);
    this.podSeriesData = r.podCount.map((s) => s.value);
    this.networkRxSeriesData = r.networkReceiveMbS.map((s) => s.value);
    this.networkTxSeriesData = r.networkTransmitMbS.map((s) => s.value);
  }

  private dateRange(): { start: Timestamp; end: Timestamp } {
    const start = timestampFromDate(new Date(this.dateFrom));
    const end = timestampFromDate(new Date(`${this.dateTo}T23:59:59`));
    return { start, end };
  }

  private refreshCharts(): void {
    this.cpuChart?.destroy();
    this.memoryChart?.destroy();
    this.podChart?.destroy();
    this.networkChart?.destroy();
    this.initializeCharts(
      this.chartLabels,
      this.cpuSeriesData,
      this.memorySeriesData,
      this.podSeriesData,
      this.networkRxSeriesData,
      this.networkTxSeriesData,
    );
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
            label: 'CPU Usage (cores)',
            data: data.length ? data : [0],
            borderColor: 'rgb(99, 102, 241)',
            backgroundColor: 'rgba(99, 102, 241, 0.1)',
            tension: 0.4,
            fill: true,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true } },
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
            label: 'Memory Usage (GiB)',
            data: data.length ? data : [0],
            borderColor: 'rgb(16, 185, 129)',
            backgroundColor: 'rgba(16, 185, 129, 0.1)',
            tension: 0.4,
            fill: true,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true } },
      },
    };

    this.memoryChart = new Chart(ctx, config);
  }

  private createPodChart(labels: string[], data: number[]): void {
    if (!this.podChartCanvas) return;
    const ctx = this.podChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: labels.length ? labels : [''],
        datasets: [
          {
            label: 'Pod Count',
            data: data.length ? data : [0],
            backgroundColor: 'rgba(245, 158, 11, 0.8)',
            borderColor: 'rgb(245, 158, 11)',
            borderWidth: 1,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true } },
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
            tension: 0.4,
            fill: true,
          },
          {
            label: 'Transmit (MB/s)',
            data: tx.length ? tx : [0],
            borderColor: 'rgb(168, 85, 247)',
            backgroundColor: 'rgba(168, 85, 247, 0.1)',
            tension: 0.4,
            fill: true,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: true, position: 'top' } },
        scales: { y: { beginAtZero: true } },
      },
    };

    this.networkChart = new Chart(ctx, config);
  }
}
