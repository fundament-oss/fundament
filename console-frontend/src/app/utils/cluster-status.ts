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
  };
  return colors[status];
}

export function getStatusLabel(status: ClusterStatus): string {
  const labels: Record<ClusterStatus, string> = {
    [ClusterStatus.PROVISIONING]: 'Provisioning',
    [ClusterStatus.STARTING]: 'Starting',
    [ClusterStatus.RUNNING]: 'Running',
    [ClusterStatus.UPGRADING]: 'Upgrading',
    [ClusterStatus.ERROR]: 'Error',
    [ClusterStatus.STOPPING]: 'Stopping',
    [ClusterStatus.STOPPED]: 'Stopped',
    [ClusterStatus.UNSPECIFIED]: 'Unknown status',
    [ClusterStatus.DELETING]: 'Deleting',
  };
  return labels[status];
}
