import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
  ChangeDetectorRef,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
  effect,
} from '@angular/core';
import { RouterOutlet, ActivatedRoute, Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { CLUSTER, NAMESPACE, PLUGIN } from '../../connect/tokens';
import {
  GetClusterRequestSchema,
  ListNodePoolsRequestSchema,
  DeleteClusterRequestSchema,
  GetClusterActivityRequestSchema,
  GetKubeconfigRequestSchema,
  GetClusterMetricsCredentialsRequestSchema,
  NodePool,
  type ClusterEvent,
  type SyncState,
} from '../../generated/v1/cluster_pb';
import { ListClusterNamespacesRequestSchema, Namespace } from '../../generated/v1/namespace_pb';
import { OrganizationDataService } from '../organization-data.service';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { ClusterStatus, NodePoolStatus } from '../../generated/v1/common_pb';
import { getStatusTagColor, getStatusLabel, isTransitionalStatus } from '../utils/cluster-status';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';

const getUsagePercentage = (used: number, limit: number): number =>
  Math.round((used / limit) * 100);

const getUsageColor = (percentage: number): string => {
  if (percentage >= 90) return 'critical';
  if (percentage >= 75) return 'warning';
  return 'success';
};

interface ResourceMetric {
  label: string;
  color: string;
  used: number;
  limit: number;
  valueText: string;
  accessibleLabel: string;
}

/** `plugin.name` is the install identifier (e.g. "openfsc") that names the
 *  PluginInstallation resource, so it is never what a user should read. */
const pluginDisplayName = (plugin: PluginSummary): string => plugin.displayName || plugin.name;

const pluginIconSrc = (plugin: PluginSummary): string =>
  `/img/plugins/${plugin.name.toLowerCase().replace(/[^a-z]+/g, '-')}.svg`;

const getNodePoolStatusLabel = (status: NodePoolStatus): string => {
  const labels: Record<NodePoolStatus, string> = {
    [NodePoolStatus.UNSPECIFIED]: 'Unknown status',
    [NodePoolStatus.HEALTHY]: 'Healthy',
    [NodePoolStatus.DEGRADED]: 'Degraded',
    [NodePoolStatus.UNHEALTHY]: 'Unhealthy',
  };
  return labels[status];
};

/** Takes the whole syncState, not just the shoot status: "Unknown" means we have
 *  no sync record at all, and not knowing whether a cluster is reconciling is a
 *  warning rather than a neutral fact. */
const getSyncStatusTagColor = (syncState: SyncState | null): string => {
  if (!syncState) return 'warning';

  const colors: Record<string, string> = {
    ready: 'success',
    progressing: 'mintgroen',
    pending: 'lichtblauw',
    error: 'critical',
    deleting: 'oranje',
  };
  return colors[syncState.shootStatus ?? ''] || 'neutral';
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

/** Only failures get tinted. On a timeline every dot is the same neutral track
 *  color, so the row itself has to carry the one distinction that matters — and
 *  the event label names it too, so color is never the sole signal. */
const getEventTypeColor = (eventType: string): string =>
  eventType === 'sync_failed' || eventType === 'status_error' ? 'critical' : 'default';

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
  imports: [RouterOutlet, DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cluster-details.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ClusterDetailsComponent implements OnInit, OnDestroy {
  protected pageNav = inject(PageNavService);

  /**
   * nldd-top-title-bar resolves `collapse-anchor` once, when the attribute
   * changes, and never retries. The heading it points at only renders once the
   * cluster has loaded, so at that moment there is nothing to find and the bar
   * never wires up its scroll listener. Re-set the attribute when the content
   * appears, so it resolves against a DOM that now has the heading.
   */
  private rearmCollapseAnchor = effect(() => {
    if (this.isLoading()) return;
    queueMicrotask(() => {
      const bar = document.querySelector('nldd-page > nldd-top-title-bar');
      const anchor = bar?.getAttribute('collapse-anchor');
      if (!bar || !anchor) return;
      bar.removeAttribute('collapse-anchor');
      (bar as HTMLElement & { updateComplete?: Promise<unknown> }).updateComplete?.then(() => {
        bar.setAttribute('collapse-anchor', anchor);
      });
    });
  });

  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private namespaceClient = inject(NAMESPACE);

  private organizationDataService = inject(OrganizationDataService);

  private pluginClient = inject(PLUGIN);

  private toastService = inject(ToastService);

  private pluginInstallationService = inject(PluginInstallationService);

  private cdr = inject(ChangeDetectorRef);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  // Expose enums for use in template
  NodePoolStatus = NodePoolStatus;

  ClusterStatus = ClusterStatus;

  // Expose utility functions for template
  getStatusTagColor = getStatusTagColor;

  getStatusLabel = getStatusLabel;

  errorMessage = signal<string | null>(null);

  isLoading = signal<boolean>(true);

  showDeleteModal = signal<boolean>(false);

  showDeleteBlockedModal = signal<boolean>(false);

  /** Why deletion is blocked, or null when it is not. */
  deleteBlockedReason = computed(() => {
    const count = this.namespaces().length;
    if (count > 0) {
      return `This cluster still has ${count} namespace${count === 1 ? '' : 's'}. Kubernetes will not release a cluster while namespaces are running on it, so remove them first and then delete the cluster.`;
    }
    if (this.namespacesLoadError()) {
      return "We couldn't load this cluster's namespaces, so there is no way to confirm it is empty. Reload the page and try again.";
    }
    return null;
  });

  /** The button stays enabled either way: blocked means "explain", not "ignore". */
  onDeleteClusterClick(): void {
    if (this.deleteBlockedReason()) {
      this.showDeleteBlockedModal.set(true);
      return;
    }
    this.showDeleteModal.set(true);
  }

  goToNamespaces(): void {
    this.showDeleteBlockedModal.set(false);
    this.pageNav.goTo(`/clusters/${this.clusterData.basics.id}/namespaces`);
  }

  showCredentialsModal = signal<boolean>(false);

  credentialsLoading = signal<boolean>(false);

  credentialsError = signal<string | null>(null);

  credentials = signal<{ username: string; password: string } | null>(null);

  // Tracks which field was just copied so we can flip the icon to a checkmark.
  copiedField = signal<'username' | 'password' | null>(null);

  // Namespace management
  namespaces = signal<Namespace[]>([]);

  // True when the namespace list failed to load, so the count is unknown and
  // cluster deletion must not be unlocked on a falsely-empty list.
  namespacesLoadError = signal<boolean>(false);

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
    observabilityUrl: '',
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

  ngOnDestroy() {
    this.stopPolling();
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
      this.clusterData.observabilityUrl = response.cluster.observabilityUrl;
      this.clusterData.nodePools = nodePoolsResponse.nodePools;

      this.titleService.setTitle(response.cluster.name);

      // Fetch namespaces, plugins, and events in parallel
      await Promise.all([
        this.loadNamespaces(clusterId),
        this.loadInstalledPlugins(clusterId),
        this.loadClusterEvents(clusterId),
      ]);

      this.updatePolling();
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
        // Cluster has been deleted
        this.stopPolling();
        this.toastService.success(`Cluster '${this.clusterData.basics.name}' has been deleted`);
        this.router.navigate(['/clusters']);
        return;
      }

      this.clusterData.status = response.cluster.status;
      this.clusterData.syncState = response.cluster.syncState ?? null;
      this.clusterData.observabilityUrl = response.cluster.observabilityUrl;
      this.cdr.markForCheck();
      this.updatePolling();
    } catch {
      // If the request fails with a not-found-like error, the cluster was deleted
      this.stopPolling();
      this.toastService.success(`Cluster '${this.clusterData.basics.name}' has been deleted`);
      this.router.navigate(['/clusters']);
    }
  }

  private updatePolling() {
    const needsPolling = isTransitionalStatus(this.clusterData.status);
    if (needsPolling && !this.pollingTimer) {
      this.pollingTimer = setInterval(() => this.pollClusterStatus(), 5000);
    } else if (!needsPolling && this.pollingTimer) {
      this.stopPolling();
    }
  }

  private stopPolling() {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
      this.pollingTimer = null;
    }
  }

  readonly formatDate = formatDateTimeUtil;

  /** The four usage bars, each carrying its own display and ARIA text. A getter
   *  rather than a computed: `clusterData` is a plain object the polling code
   *  mutates in place, so there is no signal to derive from. */
  /** The same bars, in twos, so the grid can put a pair in each column. */
  get resourceMetricPairs(): ResourceMetric[][] {
    const metrics = this.resourceMetrics;
    return metrics.reduce<ResourceMetric[][]>((pairs, metric, index) => {
      if (index % 2 === 0) pairs.push([metric]);
      else pairs[pairs.length - 1].push(metric);
      return pairs;
    }, []);
  }

  get resourceMetrics(): ResourceMetric[] {
    const usage = this.clusterData.resourceUsage;

    return [
      { label: 'CPU', ...usage.cpu },
      { label: 'Memory', ...usage.memory },
      { label: 'Disk', ...usage.disk },
      { label: 'Pods', ...usage.pods },
    ].map(({ label, used, limit, unit }) => {
      const percentage = getUsagePercentage(used, limit);

      return {
        label,
        used,
        limit,
        color: getUsageColor(percentage),
        valueText: `${used} / ${limit} ${unit} (${percentage}%)`,
        accessibleLabel: `${used} of ${limit} ${unit} used, ${percentage}%`,
      };
    });
  }

  openTerminal(): void {
    // Mock implementation - would open terminal in real app
    // eslint-disable-next-line no-console
    console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  }

  isDownloadingKubeconfig = signal<boolean>(false);

  async downloadKubeconfig(): Promise<void> {
    if (this.isDownloadingKubeconfig()) {
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
      this.router.navigate(['/clusters']);
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

  pluginDisplayName = pluginDisplayName;

  pluginIconSrc = pluginIconSrc;

  getEventDetails = getEventDetails;

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    this.errorMessage.set(null);
    this.deleteConfirmationInput.set('');
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  openObservabilityDashboard(): void {
    const url = this.clusterData.observabilityUrl;
    if (url) window.open(url, '_blank', 'noopener,noreferrer');
  }

  async openCredentialsModal(): Promise<void> {
    this.showCredentialsModal.set(true);
    this.credentialsError.set(null);
    this.copiedField.set(null);

    // Credentials are cached for the component lifetime; if Gardener rotates
    // them between reconciles, the user must refresh the page to see the new ones.
    if (this.credentials()) {
      return;
    }

    this.credentialsLoading.set(true);
    try {
      const request = create(GetClusterMetricsCredentialsRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });
      const response = await firstValueFrom(this.client.getClusterMetricsCredentials(request));
      this.credentials.set({ username: response.username, password: response.password });
    } catch (error) {
      this.credentialsError.set(
        error instanceof Error
          ? `Failed to load credentials: ${error.message}`
          : 'Failed to load credentials',
      );
    } finally {
      this.credentialsLoading.set(false);
    }
  }

  closeCredentialsModal(): void {
    this.showCredentialsModal.set(false);
  }

  async copyCredential(field: 'username' | 'password'): Promise<void> {
    const creds = this.credentials();
    if (!creds) return;
    try {
      await navigator.clipboard.writeText(creds[field]);
      this.copiedField.set(field);
      setTimeout(() => {
        if (this.copiedField() === field) {
          this.copiedField.set(null);
        }
      }, 1500);
    } catch {
      this.toastService.error('Failed to copy to clipboard');
    }
  }
}
