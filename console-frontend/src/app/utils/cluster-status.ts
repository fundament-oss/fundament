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
  };
  return labels[status];
}

/**
 * Statuses in which the shoot is ready and its API server reachable:
 * `RUNNING`; `STARTING` while a new cluster's health checks still settle;
 * `UPGRADING` while an update rolls out on the ready cluster; and
 * `UNHEALTHY` while a ready cluster's health conditions fail (a node pool
 * without machines, say) — the control plane serves in all four. Mirrors the
 * organization-api gate in `GetKubeconfig`, which keys on shoot status
 * `ready`; the same set gates everything that talks to the cluster's API
 * server, such as plugin management.
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

export function isKubeconfigAvailable(status: ClusterStatus): boolean {
  return isClusterReachable(status);
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
