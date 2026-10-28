
-- name: NodePoolListByClusterID :many
-- Each pool with its own latest sync row (view node_pool_sync_state).
SELECT
    tenant.node_pools.id,
    tenant.node_pools.cluster_id,
    tenant.node_pools.name,
    tenant.node_pools.machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pools.created,
    tenant.node_pools.deleted,
    tenant.node_pools.region_machine_type_id,
    tenant.node_pools.status,
    tenant.node_pools.status_message,
    tenant.node_pools.status_updated,
    tenant.node_pools.node_count,
    tenant.node_pools.node_count_ready,
    tenant.node_pools.node_count_updated,
    tenant.node_pool_sync_state.outbox_status AS sync_status,
    tenant.node_pool_sync_state.outbox_error AS sync_error
FROM tenant.node_pools
    LEFT JOIN tenant.node_pool_sync_state ON tenant.node_pool_sync_state.node_pool_id = tenant.node_pools.id
WHERE tenant.node_pools.cluster_id = $1 AND tenant.node_pools.deleted IS NULL
ORDER BY tenant.node_pools.created DESC;

-- name: NodePoolGetByID :one
-- The pool with its own latest sync row, as in NodePoolListByClusterID.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.cluster_id,
    tenant.node_pools.name,
    tenant.node_pools.machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pools.created,
    tenant.node_pools.deleted,
    tenant.node_pools.region_machine_type_id,
    tenant.node_pools.status,
    tenant.node_pools.status_message,
    tenant.node_pools.status_updated,
    tenant.node_pools.node_count,
    tenant.node_pools.node_count_ready,
    tenant.node_pools.node_count_updated,
    tenant.node_pool_sync_state.outbox_status AS sync_status,
    tenant.node_pool_sync_state.outbox_error AS sync_error
FROM tenant.node_pools
    LEFT JOIN tenant.node_pool_sync_state ON tenant.node_pool_sync_state.node_pool_id = tenant.node_pools.id
WHERE tenant.node_pools.id = $1 AND tenant.node_pools.deleted IS NULL;

-- name: NodePoolCreate :one
-- region_machine_type_id is the catalog reference (expand phase: the legacy
-- machine_type text column is written alongside it).
INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max, region_machine_type_id)
VALUES ($1, $2, $3, $4, $5, sqlc.narg('region_machine_type_id'))
RETURNING id;

-- name: NodePoolUpdate :execrows
UPDATE tenant.node_pools
SET autoscale_min = COALESCE(sqlc.narg('autoscale_min'), autoscale_min),
    autoscale_max = COALESCE(sqlc.narg('autoscale_max'), autoscale_max)
WHERE id = $1 AND deleted IS NULL;

-- name: NodePoolDelete :execrows
UPDATE tenant.node_pools
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: NodePoolLockCluster :one
-- Locks the cluster of a live node pool, so two deletes of a cluster's pools
-- run one after the other and the second counts what the first left. FOR NO
-- KEY UPDATE conflicts with itself, not with the key-share lock a new pool's
-- foreign key takes. No row: the pool is gone.
SELECT
    tenant.clusters.id,
    tenant.clusters.deleted
FROM tenant.clusters
WHERE tenant.clusters.id = (
    SELECT tenant.node_pools.cluster_id
    FROM tenant.node_pools
    WHERE tenant.node_pools.id = @node_pool_id
      AND tenant.node_pools.deleted IS NULL
)
FOR NO KEY UPDATE;

-- name: ClusterLockForNodePools :one
-- Locks a cluster for a node pool change, as NodePoolLockCluster does. No row:
-- the cluster does not exist.
SELECT
    tenant.clusters.id,
    tenant.clusters.deleted,
    tenant.clusters.region
FROM tenant.clusters
WHERE tenant.clusters.id = @cluster_id
FOR NO KEY UPDATE;

-- name: NodePoolCountActiveByClusterID :one
-- Live pools of a cluster that can run nodes (a maximum above 0), other than
-- the given one. A Shoot needs at least one such worker pool: Gardener refuses
-- one whose every pool has a maximum of 0.
SELECT count(*)
FROM tenant.node_pools
WHERE tenant.node_pools.cluster_id = @cluster_id
  AND tenant.node_pools.deleted IS NULL
  AND tenant.node_pools.autoscale_max > 0
  AND tenant.node_pools.id IS DISTINCT FROM sqlc.narg('except_node_pool_id')::uuid;

-- name: NodePoolCountByClusterID :one
-- Live pools of a cluster, other than the given one, whatever their maximum.
SELECT count(*)
FROM tenant.node_pools
WHERE tenant.node_pools.cluster_id = @cluster_id
  AND tenant.node_pools.deleted IS NULL
  AND tenant.node_pools.id IS DISTINCT FROM sqlc.narg('except_node_pool_id')::uuid;
