import { ClusterStatus } from '../../generated/v1/common_pb';
import {
  getStatusBadgeColor,
  getStatusLabel,
  isKubeconfigAvailable,
  isTransitionalStatus,
} from './cluster-status';

describe('isKubeconfigAvailable', () => {
  it('offers a kubeconfig once the shoot is ready underneath', () => {
    const ready = [ClusterStatus.RUNNING, ClusterStatus.UPGRADING, ClusterStatus.UNHEALTHY];
    ready.forEach((status) => {
      expect(isKubeconfigAvailable(status)).toBe(true);
    });
  });

  it('withholds the kubeconfig in every other status', () => {
    const others = [
      ClusterStatus.UNSPECIFIED,
      ClusterStatus.PROVISIONING,
      ClusterStatus.STARTING,
      ClusterStatus.ERROR,
      ClusterStatus.STOPPING,
      ClusterStatus.STOPPED,
      ClusterStatus.DELETING,
    ];
    others.forEach((status) => {
      expect(isKubeconfigAvailable(status)).toBe(false);
    });
  });

  it('keeps polling while the cluster is still becoming available', () => {
    expect(isTransitionalStatus(ClusterStatus.PROVISIONING)).toBe(true);
    expect(isKubeconfigAvailable(ClusterStatus.PROVISIONING)).toBe(false);
  });
});

describe('getStatusBadgeColor', () => {
  it('marks an unhealthy cluster critical', () => {
    expect(getStatusBadgeColor(ClusterStatus.UNHEALTHY)).toBe('critical');
  });
});

describe('getStatusLabel', () => {
  it('labels unhealthy and upgrading clusters', () => {
    expect(getStatusLabel(ClusterStatus.UNHEALTHY)).toBe('Unhealthy');
    expect(getStatusLabel(ClusterStatus.UPGRADING)).toBe('Updating');
  });
});
