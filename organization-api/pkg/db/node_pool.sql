
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
