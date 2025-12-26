import { Component, inject, AfterViewInit, ElementRef, ViewChild } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import { DateRangePickerComponent } from '../date-range-picker/date-range-picker.component';

Chart.register(...registerables);

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
}

interface Cluster {
  id: string;
  name: string;
  namespaces: string[];
}

interface Project {
  id: string;
  name: string;
  clusterIds: string[];
}

@Component({
  selector: 'app-usage',
  standalone: true,
  imports: [CommonModule, FormsModule, DateRangePickerComponent],
  templateUrl: './usage.component.html',
  styleUrl: './usage.component.css',
})
export class UsageComponent implements AfterViewInit {
  private titleService = inject(Title);

  @ViewChild('cpuChart') cpuChartCanvas!: ElementRef<HTMLCanvasElement>;
  @ViewChild('memoryChart') memoryChartCanvas!: ElementRef<HTMLCanvasElement>;
  @ViewChild('podChart') podChartCanvas!: ElementRef<HTMLCanvasElement>;

  private cpuChart?: Chart;
  private memoryChart?: Chart;
  private podChart?: Chart;

  // Filter state
  selectedProjectId = '';
  selectedClusterId = '';
  selectedNamespace = '';
  dateFrom = '';
  dateTo = '';

  // Mock data
  projects: Project[] = [
    { id: 'proj-1', name: 'Production Services', clusterIds: ['cluster-1', 'cluster-2'] },
    { id: 'proj-2', name: 'Development', clusterIds: ['cluster-2', 'cluster-3'] },
    { id: 'proj-3', name: 'Testing', clusterIds: ['cluster-1'] },
  ];

  clusters: Cluster[] = [
    {
      id: 'cluster-1',
      name: 'prod-cluster-nl1',
      namespaces: ['default', 'production', 'monitoring', 'ingress'],
    },
    { id: 'cluster-2', name: 'prod-cluster-nl2', namespaces: ['default', 'production', 'staging'] },
    { id: 'cluster-3', name: 'dev-cluster', namespaces: ['default', 'development', 'testing'] },
  ];

  clusterUsage: ClusterUsageData = {
    cpu: { used: 24.5, total: 48.0, unit: 'cores' },
    memory: { used: 89.2, total: 192.0, unit: 'GB' },
    pods: { used: 156, total: 330, unit: 'pods' },
  };

  nodeUsage: NodeUsageData[] = [
    {
      name: 'node-1',
      cpu: { used: 3.2, total: 8.0 },
      memory: { used: 15.4, total: 32.0 },
      pods: { used: 28, total: 110 },
    },
    {
      name: 'node-2',
      cpu: { used: 5.8, total: 8.0 },
      memory: { used: 22.1, total: 32.0 },
      pods: { used: 42, total: 110 },
    },
    {
      name: 'node-3',
      cpu: { used: 4.1, total: 8.0 },
      memory: { used: 18.7, total: 32.0 },
      pods: { used: 35, total: 110 },
    },
    {
      name: 'node-4',
      cpu: { used: 6.2, total: 8.0 },
      memory: { used: 19.3, total: 32.0 },
      pods: { used: 31, total: 110 },
    },
    {
      name: 'node-5',
      cpu: { used: 2.8, total: 8.0 },
      memory: { used: 8.9, total: 32.0 },
      pods: { used: 12, total: 110 },
    },
    {
      name: 'node-6',
      cpu: { used: 2.4, total: 8.0 },
      memory: { used: 4.8, total: 32.0 },
      pods: { used: 8, total: 110 },
    },
  ];

  namespaceUsage: NamespaceUsageData[] = [
    { name: 'production', cpu: 12.8, memory: 45.2, pods: 68 },
    { name: 'staging', cpu: 5.3, memory: 18.7, pods: 32 },
    { name: 'monitoring', cpu: 3.2, memory: 12.4, pods: 24 },
    { name: 'ingress', cpu: 1.8, memory: 6.3, pods: 12 },
    { name: 'default', cpu: 1.4, memory: 6.6, pods: 20 },
  ];

