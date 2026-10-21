import { ClusterStatus } from '../../generated/v1/common_pb';
import {
  getStatusBadgeColor,
  getStatusLabel,
  isKubeconfigAvailable,
  kubeconfigHint,
} from './cluster-status';

describe('isKubeconfigAvailable', () => {
  it('offers a kubeconfig from creation on', () => {
    [
      ClusterStatus.PROVISIONING,
      ClusterStatus.STARTING,
      ClusterStatus.RUNNING,
      ClusterStatus.UPGRADING,
      ClusterStatus.UNHEALTHY,
      ClusterStatus.ERROR,
    ].forEach((status) => {
      expect(isKubeconfigAvailable(status)).toBe(true);
    });
  });

  it('withholds it from a cluster that is being removed or unknown', () => {
    [
      ClusterStatus.UNSPECIFIED,
      ClusterStatus.STOPPING,
      ClusterStatus.STOPPED,
      ClusterStatus.DELETING,
    ].forEach((status) => {
      expect(isKubeconfigAvailable(status)).toBe(false);
    });
  });
});

describe('isKubeconfigAvailable with a status this console does not know', () => {
  it('fails closed', () => {
    const newer = 99 as ClusterStatus;
    expect(isKubeconfigAvailable(newer)).toBe(false);
    expect(kubeconfigHint(newer)).toBe('Available once the cluster status is known');
  });
});

describe('kubeconfigHint', () => {
  it('warns that kubectl waits for the API server while the cluster comes up', () => {
    expect(kubeconfigHint(ClusterStatus.PROVISIONING)).toContain('API server');
  });

  it('says nothing for a running cluster', () => {
    expect(kubeconfigHint(ClusterStatus.RUNNING)).toBeNull();
    expect(kubeconfigHint(ClusterStatus.UPGRADING)).toBeNull();
  });

  it('does not claim the API server is down for a cluster in error', () => {
    expect(isKubeconfigAvailable(ClusterStatus.ERROR)).toBe(true);
    expect(kubeconfigHint(ClusterStatus.ERROR)).toBeNull();
  });

  it('explains every unavailable status', () => {
    [
      ClusterStatus.UNSPECIFIED,
      ClusterStatus.STOPPING,
      ClusterStatus.STOPPED,
      ClusterStatus.DELETING,
    ].forEach((status) => {
      expect(isKubeconfigAvailable(status)).toBe(false);
      expect(kubeconfigHint(status)).not.toBeNull();
    });
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
