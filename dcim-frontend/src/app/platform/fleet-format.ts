import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import {
  ClusterHealth,
  ClusterStatus,
  DriftKind,
  type ClusterSnapshot,
  type ClusterSummary,
  type NodePool,
} from '../../generated/fleet/v1/fleet_pb';

/** The colours a tag takes here: the semantic four only. */
export type TagColor = 'neutral' | 'success' | 'warning' | 'critical';

export interface Tag {
  text: string;
  color: TagColor;
}

/** One tag for a cluster: what it is doing, and once it runs, how it is. */
export function clusterTag(cluster: Pick<ClusterSummary, 'status' | 'health'>): Tag {
  switch (cluster.status) {
    case ClusterStatus.RUNNING:
      switch (cluster.health) {
        case ClusterHealth.HEALTHY:
          return { text: 'Healthy', color: 'success' };
        case ClusterHealth.UNHEALTHY:
          return { text: 'Unhealthy', color: 'critical' };
        case ClusterHealth.UNSPECIFIED:
          return { text: 'Running', color: 'neutral' };
        default:
          throw new Error(`unhandled cluster health ${cluster.health satisfies never}`);
      }
    case ClusterStatus.UPDATING:
      return { text: 'Updating', color: 'warning' };
    case ClusterStatus.PROVISIONING:
      return { text: 'Provisioning', color: 'neutral' };
    case ClusterStatus.ERROR:
      return { text: 'Error', color: 'critical' };
    case ClusterStatus.DELETING:
      return { text: 'Deleting', color: 'neutral' };
    case ClusterStatus.STOPPED:
      return { text: 'Stopped', color: 'neutral' };
    case ClusterStatus.UNSPECIFIED:
      return { text: 'Unknown', color: 'neutral' };
    default:
      throw new Error(`unhandled cluster status ${cluster.status satisfies never}`);
  }
}

export function driftKindTag(kind: DriftKind): Tag {
  switch (kind) {
    case DriftKind.SHOOT_WITHOUT_CLUSTER:
      return { text: 'Shoot without cluster', color: 'critical' };
    case DriftKind.SHOOT_OF_DELETED_CLUSTER:
      return { text: 'Shoot of deleted cluster', color: 'warning' };
    case DriftKind.CLUSTER_WITHOUT_SHOOT:
      return { text: 'Cluster without shoot', color: 'critical' };
    case DriftKind.SNAPSHOT_STALE:
      return { text: 'Stale snapshot', color: 'warning' };
    case DriftKind.KUBERNETES_VERSION_MISMATCH:
      return { text: 'Version mismatch', color: 'warning' };
    case DriftKind.UNSPECIFIED:
      return { text: 'Unknown', color: 'neutral' };
    default:
      throw new Error(`unhandled drift kind ${kind satisfies never}`);
  }
}

/** "just now", "4 min ago", "3 h ago", "2 d ago". */
export function timeAgo(at: Date, now: Date): string {
  const seconds = Math.max(0, Math.round((now.getTime() - at.getTime()) / 1000));
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours} h ago`;
  return `${Math.floor(hours / 24)} d ago`;
}

export interface Observed {
  text: string;
  /** Older than stale_after, or never observed. */
  stale: boolean;
}

/** When something was last observed, and whether that is too long ago. */
export function observed(
  at: Timestamp | undefined,
  staleAfterSeconds: number,
  now: Date,
): Observed {
  if (!at) return { text: 'Never', stale: true };
  const date = timestampDate(at);
  return {
    text: timeAgo(date, now),
    stale: now.getTime() - date.getTime() > staleAfterSeconds * 1000,
  };
}

/** Millicores as cores, one decimal below ten. */
export function formatCores(milli: bigint): string {
  const cores = Number(milli) / 1000;
  return `${cores < 10 ? cores.toFixed(1) : Math.round(cores)} cores`;
}

/** Mebibytes in the largest unit that keeps a number of at least one. */
export function formatMemory(mebibytes: bigint): string {
  const mi = Number(mebibytes);
  if (mi >= 1024 * 1024) return `${(mi / 1024 / 1024).toFixed(1)} TiB`;
  if (mi >= 1024) return `${Math.round(mi / 1024)} GiB`;
  return `${mi} MiB`;
}

/** A day, the way the rest of DCIM writes dates. */
export function formatDate(at: Timestamp | undefined): string {
  return at ? timestampDate(at).toISOString().slice(0, 10) : '';
}

/** "3 / 4 of 2–6": ready of present, against the node pools' bounds. */
export function nodesText(
  cluster: Pick<ClusterSummary, 'snapshot' | 'nodesMin' | 'nodesMax'>,
): string {
  const bounds = `${cluster.nodesMin}–${cluster.nodesMax}`;
  const { snapshot } = cluster;
  if (!snapshot?.observed) return `of ${bounds}`;
  return `${snapshot.nodesReady} / ${snapshot.nodes} of ${bounds}`;
}

/** Allocatable CPU and memory, once observed. */
export function capacityText(snapshot: ClusterSnapshot | undefined): string {
  if (!snapshot?.observed) return '';
  return `${formatCores(snapshot.cpuAllocatableM)}, ${formatMemory(snapshot.memoryAllocatableMi)}`;
}

/** Where a node pool stands against its own bounds. */
export function poolFlag(
  pool: Pick<NodePool, 'nodes' | 'autoscaleMin' | 'autoscaleMax' | 'observed'>,
): Tag | null {
  if (!pool.observed) return null;
  if (pool.nodes < pool.autoscaleMin) return { text: 'Below minimum', color: 'warning' };
  if (pool.nodes >= pool.autoscaleMax) return { text: 'At maximum', color: 'warning' };
  return null;
}
