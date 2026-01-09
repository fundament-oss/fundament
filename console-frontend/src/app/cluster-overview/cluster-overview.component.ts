import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { GetClusterRequestSchema } from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import {
  EditIconComponent,
  TerminalIconComponent,
  DownloadIconComponent,
  UpgradeIconComponent,
  ErrorIconComponent,
} from '../icons';

@Component({
  selector: 'app-cluster-overview',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    EditIconComponent,
    TerminalIconComponent,
    DownloadIconComponent,
    UpgradeIconComponent,
    ErrorIconComponent,
  ],
  templateUrl: './cluster-overview.component.html',
})
export class ClusterOverviewComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private client = inject(CLUSTER);

  errorMessage = signal<string | null>(null);
  isLoading = signal<boolean>(true);

  // Cluster data with API-fetched and mock data
  clusterData = {
    basics: {
      id: '',
      name: '',
      region: '',
      kubernetesVersion: '',
    },
    status: '',
    creationDate: '2024-11-15T10:30:00Z', // Mock data - not available from API
    activity: [
      {
        timestamp: '2024-12-06T14:30:00Z',
        action: 'Node pool scaled up',
        details: 'Added 2 nodes to default pool',
      },
      {
        timestamp: '2024-12-06T12:15:00Z',
        action: 'Plugin updated',
        details: 'Updated monitoring plugin to v2.1.3',
      },
      {
        timestamp: '2024-12-04T11:10:00Z',
        action: 'Node maintenance',
        details: 'Completed maintenance on node-3',
      },
      {
        timestamp: '2024-12-03T08:40:00Z',
        action: 'Resource limit adjusted',
        details: 'Increased memory limit for database pod',
      },
      {
        timestamp: '2024-12-02T13:55:00Z',
        action: 'User access granted',
        details: 'Added developer@company.com to cluster',
      },
      {
        timestamp: '2024-12-01T10:15:00Z',
        action: 'Monitoring alert resolved',
        details: 'High CPU usage alert cleared',
      },
    ],
    members: [
      { name: 'John Doe', role: 'admin', lastActive: '2024-12-06T14:20:00Z' },
      { name: 'Jane Smith', role: 'edit', lastActive: '2024-12-06T11:45:00Z' },
      { name: 'Mike Johnson', role: 'view', lastActive: '2024-12-05T16:30:00Z' },
      { name: 'Sarah Wilson', role: 'edit', lastActive: '2024-12-04T09:15:00Z' },
    ],
    nodePools: [
      {
        name: 'default-pool',
        nodeType: 'n1-standard-2',
        currentNodes: 3,
        minNodes: 1,
        maxNodes: 5,
        status: 'healthy',
        version: '1.34.2',
      },
      {
        name: 'high-memory-pool',
        nodeType: 'n1-highmem-4',
        currentNodes: 2,
        minNodes: 0,
        maxNodes: 3,
        status: 'healthy',
        version: '1.34.2',
      },
    ],
    resourceUsage: {
      cpu: { used: 2.4, limit: 8.0, unit: 'cores' },
      memory: { used: 12.8, limit: 32.0, unit: 'GB' },
      disk: { used: 45.2, limit: 100.0, unit: 'GB' },
      pods: { used: 28, limit: 110, unit: 'pods' },
    },
    workerNodes: {
      nodeType: 'n1-standard-2 (2 vCPU, 7.5 GB RAM)',
      minAutoscaling: 1,
      maxAutoscaling: 5,
    },
    plugins: {
      preset: 'Haven+ preset',
      description: 'Includes monitoring, logging, security scanning, and backup solutions',
    },
    projects: [
      {
        name: 'my-project-1',
        namespaces: ['default'],
      },
      {
        name: 'my-project-2',
        namespaces: ['abc', 'xyz'],
      },
    ],
  };

  async ngOnInit() {
    const clusterId = this.route.snapshot.params['id'];

    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(GetClusterRequestSchema, { clusterId });
      const response = await firstValueFrom(this.client.getCluster(request));

      if (!response.cluster) {
        throw new Error('Cluster not found');
      }

      // Update cluster data with API response
      this.clusterData.basics = {
        id: response.cluster.id,
        name: response.cluster.name,
        region: response.cluster.region,
        kubernetesVersion: response.cluster.kubernetesVersion,
      };
      this.clusterData.status = response.cluster.status.toString();

      this.titleService.setTitle(response.cluster.name);
    } catch (error) {
      console.error('Failed to fetch cluster data:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load cluster: ${error.message}`
          : 'Failed to load cluster data',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  getStatusColor(status: string): string {
    const colors: Record<string, string> = {
      provisioning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
      starting: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
      running: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
      upgrading: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-200',
      error: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200',
      stopping: 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200',
      stopped: 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200',
    };
    return colors[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200';
  }

  formatDate(dateString: string): string {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  getUsagePercentage(used: number, limit: number): number {
    return Math.round((used / limit) * 100);
  }

  getUsageColor(percentage: number): string {
    if (percentage >= 90) return 'bg-red-500';
    if (percentage >= 75) return 'bg-yellow-500';
    return 'bg-green-500';
  }

  openTerminal(): void {
    // Mock implementation - would open terminal in real app
    console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  }

  downloadKubeconfig(): void {
    // Mock implementation - would download kubeconfig in real app
    console.log('Downloading kubeconfig for cluster:', this.clusterData.basics.name);
  }
}
