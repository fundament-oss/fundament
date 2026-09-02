import { ClusterStatus } from '../../generated/v1/common_pb';
import { isKubeconfigAvailable, isTransitionalStatus } from './cluster-status';

describe('isKubeconfigAvailable', () => {
  it('only offers a kubeconfig once the cluster is running', () => {
    expect(isKubeconfigAvailable(ClusterStatus.RUNNING)).toBe(true);
  });

  it('withholds the kubeconfig in every other status', () => {
    const others = [
      ClusterStatus.UNSPECIFIED,
      ClusterStatus.PROVISIONING,
      ClusterStatus.STARTING,
      ClusterStatus.UPGRADING,
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
