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

export function getStatusColor(status: ClusterStatus): string {
  const colors: Record<ClusterStatus, string> = {
    [ClusterStatus.PROVISIONING]: 'badge-yellow',
    [ClusterStatus.STARTING]: 'badge-blue',
    [ClusterStatus.RUNNING]: 'badge-green',
    [ClusterStatus.UPGRADING]: 'badge-purple',
    [ClusterStatus.ERROR]: 'badge-rose',
    [ClusterStatus.STOPPING]: 'badge-yellow',
    [ClusterStatus.STOPPED]: 'badge-gray',
    [ClusterStatus.UNSPECIFIED]: 'badge-gray',
    [ClusterStatus.DELETING]: 'badge-rose',
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
