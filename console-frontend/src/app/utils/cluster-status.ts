import { create } from '@bufbuild/protobuf';
import { firstValueFrom, type Observable } from 'rxjs';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { GetClusterRequestSchema } from '../../generated/v1/cluster_pb';
import type { GetClusterResponse } from '../../generated/v1/cluster_pb';

interface ClusterClient {
  getCluster(request: { clusterId: string }): Observable<GetClusterResponse>;
}

export async function fetchClusterName(
  client: ClusterClient,
  clusterId: string,
): Promise<string | null> {
  try {
    const request = create(GetClusterRequestSchema, { clusterId });
    const response = await firstValueFrom(client.getCluster(request));
    return response.cluster?.name ?? null;
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load cluster name:', error);
    return null;
  }
}

export async function fetchClusterDetails(
  client: ClusterClient,
  clusterId: string,
): Promise<{ name: string | null; status: ClusterStatus }> {
  try {
    const request = create(GetClusterRequestSchema, { clusterId });
    const response = await firstValueFrom(client.getCluster(request));
    return {
      name: response.cluster?.name ?? null,
      status: response.cluster?.status ?? ClusterStatus.UNSPECIFIED,
    };
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load cluster details:', error);
    return { name: null, status: ClusterStatus.UNSPECIFIED };
  }
}

const TRANSITIONAL_STATUSES: ReadonlySet<ClusterStatus> = new Set([
  ClusterStatus.PROVISIONING,
  ClusterStatus.STARTING,
  ClusterStatus.UPGRADING,
  ClusterStatus.STOPPING,
  ClusterStatus.DELETING,
]);

export function isTransitionalStatus(status: ClusterStatus): boolean {
  return TRANSITIONAL_STATUSES.has(status);
}

/**
 * `color` for the `nldd-badge` that shows this cluster status.
 *
 * Transitional states get their own Rijkskleur rather than `warning`: a cluster
 * that is provisioning or starting is not in trouble. `warning` and `critical`
 * are reserved for states that need attention.
 */
export function getStatusBadgeColor(status: ClusterStatus): string {
  const colors: Record<ClusterStatus, string> = {
    [ClusterStatus.PROVISIONING]: 'mintgroen',
    [ClusterStatus.STARTING]: 'hemelblauw',
    [ClusterStatus.RUNNING]: 'success',
    [ClusterStatus.UPGRADING]: 'paars',
    [ClusterStatus.ERROR]: 'critical',
    [ClusterStatus.STOPPING]: 'oranje',
    [ClusterStatus.STOPPED]: 'neutral',
    [ClusterStatus.UNSPECIFIED]: 'neutral',
    [ClusterStatus.DELETING]: 'robijnrood',
    [ClusterStatus.UNHEALTHY]: 'critical',
    [ClusterStatus.LOST]: 'critical',
  };
  return colors[status];
}

export function getStatusLabel(status: ClusterStatus): string {
  const labels: Record<ClusterStatus, string> = {
    [ClusterStatus.PROVISIONING]: 'Provisioning',
    [ClusterStatus.STARTING]: 'Starting',
    [ClusterStatus.RUNNING]: 'Running',
    [ClusterStatus.UPGRADING]: 'Updating',
    [ClusterStatus.ERROR]: 'Error',
    [ClusterStatus.STOPPING]: 'Stopping',
    [ClusterStatus.STOPPED]: 'Stopped',
    [ClusterStatus.UNSPECIFIED]: 'Unknown status',
    [ClusterStatus.DELETING]: 'Deleting',
    [ClusterStatus.UNHEALTHY]: 'Unhealthy',
    [ClusterStatus.LOST]: 'Lost',
  };
  return labels[status];
}

const KUBECONFIG_STATUSES: ReadonlySet<ClusterStatus> = new Set([
  ClusterStatus.PROVISIONING,
  ClusterStatus.STARTING,
  ClusterStatus.RUNNING,
  ClusterStatus.UPGRADING,
  ClusterStatus.UNHEALTHY,
  ClusterStatus.ERROR,
]);

/**
 * Statuses in which the shoot is ready and its API server reachable:
 * `RUNNING`; `STARTING` while a new cluster's health checks still settle;
 * `UPGRADING` while an update rolls out on the ready cluster; and
 * `UNHEALTHY` while a ready cluster's health conditions fail (a node pool
 * without machines, say) — the control plane serves in all four. The same set
 * gates everything that talks to the cluster's API server, such as plugin
 * management.
 */
const REACHABLE_STATUSES: ReadonlySet<ClusterStatus> = new Set([
  ClusterStatus.RUNNING,
  ClusterStatus.STARTING,
  ClusterStatus.UPGRADING,
  ClusterStatus.UNHEALTHY,
]);

export function isClusterReachable(status: ClusterStatus): boolean {
  return REACHABLE_STATUSES.has(status);
}

/**
 * Whether a kubeconfig can be downloaded for a cluster in this status.
 *
 * The kubeconfig only names the cluster and the kube-api-proxy, so
 * organization-api hands it out from creation on and only refuses a deleted
 * cluster. A cluster on its way out, a lost one, or one whose status this
 * console does not know (an unspecified or newer value), gets no download.
 */
export function isKubeconfigAvailable(status: ClusterStatus): boolean {
  return KUBECONFIG_STATUSES.has(status);
}

/**
 * The tooltip on the kubeconfig download, or null when there is nothing to
 * add. While a cluster is still coming up the file works, but kubectl only
 * gets answers once the API server is reachable.
 */
export function kubeconfigHint(status: ClusterStatus): string | null {
  const waiting = "kubectl works once the cluster's API server is up";
  const removed = 'Not available for a cluster that is being removed';
  const hints: Record<ClusterStatus, string | null> = {
    [ClusterStatus.PROVISIONING]: waiting,
    [ClusterStatus.STARTING]: waiting,
    // Usually a running cluster whose last operation failed: its API server
    // tends to answer, and the badge already says what is wrong.
    [ClusterStatus.ERROR]: null,
    [ClusterStatus.RUNNING]: null,
    [ClusterStatus.UPGRADING]: null,
    // Ready underneath: the API server answers, the badge says what is wrong.
    [ClusterStatus.UNHEALTHY]: null,
    [ClusterStatus.STOPPING]: removed,
    [ClusterStatus.STOPPED]: removed,
    [ClusterStatus.DELETING]: removed,
    [ClusterStatus.LOST]: 'Not available while the cluster no longer exists',
    [ClusterStatus.UNSPECIFIED]: 'Available once the cluster status is known',
  };
  // A status newer than this console has no entry.
  return status in hints ? hints[status] : hints[ClusterStatus.UNSPECIFIED];
}

/**
 * Whether a cluster's status should keep the page polling. Transitional
 * statuses move by themselves; unhealthy is not transitional (its badge does
 * not pulse), but Gardener keeps retrying behind it, so recovery has to be
 * observed without a manual reload.
 */
export function shouldPollStatus(status: ClusterStatus): boolean {
  return isTransitionalStatus(status) || status === ClusterStatus.UNHEALTHY;
}