  constructor() {
    this.titleService.setTitle('Usage — Fundament Console');

    // Set default date range (last 7 days)
    const today = new Date();
    const weekAgo = new Date(today);
    weekAgo.setDate(weekAgo.getDate() - 7);

    this.dateTo = today.toISOString().split('T')[0];
    this.dateFrom = weekAgo.toISOString().split('T')[0];
  }

  ngAfterViewInit(): void {
    this.initializeCharts();
  }

  get availableClusters(): Cluster[] {
    if (!this.selectedProjectId) {
      return this.clusters;
    }
    const project = this.projects.find((p) => p.id === this.selectedProjectId);
    if (!project) {
      return [];
    }
    return this.clusters.filter((c) => project.clusterIds.includes(c.id));
  }

  get availableNamespaces(): string[] {
    if (!this.selectedClusterId) {
      return [];
    }
    const cluster = this.clusters.find((c) => c.id === this.selectedClusterId);
    return cluster ? cluster.namespaces : [];
  }

  onProjectChange(): void {
    this.selectedClusterId = '';
    this.selectedNamespace = '';
    this.updateCharts();
  }

  onClusterChange(): void {
    this.selectedNamespace = '';
    this.updateCharts();
  }

  onNamespaceChange(): void {
    this.updateCharts();
  }

  onDateChange(): void {
    this.updateCharts();
  }

  getUsagePercentage(used: number, total: number): number {
    return Math.round((used / total) * 100);
  }

  getUsageColor(percentage: number): string {
    if (percentage >= 90) return 'bg-red-500';
    if (percentage >= 75) return 'bg-yellow-500';
    return 'bg-green-500';
  }

  private initializeCharts(): void {
    this.createCpuChart();
    this.createMemoryChart();
    this.createPodChart();
  }

  private updateCharts(): void {
    if (this.cpuChart) {
      this.cpuChart.destroy();
    }
    if (this.memoryChart) {
      this.memoryChart.destroy();
    }
    if (this.podChart) {
      this.podChart.destroy();
    }
    this.initializeCharts();
  }

  private createCpuChart(): void {
    if (!this.cpuChartCanvas) return;

    const ctx = this.cpuChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
        datasets: [
          {
            label: 'CPU Usage (cores)',
            data: [18.5, 22.3, 28.1, 32.4, 29.7, 24.8, 20.2],
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
        plugins: {
          legend: {
            display: false,
          },
        },
        scales: {
          y: {
            beginAtZero: true,
            max: 48,
          },
        },
      },
    };

    this.cpuChart = new Chart(ctx, config);
  }

  private createMemoryChart(): void {
    if (!this.memoryChartCanvas) return;

    const ctx = this.memoryChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'line',
      data: {
        labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
        datasets: [
          {
            label: 'Memory Usage (GB)',
            data: [72.4, 78.2, 95.3, 108.7, 98.4, 89.6, 82.1],
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
        plugins: {
          legend: {
            display: false,
          },
        },
        scales: {
          y: {
            beginAtZero: true,
            max: 192,
          },
        },
      },
    };

    this.memoryChart = new Chart(ctx, config);
  }

  private createPodChart(): void {
    if (!this.podChartCanvas) return;

    const ctx = this.podChartCanvas.nativeElement.getContext('2d');
    if (!ctx) return;

    const config: ChartConfiguration = {
      type: 'bar',
      data: {
        labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
        datasets: [
          {
            label: 'Pod Count',
            data: [142, 138, 165, 189, 178, 156, 149],
            backgroundColor: 'rgba(245, 158, 11, 0.8)',
            borderColor: 'rgb(245, 158, 11)',
            borderWidth: 1,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            display: false,
          },
        },
        scales: {
          y: {
            beginAtZero: true,
            max: 330,
          },
        },
      },
    };

    this.podChart = new Chart(ctx, config);
  }
}
