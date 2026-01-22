import { ClusterStatus } from '../../generated/v1/common_pb';

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
  };
  return labels[status];
}
