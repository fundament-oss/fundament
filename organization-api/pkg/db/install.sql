-- name: InstallListByClusterID :many
SELECT id, cluster_id, plugin_id, created, deleted
FROM tenant.installs
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: InstallGetByID :one
SELECT id, cluster_id, plugin_id, created, deleted
FROM tenant.installs
WHERE id = $1 AND deleted IS NULL;

-- name: InstallCreate :one
INSERT INTO tenant.installs (cluster_id, plugin_id)
VALUES ($1, $2)
RETURNING id;

-- name: InstallDelete :execrows
UPDATE tenant.installs
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
