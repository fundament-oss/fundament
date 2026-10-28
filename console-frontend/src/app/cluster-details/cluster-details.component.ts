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
import { NgTemplateOutlet } from '@angular/common';
import { RouterOutlet, ActivatedRoute, Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { type Timestamp } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';
import { firstValueFrom } from 'rxjs';
import { NodePoolNotCreated, describeRefusals } from '../utils/node-pool-refusal';
import PageNavService from '../page-nav.service';
import opensElsewhere from '../opens-elsewhere';
import { NotificationService } from '../notification.service';
import { CLUSTER, METRICS, PLUGIN, PROJECT } from '../../connect/tokens';
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
import { ListProjectsRequestSchema, type Project } from '../../generated/v1/project_pb';
import { GetClusterWorkloadMetricsRequestSchema } from '../../generated/v1/metrics_pb';
import { OrganizationDataService } from '../organization-data.service';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import { ClusterStatus, NodePoolStatus } from '../../generated/v1/common_pb';
import {
  getStatusBadgeColor,
  getStatusLabel,
  isKubeconfigAvailable,
  kubeconfigHint,
  isTransitionalStatus,
  shouldPollStatus,
} from '../utils/cluster-status';
import getPluginIconName from '../utils/plugin-icon-name';
import { PluginIconComponent } from '../icons';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import MockBadgeComponent from '../mock-badge/mock-badge.component';
import ClusterDefaultsBlockComponent from '../container-defaults/cluster-defaults-block.component';
import ClusterPluginResourcesBlockComponent from './cluster-plugin-resources-block.component';
import { ConfigService } from '../config.service';
import focusFirstModalInput from '../modal-focus';
import {
  formatDate as formatDateUtil,
  formatDateTime as formatDateTimeUtil,
  formatShortDateTime as formatShortDateTimeUtil,
  formatTime as formatTimeUtil,
} from '../utils/date-format';
import { formatUsageValue, getUsagePercentage } from '../utils/usage';
import formatEventMessage from '../utils/event-message';
import '@nldd/design-system/multi-line-text-field';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/badge';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/card';
import '@nldd/design-system/cell';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
import '@nldd/design-system/form-field';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/link';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/progress-bar';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/timeline-track-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/tooltip';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

interface ClusterResourceUsage {
  cpu: { used: number; total: number; unit: string };
  memory: { used: number; total: number; unit: string };
  pods: { used: number; total: number; unit: string };
}

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

const NODE_POOL_STATUS_LABELS: Record<NodePoolStatus, string> = {
  [NodePoolStatus.UNSPECIFIED]: 'Unknown status',
  [NodePoolStatus.HEALTHY]: 'Healthy',
  [NodePoolStatus.DEGRADED]: 'Degraded',
  [NodePoolStatus.UNHEALTHY]: 'Unhealthy',
  [NodePoolStatus.PROVISIONING]: 'Creating machines',
  [NodePoolStatus.WAITING_FOR_MACHINES]: 'Waiting for machines',
  [NodePoolStatus.FAILED]: 'Failed',
};

const getNodePoolStatusLabel = (status: NodePoolStatus): string => NODE_POOL_STATUS_LABELS[status];

// Hoisted: the template binds this per pool card, so it runs every change
// detection cycle on a page that polls.
const NODE_POOL_STATUS_COLORS: Record<NodePoolStatus, string> = {
  [NodePoolStatus.UNSPECIFIED]: 'neutral',
  [NodePoolStatus.HEALTHY]: 'success',
  [NodePoolStatus.DEGRADED]: 'warning',
  [NodePoolStatus.UNHEALTHY]: 'critical',
  [NodePoolStatus.PROVISIONING]: 'mintgroen',
  [NodePoolStatus.WAITING_FOR_MACHINES]: 'warning',
  [NodePoolStatus.FAILED]: 'critical',
};

const getNodePoolStatusColor = (status: NodePoolStatus): string => NODE_POOL_STATUS_COLORS[status];

/** Takes the whole syncState, not just the shoot status: "Unknown" means we have
 *  no sync record at all, and not knowing whether a cluster is reconciling is a
 *  warning rather than a neutral fact. */
const getSyncStatusBadgeColor = (syncState: SyncState | null): string => {
  if (!syncState) return 'warning';

  const colors: Record<string, string> = {
    ready: 'success',
    progressing: 'mintgroen',
    pending: 'lichtblauw',
    error: 'critical',
    lost: 'critical',
    deleting: 'oranje',
  };
  return colors[syncState.shootStatus ?? ''] || 'neutral';
};

/** Shoot states in which something is moving without anyone doing anything.
 *  The badge pulses for those, the same rule the cluster status above it
 *  follows: a ring for what is happening now, a still dot for what simply is.
 *  `pending` counts, because the work is queued and will start on its own;
 *  `error` does not, because nothing is going to move until someone acts. */
const MOVING_SHOOT_STATUSES: ReadonlySet<string> = new Set(['progressing', 'pending', 'deleting']);

const isSyncStatusMoving = (syncState: SyncState | null): boolean =>
  MOVING_SHOOT_STATUSES.has(syncState?.shootStatus ?? '');

const getSyncStatusLabel = (syncState: SyncState | null): string => {
  if (!syncState) return 'Unknown';
  // Every other branch here hands back a label that reads as one; the shoot
  // status comes straight from Gardener in lowercase. Capitalized here rather
  // than in CSS, so the value is the label wherever it ends up: a tooltip, a
  // screen reader, a copied line of text.
  if (syncState.shootStatus) {
    return syncState.shootStatus.charAt(0).toUpperCase() + syncState.shootStatus.slice(1);
  }
  if (syncState.outboxError) return 'Error';
  if (syncState.outboxStatus === 'completed') return 'Synced';
  if (syncState.outboxStatus === 'failed') return 'Failed';
  if (syncState.outboxMessage) return 'Waiting';
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
    status_healthy: 'Cluster healthy',
    status_unhealthy: 'Cluster unhealthy',
    status_warning: 'Cluster warning',
    status_lost: 'Cluster lost',
    status_api_available: 'API server available',
    user_sync_succeeded: 'Users synced',
    user_sync_failed: 'User sync failed',
    nodepool_waiting: 'Node pool waiting',
    nodepool_error: 'Node pool failed',
    nodepool_ready: 'Node pool ready',
    sync_waiting_for_precondition: 'Waiting',
  };
  return labels[eventType] || eventType;
};

