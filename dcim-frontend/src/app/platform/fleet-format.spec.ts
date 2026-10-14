import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { ClusterHealth, ClusterStatus, DriftKind } from '../../generated/fleet/v1/fleet_pb';
import {
  clusterTag,
  driftKindTag,
  formatCores,
  formatMemory,
  observed,
  poolFlag,
  timeAgo,
} from './fleet-format';

const now = new Date('2026-10-14T14:00:00Z');
const ago = (ms: number) => new Date(now.getTime() - ms);

describe('clusterTag', () => {
  it('reads health only for a running cluster', () => {
    expect(clusterTag({ status: ClusterStatus.RUNNING, health: ClusterHealth.HEALTHY })).toEqual({
      text: 'Healthy',
      color: 'success',
    });
    expect(
      clusterTag({ status: ClusterStatus.RUNNING, health: ClusterHealth.UNHEALTHY }).color,
    ).toBe('critical');
    expect(clusterTag({ status: ClusterStatus.UPDATING, health: ClusterHealth.HEALTHY }).text).toBe(
      'Updating',
    );
    expect(
      clusterTag({ status: ClusterStatus.ERROR, health: ClusterHealth.UNSPECIFIED }).color,
    ).toBe('critical');
  });
});

describe('driftKindTag', () => {
  it('names every kind', () => {
    [
      DriftKind.SHOOT_WITHOUT_CLUSTER,
      DriftKind.SHOOT_OF_DELETED_CLUSTER,
      DriftKind.CLUSTER_WITHOUT_SHOOT,
      DriftKind.SNAPSHOT_STALE,
      DriftKind.KUBERNETES_VERSION_MISMATCH,
    ].forEach((kind) => expect(driftKindTag(kind).text).not.toBe('Unknown'));
  });
});

describe('timeAgo', () => {
  it('rounds to the largest sensible unit', () => {
    expect(timeAgo(ago(20_000), now)).toBe('just now');
    expect(timeAgo(ago(4 * 60_000), now)).toBe('4 min ago');
    expect(timeAgo(ago(3 * 3_600_000), now)).toBe('3 h ago');
    expect(timeAgo(ago(3 * 86_400_000), now)).toBe('3 d ago');
  });
});

describe('observed', () => {
  it('flags never and too long ago as stale', () => {
    expect(observed(undefined, 2700, now)).toEqual({ text: 'Never', stale: true });
    expect(observed(timestampFromDate(ago(60 * 60_000)), 2700, now).stale).toBe(true);
    expect(observed(timestampFromDate(ago(10 * 60_000)), 2700, now)).toEqual({
      text: '10 min ago',
      stale: false,
    });
  });
});

describe('capacity', () => {
  it('formats cores and memory', () => {
    expect(formatCores(3920n)).toBe('3.9 cores');
    expect(formatCores(96000n)).toBe('96 cores');
    expect(formatMemory(512n)).toBe('512 MiB');
    expect(formatMemory(16384n)).toBe('16 GiB');
    expect(formatMemory(3n * 1024n * 1024n)).toBe('3.0 TiB');
  });
});

describe('poolFlag', () => {
  const seen = timestampFromDate(now);

  it('flags a pool below its minimum or at its maximum', () => {
    expect(poolFlag({ nodes: 1, autoscaleMin: 2, autoscaleMax: 5, observed: seen })?.text).toBe(
      'Below minimum',
    );
    expect(poolFlag({ nodes: 5, autoscaleMin: 1, autoscaleMax: 5, observed: seen })?.text).toBe(
      'At maximum',
    );
    expect(poolFlag({ nodes: 3, autoscaleMin: 1, autoscaleMax: 5, observed: seen })).toBeNull();
  });

  it('says nothing about a pool that was never observed', () => {
    expect(
      poolFlag({ nodes: 0, autoscaleMin: 2, autoscaleMax: 5, observed: undefined }),
    ).toBeNull();
  });
});
