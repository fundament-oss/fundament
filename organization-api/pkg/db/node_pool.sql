
-- name: NodePoolListByClusterID :many
SELECT id, cluster_id, name, machine_type, autoscale_min, autoscale_max, created, deleted
FROM tenant.node_pools
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: NodePoolGetByID :one
SELECT id, cluster_id, name, machine_type, autoscale_min, autoscale_max, created, deleted
FROM tenant.node_pools
WHERE id = $1 AND deleted IS NULL;

-- name: NodePoolCreate :one
INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: NodePoolUpdate :exec
UPDATE tenant.node_pools
SET autoscale_min = COALESCE(sqlc.narg('autoscale_min'), autoscale_min),
    autoscale_max = COALESCE(sqlc.narg('autoscale_max'), autoscale_max)
WHERE id = $1 AND deleted IS NULL;

-- name: NodePoolDelete :exec
UPDATE tenant.node_pools
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