const CRITICAL_EVENT_TYPES: ReadonlySet<string> = new Set([
  'sync_failed',
  'status_error',
  'status_unhealthy',
  'status_lost',
  'user_sync_failed',
  'nodepool_error',
]);

/** Something is amiss but in progress: Gardener is retrying it by itself, or
 *  the cluster waits for a precondition (such as its first node pool) and
 *  resumes on its own once that is met. A notch below critical. */
const WARNING_EVENT_TYPES: ReadonlySet<string> = new Set([
  'status_warning',
  'nodepool_waiting',
  'sync_waiting_for_precondition',
]);

/** Only problems get tinted. On a timeline every dot is the same neutral track
 *  color, so the row itself has to carry the one distinction that matters — and
 *  the event label names it too, so color is never the sole signal. */
const getEventTypeColor = (eventType: string): string => {
  if (CRITICAL_EVENT_TYPES.has(eventType)) return 'critical';
  if (WARNING_EVENT_TYPES.has(eventType)) return 'warning';
  return 'default';
};

const getEventDetails = (event: ClusterEvent): string => {
  if (event.message) {
    return formatEventMessage(event.message);
  }
  if (event.syncAction) {
    return `Action: ${event.syncAction}`;
  }
  if (event.attempt !== undefined) {
    return `Attempt ${event.attempt}`;
  }
  return '';
};

/** Label and detail on one line: "Sync completed: Shoot spec applied." The
 *  detail is the part you actually read, and as supporting text under a bold
 *  label it got the quieter of the two. Without a detail the label stands
 *  alone, so no line ends on a dangling colon. */
