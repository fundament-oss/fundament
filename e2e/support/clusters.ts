/**
 * Clusters that scenarios and `just test-env up` create. A cluster becomes a
 * shoot only once it has a node pool, so each gets one.
 */

import type { ClusterService } from './api/cluster-service.ts';

const machineType = process.env.E2E_NODE_POOL_MACHINE_TYPE ?? 'local-small';

// Gives the cluster a one-node pool unless it has a pool already, and says
// whether it created one.
export async function ensureNodePool(service: ClusterService, clusterId: string): Promise<boolean> {
  const pools = await service.listNodePools(clusterId);
  if (pools.length > 0) {
    return false;
  }
  await service.createNodePool({ clusterId, name: 'base', machineType, autoscaleMin: 1, autoscaleMax: 1 });
  return true;
}
