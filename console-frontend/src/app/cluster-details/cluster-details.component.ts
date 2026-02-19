import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerTerminal,
  tablerDownload,
  tablerArrowUp,
  tablerCaretRight,
  tablerPencil,
  tablerAlertTriangle,
} from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { CLUSTER, NAMESPACE, PROJECT, PLUGIN } from '../../connect/tokens';
import {
  GetClusterRequestSchema,
  ListNodePoolsRequestSchema,
  DeleteClusterRequestSchema,
  ListInstallsRequestSchema,
  GetClusterActivityRequestSchema,
  NodePool,
  type ClusterEvent,
  type SyncState,
} from '../../generated/v1/cluster_pb';
import {
  ListClusterNamespacesRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import { ClusterStatus, NodePoolStatus } from '../../generated/v1/common_pb';
import { LoadingIndicatorComponent } from '../icons';
import { getStatusColor, getStatusLabel } from '../utils/cluster-status';
import ModalComponent from '../modal/modal.component';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';

const getUsagePercentage = (used: number, limit: number): number =>
  Math.round((used / limit) * 100);

const getUsageColor = (percentage: number): string => {
  if (percentage >= 90) return 'bg-red-500';
  if (percentage >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
};

const getNodePoolStatusLabel = (status: NodePoolStatus): string => {
  const labels: Record<NodePoolStatus, string> = {
    [NodePoolStatus.UNSPECIFIED]: 'Unknown status',
    [NodePoolStatus.HEALTHY]: 'Healthy',
    [NodePoolStatus.DEGRADED]: 'Degraded',
    [NodePoolStatus.UNHEALTHY]: 'Unhealthy',
  };
  return labels[status];
};

const getSyncStatusColor = (status: string | undefined): string => {
  const colors: Record<string, string> = {
    ready: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
    progressing: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
    pending: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
    error: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200',
    deleting: 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200',
  };
  return colors[status ?? ''] || 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200';
};

const getSyncStatusLabel = (syncState: SyncState | null): string => {
  if (!syncState) return 'Unknown';
  if (syncState.shootStatus) return syncState.shootStatus;
  if (syncState.syncedAt) return 'Synced';
  if (syncState.syncError) return 'Error';
  return 'Pending';
};

const getEventTypeLabel = (eventType: string): string => {
  const labels: Record<string, string> = {
    sync_requested: 'Sync requested',
    sync_claimed: 'Sync started',
    sync_succeeded: 'Sync completed',
    sync_failed: 'Sync failed',
    status_progressing: 'Cluster progressing',
    status_ready: 'Cluster ready',
    status_error: 'Cluster error',
    status_deleted: 'Cluster deleted',
  };
  return labels[eventType] || eventType;
};

const getEventTypeColor = (eventType: string): string => {
  const colors: Record<string, string> = {
    sync_requested: 'bg-blue-500',
    sync_claimed: 'bg-blue-500',
    sync_succeeded: 'bg-green-500',
    sync_failed: 'bg-red-500',
    status_progressing: 'bg-blue-500',
    status_ready: 'bg-green-500',
    status_error: 'bg-red-500',
    status_deleted: 'bg-gray-500',
  };
  return colors[eventType] || 'bg-gray-500';
};

const getEventDetails = (event: ClusterEvent): string => {
  if (event.message) {
    return event.message;
  }
  if (event.syncAction) {
    return `Action: ${event.syncAction}`;
  }
  if (event.attempt !== undefined) {
    return `Attempt ${event.attempt}`;
  }
  return '';
};

@Component({
  selector: 'app-cluster-details',
  imports: [RouterLink, NgIcon, LoadingIndicatorComponent, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerTerminal,
      tablerDownload,
      tablerArrowUp,
      tablerCaretRight,
      tablerPencil,
      tablerAlertTriangle,
    }),
  ],
  templateUrl: './cluster-details.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ClusterDetailsComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private namespaceClient = inject(NAMESPACE);

  private projectClient = inject(PROJECT);

  private pluginClient = inject(PLUGIN);

  private toastService = inject(ToastService);

  // Expose enum for use in template
  NodePoolStatus = NodePoolStatus;

  // Expose utility functions for template
  getStatusColor = getStatusColor;

  getStatusLabel = getStatusLabel;

  errorMessage = signal<string | null>(null);

  isLoading = signal<boolean>(true);

  showDeleteModal = signal<boolean>(false);

  // Namespace management
  namespaces = signal<Namespace[]>([]);

  projects = signal<Project[]>([]);

  // Plugin data
  installedPlugins = signal<PluginSummary[]>([]);

  isLoadingPlugins = signal<boolean>(true);

  // Activity/Events data
  clusterEvents = signal<ClusterEvent[]>([]);

  isLoadingEvents = signal<boolean>(true);

  // Cluster data with API-fetched and mock data
  clusterData = {
    basics: {
      id: '',
      name: '',
      region: '',
      kubernetesVersion: '',
    },
    status: ClusterStatus.UNSPECIFIED,
    syncState: null as SyncState | null,
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
    nodePools: [] as NodePool[],
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
      this.clusterData.status = response.cluster.status;
      this.clusterData.syncState = response.cluster.syncState ?? null;

      this.titleService.setTitle(response.cluster.name);

      // Fetch node pools
      const nodePoolsRequest = create(ListNodePoolsRequestSchema, { clusterId });
      const nodePoolsResponse = await firstValueFrom(this.client.listNodePools(nodePoolsRequest));

      // Map node pools to the expected format
      this.clusterData.nodePools = nodePoolsResponse.nodePools;

      // Fetch namespaces, projects, plugins, and events in parallel
      await Promise.all([
        this.loadNamespaces(clusterId),
        this.loadProjects(),
        this.loadInstalledPlugins(clusterId),
        this.loadClusterEvents(clusterId),
      ]);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load cluster: ${error.message}`
          : 'Failed to load cluster data',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  readonly formatDate = formatDateTimeUtil;

  getUsagePercentage = getUsagePercentage;

  getUsageColor = getUsageColor;

  openTerminal(): void {
    // Mock implementation - would open terminal in real app
    // eslint-disable-next-line no-console
    console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  }

  downloadKubeconfig(): void {
    // Mock implementation - would download kubeconfig in real app
    // eslint-disable-next-line no-console
    console.log('Downloading kubeconfig for cluster:', this.clusterData.basics.name);
  }

  getNodePoolStatusLabel = getNodePoolStatusLabel;

  async deleteCluster(): Promise<void> {
    try {
      const request = create(DeleteClusterRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });

      await firstValueFrom(this.client.deleteCluster(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`The cluster '${this.clusterData.basics.name}' is being deleted`);
      this.router.navigate(['/']);
    } catch (error) {
      this.showDeleteModal.set(false);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete cluster: ${error.message}`
          : 'Failed to delete cluster',
      );
    }
  }

  // Namespace management methods
  async loadNamespaces(clusterId: string): Promise<void> {
    try {
      const request = create(ListClusterNamespacesRequestSchema, { clusterId });
      const response = await firstValueFrom(this.namespaceClient.listClusterNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    }
  }

  async loadProjects(): Promise<void> {
    try {
      const request = create(ListProjectsRequestSchema, { clusterId: this.clusterData.basics.id });
      const response = await firstValueFrom(this.projectClient.listProjects(request));
      this.projects.set(response.projects);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load projects: ${error.message}`
          : 'Failed to load projects',
      );
    }
  }

  getProjectName(projectId: string): string {
    const project = this.projects().find((p) => p.id === projectId);
    return project?.name || projectId;
  }

  // Load installed plugins for the cluster
  async loadInstalledPlugins(clusterId: string): Promise<void> {
    try {
      this.isLoadingPlugins.set(true);

      // Fetch installs and all available plugins in parallel
      const [installsResponse, pluginsResponse] = await Promise.all([
        firstValueFrom(this.client.listInstalls(create(ListInstallsRequestSchema, { clusterId }))),
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
      ]);

      // Get the IDs of installed plugins
      const installedPluginIds = installsResponse.installs.map((install) => install.pluginId);

      // Filter the plugins to only include installed ones
      const installed = pluginsResponse.plugins.filter((plugin) =>
        installedPluginIds.includes(plugin.id),
      );

      this.installedPlugins.set(installed);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load installed plugins: ${error.message}`
          : 'Failed to load installed plugins',
      );
    } finally {
      this.isLoadingPlugins.set(false);
    }
  }

  // Sync status methods
  getSyncStatusColor = getSyncStatusColor;

  getSyncStatusLabel = getSyncStatusLabel;

  // Load cluster activity/events
  async loadClusterEvents(clusterId: string): Promise<void> {
    try {
      this.isLoadingEvents.set(true);
      const request = create(GetClusterActivityRequestSchema, { clusterId, limit: 20 });
      const response = await firstValueFrom(this.client.getClusterActivity(request));
      this.clusterEvents.set(response.events);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load cluster events:', error);
      // Don't show toast for events - it's not critical
    } finally {
      this.isLoadingEvents.set(false);
    }
  }

  getEventTypeLabel = getEventTypeLabel;

  getEventTypeColor = getEventTypeColor;

  getEventDetails = getEventDetails;
}