const getEventLine = (event: ClusterEvent): string => {
  const detail = getEventDetails(event);
  const label = getEventTypeLabel(event.eventType);
  return detail ? `${label}: ${detail}` : label;
};

@Component({
  selector: 'app-cluster-details',
  imports: [
    RouterOutlet,
    NgTemplateOutlet,
    DialogSyncDirective,
    SheetSyncDirective,
    PluginIconComponent,
    MockBadgeComponent,
    ClusterDefaultsBlockComponent,
    ClusterPluginResourcesBlockComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cluster-details.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ClusterDetailsComponent implements OnInit, OnDestroy {
  protected pageNav = inject(PageNavService);

  protected readonly mockMetrics = inject(ConfigService).getConfig().mockMetrics ?? false;

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

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private client = inject(CLUSTER);

  private metricsClient = inject(METRICS);

  private projectClient = inject(PROJECT);

  private organizationDataService = inject(OrganizationDataService);

  private pluginClient = inject(PLUGIN);

  private notificationService = inject(NotificationService);

  private pluginInstallationService = inject(PluginInstallationService);

  private cdr = inject(ChangeDetectorRef);

  private pollingTimer: ReturnType<typeof setInterval> | null = null;

  private usageRetryTimer: ReturnType<typeof setInterval> | null = null;

  // Expose enum for use in template
  ClusterStatus = ClusterStatus;

  // Expose utility functions for template
  getStatusBadgeColor = getStatusBadgeColor;

  isTransitionalStatus = isTransitionalStatus;

  getStatusLabel = getStatusLabel;

  errorMessage = signal<string | null>(null);

  /** Pools the form asked for and did not get. Handed over as navigation state
   *  by the summary, which cannot show it itself: it closes on the way here. */
  nodePoolsNotCreated = signal<NodePoolNotCreated[]>(
    (window.history.state as { nodePoolsNotCreated?: NodePoolNotCreated[] } | null)
      ?.nodePoolsNotCreated ?? [],
  );

  /** The cluster is there, so this is not a failure of the page you are on: it
   *  is one thing you asked for that is missing from it. */
  nodePoolsNotCreatedText = computed(() => {
    const pools = this.nodePoolsNotCreated();
    if (pools.length === 0) return null;
    if (pools.length === 1) return `Node pool '${pools[0].name}' was not created`;
    return `${pools.length} node pools were not created`;
  });

  /** Why, pool by pool: a pool refused for its quota is not one to add again,
   *  so each name comes with its own reason rather than a single piece of advice. */
  nodePoolsNotCreatedDetail = computed(() => {
    const pools = this.nodePoolsNotCreated();
    return pools.length > 0 ? describeRefusals(pools) : null;
  });

  isLoading = signal<boolean>(true);

  showDeleteModal = signal<boolean>(false);

  showDeleteBlockedModal = signal<boolean>(false);

  /** Why deletion is blocked, or null when it is not. The API refuses a
   *  cluster that still has projects, so they are what has to go first. The
   *  user is sent to each project rather than offered one confirmation that
   *  deletes them all: the API does not cascade (see DeleteCluster). */
  deleteBlockedReason = computed(() => {
    const count = this.projects().length;
    if (count > 0) {
      return count === 1
        ? 'This cluster still has a project. Delete it, and its namespaces, before you delete the cluster.'
        : `This cluster still has ${count} projects. Delete them, and their namespaces, before you delete the cluster.`;
    }
    if (this.projectsLoadError()) {
      return "We couldn't load this cluster's projects, so there is no way to confirm it is empty. Reload the page and try again.";
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

  /** Routes client-side while leaving the row a real link. A project opened in
   *  another tab leaves the dialog open, so the rest of the list stays at hand. */
  goToProject(event: Event, projectId: string): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.showDeleteBlockedModal.set(false);
    this.pageNav.goTo(`/projects/${projectId}/general`);
  }

  /** A download that produced no file leaves nothing on screen to show for it,
   *  so this says so where it cannot be missed. */
  kubeconfigError = signal<string | null>(null);

  // The projects on this cluster: each one blocks its deletion.
  projects = signal<Project[]>([]);

  // True when the project list failed to load, so the count is unknown and
  // cluster deletion must not be unlocked on a falsely-empty list.
  projectsLoadError = signal<boolean>(false);

  // Plugin data
  installedPlugins = signal<PluginSummary[]>([]);

  isLoadingPlugins = signal<boolean>(true);

  // Activity/Events data
  // Live usage totals from the cluster's per-shoot Prometheus (MetricsService);
  // null until real data is available (cluster not ready, or metrics backend
  // unreachable), which renders the card's fallback text.
  resourceUsage = signal<ClusterResourceUsage | null>(null);

  clusterEvents = signal<ClusterEvent[]>([]);

  /** How many fit on the page before the history starts to dominate it. The
   *  rest live one click away, in a sheet, rather than in a scroll region
   *  nested inside a page that already scrolls. */
  private static readonly EVENTS_ON_PAGE = 6;

  eventsOnPage = computed(() =>
    this.clusterEvents().slice(0, ClusterDetailsComponent.EVENTS_ON_PAGE),
  );

  hasMoreEvents = computed(
    () => this.clusterEvents().length > ClusterDetailsComponent.EVENTS_ON_PAGE,
  );

  showAllEventsSheet = signal(false);

  isLoadingEvents = signal<boolean>(true);

  // Cluster data as fetched from the API
  clusterData = {
    basics: {
      id: '',
      name: '',
      region: '',
      kubernetesVersion: '',
    },
    status: ClusterStatus.UNSPECIFIED,
    syncState: null as SyncState | null,
    created: undefined as Timestamp | undefined,
    nodePools: [] as NodePool[],
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
      this.clusterData.created = response.cluster.created;
      this.clusterData.nodePools = nodePoolsResponse.nodePools;

      // Fetch projects, plugins, and events in parallel
      await Promise.all([
        this.loadProjects(clusterId),
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

  /** Polling fires the pool and event refreshes every tick without awaiting
   *  them, so a slow response can land after a newer one; each request takes a
   *  number and only the latest may write. */
  private nodePoolsRequest = 0;

  private eventsRequest = 0;

  /** What the node pool cards show. Its own method because the sheet that edits
   *  the pools stands over this page rather than replacing it, so this page has
   *  to fetch them again on its own. */
  private async loadNodePools(clusterId: string): Promise<void> {
    this.nodePoolsRequest += 1;
    const request = this.nodePoolsRequest;
    try {
      const response = await firstValueFrom(
        this.client.listNodePools(create(ListNodePoolsRequestSchema, { clusterId })),
      );
      if (request !== this.nodePoolsRequest) return;
      this.clusterData.nodePools = response.nodePools;
      this.cdr.markForCheck();
    } catch {
      // The cards keep what they had: pools that were there a moment ago are
      // better than an empty box saying there are none.
    }
  }

  /** The revision the cards were built from. Seeded to the signal's own initial
   *  value, so mounting does not fetch a second time on top of ngOnInit. */
  private nodePoolsRevision = 0;

  private readonly reloadNodePools = effect(() => {
    const revision = this.organizationDataService.nodePoolsChanged();
    if (revision === this.nodePoolsRevision) return;
    this.nodePoolsRevision = revision;
    const clusterId = this.clusterData.basics.id;
    if (clusterId) this.loadNodePools(clusterId);
  });

  /** Counts status poll ticks, to refresh pools and events at a slower pace
   *  while the cluster sits still. */
  private pollTick = 0;

  private async pollClusterStatus() {
    const clusterId = this.clusterData.basics.id;
    try {
      const request = create(GetClusterRequestSchema, { clusterId });
      const response = await firstValueFrom(this.client.getCluster(request));

      if (!response.cluster) {
        this.handleClusterDeleted();
        return;
      }

      const statusChanged = this.clusterData.status !== response.cluster.status;
      this.clusterData.status = response.cluster.status;
      this.clusterData.syncState = response.cluster.syncState ?? null;
      this.clusterData.created = response.cluster.created;
      // Node pool statuses and the activity timeline move with the cluster (a
      // pool waiting for machines recovers, nodepool_* events arrive), so they
      // refresh on the same tick while something happens: a status change, or
      // a status that moves on its own. On a cluster sitting still they only
      // refresh every 6th tick — listNodePools fans out to Prometheus server
      // side, and an idle tab must not fire that every 5 seconds.
      this.pollTick += 1;
      if (statusChanged || shouldPollStatus(response.cluster.status) || this.pollTick % 6 === 0) {
        this.loadNodePools(clusterId).catch(() => {});
        this.loadClusterEvents(clusterId).catch(() => {});
      }
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
    this.notificationService.success(`Cluster '${this.clusterData.basics.name}' has been deleted`);
    this.pageNav.goTo('/clusters');
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

  /** The timeline splits them: the day in its own column, the clock beneath it. */
  readonly formatDay = formatDateUtil;

  readonly formatTimestamp = formatShortDateTimeUtil;

  readonly formatTime = formatTimeUtil;

  /** The same bars, in twos, so the grid can put a pair in each column. */
  get resourceMetricPairs(): ResourceMetric[][] {
    const metrics = this.resourceMetrics;
    return metrics.reduce<ResourceMetric[][]>((pairs, metric, index) => {
      if (index % 2 === 0) pairs.push([metric]);
      else pairs[pairs.length - 1].push(metric);
      return pairs;
    }, []);
  }

  /** A usage bar per measured resource, each carrying its own display and ARIA
   *  text. Empty until the cluster's own metrics backend answers, which it does
   *  not before the cluster is ready. */
  get resourceMetrics(): ResourceMetric[] {
    const usage = this.resourceUsage();
    if (!usage) return [];

    return [
      { label: 'CPU', ...usage.cpu },
      { label: 'Memory', ...usage.memory },
      { label: 'Pods', ...usage.pods },
    ].map(({ label, used, total, unit }) => {
      const percentage = getUsagePercentage(used, total);

      return {
        label,
        used,
        limit: total,
        color: getUsageColor(percentage),
        // Rounded for reading; `used`/`limit` above stay raw, because those
        // only set the bar's geometry.
        valueText: `${formatUsageValue(used)} / ${formatUsageValue(total)} ${unit} (${percentage}%)`,
        accessibleLabel: `${formatUsageValue(used)} of ${formatUsageValue(total)} ${unit} used, ${percentage}%`,
      };
    });
  }

  // Parked with the "Open terminal" button in the template, which was the only
  // caller. It logged to the console and opened nothing.
  // openTerminal(): void {
  //   console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  // }

  isDownloadingKubeconfig = signal<boolean>(false);

  canDownloadKubeconfig(): boolean {
    return isKubeconfigAvailable(this.clusterData.status);
  }

  kubeconfigHint(): string | null {
    return kubeconfigHint(this.clusterData.status);
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
      this.kubeconfigError.set(error instanceof Error ? error.message : 'The request failed.');
    } finally {
      this.isDownloadingKubeconfig.set(false);
    }
  }

  getNodePoolStatusLabel = getNodePoolStatusLabel;

  getNodePoolStatusColor = getNodePoolStatusColor;

  deleteConfirmationInput = signal<string>('');

  /**
   * Set by the delete button. The field is not wrong until you have asked for
   * the deletion: before that it is simply not filled in yet, and a button that
   * sits there dead says nothing about what is missing.
   */
  deleteAttempted = signal(false);

  /** True from the click until the page turns. The delete is followed by a
   *  reload of the cluster list, so the modal stays up long enough to be
   *  clicked a second time, and the second DeleteCluster comes back not found. */
  isDeleting = signal(false);

  deleteConfirmationInvalid = computed(() => this.deleteAttempted() && !this.isDeleteConfirmed());

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
    this.deleteAttempted.set(true);
    if (!this.isDeleteConfirmed() || this.isDeleting()) {
      return;
    }
    this.isDeleting.set(true);
    try {
      const request = create(DeleteClusterRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });

      await firstValueFrom(this.client.deleteCluster(request));

      // The cluster is soft-deleted, so the list still holds it, now DELETING.
      // Awaited before the page turns: the clusters page takes its list from the
      // cache as it mounts and only starts polling when something in that list
      // is still moving, so a reload landing after it would be both unseen and
      // too late to start the poll.
      await this.organizationDataService.reloadClusters({
        id: this.clusterData.basics.id,
        name: this.clusterData.basics.name,
        status: ClusterStatus.DELETING,
      });

      this.showDeleteModal.set(false);
      this.notificationService.info(
        `The cluster '${this.clusterData.basics.name}' is being deleted`,
      );
      this.pageNav.goTo('/clusters');
    } catch (error) {
      if (error instanceof ConnectError && error.code === Code.FailedPrecondition) {
        // A project was added after the page loaded: show what blocks the
        // deletion now, the same way the button does. That includes a reload
        // that failed, which would otherwise leave the old, empty list.
        await this.loadProjects(this.clusterData.basics.id);
        if (this.deleteBlockedReason()) {
          this.showDeleteModal.set(false);
          this.showDeleteBlockedModal.set(true);
        } else {
          // Its projects were deleted in the meantime, so a retry can succeed.
          this.errorMessage.set(
            `This cluster could not be deleted: ${error.rawMessage}. Try again.`,
          );
        }
        return;
      }
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete cluster: ${error.message}`
          : 'Failed to delete cluster',
      );
    } finally {
      this.isDeleting.set(false);
    }
  }

  async loadProjects(clusterId: string): Promise<void> {
    try {
      const request = create(ListProjectsRequestSchema, { clusterId });
      const response = await firstValueFrom(this.projectClient.listProjects(request));
      this.projects.set(response.projects);
      this.projectsLoadError.set(false);
    } catch (error) {
      this.projectsLoadError.set(true);
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load projects: ${error.message}`
          : 'Failed to load projects',
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
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load installed plugins: ${error.message}`
          : 'Failed to load installed plugins',
      );
    } finally {
      this.isLoadingPlugins.set(false);
    }
  }

  // Sync status methods
  getSyncStatusBadgeColor = getSyncStatusBadgeColor;

  getSyncStatusLabel = getSyncStatusLabel;

  isSyncStatusMoving = isSyncStatusMoving;

  /** Named like the other overlay that belongs to one cluster, "Plugins for X":
   *  both sheets can sit open beside another window, so they say which cluster
   *  they are about. Falls back to the bare noun while it is still loading. */
  eventHistoryTitle = (): string =>
    this.clusterData.basics.name
      ? `Event history for ${this.clusterData.basics.name}`
      : 'Event history';

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
    this.eventsRequest += 1;
    const requestNumber = this.eventsRequest;
    try {
      this.isLoadingEvents.set(true);
      const request = create(GetClusterActivityRequestSchema, { clusterId, limit: 20 });
      const response = await firstValueFrom(this.client.getClusterActivity(request));
      if (requestNumber !== this.eventsRequest) return;
      this.clusterEvents.set(response.events);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load cluster events:', error);
      // No notification for events - it's not critical
    } finally {
      if (requestNumber === this.eventsRequest) {
        this.isLoadingEvents.set(false);
      }
    }
  }

  getEventTypeLabel = getEventTypeLabel;

  getEventLine = getEventLine;

  getEventTypeColor = getEventTypeColor;

  pluginDisplayName = pluginDisplayName;

  pluginIconName = getPluginIconName;

  getEventDetails = getEventDetails;

  formatEventMessage = formatEventMessage;

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    this.errorMessage.set(null);
    this.deleteConfirmationInput.set('');
    this.deleteAttempted.set(false);
    this.isDeleting.set(false);
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  closeKubeconfigError(): void {
    this.kubeconfigError.set(null);
  }
}
