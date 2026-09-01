import {
  Component,
  inject,
  signal,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
  ChangeDetectorRef,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { CLUSTER, METRICS, NAMESPACE, PLUGIN } from '../../connect/tokens';
import {
  GetClusterRequestSchema,
  ListNodePoolsRequestSchema,
  DeleteClusterRequestSchema,
  GetClusterActivityRequestSchema,
  GetKubeconfigRequestSchema,
  NodePool,
  type ClusterEvent,
  type SyncState,
} from '../../generated/v1/cluster_pb';
import { ListClusterNamespacesRequestSchema, Namespace } from '../../generated/v1/namespace_pb';
import { GetClusterWorkloadMetricsRequestSchema } from '../../generated/v1/metrics_pb';
import { OrganizationDataService } from '../organization-data.service';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { ClusterStatus, NodePoolStatus } from '../../generated/v1/common_pb';
import { LoadingIndicatorComponent } from '../icons';
import {
  getStatusTagColor,
  getStatusLabel,
  isKubeconfigAvailable,
  isTransitionalStatus,
} from '../utils/cluster-status';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';
import { getUsagePercentage, getUsageColor } from '../utils/usage';

interface ClusterResourceUsage {
  cpu: { used: number; total: number; unit: string };
  memory: { used: number; total: number; unit: string };
  pods: { used: number; total: number; unit: string };
}

const round1 = (n: number): number => Math.round(n * 10) / 10;

const getNodePoolStatusLabel = (status: NodePoolStatus): string => {
  const labels: Record<NodePoolStatus, string> = {
    [NodePoolStatus.UNSPECIFIED]: 'Unknown status',
    [NodePoolStatus.HEALTHY]: 'Healthy',
    [NodePoolStatus.DEGRADED]: 'Degraded',
    [NodePoolStatus.UNHEALTHY]: 'Unhealthy',
  };
  return labels[status];
};

const getSyncStatusTagColor = (status: string | undefined): string => {
  const colors: Record<string, string> = {
    ready: 'success',
    progressing: 'mintgroen',
    pending: 'lichtblauw',
    error: 'critical',
    deleting: 'oranje',
  };
  return colors[status ?? ''] || 'neutral';
};

const getSyncStatusLabel = (syncState: SyncState | null): string => {
  if (!syncState) return 'Unknown';
  if (syncState.shootStatus) return syncState.shootStatus;
  if (syncState.outboxError) return 'Error';
  if (syncState.outboxStatus === 'completed') return 'Synced';
  if (syncState.outboxStatus === 'failed') return 'Failed';
  if (syncState.outboxStatus) return 'Syncing';
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
    sync_failed: 'bg-danger-500',
    status_progressing: 'bg-blue-500',
    status_ready: 'bg-green-500',
    status_error: 'bg-danger-500',
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
  imports: [RouterLink, LoadingIndicatorComponent, DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cluster-details.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ClusterDetailsComponent implements OnInit, OnDestroy {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private metricsClient = inject(METRICS);

  private namespaceClient = inject(NAMESPACE);

  private organizationDataService = inject(OrganizationDataService);

  private pluginClient = inject(PLUGIN);

  private toastService = inject(ToastService);

  private pluginInstallationService = inject(PluginInstallationService);

  private cdr = inject(ChangeDetectorRef);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  private usageRetryTimer: ReturnType<typeof setInterval> | null = null;

  // Expose enums for use in template
  NodePoolStatus = NodePoolStatus;

  ClusterStatus = ClusterStatus;

  // Expose utility functions for template
  getStatusTagColor = getStatusTagColor;

  getStatusLabel = getStatusLabel;

  errorMessage = signal<string | null>(null);

  isLoading = signal<boolean>(true);

  showDeleteModal = signal<boolean>(false);

  // Namespace management
  namespaces = signal<Namespace[]>([]);

  // True when the namespace list failed to load, so the count is unknown and
  // cluster deletion must not be unlocked on a falsely-empty list.
  namespacesLoadError = signal<boolean>(false);

  // Plugin data
  installedPlugins = signal<PluginSummary[]>([]);

  isLoadingPlugins = signal<boolean>(true);

  // Activity/Events data
  // Live usage totals from the cluster's per-shoot Prometheus (MetricsService);
  // null until real data is available (cluster not ready, or metrics backend
  // unreachable), which renders the card's fallback text.
  resourceUsage = signal<ClusterResourceUsage | null>(null);

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
    nodePools: [] as NodePool[],
    workerNodes: {
      nodeType: 'n1-standard-2 (2 vCPU, 7.5 GB RAM)',
      minAutoscaling: 1,
      maxAutoscaling: 5,
    },
  };

  ngOnDestroy() {
    this.stopPolling();
    this.stopUsageRetry();
  }

  async ngOnInit() {
    const clusterId = this.route.snapshot.params['id'];

    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const [response, nodePoolsResponse] = await Promise.all([
        firstValueFrom(this.client.getCluster(create(GetClusterRequestSchema, { clusterId }))),
        firstValueFrom(
          this.client.listNodePools(create(ListNodePoolsRequestSchema, { clusterId })),
        ),
      ]);

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
      this.clusterData.nodePools = nodePoolsResponse.nodePools;

      this.titleService.setTitle(response.cluster.name);

      // Fetch namespaces, plugins, and events in parallel
      await Promise.all([
        this.loadNamespaces(clusterId),
        this.loadInstalledPlugins(clusterId),
        this.loadClusterEvents(clusterId),
        this.loadResourceUsage(clusterId),
      ]);

      this.startPolling();
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

  private async pollClusterStatus() {
    const clusterId = this.clusterData.basics.id;
    try {
      const request = create(GetClusterRequestSchema, { clusterId });
      const response = await firstValueFrom(this.client.getCluster(request));

      if (!response.cluster) {
        this.handleClusterDeleted();
        return;
      }

      this.clusterData.status = response.cluster.status;
      this.clusterData.syncState = response.cluster.syncState ?? null;
      // The usage card loads once on init; when the cluster finishes
      // provisioning while the page is open, fetch it now instead of
      // requiring a page refresh.
      if (!this.resourceUsage() && !isTransitionalStatus(response.cluster.status)) {
        this.loadResourceUsage(clusterId);
      }
      this.cdr.markForCheck();
    } catch (error) {
      // Anything else (network blip, API rollout, expired session) is
      // transient as far as this page is concerned; the next tick retries.
      if (error instanceof ConnectError && error.code === Code.NotFound) {
        this.handleClusterDeleted();
      }
    }
  }

  private handleClusterDeleted() {
    this.stopPolling();
    this.toastService.success(`Cluster '${this.clusterData.basics.name}' has been deleted`);
    this.router.navigate(['/']);
  }

  // Poll in every status, not only transitional ones: a running cluster can
  // drop to ERROR or be upgraded, and an errored one recovers on its own.
  // Actions gated on status (kubeconfig download) rely on this staying fresh.
  private startPolling() {
    if (!this.pollingTimer) {
      this.pollingTimer = setInterval(() => this.pollClusterStatus(), 5000);
    }
  }

  private stopPolling() {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }

  readonly formatDate = formatDateTimeUtil;

  getUsagePercentage = getUsagePercentage;

  round1 = round1;

  getUsageColor = getUsageColor;

  openTerminal(): void {
    // Mock implementation - would open terminal in real app
    // eslint-disable-next-line no-console
    console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  }

  isDownloadingKubeconfig = signal<boolean>(false);

  canDownloadKubeconfig(): boolean {
    return isKubeconfigAvailable(this.clusterData.status);
  }

  async downloadKubeconfig(): Promise<void> {
    if (this.isDownloadingKubeconfig() || !this.canDownloadKubeconfig()) {
      return;
    }
    this.isDownloadingKubeconfig.set(true);
    try {
      const request = create(GetKubeconfigRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });
      const response = await firstValueFrom(this.client.getKubeconfig(request));

      const blob = new Blob([response.kubeconfigContent], { type: 'application/yaml' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `kubeconfig-${this.clusterData.basics.name}.yaml`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to download kubeconfig: ${error.message}`
          : 'Failed to download kubeconfig',
      );
    } finally {
      this.isDownloadingKubeconfig.set(false);
    }
  }

  getNodePoolStatusLabel = getNodePoolStatusLabel;

  deleteConfirmationInput = signal<string>('');

  /**
   * The full slug ("orgname/clustername") the user must type to confirm deletion.
   * Empty until both parts are known, so a partial slug can never be confirmed.
   */
  deleteConfirmationSlug(): string {
    const orgName = this.organizationDataService.organizations()[0]?.name;
    const clusterName = this.clusterData.basics.name;
    if (!orgName || !clusterName) return '';
    return `${orgName}/${clusterName}`;
  }

  isDeleteConfirmed(): boolean {
    const slug = this.deleteConfirmationSlug();
    return slug !== '' && this.deleteConfirmationInput().trim() === slug;
  }

  onDeleteConfirmationInput(event: Event): void {
    this.deleteConfirmationInput.set((event as CustomEvent<{ value: string }>).detail.value);
  }

  async deleteCluster(): Promise<void> {
    if (!this.isDeleteConfirmed()) {
      return;
    }
    try {
      const request = create(DeleteClusterRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });

      await firstValueFrom(this.client.deleteCluster(request));

      this.organizationDataService.removeCluster(this.clusterData.basics.id);
      this.showDeleteModal.set(false);
      this.toastService.info(`The cluster '${this.clusterData.basics.name}' is being deleted`);
      this.router.navigate(['/']);
    } catch (error) {
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
      this.namespacesLoadError.set(false);
    } catch (error) {
      this.namespacesLoadError.set(true);
      this.toastService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    }
  }

  // Load installed plugins for the cluster
  async loadInstalledPlugins(clusterId: string): Promise<void> {
    try {
      this.isLoadingPlugins.set(true);

      const [pluginsResponse, installations] = await Promise.all([
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        this.pluginInstallationService.listInstallations(clusterId).catch(() => []),
      ]);

      const installedNames = new Set(
        installations.map((item) => item.spec.definitionRef.pluginName),
      );
      this.installedPlugins.set(pluginsResponse.plugins.filter((p) => installedNames.has(p.name)));
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
  getSyncStatusTagColor = getSyncStatusTagColor;

  getSyncStatusLabel = getSyncStatusLabel;

  // Load cluster activity/events
  async loadResourceUsage(clusterId: string): Promise<void> {
    try {
      const request = create(GetClusterWorkloadMetricsRequestSchema, { clusterId });
      const response = await firstValueFrom(this.metricsClient.getClusterWorkloadMetrics(request));
      const t = response.totals;
      // An unavailable backend (cluster provisioning, monitoring stack
      // unreachable) or all-zero totals mean there is no measurement yet;
      // keep the fallback instead of rendering 0 / 0 bars.
      if (!t || response.metricsUnavailable || (!t.cpu?.total && !t.pods?.total)) {
        return;
      }
      this.resourceUsage.set({
        cpu: { used: t.cpu?.used ?? 0, total: t.cpu?.total ?? 0, unit: t.cpu?.unit ?? 'cores' },
        memory: {
          used: t.memory?.used ?? 0,
          total: t.memory?.total ?? 0,
          unit: t.memory?.unit ?? 'GiB',
        },
        pods: { used: t.pods?.used ?? 0, total: t.pods?.total ?? 0, unit: t.pods?.unit ?? 'pods' },
      });
    } catch {
      // Non-fatal: the card falls back to its "not available" text.
    } finally {
      this.updateUsageRetry(clusterId);
    }
  }

  /**
   * A RUNNING cluster whose metrics backend is transiently unreachable would
   * otherwise show the fallback text until a full page reload: retry slowly
   * until a measurement arrives. Transitional clusters are covered by the 5s
   * status poll, which fetches usage once they finish provisioning.
   */
  private updateUsageRetry(clusterId: string): void {
    if (this.resourceUsage()) {
      this.stopUsageRetry();
      return;
    }
    if (!this.usageRetryTimer && !isTransitionalStatus(this.clusterData.status)) {
      this.usageRetryTimer = setInterval(() => {
        this.loadResourceUsage(clusterId);
      }, 30_000);
    }
  }

  private stopUsageRetry(): void {
    if (this.usageRetryTimer) {
      clearInterval(this.usageRetryTimer);
      this.usageRetryTimer = null;
    }
  }

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

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    this.errorMessage.set(null);
    this.deleteConfirmationInput.set('');
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
