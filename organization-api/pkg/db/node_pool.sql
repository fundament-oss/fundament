
-- name: NodePoolListByClusterID :many
SELECT id, cluster_id, name, machine_type, autoscale_min, autoscale_max, created, deleted, region_machine_type_id
FROM tenant.node_pools
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: NodePoolGetByID :one
SELECT id, cluster_id, name, machine_type, autoscale_min, autoscale_max, created, deleted, region_machine_type_id
FROM tenant.node_pools
WHERE id = $1 AND deleted IS NULL;

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
