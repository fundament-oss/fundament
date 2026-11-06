/**
 * Waits on a cluster's lifecycle through the API, against a real Gardener. The
 * rules follow the Terraform provider's waits
 * (terraform-provider/internal/provider/cluster_wait.go), so the e2e suite and
 * `tofu apply` agree on when a cluster is ready or gone.
 */

import type { ClusterService, ClusterDetails } from './api/cluster-service.ts';
import { ClusterStatus } from './generated/v1/common_pb.ts';
import { ConnectRpcError } from './api/client.ts';
import { pollUntil } from './poll.ts';

type SyncState = ClusterDetails['syncState'];

// A transport failure (a restarting organization-api, an ingress 503) or an
// internal error keeps the wait going, as in the provider; this many in a row
// end it. Any other answer of the API, such as permission_denied for the
// cluster's own creator, is a result the scenario reports.
const MAX_CONSECUTIVE_ERRORS = 5;

function transient(err: unknown): boolean {
  return !(err instanceof ConnectRpcError) || err.code === 'unknown' || err.code === 'internal';
}

// Wraps an API read so it returns undefined on a transient error, until those
// repeat.
function tolerant<T>(read: () => Promise<T>): () => Promise<T | undefined> {
  let errors = 0;
  return async () => {
    try {
      const result = await read();
      errors = 0;
      return result;
    } catch (err) {
      errors += 1;
      if (!transient(err) || errors >= MAX_CONSECUTIVE_ERRORS) {
        throw err;
      }
      return undefined;
    }
  };
}

function syncGivenUp(sync: SyncState, action: string) {
  if (sync?.outboxStatus === 'failed') {
    throw new Error(`fundament gave up ${action} in Gardener: ${sync.outboxError}`);
  }
}

function describe(status: ClusterStatus, sync: SyncState): string {
  const shoot = sync?.shootMessage ? `, shoot: ${sync.shootMessage}` : '';
  return `status ${ClusterStatus[status]}${shoot}`;
}

/**
 * Wait until cluster-worker has applied every change, Gardener has rolled it
 * out, and the cluster serves. Starting and unhealthy count as serving, as in
 * the provider. changed says whether the caller sent a change: only a change
 * creates a lost cluster again, so without one a lost cluster ends the wait.
 */
export async function waitForClusterRunning(
  service: ClusterService,
  clusterId: string,
  deadlineMs: number,
  changed: boolean,
): Promise<ClusterDetails> {
  let last = 'no status read';
  const read = tolerant(() => service.getCluster(clusterId));
  try {
    return await pollUntil(deadlineMs, `cluster ${clusterId} to run`, async () => {
      const cluster = await read();
      if (!cluster) {
        return undefined;
      }
      last = describe(cluster.status, cluster.syncState);
      syncGivenUp(cluster.syncState, 'applying the cluster');
      switch (cluster.status) {
        case ClusterStatus.RUNNING:
        case ClusterStatus.STARTING:
        case ClusterStatus.UNHEALTHY: {
          const sync = cluster.syncState;
          const applied = !sync?.outboxStatus || sync.outboxStatus === 'completed';
          return applied && !sync?.shootUpdating ? cluster : undefined;
        }
        case ClusterStatus.DELETING:
        case ClusterStatus.STOPPING:
        case ClusterStatus.STOPPED:
          throw new Error(`cluster is ${ClusterStatus[cluster.status]} and will not run`);
        case ClusterStatus.LOST:
          if (changed) {
            return undefined;
          }
          throw new Error('cluster is lost: it no longer exists in Gardener, and only a change to it or its node pools creates it again');
        default:
          return undefined;
      }
    });
  } catch (err) {
    throw new Error(`${(err as Error).message} (last: ${last})`);
  }
}

/**
 * Wait until Gardener has removed the cluster's shoot. GetCluster reports the
 * cluster as not found as soon as its deletion is requested; ListClusters keeps
 * listing it until the shoot is gone.
 */
export async function waitForClusterDeleted(
  service: ClusterService,
  clusterId: string,
  deadlineMs: number,
): Promise<void> {
  let last = 'no status read';
  const read = tolerant(() => service.listClusters());
  try {
    await pollUntil(deadlineMs, `cluster ${clusterId} to be deleted`, async () => {
      const clusters = await read();
      if (!clusters) {
        return undefined;
      }
      const cluster = clusters.find((c) => c.id === clusterId);
      if (!cluster) {
        return true;
      }
      last = describe(cluster.status, cluster.syncState);
      syncGivenUp(cluster.syncState, 'deleting the cluster');
      return undefined;
    });
  } catch (err) {
    throw new Error(`${(err as Error).message} (last: ${last})`);
  }
}
